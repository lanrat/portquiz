// Package main provides TCP server functionality for the portquiz server.
// It handles incoming TCP connections and responds with the magic string when detected.
package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"time"
)

// tcpServer starts a TCP server on the specified address and handles incoming connections.
// It accepts connections in a loop and spawns goroutines to handle each connection.
func tcpServer(ctx context.Context, listenAddr string) error {
	addr, err := net.ResolveTCPAddr("tcp", listenAddr)
	if err != nil {
		return err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("starting TCP server on %s", listenAddr)
	defer func() {
		if err := l.Close(); err != nil {
			log.Printf("TCP listener close error: %s", err)
		}
	}()

	// Start a goroutine to handle context cancellation
	go func() {
		<-ctx.Done()
		if err := l.Close(); err != nil {
			log.Printf("TCP listener close on context cancel error: %s", err)
		}
	}()

	web := *webEnabled
	for {
		// Check for cancellation before potentially blocking call
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c, err := l.AcceptTCP()
		if err != nil {
			// Check if context was cancelled
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return err
		}
		go handleTCPConnection(c, web)
	}
}

// handleTCPConnection processes a single TCP connection.
// It reads data from the connection, checks for the magic string, and responds accordingly.
// HTTP requests and, with -acme, TLS connections, from the browser test
// page, are handed to the web server if web is set.
func handleTCPConnection(c *net.TCPConn, web bool) {
	kind := "TCP"
	handedOff := false
	defer func() {
		if handedOff {
			return
		}
		if err := c.Close(); err != nil && *verbose {
			log.Printf("TCP connection close error: %s", err)
		}
	}()
	if err := c.SetNoDelay(true); err != nil && *verbose {
		log.Printf("TCP SetNoDelay warning: %s", err)
	}
	if err := c.SetDeadline(time.Now().Add(*timeout)); err != nil && *verbose {
		log.Printf("TCP SetDeadline warning: %s", err)
	}

	if *verbose {
		log.Printf("Serving %s %s\n", kind, c.RemoteAddr())
	}

	buffer := make([]byte, 128)

	n, err := c.Read(buffer)
	if err != nil {
		if *verbose {
			log.Printf("TCP Read Error from %s: %s", c.RemoteAddr(), err)
		}
		return
	}
	if *verbose {
		log.Printf("[%s], Got data from [%s]: %s", kind, c.RemoteAddr(), buffer[:n])
	}
	if bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("[%s] PORTQUIZ from %s", kind, c.RemoteAddr())
		}
		_, err := c.Write(magicStringBytes)
		if err != nil {
			if *verbose {
				log.Printf("TCP Write Error from %s: %s", c.RemoteAddr(), err)
			}
			return
		}
		// Wait for the client to close first, so the TIME_WAIT state of the
		// closed connection stays on the client. Every port is redirected to
		// this one listener, so here a connection is told apart only by the
		// client's address and source port; holding TIME_WAIT on the server
		// can make it ignore a new connection that reuses that source port,
		// costing the client a one second retransmission.
		_, _ = io.Copy(io.Discard, c)
		return
	}
	if web && isHTTP(buffer[:n]) {
		if *verbose {
			log.Printf("[%s] HTTP from %s", kind, c.RemoteAddr())
		}
		handedOff = true
		serveHTTP(c, buffer[:n])
		return
	}
	if web && webTLS != nil && isTLS(buffer[:n]) {
		if *verbose {
			log.Printf("[%s] TLS from %s", kind, c.RemoteAddr())
		}
		handedOff = true
		serveTLS(c, buffer[:n])
	}
}
