#!/bin/bash
# Build Docker runtime images for Nova
# Usage: ./build-runtimes.sh [prefix]
# Default prefix: nova-runtime

set -e

PREFIX=${1:-nova-runtime}
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "Building nova-agent for linux/amd64..."
cd "$ROOT_DIR"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/nova-agent ./cmd/agent

echo ""
echo "Building runtime images with prefix: $PREFIX"
echo ""

RUNTIMES=(base python node ruby java php lua dotnet deno bun wasm libkrun)

for rt in "${RUNTIMES[@]}"; do
    echo "Building $PREFIX-$rt..."
    docker build -f "$SCRIPT_DIR/Dockerfile.$rt" -t "$PREFIX-$rt" "$ROOT_DIR"
done

echo ""
echo "Done! Built images:"
for rt in "${RUNTIMES[@]}"; do
    echo "  - $PREFIX-$rt"
done
