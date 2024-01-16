package main

import (
	"log"
	"net"
)

// TODO detect canceled context
func udpServer() error {
	log.Printf("starting UDP server on %s", *listen)
	addr, err := net.ResolveUDPAddr("udp", *listen)
	if err != nil {
		return err
	}

	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer l.Close()

	buffer := make([]byte, 1024)

	for {
		n, remoteAddr, err := l.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("UDP read error: %s", err)
			continue
		}
		log.Printf("UDP data from [%s] len: %d, data: %s", remoteAddr, n, buffer[:n])

		_, err = l.WriteToUDP(buffer[:n], remoteAddr)
		if err != nil {
			log.Printf("UDP write error to [%s]: %s", remoteAddr, err)
			continue
		}
	}
}
