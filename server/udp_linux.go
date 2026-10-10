package main

import (
	"net"
	"syscall"
)

// setUDPReadBuffer sets the socket receive buffer. The server runs as root, so
// it uses SO_RCVBUFFORCE to go beyond the system limit (net.core.rmem_max,
// often only 208 KiB), falling back to the regular limited option.
func setUDPReadBuffer(c *net.UDPConn, size int) error {
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var sockErr error
	if err := raw.Control(func(fd uintptr) {
		sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUFFORCE, size)
	}); err != nil {
		return err
	}
	if sockErr != nil {
		return c.SetReadBuffer(size)
	}
	return nil
}
