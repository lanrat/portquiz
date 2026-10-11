package scan

import (
	"bytes"
	"context"
	"io"
	"net"
	"slices"
	"testing"
	"time"
)

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

// unusedTCPPort returns a localhost TCP port with nothing listening on it.
func unusedTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func testOptions(ports ...int) Options {
	opts := DefaultOptions()
	opts.Host = "127.0.0.1"
	opts.Ports = ports
	opts.Timeout = 500 * time.Millisecond
	opts.Rate = 0
	return opts
}

// collect runs a scan to completion and returns its results.
func collect(t *testing.T, opts Options) (*Scan, []Result) {
	t.Helper()
	s, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	var results []Result
	for r := range s.Results() {
		results = append(results, r)
	}
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	return s, results
}

func TestScanTCPAndUDP(t *testing.T) {
	openTCP, closedTCP := startTCPServer(t), unusedTCPPort(t)
	openUDP := startUDPServer(t, echo)
	opts := testOptions(openTCP, closedTCP, openUDP)
	opts.TCP, opts.UDP = true, true

	s, results := collect(t, opts)
	if got, want := s.Kinds(), []string{"tcp", "udp"}; !slices.Equal(got, want) {
		t.Errorf("kinds: got %v, want %v", got, want)
	}
	if len(results) != s.Total() || s.Total() != 6 {
		t.Fatalf("got %d results, Total %d, want 6", len(results), s.Total())
	}
	open := map[Result]bool{}
	for _, r := range results {
		open[Result{Kind: r.Kind, Port: r.Port}] = r.Open
	}
	for _, c := range []struct {
		kind string
		port int
		want bool
	}{
		{"tcp", openTCP, true},
		{"tcp", closedTCP, false},
		{"udp", openUDP, true},
	} {
		if got := open[Result{Kind: c.kind, Port: c.port}]; got != c.want {
			t.Errorf("%s %d: open=%v, want %v", c.kind, c.port, got, c.want)
		}
	}
	if len(s.Missed()) != 0 {
		t.Errorf("unexpected misses: %v", s.Missed())
	}
}

func TestScanDefaultsToTCP(t *testing.T) {
	s, _ := collect(t, testOptions(startTCPServer(t)))
	if got := s.Kinds(); !slices.Equal(got, []string{"tcp"}) {
		t.Errorf("got kinds %v, want [tcp]", got)
	}
}

func TestScanSortsAndDedupesPorts(t *testing.T) {
	p := startTCPServer(t)
	s, results := collect(t, testOptions(p, p, p))
	if len(s.Ports()) != 1 || len(results) != 1 {
		t.Errorf("got ports %v and %d results, want one", s.Ports(), len(results))
	}
}

func TestScanCancel(t *testing.T) {
	// 2000 unanswered UDP ports would take many seconds at a 1 port/s rate
	opts := testOptions()
	opts.Ports, _ = ParsePorts("40000-41999")
	opts.TCP, opts.UDP = false, true
	opts.Rate = 1
	ctx, cancel := context.WithCancel(context.Background())
	s, err := Start(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	for range s.Results() {
	}
	if err := s.Wait(); err != nil {
		t.Errorf("canceled scan returned %v, want nil", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("results closed %s after cancel, want promptly", time.Since(start))
	}
}

func TestStartValidates(t *testing.T) {
	for name, change := range map[string]func(*Options){
		"no host":         func(o *Options) { o.Host = "" },
		"zero timeout":    func(o *Options) { o.Timeout = 0 },
		"zero parallel":   func(o *Options) { o.Parallel = 0 },
		"zero multi":      func(o *Options) { o.Multi = 0 },
		"negative rate":   func(o *Options) { o.Rate = -1 },
		"empty password":  func(o *Options) { o.Password = "" },
		"port 0":          func(o *Options) { o.Ports = []int{0} },
		"port too high":   func(o *Options) { o.Ports = []int{MaxPort + 1} },
		"unresolvable v6": func(o *Options) { o.IPv6 = true },
	} {
		opts := testOptions(80)
		change(&opts)
		if _, err := Start(context.Background(), opts); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestStartHosts(t *testing.T) {
	for host, want := range map[string]string{
		"127.0.0.1":        "127.0.0.1",
		" 127.0.0.1\n":     "127.0.0.1",
		"::ffff:127.0.0.1": "127.0.0.1",
		"::1":              "::1",
		"[::1]":            "::1",
	} {
		opts := testOptions(80)
		opts.Host = host
		ctx, cancel := context.WithCancel(context.Background())
		s, err := Start(ctx, opts)
		cancel()
		if err != nil {
			t.Errorf("%q: %v", host, err)
			continue
		}
		_ = s.Wait()
		addrs := s.Addrs()
		if len(addrs) != 1 || (want != "" && addrs[0].String() != want) {
			t.Errorf("%q: addresses %v, want [%s]", host, addrs, want)
		}
	}
}

func TestStartAddrsForBothVersions(t *testing.T) {
	opts := testOptions(80)
	opts.Host = "localhost"
	opts.IPv4, opts.IPv6 = true, true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := Start(ctx, opts)
	if err != nil {
		t.Skipf("localhost has no IPv6 address here: %v", err)
	}
	cancel()
	_ = s.Wait()
	addrs := s.Addrs()
	if len(addrs) != 2 || addrs[0].IP.To4() == nil || addrs[1].IP.To4() != nil {
		t.Errorf("addresses %v, want IPv4 then IPv6", addrs)
	}
}

func TestStartWrongVersionAddress(t *testing.T) {
	for _, c := range []struct {
		host       string
		ipv4, ipv6 bool
		want       string
	}{
		{"127.0.0.1", false, true, "127.0.0.1 is an IPv4 address and cannot be tested over IPv6"},
		{"::1", true, false, "::1 is an IPv6 address and cannot be tested over IPv4"},
		{"[::1]", true, true, "::1 is an IPv6 address and cannot be tested over IPv4"},
	} {
		opts := testOptions(80)
		opts.Host = c.host
		opts.IPv4, opts.IPv6 = c.ipv4, c.ipv6
		_, err := Start(context.Background(), opts)
		if err == nil || err.Error() != c.want {
			t.Errorf("%q: got error %v, want %q", c.host, err, c.want)
		}
	}
}

// fakeDNS makes every name resolve to addrs, with a route only to the
// addresses in reachable, until the test ends.
func fakeDNS(t *testing.T, addrs []string, reachable ...string) {
	t.Helper()
	lookup, route := lookupIPAddr, routable
	t.Cleanup(func() { lookupIPAddr, routable = lookup, route })
	lookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
		var out []net.IPAddr
		for _, a := range addrs {
			out = append(out, net.IPAddr{IP: net.ParseIP(a)})
		}
		return out, nil
	}
	routable = func(a *net.IPAddr) bool { return slices.Contains(reachable, a.String()) }
}

func TestStartAutoVersions(t *testing.T) {
	for _, c := range []struct {
		name      string
		addrs     []string
		reachable []string
		kinds     []string
		tested    []string
	}{
		{"dual stack", []string{"::1", "127.0.0.2", "127.0.0.1"}, []string{"::1", "127.0.0.2"},
			[]string{"tcp4", "udp4", "tcp6", "udp6"}, []string{"127.0.0.2", "::1"}},
		{"no IPv6 route", []string{"::1", "127.0.0.1"}, []string{"127.0.0.1"},
			[]string{"tcp", "udp"}, []string{"127.0.0.1"}},
		{"no IPv4 route", []string{"::1", "127.0.0.1"}, []string{"::1"},
			[]string{"tcp", "udp"}, []string{"::1"}},
		{"no route", []string{"::1", "127.0.0.1"}, nil,
			[]string{"tcp", "udp"}, []string{"127.0.0.1"}},
		{"IPv6 only", []string{"::1"}, []string{"::1"},
			[]string{"tcp", "udp"}, []string{"::1"}},
		{"mapped IPv4", []string{"::ffff:127.0.0.1"}, []string{"127.0.0.1"},
			[]string{"tcp", "udp"}, []string{"127.0.0.1"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeDNS(t, c.addrs, c.reachable...)
			opts := testOptions(80)
			opts.Host = "dual.example"
			opts.TCP, opts.UDP = true, true
			ctx, cancel := context.WithCancel(context.Background())
			s, err := Start(ctx, opts)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			_ = s.Wait()
			var tested []string
			for _, a := range s.Addrs() {
				tested = append(tested, a.String())
			}
			if !slices.Equal(s.Kinds(), c.kinds) || !slices.Equal(tested, c.tested) {
				t.Errorf("kinds %v at %v, want %v at %v", s.Kinds(), tested, c.kinds, c.tested)
			}
		})
	}
}

func TestStartAutoNoAddresses(t *testing.T) {
	fakeDNS(t, nil)
	opts := testOptions(80)
	opts.Host = "empty.example"
	if _, err := Start(context.Background(), opts); err == nil {
		t.Error("expected an error")
	}
}

func TestRoutable(t *testing.T) {
	if !routable(&net.IPAddr{IP: net.ParseIP("127.0.0.1")}) {
		t.Error("no route to 127.0.0.1")
	}
}
