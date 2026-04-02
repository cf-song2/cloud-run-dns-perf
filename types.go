package dnsperformance

import "time"

// DNSServer represents a DNS server to test against.
type DNSServer struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	// Test parameters
	Region     string      `json:"region"`
	Iterations int         `json:"iterations"`
	TimeoutSec int         `json:"timeout_sec"`
	DNSServers []DNSServer `json:"dns_servers"`
	Domains    []string    `json:"domains"`

	// R2 storage
	R2 R2Config `json:"-"`
}

// R2Config holds Cloudflare R2 connection settings.
type R2Config struct {
	AccountID  string
	AccessKey  string
	SecretKey  string
	BucketName string
}

// QueryResult holds the result of a single DNS query iteration.
type QueryResult struct {
	Iteration   int      `json:"iteration"`
	RTTMs       float64  `json:"rtt_ms"`
	Success     bool     `json:"success"`
	RCode       string   `json:"rcode"`
	AnswerCount int      `json:"answer_count"`
	ResolvedIPs []string `json:"resolved_ips,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// Summary holds aggregate metrics for a (server, domain) pair.
type Summary struct {
	MinMs        float64 `json:"min_ms"`
	MaxMs        float64 `json:"max_ms"`
	AvgMs        float64 `json:"avg_ms"`
	MedianMs     float64 `json:"median_ms"`
	P95Ms        float64 `json:"p95_ms"`
	StddevMs     float64 `json:"stddev_ms"`
	SuccessRate  float64 `json:"success_rate"`
	FailureCount int     `json:"failure_count"`
	TotalQueries int     `json:"total_queries"`
}

// ServerDomainResult holds all iterations and summary for one (server, domain) pair.
type ServerDomainResult struct {
	DNSServer   string        `json:"dns_server"`
	DNSProvider string        `json:"dns_provider"`
	Domain      string        `json:"domain"`
	Iterations  []QueryResult `json:"iterations"`
	Summary     Summary       `json:"summary"`
}

// TestRun represents the complete output of a single test execution.
type TestRun struct {
	TestID    string               `json:"test_id"`
	Region    string               `json:"region"`
	Timestamp time.Time            `json:"timestamp"`
	Config    TestRunConfig        `json:"config"`
	Results   []ServerDomainResult `json:"results"`
}

// TestRunConfig is a subset of Config included in the output for context.
type TestRunConfig struct {
	Iterations int `json:"iterations"`
	TimeoutSec int `json:"timeout_sec"`
}

// CSVRow represents a single flat row for CSV output.
type CSVRow struct {
	TestID      string
	Region      string
	Timestamp   string
	DNSServer   string
	DNSProvider string
	Domain      string
	Iteration   int
	RTTMs       float64
	Success     bool
	RCode       string
	AnswerCount int
	ResolvedIPs string
	Error       string
}
