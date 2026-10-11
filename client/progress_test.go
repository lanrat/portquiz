package main

import (
	"testing"
	"time"
)

func TestFormatETA(t *testing.T) {
	for _, c := range []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{time.Minute, "1m"},
		{3*time.Minute + 20*time.Second, "3m20s"},
		{12 * time.Minute, "12m"},
		{time.Hour, "1h"},
		{65 * time.Minute, "1h5m"},
	} {
		if got := formatETA(c.in); got != c.want {
			t.Errorf("formatETA(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}
