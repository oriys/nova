#!/bin/bash
# test_all_runtimes.sh - Build and test hello functions for all supported runtimes.
#
# Supported (VM): python, go, rust, wasm, node, ruby, java, php, dotnet, deno, bun
#
# Prerequisites:
#   - nova daemon running (VM mode)
#   - Toolchains (optional, auto-skip if missing):
#       - Go (for go)
#       - Rust + musl target (for rust):  rustup target add x86_64-unknown-linux-musl
#       - Rust + WASI target (for wasm):  rustup target add wasm32-wasip1 (or wasm32-wasi)
#       - JDK (for java): javac + jar
#       - .NET SDK (for dotnet): dotnet

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/build"

echo "=========================================="
echo "  Building runtime fixtures"
echo "=========================================="

"${SCRIPT_DIR}/build_runtime_fixtures.sh"

register_or_update() {
  local name="$1"
  local runtime="$2"
  local code="$3"

  echo ""
  echo "--- Registering ${name} (${runtime}) ---"
  nova register "${name}" --runtime "${runtime}" --code "${code}" 2>/dev/null || \
    nova update "${name}" --code "${code}"
}

echo ""
echo "=========================================="
echo "  Registering functions"
echo "=========================================="

register_or_update "hello-python" "python" "${BUILD_DIR}/python/handler"
register_or_update "hello-go" "go" "${BUILD_DIR}/go/handler"
register_or_update "hello-node" "node" "${BUILD_DIR}/node/handler"
register_or_update "hello-ruby" "ruby" "${BUILD_DIR}/ruby/handler"
register_or_update "hello-php" "php" "${BUILD_DIR}/php/handler"
register_or_update "hello-deno" "deno" "${BUILD_DIR}/deno/handler"
register_or_update "hello-bun" "bun" "${BUILD_DIR}/bun/handler"

if [[ -f "${BUILD_DIR}/rust/handler" ]]; then
  register_or_update "hello-rust" "rust" "${BUILD_DIR}/rust/handler"
else
  echo ""
  echo "--- Skipping hello-rust (artifact missing) ---"
fi

if [[ -f "${BUILD_DIR}/wasm/handler" ]]; then
  register_or_update "hello-wasm" "wasm" "${BUILD_DIR}/wasm/handler"
else
  echo ""
  echo "--- Skipping hello-wasm (artifact missing) ---"
fi

if [[ -f "${BUILD_DIR}/java/handler" ]]; then
  register_or_update "hello-java" "java" "${BUILD_DIR}/java/handler"
else
  echo ""
  echo "--- Skipping hello-java (artifact missing) ---"
fi

if [[ -f "${BUILD_DIR}/dotnet/handler" ]]; then
  register_or_update "hello-dotnet" "dotnet" "${BUILD_DIR}/dotnet/handler"
else
  echo ""
  echo "--- Skipping hello-dotnet (artifact missing) ---"
fi

echo ""
echo "=========================================="
echo "  Testing (cold start)"
echo "=========================================="

nova invoke hello-python --payload '{"name": "Python"}'
nova invoke hello-go --payload '{"name": "Gopher"}'
nova invoke hello-node --payload '{"name": "Node"}'
nova invoke hello-ruby --payload '{"name": "Ruby"}'
nova invoke hello-php --payload '{"name": "PHP"}'
nova invoke hello-deno --payload '{"name": "Deno"}'
nova invoke hello-bun --payload '{"name": "Bun"}'

if nova get hello-rust &>/dev/null; then
  nova invoke hello-rust --payload '{"name": "Rustacean"}'
fi
if nova get hello-wasm &>/dev/null; then
  nova invoke hello-wasm --payload '{"name": "Wasm"}'
fi
if nova get hello-java &>/dev/null; then
  nova invoke hello-java --payload '{"name": "Java"}'
fi
if nova get hello-dotnet &>/dev/null; then
  nova invoke hello-dotnet --payload '{"name": ".NET"}'
fi

echo ""
echo "=========================================="
echo "  Warm reuse (3 rapid requests each)"
echo "=========================================="

for fn in hello-python hello-go hello-node hello-ruby hello-php hello-dotnet hello-deno hello-bun hello-rust hello-wasm hello-java; do
  if ! nova get "${fn}" &>/dev/null; then
    continue
  fi
  echo ""
  echo "--- ${fn} ---"
  for i in 1 2 3; do
    nova invoke "${fn}" --payload "{\"name\": \"${fn}-${i}\"}" >/dev/null
    echo "ok ${i}"
  done
done

echo ""
echo "=========================================="
echo "  All tests completed!"
echo "=========================================="
