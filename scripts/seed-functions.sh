#!/bin/bash
# Nova Sample Functions Seeder
# Creates sample functions for all supported runtimes via the Nova API
#
# Usage:
#   ./scripts/seed-functions.sh                   # Default: http://localhost:9000
#   ./scripts/seed-functions.sh http://nova:9000  # Custom API URL
#   SKIP_COMPILED=1 ./scripts/seed-functions.sh   # Skip compiled languages (Go, Rust, etc.)

set -e

API_URL="${1:-http://localhost:9000}"
SKIP_COMPILED="${SKIP_COMPILED:-0}"

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

    curl -sf -X POST "${API_URL}/functions" \
        -H "Content-Type: application/json" \
        -d "{
            \"name\": \"${name}\",
            \"runtime\": \"${runtime}\",
            \"handler\": \"handler\",
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
    # Python Functions
    # ─────────────────────────────────────────────────────────
    info "Creating Python functions..."

    create_function "hello-python" "python" '"import json\nimport sys\n\ndef handler(event):\n    name = event.get(\"name\", \"World\")\n    return {\"message\": f\"Hello, {name}!\", \"runtime\": \"python\"}\n\nif __name__ == \"__main__\":\n    with open(sys.argv[1]) as f:\n        event = json.load(f)\n    result = handler(event)\n    print(json.dumps(result))"'

    create_function "fibonacci" "python" '"import json\nimport sys\n\ndef fib(n):\n    if n <= 1:\n        return n\n    a, b = 0, 1\n    for _ in range(2, n + 1):\n        a, b = b, a + b\n    return b\n\ndef main():\n    with open(sys.argv[1]) as f:\n        event = json.load(f)\n    n = event.get(\"n\", 10)\n    result = fib(n)\n    print(json.dumps({\"n\": n, \"fibonacci\": result}))\n\nif __name__ == \"__main__\":\n    main()"'

    create_function "prime-checker" "python" '"import json\nimport sys\nimport math\n\ndef is_prime(n):\n    if n < 2:\n        return False\n    if n == 2:\n        return True\n    if n % 2 == 0:\n        return False\n    for i in range(3, int(math.sqrt(n)) + 1, 2):\n        if n % i == 0:\n            return False\n    return True\n\ndef main():\n    with open(sys.argv[1]) as f:\n        event = json.load(f)\n    n = event.get(\"n\", 17)\n    print(json.dumps({\"n\": n, \"is_prime\": is_prime(n)}))\n\nif __name__ == \"__main__\":\n    main()"'

    create_function "echo" "python" '"import json\nimport sys\nimport time\n\ndef main():\n    with open(sys.argv[1]) as f:\n        event = json.load(f)\n    print(json.dumps({\"echo\": event, \"timestamp\": time.time()}))\n\nif __name__ == \"__main__\":\n    main()"'

    create_function "factorial" "python" '"import json\nimport sys\n\ndef factorial(n):\n    if n <= 1:\n        return 1\n    result = 1\n    for i in range(2, n + 1):\n        result *= i\n    return result\n\ndef main():\n    with open(sys.argv[1]) as f:\n        event = json.load(f)\n    n = event.get(\"n\", 5)\n    print(json.dumps({\"n\": n, \"factorial\": factorial(n)}))\n\nif __name__ == \"__main__\":\n    main()"'

    # ─────────────────────────────────────────────────────────
    # Node.js Functions
    # ─────────────────────────────────────────────────────────
    info "Creating Node.js functions..."

    create_function "hello-node" "node" '"const fs = require(\"fs\");\n\nfunction handler(event) {\n  const name = event.name || \"World\";\n  return { message: `Hello, ${name}!`, runtime: \"node\" };\n}\n\nconst event = JSON.parse(fs.readFileSync(process.argv[2], \"utf8\"));\nconsole.log(JSON.stringify(handler(event)));"' 256

    create_function "json-transform" "node" '"const fs = require(\"fs\");\nconst event = JSON.parse(fs.readFileSync(process.argv[2], \"utf8\"));\nconst data = event.data || {};\nconst op = event.operation || \"keys\";\n\nlet result;\nif (op === \"uppercase\") {\n  result = JSON.parse(JSON.stringify(data), (k, v) => typeof v === \"string\" ? v.toUpperCase() : v);\n} else if (op === \"lowercase\") {\n  result = JSON.parse(JSON.stringify(data), (k, v) => typeof v === \"string\" ? v.toLowerCase() : v);\n} else if (op === \"keys\") {\n  result = Object.keys(data);\n} else if (op === \"values\") {\n  result = Object.values(data);\n} else {\n  result = data;\n}\n\nconsole.log(JSON.stringify({ operation: op, result }));"' 256

    create_function "uuid-generator" "node" '"const fs = require(\"fs\");\nconst crypto = require(\"crypto\");\nconst event = JSON.parse(fs.readFileSync(process.argv[2], \"utf8\"));\nconst count = event.count || 1;\nconst uuids = [];\nfor (let i = 0; i < count; i++) {\n  uuids.push(crypto.randomUUID());\n}\nconsole.log(JSON.stringify({ uuids }));"' 256

    # ─────────────────────────────────────────────────────────
    # Ruby Functions
    # ─────────────────────────────────────────────────────────
    info "Creating Ruby functions..."

    create_function "hello-ruby" "ruby" '"require \"json\"\n\ndef handler(event)\n  name = event[\"name\"] || \"World\"\n  { message: \"Hello, #{name}!\", runtime: \"ruby\" }\nend\n\nevent = JSON.parse(File.read(ARGV[0]))\nputs JSON.generate(handler(event))"'

    create_function "word-count" "ruby" '"require \"json\"\n\nevent = JSON.parse(File.read(ARGV[0]))\ntext = event[\"text\"] || \"\"\nwords = text.split(/\\s+/).reject(&:empty?)\nputs JSON.generate({\n  text: text,\n  word_count: words.length,\n  char_count: text.length,\n  unique_words: words.uniq.length\n})"'

    # ─────────────────────────────────────────────────────────
    # PHP Functions
    # ─────────────────────────────────────────────────────────
    info "Creating PHP functions..."

    create_function "hello-php" "php" '"<?php\n$event = json_decode(file_get_contents($argv[1]), true);\n$name = $event[\"name\"] ?? \"World\";\necho json_encode([\"message\" => \"Hello, $name!\", \"runtime\" => \"php\"]);"'

    create_function "array-stats" "php" '"<?php\n$event = json_decode(file_get_contents($argv[1]), true);\n$numbers = $event[\"numbers\"] ?? [1, 2, 3, 4, 5];\n$result = [\n    \"count\" => count($numbers),\n    \"sum\" => array_sum($numbers),\n    \"avg\" => count($numbers) > 0 ? array_sum($numbers) / count($numbers) : 0,\n    \"min\" => min($numbers),\n    \"max\" => max($numbers)\n];\necho json_encode($result);"'

    # ─────────────────────────────────────────────────────────
    # Deno Functions
    # ─────────────────────────────────────────────────────────
    info "Creating Deno functions..."

    create_function "hello-deno" "deno" '"const event = JSON.parse(await Deno.readTextFile(Deno.args[0]));\nconst name = event.name || \"World\";\nconsole.log(JSON.stringify({ message: `Hello, ${name}!`, runtime: \"deno\" }));"'

    create_function "base64-codec" "deno" '"const event = JSON.parse(await Deno.readTextFile(Deno.args[0]));\nconst operation = event.operation || \"encode\";\nconst data = event.data || \"\";\n\nlet result;\nif (operation === \"encode\") {\n  result = btoa(data);\n} else if (operation === \"decode\") {\n  result = atob(data);\n} else {\n  result = data;\n}\n\nconsole.log(JSON.stringify({ operation, input: data, output: result }));"'

    # ─────────────────────────────────────────────────────────
    # Bun Functions
    # ─────────────────────────────────────────────────────────
    info "Creating Bun functions..."

    create_function "hello-bun" "bun" '"const event = JSON.parse(await Bun.file(Bun.argv[2]).text());\nconst name = event.name || \"World\";\nconsole.log(JSON.stringify({ message: `Hello, ${name}!`, runtime: \"bun\" }));"'

    create_function "hash-generator" "bun" '"const event = JSON.parse(await Bun.file(Bun.argv[2]).text());\nconst data = event.data || \"hello\";\nconst algorithm = event.algorithm || \"sha256\";\n\nconst encoder = new TextEncoder();\nconst dataBuffer = encoder.encode(data);\nconst hashBuffer = await crypto.subtle.digest(algorithm.toUpperCase().replace(\"-\", \"\"), dataBuffer);\nconst hashArray = Array.from(new Uint8Array(hashBuffer));\nconst hashHex = hashArray.map(b => b.toString(16).padStart(2, \"0\")).join(\"\");\n\nconsole.log(JSON.stringify({ data, algorithm, hash: hashHex }));"'

    # ─────────────────────────────────────────────────────────
    # Compiled Languages (Go, Rust, Java, .NET)
    # ─────────────────────────────────────────────────────────
    if [[ "${SKIP_COMPILED}" == "1" ]]; then
        warn "Skipping compiled languages (SKIP_COMPILED=1)"
    else
        info "Creating Go functions (will be compiled)..."

        create_function "hello-go" "go" '"package main\n\nimport (\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"os\"\n)\n\ntype Event struct {\n\tName string `json:\"name\"`\n}\n\ntype Response struct {\n\tMessage string `json:\"message\"`\n\tRuntime string `json:\"runtime\"`\n}\n\nfunc main() {\n\tdata, _ := os.ReadFile(os.Args[1])\n\tvar event Event\n\tjson.Unmarshal(data, &event)\n\tif event.Name == \"\" {\n\t\tevent.Name = \"World\"\n\t}\n\tresp := Response{Message: fmt.Sprintf(\"Hello, %s!\", event.Name), Runtime: \"go\"}\n\tout, _ := json.Marshal(resp)\n\tfmt.Println(string(out))\n}"'

        create_function "sum-array-go" "go" '"package main\n\nimport (\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"os\"\n)\n\ntype Event struct {\n\tNumbers []int `json:\"numbers\"`\n}\n\nfunc main() {\n\tdata, _ := os.ReadFile(os.Args[1])\n\tvar event Event\n\tjson.Unmarshal(data, &event)\n\tsum := 0\n\tfor _, n := range event.Numbers {\n\t\tsum += n\n\t}\n\tresult := map[string]interface{}{\"numbers\": event.Numbers, \"sum\": sum}\n\tout, _ := json.Marshal(result)\n\tfmt.Println(string(out))\n}"'

        info "Creating Rust functions (will be compiled)..."

        create_function "hello-rust" "rust" '"use std::env;\nuse std::fs;\nuse serde::{Deserialize, Serialize};\n\n#[derive(Deserialize)]\nstruct Event {\n    name: Option<String>,\n}\n\n#[derive(Serialize)]\nstruct Response {\n    message: String,\n    runtime: String,\n}\n\nfn main() {\n    let args: Vec<String> = env::args().collect();\n    let data = fs::read_to_string(&args[1]).unwrap();\n    let event: Event = serde_json::from_str(&data).unwrap();\n    let name = event.name.unwrap_or_else(|| \"World\".to_string());\n    let resp = Response {\n        message: format!(\"Hello, {}!\", name),\n        runtime: \"rust\".to_string(),\n    };\n    println!(\"{}\", serde_json::to_string(&resp).unwrap());\n}"'

        info "Creating Java functions (will be compiled)..."

        create_function "hello-java" "java" '"import java.nio.file.*;\nimport java.util.regex.*;\n\npublic class Handler {\n    public static void main(String[] args) throws Exception {\n        String content = Files.readString(Path.of(args[0]));\n        Pattern p = Pattern.compile(\"\\\"name\\\"\\\\s*:\\\\s*\\\"([^\\\"]+)\\\"\");\n        Matcher m = p.matcher(content);\n        String name = m.find() ? m.group(1) : \"World\";\n        System.out.println(\"{\\\"message\\\": \\\"Hello, \" + name + \"!\\\", \\\"runtime\\\": \\\"java\\\"}\");\n    }\n}"' 256

        info "Creating .NET functions (will be compiled)..."

        create_function "hello-dotnet" "dotnet" '"using System.Text.Json;\n\nvar json = File.ReadAllText(args[0]);\nvar doc = JsonDocument.Parse(json);\nvar name = doc.RootElement.TryGetProperty(\"name\", out var n) ? n.GetString() : \"World\";\nConsole.WriteLine(JsonSerializer.Serialize(new { message = $\"Hello, {name}!\", runtime = \"dotnet\" }));"' 256
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
}

main "$@"
