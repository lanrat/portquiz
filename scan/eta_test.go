package scan

import (
	"math/rand"
	"testing"
	"time"
)

// simulate runs an ETA over a scan of total checks whose speed at elapsed
// time t is rate(t) checks per second, delivered in random bursts every
// 100ms like real results. Every 200ms, as the progress bar redraws, it
// calls check with the elapsed time, the true time left and the estimate.
func simulate(total int, rate func(time.Duration) float64, check func(t, truth, est time.Duration, ok bool)) {
	rng := rand.New(rand.NewSource(1))
	start := time.Unix(0, 0)
	e := NewETA(total, start)
	done := 0.0
	// the true finish time, from the same schedule without randomness
	var finish time.Duration
	for t, d := time.Duration(0), 0.0; d < float64(total); t += 100 * time.Millisecond {
		d += rate(t) / 10
		finish = t
	}
	pending := 0.0
	for t := time.Duration(0); int(done) < total; t += 100 * time.Millisecond {
		// results arrive in uneven bursts around the true rate
		pending += rate(t) / 10
		burst := pending * rng.Float64() * 2
		if burst > pending {
			burst = pending
		}
		pending -= burst
		done = min(done+burst, float64(total))
		if t%(200*time.Millisecond) == 0 {
			est, ok := e.Update(int(done), start.Add(t))
			check(t, finish-t, est, ok)
		}
		if t > time.Hour {
			return
		}
	}
}

func TestETASteadyRateIsAccurateAndStable(t *testing.T) {
	steady := func(time.Duration) float64 { return 500 }
	var prev time.Duration
	var increases, samples int
	simulate(60000, steady, func(at, truth, est time.Duration, ok bool) {
		if at < etaWarmup {
			if ok {
				t.Fatalf("estimate at %s, before the warmup", at)
			}
			return
		}
		if !ok {
			t.Fatalf("no estimate at %s", at)
		}
		if truth > 10*time.Second {
			if err := (est - truth).Abs(); err > truth/10+5*time.Second {
				t.Errorf("at %s: estimate %s, truth %s", at, est, truth)
			}
		}
		// the displayed value should count down, not bounce back up
		shown := RoundETA(est)
		if samples > 0 && shown > prev {
			increases++
		}
		prev = shown
		samples++
	})
	if increases > samples/100 {
		t.Errorf("displayed estimate went up %d times in %d updates", increases, samples)
	}
}

func TestETASlowRangeOnlyNudgesIt(t *testing.T) {
	// fast, except for a stretch of ports whose traffic is dropped, which
	// take the full timeout; the stretch should not multiply the estimate
	slowRange := func(at time.Duration) float64 {
		if at >= 60*time.Second && at < 90*time.Second {
			return 100
		}
		return 2000
	}
	var lowest, highest time.Duration
	simulate(300000, slowRange, func(at, truth, est time.Duration, ok bool) {
		if !ok || at < 20*time.Second || truth < 10*time.Second {
			return
		}
		// the estimate of when the scan ends should hold steady
		end := at + est
		if lowest == 0 || end < lowest {
			lowest = end
		}
		highest = max(highest, end)
		if err := (est - truth).Abs(); err > truth/2+5*time.Second {
			t.Errorf("at %s: estimate %s, truth %s", at, est, truth)
		}
	})
	if highest > lowest*3/2 {
		t.Errorf("estimated end varied from %s to %s", lowest, highest)
	}
}

func TestETAStall(t *testing.T) {
	start := time.Unix(0, 0)
	e := NewETA(1000, start)
	var before time.Duration
	for i := 1; i <= 10; i++ {
		before, _ = e.Update(i*50, start.Add(time.Duration(i)*time.Second))
	}
	// no progress for 20 seconds: the estimate should grow, not keep falling
	after, ok := e.Update(500, start.Add(30*time.Second))
	if !ok || after <= before {
		t.Errorf("after a 20s stall: estimate %s (ok %v), before %s", after, ok, before)
	}
}

func TestETADoneAndNoProgress(t *testing.T) {
	start := time.Unix(0, 0)
	e := NewETA(100, start)
	if _, ok := e.Update(0, start.Add(5*time.Second)); ok {
		t.Error("estimate without any progress")
	}
	if d, ok := e.Update(100, start.Add(6*time.Second)); !ok || d != 0 {
		t.Errorf("finished scan: got %s, %v; want 0, true", d, ok)
	}
}

func TestRoundETA(t *testing.T) {
	for _, c := range []struct{ in, want time.Duration }{
		{-time.Second, 0},
		{0, 0},
		{time.Millisecond, 5 * time.Second},
		{41 * time.Second, 45 * time.Second},
		{45 * time.Second, 45 * time.Second},
		{59 * time.Second, time.Minute},
		{84 * time.Second, 80 * time.Second},
		{86 * time.Second, 90 * time.Second},
		{9*time.Minute + 54*time.Second, 9*time.Minute + 50*time.Second},
		{12*time.Minute + 40*time.Second, 13 * time.Minute},
		{65*time.Minute + 10*time.Second, 65 * time.Minute},
	} {
		if got := RoundETA(c.in); got != c.want {
			t.Errorf("RoundETA(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}
