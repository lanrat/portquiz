package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"golang.org/x/sync/errgroup"
)

var (
	tcp     = flag.Bool("tcp", false, "start TCP server")
	udp     = flag.Bool("udp", false, "start UDP server")
	listen  = flag.String("listen", "[::]:1337", "ip:port to listen on")
	verbose = flag.Bool("verbose", false, "enable verbose logging")
	timeout = flag.Duration("timeout", time.Second*10, "amount of time for each connection")
)

var (
	g   *errgroup.Group
	ctx context.Context
)

var magicString = "portquiz"
var magicStringBytes = []byte(magicString)

func main() {
	flag.Parse()

	if !*tcp && !*udp {
		err := errors.New("must set TCP and/or UDP")
		check(err)
	}

	g, ctx = errgroup.WithContext(context.Background())

	check(addFWRules())
	defer cleanup()

	// setup fw cleanup if killed
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		cleanup()
		os.Exit(0)
	}()

	if *tcp {
		g.Go(tcpServer)
	}

	if *udp {
		g.Go(udpServer)
	}

	check(g.Wait())
}

func cleanup() {
	check(cleanupFW())
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
