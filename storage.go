package dnsperformance

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
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

// UploadResults uploads JSON and CSV results to R2.
func UploadResults(ctx context.Context, r2cfg R2Config, run *TestRun) error {
	client, err := createR2Client(ctx, r2cfg)
	if err != nil {
		return fmt.Errorf("create R2 client: %w", err)
	}

	ts := run.Timestamp
	datePrefix := ts.Format("2006/01/02")
	fileBase := fmt.Sprintf("%s_%s", run.Region, ts.Format("2006-01-02T15-04-05Z"))

	// Upload JSON
	jsonKey := fmt.Sprintf("json/%s/%s.json", datePrefix, fileBase)
	jsonData, err := marshalJSON(run)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	if err := uploadToR2(ctx, client, r2cfg.BucketName, jsonKey, jsonData, "application/json"); err != nil {
		return fmt.Errorf("upload JSON: %w", err)
	}

	// Upload CSV
	csvKey := fmt.Sprintf("csv/%s/%s.csv", datePrefix, fileBase)
	csvData, err := marshalCSV(run)
	if err != nil {
		return fmt.Errorf("marshal CSV: %w", err)
	}
	if err := uploadToR2(ctx, client, r2cfg.BucketName, csvKey, csvData, "text/csv"); err != nil {
		return fmt.Errorf("upload CSV: %w", err)
	}

	return nil
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

// marshalCSV serializes the test run to CSV format.
// Each row represents a single query iteration for easy analysis.
func marshalCSV(run *TestRun) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Write header
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
