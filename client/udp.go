package main

import (
	"bytes"
	"errors"
	"log"
	"net"
	"syscall"
	"time"
)

func udpClient() error {
	isOpenUDP("1337")

	return errors.New("TODO")
}

func isOpenUDP(port string) bool {
	// seteup
	udpaddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(server, port))
	check(err)
	conn, err := net.DialUDP("udp", nil, udpaddr)
	check(err)
	defer conn.Close()

	// tuning
	check(conn.SetDeadline(time.Now().Add(*timeout)))
	check(conn.SetReadBuffer(len(magicStringBytes) * 2))

	// send data
	_, err = conn.Write(magicStringBytes)
	check(err)

	// recieve data
	buffer := make([]byte, 128)
	n, err := conn.Read(buffer)
	if errors.Is(err, syscall.ECONNREFUSED) {
		// port is closed
		if *verbose {
			log.Printf("UDP CLOSED %s", port)
		}
		return false
	}
	check(err)

	// check status
	if bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("UDP OPEN %s", port)
		}
		return true
	} else {
		if *verbose {
			log.Printf("UDP, Gotdata: %s %s", port, buffer[:n])
		}
	}

	return false
}
