package scan

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// AllPorts returns every port, 1 through MaxPort.
func AllPorts() []int {
	ports := make([]int, 0, MaxPort)
	for p := 1; p <= MaxPort; p++ {
		ports = append(ports, p)
	}
	return ports
}

// ParsePorts parses a comma separated list of ports and inclusive ranges,
// such as "22,80,1000-2000", into sorted ports without duplicates. An empty
// list means every port.
func ParsePorts(list string) ([]int, error) {
	if strings.TrimSpace(list) == "" {
		return AllPorts(), nil
	}
	var ports []int
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		first, last, isRange := strings.Cut(item, "-")
		start, err := parsePort(first)
		if err != nil {
			return nil, fmt.Errorf("invalid port entry %q: %w", item, err)
		}
		end := start
		if isRange {
			end, err = parsePort(last)
			if err != nil {
				return nil, fmt.Errorf("invalid port entry %q: %w", item, err)
			}
			if start > end {
				return nil, fmt.Errorf("invalid port entry %q: start of range is greater than end", item)
			}
		}
		for p := start; p <= end; p++ {
			ports = append(ports, p)
		}
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("no ports in %q", list)
	}
	slices.Sort(ports)
	return slices.Compact(ports), nil
}

// parsePort parses a single port number and checks that it is in range.
func parsePort(s string) (int, error) {
	s = strings.TrimSpace(s)
	p, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a port number", s)
	}
	if p < 1 || p > MaxPort {
		return 0, fmt.Errorf("port %d is not between 1 and %d", p, MaxPort)
	}
	return p, nil
}

// formatPorts formats sorted ports as a list of ports and ranges, the
// reverse of ParsePorts: "1-1024,8080".
func formatPorts(ports []int) string {
	var b strings.Builder
	for i := 0; i < len(ports); {
		j := i
		for j+1 < len(ports) && ports[j+1] == ports[j]+1 {
			j++
		}
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(ports[i]))
		if j > i {
			b.WriteByte('-')
			b.WriteString(strconv.Itoa(ports[j]))
		}
		i = j + 1
	}
	return b.String()
}
