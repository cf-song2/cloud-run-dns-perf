package dnsperformance

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// createR2Client initializes an S3-compatible client for Cloudflare R2.
func createR2Client(ctx context.Context, r2cfg R2Config) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(r2cfg.AccessKey, r2cfg.SecretKey, ""),
		),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(
			fmt.Sprintf("https://%s.r2.cloudflarestorage.com", r2cfg.AccountID),
		)
	})

	return client, nil
}

// UploadResults appends results to daily CSV file and individual JSON file.
// CSV: One file per region per day (appended)
// JSON: One file per test run (for detailed debugging)
func UploadResults(ctx context.Context, r2cfg R2Config, run *TestRun) error {
	client, err := createR2Client(ctx, r2cfg)
	if err != nil {
		return fmt.Errorf("create R2 client: %w", err)
	}

	ts := run.Timestamp
	dateStr := ts.Format("2006-01-02")

	// CSV: Append to daily file per region
	// Format: csv/us-central1/2026-04-02.csv
	csvKey := fmt.Sprintf("csv/%s/%s.csv", run.Region, dateStr)
	if err := appendCSVToR2(ctx, client, r2cfg.BucketName, csvKey, run); err != nil {
		return fmt.Errorf("append CSV: %w", err)
	}

	// JSON: Still create individual files for detailed debugging
	// Format: json/us-central1/2026-04-02/2026-04-02T14-00-00Z.json
	jsonKey := fmt.Sprintf("json/%s/%s/%s.json", run.Region, dateStr, ts.Format("2006-04-02T15-04-05Z"))
	jsonData, err := marshalJSON(run)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	if err := uploadToR2(ctx, client, r2cfg.BucketName, jsonKey, jsonData, "application/json"); err != nil {
		return fmt.Errorf("upload JSON: %w", err)
	}

	return nil
}

// appendCSVToR2 downloads existing CSV (if any), appends new rows, and uploads.
func appendCSVToR2(ctx context.Context, client *s3.Client, bucket, key string, run *TestRun) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var existingData []byte
	needsHeader := true

	// Try to get existing file
	getResult, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		defer getResult.Body.Close()
		existingData, err = io.ReadAll(getResult.Body)
		if err != nil {
			log.Printf("WARN: failed to read existing CSV: %v", err)
			existingData = nil
		} else if len(existingData) > 0 {
			needsHeader = false
		}
	}
	// If file doesn't exist, that's fine - we'll create it

	// Generate new CSV rows
	newData, err := marshalCSVRows(run, needsHeader)
	if err != nil {
		return fmt.Errorf("marshal CSV rows: %w", err)
	}

	// Combine existing + new data
	var finalData []byte
	if len(existingData) > 0 {
		finalData = append(existingData, newData...)
	} else {
		finalData = newData
	}

	// Upload combined data
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(finalData),
		ContentType: aws.String("text/csv"),
	})
	return err
}

// uploadToR2 uploads a byte slice to an R2 bucket.
func uploadToR2(ctx context.Context, client *s3.Client, bucket, key string, data []byte, contentType string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	return err
}

// marshalJSON serializes the test run to indented JSON.
func marshalJSON(run *TestRun) ([]byte, error) {
	return json.MarshalIndent(run, "", "  ")
}

// marshalCSVRows generates CSV rows, optionally with header.
func marshalCSVRows(run *TestRun, includeHeader bool) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Write header only if needed
	if includeHeader {
		header := []string{
			"test_id", "region", "timestamp",
			"dns_server", "dns_provider", "domain",
			"iteration", "rtt_ms", "success", "rcode",
			"answer_count", "resolved_ips", "error",
			// Summary columns (repeated per row for easier pivot table usage)
			"avg_ms", "min_ms", "max_ms", "median_ms",
			"p95_ms", "stddev_ms", "success_rate", "failure_count",
		}
		if err := w.Write(header); err != nil {
			return nil, err
		}
	}

	tsStr := run.Timestamp.Format(time.RFC3339)

	for _, res := range run.Results {
		for _, iter := range res.Iterations {
			row := []string{
				run.TestID,
				run.Region,
				tsStr,
				res.DNSServer,
				res.DNSProvider,
				res.Domain,
				strconv.Itoa(iter.Iteration),
				fmt.Sprintf("%.3f", iter.RTTMs),
				strconv.FormatBool(iter.Success),
				iter.RCode,
				strconv.Itoa(iter.AnswerCount),
				JoinIPs(iter.ResolvedIPs),
				iter.Error,
				// Summary
				fmt.Sprintf("%.3f", res.Summary.AvgMs),
				fmt.Sprintf("%.3f", res.Summary.MinMs),
				fmt.Sprintf("%.3f", res.Summary.MaxMs),
				fmt.Sprintf("%.3f", res.Summary.MedianMs),
				fmt.Sprintf("%.3f", res.Summary.P95Ms),
				fmt.Sprintf("%.3f", res.Summary.StddevMs),
				fmt.Sprintf("%.1f", res.Summary.SuccessRate),
				strconv.Itoa(res.Summary.FailureCount),
			}
			if err := w.Write(row); err != nil {
				return nil, err
			}
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
