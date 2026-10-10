// Package main provides UDP server functionality for the portquiz server.
// It handles incoming UDP packets and responds with the magic string when detected.
package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"runtime"
	"sync"
)

// udpReadBuffer is the receive buffer requested for the UDP socket, so bursts
// of packets from fast clients are not dropped.
const udpReadBuffer = 4 << 20

// udpServer starts a UDP server on the specified address and handles incoming packets.
// Several goroutines read from the same socket so replies keep up with fast clients,
// and each responds to packets containing the magic string.
func udpServer(ctx context.Context, listenAddr string) error {
	log.Printf("starting UDP server on %s", listenAddr)
	addr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return err
	}

	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer func() {
		if err := l.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("UDP listener close error: %s", err)
		}
	}()
	if err := setUDPReadBuffer(l, udpReadBuffer); err != nil && *verbose {
		log.Printf("UDP read buffer warning: %s", err)
	}

	// close the socket on cancellation to unblock the readers
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()

	var wg sync.WaitGroup
	for range udpReaders() {
		wg.Go(func() { udpRead(l) })
	}
	wg.Wait()
	return ctx.Err()
}

// udpReaders returns how many goroutines read from the UDP socket.
func udpReaders() int {
	return min(max(runtime.NumCPU(), 2), 8)
}

// udpRead handles packets on l until it is closed.
func udpRead(l *net.UDPConn) {
	buffer := make([]byte, 128)
	for {
		n, remoteAddr, err := l.ReadFromUDP(buffer)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if *verbose {
				log.Printf("UDP read error: %s", err)
			}
			continue
		}
		if *verbose {
			log.Printf("[UDP] data from [%s] len: %d, data: %s", remoteAddr, n, buffer[:n])
		}
		if bytes.HasPrefix(buffer[:n], magicStringBytes) {
			if *verbose {
				log.Printf("[UDP] PORTQUIZ from %s", remoteAddr)
			}
			_, err = l.WriteToUDP(buffer[:n], remoteAddr)
			if err != nil && *verbose {
				log.Printf("UDP write error to [%s]: %s", remoteAddr, err)
			}
		}
	}
}
