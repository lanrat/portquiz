package main

import (
	"errors"
	"log"
	"net"

	"github.com/coreos/go-iptables/iptables"
)

var ipt *iptables.IPTables

var fwComment = magicString
var fwRules [][]string

func init() {
	var err error
	ipt, err = iptables.New()
	if err != nil {
		log.Fatalf("Failed to initialize iptables: %v", err)
	}
}

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

func addFWRules() error {
	ip, port, err := net.SplitHostPort(*listen)
	if err != nil {
		return err
	}
	if *tcp {
		fwRules = append(fwRules, newRule(ip, port, "tcp"))
	}
	if *udp {
		fwRules = append(fwRules, newRule(ip, port, "udp"))
	}
	for _, rule := range fwRules {
		if *verbose {
			log.Printf("Adding firewall rule %+v", rule)
		}
		err = ipt.Append("nat", "PREROUTING", rule...)
		if err != nil {
			return err
		}
	}
	return nil
}

func cleanupFW() error {
	var err error
	for _, rule := range fwRules {
		if *verbose {
			log.Printf("Removing firewall rule %+v", rule)
		}
		err2 := ipt.Delete("nat", "PREROUTING", rule...)
		if err != nil {
			if *verbose {
				log.Printf("Error: %s", err2)
			}
			err = errors.Join(err, err2)
		}
	}
	return err
}
