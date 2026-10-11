package scan

import (
	"math"
	"time"
)

const (
	// etaWarmup is how long a scan runs before an estimate is given.
	etaWarmup = 2 * time.Second
	// etaSettle is the time constant with which the estimate moves toward a
	// new value, instead of jumping to it.
	etaSettle = 5 * time.Second
)

// ETA estimates the time left in a scan.
//
// It uses the average rate over the whole scan so far. A scan's speed
// depends on the ports being tested: ranges where something drops the
// traffic are much slower than ranges that answer. The recent rate would
// project a slow range onto every port left, while the whole-scan average
// already reflects the mix of fast and slow ranges, so a slow range only
// nudges it. The estimate itself counts down in real time between updates
// and eases toward each new value, so it does not jump around.
type ETA struct {
	total   int
	started time.Time

	estimate  time.Duration // the last estimate given
	estimated time.Time     // when it was given
	estimates bool          // whether an estimate has been given
}

// NewETA returns an ETA for a scan of total checks starting at now.
func NewETA(total int, now time.Time) *ETA {
	return &ETA{total: total, started: now}
}

// Update records that done checks are finished at now, and returns the
// estimated time left. It returns false while there is too little data for
// an estimate.
func (e *ETA) Update(done int, now time.Time) (time.Duration, bool) {
	if done >= e.total {
		return 0, true
	}
	elapsed := now.Sub(e.started)
	if done <= 0 || elapsed < etaWarmup {
		return 0, false
	}

	target := time.Duration(float64(elapsed) * float64(e.total-done) / float64(done))
	if !e.estimates {
		e.estimate, e.estimated, e.estimates = target, now, true
		return target, true
	}
	dt := now.Sub(e.estimated)
	// count down from the last estimate, then ease toward the new one
	projected := max(e.estimate-dt, 0)
	e.estimate = projected + time.Duration(smoothing(dt, etaSettle)*float64(target-projected))
	e.estimated = now
	return e.estimate, true
}

// smoothing returns the weight of a new value after dt in an exponential
// moving average with time constant tau.
func smoothing(dt, tau time.Duration) float64 {
	return 1 - math.Exp(-dt.Seconds()/tau.Seconds())
}

// RoundETA rounds an estimate for display, so it changes in steps that
// match its precision: up to the next 5 seconds below a minute, 10 seconds
// below 10 minutes, and whole minutes beyond. Below a minute it rounds up so
// a scan that is not finished never shows 0s left.
func RoundETA(d time.Duration) time.Duration {
	switch {
	case d <= 0:
		return 0
	case d < time.Minute:
		return (d + 5*time.Second - 1) / (5 * time.Second) * (5 * time.Second)
	case d < 10*time.Minute:
		return d.Round(10 * time.Second)
	default:
		return d.Round(time.Minute)
	}
}
