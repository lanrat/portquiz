package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"time"

	"golang.org/x/sync/errgroup"
)

var (
	tcp     = flag.Bool("tcp", false, "start TCP server")
	udp     = flag.Bool("udp", false, "start UDP server")
	verbose = flag.Bool("verbose", false, "enable verbose logging")
	timeout = flag.Duration("timeout", time.Second*10, "amount of time for each connection")
)

var (
	g      *errgroup.Group
	ctx    context.Context
	server string
)

var magicString = "portquiz"
var magicStringBytes = []byte(magicString)

func main() {
	flag.Parse()

	if flag.NArg() != 1 {
		log.Fatalf("Pass IP/host to connect to")
	}
	server = flag.Arg(0)

	if !*tcp && !*udp {
		err := errors.New("must set TCP and/or UDP")
		check(err)
	}

	g, ctx = errgroup.WithContext(context.Background())

	if *tcp {
		g.Go(tcpClient)
	}

	if *udp {
		g.Go(udpClient)
	}

	check(g.Wait())

}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
