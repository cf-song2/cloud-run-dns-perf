# DNS Performance Test - Makefile
# Usage: make <command>

.PHONY: help deploy delete trigger urls logs status local clean

# Load GCP_PROJECT_ID from .env file
PROJECT := $(shell grep GCP_PROJECT_ID .env 2>/dev/null | cut -d= -f2)

help: ## Show this help
	@echo "DNS Performance Test - Available Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ===========================================
# Deployment
# ===========================================

deploy: ## Deploy all functions + schedulers
	./deploy.sh

deploy-functions: ## Deploy functions only (no scheduler)
	./deploy.sh deploy

deploy-scheduler: ## Setup schedulers only (functions must exist)
	./deploy.sh scheduler

delete: ## Delete all functions and schedulers
	./deploy.sh delete

# ===========================================
# Function Management
# ===========================================

urls: ## Show all function URLs
	@echo "=== Function URLs ==="
	@for region in $$(grep GCP_REGIONS .env | cut -d= -f2 | tr ',' ' '); do \
		url=$$(gcloud functions describe "dns-perf-test-$$region" \
			--region="$$region" \
			--project=$(PROJECT) \
			--gen2 \
			--format='value(serviceConfig.uri)' 2>/dev/null); \
		if [ -n "$$url" ]; then \
			echo "$$region: $$url"; \
		else \
			echo "$$region: NOT DEPLOYED"; \
		fi; \
	done

status: ## Show deployment status
	@echo "=== Function Status ==="
	@gcloud functions list --project=$(PROJECT) --filter="name~dns-perf-test" --format="table(name,state,region)" 2>/dev/null || echo "No functions found"
	@echo ""
	@echo "=== Scheduler Jobs ==="
	@gcloud scheduler jobs list --project=$(PROJECT) --format="table(name,state,schedule)" 2>/dev/null | grep dns-schedule || echo "No scheduler jobs found"

trigger: ## Trigger all functions manually
	@echo "=== Triggering all functions ==="
	@TOKEN=$$(gcloud auth print-identity-token) && \
	for region in $$(grep GCP_REGIONS .env | cut -d= -f2 | tr ',' ' '); do \
		url=$$(gcloud functions describe "dns-perf-test-$$region" \
			--region="$$region" \
			--project=$(PROJECT) \
			--gen2 \
			--format='value(serviceConfig.uri)' 2>/dev/null); \
		if [ -n "$$url" ]; then \
			echo "Triggering $$region..."; \
			curl -s -H "Authorization: Bearer $$TOKEN" "$$url" > /tmp/dns-result-$$region.json & \
		fi; \
	done; \
	wait; \
	echo ""; \
	echo "=== Results ==="; \
	for region in $$(grep GCP_REGIONS .env | cut -d= -f2 | tr ',' ' '); do \
		if [ -f "/tmp/dns-result-$$region.json" ]; then \
			test_id=$$(cat /tmp/dns-result-$$region.json 2>/dev/null | grep -o '"test_id":"[^"]*"' | head -1 | cut -d'"' -f4); \
			if [ -n "$$test_id" ]; then \
				echo "✅ $$region: $$test_id"; \
			else \
				echo "❌ $$region: failed"; \
			fi; \
		fi; \
	done

trigger-region: ## Trigger a single region (usage: make trigger-region REGION=us-central1)
	@if [ -z "$(REGION)" ]; then echo "Usage: make trigger-region REGION=us-central1"; exit 1; fi
	@echo "=== Triggering $(REGION) ==="
	@TOKEN=$$(gcloud auth print-identity-token) && \
	url=$$(gcloud functions describe "dns-perf-test-$(REGION)" \
		--region="$(REGION)" \
		--project=$(PROJECT) \
		--gen2 \
		--format='value(serviceConfig.uri)' 2>/dev/null) && \
	if [ -n "$$url" ]; then \
		curl -s -H "Authorization: Bearer $$TOKEN" "$$url" | head -50; \
	else \
		echo "Function not found in $(REGION)"; \
	fi

# ===========================================
# Logs
# ===========================================

logs: ## Show recent logs (usage: make logs REGION=us-central1)
	@if [ -z "$(REGION)" ]; then \
		echo "Usage: make logs REGION=us-central1"; \
		echo "Available regions:"; \
		grep GCP_REGIONS .env | cut -d= -f2 | tr ',' '\n' | sed 's/^/  /'; \
		exit 1; \
	fi
	gcloud functions logs read "dns-perf-test-$(REGION)" \
		--region="$(REGION)" \
		--project=$(PROJECT) \
		--gen2 \
		--limit=50

logs-all: ## Show logs from all regions (last 10 per region)
	@for region in $$(grep GCP_REGIONS .env | cut -d= -f2 | tr ',' ' '); do \
		echo "=== $$region ==="; \
		gcloud functions logs read "dns-perf-test-$$region" \
			--region="$$region" \
			--project=$(PROJECT) \
			--gen2 \
			--limit=10 2>/dev/null || echo "No logs"; \
		echo ""; \
	done

# ===========================================
# Local Development
# ===========================================

local: ## Run locally for testing
	cd cmd && go run main.go

test-local: ## Test local server (must be running)
	curl -s http://localhost:8080/RunDNSTest | head -50

# ===========================================
# Utilities
# ===========================================

clean: ## Clean up temporary files
	rm -f /tmp/dns-result-*.json
	rm -f .env.deploy.yaml
	rm -f cmd/dns-performance-test

check-env: ## Verify .env configuration
	@echo "=== Checking .env ==="
	@if [ ! -f .env ]; then echo "❌ .env file not found. Run: cp .env.example .env"; exit 1; fi
	@grep -q "your_" .env && echo "❌ .env contains placeholder values - update them" || echo "✅ .env looks configured"
	@echo ""
	@echo "=== Current Regions ==="
	@grep GCP_REGIONS .env | cut -d= -f2 | tr ',' '\n' | sed 's/^/  /'

auth: ## Authenticate with GCP
	gcloud auth login
	gcloud auth application-default login
