package scan

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"
)

// checkTCP tests a single TCP port once: it connects, sends a probe with the
// password, and waits for the server to send it back. It returns the outcome and, for an
// open port, the slower of the connection and the reply, each of which takes
// about one round trip.
func (t *target) checkTCP(ctx context.Context, port int, timeout time.Duration) (outcome, time.Duration) {
	addr := net.JoinHostPort(t.addr.String(), strconv.Itoa(port))
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	c, err := d.DialContext(ctx, t.network, addr)
	if err != nil {
		t.opts.logf("%s %d dial error: %s", t.kind, port, err)
		switch {
		case refusal(err):
			return outcomeRefused, 0
		case os.IsTimeout(err):
			return outcomeTimeout, 0
		case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.ECONNABORTED):
			// accepted, then cut before the connection was set up
			return outcomeInterfered, 0
		default:
			return outcomeError, 0
		}
	}
	connected := time.Now()
	conn := c.(*net.TCPConn)
	defer func() {
		if err := conn.Close(); err != nil {
			t.opts.logf("%s %d close error: %s", t.kind, port, err)
		}
	}()
	// unblock any pending read or write as soon as the context is canceled
	stopAfter := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stopAfter()

	if err := conn.SetDeadline(connected.Add(timeout)); err != nil {
		t.opts.logf("%s %d SetDeadline warning: %s", t.kind, port, err)
	}
	if err := conn.SetNoDelay(true); err != nil {
		t.opts.logf("%s %d SetNoDelay warning: %s", t.kind, port, err)
	}

	// the connection was accepted, so from here on any failure means that
	// something other than the server answered or cut the connection
	probe := newProbe(t.magic)
	if _, err := conn.Write(probe); err != nil {
		t.opts.logf("%s %d write error: %s", t.kind, port, err)
		return outcomeInterfered, 0
	}

	buffer := make([]byte, len(probe)+64)
	n := 0
	for {
		m, err := conn.Read(buffer[n:])
		n += m
		if err == nil && isPartialAnswer(buffer[:n], probe, t.magic) {
			continue
		}
		if isAnswer(buffer[:n], probe, t.magic) {
			break
		}
		if err != nil {
			t.opts.logf("%s %d read error: %s", t.kind, port, err)
			switch {
			case ctx.Err() != nil:
				return outcomeError, 0
			case os.IsTimeout(err):
				return outcomeSilent, 0
			default:
				return outcomeInterfered, 0
			}
		}
		t.opts.logf("%s %d unexpected reply: %q", t.kind, port, buffer[:n])
		return outcomeInterfered, 0
	}
	return outcomeOpen, max(connected.Sub(start), time.Since(connected))
}
