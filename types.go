package dnsperformance

import "time"

// DNSServer represents a DNS server to test against.
type DNSServer struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Region     string      `json:"region"`
	TimeoutSec int         `json:"timeout_sec"`
	DNSServers []DNSServer `json:"dns_servers"`
	Domains    []string    `json:"domains"`
	R2         R2Config    `json:"-"`
}

// R2Config holds Cloudflare R2 connection settings.
type R2Config struct {
	AccountID  string
	AccessKey  string
	SecretKey  string
	BucketName string
}

// DNSResult holds the result of a single DNS query.
type DNSResult struct {
	DNSServer   string   `json:"dns_server"`
	DNSProvider string   `json:"dns_provider"`
	Domain      string   `json:"domain"`
	RTTMs       float64  `json:"rtt_ms"`
	Success     bool     `json:"success"`
	RCode       string   `json:"rcode"`
	AnswerCount int      `json:"answer_count"`
	ResolvedIPs []string `json:"resolved_ips,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// TestRun represents the complete output of a single test execution.
type TestRun struct {
	TestID    string      `json:"test_id"`
	Region    string      `json:"region"`
	Timestamp time.Time   `json:"timestamp"`
	Results   []DNSResult `json:"results"`
}
