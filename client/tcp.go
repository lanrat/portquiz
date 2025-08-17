// Package main provides TCP connectivity testing functionality for the portquiz client.
// It implements functions to test if TCP ports are open on a remote server.
package main

import (
	"bytes"
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
func isOpenTCPMulti(port int, network string) bool {
	for try := uint(0); try < *multi; try++ {
		if !isOpenTCP(port, network) {
			return false
		}
	}
	return true
}

// isOpenTCP tests if a single TCP port is open on the remote server.
// It connects to the port, sends the magic string, and checks for a valid response.
func isOpenTCP(port int, network string) bool {
	// setup
	tcpAddr, _ := net.ResolveTCPAddr(network, net.JoinHostPort(server, fmt.Sprintf("%d", port)))
	d := net.Dialer{Timeout: *timeout}
	connInterface, err := d.Dial(network, tcpAddr.String())
	conn, _ := connInterface.(*net.TCPConn)
	if errors.Is(err, syscall.ECONNREFUSED) || /*errors.Is(err, os.ErrDeadlineExceeded) ||*/ os.IsTimeout(err) {
		// port is closed
		if *verbose {
			log.Printf("%s CLOSED %d", network, port)
		}
		return false
	}
	check(err)
	defer func() {
		if err := conn.Close(); err != nil && *verbose {
			log.Printf("TCP connection close error: %s", err)
		}
	}()

	// setup
	check(conn.SetDeadline(time.Now().Add(*timeout)))
	check(conn.SetNoDelay(true))
	check(conn.SetWriteBuffer(len(magicStringBytes)))
	check(conn.SetReadBuffer(len(magicStringBytes)))

	// send data
	check(conn.SetWriteDeadline(time.Now().Add(*timeout)))
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
