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
	tcp         = flag.Bool("tcp", false, "start TCP client")
	udp         = flag.Bool("udp", false, "start UDP client")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
	timeout     = flag.Duration("timeout", time.Second*5, "amount of time for each connection")
	retry       = flag.Uint("retry", 3, "retry count")
	parallel    = flag.Uint("parallel", 20, "number of worker threads")
	open        = flag.Bool("open", false, "print only open ports")
	closed      = flag.Bool("closed", false, "print only closed ports")
	port        = flag.String("port", "", "comma separated list of ports to test")
	multi       = flag.Uint("multi", 1, "test multiple times to ensure larger streams work")
	ipv4        = flag.Bool("4", false, "force IPv4")
	ipv6        = flag.Bool("6", false, "force IPv6")
	magicString = flag.String("password", "portquiz", "magicString to use, must be the same on client/server")
)

var (
	g                *errgroup.Group
	ctx              context.Context
	server           string
	magicStringBytes []byte
)

const maxPort = 65535

func main() {
	flag.Parse()
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

	if !*tcp && !*udp {
		err := errors.New("must set TCP and/or UDP")
		check(err)
	}

	g, ctx = errgroup.WithContext(context.Background())

	jobs := make(chan *job, 100)
	results := make(chan *job, 100)

	// start putting ports into queue
	g.Go(func() error {
		return jobSource(ctx, jobs, results)
	})

	// start workers
	for i := uint(0); i < *parallel; i++ {
		g.Go(func() error {
			return worker(ctx, jobs, results)
		})
	}

	// start results
	g.Go(func() error {
		return jobResults(ctx, results)
	})

	check(g.Wait())

}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
