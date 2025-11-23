#!/bin/bash

# Test script to send a POST request to the webhook endpoint
# This simulates a GitHub push event
#
# Usage:
#   make test
#   # Or with custom webhook secret (if different from config.yml):
#   WEBHOOK_SECRET=your_actual_secret make test
#   # Or with custom URL:
#   WEBHOOK_URL=https://your-server.com/webhook WEBHOOK_SECRET=secret make test

WEBHOOK_URL="${WEBHOOK_URL:-https://console.allinmedia.ai/tool/github-sentry/webhook}"

# Try to read webhook secret from config.yml if not set as environment variable
if [ -z "$WEBHOOK_SECRET" ] && [ -f "config.yml" ]; then
  WEBHOOK_SECRET=$(grep "^github_webhook_secret:" config.yml | sed 's/^github_webhook_secret:[[:space:]]*//' | tr -d '\r\n' | sed 's/[[:space:]]*$//')
fi

# Fallback to default if still not set
WEBHOOK_SECRET="${WEBHOOK_SECRET:-your_secret}"

# Create temporary file for payload
TMP_PAYLOAD=$(mktemp)
trap "rm -f $TMP_PAYLOAD" EXIT

# Sample GitHub push event payload
# This matches the staging branch configured in config.yml
# Use printf to avoid trailing newline issues
printf '{
  "ref": "refs/heads/staging",
  "head_commit": {
    "id": "abc123def456",
    "message": "Test commit message from make test",
    "timestamp": "2024-01-15T10:30:00Z",
    "author": {
      "name": "Test User",
      "email": "test@example.com",
      "username": "testuser"
    },
    "committer": {
      "name": "Test User",
      "email": "test@example.com",
      "username": "testuser"
    }
  },
  "repository": {
    "full_name": "test/repo",
    "name": "repo"
  },
  "pusher": {
    "name": "Test User",
    "email": "test@example.com"
  }
}' > "$TMP_PAYLOAD"

# Generate GitHub webhook signature from the exact file content
# GitHub uses HMAC-SHA256 and sends it as sha256=<hex_digest>
# Use -binary flag and then convert to hex, or just use default hex output
SIGNATURE=$(openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" "$TMP_PAYLOAD" | sed 's/^.*= //')
SIGNATURE_HEADER="sha256=$SIGNATURE"

echo "Testing webhook endpoint: $WEBHOOK_URL"
if [ "$WEBHOOK_SECRET" = "your_secret" ]; then
  echo "⚠️  WARNING: Using default placeholder secret 'your_secret'"
  echo "   If the server uses a different secret, set WEBHOOK_SECRET environment variable:"
  echo "   WEBHOOK_SECRET=actual_secret make test"
  echo ""
fi
echo "Using webhook secret: ${WEBHOOK_SECRET:0:4}..."
echo "Signature: $SIGNATURE_HEADER"
echo "Payload size: $(wc -c < "$TMP_PAYLOAD") bytes"
echo ""

# Generate a delivery ID (GitHub includes this in real webhooks)
DELIVERY_ID=$(uuidgen 2>/dev/null || echo "$(date +%s)-$(openssl rand -hex 8)")

# Send POST request with proper headers using --data-binary to send exact bytes
# Use -v for verbose output if DEBUG is set
if [ -n "$DEBUG" ]; then
  echo "Debug mode - showing request details:"
  echo "  URL: $WEBHOOK_URL"
  echo "  Secret: ${WEBHOOK_SECRET:0:4}..."
  echo "  Signature header: $SIGNATURE_HEADER"
  echo "  Payload preview (first 200 chars):"
  head -c 200 "$TMP_PAYLOAD"
  echo ""
  echo ""
fi

RESPONSE=$(curl ${DEBUG:+-v} -s -w "\n%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: push" \
  -H "X-GitHub-Delivery: $DELIVERY_ID" \
  -H "X-Hub-Signature-256: $SIGNATURE_HEADER" \
  --data-binary "@$TMP_PAYLOAD" \
  "$WEBHOOK_URL")

# Extract HTTP status code (last line) and body (everything else)
HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo "Response:"
echo "  HTTP Status: $HTTP_CODE"
echo "  Body: $BODY"
echo ""

if [ "$HTTP_CODE" -ge 200 ] && [ "$HTTP_CODE" -lt 300 ]; then
  echo "✅ Webhook test successful!"
  exit 0
else
  echo "❌ Webhook test failed with status $HTTP_CODE"
  exit 1
fi

