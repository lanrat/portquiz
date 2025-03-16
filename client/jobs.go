package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type job struct {
	try  uint
	kind string
	port int
	open bool
}

var wg sync.WaitGroup

/*
00 = tcp
01 = tcp6
10 = tcp4
11 = tcp4,tcp6
*/
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
					if isOpenTCPMulti(j.port, j.kind) {
						j.open = true
					}
				case strings.HasPrefix(j.kind, "udp"):
					if isOpenUDPMulti(j.port, j.kind) {
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
