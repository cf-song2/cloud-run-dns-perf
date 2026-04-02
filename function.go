package dnsperformance

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("RunDNSTest", handleDNSTest)
}

// handleDNSTest is the Cloud Function HTTP entry point.
func handleDNSTest(w http.ResponseWriter, r *http.Request) {
	log.Println("DNS performance test started")

	cfg, err := loadConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("configuration error: %v", err), http.StatusInternalServerError)
		log.Printf("ERROR: configuration error: %v", err)
		return
	}

	log.Printf("Config: region=%s, iterations=%d, servers=%d, domains=%d",
		cfg.Region, cfg.Iterations, len(cfg.DNSServers), len(cfg.Domains))

	// Run DNS performance tests
	run := RunDNSTest(cfg)

	log.Printf("Test completed: %d results collected", len(run.Results))

	// Upload results to R2
	if cfg.R2.AccountID != "" && cfg.R2.AccessKey != "" && cfg.R2.SecretKey != "" {
		if err := UploadResults(r.Context(), cfg.R2, run); err != nil {
			log.Printf("ERROR: failed to upload results to R2: %v", err)
			// Continue — still return results in the HTTP response
		} else {
			log.Printf("Results uploaded to R2 bucket %q", cfg.R2.BucketName)
		}
	} else {
		log.Println("WARN: R2 credentials not configured, skipping upload")
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(run); err != nil {
		log.Printf("ERROR: failed to encode response: %v", err)
	}
}

// loadConfig reads all configuration from environment variables.
func loadConfig() (Config, error) {
	cfg := Config{
		Region:     getEnvOrDefault("TEST_REGION", "unknown"),
		Iterations: getEnvAsIntOrDefault("TEST_ITERATIONS", 10),
		TimeoutSec: getEnvAsIntOrDefault("DNS_TIMEOUT_SEC", 5),
		R2: R2Config{
			AccountID:  os.Getenv("R2_ACCOUNT_ID"),
			AccessKey:  os.Getenv("R2_ACCESS_KEY_ID"),
			SecretKey:  os.Getenv("R2_SECRET_ACCESS_KEY"),
			BucketName: os.Getenv("R2_BUCKET_NAME"),
		},
	}

	// Parse DNS servers (required)
	serversStr := os.Getenv("DNS_SERVERS")
	if serversStr == "" {
		return cfg, fmt.Errorf("DNS_SERVERS environment variable is required")
	}
	servers, err := parseDNSServers(serversStr)
	if err != nil {
		return cfg, fmt.Errorf("parse DNS_SERVERS: %w", err)
	}
	cfg.DNSServers = servers

	// Parse domains (required)
	domainsStr := os.Getenv("DNS_DOMAINS")
	if domainsStr == "" {
		return cfg, fmt.Errorf("DNS_DOMAINS environment variable is required")
	}
	cfg.Domains = parseCSV(domainsStr)

	if len(cfg.DNSServers) == 0 {
		return cfg, fmt.Errorf("no DNS servers configured")
	}
	if len(cfg.Domains) == 0 {
		return cfg, fmt.Errorf("no domains configured")
	}

	return cfg, nil
}

// parseDNSServers parses "Name1=IP1,Name2=IP2" into a slice of DNSServer.
func parseDNSServers(s string) ([]DNSServer, error) {
	var servers []DNSServer
	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid server format %q, expected Name=IP", pair)
		}
		servers = append(servers, DNSServer{
			Name: strings.TrimSpace(parts[0]),
			IP:   strings.TrimSpace(parts[1]),
		})
	}
	return servers, nil
}

// parseCSV splits a comma-separated string into trimmed non-empty values.
func parseCSV(s string) []string {
	var result []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

// getEnvOrDefault returns the env var value or a default if unset/empty.
func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// getEnvAsIntOrDefault returns the env var as int, or a default.
func getEnvAsIntOrDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("WARN: invalid integer for %s=%q, using default %d", key, v, defaultVal)
		return defaultVal
	}
	return i
}
