package main

import (
	"net"
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

func TestTestingLine(t *testing.T) {
	v4 := &net.IPAddr{IP: net.ParseIP("192.0.2.1")}
	v6 := &net.IPAddr{IP: net.ParseIP("2001:db8::1")}
	for _, c := range []struct {
		host  string
		addrs []*net.IPAddr
		want  string
	}{
		{"example.com", []*net.IPAddr{v4}, "Testing example.com at 192.0.2.1"},
		{"example.com", []*net.IPAddr{v4, v6}, "Testing example.com at 192.0.2.1 and 2001:db8::1"},
		{"192.0.2.1", []*net.IPAddr{v4}, "Testing 192.0.2.1"},
		{"[2001:db8::1]", []*net.IPAddr{v6}, "Testing 2001:db8::1"},
		{"::ffff:192.0.2.1", []*net.IPAddr{v4}, "Testing ::ffff:192.0.2.1 at 192.0.2.1"},
	} {
		if got := testingLine(c.host, c.addrs); got != c.want {
			t.Errorf("testingLine(%q) = %q, want %q", c.host, got, c.want)
		}
	}
}
