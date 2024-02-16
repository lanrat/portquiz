package main

import (
	"bytes"
	"errors"
	"log"
	"net"
	"syscall"
	"time"
)

func tcpClient() error {
	isOpenTCP("1337")
	return errors.New("TODO")
}

func isOpenTCP(port string) bool {
	// setup
	tcpAddr, _ := net.ResolveTCPAddr("tcp", net.JoinHostPort(server, port))
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if errors.Is(err, syscall.ECONNREFUSED) {
		// port is closed
		if *verbose {
			log.Printf("TCP OPEN %s", port)
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
	check(err)

	// recieve data
	buffer := make([]byte, 128)
	n, err := conn.Read(buffer)
	check(err)

	if bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("TCP OPEN %s", port)
		}
		return true
	} else {
		if *verbose {
			log.Printf("TCP, Gotdata: %s", buffer[:n])
		}
	}

	return false
}
