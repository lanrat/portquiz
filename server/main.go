package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

var (
	tcp         = flag.Bool("tcp", false, "start TCP server")
	udp         = flag.Bool("udp", false, "start UDP server")
	listenIPs   = flag.String("listen", "127.0.0.123", "comma separated list of IPs to listen on")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
	timeout     = flag.Duration("timeout", time.Second*10, "amount of time for each connection")
	port        = flag.Uint("port", 1337, "default port to listen on which will have traffic redirected to")
	noIPTables  = flag.Bool("no-iptables", false, "disable automatically creating iptables rules")
	magicString = flag.String("password", "portquiz", "magicString to use, must be the same on client/server")
)

var (
	g   *errgroup.Group
	ctx context.Context
)

var magicStringBytes []byte

func main() {
	flag.Parse()
	magicStringBytes = []byte(*magicString)

	if !*tcp && !*udp {
		err := errors.New("must set TCP and/or UDP")
		check(err)
	}

	g, ctx = errgroup.WithContext(context.Background())

	// setup fw cleanup if killed
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		// run cleanup on context canceled or interrupt
		select {
		case <-c:
		case <-ctx.Done():
		}
		cleanup()
		os.Exit(0)
	}()

	listenPort := strconv.FormatUint(uint64(*port), 10)

	for _, ip := range strings.Split(*listenIPs, ",") {
		if ip == "" {
			continue
		}
		listen := net.JoinHostPort(ip, listenPort)

		if !*noIPTables {
			check(addFWRules(ip, listenPort))
		}

		if *tcp {
			g.Go(func() error { return tcpServer(listen) })
		}

		if *udp {
			g.Go(func() error { return udpServer(listen) })
		}
	}

	check(g.Wait())
}

func cleanup() {
	if *verbose {
		log.Printf("Cleaning up for exit")
	}
	if !*noIPTables {
		check(cleanupFW())
	}
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
