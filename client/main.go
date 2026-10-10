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
	"slices"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

var (
	tcp         = flag.Bool("tcp", false, "test TCP ports (default if neither -tcp nor -udp is set)")
	udp         = flag.Bool("udp", false, "test UDP ports")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
	timeout     = flag.Duration("timeout", time.Second*2, "maximum time to wait for each response")
	adaptive    = flag.Bool("adaptive", true, "shorten the timeout to fit the measured response time of open ports, up to -timeout")
	parallel    = flag.Uint("parallel", 500, "maximum number of ports to test at once")
	rateLimit   = flag.Uint("rate", 500, "maximum number of connections to start per second, 0 for no limit")
	open        = flag.Bool("open", false, "print only open ports")
	closed      = flag.Bool("closed", false, "print only closed ports")
	port        = flag.String("port", "", "comma separated list of ports and ranges to test, e.g. 22,80,1000-2000 (default all ports)")
	multi       = flag.Uint("multi", 1, "test multiple times to ensure larger streams work")
	ipv4        = flag.Bool("4", false, "force IPv4")
	ipv6        = flag.Bool("6", false, "force IPv6")
	magicString = flag.String("password", "portquiz", "magicString to use, must be the same on client/server")
	version     = flag.Bool("version", false, "show version information")
	noProgress  = flag.Bool("no-progress", false, "disable the progress bar shown when stderr is a terminal")
	format      = flag.String("format", formatRanges, "output format: \"ranges\" merges consecutive ports with the same result, \"ports\" prints one line per port")
)

// retry is the -retry flag; when it is not set each protocol uses its own default.
var retry optionalUint

func init() {
	flag.Var(&retry, "retry", "number of `times` to retry a port that did not respond (default 1, or 0 for TCP with -adaptive=false)")
}

// optionalUint is a flag value that records whether it was set.
type optionalUint struct {
	set   bool
	value uint
}

func (o *optionalUint) String() string {
	if o == nil || !o.set {
		return ""
	}
	return strconv.FormatUint(uint64(o.value), 10)
}

func (o *optionalUint) Set(s string) error {
	v, err := strconv.ParseUint(s, 10, 0)
	if err != nil {
		return err
	}
	o.value, o.set = uint(v), true
	return nil
}

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

	if *format != formatRanges && *format != formatPorts {
		log.Fatalf("-format must be %q or %q", formatRanges, formatPorts)
	}

	if *parallel < 1 {
		log.Fatal(errors.New("-parallel must be at least 1"))
	}

	if *timeout <= 0 {
		log.Fatal(errors.New("-timeout must be greater than 0"))
	}

	ports, err := parsePorts()
	if err != nil {
		log.Fatal(err)
	}
	total := totalJobs(ports)

	limit := rate.Inf
	if *rateLimit > 0 {
		limit = rate.Limit(*rateLimit)
	}
	// a small burst smooths over scheduling delays without overshooting the rate
	limiter := rate.NewLimiter(limit, max(1, int(*rateLimit)/50))

	targets, err := newTargets(limiter)
	if err != nil {
		log.Fatal(err)
	}

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
			return worker(ctx, targets, jobs, results)
		})
	}

	// start results
	g.Go(func() error {
		return jobResults(ctx, results, ports, total)
	})

	err = g.Wait()
	if err != nil {
		log.Fatal(err)
	}
	warnMissed(targets)
	if sigCtx.Err() != nil {
		fmt.Fprintln(os.Stderr, "interrupted")
		os.Exit(130)
	}
}

// warnMissed warns when rechecks found ports that the adaptive timeout had
// missed, since other ports reported closed may then be open too.
func warnMissed(targets map[string]*target) {
	kinds := make([]string, 0, len(targets))
	for k := range targets {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for _, k := range kinds {
		if n := targets[k].est.missedCount(); n > 0 {
			fmt.Fprintf(os.Stderr, "warning: %d %s port(s) only responded after the adaptive timeout and were found by a recheck; "+
				"other ports reported closed may be open. Use -adaptive=false for full accuracy.\n", n, k)
		}
	}
}
