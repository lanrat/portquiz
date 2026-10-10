package main

import (
	"testing"
	"time"
)

// withTiming sets -timeout and -adaptive for the duration of a test.
func withTiming(t *testing.T, max time.Duration, adapt bool) {
	t.Helper()
	oldTimeout, oldAdaptive := *timeout, *adaptive
	*timeout, *adaptive = max, adapt
	t.Cleanup(func() { *timeout, *adaptive = oldTimeout, oldAdaptive })
}

func TestEstimatorStartsAtFullTimeout(t *testing.T) {
	withTiming(t, 2*time.Second, true)
	if got := newEstimator().timeout(); got != 2*time.Second {
		t.Errorf("got %s, want the full -timeout", got)
	}
}

func TestEstimatorDisabled(t *testing.T) {
	withTiming(t, 2*time.Second, false)
	e := newEstimator()
	for range 10 {
		e.sample(10 * time.Millisecond)
	}
	if got := e.timeout(); got != 2*time.Second {
		t.Errorf("got %s, want the full -timeout when -adaptive=false", got)
	}
	if e.shouldRecheck() {
		t.Error("rechecked although the full timeout is already used")
	}
}

func TestEstimatorTimeouts(t *testing.T) {
	withTiming(t, 2*time.Second, true)
	tests := []struct {
		name    string
		samples []time.Duration
		want    time.Duration
	}{
		{"fast responses use the minimum", []time.Duration{10 * time.Millisecond}, minAdaptiveTimeout},
		{"one sample gives 3x", []time.Duration{200 * time.Millisecond}, 600 * time.Millisecond},
		{"steady responses give at least 2x", repeat(300*time.Millisecond, 50), 600 * time.Millisecond},
		{"slow responses are capped at -timeout", []time.Duration{1500 * time.Millisecond}, 2 * time.Second},
	}
	for _, tt := range tests {
		e := newEstimator()
		for _, s := range tt.samples {
			e.sample(s)
		}
		if got := e.timeout(); got != tt.want {
			t.Errorf("%s: got %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestEstimatorVariationWidensTimeout(t *testing.T) {
	withTiming(t, 2*time.Second, true)
	steady, jittery := newEstimator(), newEstimator()
	for i := range 50 {
		steady.sample(100 * time.Millisecond)
		jittery.sample(time.Duration(50+100*(i%2)) * time.Millisecond)
	}
	if steady.timeout() >= jittery.timeout() {
		t.Errorf("jittery timeout %s should exceed steady timeout %s", jittery.timeout(), steady.timeout())
	}
}

func TestEstimatorRecheckSampling(t *testing.T) {
	withTiming(t, 2*time.Second, true)
	e := newEstimator()
	e.sample(10 * time.Millisecond)
	rechecks := 0
	for range 100 {
		if e.shouldRecheck() {
			rechecks++
		}
	}
	if want := 100 / recheckEvery; rechecks != want {
		t.Errorf("got %d rechecks of 100 timeouts, want %d", rechecks, want)
	}
}

func TestEstimatorMissRaisesTimeout(t *testing.T) {
	withTiming(t, 2*time.Second, true)
	e := newEstimator()
	for range 20 {
		e.sample(10 * time.Millisecond)
	}
	e.missed(700 * time.Millisecond)
	if got := e.timeout(); got < 1400*time.Millisecond {
		t.Errorf("got %s after a port needed 700ms, want at least 1.4s", got)
	}
	if e.missedCount() != 1 {
		t.Errorf("got %d misses, want 1", e.missedCount())
	}
}

func repeat(d time.Duration, n int) []time.Duration {
	s := make([]time.Duration, n)
	for i := range s {
		s[i] = d
	}
	return s
}
