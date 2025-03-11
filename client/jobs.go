package main

import (
	"context"
	"fmt"
	"sync"
)

type job struct {
	try  uint
	kind string
	port int
	open bool
}

var wg sync.WaitGroup

func jobSource(ctx context.Context, jobs, results chan *job) error {
	start := 1
	end := maxPort
	if *port != -1 {
		start = *port
		end = *port
	}
	for p := start; p <= end; p++ {
		if *tcp {
			jTCP := &job{
				try:  0,
				kind: "tcp",
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
				kind: "udp",
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
				switch j.kind {
				case "tcp":
					if isOpenTCPMulti(j.port) {
						j.open = true
					}
				case "udp":
					if isOpenUDPMulti(j.port) {
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
