package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"portquiz/scan"
)

func TestFormatDiff(t *testing.T) {
	var reports [2]*scan.Report
	for i, name := range []string{"a.json", "b.json"} {
		data, err := os.ReadFile(filepath.Join("..", "scan", "testdata", "diff", name))
		if err != nil {
			t.Fatal(err)
		}
		if reports[i], err = scan.ParseReport(data); err != nil {
			t.Fatal(err)
		}
	}
	got := formatDiff(scan.DiffReports(reports[0], reports[1]), time.UTC)
	want := []string{
		`A: portquiz.example.com, 2026-10-13 09:30, browser over HTTP from 198.51.100.7, "home wifi"`,
		`B: portquiz.example.com, 2026-10-14 10:00, cli, stopped early`,
		`tcp4: 5 opened, 10 closed, 6 changed reason, 18 unchanged, 11 not compared`,
		`  6-10 open -> dropped`,
		`  11-12 rejected -> dropped`,
		`  22-25 dropped -> rejected`,
		`  26-30 dropped -> open`,
		`  36-40 open -> interfered`,
		`tcp6: only in B`,
		`udp4: 0 opened, 3 closed, 0 changed reason, 9 unchanged`,
		`  6-8 open -> rejected`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRunDiffStatus(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join("..", "scan", "testdata", "diff", "a.json")
	b := filepath.Join("..", "scan", "testdata", "diff", "b.json")
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"host": "x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "diff.json")
	*jsonPath = out
	defer func() { *jsonPath = "" }()
	for _, c := range []struct {
		args []string
		want int
	}{
		{[]string{a, b}, 1},
		{[]string{a, a}, 0},
		{[]string{a}, 2},
		{[]string{a, bad}, 2},
		{[]string{a, filepath.Join(dir, "missing.json")}, 2},
	} {
		if got := runDiff(c.args); got != c.want {
			t.Errorf("%v: status %d, want %d", c.args, got, c.want)
		}
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var d scan.Diff
	if err := json.Unmarshal(data, &d); err != nil || d.A.Label != "home wifi" || len(d.Kinds) != 2 || d.Kinds[1].Unchanged != 12 || d.Kinds[0].Unchanged != 39 {
		t.Errorf("diff JSON of the last comparison: %s", data)
	}
}
