// Package main provides job management and worker functionality for the portquiz client.
// It handles the creation, distribution, and processing of port testing jobs.
package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// job represents a single port testing task.
type job struct {
	try  uint   // Current retry attempt number
	kind string // Protocol and IP version (e.g., "tcp4", "udp6")
	port int    // Port number to test
	open bool   // Whether the port was found to be open
}

// wg tracks all active jobs to ensure proper shutdown.
var wg sync.WaitGroup

// versions returns the IP version suffixes to use based on command line flags.
// Returns:
//   - [""] for default (both IPv4 and IPv6)
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

// jobSource generates port testing jobs and sends them to the jobs channel.
// It creates jobs for the specified port range or individual ports based on command line arguments.
func jobSource(ctx context.Context, jobs, results chan *job) error {
	start := 1
	end := maxPort
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
	if *port == "" {
		for p := start; p <= end; p++ {
			err := addJob(p, ver)
			if err != nil {
				return err
			}
		}
	} else {
		ports := strings.Split(*port, ",")
		for _, ps := range ports {
			if ps == "" {
				continue
			}
			p, err := strconv.Atoi(ps)
			if err != nil {
				return err
			}
			err = addJob(p, ver)
			if err != nil {
				return err
			}
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

			for ; j.try < *retry && !j.open; j.try++ {
				err := try()
				if err != nil {
					return err
				}
			}

			results <- j
		}
	}
}

// jobResults processes completed jobs from the results channel and prints the output.
// It formats and displays the results based on the open/closed flags.
func jobResults(ctx context.Context, results chan *job) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case j, ok := <-results:
			if !ok {
				// channel closed
				return nil
			}
			if j.open {
				if *open {
					fmt.Printf("OPEN %s %d\n", j.kind, j.port)
				}
			} else {
				if *closed {
					fmt.Printf("CLOSED %s %d\n", j.kind, j.port)
				}
			}
			wg.Done()
		}
	}
}
