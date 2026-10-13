package scan

import (
	"context"
	"net"
	"testing"
	"time"
)

// startTCPHandler starts a localhost TCP server that hands each connection
// to handle, and returns its port.
func startTCPHandler(t *testing.T, handle func(*net.TCPConn)) int {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				handle(c.(*net.TCPConn))
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// newTCPTarget returns a TCP target for localhost with the default options.
func newTCPTarget() *target {
	opts := DefaultOptions()
	return &target{
		kind:    "tcp",
		network: "tcp4",
		addr:    &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)},
		opts:    &opts,
		magic:   []byte(opts.Password),
	}
}

func TestCheckTCP(t *testing.T) {
	tests := []struct {
		name string
		port int
		want outcome
	}{
		{"open", startTCPServer(t), outcomeOpen},
		{"refused", unusedTCPPort(t), outcomeRefused},
		{"wrong reply", startTCPHandler(t, func(c *net.TCPConn) {
			_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		}), outcomeInterfered},
		{"silent", startTCPHandler(t, func(c *net.TCPConn) {
			time.Sleep(time.Second)
		}), outcomeSilent},
		{"reset", startTCPHandler(t, func(c *net.TCPConn) {
			_ = c.SetLinger(0)
		}), outcomeInterfered},
	}
	for _, tt := range tests {
		got, _ := newTCPTarget().checkTCP(context.Background(), tt.port, 300*time.Millisecond)
		if got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestOutcomeStatus(t *testing.T) {
	for o, want := range map[outcome]Status{
		outcomeOpen:       StatusOpen,
		outcomeRefused:    StatusRejected,
		outcomeTimeout:    StatusDropped,
		outcomeSilent:     StatusInterfered,
		outcomeInterfered: StatusInterfered,
		outcomeError:      StatusError,
	} {
		if got := o.status(); got != want {
			t.Errorf("outcome %d: got %s, want %s", o, got, want)
		}
	}
}
