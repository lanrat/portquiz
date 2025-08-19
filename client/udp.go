// Package main provides UDP connectivity testing functionality for the portquiz client.
// It implements functions to test if UDP ports are open on a remote server.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"
)

// isOpenUDPMulti tests a UDP port multiple times to ensure reliability.
// It returns true only if all attempts succeed, false if any attempt fails.
func isOpenUDPMulti(ctx context.Context, port int, network string) bool {
	for try := uint(0); try < *multi; try++ {
		// Check for cancellation before each attempt
		select {
		case <-ctx.Done():
			return false
		default:
		}

		if !isOpenUDP(ctx, port, network) {
			return false
		}
	}
	return true
}

// isOpenUDP tests if a single UDP port is open on the remote server.
// It sends the magic string via UDP and checks for a valid response.
func isOpenUDP(ctx context.Context, port int, network string) bool {
	// Check for cancellation before starting
	select {
	case <-ctx.Done():
		return false
	default:
	}

	// setup
	udpAddr, err := net.ResolveUDPAddr(network, net.JoinHostPort(server, fmt.Sprintf("%d", port)))
	if err != nil {
		if *verbose {
			log.Printf("UDP resolve error for %s:%d: %s", server, port, err)
		}
		return false
	}
	conn, err := net.DialUDP(network, nil, udpAddr)
	if err != nil {
		if *verbose {
			log.Printf("UDP dial error for %s:%d: %s", server, port, err)
		}
		return false
	}
	defer func() {
		if err := conn.Close(); err != nil && *verbose {
			log.Printf("UDP connection close error: %s", err)
		}
	}()

	// tuning
	if err := conn.SetDeadline(time.Now().Add(*timeout)); err != nil && *verbose {
		log.Printf("UDP SetDeadline warning: %s", err)
	}
	if err := conn.SetReadBuffer(len(magicStringBytes) * 2); err != nil && *verbose {
		log.Printf("UDP SetReadBuffer warning: %s", err)
	}

	// Check for cancellation before send
	select {
	case <-ctx.Done():
		return false
	default:
	}

	// send data
	_, err = conn.Write(magicStringBytes)
	if err != nil && *verbose {
		log.Printf("%s write error: %s", network, err)
		return false
	}

	// Check for cancellation before receive
	select {
	case <-ctx.Done():
		return false
	default:
	}

	// receive data
	buffer := make([]byte, 128)
	n, err := conn.Read(buffer)
	if errors.Is(err, syscall.ECONNREFUSED) {
		// port is closed
		if *verbose {
			log.Printf("%s CLOSED %d", network, port)
		}
		return false
	}
	if err != nil && *verbose {
		log.Printf("%s read error: %s", network, err)
		return false
	}

	// check status
	if bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("%s OPEN %d", network, port)
		}
		return true
	} else {
		if *verbose {
			log.Printf("%s, Got data: %d %s", network, port, buffer[:n])
		}
	}

	return false
}
