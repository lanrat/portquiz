package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	magicStringBytes = []byte("portquiz")
	testCert, testCertPool = selfSigned("portquiz.test")
	webTLS = withoutHTTP2(&tls.Config{Certificates: []tls.Certificate{testCert}, NextProtos: []string{"h2", "http/1.1", "acme-tls/1"}})
	os.Exit(m.Run())
}

var (
	testCert     tls.Certificate
	testCertPool *x509.CertPool
)

// selfSigned returns a certificate for host, and a pool that trusts it.
func selfSigned(host string) (tls.Certificate, *x509.CertPool) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: cert}, pool
}

// httpsClient returns a client that reaches the server at addr as
// portquiz.test over HTTPS, trusting the test certificate.
func httpsClient(addr string) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{RootCAs: testCertPool, ServerName: "portquiz.test"},
			ForceAttemptHTTP2: true,
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}
}

// startServer starts the TCP handler on a localhost port, as the server
// does for every redirected port, and returns its address.
func startServer(t *testing.T) string {
	return startServerWeb(t, true)
}

// startServerWeb is startServer with the web server on or off.
func startServerWeb(t *testing.T, web bool) string {
	t.Helper()
	l, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.AcceptTCP()
			if err != nil {
				return
			}
			go handleTCPConnection(c, web)
		}
	}()
	return l.Addr().String()
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, string(body)
}

func TestProbe(t *testing.T) {
	addr := startServer(t)
	res, body := get(t, "http://"+addr+"/probe?n=1")
	if res.StatusCode != 200 || body != webToken {
		t.Errorf("probe: %d %q, want 200 %q", res.StatusCode, body, webToken)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("CORS header %q", got)
	}
	if !strings.HasPrefix(webToken, "portquiz-") || len(webToken) != len("portquiz-")+32 {
		t.Errorf("token %q", webToken)
	}
}

func TestInfo(t *testing.T) {
	addr := startServer(t)
	res, body := get(t, "http://"+addr+"/info")
	var info struct {
		Token, Client, Server, Version string
	}
	if err := json.Unmarshal([]byte(body), &info); err != nil || res.StatusCode != 200 {
		t.Fatalf("info: %d %q: %v", res.StatusCode, body, err)
	}
	if info.Token != webToken || info.Client != "127.0.0.1" || info.Server != "127.0.0.1" || info.Version == "" {
		t.Errorf("info: %+v", info)
	}
}

func TestPage(t *testing.T) {
	addr := startServer(t)
	res, body := get(t, "http://"+addr+"/")
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Type"), "text/html") || !strings.Contains(body, "<title>") {
		t.Errorf("page: %d %s %q", res.StatusCode, res.Header.Get("Content-Type"), body[:min(len(body), 100)])
	}
	if !strings.Contains(res.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Errorf("CSP header %q", res.Header.Get("Content-Security-Policy"))
	}
	for _, f := range []string{"app.js", "core.js", "style.css"} {
		if res, _ := get(t, "http://"+addr+"/"+f); res.StatusCode != 200 {
			t.Errorf("%s: %d", f, res.StatusCode)
		}
	}
	if res, _ := get(t, "http://"+addr+"/missing"); res.StatusCode != 404 {
		t.Errorf("missing file: %d", res.StatusCode)
	}
}

func TestKeepAlive(t *testing.T) {
	addr := startServer(t)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	r := bufio.NewReader(c)
	for range 2 {
		if _, err := io.WriteString(c, "GET /probe HTTP/1.1\r\nHost: x\r\n\r\n"); err != nil {
			t.Fatal(err)
		}
		res, err := http.ReadResponse(r, nil)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		if string(body) != webToken {
			t.Errorf("got %q", body)
		}
	}
}

// exchange writes data to a new connection and returns what comes back
// before the server closes it.
func exchange(t *testing.T, addr, data string) string {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, data); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := io.ReadAtLeast(c, buf, len("portquiz"))
	return string(buf[:n])
}

func TestPasswordStillWorks(t *testing.T) {
	addr := startServer(t)
	if got := exchange(t, addr, "portquiz"); got != "portquiz" {
		t.Errorf("got %q", got)
	}
	if got := exchange(t, addr, "something else"); got != "" {
		t.Errorf("unknown data: got %q, want nothing", got)
	}
}

func TestWebDisabled(t *testing.T) {
	addr := startServerWeb(t, false)
	if got := exchange(t, addr, "GET /probe HTTP/1.1\r\nHost: x\r\n\r\n"); got != "" {
		t.Errorf("with -web=false: got %q, want nothing", got)
	}
}

func TestIsHTTP(t *testing.T) {
	for data, want := range map[string]bool{
		"GET / HTTP/1.1":     true,
		"HEAD / HTTP/1.1":    true,
		"OPTIONS * HTTP/1.1": true,
		"POST /x HTTP/1.1":   true,
		"portquiz":           false,
		"GET":                false,
		"\x16\x03\x01":       false,
		"":                   false,
	} {
		if got := isHTTP([]byte(data)); got != want {
			t.Errorf("isHTTP(%q) = %v", data, got)
		}
	}
}

func TestHTTPS(t *testing.T) {
	addr := startServer(t)
	client := httpsClient(addr)
	res, err := client.Get("https://portquiz.test/probe")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || string(body) != webToken || res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("probe over HTTPS: %d %q", res.StatusCode, body)
	}
	// browsers keep idle HTTP/2 connections, so they would run out of them
	if res.ProtoMajor != 1 {
		t.Errorf("protocol %s, want HTTP/1.1", res.Proto)
	}
	if !slices.Contains(webTLS.NextProtos, "acme-tls/1") {
		t.Error("Let's Encrypt's TLS challenge protocol was removed")
	}
	if res.Header.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS would stop browsers using the plain HTTP page")
	}
	res, err = client.Get("https://portquiz.test/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(page), "<title>") {
		t.Errorf("page over HTTPS: %q", page[:min(len(page), 100)])
	}
	// plain HTTP and the password still work on the same port
	if res, body := get(t, "http://"+addr+"/probe"); res.StatusCode != 200 || body != webToken {
		t.Errorf("plain HTTP: %d %q", res.StatusCode, body)
	}
	if got := exchange(t, addr, "portquiz"); got != "portquiz" {
		t.Errorf("password: got %q", got)
	}
}

func TestInfoNamesHTTPSPage(t *testing.T) {
	addr := startServer(t)
	_, body := get(t, "http://"+addr+"/info")
	var info struct{ HTTPS string }
	if err := json.Unmarshal([]byte(body), &info); err != nil || info.HTTPS != httpsHost {
		t.Errorf("info %q: %v", body, err)
	}
}

func TestTLSWithoutWeb(t *testing.T) {
	addr := startServerWeb(t, false)
	if _, err := httpsClient(addr).Get("https://portquiz.test/probe"); err == nil {
		t.Error("HTTPS answered with -web=false")
	}
}

func TestIsTLS(t *testing.T) {
	for data, want := range map[string]bool{
		"\x16\x03\x01\x02\x00": true,
		"\x16\x03\x03":         true,
		"\x16\x03\x05":         false,
		"\x16\x02\x01":         false,
		"GET / HTTP/1.1":       false,
		"portquiz":             false,
		"\x16":                 false,
	} {
		if got := isTLS([]byte(data)); got != want {
			t.Errorf("isTLS(%q) = %v", data, got)
		}
	}
}

func TestCertHelloAsksForECDSA(t *testing.T) {
	h := certHello("portquiz.test")
	if h.ServerName != "portquiz.test" || len(h.SignatureSchemes) == 0 || h.SupportedCurves[0] != tls.CurveP256 {
		t.Errorf("hello %+v", h)
	}
}
