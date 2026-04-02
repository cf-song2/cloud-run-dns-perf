# DNS Performance Test

A serverless DNS performance testing tool that measures DNS query latency from multiple global regions. Deployed on Google Cloud Functions and stores results in Cloudflare R2.

## Features

- **Multi-region testing**: Deploy to 8+ GCP regions for global DNS performance visibility
- **Multiple DNS providers**: Test against Cloudflare, Google, OpenDNS, Quad9, and custom DNS servers
- **Automated scheduling**: Cloud Scheduler triggers tests every 6 hours (configurable)
- **Detailed metrics**: RTT, P95, success rate, standard deviation per server/domain pair
- **Dual output format**: Results stored as both JSON and CSV in Cloudflare R2
- **Cost-effective**: Runs within GCP free tier (~$0.50/month for scheduler)

## Architecture

```
┌─────────────────────┐     OIDC Auth      ┌─────────────────────────────┐
│   Cloud Scheduler   │ ─────────────────▶ │   Cloud Functions (8 regions)│
│   (every 6 hours)   │                    │   - Query DNS servers        │
└─────────────────────┘                    │   - Measure latency          │
                                           │   - Calculate statistics     │
                                           └─────────────────────────────┘
                                                         │
                                                         ▼ S3 API
                                           ┌─────────────────────────────┐
                                           │   Cloudflare R2 Bucket      │
                                           │   json/YYYY/MM/DD/...json   │
                                           │   csv/YYYY/MM/DD/...csv     │
                                           └─────────────────────────────┘
```

## Prerequisites

- [Google Cloud CLI](https://cloud.google.com/sdk/docs/install) installed and authenticated
- GCP project with billing enabled
- Cloudflare account with R2 bucket created
- Go 1.22+ (for local testing only)

## Quick Start

### 1. Clone and Configure

```bash
git clone https://github.com/YOUR_USERNAME/dns-performance-test.git
cd dns-performance-test

# Copy example config and fill in your values
cp .env.example .env
```

### 2. Edit `.env`

```env
# Cloudflare R2 (required)
R2_ACCOUNT_ID=your_cloudflare_account_id
R2_ACCESS_KEY_ID=your_r2_access_key_id
R2_SECRET_ACCESS_KEY=your_r2_secret_access_key
R2_BUCKET_NAME=your-bucket-name

# Test Configuration (required)
TEST_ITERATIONS=10
DNS_TIMEOUT_SEC=5
DNS_SERVERS=Cloudflare=1.1.1.1,Google=8.8.8.8,OpenDNS=208.67.222.222,Quad9=9.9.9.9
DNS_DOMAINS=cloudflare.com,google.com,your-domain.com

# GCP (required)
GCP_PROJECT_ID=your-gcp-project-id
GCP_REGIONS=us-central1,europe-west1,asia-northeast3
```

### 3. Deploy

```bash
chmod +x deploy.sh
./deploy.sh
```

## Deployment Commands

| Command | Description |
|---------|-------------|
| `./deploy.sh` | Deploy functions + setup scheduler (full deployment) |
| `./deploy.sh deploy` | Deploy functions only |
| `./deploy.sh scheduler` | Setup scheduler only (functions must exist) |
| `./deploy.sh delete` | Delete all functions and scheduler jobs |

## Local Development

```bash
# Install dependencies
go mod download

# Run locally
cd cmd
go run main.go

# Test (in another terminal)
curl http://localhost:8080/RunDNSTest
```

## Configuration Reference

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `R2_ACCOUNT_ID` | Cloudflare account ID |
| `R2_ACCESS_KEY_ID` | R2 API access key |
| `R2_SECRET_ACCESS_KEY` | R2 API secret key |
| `R2_BUCKET_NAME` | R2 bucket name for results |
| `DNS_SERVERS` | Comma-separated `Name=IP` pairs |
| `DNS_DOMAINS` | Comma-separated domains to test |
| `DNS_TIMEOUT_SEC` | Timeout for each DNS query |
| `GCP_PROJECT_ID` | Google Cloud project ID |
| `GCP_REGIONS` | Comma-separated GCP regions (see supported list below) |

### Supported GCP Regions

Only regions that support **both** Cloud Functions and Cloud Scheduler:

| Region | Location |
|--------|----------|
| us-central1 | Iowa, USA |
| us-east1 | South Carolina, USA |
| us-west1 | Oregon, USA |
| europe-west1 | Belgium |
| europe-west2 | London |
| asia-northeast1 | Tokyo |
| asia-northeast3 | Seoul |
| asia-south1 | Mumbai |
| asia-southeast1 | Singapore |
| australia-southeast1 | Sydney |
| southamerica-east1 | São Paulo |

**Not Supported:** `africa-south1` (Cloud Scheduler not available)

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SCHEDULE_CRON` | `0 */6 * * *` | Cron schedule (UTC) |
| `FUNCTION_MEMORY` | `256Mi` | Memory allocation |
| `FUNCTION_TIMEOUT` | `300` | Timeout in seconds |
| `FUNCTION_MAX_INSTANCES` | `3` | Max concurrent instances |
| `GO_RUNTIME` | `go122` | Go runtime version |

## Output Format

### JSON Structure

```json
{
  "test_id": "20260402-140000-abc1",
  "region": "us-central1",
  "timestamp": "2026-04-02T14:00:00Z",
  "results": [
    {
      "dns_server": "1.1.1.1",
      "dns_provider": "Cloudflare",
      "domain": "google.com",
      "rtt_ms": 12.345,
      "success": true,
      "rcode": "NOERROR",
      "answer_count": 2,
      "resolved_ips": ["142.250.80.46", "142.250.80.47"]
    }
  ]
}
```

### R2 File Structure

```
your-bucket/
├── csv/
│   ├── us-central1/
│   │   ├── 2026-04-02.csv    ← All results for that day (appended)
│   │   └── 2026-04-03.csv
│   └── europe-west1/
│       └── 2026-04-02.csv
└── json/
    └── us-central1/
        └── 2026-04-02/
            ├── 2026-04-02T14-00-00Z.json   ← Individual test runs
            └── 2026-04-02T14-10-00Z.json
```

### CSV Columns

| Column | Description |
|--------|-------------|
| test_id | Unique test run identifier |
| region | GCP region where test ran |
| timestamp | ISO 8601 timestamp |
| dns_server | DNS server IP |
| dns_provider | DNS provider name |
| domain | Domain that was queried |
| rtt_ms | Round-trip time in milliseconds |
| success | true/false |
| rcode | DNS response code (NOERROR, TIMEOUT, etc.) |
| answer_count | Number of DNS answers |
| resolved_ips | Resolved IP addresses |
| error | Error message (if any) |

## Recommended DNS Servers

| Provider | IP | Description |
|----------|-----|-------------|
| Cloudflare | 1.1.1.1 | Fast, privacy-focused |
| Cloudflare Secondary | 1.0.0.1 | Cloudflare backup |
| Google | 8.8.8.8 | Google Public DNS |
| Google Secondary | 8.8.4.4 | Google backup |
| OpenDNS | 208.67.222.222 | Cisco Umbrella |
| Quad9 | 9.9.9.9 | Security-focused |

## Cost Estimate

| Component | Monthly Cost |
|-----------|--------------|
| Cloud Functions | ~$0.00 (free tier) |
| Cloud Scheduler | ~$0.50 (8 jobs) |
| Cloudflare R2 | ~$0.00 (free tier) |
| **Total** | **~$0.50/month** |

## Troubleshooting

### Permission Denied on Deploy

Your GCP account needs these roles:
- Cloud Functions Developer
- Service Account User
- Cloud Scheduler Admin

If you can't enable APIs, ask your GCP admin or ensure APIs are pre-enabled.

### Organization Policy Blocks Public Access

The script uses `--no-allow-unauthenticated` by default. Scheduler uses OIDC tokens for authentication.

### Manual Trigger

```bash
# Get auth token
TOKEN=$(gcloud auth print-identity-token)

# Trigger a function
curl -H "Authorization: Bearer $TOKEN" \
  "$(gcloud functions describe dns-perf-test-us-central1 \
     --region=us-central1 \
     --project=YOUR_PROJECT \
     --gen2 \
     --format='value(serviceConfig.uri)')"
```

## License

MIT License
