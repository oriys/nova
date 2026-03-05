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

set -euxo pipefail

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
        if [ "${retries}" -eq 0 ]; then
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
    local create_payload

    if [ -z "${handler}" ]; then
        case "${runtime}" in
            java*|kotlin*|scala*)
                handler="Handler::handler"
                ;;
            go*|rust*|swift*|zig*|wasm*|provided*|custom*|c|cpp*|graalvm*)
                handler="handler"
                ;;
            *)
                handler="main.handler"
                ;;
        esac
    fi

    create_payload="{
            \"name\": \"${name}\",
            \"runtime\": \"${runtime}\",
            \"handler\": \"${handler}\",
            \"memory_mb\": ${memory},
            \"timeout_s\": ${timeout},
            \"code\": ${code}
        }"

    if curl -sf -X POST "${API_URL}/functions" \
        -H "Content-Type: application/json" \
        -d "${create_payload}" >/dev/null 2>&1; then
        log "  Created: ${name} (${runtime})"
        return
    fi

    if curl -sf -X PUT "${API_URL}/functions/${name}/code" \
        -H "Content-Type: application/json" \
        -d "{
            \"code\": ${code}
        }" >/dev/null 2>&1; then
        log "  Updated: ${name} (${runtime})"
        return
    fi

    warn "  Skipped: ${name} (create/update failed)"
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
    # Elixir Functions (handler-only style)
    # ─────────────────────────────────────────────────────────
    info "Creating Elixir functions..."

    create_function "hello-elixir" "elixir" '"defmodule Handler do\n  def handler(event, context) do\n    name = Map.get(event, \"name\", \"World\")\n    %{message: \"Hello, #{name}!\", runtime: \"elixir\", request_id: context.request_id}\n  end\nend"'

    # ─────────────────────────────────────────────────────────
    # Lua Functions (handler-only style)
    # ─────────────────────────────────────────────────────────
    info "Creating Lua functions..."

    create_function "hello-lua" "lua" '"function handler(event, context)\n    local name = event[\"name\"] or \"World\"\n    return {message = \"Hello, \" .. name .. \"!\", runtime = \"lua\", request_id = context.request_id}\nend"'

    # ─────────────────────────────────────────────────────────
    # Perl Functions (handler-only style)
    # ─────────────────────────────────────────────────────────
    info "Creating Perl functions..."

    create_function "hello-perl" "perl" '"sub handler {\n    my ($event, $context) = @_;\n    my $name = $event->{\"name\"} // \"World\";\n    return {message => \"Hello, $name!\", runtime => \"perl\", request_id => $context->{\"request_id\"} // \"\"};\n}\n1;"'

    # ─────────────────────────────────────────────────────────
    # R Functions (handler-only style)
    # ─────────────────────────────────────────────────────────
    info "Creating R functions..."

    create_function "hello-r" "r" '"handler <- function(event, context) {\n  name <- if (!is.null(event$name)) event$name else \"World\"\n  list(message = paste0(\"Hello, \", name, \"!\"), runtime = \"r\", request_id = context$request_id)\n}"' 256 120

    # ─────────────────────────────────────────────────────────
    # Julia Functions (handler-only style)
    # ─────────────────────────────────────────────────────────
    info "Creating Julia functions..."

    create_function "hello-julia" "julia" '"function handler(event, context)\n    name = get(event, \"name\", \"World\")\n    return Dict(\"message\" => \"Hello, $(name)!\", \"runtime\" => \"julia\", \"request_id\" => get(context, \"request_id\", \"\"))\nend"' 512 120

    # ─────────────────────────────────────────────────────────
    # Compiled Languages (Go, Rust, Java, Kotlin, Scala, C, C++, Swift, Zig, GraalVM)
    # ─────────────────────────────────────────────────────────
    if [ "${SKIP_COMPILED}" = "1" ]; then
        warn "Skipping compiled languages (SKIP_COMPILED=1)"
    else
        info "Creating Go functions (will be compiled)..."

        create_function "hello-go" "go" '"package main\n\nimport (\n\t\"encoding/json\"\n\t\"fmt\"\n)\n\ntype Event struct {\n\tName string `json:\"name\"`\n}\n\nfunc Handler(event json.RawMessage, ctx Context) (interface{}, error) {\n\tvar e Event\n\tjson.Unmarshal(event, &e)\n\tif e.Name == \"\" {\n\t\te.Name = \"World\"\n\t}\n\treturn map[string]string{\"message\": fmt.Sprintf(\"Hello, %s!\", e.Name), \"runtime\": \"go\"}, nil\n}"'

        create_function "sum-array-go" "go" '"package main\n\nimport \"encoding/json\"\n\ntype SumEvent struct {\n\tNumbers []int `json:\"numbers\"`\n}\n\nfunc Handler(event json.RawMessage, ctx Context) (interface{}, error) {\n\tvar e SumEvent\n\tjson.Unmarshal(event, &e)\n\tsum := 0\n\tfor _, n := range e.Numbers {\n\t\tsum += n\n\t}\n\treturn map[string]interface{}{\"numbers\": e.Numbers, \"sum\": sum}, nil\n}"'

        info "Creating Rust functions (will be compiled)..."

        create_function "hello-rust" "rust" '"use serde::{Deserialize, Serialize};\nuse serde_json::Value;\n\n#[derive(Deserialize)]\nstruct Event {\n    name: Option<String>,\n}\n\n#[derive(Serialize)]\nstruct Response {\n    message: String,\n    runtime: String,\n}\n\npub fn handler(event: Value, _ctx: crate::context::Context) -> Result<Value, String> {\n    let e: Event = serde_json::from_value(event).map_err(|e| e.to_string())?;\n    let name = e.name.unwrap_or_else(|| \"World\".to_string());\n    let resp = Response {\n        message: format!(\"Hello, {}!\", name),\n        runtime: \"rust\".to_string(),\n    };\n    serde_json::to_value(&resp).map_err(|e| e.to_string())\n}"'

        create_function "number-stats-rust" "rust" '"use serde_json::{json, Value};\n\npub fn handler(event: Value, _ctx: crate::context::Context) -> Result<Value, String> {\n    let numbers = event\n        .get(\"numbers\")\n        .and_then(|v| v.as_array())\n        .cloned()\n        .unwrap_or_else(|| vec![Value::from(1), Value::from(2), Value::from(3)]);\n\n    let mut sum = 0.0_f64;\n    for n in &numbers {\n        if let Some(v) = n.as_f64() {\n            sum += v;\n        }\n    }\n\n    let count = numbers.len() as f64;\n    let avg = if count > 0.0 { sum / count } else { 0.0 };\n    Ok(json!({\"count\": numbers.len(), \"sum\": sum, \"avg\": avg}))\n}"'

        info "Creating Java functions (will be compiled)..."

        create_function "hello-java" "java" '"import java.util.*;\n\npublic class Handler {\n    public static Object handler(String event, Map<String, Object> context) {\n        return \"{\\\"message\\\":\\\"Hello, World!\\\",\\\"runtime\\\":\\\"java\\\"}\";\n    }\n}"' 256

        create_function "number-stats-java" "java" '"import java.util.Locale;\nimport java.util.Map;\nimport java.util.regex.Matcher;\nimport java.util.regex.Pattern;\n\npublic class Handler {\n    public static Object handler(String event, Map<String, Object> context) {\n        Pattern pattern = Pattern.compile(\"-?\\\\d+(?:\\\\.\\\\d+)?\");\n        Matcher matcher = pattern.matcher(event == null ? \"\" : event);\n        int count = 0;\n        double sum = 0.0;\n        while (matcher.find()) {\n            sum += Double.parseDouble(matcher.group());\n            count++;\n        }\n        double avg = count == 0 ? 0.0 : sum / count;\n        return String.format(Locale.US, \"{\\\"count\\\":%d,\\\"sum\\\":%.2f,\\\"avg\\\":%.2f}\", count, sum, avg);\n    }\n}"' 256

        info "Creating Kotlin functions (will be compiled)..."

        create_function "hello-kotlin" "kotlin" '"object Handler {\n    fun handler(event: String, context: Map<String, Any>): Any {\n        val name = Regex(\"\\\"name\\\"\\\\s*:\\\\s*\\\"([^\\\"]+)\\\"\").find(event)?.groupValues?.get(1) ?: \"World\"\n        return \"{\\\"message\\\":\\\"Hello, \" + name + \"!\\\",\\\"runtime\\\":\\\"kotlin\\\"}\"\n    }\n}"' 256

        create_function "word-stats-kotlin" "kotlin" '"object Handler {\n    fun handler(event: String, context: Map<String, Any>): Any {\n        val words = Regex(\"[A-Za-z0-9_]+\").findAll(event).map { it.value.toLowerCase() }.toList()\n        val unique = words.toSet().size\n        return \"{\\\"words\\\":\" + words.size + \",\\\"unique\\\":\" + unique + \"}\"\n    }\n}"' 256

        info "Creating Scala functions (will be compiled)..."

        create_function "hello-scala" "scala" '"object Handler {\n  def handler(event: String, context: Map[String, Any]): Any = {\n    val namePattern = \"\\\"name\\\"\\\\s*:\\\\s*\\\"([^\\\"]+)\\\"\".r\n    val name = namePattern.findFirstMatchIn(Option(event).getOrElse(\"\")).map(_.group(1)).getOrElse(\"World\")\n    \"{\\\"message\\\":\\\"Hello, \" + name + \"!\\\",\\\"runtime\\\":\\\"scala\\\"}\"\n  }\n}"' 256

        create_function "number-stats-scala" "scala" '"object Handler {\n  def handler(event: String, context: Map[String, Any]): Any = {\n    val numbers = \"-?\\\\d+(?:\\\\.\\\\d+)?\".r.findAllIn(Option(event).getOrElse(\"\")).toList.map(_.toDouble)\n    val sum = numbers.sum\n    val avg = if (numbers.nonEmpty) sum / numbers.size else 0.0\n    \"{\\\"count\\\":\" + numbers.size + \",\\\"sum\\\":\" + \"%.2f\".format(sum) + \",\\\"avg\\\":\" + \"%.2f\".format(avg) + \"}\"\n  }\n}"' 256

        info "Creating C functions (will be compiled)..."

        create_function "hello-c" "c" '"#include <stdio.h>\n#include <string.h>\n\nconst char* handler(const char* event, const char* context) {\n    static char result[256];\n    char name[96];\n    const char* source = event ? event : \"\";\n    const char* key = \"\\\"name\\\"\";\n    const char* pos = strstr(source, key);\n    snprintf(name, sizeof(name), \"World\");\n    if (pos) {\n        pos = strchr(pos + 6, 34);\n        if (pos) {\n            const char* start = pos + 1;\n            const char* end = strchr(start, 34);\n            if (end && end > start) {\n                size_t len = (size_t)(end - start);\n                if (len >= sizeof(name)) {\n                    len = sizeof(name) - 1;\n                }\n                memcpy(name, start, len);\n                name[len] = 0;\n            }\n        }\n    }\n    snprintf(result, sizeof(result), \"{\\\"message\\\":\\\"Hello, %s!\\\",\\\"runtime\\\":\\\"c\\\"}\", name);\n    return result;\n}"'

        create_function "payload-stats-c" "c" '"#include <stdio.h>\n#include <string.h>\n\nconst char* handler(const char* event, const char* context) {\n    static char result[256];\n    const char* source = event ? event : \"\";\n    size_t len = strlen(source);\n    int braces = 0;\n    int digits = 0;\n\n    for (size_t i = 0; i < len; i++) {\n        unsigned char ch = (unsigned char)source[i];\n        if (ch == 123 || ch == 125) {\n            braces++;\n        }\n        if (ch >= 48 && ch <= 57) {\n            digits++;\n        }\n    }\n\n    snprintf(result, sizeof(result), \"{\\\"length\\\":%zu,\\\"braces\\\":%d,\\\"digits\\\":%d}\", len, braces, digits);\n    return result;\n}"'

        info "Creating C++ functions (will be compiled)..."

        create_function "hello-cpp" "cpp" '"#include <string>\n\nstd::string handler(const std::string& event, const std::string& context) {\n    std::string name = \"World\";\n    std::string key = \"\\\"name\\\"\";\n    std::size_t keyPos = event.find(key);\n    if (keyPos != std::string::npos) {\n        std::size_t firstQuote = event.find(\"\\\"\", keyPos + key.size());\n        if (firstQuote != std::string::npos) {\n            std::size_t secondQuote = event.find(\"\\\"\", firstQuote + 1);\n            if (secondQuote != std::string::npos && secondQuote > firstQuote + 1) {\n                name = event.substr(firstQuote + 1, secondQuote - firstQuote - 1);\n            }\n        }\n    }\n    return std::string(\"{\\\"message\\\":\\\"Hello, \") + name + \"!\\\",\\\"runtime\\\":\\\"cpp\\\"}\";\n}"'

        create_function "number-stats-cpp" "cpp" '"#include <iomanip>\n#include <regex>\n#include <sstream>\n#include <string>\n\nstd::string handler(const std::string& event, const std::string& context) {\n    std::regex pattern(\"-?\\\\d+(?:\\\\.\\\\d+)?\");\n    auto begin = std::sregex_iterator(event.begin(), event.end(), pattern);\n    auto end = std::sregex_iterator();\n\n    int count = 0;\n    double sum = 0.0;\n    for (auto it = begin; it != end; ++it) {\n        sum += std::stod(it->str());\n        count++;\n    }\n\n    double avg = count == 0 ? 0.0 : sum / count;\n    std::ostringstream out;\n    out << std::fixed << std::setprecision(2);\n    out << \"{\\\"count\\\":\" << count << \",\\\"sum\\\":\" << sum << \",\\\"avg\\\":\" << avg << \"}\";\n    return out.str();\n}"'

        info "Creating Swift functions (will be compiled)..."

        create_function "hello-swift" "swift" '"func handler(event: String, context: NovaContext) -> String {\n    var name = \"World\"\n    let chars = Array(event)\n    let key: [Character] = [\"\\u{22}\", \"n\", \"a\", \"m\", \"e\", \"\\u{22}\"]\n    for i in 0..<(chars.count - key.count) {\n        var match = true\n        for j in 0..<key.count {\n            if chars[i+j] != key[j] { match = false; break }\n        }\n        if match {\n            var s = i + key.count\n            while s < chars.count && chars[s] != Character(\"\\u{22}\") { s += 1 }\n            s += 1\n            var e = s\n            while e < chars.count && chars[e] != Character(\"\\u{22}\") { e += 1 }\n            if e > s { name = String(chars[s..<e]) }\n            break\n        }\n    }\n    return \"{\\u{22}message\\u{22}:\\u{22}Hello, \\(name)!\\u{22},\\u{22}runtime\\u{22}:\\u{22}swift\\u{22}}\"\n}"' 256

        info "Creating Zig functions (will be compiled)..."

        create_function "hello-zig" "zig" '"const std = @import(\"std\");\n\npub fn handler(event: []const u8, allocator: std.mem.Allocator) ![]const u8 {\n    var name: []const u8 = \"World\";\n    if (std.mem.indexOf(u8, event, \"\\\"name\\\"\")) |key_pos| {\n        var i = key_pos + 6;\n        while (i < event.len and event[i] != 34) : (i += 1) {}\n        if (i < event.len) {\n            i += 1;\n            const start = i;\n            while (i < event.len and event[i] != 34) : (i += 1) {}\n            if (i > start) name = event[start..i];\n        }\n    }\n    return try std.fmt.allocPrint(allocator, \"{{\\\"message\\\":\\\"Hello, {s}!\\\",\\\"runtime\\\":\\\"zig\\\"}}\", .{name});\n}"' 256

        info "Creating GraalVM functions (will be compiled to native-image)..."

        create_function "hello-graalvm" "graalvm" '"import java.util.*;\n\npublic class Handler {\n    public static Object handler(String event, Map<String, Object> context) {\n        String name = \"World\";\n        int idx = event.indexOf(\"\\\"name\\\"\");\n        if (idx >= 0) {\n            int colon = event.indexOf(\":\", idx);\n            int q1 = event.indexOf(\"\\\"\", colon + 1);\n            int q2 = event.indexOf(\"\\\"\", q1 + 1);\n            if (q1 >= 0 && q2 > q1) name = event.substring(q1 + 1, q2);\n        }\n        return \"{\\\"message\\\":\\\"Hello, \" + name + \"!\\\",\\\"runtime\\\":\\\"graalvm\\\"}\";\n    }\n}"' 256

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
    if [ "${SKIP_WORKFLOWS}" = "1" ]; then
        warn "Skipping workflow seeding (SKIP_WORKFLOWS=1)"
    else
        SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
        if [ -x "${SCRIPT_DIR}/seed-workflows.sh" ]; then
            "${SCRIPT_DIR}/seed-workflows.sh" "${API_URL}"
        fi
    fi
}

main "$@"
