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

func isOpenTCPMulti(port int) bool {
	for try := uint(0); try < *multi; try++ {
		if !isOpenTCP(port) {
			return false
		}
	}
	return true
}

func isOpenTCP(port int) bool {
	// setup
	tcpAddr, _ := net.ResolveTCPAddr("tcp", net.JoinHostPort(server, fmt.Sprintf("%d", port)))
	d := net.Dialer{Timeout: *timeout}
	connInterface, err := d.Dial("tcp", tcpAddr.String())
	conn, _ := connInterface.(*net.TCPConn)
	if errors.Is(err, syscall.ECONNREFUSED) || /*errors.Is(err, os.ErrDeadlineExceeded) ||*/ os.IsTimeout(err) {
		// port is closed
		if *verbose {
			log.Printf("TCP CLOSED %d", port)
		}
		return false
	}
	check(err)
	defer conn.Close()

	// setup
	check(conn.SetDeadline(time.Now().Add(*timeout)))
	check(conn.SetNoDelay(true))
	check(conn.SetWriteBuffer(len(magicStringBytes)))
	check(conn.SetReadBuffer(len(magicStringBytes)))

	// send data
	check(conn.SetWriteDeadline(time.Now().Add(*timeout)))
	_, err = conn.Write(magicStringBytes)
	if err != nil && *verbose {
		log.Printf("TCP write error: %s", err)
		return false
	}

	// receive data
	buffer := make([]byte, 128)
	n, err := conn.Read(buffer)
	if err != nil && *verbose {
		log.Printf("TCP read error: %s", err)
		return false
	}

	if bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("TCP OPEN %d", port)
		}
		return true
	} else {
		if *verbose {
			log.Printf("TCP, Got data: %s", buffer[:n])
		}
	}

	return false
}
