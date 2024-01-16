package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
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

	fmt.Printf("Serving %s %s\n", kind, c.RemoteAddr())

	netData, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		fmt.Printf("error from [%s]: %s", c.RemoteAddr(), err)
		return
	}

	if *verbose {
		log.Printf("Got %s data from [%s]: %s", kind, c.RemoteAddr(), netData)
	}

	//netData = netData + "\n"
	_, err = c.Write([]byte(netData))
	check(err)
}

// // TODO detect canceled context
// func tcpServer() error {
// 	log.Printf("starting TCP server on %s", *listen)
// 	l, err := net.Listen("tcp", *listen)
// 	if err != nil {
// 		return err
// 	}
// 	defer l.Close()

// 	for {
// 		c, err := l.Accept()
// 		if err != nil {
// 			return err
// 		}
// 		go handleTCPConnection(c)
// 	}
// }

// func handleTCPConnection(c net.Conn) {
// 	kind := "TCP"

// 	defer c.Close()

// 	fmt.Printf("Serving %s %s\n", kind, c.RemoteAddr())

// 	netData, err := bufio.NewReader(c).ReadString('\n')
// 	if err != nil {
// 		fmt.Printf("error from [%s]: %s", c.RemoteAddr(), err)
// 		return
// 	}

// 	if *verbose {
// 		log.Printf("Got %s data from [%s]: %s", kind, c.RemoteAddr(), netData)
// 	}

// 	//netData = netData + "\n"
// 	_, err = c.Write([]byte(netData))
// 	check(err)
// }
