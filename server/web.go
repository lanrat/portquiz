// Package main serves the browser test page. Browsers cannot open raw
// sockets, so the page tests TCP ports with HTTP requests instead: every
// port is redirected to the server, so it answers HTTP on every port, and
// the page checks each answer carries this run's token. UDP ports are
// tested with STUN, see stun.go.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

var webEnabled = flag.Bool("web", true, "serve the browser test page, and answer its HTTP and STUN checks on every port")

//go:embed web/index.html web/style.css web/app.js web/core.js
var webFiles embed.FS

var (
	// webToken is sent in answer to every probe, so the page can tell the
	// server's answers from anything else that answers on a port. It is
	// random for each run, so it is never the password.
	webToken = newToken()

	webOnce     sync.Once
	webListener *connListener
)

// httpMethods are how HTTP requests start; a connection that starts with one
// is handed to the web server instead of being checked for the password.
var httpMethods = [][]byte{[]byte("GET "), []byte("HEAD "), []byte("OPTIONS "), []byte("POST ")}

// isHTTP reports whether data, the start of a connection, is an HTTP request.
func isHTTP(data []byte) bool {
	for _, m := range httpMethods {
		if bytes.HasPrefix(data, m) {
			return true
		}
	}
	return false
}

// serveHTTP hands a connection whose first bytes, already read, are first
// to the web server, which then owns it.
func serveHTTP(c net.Conn, first []byte) {
	webOnce.Do(startWebServer)
	// the web server sets its own deadlines
	_ = c.SetDeadline(time.Time{})
	webListener.conns <- &prefixConn{Conn: c, r: io.MultiReader(bytes.NewReader(first), c)}
}

// serveTLS hands a TLS connection whose first bytes, already read, are
// first to the web server, which then owns it and does the handshake.
func serveTLS(c net.Conn, first []byte) {
	webOnce.Do(startWebServer)
	_ = c.SetDeadline(time.Time{})
	webListener.conns <- tls.Server(&prefixConn{Conn: c, r: io.MultiReader(bytes.NewReader(first), c)}, webTLS)
}

// startWebServer starts the HTTP server that handles connections passed to
// serveHTTP.
func startWebServer() {
	webListener = &connListener{conns: make(chan net.Conn)}
	srv := &http.Server{
		Handler:           webHandler(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Second,
		// browsers close idle connections themselves, which leaves TIME_WAIT
		// on the browser's side rather than the server's, see
		// handleTCPConnection
		IdleTimeout:    2 * time.Minute,
		MaxHeaderBytes: 8 << 10,
		ErrorLog:       log.New(io.Discard, "", 0),
	}
	if *verbose {
		// TLS handshake failures and the like
		srv.ErrorLog = log.Default()
	}
	go func() { _ = srv.Serve(webListener) }()
}

// webHandler serves the page, the probes it sends to each port, and the
// details the page needs.
func webHandler() http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(static)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /probe", func(w http.ResponseWriter, r *http.Request) {
		// the page is on another port, so another origin
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, webToken)
	})
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		client, _, _ := net.SplitHostPort(r.RemoteAddr)
		server := ""
		if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
			server, _, _ = net.SplitHostPort(local.String())
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": Version,
			"token":   webToken,
			"client":  client,
			"server":  server,
			"tcp":     *tcp,
			"udp":     *udp,
			// the name of the HTTPS page, if there is one
			"https": httpsHost,
		})
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
	mux.Handle("GET /{file}", files)
	var h http.Handler = mux
	if acmeManager != nil {
		// Let's Encrypt checks the server answers for each name with a
		// plain HTTP request to port 80, which arrives here
		h = acmeManager.HTTPHandler(mux)
	}
	return securityHeaders(keepHTTP(h))
}

// keepHTTP sends HTTPS requests for the plain page, marked with ?http, back
// to HTTP. Browsers now try HTTPS first for http:// links, and the server
// has a certificate, so the HTTPS page's link to the plain one would land on
// HTTPS again; a redirect straight back to the same http:// address is the
// sign browsers take to fall back to HTTP.
func keepHTTP(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && r.URL.Path == "/" && r.URL.Query().Has("http") {
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, "http://"+r.Host+r.URL.RequestURI(), http.StatusFound)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// securityHeaders adds headers that keep the page to its own scripts. It
// connects to every port of the server, which are other origins.
func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; connect-src *; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

// newToken returns a random token for webToken.
func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "portquiz-" + hex.EncodeToString(b)
}

// connListener is a net.Listener whose connections are handed to it by
// serveHTTP rather than accepted from a socket.
type connListener struct {
	conns chan net.Conn
}

func (l *connListener) Accept() (net.Conn, error) { return <-l.conns, nil }
func (l *connListener) Close() error              { return nil }
func (l *connListener) Addr() net.Addr            { return &net.TCPAddr{} }

// prefixConn is a connection whose first bytes were already read; reads
// return them again first.
type prefixConn struct {
	net.Conn
	r io.Reader
}

func (c *prefixConn) Read(b []byte) (int, error) { return c.r.Read(b) }
