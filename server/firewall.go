package main

import (
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/coreos/go-iptables/iptables"
)

var ip4t *iptables.IPTables
var ip6t *iptables.IPTables

var fwComment = magicString
var fw4Rules [][]string
var fw6Rules [][]string

const insertRilePos = 1

func newRule(ip, port, proto string) []string {
	return []string{
		"--destination", ip,
		"-p", proto,
		"-j", "DNAT",
		"--to-destination", ":" + port, // Correctly format the destination
		"-m", "comment",
		"--comment", fwComment,
	}
}

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
			if *verbose {
				log.Printf("Adding firewall IPv4 rule %+v", rule)
				docker, err := ip4t.ChainExists("nat", "DOCKER")
				if err != nil {
					return err
				}
				log.Printf("docker chain: %t", docker)
			}
			err := ip4t.InsertUnique("nat", "PREROUTING", insertRilePos, rule...)
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
			err := ip6t.InsertUnique("nat", "PREROUTING", insertRilePos, rule...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanupFW() error {
	var err error
	for _, rule := range fw4Rules {
		if *verbose {
			log.Printf("Removing firewall IPv4 rule %+v", rule)
		}
		err2 := ip4t.Delete("nat", "PREROUTING", rule...)
		if err != nil {
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
		if err != nil {
			if *verbose {
				log.Printf("Error: %s", err2)
			}
			err = errors.Join(err, err2)
		}
	}
	return err
}
