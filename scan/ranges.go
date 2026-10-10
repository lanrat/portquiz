package scan

import "slices"

// Range is a run of consecutive tested ports of one kind with the same result.
type Range struct {
	Kind        string
	First, Last int
	Open        bool
}

// port states tracked while building ranges
const (
	statePending int8 = iota
	stateOpen
	stateClosed
)

// Ranges merges results into ranges of consecutive ports with the same
// result, and emits each range as soon as it is final. Results arrive out of
// order, so it tracks the lowest port of each kind that is still pending:
// every port below it is final, and a range is emitted once the port after it
// is final and has a different result, or is not the next port number.
// Ranges is not safe for concurrent use.
type Ranges struct {
	ports    []int                    // sorted ports under test
	index    []int                    // port number -> position in ports, -1 if not tested
	trackers map[string]*rangeTracker // per kind
	emit     func(Range)
}

// NewRanges returns a Ranges for a scan of the given sorted, unique ports,
// such as Scan.Ports, that passes each finished range to emit.
func NewRanges(ports []int, emit func(Range)) *Ranges {
	r := &Ranges{
		ports:    ports,
		index:    make([]int, MaxPort+1),
		trackers: make(map[string]*rangeTracker),
		emit:     emit,
	}
	for i := range r.index {
		r.index[i] = -1
	}
	for i, p := range ports {
		r.index[p] = i
	}
	return r
}

// Add records a result, emitting any ranges that are now final.
func (r *Ranges) Add(res Result) {
	i := r.index[res.Port]
	if i < 0 {
		return
	}
	t, ok := r.trackers[res.Kind]
	if !ok {
		t = &rangeTracker{r: r, kind: res.Kind, state: make([]int8, len(r.ports))}
		r.trackers[res.Kind] = t
	}
	t.add(i, res.Open)
}

// Flush emits the ranges still being built, in order of kind. Call it when the
// scan finishes or is canceled; ranges only cover ports whose results, and
// those of every port below them, are known.
func (r *Ranges) Flush() {
	kinds := make([]string, 0, len(r.trackers))
	for k := range r.trackers {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for _, k := range kinds {
		r.trackers[k].flush()
	}
}

// rangeTracker builds the ranges of one kind.
type rangeTracker struct {
	r        *Ranges
	kind     string
	state    []int8 // result per position in r.ports
	next     int    // position of the lowest pending port
	runStart int    // position where the current unemitted range starts
}

// add records the result for the port at position i in r.ports.
func (t *rangeTracker) add(i int, open bool) {
	if open {
		t.state[i] = stateOpen
	} else {
		t.state[i] = stateClosed
	}
	ports := t.r.ports
	for t.next < len(t.state) && t.state[t.next] != statePending {
		if t.next > t.runStart &&
			(t.state[t.next] != t.state[t.runStart] || ports[t.next] != ports[t.next-1]+1) {
			t.emit(t.next - 1)
			t.runStart = t.next
		}
		t.next++
	}
}

// flush emits the current range, if any.
func (t *rangeTracker) flush() {
	if t.next > t.runStart {
		t.emit(t.next - 1)
		t.runStart = t.next
	}
}

// emit emits the range from runStart through position last.
func (t *rangeTracker) emit(last int) {
	t.r.emit(Range{
		Kind:  t.kind,
		First: t.r.ports[t.runStart],
		Last:  t.r.ports[last],
		Open:  t.state[t.runStart] == stateOpen,
	})
}
