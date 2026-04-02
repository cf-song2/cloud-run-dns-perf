// Package main provides a local testing entry point for the DNS performance function.
// It loads environment variables from a .env file in the project root (if present)
// and starts a local HTTP server using the Functions Framework.
//
// Usage:
//
//	cd cmd && go run main.go
//	# Then visit http://localhost:8080/RunDNSTest or curl http://localhost:8080/RunDNSTest
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	// Blank import registers the function via init().
	_ "github.com/dns-performance-test"

	"github.com/GoogleCloudPlatform/functions-framework-go/funcframework"
)

func main() {
	// Try to load .env from the project root (one level up from cmd/).
	loadEnvFile(filepath.Join("..", ".env"))

	port := "8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	fmt.Printf("Starting local server on :%s\n", port)
	fmt.Printf("  Test endpoint: http://localhost:%s/RunDNSTest\n", port)
	fmt.Println("  Press Ctrl+C to stop")
	fmt.Println()

	if err := funcframework.Start(port); err != nil {
		log.Fatalf("funcframework.Start: %v\n", err)
	}
}

// loadEnvFile reads a .env file and sets environment variables.
// It silently does nothing if the file doesn't exist.
// It does NOT override variables that are already set in the environment.
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		// .env file is optional for local dev
		return
	}
	defer f.Close()

	log.Printf("Loading environment from %s", path)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes if present
		value = strings.Trim(value, `"'`)

		// Don't override existing env vars
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}
