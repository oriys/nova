#!/bin/bash
# test_all_runtimes.sh - Test hello functions for Python, Go, and Rust
#
# Prerequisites:
#   - Go installed
#   - Rust installed with musl target: rustup target add x86_64-unknown-linux-musl
#   - nova daemon running

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/build"

mkdir -p "${BUILD_DIR}"

echo "=========================================="
echo "  Building binaries"
echo "=========================================="

echo ""
echo "--- Building Go ---"
cd "${SCRIPT_DIR}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "${BUILD_DIR}/hello-go" hello.go
ls -lh "${BUILD_DIR}/hello-go"

echo ""
echo "--- Building Rust ---"
cd "${SCRIPT_DIR}"
if command -v cargo &> /dev/null; then
    cargo build --release --target x86_64-unknown-linux-musl --bin hello-rust 2>/dev/null || {
        echo "Rust build failed. Make sure you have:"
        echo "  rustup target add x86_64-unknown-linux-musl"
        echo "Skipping Rust tests."
        SKIP_RUST=1
    }
    if [ -z "$SKIP_RUST" ]; then
        cp target/x86_64-unknown-linux-musl/release/hello-rust "${BUILD_DIR}/"
        ls -lh "${BUILD_DIR}/hello-rust"
    fi
else
    echo "Rust not installed, skipping Rust tests"
    SKIP_RUST=1
fi

echo ""
echo "=========================================="
echo "  Registering functions"
echo "=========================================="

echo ""
echo "--- Registering Python ---"
nova register hello-python --runtime python --code "${SCRIPT_DIR}/hello.py" 2>/dev/null || \
    nova update hello-python --code "${SCRIPT_DIR}/hello.py"

echo ""
echo "--- Registering Go ---"
nova register hello-go --runtime go --code "${BUILD_DIR}/hello-go" 2>/dev/null || \
    nova update hello-go --code "${BUILD_DIR}/hello-go"

if [ -z "$SKIP_RUST" ]; then
    echo ""
    echo "--- Registering Rust ---"
    nova register hello-rust --runtime rust --code "${BUILD_DIR}/hello-rust" 2>/dev/null || \
        nova update hello-rust --code "${BUILD_DIR}/hello-rust"
fi

echo ""
echo "=========================================="
echo "  Testing cold starts"
echo "=========================================="

echo ""
echo "--- Python cold start ---"
nova invoke hello-python --payload '{"name": "Python"}'

echo ""
echo "--- Go cold start ---"
nova invoke hello-go --payload '{"name": "Gopher"}'

if [ -z "$SKIP_RUST" ]; then
    echo ""
    echo "--- Rust cold start ---"
    nova invoke hello-rust --payload '{"name": "Rustacean"}'
fi

echo ""
echo "=========================================="
echo "  Testing warm reuse (5 rapid requests)"
echo "=========================================="

echo ""
echo "--- Python warm test ---"
for i in {1..5}; do
    result=$(nova invoke hello-python --payload "{\"name\": \"Py${i}\"}" 2>&1)
    echo "$result" | grep -o '"cold_start":[^,}]*' || echo "$result"
done

echo ""
echo "--- Go warm test ---"
for i in {1..5}; do
    result=$(nova invoke hello-go --payload "{\"name\": \"Go${i}\"}" 2>&1)
    echo "$result" | grep -o '"cold_start":[^,}]*' || echo "$result"
done

if [ -z "$SKIP_RUST" ]; then
    echo ""
    echo "--- Rust warm test ---"
    for i in {1..5}; do
        result=$(nova invoke hello-rust --payload "{\"name\": \"Rs${i}\"}" 2>&1)
        echo "$result" | grep -o '"cold_start":[^,}]*' || echo "$result"
    done
fi

echo ""
echo "=========================================="
echo "  Performance comparison"
echo "=========================================="

echo ""
echo "--- Python (5 invocations) ---"
for i in {1..5}; do
    nova invoke hello-python --payload '{"name": "Benchmark"}' 2>&1 | grep -o '"duration_ms":[0-9]*'
done

echo ""
echo "--- Go (5 invocations) ---"
for i in {1..5}; do
    nova invoke hello-go --payload '{"name": "Benchmark"}' 2>&1 | grep -o '"duration_ms":[0-9]*'
done

if [ -z "$SKIP_RUST" ]; then
    echo ""
    echo "--- Rust (5 invocations) ---"
    for i in {1..5}; do
        nova invoke hello-rust --payload '{"name": "Benchmark"}' 2>&1 | grep -o '"duration_ms":[0-9]*'
    done
fi

echo ""
echo "=========================================="
echo "  All tests completed!"
echo "=========================================="
