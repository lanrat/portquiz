package main

import "bytes"

// probeTagLen is the length of the random tag a client adds after the
// password and a zero byte, so no two of its checks send the same bytes.
// It must match the client's (scan/probe.go).
const probeTagLen = 8

// probeReply returns the answer to data, a client's probe: the bare
// password, from clients before tags, is answered with the password, and
// the password with a zero byte and a tag is sent back as it is, so the
// client can tell the answer is to that probe. Anything else gets no
// answer, including a wrong password that merely starts with the right
// one, such as "portquiz123" for "portquiz".
func probeReply(data []byte) ([]byte, bool) {
	p := magicStringBytes
	if bytes.Equal(data, p) {
		return p, true
	}
	if len(data) == len(p)+1+probeTagLen && bytes.HasPrefix(data, p) && data[len(p)] == 0 {
		return data, true
	}
	return nil, false
}

// isPartialProbe reports whether data is the start of a probe with more
// still to arrive: part of the password, or the password and part of its
// tag.
func isPartialProbe(data []byte) bool {
	p := magicStringBytes
	if len(data) < len(p) {
		return bytes.HasPrefix(p, data)
	}
	return len(data) > len(p) && len(data) < len(p)+1+probeTagLen && bytes.HasPrefix(data, p) && data[len(p)] == 0
}
