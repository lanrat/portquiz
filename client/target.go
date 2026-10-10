// Package main provides the per-protocol test targets for the portquiz client.
// A target holds the resolved server address and the shared state used to test
// every port of one kind, such as "tcp" or "udp6".
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/time/rate"
)

// outcome is the result of a single check of one port.
type outcome int

const (
	outcomeOpen    outcome = iota // the server replied with the magic string
	outcomeRefused                // the connection was refused or the port was unreachable
	outcomeTimeout                // there was no response before the timeout
	outcomeFailed                 // any other error, or an unexpected reply
)

// target is everything needed to test ports of one kind.
type target struct {
	kind    string        // name shown in results, e.g. "tcp" or "udp6"
	network string        // network to dial, always with an IP version, e.g. "tcp4"
	addr    *net.IPAddr   // server address, resolved once for the whole scan
	est     *estimator    // adaptive timeout for this kind
	udp     bool          // whether this is a UDP kind
	limiter *rate.Limiter // limits how often connections are started, shared by all targets
}

// newTargets resolves the server once per IP version and returns a target for
// every kind that will be tested, keyed by kind.
func newTargets(limiter *rate.Limiter) (map[string]*target, error) {
	targets := make(map[string]*target)
	for _, v := range versions() {
		addr, err := net.ResolveIPAddr("ip"+v, server)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", server, err)
		}
		ipVersion := "6"
		if addr.IP.To4() != nil {
			ipVersion = "4"
		}
		if *verbose {
			log.Printf("Testing %s at %s", server, addr)
		}
		if *tcp {
			targets["tcp"+v] = &target{
				kind:    "tcp" + v,
				network: "tcp" + ipVersion,
				addr:    addr,
				est:     newEstimator(),
				limiter: limiter,
			}
		}
		if *udp {
			targets["udp"+v] = &target{
				kind:    "udp" + v,
				network: "udp" + ipVersion,
				addr:    addr,
				est:     newEstimator(),
				udp:     true,
				limiter: limiter,
			}
		}
	}
	return targets, nil
}

// retries returns how many times a port of this kind is retried after the
// first attempt. UDP has no retransmission of its own, so it gets one retry.
// TCP resends a lost connection request after about a second, which the full
// -timeout covers, so with -adaptive=false it needs none; adaptive timeouts
// are usually shorter than that, so TCP then gets one retry as well.
func (t *target) retries() uint {
	if retry.set {
		return retry.value
	}
	if t.udp || *adaptive {
		return 1
	}
	return 0
}

// check tests one port, -multi times in a row, with the adaptive timeout, or
// the full -timeout when full is set. It returns the first outcome that is not
// open, and the slowest response time.
func (t *target) check(ctx context.Context, port int, full bool) (outcome, time.Duration) {
	var slowest time.Duration
	for range *multi {
		if err := t.limiter.Wait(ctx); err != nil {
			return outcomeFailed, 0
		}
		// read the timeout only now: waiting for the rate limiter can take a
		// while, and the estimate may have improved in the meantime
		timeout := *timeout
		if !full {
			timeout = t.est.timeout()
		}
		var out outcome
		var rtt time.Duration
		if t.udp {
			out, rtt = checkUDP(ctx, t.network, t.addr, port, timeout)
		} else {
			out, rtt = checkTCP(ctx, t, port, timeout)
		}
		if out != outcomeOpen {
			return out, 0
		}
		slowest = max(slowest, rtt)
	}
	return outcomeOpen, slowest
}

// test runs every attempt for one port and reports whether it is open.
// Ports that did not respond are retried with the adaptive timeout. If the
// port still timed out, a sample of such ports is rechecked with the full
// -timeout to catch an adaptive timeout that has become too short.
func (t *target) test(ctx context.Context, port int) bool {
	last := outcomeFailed
	for attempt := uint(0); attempt <= t.retries(); attempt++ {
		if ctx.Err() != nil {
			return false
		}
		var rtt time.Duration
		last, rtt = t.check(ctx, port, false)
		if last == outcomeOpen {
			t.est.sample(rtt)
			return true
		}
		if last == outcomeRefused {
			// a refusal is definite, retrying will not change it
			return false
		}
	}
	if last == outcomeTimeout && t.est.shouldRecheck() {
		out, rtt := t.check(ctx, port, true)
		if out == outcomeOpen {
			t.est.missed(rtt)
			if *verbose {
				log.Printf("%s %d only responded after %s, raising the adaptive timeout", t.kind, port, rtt)
			}
			return true
		}
	}
	return false
}
