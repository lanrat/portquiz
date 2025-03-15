package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"
)

func isOpenUDPMulti(port int) bool {
	for try := uint(0); try < *multi; try++ {
		if !isOpenUDP(port) {
			return false
		}
	}
	return true
}

func isOpenUDP(port int) bool {
	// setup
	udpAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(server, fmt.Sprintf("%d", port)))
	check(err)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	check(err)
	defer conn.Close()

	// tuning
	check(conn.SetDeadline(time.Now().Add(*timeout)))
	check(conn.SetReadBuffer(len(magicStringBytes) * 2))

	// send data
	_, err = conn.Write(magicStringBytes)
	if err != nil && *verbose {
		log.Printf("UDP write error: %s", err)
		return false
	}

	// receive data
	buffer := make([]byte, 128)
	n, err := conn.Read(buffer)
	if errors.Is(err, syscall.ECONNREFUSED) {
		// port is closed
		if *verbose {
			log.Printf("UDP CLOSED %d", port)
		}
		return false
	}
	if err != nil && *verbose {
		log.Printf("UDP read error: %s", err)
		return false
	}

	// check status
	if bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("UDP OPEN %d", port)
		}
		return true
	} else {
		if *verbose {
			log.Printf("UDP, Got data: %d %s", port, buffer[:n])
		}
	}

	return false
}
