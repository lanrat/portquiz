package scan

import (
	"bytes"
	"crypto/rand"
)

// A check sends a probe: the password, a zero byte and a random tag of
// probeTagLen bytes. The server sends the probe back, so no two checks
// send the same bytes, and an answer can be told apart from an answer to
// another check. Servers from before tags answer with just the password
// (TCP) or the whole probe (UDP), so the bare password is accepted too.
const probeTagLen = 8

// newProbe returns a probe for password with a new random tag.
func newProbe(password []byte) []byte {
	p := make([]byte, len(password)+1+probeTagLen)
	copy(p, password)
	_, _ = rand.Read(p[len(password)+1:])
	return p
}

// isAnswer reports whether reply answers probe for password: the probe
// itself, or from an older server the bare password.
func isAnswer(reply, probe, password []byte) bool {
	return bytes.Equal(reply, probe) || bytes.Equal(reply, password)
}

// isPartialAnswer reports whether reply is the start of the answer to
// probe, with more still to come.
func isPartialAnswer(reply, probe, password []byte) bool {
	return len(reply) < len(probe) && bytes.HasPrefix(probe, reply) && !bytes.Equal(reply, password)
}
