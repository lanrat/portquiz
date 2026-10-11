package mobile

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"slices"
	"sync"
	"testing"
	"time"
)

// recorder is a Listener that records every event.
type recorder struct {
	mu       sync.Mutex
	ranges   []string
	progress [][4]int
	summary  *Summary
	finished chan struct{}
}

func newRecorder() *recorder { return &recorder{finished: make(chan struct{})} }

func (r *recorder) OnRange(kind string, first, last int, open bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ranges = append(r.ranges, fmt.Sprintf("%s %d-%d %v", kind, first, last, open))
}

func (r *recorder) OnProgress(done, total, open, etaMillis int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress = append(r.progress, [4]int{done, total, open, etaMillis})
}

func (r *recorder) OnFinish(s *Summary) {
	r.mu.Lock()
	r.summary = s
	r.mu.Unlock()
	close(r.finished)
}

// startTCPServer starts a TCP server on localhost that answers like the
// portquiz server, and returns its port.
func startTCPServer(t *testing.T) int {
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
				buf := make([]byte, 128)
				n, _ := c.Read(buf)
				if bytes.HasPrefix(buf[:n], []byte("portquiz")) {
					_, _ = c.Write([]byte("portquiz"))
					_, _ = io.Copy(io.Discard, c)
				}
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func TestNewConfigDefaults(t *testing.T) {
	c := NewConfig()
	if !c.TCP || c.UDP || c.TimeoutMillis != 2000 || !c.Adaptive || c.Retries != -1 || c.Password != "portquiz" {
		t.Errorf("unexpected defaults: %+v", c)
	}
}

func TestScan(t *testing.T) {
	open := startTCPServer(t)
	c := NewConfig()
	c.Host = "127.0.0.1"
	c.Ports = fmt.Sprintf("%d-%d", open, open+2)
	c.Rate = 0
	r := newRecorder()
	m, err := Start(c, r)
	if err != nil {
		t.Fatal(err)
	}
	<-r.finished
	if got := m.Addresses(); got != "127.0.0.1" {
		t.Errorf("addresses: got %q, want 127.0.0.1", got)
	}

	want := []string{fmt.Sprintf("tcp %d-%d true", open, open), fmt.Sprintf("tcp %d-%d false", open+1, open+2)}
	if !slices.Equal(r.ranges, want) {
		t.Errorf("ranges: got %q, want %q", r.ranges, want)
	}
	if got := r.progress[len(r.progress)-1]; got != [4]int{3, 3, 1, 0} {
		t.Errorf("last progress: got %v, want [3 3 1 0]", got)
	}
	s := r.summary
	if s.Done != 3 || s.Total != 3 || s.Open != 1 || s.Canceled || s.Error != "" {
		t.Errorf("summary: got %+v", s)
	}
}

func TestCancel(t *testing.T) {
	c := NewConfig()
	c.Host = "127.0.0.1"
	c.Ports = "40000-41999"
	c.TCP, c.UDP = false, true
	c.Rate = 1
	r := newRecorder()
	m, err := Start(c, r)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(100*time.Millisecond, m.Cancel)
	select {
	case <-r.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("OnFinish not called after Cancel")
	}
	m.AwaitFinish()
	if !r.summary.Canceled || r.summary.Done >= r.summary.Total {
		t.Errorf("summary: got %+v, want a canceled scan", r.summary)
	}
	m.Cancel() // a second call is harmless
}

func TestStartErrors(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"no host":   func(c *Config) { c.Host = "" },
		"bad ports": func(c *Config) { c.Ports = "10-5" },
		"timeout":   func(c *Config) { c.TimeoutMillis = 0 },
	} {
		c := NewConfig()
		c.Host = "127.0.0.1"
		change(c)
		r := newRecorder()
		if _, err := Start(c, r); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if r.summary != nil || len(r.progress) != 0 {
			t.Errorf("%s: listener was called", name)
		}
	}
}

func TestPassword(t *testing.T) {
	open := startTCPServer(t)
	for _, c := range []struct {
		password string
		want     string
	}{
		{"portquiz", fmt.Sprintf("tcp %d-%d true", open, open)},
		{"wrong", fmt.Sprintf("tcp %d-%d false", open, open)},
	} {
		cfg := NewConfig()
		cfg.Host = "127.0.0.1"
		cfg.Ports = fmt.Sprint(open)
		cfg.Rate = 0
		cfg.TimeoutMillis = 300
		cfg.Password = c.password
		r := newRecorder()
		if _, err := Start(cfg, r); err != nil {
			t.Fatal(err)
		}
		<-r.finished
		if !slices.Equal(r.ranges, []string{c.want}) {
			t.Errorf("password %q: got %q, want %q", c.password, r.ranges, c.want)
		}
	}
}
