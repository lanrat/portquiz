// Package main provides TCP server functionality for the portquiz server.
// It handles incoming TCP connections and responds with the magic string when detected.
package main

import (
	"bytes"
	"context"
	"log"
	"net"
	"time"
)

// tcpServer starts a TCP server on the specified address and handles incoming connections.
// It accepts connections in a loop and spawns goroutines to handle each connection.
// TODO: detect canceled context
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

	for {
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
		go handleTCPConnection(c)
	}
}

// handleTCPConnection processes a single TCP connection.
// It reads data from the connection, checks for the magic string, and responds accordingly.
func handleTCPConnection(c *net.TCPConn) {
	kind := "TCP"
	defer func() {
		if err := c.Close(); err != nil && *verbose {
			log.Printf("TCP connection close error: %s", err)
		}
	}()
	check(c.SetNoDelay(true))
	check(c.SetDeadline(time.Now().Add(*timeout)))

	if *verbose {
		log.Printf("Serving %s %s\n", kind, c.RemoteAddr())
	}

	buffer := make([]byte, 128)

	n, err := c.Read(buffer)
	if err != nil && *verbose {
		log.Printf("TCP Read Error from %s: %s", c.RemoteAddr(), err)
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
		if err != nil && *verbose {
			log.Printf("TCP Write Error from %s: %s", c.RemoteAddr(), err)
			return
		}
	}
}
