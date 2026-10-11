package scan

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
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
	udp     bool          // whether this is a UDP kind
	opts    *Options      // the scan's options
	magic   []byte        // the magic string sent and expected back
	est     *estimator    // adaptive timeout for this kind
	limiter *rate.Limiter // limits how often connections are started, shared by all targets
}

// newTargets resolves the server once per IP version and returns a target for
// every kind that will be tested, keyed by kind, and the addresses tested in
// the order of versions.
func newTargets(opts *Options, limiter *rate.Limiter) (map[string]*target, []*net.IPAddr, error) {
	targets := make(map[string]*target)
	var addrs []*net.IPAddr
	magic := []byte(opts.Password)
	for _, v := range versions(opts) {
		addr, err := resolve(opts.Host, v)
		if err != nil {
			return nil, nil, err
		}
		addrs = append(addrs, addr)
		ipVersion := "6"
		if addr.IP.To4() != nil {
			ipVersion = "4"
		}
		opts.logf("Testing %s at %s", opts.Host, addr)
		add := func(proto string) {
			targets[proto+v] = &target{
				kind:    proto + v,
				network: proto + ipVersion,
				addr:    addr,
				udp:     proto == "udp",
				opts:    opts,
				magic:   magic,
				est:     newEstimator(opts.Timeout, opts.Adaptive),
				limiter: limiter,
			}
		}
		if opts.TCP {
			add("tcp")
		}
		if opts.UDP {
			add("udp")
		}
	}
	return targets, addrs, nil
}

// resolve returns the address of host for IP version v ("4", "6", or "" for
// either). host is a name or an IP address, which may be in brackets like
// [2001:db8::1].
func resolve(host, v string) (*net.IPAddr, error) {
	// an address with the wrong version for v cannot be resolved; say why
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		if v == "4" && !ip.Is4() {
			return nil, fmt.Errorf("%s is an IPv6 address and cannot be tested over IPv4", host)
		}
		if v == "6" && ip.Is4() {
			return nil, fmt.Errorf("%s is an IPv4 address and cannot be tested over IPv6", host)
		}
	}
	addr, err := net.ResolveIPAddr("ip"+v, host)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", host, err)
	}
	return addr, nil
}

// cleanHost removes surrounding space and the brackets around an IPv6
// address, as in a URL, from a host given by the user.
func cleanHost(host string) string {
	host = strings.TrimSpace(host)
	if inner, ok := strings.CutPrefix(host, "["); ok {
		if inner, ok := strings.CutSuffix(inner, "]"); ok {
			host = inner
		}
	}
	return host
}

// retries returns how many times a port of this kind is retried after the
// first attempt. UDP has no retransmission of its own, so it gets one retry.
// TCP resends a lost connection request after about a second, which the full
// timeout covers, so with Adaptive off it needs none; adaptive timeouts are
// usually shorter than that, so TCP then gets one retry as well.
func (t *target) retries() int {
	if t.opts.Retries >= 0 {
		return t.opts.Retries
	}
	if t.udp || t.opts.Adaptive {
		return 1
	}
	return 0
}

// check tests one port, Multi times in a row, with the adaptive timeout, or
// the full timeout when full is set. It returns the first outcome that is not
// open, and the slowest response time.
func (t *target) check(ctx context.Context, port int, full bool) (outcome, time.Duration) {
	var slowest time.Duration
	for range t.opts.Multi {
		if err := t.limiter.Wait(ctx); err != nil {
			return outcomeFailed, 0
		}
		// read the timeout only now: waiting for the rate limiter can take a
		// while, and the estimate may have improved in the meantime
		timeout := t.opts.Timeout
		if !full {
			timeout = t.est.timeout()
		}
		var out outcome
		var rtt time.Duration
		if t.udp {
			out, rtt = t.checkUDP(ctx, port, timeout)
		} else {
			out, rtt = t.checkTCP(ctx, port, timeout)
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
// timeout to catch an adaptive timeout that has become too short.
func (t *target) test(ctx context.Context, port int) bool {
	last := outcomeFailed
	for attempt := 0; attempt <= t.retries(); attempt++ {
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
			t.opts.logf("%s %d only responded after %s, raising the adaptive timeout", t.kind, port, rtt)
			return true
		}
	}
	return false
}
