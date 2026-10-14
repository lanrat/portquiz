package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"portquiz/scan"
)

// runDiff compares the JSON reports at args[0] and args[1] for -diff and
// returns the exit status: 0 if no port's result changed, 1 if some did, 2
// on errors.
func runDiff(args []string) int {
	if len(args) != 2 {
		log.Print("-diff needs two JSON reports: portquiz -diff a.json b.json")
		return 2
	}
	var reports [2]*scan.Report
	for i, path := range args {
		data, err := os.ReadFile(path)
		if err == nil {
			reports[i], err = scan.ParseReport(data)
		}
		if err != nil {
			log.Printf("%s: %s", path, err)
			return 2
		}
	}
	d := scan.DiffReports(reports[0], reports[1])
	if *jsonPath != "" {
		if err := writeJSON(*jsonPath, d); err != nil {
			log.Print(err)
			return 2
		}
	}
	if *jsonPath != "-" {
		for _, line := range formatDiff(d, time.Local) {
			fmt.Println(line)
		}
	}
	if d.Changed() > 0 {
		return 1
	}
	return 0
}

// formatDiff describes a diff as lines of text: the two runs, then per
// kind the counts and the changed ports, with times in loc.
func formatDiff(d *scan.Diff, loc *time.Location) []string {
	lines := []string{"A: " + describeRun(d.A, loc), "B: " + describeRun(d.B, loc)}
	for _, k := range d.Kinds {
		switch {
		case k.BKind == "":
			lines = append(lines, k.Kind+": only in A")
			continue
		case k.AKind == "":
			lines = append(lines, k.Kind+": only in B")
			continue
		}
		counts := fmt.Sprintf("%s: %d opened, %d closed, %d changed reason, %d unchanged", k.Kind, k.Opened, k.Closed, k.ReasonChanged, k.Unchanged)
		if n := k.OnlyA + k.OnlyB; n > 0 {
			counts += fmt.Sprintf(", %d not compared", n)
		}
		lines = append(lines, counts)
		for _, c := range k.Changes {
			ports := fmt.Sprint(c.First)
			if c.Last != c.First {
				ports = fmt.Sprintf("%d-%d", c.First, c.Last)
			}
			lines = append(lines, fmt.Sprintf("  %s %s -> %s", ports, c.A, c.B))
		}
	}
	return lines
}

// describeRun describes a run on one line, such as
// `example.com, 2026-10-13 09:30, android, "home wifi"`.
func describeRun(r scan.RunInfo, loc *time.Location) string {
	parts := []string{r.Host, r.Started.In(loc).Format("2006-01-02 15:04")}
	if r.Client != "" {
		client := r.Client
		if r.TCPRequests != "" {
			client += " over " + strings.ToUpper(r.TCPRequests)
		}
		if r.ClientAddress != "" {
			client += " from " + r.ClientAddress
		}
		parts = append(parts, client)
	}
	if r.Label != "" {
		parts = append(parts, fmt.Sprintf("%q", r.Label))
	}
	if r.Canceled {
		parts = append(parts, "stopped early")
	}
	return strings.Join(parts, ", ")
}
