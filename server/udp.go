// Package main provides UDP server functionality for the portquiz server.
// It handles incoming UDP packets and responds with the magic string when detected.
package main

import (
	"bytes"
	"context"
	"log"
	"net"
)

// udpServer starts a UDP server on the specified address and handles incoming packets.
// It reads packets in a loop and responds to those containing the magic string.
// TODO: detect canceled context
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
		if err := l.Close(); err != nil {
			log.Printf("UDP listener close error: %s", err)
		}
	}()
	if err := l.SetReadBuffer(len(magicStringBytes) * 2); err != nil && *verbose {
		log.Printf("UDP SetReadBuffer error: %s", err)
	}

	buffer := make([]byte, 128)

	// Start a goroutine to handle context cancellation
	go func() {
		<-ctx.Done()
		if err := l.Close(); err != nil {
			log.Printf("UDP listener close on context cancel error: %s", err)
		}
	}()

	for {
		n, remoteAddr, err := l.ReadFromUDP(buffer)
		if err != nil {
			// Check if context was cancelled
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
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
				continue
			}
		}
	}
}
