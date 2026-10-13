// Package main serves the browser test page over HTTPS with certificates
// from Let's Encrypt. A page loaded over HTTPS may only make HTTPS requests,
// so with -acme the server answers TLS on every port, alongside HTTP and the
// client's password, and the page tests TCP ports over HTTPS.
package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"log"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

var (
	acmeHosts     = flag.String("acme", "", "comma separated DNS names of this server to get Let's Encrypt certificates for, to serve the browser test page over HTTPS too")
	acmeEmail     = flag.String("acme-email", "", "email address for Let's Encrypt notices about the certificates (optional)")
	acmeCache     = flag.String("acme-cache", "/var/lib/portquiz/acme", "directory to keep the certificates and Let's Encrypt account in")
	acmeDirectory = flag.String("acme-directory", acme.LetsEncryptURL, "ACME directory URL, such as Let's Encrypt's staging server for testing")
)

var (
	// webTLS is the TLS configuration for HTTPS on every port, or nil when
	// the server has no certificates. Set before the servers start.
	webTLS *tls.Config
	// acmeManager gets and renews the certificates, and answers Let's
	// Encrypt's challenges. Nil without -acme.
	acmeManager *autocert.Manager
	// httpsHost is the name the HTTPS page is reached at, for the plain
	// page to link to.
	httpsHost string
)

// setupACME prepares HTTPS with certificates from Let's Encrypt for the
// names in -acme, if any. Certificates are requested once the servers are
// up, since Let's Encrypt checks the server answers for each name, and are
// renewed before they expire.
func setupACME() error {
	var hosts []string
	for _, h := range strings.Split(*acmeHosts, ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return nil
	}
	if !*webEnabled {
		return errors.New("-acme needs the browser test page, which -web=false turns off")
	}
	if !*tcp {
		return errors.New("-acme needs the TCP server")
	}
	acmeManager = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(hosts...),
		Cache:      autocert.DirCache(*acmeCache),
		Email:      *acmeEmail,
		Client:     &acme.Client{DirectoryURL: *acmeDirectory},
	}
	webTLS = withoutHTTP2(acmeManager.TLSConfig())
	httpsHost = hosts[0]
	go func() {
		// give the servers and firewall rules time to start
		time.Sleep(2 * time.Second)
		for _, h := range hosts {
			if _, err := acmeManager.GetCertificate(certHello(h)); err != nil {
				log.Printf("getting a certificate for %s: %s", h, err)
				continue
			}
			log.Printf("certificate for %s ready, serving https://%s/", h, h)
		}
	}()
	return nil
}

// certHello is a TLS hello like a modern browser's, for getting a
// certificate before the first visit: an ECDSA one, which is what browsers
// are then given.
func certHello(host string) *tls.ClientHelloInfo {
	return &tls.ClientHelloInfo{
		ServerName:       host,
		CipherSuites:     []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
		SignatureSchemes: []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256},
		SupportedCurves:  []tls.CurveID{tls.CurveP256},
	}
}

// withoutHTTP2 returns c offering HTTP/1.1 but not HTTP/2. The page makes
// one request to each port, and browsers keep idle HTTP/2 connections open
// rather than closing them to make room for new ones, so after a couple of
// hundred ports Chromium runs out of connections. Other protocols, such as
// "acme-tls/1" for Let's Encrypt's TLS challenge, stay.
func withoutHTTP2(c *tls.Config) *tls.Config {
	c.NextProtos = slices.DeleteFunc(slices.Clone(c.NextProtos), func(p string) bool { return p == "h2" })
	return c
}

// isTLS reports whether data, the start of a connection, is a TLS
// handshake: a handshake record of TLS 1.0 or later.
func isTLS(data []byte) bool {
	return len(data) >= 3 && data[0] == 0x16 && data[1] == 0x03 && data[2] <= 0x04
}
