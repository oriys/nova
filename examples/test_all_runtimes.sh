#!/bin/bash
# test_all_runtimes.sh - Seed and test functions for all supported runtimes.
#
# Uses control-plane compilation (Docker toolchains), so no local Go/Rust/JDK/GCC
# installation is required.

set -euxo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
API_URL="${NOVA_API_URL:-http://localhost:9000}"

invoke_with_retry() {
  local name="$1"
  local payload="$2"
  local retries="${3:-90}"

  echo ""
  echo "--- Invoking ${name} ---"

  for attempt in $(seq 1 "${retries}"); do
    local resp
    resp="$(curl -sS -X POST "${API_URL}/functions/${name}/invoke" \
      -H "Content-Type: application/json" \
      -d "${payload}")"
    local err=""
    err="$(echo "${resp}" | jq -r '.error // empty' 2>/dev/null || true)"

    if [ -n "${err}" ] && echo "${err}" | grep -q "still compiling"; then
      sleep 2
      continue
    fi

    echo "${resp}" | jq .
    if [ -n "${err}" ]; then
      return 1
    fi
    return 0
  done

  echo "timed out waiting for compilation: ${name}"
  return 1
}

echo "=========================================="
echo "  Seeding Runtime Functions"
echo "=========================================="
SKIP_WORKFLOWS=1 "${SCRIPT_DIR}/../scripts/seed-functions.sh" "${API_URL}"

echo ""
echo "=========================================="
echo "  Testing (Simple + Complex)"
echo "=========================================="

invoke_with_retry "hello-python" '{"name":"Python"}'
invoke_with_retry "fibonacci" '{"n":10}'
invoke_with_retry "hello-node" '{"name":"Node"}'
invoke_with_retry "json-transform" '{"operation":"keys","data":{"a":1,"b":2}}'
invoke_with_retry "hello-go" '{"name":"Gopher"}'
invoke_with_retry "sum-array-go" '{"numbers":[1,2,3,4]}'
invoke_with_retry "hello-rust" '{"name":"Rustacean"}'
invoke_with_retry "number-stats-rust" '{"numbers":[1,2,3,4]}'
invoke_with_retry "hello-java" '{"name":"Java"}'
invoke_with_retry "number-stats-java" '{"numbers":[10,20,30]}'
invoke_with_retry "hello-kotlin" '{"name":"Kotlin"}'
invoke_with_retry "word-stats-kotlin" '{"text":"Nova nova seed runtime"}'
invoke_with_retry "hello-scala" '{"name":"Scala"}'
invoke_with_retry "number-stats-scala" '{"numbers":[1,2,3.5]}'
invoke_with_retry "hello-c" '{"name":"C"}'
invoke_with_retry "payload-stats-c" '{"payload":{"n":123}}'
invoke_with_retry "hello-cpp" '{"name":"Cpp"}'
invoke_with_retry "number-stats-cpp" '{"numbers":[2,4,8]}'
invoke_with_retry "hello-ruby" '{"name":"Ruby"}'
invoke_with_retry "word-count" '{"text":"nova runtime seed seed"}'
invoke_with_retry "hello-php" '{"name":"PHP"}'
invoke_with_retry "array-stats" '{"numbers":[3,6,9]}'
invoke_with_retry "hello-deno" '{"name":"Deno"}'
invoke_with_retry "base64-codec" '{"operation":"encode","data":"nova"}'
invoke_with_retry "hello-bun" '{"name":"Bun"}'
invoke_with_retry "hash-generator" '{"data":"nova","algorithm":"sha256"}'

echo ""
echo "=========================================="
echo "  Warm reuse (3 rapid requests each)"
echo "=========================================="

for fn in hello-python hello-node hello-go hello-rust hello-java hello-kotlin hello-scala hello-c hello-cpp hello-ruby hello-php hello-deno hello-bun; do
  echo ""
  echo "--- ${fn} ---"
  for run_idx in 1 2 3; do
    invoke_with_retry "${fn}" "{\"name\":\"${fn}-${run_idx}\"}" >/dev/null
    echo "ok ${run_idx}"
  done
done

echo ""
echo "=========================================="
echo "  All tests completed!"
echo "=========================================="
