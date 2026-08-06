#!/usr/bin/env bash

set -euo pipefail

echo "==> Formatting check..."
gofmt -w .

echo "==> go vet..."
go vet ./...

echo "==> go test..."
go test ./...

echo
echo "All checks passed!"
