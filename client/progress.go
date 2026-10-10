// Package main provides a terminal progress display for the portquiz client.
// It renders a single, continuously updated status line on stderr.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
	return &progress{
		out:   os.Stderr,
		total: total,
		start: time.Now(),
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
	stats := fmt.Sprintf(" %3d%% %*d/%d open:%-*d closed:%-*d %-10s",
		pct, n, p.done, p.total, n, p.open, n, p.done-p.open, p.eta())

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

// eta estimates the time remaining from the average rate so far.
// No estimate is given until there is enough data for it to be meaningful.
func (p *progress) eta() string {
	elapsed := time.Since(p.start)
	if p.done == 0 || elapsed < 2*time.Second {
		return "ETA --"
	}
	if p.done >= p.total {
		return "ETA 0s"
	}
	rate := float64(p.done) / elapsed.Seconds()
	remaining := time.Duration(float64(p.total-p.done) / rate * float64(time.Second))
	return "ETA " + remaining.Round(time.Second).String()
}

// terminalWidth returns the width of the terminal on stderr, or 80 if unknown.
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}
