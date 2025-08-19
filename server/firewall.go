// Package main provides firewall management functionality for the portquiz server.
// It handles creation and cleanup of iptables rules for both IPv4 and IPv6.
package main

import (
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/coreos/go-iptables/iptables"
)

// ip4t holds the IPv4 iptables instance for managing firewall rules.
var ip4t *iptables.IPTables

// ip6t holds the IPv6 iptables instance for managing firewall rules.
var ip6t *iptables.IPTables

// fw4Rules stores all IPv4 firewall rules created by the server for cleanup.
var fw4Rules [][]string

// fw6Rules stores all IPv6 firewall rules created by the server for cleanup.
var fw6Rules [][]string

// insertRulePos specifies the position where iptables rules should be inserted.
const insertRulePos = 1

// newRule creates a new iptables DNAT rule for the specified IP, port, and protocol.
// It returns a slice of iptables rule arguments that can be used with the iptables library.
func newRule(ip, port, proto string) []string {
	fwComment := *magicString
	return []string{
		"--destination", ip,
		"-p", proto,
		"-j", "DNAT",
		"--to-destination", ":" + port, // Correctly format the destination
		"-m", "comment",
		"--comment", fwComment,
	}
}

// addFWRules creates and applies iptables rules for the specified IP and port.
// It supports both IPv4 and IPv6 addresses and creates rules for TCP and/or UDP based on flags.
func addFWRules(ip, port string) error {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		// Not a valid IP address
		return fmt.Errorf("%q is not a valid IP", ip)
	}

	if parsedIP.IsLoopback() {
		// running on loopback interfaces is unsupported
		return fmt.Errorf("%q is a loopback IP", ip)
	}
	if parsedIP.To4() != nil {
		// IPv4
		// init v4
		if ip4t == nil {
			var err error
			ip4t, err = iptables.NewWithProtocol(iptables.ProtocolIPv4)
			if err != nil {
				return err
			}
		}

		// add rules
		if *tcp {
			fw4Rules = append(fw4Rules, newRule(ip, port, "tcp"))
		}
		if *udp {
			fw4Rules = append(fw4Rules, newRule(ip, port, "udp"))
		}
		for _, rule := range fw4Rules {
			err := ip4t.InsertUnique("nat", "PREROUTING", insertRulePos, rule...)
			if err != nil {
				return err
			}
		}
	} else {
		// IPv6
		// init IPv6
		if ip6t == nil {
			var err error
			ip6t, err = iptables.NewWithProtocol(iptables.ProtocolIPv6)
			if err != nil {
				return err
			}
		}

		// add rules
		if *tcp {
			fw6Rules = append(fw6Rules, newRule(ip, port, "tcp"))
		}
		if *udp {
			fw6Rules = append(fw6Rules, newRule(ip, port, "udp"))
		}
		for _, rule := range fw6Rules {
			if *verbose {
				log.Printf("Adding firewall IPv6 rule %+v", rule)
			}
			err := ip6t.InsertUnique("nat", "PREROUTING", insertRulePos, rule...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// cleanupFW removes all iptables rules that were created by the server.
// It attempts to remove both IPv4 and IPv6 rules and returns any accumulated errors.
func cleanupFW() error {
	var err error
	for _, rule := range fw4Rules {
		if *verbose {
			log.Printf("Removing firewall IPv4 rule %+v", rule)
		}
		err2 := ip4t.Delete("nat", "PREROUTING", rule...)
		if err2 != nil {
			if *verbose {
				log.Printf("Error: %s", err2)
			}
			err = errors.Join(err, err2)
		}
	}
	for _, rule := range fw6Rules {
		if *verbose {
			log.Printf("Removing firewall IPv6 rule %+v", rule)
		}
		err2 := ip6t.Delete("nat", "PREROUTING", rule...)
		if err2 != nil {
			if *verbose {
				log.Printf("Error: %s", err2)
			}
			err = errors.Join(err, err2)
		}
	}
	return err
}
