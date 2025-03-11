package main

import (
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

func addFWRules() error {
	ip, port, err := net.SplitHostPort(*listen)
	if err != nil {
		return err
	}
	ruleSpec := []string{
		"--destination", ip,
		"-p", "",
		"-j", "DNAT",
		"--to-destination", ":" + port, // Correctly format the destination
		"-m", "comment",
		"--comment", fwComment,
	}
	if *tcp {
		ruleSpec[3] = "tcp"
		fwRules = append(fwRules, ruleSpec)
	}
	if *udp {
		ruleSpec[3] = "udp"
		fwRules = append(fwRules, ruleSpec)
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
	for _, rule := range fwRules {
		if *verbose {
			log.Printf("Removing firewall rule %+v", rule)
		}
		err := ipt.Delete("nat", "PREROUTING", rule...)
		if err != nil {
			return err
		}
	}
	return nil
}
