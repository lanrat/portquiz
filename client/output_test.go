package main

import (
	"slices"
	"testing"

	"portquiz/scan"
)

// runPrinter feeds results to a printer in the given order, flushes it, and
// returns every line printed.
func runPrinter(t *testing.T, fmtName string, showOpen, showClosed bool, ports []int, results []scan.Result) []string {
	t.Helper()
	*format, *open, *closed = fmtName, showOpen, showClosed
	var lines []string
	w := newPrinter(ports, func(s string) { lines = append(lines, s) })
	for _, r := range results {
		w.add(r)
	}
	w.flush()
	return lines
}

func TestRangesFormat(t *testing.T) {
	results := []scan.Result{
		{Kind: "tcp", Port: 1}, {Kind: "tcp", Port: 2},
		{Kind: "tcp", Port: 3, Open: true}, {Kind: "tcp", Port: 4},
	}
	want := []string{"CLOSED tcp 1-2", "OPEN tcp 3", "CLOSED tcp 4"}
	if got := runPrinter(t, formatRanges, true, true, []int{1, 2, 3, 4}, results); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRangesFilter(t *testing.T) {
	results := []scan.Result{
		{Kind: "tcp4", Port: 1, Open: true}, {Kind: "tcp4", Port: 2}, {Kind: "tcp4", Port: 3, Open: true},
	}
	if got, want := runPrinter(t, formatRanges, true, false, []int{1, 2, 3}, results), []string{"OPEN tcp4 1", "OPEN tcp4 3"}; !slices.Equal(got, want) {
		t.Errorf("-open: got %q, want %q", got, want)
	}
	if got, want := runPrinter(t, formatRanges, false, true, []int{1, 2, 3}, results), []string{"CLOSED tcp4 2"}; !slices.Equal(got, want) {
		t.Errorf("-closed: got %q, want %q", got, want)
	}
}

func TestPortsFormat(t *testing.T) {
	results := []scan.Result{{Kind: "tcp", Port: 2}, {Kind: "tcp", Port: 1, Open: true}, {Kind: "tcp", Port: 3}}
	want := []string{"CLOSED tcp 2", "OPEN tcp 1", "CLOSED tcp 3"}
	if got := runPrinter(t, formatPorts, true, true, []int{1, 2, 3}, results); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
