package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"time"
)

// TODO detect canceled context
func tcpServer() error {
	addr, err := net.ResolveTCPAddr("tcp", *listen)
	if err != nil {
		return err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("starting TCP server on %s", *listen)
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

	fmt.Printf("Serving %s %s\n", kind, c.RemoteAddr())

	buffer := make([]byte, 128)

	n, err := c.Read(buffer)
	check(err)
	if *verbose {
		log.Printf("[%s], Gotdata from [%s]: %s", kind, c.RemoteAddr(), buffer[:n])
	}
	if bytes.HasPrefix(buffer[:n], magicStringBytes) {
		if *verbose {
			log.Printf("[%s] PORTQUIZ from %s", kind, c.RemoteAddr())
		}
		_, err := c.Write(magicStringBytes)
		check(err)
	}
}
