#!/bin/bash
# build_runtime_fixtures.sh - Build per-runtime "hello" artifacts for Nova.
#
# Output layout:
#   examples/build/<runtime>/handler
#
# Notes:
# - Go/Rust are built as Linux amd64 binaries (Go static, Rust musl).
# - Java builds a single runnable JAR (renamed to "handler").
# - WASM builds a WASI module (renamed to "handler").

set -euxo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/build"

mkdir -p "${BUILD_DIR}"/{python,go,rust,node,ruby,java,php,deno,bun,wasm}

echo "=========================================="
echo "  Preparing sources"
echo "=========================================="

cp "${SCRIPT_DIR}/hello.py" "${BUILD_DIR}/python/handler"
cp "${SCRIPT_DIR}/hello_node.js" "${BUILD_DIR}/node/handler"
cp "${SCRIPT_DIR}/hello_ruby.rb" "${BUILD_DIR}/ruby/handler"
cp "${SCRIPT_DIR}/hello_php.php" "${BUILD_DIR}/php/handler"
cp "${SCRIPT_DIR}/hello_deno.js" "${BUILD_DIR}/deno/handler"
cp "${SCRIPT_DIR}/hello_bun.js" "${BUILD_DIR}/bun/handler"

chmod +x "${BUILD_DIR}/python/handler" || true
chmod +x "${BUILD_DIR}/node/handler" || true
chmod +x "${BUILD_DIR}/ruby/handler" || true
chmod +x "${BUILD_DIR}/php/handler" || true
chmod +x "${BUILD_DIR}/deno/handler" || true
chmod +x "${BUILD_DIR}/bun/handler" || true

echo ""
echo "=========================================="
echo "  Building Go (linux/amd64, static)"
echo "=========================================="

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -ldflags="-s -w" -o "${BUILD_DIR}/go/handler" "${SCRIPT_DIR}/hello.go"
ls -lh "${BUILD_DIR}/go/handler"

echo ""
echo "=========================================="
echo "  Building Rust (linux/amd64, musl)"
echo "=========================================="

SKIP_RUST=0
if command -v cargo &>/dev/null; then
  if cargo build --release --target x86_64-unknown-linux-musl --bin hello-rust 2>/dev/null; then
    cp "${SCRIPT_DIR}/target/x86_64-unknown-linux-musl/release/hello-rust" "${BUILD_DIR}/rust/handler"
    ls -lh "${BUILD_DIR}/rust/handler"
  else
    echo "Rust build failed. Make sure you have:"
    echo "  rustup target add x86_64-unknown-linux-musl"
    SKIP_RUST=1
  fi
else
  echo "Rust not installed (cargo not found), skipping Rust build"
  SKIP_RUST=1
fi

echo ""
echo "=========================================="
echo "  Building Java (JAR)"
echo "=========================================="

SKIP_JAVA=0
if command -v javac &>/dev/null && command -v jar &>/dev/null; then
  JAVA_TMP="$(mktemp -d)"
  trap 'rm -rf "${JAVA_TMP}"' EXIT
  mkdir -p "${JAVA_TMP}/classes"
  javac -d "${JAVA_TMP}/classes" "${SCRIPT_DIR}/hello_java/Main.java"
  echo "Main-Class: Main" > "${JAVA_TMP}/manifest.mf"
  jar cfm "${BUILD_DIR}/java/handler" "${JAVA_TMP}/manifest.mf" -C "${JAVA_TMP}/classes" .
  ls -lh "${BUILD_DIR}/java/handler"
else
  echo "Java build tools not found (javac/jar), skipping Java build"
  SKIP_JAVA=1
fi

echo ""
echo "=========================================="
echo "  Building WASM (WASI)"
echo "=========================================="

SKIP_WASM=0
if command -v cargo &>/dev/null; then
  WASM_TARGET=""
  if cargo build --release --target wasm32-wasip1 --bin hello-wasm 2>/dev/null; then
    WASM_TARGET="wasm32-wasip1"
  elif cargo build --release --target wasm32-wasi --bin hello-wasm 2>/dev/null; then
    WASM_TARGET="wasm32-wasi"
  else
    echo "WASM build failed. Make sure you have a WASI target installed, e.g.:"
    echo "  rustup target add wasm32-wasip1    (new)"
    echo "  rustup target add wasm32-wasi      (legacy)"
    SKIP_WASM=1
  fi

  if [[ "${SKIP_WASM}" -eq 0 ]]; then
    cp "${SCRIPT_DIR}/target/${WASM_TARGET}/release/hello-wasm.wasm" "${BUILD_DIR}/wasm/handler"
    ls -lh "${BUILD_DIR}/wasm/handler"
  fi
else
  echo "Rust not installed (cargo not found), skipping WASM build"
  SKIP_WASM=1
fi

echo ""
echo "=========================================="
echo "  Build complete"
echo "=========================================="
echo "Artifacts in: ${BUILD_DIR}"
