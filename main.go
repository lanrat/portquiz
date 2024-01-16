package main

import (
	"context"
	"errors"
	"flag"
	"log"

	"golang.org/x/sync/errgroup"
)

var (
	tcp     = flag.Bool("tcp", false, "start TCP server")
	udp     = flag.Bool("udp", false, "start UDP server")
	listen  = flag.String("listen", "[::]:1337", "ip:port to listen on")
	verbose = flag.Bool("verbose", false, "enable verbose logging")
)

var (
	g   *errgroup.Group
	ctx context.Context
)

func main() {
	flag.Parse()

	if !*tcp && !*udp {
		err := errors.New("must set TCP and/or UDP")
		check(err)
	}

	g, ctx = errgroup.WithContext(context.Background())

	if *tcp {
		g.Go(tcpServer)
	}

	if *udp {
		g.Go(udpServer)
	}

	check(g.Wait())
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
