package main

import (
	"bytes"
	"log"
	"net"
	"time"
)

// TODO detect canceled context
func tcpServer(listenAddr string) error {
	addr, err := net.ResolveTCPAddr("tcp", listenAddr)
	if err != nil {
		return err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("starting TCP server on %s", listenAddr)
	defer l.Close()

	for {
		c, err := l.AcceptTCP()
		if err != nil {
			return err
		}
		go handleTCPConnection(c)
	}
}

func handleTCPConnection(c *net.TCPConn) {
	kind := "TCP"
	defer c.Close()
	check(c.SetNoDelay(true))
	check(c.SetDeadline(time.Now().Add(*timeout)))

	if *verbose {
		log.Printf("Serving %s %s\n", kind, c.RemoteAddr())
	}

	buffer := make([]byte, 128)

	n, err := c.Read(buffer)
	if err != nil && *verbose {
		log.Printf("TCP Read Error from %s: %s", c.RemoteAddr(), err)
		return
	}
	if *verbose {
		log.Printf("[%s], Gotdata from [%s]: %s", kind, c.RemoteAddr(), buffer[:n])
	}
	if bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("[%s] PORTQUIZ from %s", kind, c.RemoteAddr())
		}
		_, err := c.Write(magicStringBytes)
		if err != nil && *verbose {
			log.Printf("TCP Write Error from %s: %s", c.RemoteAddr(), err)
			return
		}
	}
}
