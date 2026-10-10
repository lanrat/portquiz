//go:build !linux

package main

import "net"

// setUDPReadBuffer sets the socket receive buffer, capped by the system limit.
func setUDPReadBuffer(c *net.UDPConn, size int) error {
	return c.SetReadBuffer(size)
}
