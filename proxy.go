package obscura

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"sync"
	"strings"
	"time"
)

// ProxyServer 是一个本地 MITM HTTP/HTTPS 代理。
// obscura 通过 --proxy http://127.0.0.1:<port> 将全部流量路由到此代理。
// HTTP 请求直接转发；HTTPS CONNECT 请求做 MITM 解密后转发。
type ProxyServer struct {
	Port       int        // 0 = 随机端口
	HTTPClient *http.Client

	ln      net.Listener
	certs   map[string]tls.Certificate
	certMu  sync.Mutex
	srv     *http.Server

	// OnRequest 在请求发出前调用，可修改 req。
	OnRequest func(req *http.Request)
	// OnResponse 在收到响应后调用。
	OnResponse func(req *http.Request, resp *http.Response)
}

// Start 启动代理服务器，返回实际监听的地址。
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

// Stop 停止代理服务器。
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

// handleHTTP 处理普通 HTTP 请求（非 CONNECT）。
func (p *ProxyServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	// 构造目标 URL
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

// handleConnect 处理 HTTPS CONNECT（MITM 模式）。
func (p *ProxyServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	client, _, err := hj.Hijack()
	if err != nil {
		return
	}

	// 告诉 obscura 隧道已建立
	client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// 生成自签证书（obscura 启用了 IgnoreCertErrors，会接受）
	cert := p.getCert(r.Host)
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsClient := tls.Server(client, tlsCfg)
	defer tlsClient.Close()

	// 在 TLS 连接上循环读取 HTTP 请求
	buf := bufio.NewReader(tlsClient)
	for {
		req, err := http.ReadRequest(buf)
		if err != nil {
			return
		}

		// 补全请求 URL（从 CONNECT 中解析出的，这里用绝对 URL）
		req.URL.Scheme = "https"
		req.URL.Host = strings.Split(r.Host, ":")[0]
		req.RequestURI = ""

		if p.OnRequest != nil {
			p.OnRequest(req)
		}

		resp, err := p.HTTPClient.Do(req)
		if err != nil {
			log.Printf("[Proxy] 请求失败: %v", err)
			resp = &http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
				ProtoMajor: 1, ProtoMinor: 1,
				Header:        http.Header{},
				Body:          io.NopCloser(strings.NewReader("")),
				ContentLength: 0,
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
	cert, _ := generateCert(host)
	p.certs[host] = cert
	return cert
}

func generateCert(host string) (tls.Certificate, error) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: host,
		},
		DNSNames: []string{host},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}, nil
}
