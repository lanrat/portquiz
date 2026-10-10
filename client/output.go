// Package main provides result formatting for the portquiz client.
// Results can be printed one line per port, or merged into ranges of
// consecutive ports with the same result as soon as those ranges are final.
package main

import (
	"fmt"
	"slices"
)

// Output formats accepted by -format.
const (
	formatRanges = "ranges"
	formatPorts  = "ports"
)

// port states tracked while building ranges
const (
	statePending int8 = iota
	stateOpen
	stateClosed
)

// resultWriter prints job results in the format selected by -format.
// It is not safe for concurrent use; jobResults owns it.
type resultWriter struct {
	ranges   bool
	ports    []int                    // sorted ports under test
	index    []int                    // port number -> position in ports, -1 if not tested
	trackers map[string]*rangeTracker // per kind, e.g. "tcp" or "udp6"
	print    func(string)             // prints a line of output
}

// newResultWriter returns a resultWriter for the given sorted, unique ports.
// Each line of output is passed to print.
func newResultWriter(ports []int, print func(string)) *resultWriter {
	w := &resultWriter{
		ranges:   *format == formatRanges,
		ports:    ports,
		trackers: make(map[string]*rangeTracker),
		print:    print,
	}
	if w.ranges {
		w.index = make([]int, maxPort+1)
		for i := range w.index {
			w.index[i] = -1
		}
		for i, p := range ports {
			w.index[p] = i
		}
	}
	return w
}

// add records a finished job, printing any output that is now final.
func (w *resultWriter) add(j *job) {
	if !w.ranges {
		w.line(j.open, j.kind, j.port, j.port)
		return
	}
	t, ok := w.trackers[j.kind]
	if !ok {
		t = &rangeTracker{w: w, kind: j.kind, state: make([]int8, len(w.ports))}
		w.trackers[j.kind] = t
	}
	t.add(w.index[j.port], j.open)
}

// flush prints any ranges that are still being built. It is called when
// testing finishes or is interrupted; ranges only cover ports whose results,
// and those of every port below them, are known.
func (w *resultWriter) flush() {
	kinds := make([]string, 0, len(w.trackers))
	for k := range w.trackers {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for _, k := range kinds {
		w.trackers[k].flush()
	}
}

// line prints one result line for ports first through last, if -open/-closed allow it.
func (w *resultWriter) line(isOpen bool, kind string, first, last int) {
	if (isOpen && !*open) || (!isOpen && !*closed) {
		return
	}
	state := "CLOSED"
	if isOpen {
		state = "OPEN"
	}
	if first == last {
		w.print(fmt.Sprintf("%s %s %d", state, kind, first))
	} else {
		w.print(fmt.Sprintf("%s %s %d-%d", state, kind, first, last))
	}
}

// rangeTracker merges results for one kind into ranges of consecutive ports
// with the same result. Jobs finish out of order, so it tracks the lowest
// port that is still pending: every port below it is final, and a range is
// printed once the port after it is final and has a different result, or is
// not the next port number.
type rangeTracker struct {
	w        *resultWriter
	kind     string
	state    []int8 // result per position in w.ports
	next     int    // position of the lowest pending port
	runStart int    // position where the current unprinted range starts
}

// add records the result for the port at position i in w.ports.
func (t *rangeTracker) add(i int, isOpen bool) {
	if isOpen {
		t.state[i] = stateOpen
	} else {
		t.state[i] = stateClosed
	}
	ports := t.w.ports
	for t.next < len(t.state) && t.state[t.next] != statePending {
		if t.next > t.runStart &&
			(t.state[t.next] != t.state[t.runStart] || ports[t.next] != ports[t.next-1]+1) {
			t.emit(t.next - 1)
			t.runStart = t.next
		}
		t.next++
	}
}

// flush prints the current range, if any.
func (t *rangeTracker) flush() {
	if t.next > t.runStart {
		t.emit(t.next - 1)
		t.runStart = t.next
	}
}

// emit prints the range from runStart through position last.
func (t *rangeTracker) emit(last int) {
	t.w.line(t.state[t.runStart] == stateOpen, t.kind, t.w.ports[t.runStart], t.w.ports[last])
}
