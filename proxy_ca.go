package obscura

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"
)

const caCertPEM = `-----BEGIN CERTIFICATE-----
MIIC2zCCAcOgAwIBAgIUe4pwlWUch6toYWLf0d7cHJCz6GowDQYJKoZIhvcNAQEL
BQAwHTEbMBkGA1UEAwwSb2JzY3VyYS1nbyBNSVRNIENBMB4XDTI2MDUyMjIzNTQ1
NFoXDTM2MDUxOTIzNTQ1NFowHTEbMBkGA1UEAwwSb2JzY3VyYS1nbyBNSVRNIENB
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0gHlOrkWrR0xCuvXzFH3
xFBWnw3LmAp7v/MyM+AanjrsFFtQ4IgDpY5ecbm09ZviF/cJwJfXiPCO/t57nS0I
OEPq9qZmsno4pwGjE4JcajHz4vSKLNO1mYIAn5UPE732p/bPaB5oFUBpoOAatwCJ
pKHNmEAJZx7MEU/jbi5zfdVVBUlKJXSE7zgPo99CyrEfLzThWW0DEZGHxik10ZRR
KCUM81DF7uIgRI+ThMBBxii0+UOqTbOh0D0/qpF5Xqj/rdo749cB8km38t55GR/c
dO9xmz5OHWGIBez5fEoEOK1H8b47V4hJjt5vtp9Tylio7rwTtrF1A8TBcsLg2kGJ
twIDAQABoxMwETAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQAP
nMNLxpnUZmxdcP0J9tdIaPzhrHPY4muBoY9mzWibsF441q7cFRHNl/1obyp4yklf
OXMuAqZ4NdW+cfmkd30kjWD7zxg1/f9MAlzh5ghIKdawvoq0fJKSwgiEoRXgAykj
dZszrQRFTBWGmvu0XPZSFHkPwIVpANvmNR4W2Fy7/+eEJ4cOqAtWxR8O4Va2qtat
Xm7PtnW1pp3sI+q3TD47o100afxXinm98fCD3tJAd7t6VMbC+GMh5Er7YyfksVym
Ivs3HXMEG8rTaTLvB2w1p+7PsGB8mGXnJsnYDbjWaTLyVpLYu4RtT9+CPXlPPtb3
XjwsP1NmANZbK9FqjDML
-----END CERTIFICATE-----
`

const caKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0gHlOrkWrR0xCuvXzFH3xFBWnw3LmAp7v/MyM+AanjrsFFtQ
4IgDpY5ecbm09ZviF/cJwJfXiPCO/t57nS0IOEPq9qZmsno4pwGjE4JcajHz4vSK
LNO1mYIAn5UPE732p/bPaB5oFUBpoOAatwCJpKHNmEAJZx7MEU/jbi5zfdVVBUlK
JXSE7zgPo99CyrEfLzThWW0DEZGHxik10ZRRKCUM81DF7uIgRI+ThMBBxii0+UOq
TbOh0D0/qpF5Xqj/rdo749cB8km38t55GR/cdO9xmz5OHWGIBez5fEoEOK1H8b47
V4hJjt5vtp9Tylio7rwTtrF1A8TBcsLg2kGJtwIDAQABAoIBABKkQ25ihq5IRJW1
GLtU3VsKTJ4i0dtFtvVzh5XOQ16fWVx3PKcpu7Ui/aQ3uWYB9+Brt+xmLiZEQFVE
d5GcsTmZYc6SN9SI/+VnQkwVitGMbJtNXMSc6GZfgWGpECUO/EmtoXybEl8skBPN
QOHUxOMdz8u/h3YlaDTzM/uQWQUm1PaD6jExY62Yu89jAQF3lVVnp9nQ6U42HMbu
TxdPY0kTcgE7iHj+ElFwHnLn/cULlPIdnfs5MDVvf63RpD2+t75o6BfwrhHmNWU+
2nGv74SaR/eBeMJGdyOS616q010ENPeQLCjUX8XMEPzhDfBTe33WETgUHC8yRMbl
0KfyqJUCgYEA+Wv8EfYoFtIH5mtHwPu8axXiQqDTEo+h1vnYyMNHMzKUTBCrkOOt
zdiFIdVK1XCv7FT5bu7/kjzdPRvp8FMKGftX83CT8+E/VtRTAMHG7fwJjCRyDbwn
zFRkEK7aIRBTPSOiA1UegvuqovNwTFdiN6mYivdkcIvyMoVQz3nvZF0CgYEA14vM
JKnqU4kXuGL0eQI3zFMDk1MzsCqn2ksF8kEViMiJuygmWnJwQ+wCtEOazD3TWMWf
AOqFtFNWocEBFUt5Vt95vvDc1J1F0T8ZARuBc4UpjRxvm+ZPKt7sbfVRss2LQ0B5
NsFT/RcBP1UPFuDNkdeE9Yspiah+iBlyWf7FBSMCgYAEskXSyMHEfDvt2MNHHPZV
Rdo2yvRuewnfFGFClnq2uhMUw5OXbNIO+C65jlyUXETTvF3d+t4RENhRmD71aXrd
NmBXkx0WEH2y0tilQQDP5lj/rIgBPjO5ozUnI0O0L6yBkDQyv92NNdPmsZLBvTt6
NNVMeJAJlnj+/oehAHjDeQKBgQCDQEJP9ROWSH2kLsWVRg96IPalaF2qIV9Soqp9
SLp3Lz4HNDyeiN7pzTYcVKpXQjKG9NeMtEI0eybemmsxb2L0zmIRLhQad6ZC83wj
W39pO6YAolcoBIcioNoxXtef3F+31PO6ruCY1cBRs1bb5InpC+aPqmzhwTNDZtNm
D6gjJwKBgQDEe/PxOy5HoJ2A9/BMxoMurv8q6P6ajKSx7LfHgfZ+0Bg0JQBIvwZ8
fu82br0m4Gnj7O7syic3FIrdmxaO7DIshGmptnNNU5YRMpFfX1xZ5X8gZHUkjX4+
6GLLES+Bf2f21CUO8yk0yxZ5yUeJ3Qi624PbyfP97mP3iHaxbN0aSg==
-----END RSA PRIVATE KEY-----
`

var caTLSCert *tls.Certificate

func init() {
	cert, err := tls.X509KeyPair([]byte(caCertPEM), []byte(caKeyPEM))
	if err != nil {
		panic("failed to load MITM CA: " + err.Error())
	}
	caTLSCert = &cert
}

// signHostCert 用 MITM CA 为 host 签发证书，obscura 信任此 CA。
func signHostCert(host string) (tls.Certificate, error) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
	}

	caCert, _ := x509.ParseCertificate(caTLSCert.Certificate[0])
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caTLSCert.PrivateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
