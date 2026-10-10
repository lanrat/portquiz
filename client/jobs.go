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
// every port, or the ports and ranges listed in -port, e.g. "22,80,1000-2000".
func parsePorts() ([]int, error) {
	var ports []int
	if *port == "" {
		ports = make([]int, 0, maxPort)
		for p := 1; p <= maxPort; p++ {
			ports = append(ports, p)
		}
		return ports, nil
	}
	for _, item := range strings.Split(*port, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		first, last, isRange := strings.Cut(item, "-")
		start, err := parsePort(first)
		if err != nil {
			return nil, fmt.Errorf("invalid -port entry %q: %w", item, err)
		}
		end := start
		if isRange {
			end, err = parsePort(last)
			if err != nil {
				return nil, fmt.Errorf("invalid -port entry %q: %w", item, err)
			}
			if start > end {
				return nil, fmt.Errorf("invalid -port entry %q: start of range is greater than end", item)
			}
		}
		for p := start; p <= end; p++ {
			ports = append(ports, p)
		}
	}
	slices.Sort(ports)
	return slices.Compact(ports), nil
}

// parsePort parses a single port number and checks that it is in range.
func parsePort(s string) (int, error) {
	s = strings.TrimSpace(s)
	p, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a port number", s)
	}
	if p < 1 || p > maxPort {
		return 0, fmt.Errorf("port %d is not between 1 and %d", p, maxPort)
	}
	return p, nil
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
// It tests each port with the target for the job's kind.
func worker(ctx context.Context, targets map[string]*target, jobs, results chan *job) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case j, ok := <-jobs:
			if !ok {
				// done
				return nil
			}
			t, ok := targets[j.kind]
			if !ok {
				return fmt.Errorf("unknown kind: %s", j.kind)
			}
			j.open = t.test(ctx, j.port)

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
