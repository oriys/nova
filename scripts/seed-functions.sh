#!/bin/bash
# Nova Sample Functions Seeder
# Creates sample functions for all supported runtimes via the Nova API
#
# All handler functions follow the AWS Lambda convention:
#   handler(event, context) -> result
#
# Usage:
#   ./scripts/seed-functions.sh                   # Default: http://localhost:9000
#   ./scripts/seed-functions.sh http://nova:9000  # Custom API URL
#   SKIP_COMPILED=1 ./scripts/seed-functions.sh   # Skip compiled languages (Go, Rust, etc.)

set -e

API_URL="${1:-http://localhost:9000}"
SKIP_COMPILED="${SKIP_COMPILED:-0}"
SKIP_WORKFLOWS="${SKIP_WORKFLOWS:-0}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()  { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
info() { echo -e "${BLUE}[*]${NC} $1"; }

# Wait for API to be ready
wait_for_api() {
    log "Waiting for Nova API at ${API_URL}..."
    local retries=30
    while ! curl -sf "${API_URL}/health" >/dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ ${retries} -eq 0 ]]; then
            warn "API not ready after 30 seconds, giving up"
            exit 1
        fi
        sleep 1
    done
    log "API is ready"
}

create_function() {
    local name="$1"
    local runtime="$2"
    local code="$3"
    local memory="${4:-128}"
    local timeout="${5:-30}"
    local handler="${6:-}"

    if [[ -z "${handler}" ]]; then
        case "${runtime}" in
            java*|kotlin*|scala*)
                handler="Handler::handler"
                ;;
            dotnet*)
                handler="handler::Handler::Handle"
                ;;
            go*|rust*|swift*|zig*|wasm*|provided*|custom*)
                handler="handler"
                ;;
            *)
                handler="main.handler"
                ;;
        esac
    fi

    curl -sf -X POST "${API_URL}/functions" \
        -H "Content-Type: application/json" \
        -d "{
            \"name\": \"${name}\",
            \"runtime\": \"${runtime}\",
            \"handler\": \"${handler}\",
            \"memory_mb\": ${memory},
            \"timeout_s\": ${timeout},
            \"code\": ${code}
        }" >/dev/null 2>&1 && log "  Created: ${name} (${runtime})" || warn "  Skipped: ${name} (may already exist)"
}

main() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  Nova Sample Functions Seeder${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    wait_for_api

    # ─────────────────────────────────────────────────────────
    # Python Functions (handler-only style, AWS Lambda-compatible)
    # ─────────────────────────────────────────────────────────
    info "Creating Python functions..."

    create_function "hello-python" "python" '"def handler(event, context):\n    name = event.get(\"name\", \"World\")\n    return {\n        \"message\": f\"Hello, {name}!\",\n        \"runtime\": \"python\",\n        \"request_id\": context.request_id,\n    }"'

    create_function "fibonacci" "python" '"def handler(event, context):\n    def fib(n):\n        if n <= 1:\n            return n\n        a, b = 0, 1\n        for _ in range(2, n + 1):\n            a, b = b, a + b\n        return b\n    n = event.get(\"n\", 10)\n    return {\"n\": n, \"fibonacci\": fib(n)}"'

    create_function "prime-checker" "python" '"import math\n\ndef handler(event, context):\n    n = event.get(\"n\", 17)\n    if n < 2:\n        return {\"n\": n, \"is_prime\": False}\n    if n == 2:\n        return {\"n\": n, \"is_prime\": True}\n    if n % 2 == 0:\n        return {\"n\": n, \"is_prime\": False}\n    for i in range(3, int(math.sqrt(n)) + 1, 2):\n        if n % i == 0:\n            return {\"n\": n, \"is_prime\": False}\n    return {\"n\": n, \"is_prime\": True}"'

    create_function "echo" "python" '"import time\n\ndef handler(event, context):\n    return {\n        \"echo\": event,\n        \"timestamp\": time.time(),\n        \"remaining_ms\": context.get_remaining_time_in_millis(),\n    }"'

    create_function "factorial" "python" '"def handler(event, context):\n    n = event.get(\"n\", 5)\n    result = 1\n    for i in range(2, n + 1):\n        result *= i\n    return {\"n\": n, \"factorial\": result}"'

    # ─────────────────────────────────────────────────────────
    # Node.js Functions (handler-only style, AWS Lambda-compatible)
    # ─────────────────────────────────────────────────────────
    info "Creating Node.js functions..."

    create_function "hello-node" "node" '"function handler(event, context) {\n  const name = event.name || \"World\";\n  return {\n    message: \"Hello, \" + name + \"!\",\n    runtime: \"node\",\n    requestId: context.requestId,\n  };\n}\n\nmodule.exports = { handler };"' 256

    create_function "json-transform" "node" '"function handler(event, context) {\n  const data = event.data || {};\n  const op = event.operation || \"keys\";\n  let result;\n  if (op === \"uppercase\") {\n    result = JSON.parse(JSON.stringify(data), (k, v) => typeof v === \"string\" ? v.toUpperCase() : v);\n  } else if (op === \"lowercase\") {\n    result = JSON.parse(JSON.stringify(data), (k, v) => typeof v === \"string\" ? v.toLowerCase() : v);\n  } else if (op === \"keys\") {\n    result = Object.keys(data);\n  } else if (op === \"values\") {\n    result = Object.values(data);\n  } else {\n    result = data;\n  }\n  return { operation: op, result };\n}\n\nmodule.exports = { handler };"' 256

    create_function "uuid-generator" "node" '"const crypto = require(\"crypto\");\n\nfunction handler(event, context) {\n  const count = event.count || 1;\n  const uuids = [];\n  for (let i = 0; i < count; i++) {\n    uuids.push(crypto.randomUUID());\n  }\n  return { uuids };\n}\n\nmodule.exports = { handler };"' 256

    # ─────────────────────────────────────────────────────────
    # Ruby Functions (handler-only style, AWS Lambda-compatible)
    # ─────────────────────────────────────────────────────────
    info "Creating Ruby functions..."

    create_function "hello-ruby" "ruby" '"def handler(event, context)\n  name = event[\"name\"] || \"World\"\n  {\n    message: \"Hello, #{name}!\",\n    runtime: \"ruby\",\n    request_id: context.request_id,\n  }\nend"'

    create_function "word-count" "ruby" '"def handler(event, context)\n  text = event[\"text\"] || \"\"\n  words = text.split(/\\s+/).reject(&:empty?)\n  {\n    text: text,\n    word_count: words.length,\n    char_count: text.length,\n    unique_words: words.uniq.length,\n  }\nend"'

    # ─────────────────────────────────────────────────────────
    # PHP Functions (handler-only style, AWS Lambda-compatible)
    # ─────────────────────────────────────────────────────────
    info "Creating PHP functions..."

    create_function "hello-php" "php" '"<?php\nfunction handler($event, $context) {\n    $name = $event[\"name\"] ?? \"World\";\n    return [\"message\" => \"Hello, $name!\", \"runtime\" => \"php\", \"request_id\" => $context[\"request_id\"] ?? \"\"];\n}"'

    create_function "array-stats" "php" '"<?php\nfunction handler($event, $context) {\n    $numbers = $event[\"numbers\"] ?? [1, 2, 3, 4, 5];\n    return [\n        \"count\" => count($numbers),\n        \"sum\" => array_sum($numbers),\n        \"avg\" => count($numbers) > 0 ? array_sum($numbers) / count($numbers) : 0,\n        \"min\" => min($numbers),\n        \"max\" => max($numbers),\n    ];\n}"'

    # ─────────────────────────────────────────────────────────
    # Deno Functions (handler-only style, AWS Lambda-compatible)
    # ─────────────────────────────────────────────────────────
    info "Creating Deno functions..."

    create_function "hello-deno" "deno" '"export function handler(event, context) {\n  const name = event.name || \"World\";\n  return { message: `Hello, ${name}!`, runtime: \"deno\", requestId: context.requestId };\n}"'

    create_function "base64-codec" "deno" '"export function handler(event, context) {\n  const operation = event.operation || \"encode\";\n  const data = event.data || \"\";\n  let result;\n  if (operation === \"encode\") {\n    result = btoa(data);\n  } else if (operation === \"decode\") {\n    result = atob(data);\n  } else {\n    result = data;\n  }\n  return { operation, input: data, output: result };\n}"'

    # ─────────────────────────────────────────────────────────
    # Bun Functions (handler-only style, AWS Lambda-compatible)
    # ─────────────────────────────────────────────────────────
    info "Creating Bun functions..."

    create_function "hello-bun" "bun" '"function handler(event, context) {\n  const name = event.name || \"World\";\n  return { message: `Hello, ${name}!`, runtime: \"bun\", requestId: context.requestId };\n}\n\nmodule.exports = { handler };"'

    create_function "hash-generator" "bun" '"const crypto = require(\"crypto\");\n\nfunction handler(event, context) {\n  const data = event.data || \"hello\";\n  const algorithm = event.algorithm || \"sha256\";\n  const hash = crypto.createHash(algorithm).update(data).digest(\"hex\");\n  return { data, algorithm, hash };\n}\n\nmodule.exports = { handler };"'

    # ─────────────────────────────────────────────────────────
    # Compiled Languages (Go, Rust, Java, .NET)
    # ─────────────────────────────────────────────────────────
    if [[ "${SKIP_COMPILED}" == "1" ]]; then
        warn "Skipping compiled languages (SKIP_COMPILED=1)"
    else
        info "Creating Go functions (will be compiled)..."

        create_function "hello-go" "go" '"package main\n\nimport (\n\t\"encoding/json\"\n\t\"fmt\"\n)\n\ntype Event struct {\n\tName string `json:\"name\"`\n}\n\nfunc Handler(event json.RawMessage, ctx Context) (interface{}, error) {\n\tvar e Event\n\tjson.Unmarshal(event, &e)\n\tif e.Name == \"\" {\n\t\te.Name = \"World\"\n\t}\n\treturn map[string]string{\"message\": fmt.Sprintf(\"Hello, %s!\", e.Name), \"runtime\": \"go\"}, nil\n}"'

        create_function "sum-array-go" "go" '"package main\n\nimport \"encoding/json\"\n\ntype SumEvent struct {\n\tNumbers []int `json:\"numbers\"`\n}\n\nfunc Handler(event json.RawMessage, ctx Context) (interface{}, error) {\n\tvar e SumEvent\n\tjson.Unmarshal(event, &e)\n\tsum := 0\n\tfor _, n := range e.Numbers {\n\t\tsum += n\n\t}\n\treturn map[string]interface{}{\"numbers\": e.Numbers, \"sum\": sum}, nil\n}"'

        info "Creating Rust functions (will be compiled)..."

        create_function "hello-rust" "rust" '"use serde::{Deserialize, Serialize};\nuse serde_json::Value;\n\n#[derive(Deserialize)]\nstruct Event {\n    name: Option<String>,\n}\n\n#[derive(Serialize)]\nstruct Response {\n    message: String,\n    runtime: String,\n}\n\npub fn handler(event: Value, _ctx: crate::context::Context) -> Result<Value, String> {\n    let e: Event = serde_json::from_value(event).map_err(|e| e.to_string())?;\n    let name = e.name.unwrap_or_else(|| \"World\".to_string());\n    let resp = Response {\n        message: format!(\"Hello, {}!\", name),\n        runtime: \"rust\".to_string(),\n    };\n    serde_json::to_value(&resp).map_err(|e| e.to_string())\n}"'

        info "Creating Java functions (will be compiled)..."

        create_function "hello-java" "java" '"import java.util.*;\n\npublic class Handler {\n    public static Object handler(String event, Map<String, Object> context) {\n        return \"{\\\"message\\\":\\\"Hello, World!\\\",\\\"runtime\\\":\\\"java\\\"}\";\n    }\n}"' 256

        info "Creating .NET functions (will be compiled)..."

        create_function "hello-dotnet" "dotnet" '"using System.Text.Json;\nusing System.Collections.Generic;\n\npublic static class Handler {\n    public static object Handle(string eventJson, Dictionary<string, object> context) {\n        var doc = JsonDocument.Parse(eventJson);\n        var name = doc.RootElement.TryGetProperty(\"name\", out var n) ? n.GetString() : \"World\";\n        return new { message = $\"Hello, {name}!\", runtime = \"dotnet\" };\n    }\n}"' 256
    fi

    echo ""
    echo -e "${GREEN}========================================${NC}"
    log "Sample functions created successfully!"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "  Try invoking:"
    echo "  -------------"
    echo "  curl -X POST ${API_URL}/functions/hello-python/invoke -d '{\"name\": \"Nova\"}'"
    echo "  curl -X POST ${API_URL}/functions/fibonacci/invoke -d '{\"n\": 20}'"
    echo "  curl -X POST ${API_URL}/functions/prime-checker/invoke -d '{\"n\": 97}'"
    echo "  curl -X POST ${API_URL}/functions/hello-node/invoke -d '{\"name\": \"World\"}'"
    echo "  curl -X POST ${API_URL}/functions/hello-ruby/invoke -d '{\"name\": \"Ruby\"}'"
    echo ""
    echo "  List all functions:"
    echo "  curl ${API_URL}/functions | jq '.[] | {name, runtime}'"
    echo ""

    # Seed DAG workflows (multi-language pipelines)
    if [[ "${SKIP_WORKFLOWS}" == "1" ]]; then
        warn "Skipping workflow seeding (SKIP_WORKFLOWS=1)"
    else
        SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
        if [ -x "${SCRIPT_DIR}/seed-workflows.sh" ]; then
            "${SCRIPT_DIR}/seed-workflows.sh" "${API_URL}"
        fi
    fi
}

main "$@"
