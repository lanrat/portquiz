package scan

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestNewProbe(t *testing.T) {
	a, b := newProbe([]byte("portquiz")), newProbe([]byte("portquiz"))
	if len(a) != len("portquiz")+1+probeTagLen || !bytes.HasPrefix(a, []byte("portquiz\x00")) {
		t.Errorf("probe % x", a)
	}
	if bytes.Equal(a, b) {
		t.Error("two probes are the same")
	}
}

func TestIsAnswer(t *testing.T) {
	pw := []byte("portquiz")
	probe := []byte("portquiz\x0012345678")
	for reply, want := range map[string]bool{
		"portquiz\x0012345678":  true,  // the probe sent back
		"portquiz":              true,  // an older server over TCP
		"portquiz\x0012345679":  false, // another check's tag
		"portquiz123":           false,
		"portqui":               false,
		"":                      false,
		"portquiz\x0012345678x": false,
	} {
		if got := isAnswer([]byte(reply), probe, pw); got != want {
			t.Errorf("isAnswer(%q) = %v", reply, got)
		}
	}
	for reply, want := range map[string]bool{
		"port":                 true,
		"portquiz\x00123":      true,
		"portquiz":             false, // already an answer
		"portquiz\x0099":       false,
		"portquiz\x0012345678": false,
		"x":                    false,
	} {
		if got := isPartialAnswer([]byte(reply), probe, pw); got != want {
			t.Errorf("isPartialAnswer(%q) = %v", reply, got)
		}
	}
}

func TestCheckTCPTaggedAnswers(t *testing.T) {
	var mu sync.Mutex
	var probes [][]byte
	record := func(p []byte) {
		mu.Lock()
		probes = append(probes, append([]byte{}, p...))
		mu.Unlock()
	}
	read := func(c *net.TCPConn) []byte {
		buf := make([]byte, 64)
		n, _ := c.Read(buf)
		record(buf[:n])
		return buf[:n]
	}
	tests := []struct {
		name string
		port int
		want outcome
	}{
		{"probe sent back", startTCPHandler(t, func(c *net.TCPConn) {
			_, _ = c.Write(read(c))
			time.Sleep(100 * time.Millisecond)
		}), outcomeOpen},
		{"probe sent back in two parts", startTCPHandler(t, func(c *net.TCPConn) {
			p := read(c)
			_, _ = c.Write(p[:5])
			time.Sleep(50 * time.Millisecond)
			_, _ = c.Write(p[5:])
			time.Sleep(100 * time.Millisecond)
		}), outcomeOpen},
		{"another tag", startTCPHandler(t, func(c *net.TCPConn) {
			p := read(c)
			p[len(p)-1] ^= 0xff
			_, _ = c.Write(p)
			time.Sleep(100 * time.Millisecond)
		}), outcomeInterfered},
		{"longer password", startTCPHandler(t, func(c *net.TCPConn) {
			read(c)
			_, _ = c.Write([]byte("portquiz123"))
			time.Sleep(100 * time.Millisecond)
		}), outcomeInterfered},
	}
	for _, tt := range tests {
		got, _ := newTCPTarget().checkTCP(context.Background(), tt.port, time.Second)
		if got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(probes) != len(tests) {
		t.Fatalf("%d probes for %d checks", len(probes), len(tests))
	}
	for i := range probes {
		for j := range i {
			if bytes.Equal(probes[i], probes[j]) {
				t.Errorf("checks %d and %d sent the same probe", i, j)
			}
		}
	}
}

func TestCheckUDPTaggedAnswers(t *testing.T) {
	tests := []struct {
		name  string
		reply func([]byte) []byte
		want  outcome
	}{
		{"probe sent back", echo, outcomeOpen},
		{"bare password, as from an older server", func([]byte) []byte { return []byte("portquiz") }, outcomeOpen},
		{"another tag", func(p []byte) []byte {
			p = append([]byte{}, p...)
			p[len(p)-1] ^= 0xff
			return p
		}, outcomeInterfered},
	}
	for _, tt := range tests {
		got, _ := newUDPTarget().checkUDP(context.Background(), startUDPServer(t, tt.reply), 500*time.Millisecond)
		if got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
