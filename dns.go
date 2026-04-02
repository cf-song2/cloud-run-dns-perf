package dnsperformance

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// RunDNSTest executes DNS performance tests for all server/domain combinations.
func RunDNSTest(cfg Config) *TestRun {
	run := &TestRun{
		TestID:    generateTestID(),
		Region:    cfg.Region,
		Timestamp: time.Now().UTC(),
		Results:   make([]DNSResult, 0, len(cfg.DNSServers)*len(cfg.Domains)),
	}

	client := &dns.Client{
		Net:     "udp",
		Timeout: time.Duration(cfg.TimeoutSec) * time.Second,
	}

	for _, server := range cfg.DNSServers {
		addr := net.JoinHostPort(server.IP, "53")
		for _, domain := range cfg.Domains {
			result := executeQuery(client, addr, server, domain)
			run.Results = append(run.Results, result)
		}
	}

	return run
}

// executeQuery performs a single DNS A-record query and returns the result.
func executeQuery(client *dns.Client, addr string, server DNSServer, domain string) DNSResult {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	msg.RecursionDesired = true

	resp, rtt, err := client.Exchange(msg, addr)
	if err != nil {
		return DNSResult{
			DNSServer:   server.IP,
			DNSProvider: server.Name,
			Domain:      domain,
			RTTMs:       0,
			Success:     false,
			RCode:       "TIMEOUT",
			Error:       err.Error(),
		}
	}

	result := DNSResult{
		DNSServer:   server.IP,
		DNSProvider: server.Name,
		Domain:      domain,
		RTTMs:       float64(rtt.Microseconds()) / 1000.0,
		Success:     resp.Rcode == dns.RcodeSuccess,
		RCode:       dns.RcodeToString[resp.Rcode],
		AnswerCount: len(resp.Answer),
	}

	// Extract resolved IPs from A records.
	for _, ans := range resp.Answer {
		if a, ok := ans.(*dns.A); ok {
			result.ResolvedIPs = append(result.ResolvedIPs, a.A.String())
		}
	}

	if resp.Rcode != dns.RcodeSuccess {
		result.Error = fmt.Sprintf("DNS response code: %s", dns.RcodeToString[resp.Rcode])
	}

	return result
}

// generateTestID creates a simple unique ID from the current timestamp.
func generateTestID() string {
	now := time.Now().UTC()
	return fmt.Sprintf("%s-%s",
		now.Format("20060102-150405"),
		randomSuffix(),
	)
}

// randomSuffix generates a short random-ish suffix from nanoseconds.
func randomSuffix() string {
	return fmt.Sprintf("%04x", time.Now().UnixNano()&0xFFFF)
}

// JoinIPs joins a slice of IPs into a semicolon-separated string for CSV.
func JoinIPs(ips []string) string {
	return strings.Join(ips, ";")
}
