// Package main implements a portquiz client that tests port connectivity to a remote server.
// It supports both TCP and UDP protocols and can test specific ports or scan ranges.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

var (
	tcp         = flag.Bool("tcp", false, "test TCP ports (default if neither -tcp nor -udp is set)")
	udp         = flag.Bool("udp", false, "test UDP ports")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
	timeout     = flag.Duration("timeout", time.Second*2, "amount of time for each connection")
	retry       = flag.Uint("retry", 1, "number of times to retry a port that did not respond")
	parallel    = flag.Uint("parallel", 100, "number of ports to test at once")
	open        = flag.Bool("open", false, "print only open ports")
	closed      = flag.Bool("closed", false, "print only closed ports")
	port        = flag.String("port", "", "comma separated list of ports to test")
	multi       = flag.Uint("multi", 1, "test multiple times to ensure larger streams work")
	ipv4        = flag.Bool("4", false, "force IPv4")
	ipv6        = flag.Bool("6", false, "force IPv6")
	magicString = flag.String("password", "portquiz", "magicString to use, must be the same on client/server")
	version     = flag.Bool("version", false, "show version information")
	noProgress  = flag.Bool("no-progress", false, "disable the progress bar shown when stderr is a terminal")
)

var (
	g                *errgroup.Group
	ctx              context.Context
	server           string
	magicStringBytes []byte
	Version          = "dev"
)

// maxPort defines the maximum valid port number.
const maxPort = 65535

// main initializes the client, parses arguments, and starts the port testing workflow.
// It sets up worker goroutines and manages the job queue for testing ports.
func main() {
	flag.Parse()

	if *version {
		fmt.Printf("portquiz client %s\n", Version)
		os.Exit(0)
	}

	magicStringBytes = []byte(*magicString)

	if flag.NArg() != 1 {
		log.Fatalf("Pass IP/host to connect to")
	}
	server = flag.Arg(0)

	// if open or closed not set, enable both
	if !*open && !*closed {
		*open = true
		*closed = true
	}

	// if neither protocol is set, test TCP
	if !*tcp && !*udp {
		*tcp = true
	}

	if *parallel < 1 {
		log.Fatal(errors.New("-parallel must be at least 1"))
	}

	ports, err := parsePorts()
	if err != nil {
		log.Fatal(err)
	}
	total := totalJobs(ports)

	// stop gracefully on the first interrupt so the progress line is cleaned up;
	// restoring default signal handling lets a second interrupt exit immediately
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	context.AfterFunc(sigCtx, stop)

	g, ctx = errgroup.WithContext(sigCtx)

	jobs := make(chan *job, 100)
	results := make(chan *job, 100)

	// start putting ports into queue
	g.Go(func() error {
		return jobSource(ctx, ports, jobs, results)
	})

	// start workers
	for i := uint(0); i < *parallel; i++ {
		g.Go(func() error {
			return worker(ctx, jobs, results)
		})
	}

	// start results
	g.Go(func() error {
		return jobResults(ctx, results, total)
	})

	err = g.Wait()
	if err != nil {
		log.Fatal(err)
	}
	if sigCtx.Err() != nil {
		fmt.Fprintln(os.Stderr, "interrupted")
		os.Exit(130)
	}

}
