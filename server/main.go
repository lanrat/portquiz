// Package main implements a portquiz server that listens on specified TCP and UDP ports.
// It automatically creates iptables rules to redirect traffic and responds to clients
// with a magic string to confirm port accessibility.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

var (
	tcp         = flag.Bool("tcp", false, "start TCP server")
	udp         = flag.Bool("udp", false, "start UDP server")
	listenIPs   = flag.String("listen", "", "comma separated list of non-loopback IPs to listen on (required)")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
	timeout     = flag.Duration("timeout", time.Second*10, "amount of time for each connection")
	port        = flag.Uint("port", 1337, "default port to listen on which will have traffic redirected to")
	noIPTables  = flag.Bool("no-iptables", false, "disable automatically creating iptables rules")
	magicString = flag.String("password", "portquiz", "magicString to use, must be the same on client/server")
	version     = flag.Bool("version", false, "show version information")
)

var (
	g                *errgroup.Group
	ctx              context.Context
	magicStringBytes []byte
	Version          = "dev"
	cleanupOnce      sync.Once
)

// main initializes and starts the portquiz server with the specified configuration.
// It sets up signal handling for cleanup, creates firewall rules, and starts TCP/UDP servers.
func main() {
	flag.Parse()

	if *version {
		fmt.Printf("portquiz server %s\n", Version)
		os.Exit(0)
	}

	magicStringBytes = []byte(*magicString)

	if !*tcp && !*udp {
		err := errors.New("must set TCP and/or UDP")
		log.Fatal(err)
	}

	if strings.Trim(*listenIPs, ",") == "" {
		log.Fatal(errors.New("must set -listen"))
	}

	g, ctx = errgroup.WithContext(context.Background())

	// setup fw cleanup if killed
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
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
			err := addFWRules(ip, listenPort)
			if err != nil {
				cleanup()
				log.Fatal(err)
			}
		}

		if *tcp {
			g.Go(func() error { return tcpServer(ctx, listen) })
		}

		if *udp {
			g.Go(func() error { return udpServer(ctx, listen) })
		}
	}

	err := g.Wait()
	if err != nil {
		cleanup()
		log.Fatal(err)
	}
}

// cleanup removes all firewall rules created by the server and performs shutdown tasks.
// It is safe to call from multiple goroutines; the work runs only once and
// concurrent callers block until it has finished.
func cleanup() {
	cleanupOnce.Do(func() {
		if *verbose {
			log.Printf("Cleaning up for exit")
		}
		if !*noIPTables {
			if err := cleanupFW(); err != nil {
				log.Printf("Cleanup firewall error: %s", err)
			}
		}
	})
}
