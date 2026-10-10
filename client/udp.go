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
	"os"
	"syscall"
	"time"
)

// checkUDP tests a single UDP port once: it sends the magic string and waits
// for the server to send it back. Each check uses its own connected socket so
// that an ICMP port unreachable from a rejecting firewall is reported at once
// as refused; an unconnected socket shared by all checks never sees those, so
// every rejected port would wait out the full timeout instead.
func checkUDP(ctx context.Context, network string, addr *net.IPAddr, port int, timeout time.Duration) (outcome, time.Duration) {
	conn, err := dialUDP(network, &net.UDPAddr{IP: addr.IP, Port: port, Zone: addr.Zone})
	if err != nil {
		if *verbose {
			log.Printf("%s %d dial error: %s", network, port, err)
		}
		return outcomeFailed, 0
	}
	defer func() { _ = conn.Close() }()
	// unblock a pending read as soon as the context is canceled
	stopAfter := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stopAfter()

	start := time.Now()
	if err := conn.SetDeadline(start.Add(timeout)); err != nil && *verbose {
		log.Printf("%s %d SetDeadline warning: %s", network, port, err)
	}
	if _, err := conn.Write(magicStringBytes); err != nil {
		return outcomeFailed, 0
	}
	buffer := make([]byte, 2048)
	n, err := conn.Read(buffer)
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return outcomeRefused, 0
	case err != nil && ctx.Err() == nil && errors.Is(err, os.ErrDeadlineExceeded):
		return outcomeTimeout, 0
	case err != nil:
		return outcomeFailed, 0
	case !bytes.HasPrefix(buffer[:n], magicStringBytes):
		return outcomeFailed, 0
	}
	return outcomeOpen, time.Since(start)
}

// dialUDP connects a UDP socket to addr. When testing a server on the same
// host, the OS can pick a local port equal to the port being tested, which
// connects the socket to itself so it reads back its own packet and reports a
// false open. Like Go's TCP dialer, it detects this and tries again.
func dialUDP(network string, addr *net.UDPAddr) (*net.UDPConn, error) {
	for range 3 {
		conn, err := net.DialUDP(network, nil, addr)
		if err != nil {
			return nil, err
		}
		if conn.LocalAddr().String() != conn.RemoteAddr().String() {
			return conn, nil
		}
		_ = conn.Close()
	}
	return nil, fmt.Errorf("UDP socket connected to itself on %s", addr)
}
