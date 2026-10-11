// Package main provides a terminal progress display for the portquiz client.
// It renders a single, continuously updated status line on stderr.
package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"portquiz/scan"

	"golang.org/x/term"
)

// progressInterval is how often the progress line is redrawn.
const progressInterval = 200 * time.Millisecond

// progress tracks completed checks and renders a status line on stderr.
// It is not safe for concurrent use; jobResults owns it.
type progress struct {
	out     *os.File
	total   int       // Total number of checks to run
	done    int       // Number of checks completed
	open    int       // Number of checks that found an open port
	start   time.Time // When testing started
	lastLen int       // Length of the line currently on screen, 0 if none
	eta     *scan.ETA // Estimates the time left
}

// progressEnabled reports whether the progress line should be shown.
// It is only shown when stderr is an interactive terminal, so piped or
// redirected output stays clean, and never in verbose mode where log
// output would constantly overwrite it.
func progressEnabled() bool {
	return !*noProgress && !*verbose && term.IsTerminal(int(os.Stderr.Fd()))
}

// newProgress returns a progress display for the given number of checks.
func newProgress(total int) *progress {
	now := time.Now()
	return &progress{
		out:   os.Stderr,
		total: total,
		start: now,
		eta:   scan.NewETA(total, now),
	}
}

// record counts a completed check.
func (p *progress) record(open bool) {
	p.done++
	if open {
		p.open++
	}
}

// clear erases the progress line so other output can be printed in its place.
// It only uses carriage returns and spaces so it works on any terminal.
func (p *progress) clear() {
	if p.lastLen == 0 {
		return
	}
	_, _ = fmt.Fprintf(p.out, "\r%s\r", strings.Repeat(" ", p.lastLen))
	p.lastLen = 0
}

// draw renders the current progress line, overwriting the previous one.
func (p *progress) draw() {
	line := p.line(terminalWidth())
	pad := 0
	if p.lastLen > len(line) {
		pad = p.lastLen - len(line)
	}
	_, _ = fmt.Fprintf(p.out, "\r%s%s", line, strings.Repeat(" ", pad))
	p.lastLen = len(line)
}

// summary clears the progress line and prints the final totals.
func (p *progress) summary() {
	p.clear()
	elapsed := time.Since(p.start).Round(time.Second)
	_, _ = fmt.Fprintf(p.out, "Tested %d in %s: %d open, %d closed\n", p.done, elapsed, p.open, p.done-p.open)
}

// line builds the progress line, fitted to the given terminal width.
// The bar shrinks to fit narrow terminals and is dropped entirely when there
// is no room for it.
func (p *progress) line(width int) string {
	pct := 0
	if p.total > 0 {
		pct = p.done * 100 / p.total
	}
	// pad every field to its widest possible value so the bar doesn't jitter
	n := len(strconv.Itoa(p.total))
	stats := fmt.Sprintf(" %3d%% %*d/%d open:%-*d closed:%-*d %10s",
		pct, n, p.done, p.total, n, p.open, n, p.done-p.open, p.remaining())

	// leave the last column empty so the terminal never wraps the line
	avail := width - 1
	barWidth := avail - len(stats) - 2
	if barWidth < 10 {
		stats = strings.TrimPrefix(stats, " ")
		if len(stats) > avail {
			stats = stats[:avail]
		}
		return stats
	}

	filled := 0
	if p.total > 0 {
		filled = p.done * barWidth / p.total
	}
	bar := strings.Repeat("=", filled)
	if filled < barWidth {
		bar += ">" + strings.Repeat(" ", barWidth-filled-1)
	}
	return "[" + bar + "]" + stats
}

// remaining estimates the time left. It is smoothed and rounded so it
// counts down steadily rather than jumping with every burst of results.
// No estimate is given until there is enough data for it to be meaningful.
func (p *progress) remaining() string {
	left, ok := p.eta.Update(p.done, time.Now())
	if !ok {
		return "ETA --"
	}
	return "ETA " + formatETA(scan.RoundETA(left))
}

// formatETA formats a rounded estimate compactly, leaving out zero units:
// 45s, 3m20s, 12m, 1h5m.
func formatETA(d time.Duration) string {
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0 && s > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// testingLine describes the server addresses being tested, naming host
// unless it is already the only address: "Testing example.com at
// 192.0.2.1" or "Testing example.com at 192.0.2.1 and 2001:db8::1".
func testingLine(host string, addrs []*net.IPAddr) string {
	ips := make([]string, len(addrs))
	for i, a := range addrs {
		ips[i] = a.String()
	}
	list := strings.Join(ips, " and ")
	if len(ips) == 1 && strings.Trim(strings.TrimSpace(host), "[]") == list {
		return "Testing " + list
	}
	return "Testing " + strings.TrimSpace(host) + " at " + list
}

// terminalWidth returns the width of the terminal on stderr, or 80 if unknown.
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}
