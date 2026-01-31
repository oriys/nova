#!/bin/bash
# test_hello.sh - Test hello functions for all runtimes
# Run this on the Linux machine where nova daemon is running

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/build"

mkdir -p "${BUILD_DIR}"

echo "=== Building Go binary ==="
cd "${SCRIPT_DIR}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "${BUILD_DIR}/hello-go" hello.go
echo "Built: ${BUILD_DIR}/hello-go"

echo ""
echo "=== Registering functions ==="

# Python (source file)
nova register hello-python --runtime python --code "${SCRIPT_DIR}/hello.py"
echo "Registered: hello-python"

# Go (compiled binary)
nova register hello-go --runtime go --code "${BUILD_DIR}/hello-go"
echo "Registered: hello-go"

echo ""
echo "=== Testing functions ==="

echo ""
echo "--- Python ---"
nova invoke hello-python --payload '{"name": "Python"}'

echo ""
echo "--- Go ---"
nova invoke hello-go --payload '{"name": "Gopher"}'

echo ""
echo "=== Warm reuse test (5 rapid requests) ==="

echo ""
echo "--- Python warm test ---"
for i in {1..5}; do
    nova invoke hello-python --payload "{\"name\": \"Test${i}\"}"
done

echo ""
echo "--- Go warm test ---"
for i in {1..5}; do
    nova invoke hello-go --payload "{\"name\": \"Test${i}\"}"
done

echo ""
echo "=== Done ==="
