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

func isOpenUDPMulti(port int, network string) bool {
	for try := uint(0); try < *multi; try++ {
		if !isOpenUDP(port, network) {
			return false
		}
	}
	return true
}

func isOpenUDP(port int, network string) bool {
	// setup
	udpAddr, err := net.ResolveUDPAddr(network, net.JoinHostPort(server, fmt.Sprintf("%d", port)))
	check(err)
	conn, err := net.DialUDP(network, nil, udpAddr)
	check(err)
	defer conn.Close()

	// tuning
	check(conn.SetDeadline(time.Now().Add(*timeout)))
	check(conn.SetReadBuffer(len(magicStringBytes) * 2))

	// send data
	_, err = conn.Write(magicStringBytes)
	if err != nil && *verbose {
		log.Printf("%s write error: %s", network, err)
		return false
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
