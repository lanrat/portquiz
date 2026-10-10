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

	"portquiz/scan"
)

// defaults are the scan defaults the flags start from.
var defaults = scan.DefaultOptions()

var (
	tcp         = flag.Bool("tcp", false, "test TCP ports (default if neither -tcp nor -udp is set)")
	udp         = flag.Bool("udp", false, "test UDP ports")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
	timeout     = flag.Duration("timeout", defaults.Timeout, "maximum time to wait for each response")
	adaptive    = flag.Bool("adaptive", defaults.Adaptive, "shorten the timeout to fit the measured response time of open ports, up to -timeout")
	parallel    = flag.Uint("parallel", uint(defaults.Parallel), "maximum number of ports to test at once")
	rateLimit   = flag.Uint("rate", uint(defaults.Rate), "maximum number of connections to start per second, 0 for no limit")
	open        = flag.Bool("open", false, "print only open ports")
	closed      = flag.Bool("closed", false, "print only closed ports")
	port        = flag.String("port", "", "comma separated list of ports and ranges to test, e.g. 22,80,1000-2000 (default all ports)")
	multi       = flag.Uint("multi", uint(defaults.Multi), "test multiple times to ensure larger streams work")
	ipv4        = flag.Bool("4", false, "force IPv4")
	ipv6        = flag.Bool("6", false, "force IPv6")
	magicString = flag.String("password", defaults.Password, "magicString to use, must be the same on client/server")
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

// Version is set at build time.
var Version = "dev"

// main parses the flags, runs the scan and prints its results.
func main() {
	flag.Parse()

	if *version {
		fmt.Printf("portquiz client %s\n", Version)
		os.Exit(0)
	}

	if flag.NArg() != 1 {
		log.Fatalf("Pass IP/host to connect to")
	}

	// if open or closed not set, enable both
	if !*open && !*closed {
		*open = true
		*closed = true
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

	ports, err := scan.ParsePorts(*port)
	if err != nil {
		log.Fatal(err)
	}

	opts := scan.DefaultOptions()
	opts.Host = flag.Arg(0)
	opts.Ports = ports
	opts.TCP, opts.UDP = *tcp, *udp
	opts.IPv4, opts.IPv6 = *ipv4, *ipv6
	opts.Timeout = *timeout
	opts.Adaptive = *adaptive
	opts.Parallel = int(*parallel)
	opts.Rate = int(*rateLimit)
	opts.Multi = int(*multi)
	opts.Password = *magicString
	if retry.set {
		opts.Retries = int(retry.value)
	}
	if *verbose {
		opts.Logf = log.Printf
	}

	// stop gracefully on the first interrupt so the progress line is cleaned up;
	// restoring default signal handling lets a second interrupt exit immediately
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	context.AfterFunc(ctx, stop)

	s, err := scan.Start(ctx, opts)
	if err != nil {
		log.Fatal(err)
	}
	printResults(ctx, s)
	if err := s.Wait(); err != nil {
		log.Fatal(err)
	}
	warnMissed(s.Missed())
	if ctx.Err() != nil {
		fmt.Fprintln(os.Stderr, "interrupted")
		os.Exit(130)
	}
}

// printResults prints the scan's results as they arrive, in the format
// selected by -format and -open/-closed, and keeps the progress line up to
// date when it is enabled.
func printResults(ctx context.Context, s *scan.Scan) {
	var p *progress
	var tick <-chan time.Time
	if progressEnabled() {
		p = newProgress(s.Total())
		p.draw()
		t := time.NewTicker(progressInterval)
		defer t.Stop()
		tick = t.C
	}

	printed := false
	w := newPrinter(s.Ports(), func(line string) {
		if p != nil {
			p.clear()
		}
		printed = true
		fmt.Println(line)
	})

	results := s.Results()
	for {
		select {
		case <-tick:
			p.draw()
		case r, ok := <-results:
			if !ok {
				// print what is known so far
				w.flush()
				if p != nil {
					if ctx.Err() != nil {
						p.clear()
					} else {
						p.summary()
					}
				}
				return
			}
			if p != nil {
				p.record(r.Open)
			}
			w.add(r)
			// redraw once the current batch of results has been printed
			if printed && len(results) == 0 {
				printed = false
				if p != nil {
					p.draw()
				}
			}
		}
	}
}

// warnMissed warns when rechecks found ports that the adaptive timeout had
// missed, since other ports reported closed may then be open too.
func warnMissed(missed map[string]int) {
	kinds := make([]string, 0, len(missed))
	for k := range missed {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for _, k := range kinds {
		fmt.Fprintf(os.Stderr, "warning: %d %s port(s) only responded after the adaptive timeout and were found by a recheck; "+
			"other ports reported closed may be open. Use -adaptive=false for full accuracy.\n", missed[k], k)
	}
}
