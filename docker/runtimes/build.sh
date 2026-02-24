#!/bin/bash
# Build Docker runtime images for Nova
# Usage: ./build.sh [prefix] [runtime...]
# Default prefix: nova-runtime

set -euxo pipefail

PREFIX=${1:-nova-runtime}
shift || true

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

if [ "$#" -gt 0 ]; then
    RUNTIMES=("$@")
else
    RUNTIMES=(base python node ruby java php lua deno bun wasm)
fi

HOST_ARCH="$(uname -m)"
case "$HOST_ARCH" in
    x86_64|amd64) DEFAULT_BUILD_PLATFORM="linux/amd64" ;;
    arm64|aarch64) DEFAULT_BUILD_PLATFORM="linux/arm64" ;;
    *) DEFAULT_BUILD_PLATFORM="linux/amd64" ;;
esac
BUILD_PLATFORM="${NOVA_BUILD_PLATFORM:-$DEFAULT_BUILD_PLATFORM}"

echo "Building nova-agent for linux/amd64 in Docker..."
docker run --rm \
    --platform "$BUILD_PLATFORM" \
    -u "$(id -u):$(id -g)" \
    -v "$ROOT_DIR:/src" \
    -w /src \
    golang:1.24-alpine \
    sh -c 'mkdir -p /tmp/go-cache /tmp/go-mod && GOCACHE=/tmp/go-cache GOMODCACHE=/tmp/go-mod CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/nova-agent ./cmd/agent'

echo ""
echo "Building runtime images with prefix: $PREFIX"
echo ""

for rt in "${RUNTIMES[@]}"; do
    echo "Building $PREFIX-$rt..."
    docker build -f "$SCRIPT_DIR/Dockerfile.$rt" -t "$PREFIX-$rt" "$ROOT_DIR"
done

echo ""
echo "Done! Built images:"
for rt in "${RUNTIMES[@]}"; do
    echo "  - $PREFIX-$rt"
done
