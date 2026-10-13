package scan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

// outcome is the result of a single check of one port.
type outcome int

const (
	outcomeOpen       outcome = iota // the server replied with the magic string
	outcomeRefused                   // the connection was refused, or a host or network unreachable came back
	outcomeTimeout                   // there was no response before the timeout
	outcomeSilent                    // a TCP connection was accepted, but nothing replied before the timeout
	outcomeInterfered                // the connection was reset, or something other than the server replied
	outcomeError                     // a local error, such as running out of sockets, or the scan was canceled
)

// status returns the Status reported for a port whose last check had this
// outcome.
func (o outcome) status() Status {
	switch o {
	case outcomeOpen:
		return StatusOpen
	case outcomeRefused:
		return StatusRejected
	case outcomeTimeout:
		return StatusDropped
	case outcomeSilent, outcomeInterfered:
		return StatusInterfered
	default:
		return StatusError
	}
}

// refusal reports whether a connection error means the traffic was actively
// rejected: a TCP reset or ICMP port unreachable from the server or a
// firewall, an ICMP host or network unreachable, which firewalls also send to
// reject traffic, or a local firewall denying it.
func refusal(err error) bool {
	for _, e := range []error{syscall.ECONNREFUSED, syscall.EHOSTUNREACH, syscall.ENETUNREACH, syscall.EACCES, syscall.EPERM} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

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

// newTargets resolves the server and returns a target for every kind that
// will be tested, keyed by kind, along with the kinds and the addresses
// tested in the order they are tested.
func newTargets(ctx context.Context, opts *Options, limiter *rate.Limiter) (map[string]*target, []string, []*net.IPAddr, error) {
	eps, err := endpoints(ctx, opts)
	if err != nil {
		return nil, nil, nil, err
	}
	targets := make(map[string]*target)
	var kinds []string
	var addrs []*net.IPAddr
	magic := []byte(opts.Password)
	for _, ep := range eps {
		addrs = append(addrs, ep.addr)
		ipVersion := "6"
		if ep.addr.IP.To4() != nil {
			ipVersion = "4"
		}
		opts.logf("Testing %s at %s", opts.Host, ep.addr)
		add := func(proto string) {
			kind := proto + ep.version
			kinds = append(kinds, kind)
			targets[kind] = &target{
				kind:    kind,
				network: proto + ipVersion,
				addr:    ep.addr,
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
	return targets, kinds, addrs, nil
}

// endpoint is a server address and the IP version suffix of the kinds
// tested at it: "4", "6", or "" when only one version is tested without
// IPv4 or IPv6 set.
type endpoint struct {
	version string
	addr    *net.IPAddr
}

// endpoints resolves the server addresses to test. With IPv4 or IPv6 set it
// returns an address of each version set, or an error. With neither, it
// tests both versions when the server has an address of each and this
// machine has a route to both, and otherwise the one version it can.
func endpoints(ctx context.Context, opts *Options) ([]endpoint, error) {
	var forced []string
	if opts.IPv4 {
		forced = append(forced, "4")
	}
	if opts.IPv6 {
		forced = append(forced, "6")
	}
	if len(forced) > 0 {
		var eps []endpoint
		for _, v := range forced {
			addr, err := resolve(opts.Host, v)
			if err != nil {
				return nil, err
			}
			eps = append(eps, endpoint{v, addr})
		}
		return eps, nil
	}

	// an IP address is tested as given
	if _, err := netip.ParseAddr(opts.Host); err == nil {
		addr, err := resolve(opts.Host, "")
		if err != nil {
			return nil, err
		}
		return []endpoint{{"", addr}}, nil
	}

	all, err := lookupIPAddr(ctx, opts.Host)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", opts.Host, err)
	}
	var v4, v6 *net.IPAddr
	for _, a := range all {
		if ip4 := a.IP.To4(); ip4 != nil {
			if v4 == nil {
				v4 = &net.IPAddr{IP: ip4}
			}
		} else if v6 == nil {
			v6 = &a
		}
	}
	usable4 := v4 != nil && routable(v4)
	usable6 := v6 != nil && routable(v6)
	switch {
	case usable4 && usable6:
		return []endpoint{{"4", v4}, {"6", v6}}, nil
	case usable6:
		return []endpoint{{"", v6}}, nil
	case v4 != nil:
		// with no route at all, IPv4 fails the same way it always has
		return []endpoint{{"", v4}}, nil
	case v6 != nil:
		return []endpoint{{"", v6}}, nil
	default:
		return nil, fmt.Errorf("resolving %s: no addresses found", opts.Host)
	}
}

// lookupIPAddr looks up a host's addresses; tests replace it.
var lookupIPAddr = net.DefaultResolver.LookupIPAddr

// routable reports whether this machine has a route to addr, as an IPv4-only
// network has none to IPv6 addresses; tests replace it. Connecting a UDP
// socket only looks up the route; it sends nothing.
var routable = func(addr *net.IPAddr) bool {
	network := "udp6"
	if addr.IP.To4() != nil {
		network = "udp4"
	}
	c, err := net.DialUDP(network, nil, &net.UDPAddr{IP: addr.IP, Port: 9, Zone: addr.Zone})
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// resolve returns the address of host for IP version v ("4", "6", or "" for
// either). host is a name or an IP address.
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
			return outcomeError, 0
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

// test runs every attempt for one port and returns its status, how long it
// took to answer if it is open, and whether it only answered when rechecked.
// Ports that did not respond are retried with the adaptive timeout. If the
// port still timed out, a sample of such ports is rechecked with the full
// timeout to catch an adaptive timeout that has become too short.
func (t *target) test(ctx context.Context, port int) (status Status, rtt time.Duration, late bool) {
	last := outcomeError
	for attempt := 0; attempt <= t.retries(); attempt++ {
		if ctx.Err() != nil {
			return StatusError, 0, false
		}
		last, rtt = t.check(ctx, port, false)
		if last == outcomeOpen {
			t.est.sample(rtt)
			return StatusOpen, rtt, false
		}
		if last == outcomeRefused {
			// a refusal is definite, retrying will not change it
			return StatusRejected, 0, false
		}
	}
	if last == outcomeTimeout && t.est.shouldRecheck() {
		out, rtt := t.check(ctx, port, true)
		if out == outcomeOpen {
			t.est.missed(rtt)
			t.opts.logf("%s %d only responded after %s, raising the adaptive timeout", t.kind, port, rtt)
			return StatusOpen, rtt, true
		}
	}
	return last.status(), 0, false
}
