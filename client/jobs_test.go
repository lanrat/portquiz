package main

import (
	"slices"
	"testing"
)

func TestParsePorts(t *testing.T) {
	defer func() { *port = "" }()
	tests := []struct {
		in   string
		want []int
	}{
		{"22", []int{22}},
		{"443,22,80,22,,443", []int{22, 80, 443}},
		{"1-5", []int{1, 2, 3, 4, 5}},
		{"8080,1-3,2-4", []int{1, 2, 3, 4, 8080}},
		{"5-5", []int{5}},
		{" 22 , 80 - 82 ", []int{22, 80, 81, 82}},
		{"65534-65535", []int{65534, 65535}},
	}
	for _, tt := range tests {
		*port = tt.in
		got, err := parsePorts()
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tt.in, err)
			continue
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%q: got %v, want %v", tt.in, got, tt.want)
		}
	}

	*port = ""
	all, err := parsePorts()
	if err != nil || len(all) != maxPort || all[0] != 1 || all[len(all)-1] != maxPort {
		t.Errorf("empty -port: got %d ports (err %v), want 1-%d", len(all), err, maxPort)
	}
}

func TestParsePortsInvalid(t *testing.T) {
	defer func() { *port = "" }()
	for _, in := range []string{
		"0", "65536", "abc", "-5", "5-", "10-5", "1-2-3", "1-65536", "0-10", "1-x",
	} {
		*port = in
		if got, err := parsePorts(); err == nil {
			t.Errorf("%q: expected an error, got %v", in, got)
		}
	}
}
