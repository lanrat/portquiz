// Package mobile adapts package scan to the Android app through gomobile
// bind. It only uses types gomobile can translate to Java: a scan is
// configured with a Config, started with Start, and reports to a Listener.
package mobile

import (
	"context"
	"time"

	"portquiz/scan"
)

// progressInterval is the shortest time between OnProgress calls, so a full
// scan does not call into Java for every port.
const progressInterval = 100 * time.Millisecond

// Listener receives a scan's events. Its methods are called from a
// background goroutine, one at a time and never on the UI thread.
type Listener interface {
	// OnRange is called for each range of consecutive ports with the same
	// result, as soon as it is final.
	OnRange(kind string, first, last int, open bool)
	// OnProgress reports how many of the total checks are done, and how
	// many of those found an open port.
	OnProgress(done, total, open int)
	// OnFinish is called once, last, when the scan ends for any reason.
	OnFinish(summary *Summary)
}

// Summary describes how a scan ended.
type Summary struct {
	Done     int  // checks completed
	Total    int  // checks the scan would run if it finished
	Open     int  // checks that found an open port
	Canceled bool // whether Cancel stopped the scan
	// Missed counts ports that only responded when rechecked with the full
	// timeout; when it is not 0, other ports reported closed may be open.
	Missed int
	// Error is empty unless the scan failed.
	Error string
}

// Config configures a scan; NewConfig returns the defaults.
type Config struct {
	Host          string // server IP address or hostname
	Ports         string // ports and ranges, e.g. "22,80,1000-2000"; empty for all
	TCP           bool
	UDP           bool
	IPv4          bool
	IPv6          bool
	TimeoutMillis int  // longest wait for each response
	Adaptive      bool // shorten the timeout to fit measured response times
	Retries       int  // retries for ports that did not respond, -1 for the default
	Parallel      int  // ports tested at once
	Rate          int  // connections started per second, 0 for no limit
	Multi         int  // times each port is tested
	Password      string
}

// NewConfig returns a Config with the default settings and TCP selected.
func NewConfig() *Config {
	d := scan.DefaultOptions()
	return &Config{
		TCP:           true,
		TimeoutMillis: int(d.Timeout / time.Millisecond),
		Adaptive:      d.Adaptive,
		Retries:       d.Retries,
		Parallel:      d.Parallel,
		Rate:          d.Rate,
		Multi:         d.Multi,
		Password:      d.Password,
	}
}

// options converts the Config to scan options.
func (c *Config) options() (scan.Options, error) {
	ports, err := scan.ParsePorts(c.Ports)
	if err != nil {
		return scan.Options{}, err
	}
	return scan.Options{
		Host:     c.Host,
		Ports:    ports,
		TCP:      c.TCP,
		UDP:      c.UDP,
		IPv4:     c.IPv4,
		IPv6:     c.IPv6,
		Timeout:  time.Duration(c.TimeoutMillis) * time.Millisecond,
		Adaptive: c.Adaptive,
		Retries:  c.Retries,
		Parallel: c.Parallel,
		Rate:     c.Rate,
		Multi:    c.Multi,
		Password: c.Password,
	}, nil
}

// Scan is a running scan.
type Scan struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// Start checks the config, resolves the host and starts the scan in the
// background, reporting to l. Configuration and resolution errors are
// returned directly and l is not called. Start blocks while resolving the
// host, so call it off the UI thread.
func Start(c *Config, l Listener) (*Scan, error) {
	opts, err := c.options()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := scan.Start(ctx, opts)
	if err != nil {
		cancel()
		return nil, err
	}
	m := &Scan{cancel: cancel, done: make(chan struct{})}
	go m.report(ctx, s, l)
	return m, nil
}

// Cancel stops the scan. The listener still receives the ranges known so far
// and then OnFinish. It is safe to call more than once.
func (m *Scan) Cancel() {
	m.cancel()
}

// AwaitFinish blocks until the scan has ended and OnFinish has returned.
// (It is not named Wait, which would clash with Java's Object.wait.)
func (m *Scan) AwaitFinish() {
	<-m.done
}

// report forwards the scan's results to l until the scan ends.
func (m *Scan) report(ctx context.Context, s *scan.Scan, l Listener) {
	defer close(m.done)
	defer m.cancel()

	total := s.Total()
	var done, open int
	ranges := scan.NewRanges(s.Ports(), func(r scan.Range) {
		l.OnRange(r.Kind, r.First, r.Last, r.Open)
	})
	tick := time.NewTicker(progressInterval)
	defer tick.Stop()
	changed := false

	l.OnProgress(0, total, 0)
	results := s.Results()
	for running := true; running; {
		select {
		case <-tick.C:
			if changed {
				l.OnProgress(done, total, open)
				changed = false
			}
		case r, ok := <-results:
			if !ok {
				running = false
				break
			}
			done++
			if r.Open {
				open++
			}
			changed = true
			ranges.Add(r)
		}
	}
	ranges.Flush()
	l.OnProgress(done, total, open)

	summary := &Summary{
		Done:     done,
		Total:    total,
		Open:     open,
		Canceled: ctx.Err() != nil && done < total,
	}
	if err := s.Wait(); err != nil {
		summary.Error = err.Error()
	}
	for _, n := range s.Missed() {
		summary.Missed += n
	}
	l.OnFinish(summary)
}
