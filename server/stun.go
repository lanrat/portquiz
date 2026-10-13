// Package main answers STUN binding requests (RFC 8489), which the browser
// test page uses to test UDP ports: browsers cannot send raw UDP, but a page
// can have WebRTC ask a STUN server, on any port, for its public address.
package main

import (
	"encoding/binary"
	"hash/crc32"
	"net"
)

const (
	stunHeaderLen      = 20
	stunMagicCookie    = 0x2112A442
	stunBindingRequest = 0x0001
	stunBindingSuccess = 0x0101
	stunXORMappedAddr  = 0x0020
	stunFingerprint    = 0x8028
	stunFingerprintXOR = 0x5354554e
)

// isSTUNBinding reports whether a packet is a STUN binding request.
func isSTUNBinding(p []byte) bool {
	return len(p) >= stunHeaderLen &&
		p[0]&0xC0 == 0 &&
		binary.BigEndian.Uint16(p[0:2]) == stunBindingRequest &&
		int(binary.BigEndian.Uint16(p[2:4])) == len(p)-stunHeaderLen &&
		binary.BigEndian.Uint32(p[4:8]) == stunMagicCookie
}

// stunBindingResponse returns the success response to the binding request
// req from addr: the address the request came from, as the client's
// public address, and a fingerprint.
func stunBindingResponse(req []byte, addr *net.UDPAddr) []byte {
	family := byte(0x01)
	ip := addr.IP.To4()
	if ip == nil {
		family = 0x02
		ip = addr.IP.To16()
	}
	// XOR-MAPPED-ADDRESS: the port and address XORed with the magic cookie,
	// and for IPv6 also the transaction ID
	value := make([]byte, 4+len(ip))
	value[1] = family
	binary.BigEndian.PutUint16(value[2:4], uint16(addr.Port)^(stunMagicCookie>>16))
	mask := make([]byte, 16)
	binary.BigEndian.PutUint32(mask[0:4], stunMagicCookie)
	copy(mask[4:], req[8:20])
	for i := range ip {
		value[4+i] = ip[i] ^ mask[i]
	}

	res := make([]byte, stunHeaderLen, stunHeaderLen+4+len(value)+8)
	binary.BigEndian.PutUint16(res[0:2], stunBindingSuccess)
	binary.BigEndian.PutUint32(res[4:8], stunMagicCookie)
	copy(res[8:20], req[8:20])
	res = appendSTUNAttr(res, stunXORMappedAddr, value)
	// the fingerprint covers the message up to it, with the length already
	// including it
	binary.BigEndian.PutUint16(res[2:4], uint16(len(res)-stunHeaderLen+8))
	crc := crc32.ChecksumIEEE(res) ^ stunFingerprintXOR
	res = appendSTUNAttr(res, stunFingerprint, binary.BigEndian.AppendUint32(nil, crc))
	return res
}

// appendSTUNAttr appends an attribute to a STUN message, padded to four
// bytes, and updates the message length.
func appendSTUNAttr(msg []byte, typ uint16, value []byte) []byte {
	msg = binary.BigEndian.AppendUint16(msg, typ)
	msg = binary.BigEndian.AppendUint16(msg, uint16(len(value)))
	msg = append(msg, value...)
	for len(msg)%4 != 0 {
		msg = append(msg, 0)
	}
	binary.BigEndian.PutUint16(msg[2:4], uint16(len(msg)-stunHeaderLen))
	return msg
}
