package dnsperformance

import (
	"fmt"
	"math"
	"net"
	"sort"
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
		Config: TestRunConfig{
			Iterations: cfg.Iterations,
			TimeoutSec: cfg.TimeoutSec,
		},
		Results: make([]ServerDomainResult, 0, len(cfg.DNSServers)*len(cfg.Domains)),
	}

	for _, server := range cfg.DNSServers {
		for _, domain := range cfg.Domains {
			result := testServerDomain(server, domain, cfg.Iterations, cfg.TimeoutSec)
			run.Results = append(run.Results, result)
		}
	}

	return run
}

// testServerDomain runs all iterations for a single (server, domain) pair.
func testServerDomain(server DNSServer, domain string, iterations, timeoutSec int) ServerDomainResult {
	result := ServerDomainResult{
		DNSServer:   server.IP,
		DNSProvider: server.Name,
		Domain:      domain,
		Iterations:  make([]QueryResult, 0, iterations),
	}

	client := &dns.Client{
		Net:     "udp",
		Timeout: time.Duration(timeoutSec) * time.Second,
	}

	addr := net.JoinHostPort(server.IP, "53")

	for i := 0; i < iterations; i++ {
		qr := executeQuery(client, addr, domain, i+1)
		result.Iterations = append(result.Iterations, qr)
	}

	result.Summary = calculateSummary(result.Iterations)
	return result
}

// executeQuery performs a single DNS A-record query and returns the result.
func executeQuery(client *dns.Client, addr, domain string, iteration int) QueryResult {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	msg.RecursionDesired = true

	resp, rtt, err := client.Exchange(msg, addr)
	if err != nil {
		return QueryResult{
			Iteration: iteration,
			RTTMs:     0,
			Success:   false,
			RCode:     "TIMEOUT",
			Error:     err.Error(),
		}
	}

	qr := QueryResult{
		Iteration:   iteration,
		RTTMs:       float64(rtt.Microseconds()) / 1000.0,
		Success:     resp.Rcode == dns.RcodeSuccess,
		RCode:       dns.RcodeToString[resp.Rcode],
		AnswerCount: len(resp.Answer),
	}

	// Extract resolved IPs from A records.
	for _, ans := range resp.Answer {
		if a, ok := ans.(*dns.A); ok {
			qr.ResolvedIPs = append(qr.ResolvedIPs, a.A.String())
		}
	}

	if resp.Rcode != dns.RcodeSuccess {
		qr.Error = fmt.Sprintf("DNS response code: %s", dns.RcodeToString[resp.Rcode])
	}

	return qr
}

// calculateSummary computes aggregate metrics from a set of query results.
func calculateSummary(iterations []QueryResult) Summary {
	total := len(iterations)
	if total == 0 {
		return Summary{}
	}

	var successCount int
	var rtts []float64

	for _, q := range iterations {
		if q.Success {
			successCount++
			rtts = append(rtts, q.RTTMs)
		}
	}

	s := Summary{
		TotalQueries: total,
		FailureCount: total - successCount,
		SuccessRate:  float64(successCount) / float64(total) * 100.0,
	}

	if len(rtts) == 0 {
		return s
	}

	sort.Float64s(rtts)

	s.MinMs = rtts[0]
	s.MaxMs = rtts[len(rtts)-1]
	s.AvgMs = mean(rtts)
	s.MedianMs = percentile(rtts, 50)
	s.P95Ms = percentile(rtts, 95)
	s.StddevMs = stddev(rtts)

	// Round all values to 3 decimal places.
	s.MinMs = roundTo(s.MinMs, 3)
	s.MaxMs = roundTo(s.MaxMs, 3)
	s.AvgMs = roundTo(s.AvgMs, 3)
	s.MedianMs = roundTo(s.MedianMs, 3)
	s.P95Ms = roundTo(s.P95Ms, 3)
	s.StddevMs = roundTo(s.StddevMs, 3)
	s.SuccessRate = roundTo(s.SuccessRate, 1)

	return s
}

// mean calculates the arithmetic mean of a sorted slice.
func mean(sorted []float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	var sum float64
	for _, v := range sorted {
		sum += v
	}
	return sum / float64(len(sorted))
}

// percentile calculates the p-th percentile using linear interpolation.
// Input must be sorted in ascending order.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}

	rank := (p / 100.0) * float64(n-1)
	lower := int(math.Floor(rank))
	upper := lower + 1
	if upper >= n {
		return sorted[n-1]
	}

	frac := rank - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
}

// stddev calculates the population standard deviation.
func stddev(sorted []float64) float64 {
	if len(sorted) <= 1 {
		return 0
	}
	avg := mean(sorted)
	var sumSqDiff float64
	for _, v := range sorted {
		diff := v - avg
		sumSqDiff += diff * diff
	}
	return math.Sqrt(sumSqDiff / float64(len(sorted)))
}

// roundTo rounds a float to n decimal places.
func roundTo(val float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(val*pow) / pow
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
