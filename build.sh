#!/usr/bin/env bash
set -euo pipefail

echo "==> Running tests..."
if go test ./...; then
    echo "==> Tests passed. Building splunk-tui..."
    go build -o splunk-tui .
    echo "==> Build successful: ./splunk-tui"
else
    echo "==> Tests failed. Build aborted."
    exit 1
fi
