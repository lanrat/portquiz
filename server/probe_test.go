package main

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestProbeReply(t *testing.T) {
	tagged := []byte("portquiz\x0012345678")
	for data, want := range map[string]string{
		"portquiz":                "portquiz", // clients before tags
		string(tagged):            string(tagged),
		"portquiz123":             "",
		"portquiz123\x0012345678": "",
		"portquiz\x001234567":     "", // tag too short
		"portquiz\x00123456789":   "", // tag too long
		"portquiz\x0112345678":    "", // no zero byte
		"port":                    "",
		"xportquiz":               "",
		"":                        "",
	} {
		reply, ok := probeReply([]byte(data))
		if ok != (want != "") || string(reply) != want {
			t.Errorf("probeReply(%q) = %q, %v; want %q", data, reply, ok, want)
		}
	}
	for data, want := range map[string]bool{
		"port":                 true,
		"portquiz\x00123":      true,
		"portquiz":             false,
		"portquiz\x0012345678": false,
		"portquiz1":            false,
		"x":                    false,
	} {
		if got := isPartialProbe([]byte(data)); got != want {
			t.Errorf("isPartialProbe(%q) = %v", data, got)
		}
	}
}

func TestTCPTaggedProbe(t *testing.T) {
	addr := startServer(t)
	probe := "portquiz\x00abcdefgh"
	if got := exchange(t, addr, probe); got != probe {
		t.Errorf("tagged probe: got %q, want it sent back", got)
	}
	if got := exchange(t, addr, "portquiz123\x00abcdefgh"); got != "" {
		t.Errorf("wrong password with a tag: got %q", got)
	}
	// a probe split inside the password and inside the tag is put back
	// together; split right after the password, it is taken for a bare
	// password from an older client, which clients accept as an answer too
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	for _, part := range []string{"port", "quiz\x00abc", "defgh"} {
		_, _ = io.WriteString(c, part)
		time.Sleep(30 * time.Millisecond)
	}
	buf := make([]byte, 64)
	n, _ := io.ReadAtLeast(c, buf, len(probe))
	if string(buf[:n]) != probe {
		t.Errorf("split probe: got %q", buf[:n])
	}
}

func TestUDPTaggedProbe(t *testing.T) {
	c := startUDP(t, true)
	probe := []byte("portquiz\x00abcdefgh")
	if got := roundTrip(t, c, probe); !bytes.Equal(got, probe) {
		t.Errorf("tagged probe: got %q", got)
	}
	for _, wrong := range []string{"portquiz123\x00abcdefgh", "portquiz\x00abc", "portquiz\x00abcdefghi"} {
		if got := roundTrip(t, c, []byte(wrong)); got != nil {
			t.Errorf("%q: got %q, want no answer", wrong, got)
		}
	}
}
