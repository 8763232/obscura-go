package obscura

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProxyServer 是一个本地 MITM HTTP/HTTPS 代理。
// obscura 通过 --proxy http://127.0.0.1:<port> 路由流量到此代理。
// HTTP 请求直接转发（完整可见）；HTTPS CONNECT 做 MITM 解密后转发。
// obscura 已硬编码 danger_accept_invalid_certs(true)，接受代理的自签证书。
type ProxyServer struct {
	Port       int
	HTTPClient *http.Client

	ln      net.Listener
	srv     *http.Server
	certs   map[string]tls.Certificate
	certMu  sync.Mutex

	OnRequest  func(req *http.Request)
	OnResponse func(req *http.Request, resp *http.Response)
	OnConnect  func(host string)
}

// Start 启动代理服务器。
func (p *ProxyServer) Start() (addr string, err error) {
	if p.HTTPClient == nil {
		p.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 30 * time.Second,
		}
	}
	p.certs = make(map[string]tls.Certificate)

	p.ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p.Port))
	if err != nil {
		return "", err
	}

	p.srv = &http.Server{Handler: http.HandlerFunc(p.handle)}
	go p.srv.Serve(p.ln)

	addr = fmt.Sprintf("http://127.0.0.1:%d", p.ln.Addr().(*net.TCPAddr).Port)
	return addr, nil
}

func (p *ProxyServer) Stop() error {
	if p.ln != nil {
		return p.ln.Close()
	}
	return nil
}

func (p *ProxyServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

func (p *ProxyServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.String()
	if r.URL.Host == "" {
		targetURL = fmt.Sprintf("http://%s%s", r.Host, r.URL.Path)
	}
	outReq, _ := http.NewRequestWithContext(context.Background(), r.Method, targetURL, r.Body)
	outReq.Header = r.Header.Clone()
	if p.OnRequest != nil {
		p.OnRequest(outReq)
	}
	resp, err := p.HTTPClient.Do(outReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if p.OnResponse != nil {
		p.OnResponse(outReq, resp)
	}
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *ProxyServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	if p.OnConnect != nil {
		p.OnConnect(r.Host)
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	client, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer client.Close()

	client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// MITM: 自签证书做 TLS（obscura 已 hardcode danger_accept_invalid_certs(true)）
	host := strings.Split(r.Host, ":")[0]
	cert := p.getCert(host)
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsClient := tls.Server(client, tlsCfg)
	defer tlsClient.Close()

	if err := tlsClient.Handshake(); err != nil {
		log.Printf("[Proxy] TLS握手失败 (%s): %v", host, err)
		return
	}

	buf := bufio.NewReader(tlsClient)
	for {
		req, err := http.ReadRequest(buf)
		if err != nil {
			return
		}
		req.URL.Scheme = "https"
		req.URL.Host = host
		req.RequestURI = ""

		if p.OnRequest != nil {
			p.OnRequest(req)
		}

		resp, err := p.HTTPClient.Do(req)
		if err != nil {
			log.Printf("[Proxy] 请求失败: %v", err)
			resp = &http.Response{
				StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway",
				ProtoMajor: 1, ProtoMinor: 1, ContentLength: 0,
				Header: http.Header{},
				Body:   io.NopCloser(strings.NewReader("")),
			}
		}

		if p.OnResponse != nil {
			p.OnResponse(req, resp)
		}

		if resp.Body != nil {
			resp.Write(tlsClient)
			resp.Body.Close()
		}
	}
}

func (p *ProxyServer) getCert(host string) tls.Certificate {
	p.certMu.Lock()
	defer p.certMu.Unlock()
	if cert, ok := p.certs[host]; ok {
		return cert
	}
	// 用 MITM CA 签发每主机证书（而非自签），obscura 信任此 CA
	cert, _ := signHostCert(host)
	p.certs[host] = cert
	return cert
}
