package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"net"
	"testing"
	"time"
)

// the transaction ID of the RFC 5769 test vectors
var rfcTxID, _ = hex.DecodeString("b7e7a701bc34d686fa87dfae")

func bindingRequest(txID []byte) []byte {
	req := make([]byte, stunHeaderLen)
	binary.BigEndian.PutUint16(req[0:2], stunBindingRequest)
	binary.BigEndian.PutUint32(req[4:8], stunMagicCookie)
	copy(req[8:], txID)
	return req
}

func TestIsSTUNBinding(t *testing.T) {
	req := bindingRequest(rfcTxID)
	if !isSTUNBinding(req) {
		t.Error("binding request not recognized")
	}
	withAttr := appendSTUNAttr(append([]byte{}, req...), 0x8022, []byte("chrome"))
	if !isSTUNBinding(withAttr) {
		t.Error("binding request with an attribute not recognized")
	}
	for name, p := range map[string][]byte{
		"short":       req[:19],
		"portquiz":    []byte("portquiz-password-and-more"),
		"response":    append([]byte{0x01, 0x01}, req[2:]...),
		"bad cookie":  append(append([]byte{}, req[:4]...), make([]byte, 16)...),
		"bad length":  append(append([]byte{}, req...), 0, 0, 0, 0),
		"high bits":   append([]byte{0xC0, 0x01}, req[2:]...),
		"empty":       nil,
		"http":        []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"),
		"truncated":   withAttr[:len(withAttr)-4],
		"zero header": make([]byte, 20),
	} {
		if isSTUNBinding(p) {
			t.Errorf("%s: recognized as a binding request", name)
		}
	}
}

// attrs returns a STUN message's attributes by type.
func attrs(t *testing.T, msg []byte) map[uint16][]byte {
	t.Helper()
	out := map[uint16][]byte{}
	for p := msg[stunHeaderLen:]; len(p) > 0; {
		typ, n := binary.BigEndian.Uint16(p[0:2]), int(binary.BigEndian.Uint16(p[2:4]))
		out[typ] = p[4 : 4+n]
		p = p[4+(n+3)/4*4:]
	}
	return out
}

func TestBindingResponseMatchesRFC5769(t *testing.T) {
	for _, c := range []struct {
		addr *net.UDPAddr
		want string
	}{
		// RFC 5769 section 2.2 and 2.3
		{&net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 32853}, "0001a147e112a643"},
		{&net.UDPAddr{IP: net.ParseIP("2001:db8:1234:5678:11:2233:4455:6677"), Port: 32853}, "0002a1470113a9faa5d3f179bc25f4b5bed2b9d9"},
	} {
		res := stunBindingResponse(bindingRequest(rfcTxID), c.addr)
		if binary.BigEndian.Uint16(res[0:2]) != stunBindingSuccess || !bytes.Equal(res[8:20], rfcTxID) ||
			int(binary.BigEndian.Uint16(res[2:4])) != len(res)-stunHeaderLen || len(res)%4 != 0 {
			t.Errorf("%s: bad header % x", c.addr, res[:20])
		}
		a := attrs(t, res)
		if got := hex.EncodeToString(a[stunXORMappedAddr]); got != c.want {
			t.Errorf("%s: XOR-MAPPED-ADDRESS %s, want %s", c.addr, got, c.want)
		}
		fp := a[stunFingerprint]
		want := crc32.ChecksumIEEE(res[:len(res)-8]) ^ stunFingerprintXOR
		if len(fp) != 4 || binary.BigEndian.Uint32(fp) != want {
			t.Errorf("%s: bad fingerprint", c.addr)
		}
	}
}

// startUDP runs the UDP reader on a localhost port and returns a socket
// connected to it.
func startUDP(t *testing.T, web bool) *net.UDPConn {
	t.Helper()
	l, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go udpRead(l, web)
	c, err := net.DialUDP("udp4", nil, l.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func roundTrip(t *testing.T, c *net.UDPConn, p []byte) []byte {
	t.Helper()
	if _, err := c.Write(p); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	buf := make([]byte, 2048)
	n, err := c.Read(buf)
	if err != nil {
		return nil
	}
	return buf[:n]
}

func TestUDPAnswersSTUNAndPassword(t *testing.T) {
	c := startUDP(t, true)
	res := roundTrip(t, c, bindingRequest(rfcTxID))
	if res == nil || binary.BigEndian.Uint16(res[0:2]) != stunBindingSuccess {
		t.Fatalf("no binding response: % x", res)
	}
	// the mapped address is the client's own
	x := attrs(t, res)[stunXORMappedAddr]
	port := binary.BigEndian.Uint16(x[2:4]) ^ (stunMagicCookie >> 16)
	if int(port) != c.LocalAddr().(*net.UDPAddr).Port {
		t.Errorf("mapped port %d, want %d", port, c.LocalAddr().(*net.UDPAddr).Port)
	}
	if got := roundTrip(t, c, []byte("portquiz")); string(got) != "portquiz" {
		t.Errorf("password: got %q", got)
	}
	if got := roundTrip(t, c, []byte("something else")); got != nil {
		t.Errorf("unknown data: got %q", got)
	}
}

func TestUDPWebDisabled(t *testing.T) {
	c := startUDP(t, false)
	if got := roundTrip(t, c, bindingRequest(rfcTxID)); got != nil {
		t.Errorf("with -web=false: got % x", got)
	}
	if got := roundTrip(t, c, []byte("portquiz")); string(got) != "portquiz" {
		t.Errorf("password: got %q", got)
	}
}

func TestUDPPasswordMustMatchExactly(t *testing.T) {
	c := startUDP(t, true)
	for _, wrong := range []string{"portquiz123", "port", "portquiz\n", "xportquiz"} {
		if got := roundTrip(t, c, []byte(wrong)); got != nil {
			t.Errorf("%q: got %q, want no answer", wrong, got)
		}
	}
	if got := roundTrip(t, c, []byte("portquiz")); string(got) != "portquiz" {
		t.Errorf("right password: got %q", got)
	}
}
