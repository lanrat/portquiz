// Package main provides job management and worker functionality for the portquiz client.
// It handles the creation, distribution, and processing of port testing jobs.
package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// job represents a single port testing task.
type job struct {
	try  uint   // Number of attempts made so far
	kind string // Protocol and IP version (e.g., "tcp4", "udp6")
	port int    // Port number to test
	open bool   // Whether the port was found to be open
}

// wg tracks all active jobs to ensure proper shutdown.
var wg sync.WaitGroup

// versions returns the IP version suffixes to use based on command line flags.
// Returns:
//   - [""] for default (whichever IP version the resolver picks)
//   - ["4"] for IPv4 only
//   - ["6"] for IPv6 only
//   - ["4", "6"] for both explicitly
func versions() []string {
	if !*ipv4 && !*ipv6 {
		return []string{""}
	}
	if *ipv4 && !*ipv6 {
		return []string{"4"}
	}
	if !*ipv4 && *ipv6 {
		return []string{"6"}
	}
	return []string{"4", "6"}
}

// parsePorts returns the ports to test, sorted and without duplicates:
// every port, or the ports listed in -port.
func parsePorts() ([]int, error) {
	var ports []int
	if *port == "" {
		ports = make([]int, 0, maxPort)
		for p := 1; p <= maxPort; p++ {
			ports = append(ports, p)
		}
		return ports, nil
	}
	for _, ps := range strings.Split(*port, ",") {
		if ps == "" {
			continue
		}
		p, err := strconv.Atoi(ps)
		if err != nil {
			return nil, err
		}
		if p < 1 || p > maxPort {
			return nil, fmt.Errorf("invalid port %d: must be between 1 and %d", p, maxPort)
		}
		ports = append(ports, p)
	}
	slices.Sort(ports)
	return slices.Compact(ports), nil
}

// totalJobs returns the number of jobs that jobSource will create for the given ports.
func totalJobs(ports []int) int {
	protocols := 0
	if *tcp {
		protocols++
	}
	if *udp {
		protocols++
	}
	return len(ports) * len(versions()) * protocols
}

// jobSource generates port testing jobs for the given ports and sends them to the jobs channel.
func jobSource(ctx context.Context, ports []int, jobs, results chan *job) error {
	addJob := func(p int, ver []string) error {
		for _, v := range ver {
			if *tcp {
				jTCP := &job{
					try:  0,
					kind: "tcp" + v,
					port: p,
				}
				wg.Add(1)
				select {
				case <-ctx.Done():
					return nil
				case jobs <- jTCP:
				}
			}
			if *udp {
				jUDP := &job{
					try:  0,
					kind: "udp" + v,
					port: p,
				}
				wg.Add(1)
				select {
				case <-ctx.Done():
					return nil
				case jobs <- jUDP:
				}
			}
		}
		return nil
	}
	ver := versions()
	for _, p := range ports {
		err := addJob(p, ver)
		if err != nil {
			return err
		}
	}
	close(jobs)
	go func() {
		wg.Wait()
		close(results)
	}()
	return nil
}

// worker processes jobs from the jobs channel and sends results to the results channel.
// It performs the actual port connectivity tests and retries failed attempts.
func worker(ctx context.Context, jobs, results chan *job) error {
	//var a sync.WaitGroup
	for {
		select {
		case <-ctx.Done():
			return nil
		case j, ok := <-jobs:
			if !ok {
				// done
				return nil
			}

			try := func() error {
				switch {
				case strings.HasPrefix(j.kind, "tcp"):
					if isOpenTCPMulti(ctx, j.port, j.kind) {
						j.open = true
					}
				case strings.HasPrefix(j.kind, "udp"):
					if isOpenUDPMulti(ctx, j.port, j.kind) {
						j.open = true
					}
				default:
					return fmt.Errorf("unknown kind: %s", j.kind)
				}
				return nil
			}

			// the first attempt plus up to -retry retries
			for ; j.try <= *retry && !j.open; j.try++ {
				err := try()
				if err != nil {
					return err
				}
			}

			select {
			case <-ctx.Done():
				return nil
			case results <- j:
			}
		}
	}
}

// jobResults processes completed jobs from the results channel and prints the output.
// It formats and displays the results based on the -format and -open/-closed flags,
// and keeps the progress line up to date when it is enabled.
func jobResults(ctx context.Context, results chan *job, ports []int, total int) error {
	var p *progress
	var tick <-chan time.Time
	if progressEnabled() {
		p = newProgress(total)
		p.draw()
		t := time.NewTicker(progressInterval)
		defer t.Stop()
		tick = t.C
	}

	printed := false
	w := newResultWriter(ports, func(line string) {
		if p != nil {
			p.clear()
		}
		printed = true
		fmt.Println(line)
	})

	for {
		select {
		case <-ctx.Done():
			if p != nil {
				p.clear()
			}
			// print what is known so far
			w.flush()
			return nil
		case <-tick:
			p.draw()
		case j, ok := <-results:
			if !ok {
				// channel closed
				w.flush()
				if p != nil {
					p.summary()
				}
				return nil
			}
			if p != nil {
				p.record(j.open)
			}
			w.add(j)
			// redraw once the current batch of results has been printed
			if printed && len(results) == 0 {
				printed = false
				if p != nil {
					p.draw()
				}
			}
			wg.Done()
		}
	}
}
