// Package main provides result formatting for the portquiz client.
// Results can be printed one line per port, or merged into ranges of
// consecutive ports with the same result as soon as those ranges are final.
package main

import (
	"fmt"

	"portquiz/scan"
)

// Output formats accepted by -format.
const (
	formatRanges = "ranges"
	formatPorts  = "ports"
)

// printer prints scan results in the format selected by -format.
// It is not safe for concurrent use; printResults owns it.
type printer struct {
	ranges *scan.Ranges // merges results into ranges, nil for -format ports
	print  func(string) // prints a line of output
}

// newPrinter returns a printer for a scan of the given sorted, unique ports.
// Each line of output is passed to print.
func newPrinter(ports []int, print func(string)) *printer {
	w := &printer{print: print}
	if *format == formatRanges {
		w.ranges = scan.NewRanges(ports, func(r scan.Range) {
			w.line(r.Open, r.Kind, r.First, r.Last)
		})
	}
	return w
}

// add records a result, printing any output that is now final.
func (w *printer) add(r scan.Result) {
	if w.ranges == nil {
		w.line(r.Open, r.Kind, r.Port, r.Port)
		return
	}
	w.ranges.Add(r)
}

// flush prints any ranges that are still being built. It is called when
// testing finishes or is interrupted.
func (w *printer) flush() {
	if w.ranges != nil {
		w.ranges.Flush()
	}
}

// line prints one result line for ports first through last, if -open/-closed allow it.
func (w *printer) line(isOpen bool, kind string, first, last int) {
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
