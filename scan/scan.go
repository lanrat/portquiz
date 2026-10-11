// Package scan tests which TCP and UDP ports of a portquiz server can be
// reached. It is used by the portquiz command line client and the Android app.
//
// A scan connects to each port, sends the magic string and waits for the
// server to send it back. Ports are tested concurrently and results are
// delivered as they finish, in no particular order; use Ranges to merge them
// into ranges of consecutive ports.
package scan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// MaxPort is the highest valid port number.
const MaxPort = 65535

// Options configures a scan. Start from DefaultOptions and set Host.
type Options struct {
	// Host is the portquiz server's hostname or IP address, such as
	// 192.0.2.1, 2001:db8::1 or [2001:db8::1]. It is resolved once, when the
	// scan starts.
	Host string

	// Ports lists the ports to test, nil for every port. ParsePorts builds
	// it from a list such as "22,80,1000-2000".
	Ports []int

	// TCP and UDP select the protocols to test; TCP is tested when neither
	// is set.
	TCP, UDP bool

	// IPv4 and IPv6 force testing over that IP version; when both are set
	// every port is tested over both. When neither is set the resolver
	// picks one.
	IPv4, IPv6 bool

	// Timeout is the longest to wait for each response.
	Timeout time.Duration

	// Adaptive shortens the timeout to fit the measured response time of
	// open ports, up to Timeout.
	Adaptive bool

	// Retries is how many times a port that did not respond is retried, or
	// -1 for the default: 1, or 0 for TCP when Adaptive is off, since the
	// full Timeout then covers the operating system's own retransmission.
	Retries int

	// Parallel is the maximum number of ports tested at once.
	Parallel int

	// Rate is the maximum number of connections started per second, 0 for
	// no limit. It protects the connection tracking tables of routers and
	// firewalls along the way.
	Rate int

	// Multi tests each port this many times in a row; a port is open only
	// if every test succeeds.
	Multi int

	// Password is the magic string; it must match the server's.
	Password string

	// Logf, if set, receives verbose diagnostic messages.
	Logf func(format string, args ...any)
}

// DefaultOptions returns the default options, without a Host.
func DefaultOptions() Options {
	return Options{
		Timeout:  2 * time.Second,
		Adaptive: true,
		Retries:  -1,
		Parallel: 500,
		Rate:     500,
		Multi:    1,
		Password: "portquiz",
	}
}

// Result is the outcome for one port.
type Result struct {
	Kind string // protocol and IP version: "tcp", "udp", "tcp4", "udp6" etc.
	Port int
	Open bool
}

// Scan is a running scan.
type Scan struct {
	opts    Options
	ports   []int
	kinds   []string
	targets map[string]*target
	addrs   []*net.IPAddr
	results chan Result
	done    chan struct{}
	err     error
}

// Start resolves the server and starts testing in the background. Results
// are delivered on Results until every port is tested or ctx is canceled.
func Start(ctx context.Context, opts Options) (*Scan, error) {
	opts.Host = cleanHost(opts.Host)
	if opts.Host == "" {
		return nil, errors.New("no host to test")
	}
	if !opts.TCP && !opts.UDP {
		opts.TCP = true
	}
	if opts.Timeout <= 0 {
		return nil, errors.New("timeout must be greater than 0")
	}
	if opts.Parallel < 1 {
		return nil, errors.New("parallel must be at least 1")
	}
	if opts.Multi < 1 {
		return nil, errors.New("multi must be at least 1")
	}
	if opts.Rate < 0 {
		return nil, errors.New("rate must not be negative")
	}
	if opts.Password == "" {
		return nil, errors.New("password must not be empty")
	}

	ports := opts.Ports
	if ports == nil {
		ports = AllPorts()
	} else {
		for _, p := range ports {
			if p < 1 || p > MaxPort {
				return nil, fmt.Errorf("port %d is not between 1 and %d", p, MaxPort)
			}
		}
		ports = slices.Compact(slices.Sorted(slices.Values(ports)))
	}

	limit := rate.Inf
	if opts.Rate > 0 {
		limit = rate.Limit(opts.Rate)
	}
	// a small burst smooths over scheduling delays without overshooting the rate
	limiter := rate.NewLimiter(limit, max(1, opts.Rate/50))

	targets, addrs, err := newTargets(&opts, limiter)
	if err != nil {
		return nil, err
	}

	s := &Scan{
		opts:    opts,
		ports:   ports,
		targets: targets,
		addrs:   addrs,
		results: make(chan Result, 100),
		done:    make(chan struct{}),
	}
	for _, v := range versions(&opts) {
		if opts.TCP {
			s.kinds = append(s.kinds, "tcp"+v)
		}
		if opts.UDP {
			s.kinds = append(s.kinds, "udp"+v)
		}
	}
	go s.run(ctx)
	return s, nil
}

// Results returns the channel results are delivered on. It is closed when
// the scan finishes or is canceled.
func (s *Scan) Results() <-chan Result {
	return s.results
}

// Ports returns the sorted ports being tested.
func (s *Scan) Ports() []int {
	return s.ports
}

// Addrs returns the server addresses being tested: one, or with both IPv4
// and IPv6 set, the IPv4 address and then the IPv6 address.
func (s *Scan) Addrs() []*net.IPAddr {
	return s.addrs
}

// Kinds returns the kinds being tested, e.g. ["tcp", "udp"].
func (s *Scan) Kinds() []string {
	return s.kinds
}

// Total returns the number of results the scan delivers if it is not canceled.
func (s *Scan) Total() int {
	return len(s.ports) * len(s.kinds)
}

// Wait waits for the scan to finish and returns its error, if any. A
// canceled scan returns nil.
func (s *Scan) Wait() error {
	<-s.done
	return s.err
}

// Missed returns, per kind, how many ports timed out with the adaptive
// timeout but were found open when rechecked with the full timeout. When it
// is not empty, other ports reported closed may be open too. Call it after
// Wait.
func (s *Scan) Missed() map[string]int {
	missed := make(map[string]int)
	for kind, t := range s.targets {
		if n := t.est.missedCount(); n > 0 {
			missed[kind] = n
		}
	}
	return missed
}

// job is one port of one kind to test.
type job struct {
	kind string
	port int
}

// run feeds jobs to the workers and closes the results channel when they
// are all done, or the scan is canceled.
func (s *Scan) run(ctx context.Context) {
	defer close(s.done)
	defer close(s.results)

	g, ctx := errgroup.WithContext(ctx)
	jobs := make(chan job, 100)
	g.Go(func() error {
		defer close(jobs)
		for _, p := range s.ports {
			for _, k := range s.kinds {
				select {
				case <-ctx.Done():
					return nil
				case jobs <- job{kind: k, port: p}:
				}
			}
		}
		return nil
	})
	var once sync.Once
	for range s.opts.Parallel {
		g.Go(func() error {
			for j := range jobs {
				t, ok := s.targets[j.kind]
				if !ok {
					err := fmt.Errorf("unknown kind: %s", j.kind)
					once.Do(func() { s.err = err })
					return err
				}
				r := Result{Kind: j.kind, Port: j.port, Open: t.test(ctx, j.port)}
				if ctx.Err() != nil {
					// the result of a canceled test is not meaningful
					return nil
				}
				select {
				case <-ctx.Done():
					return nil
				case s.results <- r:
				}
			}
			return nil
		})
	}
	_ = g.Wait()
}

// versions returns the IP version suffixes to test:
//   - [""] when neither IPv4 nor IPv6 is forced (whichever the resolver picks)
//   - ["4"] or ["6"] when one is forced
//   - ["4", "6"] when both are
func versions(opts *Options) []string {
	switch {
	case opts.IPv4 && opts.IPv6:
		return []string{"4", "6"}
	case opts.IPv4:
		return []string{"4"}
	case opts.IPv6:
		return []string{"6"}
	default:
		return []string{""}
	}
}

// logf logs a verbose message if logging is enabled.
func (o *Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}
