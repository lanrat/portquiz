package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	magicStringBytes = []byte("portquiz")
	os.Exit(m.Run())
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
