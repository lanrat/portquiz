package scan

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"
)

// result is a test input: the result for one port.
type result struct {
	port int
	open bool
}

// format describes a range the way the command line client prints it.
func format(r Range) string {
	state := "CLOSED"
	if r.Open {
		state = "OPEN"
	}
	if r.First == r.Last {
		return fmt.Sprintf("%s %s %d", state, r.Kind, r.First)
	}
	return fmt.Sprintf("%s %s %d-%d", state, r.Kind, r.First, r.Last)
}

// runRanges feeds results for each kind to a Ranges in the given order,
// flushes it, and returns every range emitted.
func runRanges(ports []int, kinds []string, order []result) []string {
	var got []string
	r := NewRanges(ports, func(rg Range) { got = append(got, format(rg)) })
	for _, k := range kinds {
		for _, res := range order {
			r.Add(Result{Kind: k, Port: res.port, Open: res.open})
		}
	}
	r.Flush()
	return got
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
	if got := runRanges(ports, []string{"tcp"}, in); !slices.Equal(got, want) {
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
	want := runRanges(ports, []string{"tcp"}, in)

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		shuffled := slices.Clone(in)
		rng.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		if got := runRanges(ports, []string{"tcp"}, shuffled); !slices.Equal(got, want) {
			t.Fatalf("shuffle %d: got %q, want %q", i, got, want)
		}
	}
}

func TestRangesStreaming(t *testing.T) {
	// a range is emitted as soon as it is final, before testing finishes
	var got []string
	r := NewRanges([]int{1, 2, 3, 4}, func(rg Range) { got = append(got, format(rg)) })
	r.Add(Result{Kind: "tcp", Port: 2})
	r.Add(Result{Kind: "tcp", Port: 3, Open: true})
	if len(got) != 0 {
		t.Fatalf("emitted %q before port 1 finished", got)
	}
	r.Add(Result{Kind: "tcp", Port: 1})
	if want := []string{"CLOSED tcp 1-2"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRangesGaps(t *testing.T) {
	// ports that are not adjacent are never merged, even with the same result
	ports := []int{22, 80, 81, 443}
	in := []result{{22, false}, {80, false}, {81, false}, {443, false}}
	want := []string{"CLOSED tcp 22", "CLOSED tcp 80-81", "CLOSED tcp 443"}
	if got := runRanges(ports, []string{"tcp"}, in); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRangesKinds(t *testing.T) {
	ports := []int{1, 2, 3}
	in := []result{{1, true}, {2, false}, {3, true}}
	// the last range of each kind is only final once testing finishes
	want := []string{"OPEN tcp4 1", "CLOSED tcp4 2", "OPEN udp4 1", "CLOSED udp4 2", "OPEN tcp4 3", "OPEN udp4 3"}
	if got := runRanges(ports, []string{"tcp4", "udp4"}, in); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRangesInterrupted(t *testing.T) {
	// on flush only ports whose results, and all those below, are known are emitted
	var got []string
	r := NewRanges([]int{1, 2, 3, 4, 5}, func(rg Range) { got = append(got, format(rg)) })
	for _, p := range []int{1, 2, 4, 5} {
		r.Add(Result{Kind: "tcp", Port: p})
	}
	r.Flush()
	if want := []string{"CLOSED tcp 1-2"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRangesIgnoresUntestedPorts(t *testing.T) {
	var got []string
	r := NewRanges([]int{1, 2}, func(rg Range) { got = append(got, format(rg)) })
	r.Add(Result{Kind: "tcp", Port: 99, Open: true})
	r.Add(Result{Kind: "tcp", Port: 1})
	r.Add(Result{Kind: "tcp", Port: 2})
	r.Flush()
	if want := []string{"CLOSED tcp 1-2"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
