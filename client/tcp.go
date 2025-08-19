// Package main provides TCP connectivity testing functionality for the portquiz client.
// It implements functions to test if TCP ports are open on a remote server.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"syscall"
	"time"
)

// isOpenTCPMulti tests a TCP port multiple times to ensure reliability.
// It returns true only if all attempts succeed, false if any attempt fails.
func isOpenTCPMulti(ctx context.Context, port int, network string) bool {
	for try := uint(0); try < *multi; try++ {
		// Check for cancellation before each attempt
		select {
		case <-ctx.Done():
			return false
		default:
		}

		if !isOpenTCP(ctx, port, network) {
			return false
		}
	}
	return true
}

// isOpenTCP tests if a single TCP port is open on the remote server.
// It connects to the port, sends the magic string, and checks for a valid response.
func isOpenTCP(ctx context.Context, port int, network string) bool {
	// Check for cancellation before starting
	select {
	case <-ctx.Done():
		return false
	default:
	}

	// setup
	tcpAddr, err := net.ResolveTCPAddr(network, net.JoinHostPort(server, fmt.Sprintf("%d", port)))
	if err != nil {
		if *verbose {
			log.Printf("TCP resolve error for %s:%d: %s", server, port, err)
		}
		return false
	}
	d := net.Dialer{Timeout: *timeout}
	connInterface, err := d.DialContext(ctx, network, tcpAddr.String())
	conn, ok := connInterface.(*net.TCPConn)
	if !ok && err == nil {
		// This shouldn't happen with TCP dialing, but handle it gracefully
		if *verbose {
			log.Printf("TCP dial returned unexpected connection type")
		}
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) || os.IsTimeout(err) {
		// port is closed
		if *verbose {
			log.Printf("%s CLOSED %d", network, port)
		}
		return false
	}
	if err != nil {
		if *verbose {
			log.Printf("TCP dial error for %s:%d: %s", server, port, err)
		}
		return false
	}
	defer func() {
		if err := conn.Close(); err != nil && *verbose {
			log.Printf("TCP connection close error: %s", err)
		}
	}()

	// setup
	if err := conn.SetDeadline(time.Now().Add(*timeout)); err != nil && *verbose {
		log.Printf("TCP SetDeadline warning: %s", err)
	}
	if err := conn.SetNoDelay(true); err != nil && *verbose {
		log.Printf("TCP SetNoDelay warning: %s", err)
	}
	if err := conn.SetWriteBuffer(len(magicStringBytes)); err != nil && *verbose {
		log.Printf("TCP SetWriteBuffer warning: %s", err)
	}
	if err := conn.SetReadBuffer(len(magicStringBytes)); err != nil && *verbose {
		log.Printf("TCP SetReadBuffer warning: %s", err)
	}

	// send data
	if err := conn.SetWriteDeadline(time.Now().Add(*timeout)); err != nil && *verbose {
		log.Printf("TCP SetWriteDeadline warning: %s", err)
	}
	_, err = conn.Write(magicStringBytes)
	if err != nil && *verbose {
		log.Printf("%s write error: %s", network, err)
		return false
	}

	// receive data
	buffer := make([]byte, 128)
	n, err := conn.Read(buffer)
	if err != nil && *verbose {
		log.Printf("%s read error: %s", network, err)
		return false
	}

	if bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("%s OPEN %d", network, port)
		}
		return true
	} else {
		if *verbose {
			log.Printf("%s, Got data: %s", network, buffer[:n])
		}
	}

	return false
}
