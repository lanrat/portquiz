package scan

import (
	"math"
	"time"
)

const (
	// etaWarmup is how long a scan runs before an estimate is given.
	etaWarmup = 2 * time.Second
	// etaSample is how often the rate is sampled.
	etaSample = time.Second
	// etaRateWindow is the time constant of the rate's moving average:
	// roughly how far back the rate looks.
	etaRateWindow = 10 * time.Second
	// etaSettle is the time constant with which the estimate moves toward a
	// new value, instead of jumping to it.
	etaSettle = 5 * time.Second
)

// ETA estimates the time left in a scan.
//
// It uses the rate over roughly the last etaRateWindow, as an exponential
// moving average sampled every second, rather than the average over the
// whole scan, so it follows real changes in speed while ignoring bursts of
// results. The estimate itself counts down in real time between updates and
// eases toward each new value, so it does not jump around.
type ETA struct {
	total   int
	started time.Time

	sampled     time.Time // when the rate was last sampled
	sampledDone int       // checks done at that time
	rate        float64   // smoothed checks per second
	rated       bool      // whether rate has been measured

	estimate  time.Duration // the last estimate given
	estimated time.Time     // when it was given
	estimates bool          // whether an estimate has been given
}

// NewETA returns an ETA for a scan of total checks starting at now.
func NewETA(total int, now time.Time) *ETA {
	return &ETA{total: total, started: now, sampled: now}
}

// Update records that done checks are finished at now, and returns the
// estimated time left. It returns false while there is too little data for
// an estimate.
func (e *ETA) Update(done int, now time.Time) (time.Duration, bool) {
	if done >= e.total {
		return 0, true
	}

	if dt := now.Sub(e.sampled); dt >= etaSample {
		current := float64(done-e.sampledDone) / dt.Seconds()
		if e.rated {
			e.rate += smoothing(dt, etaRateWindow) * (current - e.rate)
		} else if current > 0 {
			e.rate, e.rated = current, true
		}
		e.sampled, e.sampledDone = now, done
	}
	if !e.rated || e.rate <= 0 || now.Sub(e.started) < etaWarmup {
		return 0, false
	}

	target := time.Duration(float64(e.total-done) / e.rate * float64(time.Second))
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
