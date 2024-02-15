package main

import (
	"bytes"
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
	l.SetReadBuffer(len(magicString) * 2)
	//check(l.SetDeadline(time.Now().Add(*timeout))) // TODO pretty sure this should not be here

	buffer := make([]byte, 1024)

	for {
		n, remoteAddr, err := l.ReadFromUDP(buffer)
		if err != nil {
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
			if err != nil {
				log.Printf("UDP write error to [%s]: %s", remoteAddr, err)
				continue
			}
		}
	}
}
