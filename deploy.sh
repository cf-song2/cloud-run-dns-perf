#!/usr/bin/env bash
#
# deploy.sh — Deploy DNS performance test to multiple GCP regions
#              and optionally set up Cloud Scheduler for automated runs.
#
# Usage:
#   ./deploy.sh                  # Deploy to all regions + set up scheduler
#   ./deploy.sh deploy           # Deploy functions only (no scheduler)
#   ./deploy.sh scheduler        # Set up scheduler only (functions must exist)
#   ./deploy.sh delete           # Delete all functions and scheduler jobs
#
# Prerequisites:
#   - gcloud CLI installed and authenticated
#   - .env file in the same directory (copy from .env.example)
#   - Required GCP APIs enabled (see below)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.env"

# ─── Load .env ───────────────────────────────────────────────────────

if [[ ! -f "$ENV_FILE" ]]; then
    echo "ERROR: .env file not found at $ENV_FILE"
    echo "  Copy .env.example to .env and fill in your values:"
    echo "  cp .env.example .env"
    exit 1
fi

# Source .env (skip comments and empty lines)
set -a
while IFS='=' read -r key value; do
    key=$(echo "$key" | xargs)
    [[ -z "$key" || "$key" == \#* ]] && continue
    value=$(echo "$value" | xargs | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//")
    export "$key"="$value"
done < "$ENV_FILE"
set +a

# ─── Validate required variables ────────────────────────────────────

: "${GCP_PROJECT_ID:?GCP_PROJECT_ID is required in .env}"
: "${R2_ACCOUNT_ID:?R2_ACCOUNT_ID is required in .env}"
: "${R2_ACCESS_KEY_ID:?R2_ACCESS_KEY_ID is required in .env}"
: "${R2_SECRET_ACCESS_KEY:?R2_SECRET_ACCESS_KEY is required in .env}"
: "${R2_BUCKET_NAME:?R2_BUCKET_NAME is required in .env}"
: "${GCP_REGIONS:?GCP_REGIONS is required in .env}"
: "${DNS_SERVERS:?DNS_SERVERS is required in .env}"
: "${DNS_DOMAINS:?DNS_DOMAINS is required in .env}"
: "${DNS_TIMEOUT_SEC:?DNS_TIMEOUT_SEC is required in .env}"

# Defaults (only for deployment settings, not test config)
FUNCTION_MEMORY="${FUNCTION_MEMORY:-256Mi}"
FUNCTION_TIMEOUT="${FUNCTION_TIMEOUT:-300}"
FUNCTION_MAX_INSTANCES="${FUNCTION_MAX_INSTANCES:-3}"
GO_RUNTIME="${GO_RUNTIME:-go122}"
SCHEDULE_CRON="${SCHEDULE_CRON:-0 */6 * * *}"
SCHEDULER_SA_NAME="${SCHEDULER_SA_NAME:-dns-perf-scheduler-sa}"

FUNCTION_NAME="dns-perf-test"
SCHEDULER_SA_EMAIL="${SCHEDULER_SA_NAME}@${GCP_PROJECT_ID}.iam.gserviceaccount.com"

# Parse regions into array
IFS=',' read -ra REGIONS <<< "$GCP_REGIONS"

# ─── Helper functions ────────────────────────────────────────────────

log() { echo "[$(date '+%H:%M:%S')] $*"; }

enable_apis() {
    log "Enabling required GCP APIs..."
    gcloud services enable \
        artifactregistry.googleapis.com \
        cloudbuild.googleapis.com \
        run.googleapis.com \
        logging.googleapis.com \
        cloudscheduler.googleapis.com \
        --project="$GCP_PROJECT_ID" \
        --quiet
    log "APIs enabled"
}

create_scheduler_sa() {
    log "Creating scheduler service account: $SCHEDULER_SA_EMAIL"

    # Create SA if it doesn't exist
    if ! gcloud iam service-accounts describe "$SCHEDULER_SA_EMAIL" \
        --project="$GCP_PROJECT_ID" &>/dev/null; then
        gcloud iam service-accounts create "$SCHEDULER_SA_NAME" \
            --display-name="DNS Performance Test Scheduler" \
            --project="$GCP_PROJECT_ID"
        log "Service account created"
    else
        log "Service account already exists"
    fi
}

deploy_function() {
    local region="$1"
    local service_name="${FUNCTION_NAME}-${region}"

    log "Deploying ${service_name} to ${region}..."

    # Use Cloud Functions 2nd gen (uses existing gcf-artifacts repo)
    # Note: --no-allow-unauthenticated due to org policy; scheduler uses OIDC auth
    gcloud functions deploy "$service_name" \
        --gen2 \
        --runtime="${GO_RUNTIME}" \
        --region="$region" \
        --source="$SCRIPT_DIR" \
        --entry-point=RunDNSTest \
        --trigger-http \
        --no-allow-unauthenticated \
        --memory="${FUNCTION_MEMORY}" \
        --timeout="${FUNCTION_TIMEOUT}s" \
        --max-instances="${FUNCTION_MAX_INSTANCES}" \
        --set-env-vars="R2_ACCOUNT_ID=${R2_ACCOUNT_ID}" \
        --set-env-vars="R2_ACCESS_KEY_ID=${R2_ACCESS_KEY_ID}" \
        --set-env-vars="R2_SECRET_ACCESS_KEY=${R2_SECRET_ACCESS_KEY}" \
        --set-env-vars="R2_BUCKET_NAME=${R2_BUCKET_NAME}" \
        --set-env-vars="TEST_REGION=${region}" \
        --set-env-vars="DNS_TIMEOUT_SEC=${DNS_TIMEOUT_SEC}" \
        --set-env-vars="^@^DNS_SERVERS=${DNS_SERVERS}" \
        --set-env-vars="^@^DNS_DOMAINS=${DNS_DOMAINS}" \
        --project="$GCP_PROJECT_ID" \
        --quiet

    log "Deployed ${service_name}"
}

setup_scheduler() {
    local region="$1"
    local service_name="${FUNCTION_NAME}-${region}"
    local job_name="dns-schedule-${region}"

    # Get the function URL (Cloud Functions 2nd gen)
    local service_url
    service_url=$(gcloud functions describe "$service_name" \
        --region "$region" \
        --project "$GCP_PROJECT_ID" \
        --gen2 \
        --format='value(serviceConfig.uri)')

    if [[ -z "$service_url" ]]; then
        log "ERROR: Could not get URL for ${service_name} in ${region}"
        return 1
    fi

    log "Setting up scheduler: ${job_name} -> ${service_url}"

    # Delete existing job if present
    gcloud scheduler jobs delete "$job_name" \
        --location "$region" \
        --project "$GCP_PROJECT_ID" \
        --quiet 2>/dev/null || true

    # Grant invoker role to scheduler SA for the underlying Cloud Run service
    gcloud functions add-invoker-policy-binding "$service_name" \
        --region="$region" \
        --project="$GCP_PROJECT_ID" \
        --member="serviceAccount:${SCHEDULER_SA_EMAIL}" \
        --gen2 \
        --quiet 2>/dev/null || true

    # Create scheduler job
    gcloud scheduler jobs create http "$job_name" \
        --schedule="$SCHEDULE_CRON" \
        --http-method=POST \
        --uri="$service_url" \
        --oidc-service-account-email="$SCHEDULER_SA_EMAIL" \
        --oidc-token-audience="$service_url" \
        --location="$region" \
        --project="$GCP_PROJECT_ID" \
        --time-zone="UTC" \
        --quiet

    log "Scheduler created: ${job_name}"
}

delete_all() {
    for region in "${REGIONS[@]}"; do
        region=$(echo "$region" | xargs)
        local service_name="${FUNCTION_NAME}-${region}"
        local job_name="dns-schedule-${region}"

        log "Deleting scheduler job: ${job_name}"
        gcloud scheduler jobs delete "$job_name" \
            --location "$region" \
            --project "$GCP_PROJECT_ID" \
            --quiet 2>/dev/null || true

        log "Deleting function: ${service_name}"
        gcloud functions delete "$service_name" \
            --region "$region" \
            --project "$GCP_PROJECT_ID" \
            --gen2 \
            --quiet 2>/dev/null || true
    done
    log "All resources deleted"
}

# ─── Main ────────────────────────────────────────────────────────────

ACTION="${1:-all}"

echo "============================================="
echo " DNS Performance Test — Deployment"
echo "============================================="
echo " Project:    $GCP_PROJECT_ID"
echo " Regions:    ${REGIONS[*]}"
echo " Schedule:   $SCHEDULE_CRON (UTC)"
echo " Action:     $ACTION"
echo "============================================="
echo ""

case "$ACTION" in
    deploy)
        # enable_apis  # Skipped - APIs already enabled
        for region in "${REGIONS[@]}"; do
            region=$(echo "$region" | xargs)
            deploy_function "$region"
        done
        ;;
    scheduler)
        create_scheduler_sa
        for region in "${REGIONS[@]}"; do
            region=$(echo "$region" | xargs)
            setup_scheduler "$region"
        done
        ;;
    delete)
        delete_all
        ;;
    all)
        # enable_apis  # Skipped - APIs already enabled
        create_scheduler_sa
        for region in "${REGIONS[@]}"; do
            region=$(echo "$region" | xargs)
            deploy_function "$region"
            setup_scheduler "$region"
        done
        ;;
    *)
        echo "Usage: $0 [deploy|scheduler|delete|all]"
        exit 1
        ;;
esac

echo ""
log "Done!"
