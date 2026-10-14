package scan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// checkUDP tests a single UDP port once: it sends a probe with the password and waits
// for the server to send it back. Each check uses its own connected socket so
// that an ICMP port unreachable from a rejecting firewall is reported at once
// as refused; an unconnected socket shared by all checks never sees those, so
// every rejected port would wait out the full timeout instead.
func (t *target) checkUDP(ctx context.Context, port int, timeout time.Duration) (outcome, time.Duration) {
	network, addr := t.network, t.addr
	conn, err := dialUDP(network, &net.UDPAddr{IP: addr.IP, Port: port, Zone: addr.Zone})
	if err != nil {
		t.opts.logf("%s %d dial error: %s", network, port, err)
		return outcomeError, 0
	}
	defer func() { _ = conn.Close() }()
	// unblock a pending read as soon as the context is canceled
	stopAfter := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stopAfter()

	start := time.Now()
	if err := conn.SetDeadline(start.Add(timeout)); err != nil {
		t.opts.logf("%s %d SetDeadline warning: %s", network, port, err)
	}
	probe := newProbe(t.magic)
	if _, err := conn.Write(probe); err != nil {
		t.opts.logf("%s %d write error: %s", network, port, err)
		if refusal(err) {
			return outcomeRefused, 0
		}
		return outcomeError, 0
	}
	buffer := make([]byte, 2048)
	n, err := conn.Read(buffer)
	switch {
	case err != nil && ctx.Err() != nil:
		return outcomeError, 0
	case refusal(err):
		return outcomeRefused, 0
	case errors.Is(err, os.ErrDeadlineExceeded):
		return outcomeTimeout, 0
	case err != nil:
		t.opts.logf("%s %d read error: %s", network, port, err)
		return outcomeError, 0
	case !isAnswer(buffer[:n], probe, t.magic):
		t.opts.logf("%s %d unexpected reply: %q", network, port, buffer[:n])
		return outcomeInterfered, 0
	}
	return outcomeOpen, time.Since(start)
}

// dialUDP connects a UDP socket to addr. When testing a server on the same
// host, the OS can pick a local port equal to the port being tested, which
// connects the socket to itself so it reads back its own packet and reports a
// false open. Like Go's TCP dialer, it detects this and tries again.
func dialUDP(network string, addr *net.UDPAddr) (*net.UDPConn, error) {
	for range 3 {
		conn, err := net.DialUDP(network, nil, addr)
		if err != nil {
			return nil, err
		}
		if conn.LocalAddr().String() != conn.RemoteAddr().String() {
			return conn, nil
		}
		_ = conn.Close()
	}
	return nil, fmt.Errorf("UDP socket connected to itself on %s", addr)
}
