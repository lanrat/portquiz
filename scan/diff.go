package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

const (
	// statusBrowser is the status the browser page gives ports the browser
	// refused to connect to: they have no result.
	statusBrowser Status = "browser"
	// statusClosed is the status of a closed port when how it was closed is
	// not known, as for UDP ports tested by the browser page: a browser
	// cannot tell a rejected UDP port from a dropped one.
	statusClosed Status = "closed"
)

// Diff compares two scans' reports, A and B: per kind, the ports whose
// status changed from A to B.
type Diff struct {
	A     RunInfo    `json:"a"`
	B     RunInfo    `json:"b"`
	Kinds []KindDiff `json:"kinds"`
}

// RunInfo identifies a scan in a Diff.
type RunInfo struct {
	Host          string    `json:"host"`
	Label         string    `json:"label,omitempty"`
	Client        string    `json:"client,omitempty"`
	ClientAddress string    `json:"client_address,omitempty"`
	TCPRequests   string    `json:"tcp_requests,omitempty"`
	Version       string    `json:"version,omitempty"`
	Started       time.Time `json:"started"`
	Finished      time.Time `json:"finished"`
	Canceled      bool      `json:"canceled"`
}

// KindDiff compares the results of one kind, matched between the runs by
// protocol and IP version: "tcp" over IPv4 in one run is compared with
// "tcp4" in the other.
type KindDiff struct {
	// Kind is the protocol and IP version, such as "tcp4", or just the
	// protocol when the version is not known.
	Kind string `json:"kind"`
	// AKind and BKind are the kind's names in each run, empty when the kind
	// is only in the other run.
	AKind    string `json:"a_kind"`
	BKind    string `json:"b_kind"`
	AAddress string `json:"a_address"`
	BAddress string `json:"b_address"`
	// Compared counts the ports with a result in both runs, and Unchanged,
	// Opened, Closed and ReasonChanged split them: closed in A and open in
	// B, open in A and closed in B, and closed in both but differently,
	// such as rejected in A and dropped in B.
	Compared      int `json:"compared"`
	Unchanged     int `json:"unchanged"`
	Opened        int `json:"opened"`
	Closed        int `json:"closed"`
	ReasonChanged int `json:"reason_changed"`
	// OnlyA and OnlyB count the ports with a result in just one run, which
	// are not compared.
	OnlyA int       `json:"only_a"`
	OnlyB int       `json:"only_b"`
	AOpen int       `json:"a_open"`
	BOpen int       `json:"b_open"`
	ARTT  *RTTStats `json:"a_open_rtt_ms,omitempty"`
	BRTT  *RTTStats `json:"b_open_rtt_ms,omitempty"`
	// Changes lists the ports whose status changed, as ranges of
	// consecutive ports with the same change.
	Changes []StatusChange `json:"changes"`
	// Ranges covers every port with a result in either run, as ranges of
	// consecutive ports with the same Change, for drawing a map.
	Ranges []ChangeRange `json:"ranges"`
}

// Change is how a port's result compares between two runs.
type Change string

// The Changes: the port was tested in both runs and its status is the
// same, or it was closed in A and open in B, open in A and closed in B, or
// closed in both but differently; or it was only tested in one run.
const (
	ChangeUnchanged     Change = "unchanged"
	ChangeOpened        Change = "opened"
	ChangeClosed        Change = "closed"
	ChangeReasonChanged Change = "reason_changed"
	ChangeOnlyA         Change = "only_a"
	ChangeOnlyB         Change = "only_b"
)

// ChangeRange is a run of consecutive ports with the same Change.
type ChangeRange struct {
	First  int    `json:"first"`
	Last   int    `json:"last"`
	Change Change `json:"change"`
}

// Changed counts the ports whose status changed.
func (k KindDiff) Changed() int {
	return k.Opened + k.Closed + k.ReasonChanged
}

// StatusChange is a run of consecutive ports whose status changed from A
// to B.
type StatusChange struct {
	First int    `json:"first"`
	Last  int    `json:"last"`
	A     Status `json:"a"`
	B     Status `json:"b"`
}

// Changed counts the ports whose status changed, across kinds.
func (d *Diff) Changed() int {
	n := 0
	for _, k := range d.Kinds {
		n += k.Changed()
	}
	return n
}

// ParseReport parses a report as JSON, as written by any portquiz client,
// and checks its ranges: in order, without overlaps, and of valid ports.
func ParseReport(data []byte) (*Report, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("not a portquiz report: %w", err)
	}
	if _, ok := raw["a"]; ok && raw["b"] != nil {
		return nil, errors.New("this is a comparison of two reports, not a report")
	}
	if _, ok := raw["kinds"]; !ok {
		return nil, errors.New("not a portquiz report: it has no kinds")
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("not a portquiz report: %w", err)
	}
	for _, k := range r.Kinds {
		prev := 0
		for _, x := range k.Ranges {
			if x.First <= prev || x.Last > MaxPort || x.First > x.Last {
				return nil, fmt.Errorf("not a portquiz report: bad range %d-%d in kind %q", x.First, x.Last, k.Kind)
			}
			prev = x.Last
		}
	}
	return &r, nil
}

// DiffReports compares report a with report b, whose ranges must be in
// order and not overlap, as ParseReport checks.
func DiffReports(a, b *Report) *Diff {
	d := &Diff{A: runInfo(a), B: runInfo(b), Kinds: []KindDiff{}}
	keys := map[string]bool{}
	for _, k := range a.Kinds {
		keys[kindKey(k)] = true
	}
	for _, k := range b.Kinds {
		keys[kindKey(k)] = true
	}
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		d.Kinds = append(d.Kinds, diffKind(key, tested(a, key), tested(b, key)))
	}
	return d
}

func runInfo(r *Report) RunInfo {
	return RunInfo{
		Host:          r.Host,
		Label:         r.Label,
		Client:        r.Client,
		ClientAddress: r.ClientAddress,
		TCPRequests:   r.TCPRequests,
		Version:       r.Version,
		Started:       r.Started,
		Finished:      r.Finished,
		Canceled:      r.Canceled,
	}
}

// kindKey is the protocol and IP version of a kind: from the kind's name,
// such as "tcp6", or for a kind named just "tcp" from its address.
func kindKey(k KindReport) string {
	protocol, version := k.Kind, ""
	if len(k.Kind) > 3 {
		protocol, version = k.Kind[:3], k.Kind[3:]
	} else if k.Address != "" {
		version = "4"
		if strings.Contains(k.Address, ":") {
			version = "6"
		}
	}
	return protocol + version
}

// tested returns the kind of r matching key, or nil, with only the ranges
// of ports with a result, and statusClosed for closed ports when the client
// cannot tell how they were closed.
func tested(r *Report, key string) *KindReport {
	for _, k := range r.Kinds {
		if kindKey(k) != key {
			continue
		}
		unknown := r.Client == "browser" && strings.HasPrefix(k.Kind, "udp")
		ranges := make([]StatusRange, 0, len(k.Ranges))
		for _, x := range k.Ranges {
			switch {
			case x.Status == statusBrowser:
				continue
			case unknown && x.Status != StatusOpen:
				x.Status = statusClosed
			}
			ranges = append(ranges, x)
		}
		k.Ranges = ranges
		return &k
	}
	return nil
}

// diffKind compares kind a with kind b, as returned by tested: either may be
// missing.
func diffKind(key string, a, b *KindReport) KindDiff {
	d := KindDiff{Kind: key, Changes: []StatusChange{}, Ranges: []ChangeRange{}}
	if a != nil {
		d.AKind, d.AAddress, d.ARTT = a.Kind, a.Address, a.OpenRTT
	}
	if b != nil {
		d.BKind, d.BAddress, d.BRTT = b.Kind, b.Address, b.OpenRTT
	}
	var ar, br []StatusRange
	if a != nil {
		ar = a.Ranges
	}
	if b != nil {
		br = b.Ranges
	}
	d.AOpen, d.BOpen = openPorts(ar), openPorts(br)
	counts := map[Change]*int{
		ChangeUnchanged:     &d.Unchanged,
		ChangeOpened:        &d.Opened,
		ChangeClosed:        &d.Closed,
		ChangeReasonChanged: &d.ReasonChanged,
		ChangeOnlyA:         &d.OnlyA,
		ChangeOnlyB:         &d.OnlyB,
	}
	// walk the ports with a result in either run, a stretch at a time up to
	// where a range of either starts or ends
	i, j := 0, 0
	for port := 1; ; {
		for i < len(ar) && ar[i].Last < port {
			i++
		}
		for j < len(br) && br[j].Last < port {
			j++
		}
		if i == len(ar) && j == len(br) {
			break
		}
		// skip to the next port with a result
		next := MaxPort
		if i < len(ar) {
			next = min(next, max(port, ar[i].First))
		}
		if j < len(br) {
			next = min(next, max(port, br[j].First))
		}
		port = next
		var sa, sb Status
		last := MaxPort
		if i < len(ar) {
			if ar[i].First <= port {
				sa, last = ar[i].Status, min(last, ar[i].Last)
			} else {
				last = min(last, ar[i].First-1)
			}
		}
		if j < len(br) {
			if br[j].First <= port {
				sb, last = br[j].Status, min(last, br[j].Last)
			} else {
				last = min(last, br[j].First-1)
			}
		}
		change := compare(sa, sb)
		*counts[change] += last - port + 1
		if r := &d.Ranges; len(*r) > 0 && (*r)[len(*r)-1].Last+1 == port && (*r)[len(*r)-1].Change == change {
			(*r)[len(*r)-1].Last = last
		} else {
			*r = append(*r, ChangeRange{First: port, Last: last, Change: change})
		}
		if change == ChangeOpened || change == ChangeClosed || change == ChangeReasonChanged {
			if c := &d.Changes; len(*c) > 0 && (*c)[len(*c)-1].Last+1 == port &&
				(*c)[len(*c)-1].A == sa && (*c)[len(*c)-1].B == sb {
				(*c)[len(*c)-1].Last = last
			} else {
				*c = append(*c, StatusChange{First: port, Last: last, A: sa, B: sb})
			}
		}
		port = last + 1
	}
	d.Compared = d.Unchanged + d.Opened + d.Closed + d.ReasonChanged
	return d
}

// openPorts counts the open ports in ranges.
func openPorts(ranges []StatusRange) int {
	n := 0
	for _, r := range ranges {
		if r.Status == StatusOpen {
			n += r.Last - r.First + 1
		}
	}
	return n
}

// compare returns the Change from status a to status b, either of which is
// empty for a port without a result in that run.
func compare(a, b Status) Change {
	switch {
	case b == "":
		return ChangeOnlyA
	case a == "":
		return ChangeOnlyB
	case a == b:
		return ChangeUnchanged
	case b == StatusOpen:
		return ChangeOpened
	case a == StatusOpen:
		return ChangeClosed
	case a == statusClosed || b == statusClosed:
		// closed in both, and how is only known for one
		return ChangeUnchanged
	default:
		return ChangeReasonChanged
	}
}
