package main

import (
	"bufio"
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
	check(c.SetNoDelay(false))
	check(c.SetDeadline(time.Now().Add(*timeout)))

	fmt.Printf("Serving %s %s\n", kind, c.RemoteAddr())

	scanner := bufio.NewScanner(c)
	scanner.Buffer(make([]byte, len(magicString)*2), len(magicString)*2)
	if scanner.Scan() {
		got := scanner.Text()
		if *verbose {
			log.Printf("[%s], Gotdata from [%s]: %s", kind, c.RemoteAddr(), got)
		}
		if got == magicString {
			if *verbose {
				log.Printf("[%s] PORTQUIZ from %s", kind, c.RemoteAddr())
			}
			_, err := c.Write(magicStringBytes)
			check(err)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("error from [%s]: %s", c.RemoteAddr(), err)
		return
	}
}
