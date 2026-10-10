// Package main provides TCP connectivity testing functionality for the portquiz client.
// It implements functions to test if TCP ports are open on a remote server.
package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"
)

// checkTCP tests a single TCP port once: it connects, sends the magic string,
// and waits for the server to send it back. It returns the outcome and, for an
// open port, the slower of the connection and the reply, each of which takes
// about one round trip.
func checkTCP(ctx context.Context, t *target, port int, timeout time.Duration) (outcome, time.Duration) {
	addr := net.JoinHostPort(t.addr.String(), strconv.Itoa(port))
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	c, err := d.DialContext(ctx, t.network, addr)
	if err != nil {
		if *verbose {
			log.Printf("%s %d dial error: %s", t.kind, port, err)
		}
		switch {
		case errors.Is(err, syscall.ECONNREFUSED):
			return outcomeRefused, 0
		case os.IsTimeout(err):
			return outcomeTimeout, 0
		default:
			return outcomeFailed, 0
		}
	}
	connected := time.Now()
	conn := c.(*net.TCPConn)
	defer func() {
		if err := conn.Close(); err != nil && *verbose {
			log.Printf("%s %d close error: %s", t.kind, port, err)
		}
	}()
	// unblock any pending read or write as soon as the context is canceled
	stopAfter := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stopAfter()

	if err := conn.SetDeadline(connected.Add(timeout)); err != nil && *verbose {
		log.Printf("%s %d SetDeadline warning: %s", t.kind, port, err)
	}
	if err := conn.SetNoDelay(true); err != nil && *verbose {
		log.Printf("%s %d SetNoDelay warning: %s", t.kind, port, err)
	}

	if _, err := conn.Write(magicStringBytes); err != nil {
		if *verbose {
			log.Printf("%s %d write error: %s", t.kind, port, err)
		}
		return outcomeFailed, 0
	}

	buffer := make([]byte, 128)
	n, err := conn.Read(buffer)
	if err != nil {
		if *verbose {
			log.Printf("%s %d read error: %s", t.kind, port, err)
		}
		if os.IsTimeout(err) && ctx.Err() == nil {
			return outcomeTimeout, 0
		}
		return outcomeFailed, 0
	}
	if !bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("%s %d unexpected reply: %q", t.kind, port, buffer[:n])
		}
		return outcomeFailed, 0
	}
	return outcomeOpen, max(connected.Sub(start), time.Since(connected))
}
