package scan

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"os"
	"slices"
	"testing"
)

func readReport(t *testing.T, path string) *Report {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ParseReport(data)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestDiffMatchesTestdata compares the shared example reports, which the
// browser page's tests also diff, with the expected diff.
func TestDiffMatchesTestdata(t *testing.T) {
	d := DiffReports(readReport(t, "testdata/diff/a.json"), readReport(t, "testdata/diff/b.json"))
	got, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if os.Getenv("UPDATE_TESTDATA") != "" {
		if err := os.WriteFile("testdata/diff/want.json", got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/diff/want.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("diff:\n%s\nwant:\n%s", got, want)
	}
}

func kindOf(kind, addr string, ranges ...StatusRange) KindReport {
	return KindReport{Kind: kind, Address: addr, Ranges: ranges}
}

func TestDiffChanges(t *testing.T) {
	a := &Report{Kinds: []KindReport{kindOf("tcp", "192.0.2.1",
		StatusRange{1, 5, StatusOpen}, StatusRange{7, 10, StatusOpen},
		// ranges that were not merged still give merged changes
		StatusRange{20, 22, StatusRejected}, StatusRange{23, 25, StatusRejected})}}
	b := &Report{Kinds: []KindReport{kindOf("tcp", "192.0.2.2",
		StatusRange{1, 10, StatusDropped}, StatusRange{20, 21, StatusOpen}, StatusRange{22, 30, StatusOpen})}}
	d := DiffReports(a, b)
	if len(d.Kinds) != 1 {
		t.Fatalf("kinds: %+v", d.Kinds)
	}
	k := d.Kinds[0]
	want := []StatusChange{{1, 5, StatusOpen, StatusDropped}, {7, 10, StatusOpen, StatusDropped}, {20, 25, StatusRejected, StatusOpen}}
	if !slices.Equal(k.Changes, want) {
		t.Errorf("changes %+v, want %+v", k.Changes, want)
	}
	if k.Kind != "tcp4" || k.Compared != 15 || k.Closed != 9 || k.Opened != 6 || k.ReasonChanged != 0 || k.Unchanged != 0 ||
		k.OnlyA != 0 || k.OnlyB != 6 || k.Changed() != 15 || d.Changed() != 15 {
		t.Errorf("counts: %+v", k)
	}
}

func TestDiffIdentical(t *testing.T) {
	r := &Report{Kinds: []KindReport{kindOf("udp6", "2001:db8::1", StatusRange{1, 100, StatusDropped}, StatusRange{101, 200, StatusOpen})}}
	d := DiffReports(r, r)
	if d.Changed() != 0 || d.Kinds[0].Unchanged != 200 || d.Kinds[0].Kind != "udp6" || len(d.Kinds[0].Changes) != 0 {
		t.Errorf("diff: %+v", d)
	}
}

func TestKindKey(t *testing.T) {
	for _, c := range []struct{ kind, addr, want string }{
		{"tcp", "192.0.2.1", "tcp4"},
		{"tcp", "2001:db8::1", "tcp6"},
		{"udp", "fe80::1%eth0", "udp6"},
		{"tcp6", "", "tcp6"},
		{"udp4", "2001:db8::1", "udp4"},
		{"tcp", "", "tcp"},
	} {
		if got := kindKey(KindReport{Kind: c.kind, Address: c.addr}); got != c.want {
			t.Errorf("%s at %q: %s, want %s", c.kind, c.addr, got, c.want)
		}
	}
}

func TestParseReportRejects(t *testing.T) {
	for name, data := range map[string]string{
		"not json":   "hello",
		"not object": "[1, 2]",
		"no kinds":   `{"host": "x"}`,
		"bad port":   `{"kinds": [{"kind": "tcp", "ranges": [{"first": 0, "last": 3, "status": "open"}]}]}`,
		"too high":   `{"kinds": [{"kind": "tcp", "ranges": [{"first": 1, "last": 65536, "status": "open"}]}]}`,
		"backwards":  `{"kinds": [{"kind": "tcp", "ranges": [{"first": 5, "last": 3, "status": "open"}]}]}`,
		"overlap":    `{"kinds": [{"kind": "tcp", "ranges": [{"first": 1, "last": 3, "status": "open"}, {"first": 3, "last": 4, "status": "dropped"}]}]}`,
		"wrong type": `{"kinds": "tcp"}`,
		"a diff":     `{"a": {}, "b": {}, "kinds": []}`,
	} {
		if _, err := ParseReport([]byte(data)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
	if _, err := ParseReport([]byte(`{"kinds": []}`)); err != nil {
		t.Errorf("empty kinds: %s", err)
	}
}

// randomRanges returns ranges over ports 1 to n with random statuses and
// gaps, and the status of each port, "" for none.
func randomRanges(rng *rand.Rand, n int) ([]StatusRange, []Status) {
	choices := []Status{"", StatusOpen, StatusRejected, StatusDropped, statusBrowser}
	per := make([]Status, n+1)
	var out []StatusRange
	for p := 1; p <= n; {
		s := choices[rng.IntN(len(choices))]
		last := min(p+rng.IntN(5), n)
		for q := p; q <= last; q++ {
			per[q] = s
		}
		if s != "" {
			// sometimes split a range, as a report from elsewhere might
			if prev := len(out) - 1; prev >= 0 && out[prev].Last+1 == p && out[prev].Status == s && rng.IntN(2) == 0 {
				out[prev].Last = last
			} else {
				out = append(out, StatusRange{p, last, s})
			}
		}
		if s == statusBrowser {
			for q := p; q <= last; q++ {
				per[q] = ""
			}
		}
		p = last + 1
	}
	return out, per
}

// TestDiffAgainstPortByPort checks diffs of random reports against
// comparing each port.
func TestDiffAgainstPortByPort(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		n := 1 + rng.IntN(60)
		ar, pa := randomRanges(rng, n)
		br, pb := randomRanges(rng, n)
		// half the time A is the browser page's UDP, which cannot tell how
		// ports were closed
		ra := &Report{Kinds: []KindReport{{Kind: "tcp4", Ranges: ar}}}
		if rng.IntN(2) == 0 {
			ra = &Report{Client: "browser", Kinds: []KindReport{{Kind: "udp", Address: "192.0.2.1", Ranges: ar}}}
			for p := range pa {
				if pa[p] != "" && pa[p] != StatusOpen {
					pa[p] = statusClosed
				}
			}
		}
		rb := &Report{Kinds: []KindReport{{Kind: ra.Kinds[0].Kind[:3] + "4", Ranges: br}}}
		dr := DiffReports(ra, rb)
		if len(dr.Kinds) != 1 {
			t.Fatalf("kinds %+v", dr.Kinds)
		}
		d := dr.Kinds[0]
		want := map[Change]int{}
		wantPer := make([]Change, n+1)
		for p := 1; p <= n; p++ {
			if pa[p] == "" && pb[p] == "" {
				continue
			}
			wantPer[p] = compare(pa[p], pb[p])
			want[wantPer[p]]++
		}
		got := map[Change]int{
			ChangeUnchanged: d.Unchanged, ChangeOpened: d.Opened, ChangeClosed: d.Closed,
			ChangeReasonChanged: d.ReasonChanged, ChangeOnlyA: d.OnlyA, ChangeOnlyB: d.OnlyB,
		}
		for c, v := range got {
			if want[c] != v {
				t.Fatalf("%s: %d, want %d\na %v\nb %v", c, v, want[c], ar, br)
			}
		}
		gotPer := make([]Change, n+1)
		prev := 0
		for i, r := range d.Ranges {
			if r.First <= prev || (i > 0 && d.Ranges[i-1].Last+1 == r.First && d.Ranges[i-1].Change == r.Change) {
				t.Fatalf("ranges not merged or out of order: %v", d.Ranges)
			}
			prev = r.Last
			for p := r.First; p <= r.Last; p++ {
				gotPer[p] = r.Change
			}
		}
		if !slices.Equal(gotPer, wantPer) {
			t.Fatalf("ranges %v\nwant per port %v", d.Ranges, wantPer)
		}
		changed := 0
		for _, c := range d.Changes {
			for p := c.First; p <= c.Last; p++ {
				if pa[p] != c.A || pb[p] != c.B || c.A == c.B {
					t.Fatalf("change %+v wrong at %d", c, p)
				}
				changed++
			}
		}
		if changed != d.Changed() {
			t.Fatalf("changes cover %d ports, want %d", changed, d.Changed())
		}
	}
}
