package main

import (
	"math/rand"
	"slices"
	"testing"
)

// result is a test input: the result for one port.
type result struct {
	port int
	open bool
}

// runWriter feeds results to a resultWriter in the given order, flushes it,
// and returns every line printed.
func runWriter(t *testing.T, fmtName string, showOpen, showClosed bool, ports []int, kinds []string, order []result) []string {
	t.Helper()
	*format, *open, *closed = fmtName, showOpen, showClosed
	var lines []string
	w := newResultWriter(ports, func(s string) { lines = append(lines, s) })
	for _, k := range kinds {
		for _, r := range order {
			w.add(&job{kind: k, port: r.port, open: r.open})
		}
	}
	w.flush()
	return lines
}

func TestRanges(t *testing.T) {
	ports := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	in := []result{
		{1, false}, {2, false}, {3, true}, {4, false}, {5, false},
		{6, true}, {7, true}, {8, true}, {9, false}, {10, false},
	}
	want := []string{
		"CLOSED tcp 1-2",
		"OPEN tcp 3",
		"CLOSED tcp 4-5",
		"OPEN tcp 6-8",
		"CLOSED tcp 9-10",
	}
	got := runWriter(t, formatRanges, true, true, ports, []string{"tcp"}, in)
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRangesOutOfOrder(t *testing.T) {
	ports := make([]int, 0, 2000)
	in := make([]result, 0, 2000)
	for p := 1; p <= 2000; p++ {
		ports = append(ports, p)
		in = append(in, result{p, p%300 < 40})
	}
	want := runWriter(t, formatRanges, true, true, ports, []string{"tcp"}, in)

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		shuffled := slices.Clone(in)
		rng.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		got := runWriter(t, formatRanges, true, true, ports, []string{"tcp"}, shuffled)
		if !slices.Equal(got, want) {
			t.Fatalf("shuffle %d: got %q, want %q", i, got, want)
		}
	}
}

func TestRangesStreaming(t *testing.T) {
	// a range is printed as soon as it is final, before testing finishes
	*format, *open, *closed = formatRanges, true, true
	var lines []string
	w := newResultWriter([]int{1, 2, 3, 4}, func(s string) { lines = append(lines, s) })
	w.add(&job{kind: "tcp", port: 2, open: false})
	w.add(&job{kind: "tcp", port: 3, open: true})
	if len(lines) != 0 {
		t.Fatalf("printed %q before port 1 finished", lines)
	}
	w.add(&job{kind: "tcp", port: 1, open: false})
	if want := []string{"CLOSED tcp 1-2"}; !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestRangesGaps(t *testing.T) {
	// ports that are not adjacent are never merged, even with the same result
	ports := []int{22, 80, 81, 443}
	in := []result{{22, false}, {80, false}, {81, false}, {443, false}}
	want := []string{"CLOSED tcp 22", "CLOSED tcp 80-81", "CLOSED tcp 443"}
	got := runWriter(t, formatRanges, true, true, ports, []string{"tcp"}, in)
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRangesFilterAndKinds(t *testing.T) {
	ports := []int{1, 2, 3}
	in := []result{{1, true}, {2, false}, {3, true}}
	got := runWriter(t, formatRanges, true, false, ports, []string{"tcp4", "udp4"}, in)
	// the last range of each kind is only final once testing finishes
	want := []string{"OPEN tcp4 1", "OPEN udp4 1", "OPEN tcp4 3", "OPEN udp4 3"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRangesInterrupted(t *testing.T) {
	// on flush only ports whose results, and all those below, are known are printed
	*format, *open, *closed = formatRanges, true, true
	var lines []string
	w := newResultWriter([]int{1, 2, 3, 4, 5}, func(s string) { lines = append(lines, s) })
	for _, p := range []int{1, 2, 4, 5} {
		w.add(&job{kind: "tcp", port: p, open: false})
	}
	w.flush()
	if want := []string{"CLOSED tcp 1-2"}; !slices.Equal(lines, want) {
		t.Errorf("got %q, want %q", lines, want)
	}
}

func TestPortsFormat(t *testing.T) {
	ports := []int{1, 2, 3}
	in := []result{{2, false}, {1, true}, {3, false}}
	want := []string{"CLOSED tcp 2", "OPEN tcp 1", "CLOSED tcp 3"}
	got := runWriter(t, formatPorts, true, true, ports, []string{"tcp"}, in)
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
