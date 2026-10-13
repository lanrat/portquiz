package scan

import (
	"context"
	"encoding/json"
	"net"
	"slices"
	"testing"
	"time"
)

// stoppedScan returns a scan of ports that is already canceled, for feeding
// results to a Recorder by hand.
func stoppedScan(t *testing.T, ports ...int) *Scan {
	t.Helper()
	opts := testOptions(ports...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err := Start(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Wait()
	return s
}

func TestRecorderRangesAndCounts(t *testing.T) {
	s := stoppedScan(t, 1, 2, 3, 4, 5, 7, 8, 9)
	start := time.Unix(1000, 0)
	r := NewRecorder(s, start)
	for _, res := range []Result{
		{Kind: "tcp", Port: 1, Status: StatusDropped},
		{Kind: "tcp", Port: 2, Status: StatusDropped},
		{Kind: "tcp", Port: 3, Status: StatusOpen, Open: true, RTT: 10 * time.Millisecond},
		{Kind: "tcp", Port: 4, Status: StatusOpen, Open: true, RTT: 30 * time.Millisecond, Late: true},
		{Kind: "tcp", Port: 5, Status: StatusRejected},
		// 6 is not tested, 7 has no result yet
		{Kind: "tcp", Port: 8, Status: StatusInterfered},
		{Kind: "tcp", Port: 9, Status: StatusError},
		{Kind: "udp", Port: 9, Status: StatusOpen}, // not a kind being tested
		{Kind: "tcp", Port: 6, Status: StatusOpen}, // not a port being tested
	} {
		r.Add(res)
	}
	rep := r.Report(start.Add(2*time.Second), true)

	if rep.Host != "127.0.0.1" || !rep.Canceled || rep.Seconds != 2 || rep.PerSecond != 3.5 || rep.Options.Ports != "1-5,7-9" {
		t.Errorf("report: %+v", rep)
	}
	if len(rep.Kinds) != 1 {
		t.Fatalf("kinds: %+v", rep.Kinds)
	}
	k := rep.Kinds[0]
	if k.Kind != "tcp" || k.Address != "127.0.0.1" || k.Tested != 7 || k.Untested != 1 ||
		k.Open != 2 || k.Rejected != 1 || k.Dropped != 2 || k.Interfered != 1 || k.Errors != 1 || k.Late != 1 {
		t.Errorf("counts: %+v", k)
	}
	want := []StatusRange{
		{1, 2, StatusDropped}, {3, 4, StatusOpen}, {5, 5, StatusRejected},
		{8, 8, StatusInterfered}, {9, 9, StatusError},
	}
	if !slices.Equal(k.Ranges, want) {
		t.Errorf("ranges: got %v, want %v", k.Ranges, want)
	}
	if k.OpenRTT == nil || *k.OpenRTT != (RTTStats{Min: 10, Median: 10, P95: 30, Max: 30}) {
		t.Errorf("open RTT: %+v", k.OpenRTT)
	}
}

func TestRTTStats(t *testing.T) {
	if rttStats(nil) != nil {
		t.Error("stats without samples")
	}
	var rtts []time.Duration
	for i := 100; i >= 1; i-- {
		rtts = append(rtts, time.Duration(i)*time.Millisecond+50*time.Microsecond)
	}
	got := *rttStats(rtts)
	if got != (RTTStats{Min: 1.1, Median: 50.1, P95: 95.1, Max: 100.1}) {
		t.Errorf("got %+v", got)
	}
}

func TestFormatPorts(t *testing.T) {
	for want, ports := range map[string][]int{
		"":              nil,
		"80":            {80},
		"1-3":           {1, 2, 3},
		"1-3,22,80-81":  {1, 2, 3, 22, 80, 81},
		"1-65535":       AllPorts(),
		"22,80,443-444": {22, 80, 443, 444},
	} {
		if got := formatPorts(ports); got != want {
			t.Errorf("formatPorts(%v) = %q, want %q", ports, got, want)
		}
	}
}

// TestReportFromScan runs a real scan against local servers that answer in
// each way and checks the report.
func TestReportFromScan(t *testing.T) {
	open := startTCPServer(t)
	refused := unusedTCPPort(t)
	interfered := startTCPHandler(t, func(c *net.TCPConn) {
		_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
	})
	opts := testOptions(open, refused, interfered)
	opts.Retries = 0
	start := time.Now()
	s, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(s, start)
	byPort := map[int]Status{}
	for res := range s.Results() {
		r.Add(res)
		byPort[res.Port] = res.Status
		if res.Open != (res.Status == StatusOpen) {
			t.Errorf("port %d: Open %v with status %s", res.Port, res.Open, res.Status)
		}
		if res.Open && res.RTT <= 0 {
			t.Errorf("port %d: open without a response time", res.Port)
		}
	}
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	want := map[int]Status{open: StatusOpen, refused: StatusRejected, interfered: StatusInterfered}
	for p, st := range want {
		if byPort[p] != st {
			t.Errorf("port %d: got %s, want %s", p, byPort[p], st)
		}
	}

	rep := r.Report(time.Now(), false)
	k := rep.Kinds[0]
	if k.Tested != 3 || k.Open != 1 || k.Rejected != 1 || k.Interfered != 1 || k.OpenRTT == nil {
		t.Errorf("kind report: %+v", k)
	}
	// it must survive a round trip through JSON
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(data, &back); err != nil || back.Kinds[0].Open != 1 || back.Kinds[0].Ranges[0].Status == "" {
		t.Errorf("round trip: %v, %s", err, data)
	}
}
