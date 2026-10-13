package scan

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

// startUDPServer starts a UDP server on localhost that replies to every packet
// with reply(packet), or not at all when reply returns nil. It returns its port.
func startUDPServer(t *testing.T, reply func([]byte) []byte) int {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if r := reply(buf[:n]); r != nil {
				_, _ = c.WriteToUDP(r, from)
			}
		}
	}()
	return c.LocalAddr().(*net.UDPAddr).Port
}

// echo replies with the packet, like the portquiz server.
func echo(b []byte) []byte { return bytes.Clone(b) }

// unusedUDPPort returns a localhost UDP port with nothing listening on it.
func unusedUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	return port
}

// newUDPTarget returns a UDP target for localhost with the default options.
func newUDPTarget() *target {
	opts := DefaultOptions()
	return &target{
		kind:    "udp",
		network: "udp4",
		addr:    &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)},
		udp:     true,
		opts:    &opts,
		magic:   []byte(opts.Password),
	}
}

func TestCheckUDP(t *testing.T) {
	tests := []struct {
		name string
		port int
		want outcome
	}{
		{"open", startUDPServer(t, echo), outcomeOpen},
		{"no reply", startUDPServer(t, func([]byte) []byte { return nil }), outcomeTimeout},
		{"wrong reply", startUDPServer(t, func([]byte) []byte { return []byte("nope") }), outcomeInterfered},
		{"refused", unusedUDPPort(t), outcomeRefused},
	}
	for _, tt := range tests {
		start := time.Now()
		got, _ := newUDPTarget().checkUDP(context.Background(), tt.port, 300*time.Millisecond)
		if got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
		if tt.want == outcomeRefused && time.Since(start) > 100*time.Millisecond {
			t.Errorf("%s: took %s, want an immediate refusal", tt.name, time.Since(start))
		}
	}
}

func TestCheckUDPCanceled(t *testing.T) {
	port := startUDPServer(t, func([]byte) []byte { return nil })
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	if out, _ := newUDPTarget().checkUDP(ctx, port, 5*time.Second); out == outcomeOpen || time.Since(start) > time.Second {
		t.Errorf("got %v after %s, want a prompt non-open result", out, time.Since(start))
	}
}
