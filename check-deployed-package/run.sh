#!/bin/sh
set -e

# The app the check program sends as, so it shows up as its own app on the shared domain
APP=go-sdk

# Generate random 5-character seed
LOGS_SEED=$(openssl rand -hex 2 | cut -c1-5)
echo "Generated seed: $LOGS_SEED"

# Generate random metrics seed (1-1,000,000)
METRICS_SEED=$(awk 'BEGIN{srand(); print int(rand()*1000000)+1}')
echo "Generated metrics seed: $METRICS_SEED"

echo "Building LogDash demo Docker image (using published package ${LOGDASH_VERSION:-latest})..."
docker build --no-cache --build-arg LOGDASH_VERSION="${LOGDASH_VERSION}" -t logdash-go-demo -f check-deployed-package/Dockerfile .

echo
echo "Running LogDash demo..."
echo

# Run in non-interactive mode which works everywhere
docker run --rm \
  -e LOGDASH_API_KEY="${LOGDASH_API_KEY}" \
  -e LOGS_SEED="${LOGS_SEED}" \
  -e METRICS_SEED="${METRICS_SEED}" \
  logdash-go-demo

echo
echo "Demo completed!"

echo
echo "Authenticating with LogDash API..."

# Identify the domain via the read-only personal API key (whoami).
# The ingest key (LOGDASH_API_KEY) is write-only and is no longer exchangeable for a token.
TOKEN="${LOGDASH_PERSONAL_API_KEY}"
AUTH_RESPONSE=$(curl -s -X 'GET' \
  'https://api.logdash.io/personal-api-keys/whoami' \
  -H 'accept: application/json' \
  -H "Authorization: Bearer ${TOKEN}")

# The key is restricted to the smoke-test domain; whoami returns it as access: { kind: "clusters", ids: [domainId] }.
DOMAIN_ID=$(echo "$AUTH_RESPONSE" | jq -r 'if .access.kind == "clusters" then .access.ids[0] // empty else empty end' 2>/dev/null || true)

if [ -z "$TOKEN" ] || [ -z "$DOMAIN_ID" ]; then
    echo "Error: LOGDASH_PERSONAL_API_KEY must be a personal API key (ldp_) with Logs read and Metrics read, restricted to the \"SDK smoke tests\" domain"
    echo "whoami access: $(echo "$AUTH_RESPONSE" | jq -c '.access // .message // "no access field"' 2>/dev/null || echo 'not JSON')"
    exit 1
fi

echo "Authentication successful. Domain ID: $DOMAIN_ID"

echo
echo "Waiting for logs to be available..."
sleep 3

echo
echo "Fetching logs from LogDash API..."

# Fetch this run's go-sdk logs; other SDKs and runs write to the same domain
LOGS_RESPONSE=$(curl -s -X 'GET' \
  "https://api.logdash.io/clusters/${DOMAIN_ID}/logs?apps=${APP}&limit=100&searchString=${LOGS_SEED}" \
  -H 'accept: application/json' \
  -H "Authorization: Bearer ${TOKEN}")

echo "Logs fetched successfully"

echo
echo "Validating log messages..."

# Expected log messages with seed
EXPECTED_MESSAGES="This is an info log ${LOGS_SEED}
This is an error log ${LOGS_SEED}
This is a warning log ${LOGS_SEED}
This is a debug log ${LOGS_SEED}
This is a http log ${LOGS_SEED}
This is a silly log ${LOGS_SEED}
This is an info log ${LOGS_SEED}
This is a verbose log ${LOGS_SEED}"

# Check if all expected messages are present in the logs
echo "$EXPECTED_MESSAGES" | while IFS= read -r expected_msg; do
    if ! echo "$LOGS_RESPONSE" | grep -q "$expected_msg"; then
        echo "Error: Expected log message not found: '$expected_msg'"
        echo "Logs response: $LOGS_RESPONSE"
        exit 1
    fi
    echo "✓ Found: '$expected_msg'"
done

echo
echo "Fetching metrics from LogDash API..."

# Fetch the domain's metrics; each entry has the app that reports it
METRICS_RESPONSE=$(curl -s -X 'GET' \
  "https://api.logdash.io/clusters/${DOMAIN_ID}/metrics" \
  -H 'accept: application/json' \
  -H "Authorization: Bearer ${TOKEN}")

echo "Metrics fetched successfully"

echo
echo "Validating metrics..."

# Expected users metric value (metrics_seed + 1)
EXPECTED_USERS_VALUE=$((METRICS_SEED + 1))

# Find the users metric of this app and check its value
USERS_VALUE=$(echo "$METRICS_RESPONSE" | jq -r --arg app "$APP" '.[] | select(.app == $app and .name == "users") | .value' 2>/dev/null | head -n 1)

if [ -z "$USERS_VALUE" ]; then
    echo "Error: Users metric of app $APP not found"
    echo "Metrics response: $METRICS_RESPONSE"
    exit 1
fi

if [ "$(jq -n --argjson a "$USERS_VALUE" --argjson b "$EXPECTED_USERS_VALUE" '$a == $b')" != "true" ]; then
    echo "Error: Users metric value mismatch. Expected: $EXPECTED_USERS_VALUE, Found: $USERS_VALUE"
    echo "Metrics response: $METRICS_RESPONSE"
    exit 1
fi

echo "✓ Found users metric with correct value: $USERS_VALUE"

echo
echo "All expected log messages and metrics found successfully!"
echo "Validation completed!"
