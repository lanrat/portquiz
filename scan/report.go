package scan

import (
	"math"
	"slices"
	"time"
)

// Report summarizes a scan, for saving as JSON: each kind's totals, how
// quickly its open ports answered, and its results as ranges of consecutive
// ports with the same status.
type Report struct {
	Version string `json:"version,omitempty"`
	// Client is the client that ran the scan: "cli", "android" or
	// "browser".
	Client string `json:"client,omitempty"`
	// ClientAddress is the client's address as the server saw it, when the
	// client knows it; only the browser page does.
	ClientAddress string `json:"client_address,omitempty"`
	// Label is the user's name for the run, such as "home wifi".
	Label string `json:"label,omitempty"`
	// TCPRequests is how the browser page tested TCP ports: "http" or
	// "https" requests.
	TCPRequests string        `json:"tcp_requests,omitempty"`
	Host        string        `json:"host"`
	Started     time.Time     `json:"started"`
	Finished    time.Time     `json:"finished"`
	Seconds     float64       `json:"duration_seconds"`
	PerSecond   float64       `json:"tested_per_second"`
	Canceled    bool          `json:"canceled"`
	Options     ReportOptions `json:"options"`
	Kinds       []KindReport  `json:"kinds"`
}

// ReportOptions are the options a scan ran with.
type ReportOptions struct {
	Ports         string `json:"ports"`
	TCP           bool   `json:"tcp"`
	UDP           bool   `json:"udp"`
	IPv4          bool   `json:"ipv4"`
	IPv6          bool   `json:"ipv6"`
	TimeoutMillis int64  `json:"timeout_ms"`
	Adaptive      bool   `json:"adaptive"`
	Retries       int    `json:"retries"`
	Parallel      int    `json:"parallel"`
	Rate          int    `json:"rate"`
	Multi         int    `json:"multi"`
}

// KindReport summarizes the results of one kind, such as "tcp" or "udp6".
type KindReport struct {
	Kind    string `json:"kind"`
	Address string `json:"address"`
	// Tested counts the ports with a result; Untested those left when the
	// scan was canceled.
	Tested   int `json:"tested"`
	Untested int `json:"untested"`
	// Open, Rejected, Dropped, Interfered and Errors count the ports with
	// each Status.
	Open       int `json:"open"`
	Rejected   int `json:"rejected"`
	Dropped    int `json:"dropped"`
	Interfered int `json:"interfered"`
	Errors     int `json:"errors"`
	// Late counts the open ports that only answered when rechecked with the
	// full timeout; when it is not 0, dropped ports may be open too.
	Late int `json:"late"`
	// Retries is how many times a port that did not answer was retried.
	Retries int `json:"retries"`
	// TimeoutMillis is the timeout in use when the scan ended, which the
	// adaptive timeout lowers from the full timeout.
	TimeoutMillis float64 `json:"timeout_ms"`
	// OpenRTT is how quickly open ports answered, if any did.
	OpenRTT *RTTStats `json:"open_rtt_ms,omitempty"`
	// Ranges lists the tested ports as ranges of consecutive ports with the
	// same status.
	Ranges []StatusRange `json:"ranges"`
}

// RTTStats summarizes response times, in milliseconds.
type RTTStats struct {
	Min    float64 `json:"min"`
	Median float64 `json:"median"`
	P95    float64 `json:"p95"`
	Max    float64 `json:"max"`
}

// StatusRange is a run of consecutive ports with the same status.
type StatusRange struct {
	First  int    `json:"first"`
	Last   int    `json:"last"`
	Status Status `json:"status"`
}

// statuses lists every Status; a port's position in it plus one is how
// Recorder stores it, with 0 for a port without a result.
var statuses = []Status{StatusOpen, StatusRejected, StatusDropped, StatusInterfered, StatusError}

// Recorder collects a scan's results into a Report. It is not safe for
// concurrent use.
type Recorder struct {
	scan    *Scan
	started time.Time
	index   []int // port number -> position in scan.ports, -1 if not tested
	kinds   map[string]*kindRecord
}

// kindRecord holds the results of one kind.
type kindRecord struct {
	status []uint8 // per position in scan.ports: 0 without a result, else 1 + index in statuses
	rtts   []time.Duration
	late   int
}

// NewRecorder returns a Recorder for scan s, which started at started.
func NewRecorder(s *Scan, started time.Time) *Recorder {
	r := &Recorder{
		scan:    s,
		started: started,
		index:   make([]int, MaxPort+1),
		kinds:   make(map[string]*kindRecord),
	}
	for i := range r.index {
		r.index[i] = -1
	}
	for i, p := range s.ports {
		r.index[p] = i
	}
	for _, k := range s.kinds {
		r.kinds[k] = &kindRecord{status: make([]uint8, len(s.ports))}
	}
	return r
}

// Add records a result.
func (r *Recorder) Add(res Result) {
	k, ok := r.kinds[res.Kind]
	if !ok || res.Port < 0 || res.Port > MaxPort || r.index[res.Port] < 0 {
		return
	}
	k.status[r.index[res.Port]] = uint8(slices.Index(statuses, res.Status) + 1)
	if res.Status == StatusOpen {
		k.rtts = append(k.rtts, res.RTT)
		if res.Late {
			k.late++
		}
	}
}

// Report returns the report of the results recorded so far, for a scan that
// finished at finished, or was canceled then.
func (r *Recorder) Report(finished time.Time, canceled bool) *Report {
	s := r.scan
	rep := &Report{
		Host:     s.opts.Host,
		Started:  r.started.Round(time.Millisecond),
		Finished: finished.Round(time.Millisecond),
		Canceled: canceled,
		Options: ReportOptions{
			Ports:         formatPorts(s.ports),
			TCP:           s.opts.TCP,
			UDP:           s.opts.UDP,
			IPv4:          s.opts.IPv4,
			IPv6:          s.opts.IPv6,
			TimeoutMillis: s.opts.Timeout.Milliseconds(),
			Adaptive:      s.opts.Adaptive,
			Retries:       s.opts.Retries,
			Parallel:      s.opts.Parallel,
			Rate:          s.opts.Rate,
			Multi:         s.opts.Multi,
		},
		Kinds: []KindReport{},
	}
	tested := 0
	for _, kind := range s.kinds {
		kr := r.kindReport(kind)
		tested += kr.Tested
		rep.Kinds = append(rep.Kinds, kr)
	}
	elapsed := finished.Sub(r.started)
	rep.Seconds = math.Round(elapsed.Seconds()*1000) / 1000
	if elapsed > 0 {
		rep.PerSecond = math.Round(float64(tested)/elapsed.Seconds()*10) / 10
	}
	return rep
}

// kindReport summarizes the results of one kind.
func (r *Recorder) kindReport(kind string) KindReport {
	t := r.scan.targets[kind]
	k := r.kinds[kind]
	kr := KindReport{
		Kind:          kind,
		Address:       t.addr.String(),
		Late:          k.late,
		Retries:       t.retries(),
		TimeoutMillis: millis(t.est.timeout()),
		OpenRTT:       rttStats(k.rtts),
		Ranges:        []StatusRange{},
	}
	counts := map[Status]*int{
		StatusOpen:       &kr.Open,
		StatusRejected:   &kr.Rejected,
		StatusDropped:    &kr.Dropped,
		StatusInterfered: &kr.Interfered,
		StatusError:      &kr.Errors,
	}
	var cur *StatusRange
	for i, code := range k.status {
		if code == 0 {
			kr.Untested++
			cur = nil
			continue
		}
		kr.Tested++
		status := statuses[code-1]
		*counts[status]++
		port := r.scan.ports[i]
		if cur != nil && cur.Status == status && cur.Last+1 == port {
			cur.Last = port
			continue
		}
		kr.Ranges = append(kr.Ranges, StatusRange{First: port, Last: port, Status: status})
		cur = &kr.Ranges[len(kr.Ranges)-1]
	}
	return kr
}

// rttStats summarizes response times, or returns nil if there are none.
func rttStats(rtts []time.Duration) *RTTStats {
	if len(rtts) == 0 {
		return nil
	}
	sorted := slices.Sorted(slices.Values(rtts))
	// the nearest-rank percentile
	pct := func(p float64) time.Duration {
		return sorted[max(int(math.Ceil(p*float64(len(sorted))))-1, 0)]
	}
	return &RTTStats{
		Min:    millis(sorted[0]),
		Median: millis(pct(0.5)),
		P95:    millis(pct(0.95)),
		Max:    millis(sorted[len(sorted)-1]),
	}
}

// millis returns d in milliseconds, to a tenth of a millisecond.
func millis(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*10) / 10
}
