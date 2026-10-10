package scan

// Adaptive timeouts: blocked ports cost a full timeout each, so instead of
// always waiting the whole Timeout, the scan learns how quickly the server
// really responds and waits only a safe multiple of that.

import (
	"sync"
	"time"
)

const (
	// minAdaptiveTimeout is the shortest timeout ever used, however fast the
	// server responds, to absorb bursts of latency.
	minAdaptiveTimeout = 250 * time.Millisecond

	// minSamples is how many responses must be measured before the timeout
	// adapts; until then the full timeout is used. One is enough: filtered
	// networks often have only one or two open ports, and the first sample
	// alone already gives three times the response time.
	minSamples = 1

	// recheckEvery is how often a port that timed out with the adaptive
	// timeout is rechecked with the full timeout: one in recheckEvery.
	recheckEvery = 20
)

// estimator tracks how quickly the server responds and derives the timeout
// from it, using the smoothed round trip time and its variation as TCP does
// (RFC 6298). Only responses from open ports are measured: a refusal may come
// from a firewall much closer than the server, which would make the timeout
// too short.
type estimator struct {
	mu       sync.Mutex
	max      time.Duration // the full Timeout
	adaptive bool          // whether to adapt at all
	srtt     time.Duration // smoothed response time
	rttvar   time.Duration // smoothed variation of the response time
	samples  int           // number of responses measured
	floor    time.Duration // lowest allowed timeout, raised when a recheck finds a missed port
	timeouts int           // ports that timed out with the adaptive timeout, for sampling rechecks
	misses   int           // ports that only responded when rechecked with the full timeout
}

// newEstimator returns an estimator that starts with the full timeout max.
func newEstimator(max time.Duration, adaptive bool) *estimator {
	return &estimator{max: max, adaptive: adaptive, floor: minAdaptiveTimeout}
}

// timeout returns the timeout to use for the next check.
func (e *estimator) timeout() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.timeoutLocked()
}

// timeoutLocked returns the current timeout; e.mu must be held.
func (e *estimator) timeoutLocked() time.Duration {
	if !e.adaptive || e.samples < minSamples {
		return e.max
	}
	// allow for the usual variation, and never less than twice the
	// typical response time
	t := max(e.srtt+4*e.rttvar, 2*e.srtt, e.floor)
	return min(t, e.max)
}

// sample records the response time of an open port.
func (e *estimator) sample(rtt time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sampleLocked(rtt)
}

// sampleLocked records a response time; e.mu must be held.
func (e *estimator) sampleLocked(rtt time.Duration) {
	if e.samples == 0 {
		e.srtt = rtt
		e.rttvar = rtt / 2
	} else {
		diff := e.srtt - rtt
		if diff < 0 {
			diff = -diff
		}
		e.rttvar = (3*e.rttvar + diff) / 4
		e.srtt = (7*e.srtt + rtt) / 8
	}
	e.samples++
}

// shouldRecheck is called when a port timed out with the adaptive timeout and
// reports whether to recheck it with the full timeout.
func (e *estimator) shouldRecheck() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.timeoutLocked() >= e.max {
		// the full timeout was already used
		return false
	}
	e.timeouts++
	return e.timeouts%recheckEvery == 1
}

// missed records a port that timed out with the adaptive timeout but responded
// within the full timeout, after rtt. That is strong evidence the adaptive
// timeout is too short, so the floor is raised well above it.
func (e *estimator) missed(rtt time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.misses++
	e.floor = max(e.floor, 2*rtt)
	e.sampleLocked(rtt)
}

// missedCount returns how many ports were found open only by a recheck.
func (e *estimator) missedCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.misses
}
