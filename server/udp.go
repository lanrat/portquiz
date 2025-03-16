package main

import (
	"bytes"
	"log"
	"net"
)

// TODO detect canceled context
func udpServer(listenAddr string) error {
	log.Printf("starting UDP server on %s", listenAddr)
	addr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return err
	}

	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer l.Close()
	l.SetReadBuffer(len(magicStringBytes) * 2)

	buffer := make([]byte, 128)

	for {
		n, remoteAddr, err := l.ReadFromUDP(buffer)
		if err != nil && *verbose {
			log.Printf("UDP read error: %s", err)
			continue
		}
		if *verbose {
			log.Printf("[UDP] data from [%s] len: %d, data: %s", remoteAddr, n, buffer[:n])
		}
		if bytes.HasPrefix(buffer[:n], magicStringBytes) {
			if *verbose {
				log.Printf("[UDP] PORTQUIZ from %s", remoteAddr)
			}
			_, err = l.WriteToUDP(buffer[:n], remoteAddr)
			if err != nil && *verbose {
				log.Printf("UDP write error to [%s]: %s", remoteAddr, err)
				continue
			}
		}
	}
}
