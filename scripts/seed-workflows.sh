#!/bin/sh
# Nova Workflow Seeder
# Creates demo DAG workflows showcasing multi-language orchestration
#
# Workflows:
#   1. data-pipeline      — Sequential ETL: Python → Node.js → Ruby
#   2. parallel-compute   — Fan-out/Fan-in: Node.js → (Python | PHP | Deno) → Python
#   3. order-processing   — Complex business DAG: Python → Node.js → (PHP | Python) → Ruby → Deno
#
# Usage:
#   ./scripts/seed-workflows.sh                   # Default: http://localhost:9000
#   ./scripts/seed-workflows.sh http://nova:9000  # Custom API URL

set -e

API_URL="${1:-http://localhost:9000}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
info() { echo -e "${BLUE}[*]${NC} $1"; }
step() { echo -e "${CYAN}[→]${NC} $1"; }

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

    if [ -z "${handler}" ]; then
        case "${runtime}" in
            java*|kotlin*|scala*)
                handler="Handler::handler"
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

create_workflow() {
    local name="$1"
    local description="$2"

    curl -sf -X POST "${API_URL}/workflows" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"${name}\", \"description\": \"${description}\"}" \
        >/dev/null 2>&1 && log "  Created workflow: ${name}" || warn "  Skipped workflow: ${name} (may already exist)"
}

publish_version() {
    local name="$1"
    local definition="$2"

    curl -sf -X POST "${API_URL}/workflows/${name}/versions" \
        -H "Content-Type: application/json" \
        -d "${definition}" \
        >/dev/null 2>&1 && log "  Published version: ${name}" || warn "  Failed to publish: ${name}"
}

# ═══════════════════════════════════════════════════════════════════
# Pipeline Functions
# ═══════════════════════════════════════════════════════════════════

create_pipeline_functions() {
    echo ""
    info "Creating pipeline functions for workflows..."
    echo ""

    # ── Workflow 1: data-pipeline (Python → Node.js → Ruby) ──────

    step "Data Pipeline functions..."

    create_function "wf-extract-data" "python" '"def handler(event, context):\n    \"\"\"Extract and validate user data from raw input.\"\"\"\n    users = event.get(\"users\", [])\n    valid = []\n    invalid = 0\n    for u in users:\n        if u.get(\"name\") and u.get(\"email\") and isinstance(u.get(\"age\"), (int, float)):\n            valid.append({\n                \"name\": u[\"name\"].strip(),\n                \"email\": u[\"email\"].strip().lower(),\n                \"age\": int(u[\"age\"]),\n            })\n        else:\n            invalid += 1\n    return {\n        \"valid_users\": valid,\n        \"total_input\": len(users),\n        \"valid_count\": len(valid),\n        \"invalid_count\": invalid,\n        \"stage\": \"extract\",\n    }"'

    create_function "wf-transform-data" "node" '"function handler(event, context) {\n  const users = event.valid_users || [];\n  const transformed = users.map(u => ({\n    ...u,\n    display_name: u.name.toUpperCase(),\n    domain: u.email.split(\"@\")[1] || \"unknown\",\n    age_group: u.age < 18 ? \"minor\" : u.age < 35 ? \"young_adult\" : u.age < 55 ? \"adult\" : \"senior\",\n    initials: u.name.split(\" \").map(w => w[0]).join(\"\").toUpperCase(),\n  }));\n  const domains = [...new Set(transformed.map(u => u.domain))];\n  const ageGroups = {};\n  transformed.forEach(u => { ageGroups[u.age_group] = (ageGroups[u.age_group] || 0) + 1; });\n  return {\n    users: transformed,\n    domains: domains,\n    age_distribution: ageGroups,\n    count: transformed.length,\n    stage: \"transform\",\n  };\n}\n\nmodule.exports = { handler };"' 256

    create_function "wf-generate-report" "ruby" '"def handler(event, context)\n  users = event[\"users\"] || []\n  domains = event[\"domains\"] || []\n  age_dist = event[\"age_distribution\"] || {}\n  count = event[\"count\"] || 0\n\n  top_domain = domains.max_by { |d| users.count { |u| u[\"domain\"] == d } }\n\n  {\n    report_title: \"User Data Pipeline Report\",\n    summary: {\n      total_processed: count,\n      unique_domains: domains.length,\n      top_domain: top_domain,\n      age_distribution: age_dist,\n    },\n    sample_users: users.first(3).map { |u|\n      { name: u[\"display_name\"], email: u[\"email\"], group: u[\"age_group\"] }\n    },\n    stage: \"report\",\n    pipeline: \"data-pipeline\",\n  }\nend"'

    # ── Workflow 2: parallel-compute (Node.js → Python|PHP|Deno → Python) ──

    step "Parallel Compute functions..."

    create_function "wf-split-input" "node" '"function handler(event, context) {\n  const numbers = event.numbers || [5, 8, 13, 21, 34];\n  const sorted = [...numbers].sort((a, b) => a - b);\n  return {\n    numbers: numbers,\n    max_n: Math.max(...numbers),\n    min_n: Math.min(...numbers),\n    text_repr: numbers.join(\",\"),\n    sorted: sorted,\n    count: numbers.length,\n  };\n}\n\nmodule.exports = { handler };"' 256

    create_function "wf-compute-fib" "python" '"def handler(event, context):\n    \"\"\"Compute Fibonacci numbers for the input array.\"\"\"\n    def fib(n):\n        if n <= 1:\n            return n\n        a, b = 0, 1\n        for _ in range(2, n + 1):\n            a, b = b, a + b\n        return b\n\n    numbers = event.get(\"numbers\", [])\n    fib_map = {}\n    for n in numbers:\n        fib_map[str(n)] = fib(n)\n    max_n = event.get(\"max_n\", 10)\n    return {\n        \"fibonacci_map\": fib_map,\n        \"max_fib\": fib(max_n),\n        \"golden_ratio_approx\": round(fib(max_n) / fib(max_n - 1), 6) if max_n > 1 else 1,\n    }"'

    create_function "wf-compute-stats" "php" '"<?php\nfunction handler($event, $context) {\n    $numbers = $event[\"numbers\"] ?? [1, 2, 3, 4, 5];\n    sort($numbers);\n    $count = count($numbers);\n    $sum = array_sum($numbers);\n    $avg = $count > 0 ? $sum / $count : 0;\n    $median = 0;\n    if ($count > 0) {\n        $mid = intdiv($count, 2);\n        $median = $count % 2 === 0\n            ? ($numbers[$mid - 1] + $numbers[$mid]) / 2\n            : $numbers[$mid];\n    }\n    $variance = 0;\n    foreach ($numbers as $n) {\n        $variance += ($n - $avg) * ($n - $avg);\n    }\n    $variance = $count > 0 ? $variance / $count : 0;\n    return [\n        \"count\" => $count,\n        \"sum\" => $sum,\n        \"avg\" => round($avg, 4),\n        \"median\" => $median,\n        \"min\" => $numbers[0] ?? 0,\n        \"max\" => $numbers[$count - 1] ?? 0,\n        \"variance\" => round($variance, 4),\n        \"std_dev\" => round(sqrt($variance), 4),\n        \"range\" => ($numbers[$count - 1] ?? 0) - ($numbers[0] ?? 0),\n    ];\n}"'

    create_function "wf-encode-payload" "deno" '"export function handler(event, context) {\n  const text = event.text_repr || \"\";\n  const numbers = event.numbers || [];\n  const encoded = btoa(text);\n  const reversed = text.split(\"\").reverse().join(\"\");\n  const charSum = text.split(\"\").reduce((s, c) => s + c.charCodeAt(0), 0);\n  return {\n    original: text,\n    base64: encoded,\n    reversed: reversed,\n    char_sum: charSum,\n    hex: numbers.map(n => \"0x\" + n.toString(16)).join(\" \"),\n    binary: numbers.map(n => \"0b\" + n.toString(2)).join(\" \"),\n    length: text.length,\n  };\n}"'

    create_function "wf-aggregate-results" "python" '"def handler(event, context):\n    \"\"\"Aggregate results from parallel compute branches.\"\"\"\n    fib = event.get(\"wf_compute_fib\", event.get(\"compute_fib\", {}))\n    stats = event.get(\"wf_compute_stats\", event.get(\"compute_stats\", {}))\n    encode = event.get(\"wf_encode_payload\", event.get(\"encode_payload\", {}))\n    return {\n        \"analysis_complete\": True,\n        \"fibonacci\": {\n            \"map\": fib.get(\"fibonacci_map\", {}),\n            \"max_fib\": fib.get(\"max_fib\"),\n            \"golden_ratio\": fib.get(\"golden_ratio_approx\"),\n        },\n        \"statistics\": {\n            \"count\": stats.get(\"count\"),\n            \"sum\": stats.get(\"sum\"),\n            \"avg\": stats.get(\"avg\"),\n            \"median\": stats.get(\"median\"),\n            \"std_dev\": stats.get(\"std_dev\"),\n        },\n        \"encoding\": {\n            \"base64\": encode.get(\"base64\"),\n            \"hex\": encode.get(\"hex\"),\n            \"binary\": encode.get(\"binary\"),\n        },\n        \"pipeline\": \"parallel-compute\",\n    }"'

    # ── Workflow 3: order-processing (Python → Node.js → PHP|Python → Ruby → Deno) ──

    step "Order Processing functions..."

    create_function "wf-validate-order" "python" '"def handler(event, context):\n    \"\"\"Validate incoming order structure and compute subtotal.\"\"\"\n    order_id = event.get(\"order_id\", \"ORD-UNKNOWN\")\n    customer = event.get(\"customer\", \"\")\n    items = event.get(\"items\", [])\n    payment = event.get(\"payment_method\", \"unknown\")\n    errors = []\n    if not customer:\n        errors.append(\"missing_customer\")\n    if not items:\n        errors.append(\"no_items\")\n    for i, item in enumerate(items):\n        if not item.get(\"name\"):\n            errors.append(f\"item_{i}_missing_name\")\n        if item.get(\"price\", 0) <= 0:\n            errors.append(f\"item_{i}_invalid_price\")\n        if item.get(\"qty\", 0) <= 0:\n            errors.append(f\"item_{i}_invalid_qty\")\n    subtotal = sum(i.get(\"price\", 0) * i.get(\"qty\", 1) for i in items)\n    return {\n        \"order_id\": order_id,\n        \"customer\": customer,\n        \"items\": items,\n        \"item_count\": sum(i.get(\"qty\", 1) for i in items),\n        \"subtotal\": round(subtotal, 2),\n        \"payment_method\": payment,\n        \"valid\": len(errors) == 0,\n        \"validation_errors\": errors,\n        \"stage\": \"validate\",\n    }"'

    create_function "wf-calc-pricing" "node" '"function handler(event, context) {\n  const subtotal = event.subtotal || 0;\n  const itemCount = event.item_count || 0;\n  // Volume discount tiers\n  let discountPct = 0;\n  if (itemCount >= 10) discountPct = 15;\n  else if (itemCount >= 5) discountPct = 10;\n  else if (itemCount >= 3) discountPct = 5;\n  const discountAmt = Math.round(subtotal * discountPct / 100 * 100) / 100;\n  const afterDiscount = subtotal - discountAmt;\n  const taxRate = 8.25;\n  const tax = Math.round(afterDiscount * taxRate / 100 * 100) / 100;\n  const shipping = afterDiscount >= 100 ? 0 : 9.99;\n  const total = Math.round((afterDiscount + tax + shipping) * 100) / 100;\n  return {\n    order_id: event.order_id,\n    customer: event.customer,\n    items: event.items,\n    subtotal: subtotal,\n    discount_pct: discountPct,\n    discount_amount: discountAmt,\n    after_discount: afterDiscount,\n    tax_rate: taxRate,\n    tax: tax,\n    shipping: shipping,\n    total: total,\n    payment_method: event.payment_method,\n    stage: \"pricing\",\n  };\n}\n\nmodule.exports = { handler };"' 256

    create_function "wf-check-inventory" "php" '"<?php\nfunction handler($event, $context) {\n    $items = $event[\"items\"] ?? [];\n    $orderId = $event[\"order_id\"] ?? \"\";\n    $results = [];\n    $allAvailable = true;\n    $totalWeight = 0;\n    foreach ($items as $item) {\n        $name = $item[\"name\"] ?? \"unknown\";\n        $qty = $item[\"qty\"] ?? 1;\n        // Simulated inventory levels (hash-based for consistency)\n        $stock = abs(crc32($name)) % 200 + 10;\n        $inStock = $stock >= $qty;\n        $weight = round($qty * 0.5, 1);\n        $results[] = [\n            \"item\" => $name,\n            \"requested\" => $qty,\n            \"available\" => $stock,\n            \"in_stock\" => $inStock,\n            \"weight_kg\" => $weight,\n        ];\n        $totalWeight += $weight;\n        if (!$inStock) $allAvailable = false;\n    }\n    return [\n        \"order_id\" => $orderId,\n        \"inventory\" => $results,\n        \"all_available\" => $allAvailable,\n        \"total_weight_kg\" => $totalWeight,\n        \"warehouse\" => \"WH-\" . strtoupper(substr(md5($orderId), 0, 4)),\n        \"stage\" => \"inventory\",\n    ];\n}"'

    create_function "wf-fraud-check" "python" '"import hashlib\nimport json\n\ndef handler(event, context):\n    \"\"\"Score fraud risk for the order.\"\"\"\n    order_id = event.get(\"order_id\", \"\")\n    total = event.get(\"total\", 0)\n    payment = event.get(\"payment_method\", \"unknown\")\n    customer = event.get(\"customer\", \"\")\n    risk_score = 0\n    flags = []\n    # Rule-based fraud scoring\n    if total > 1000:\n        risk_score += 30\n        flags.append(\"high_value_order\")\n    if total > 500 and payment != \"credit_card\":\n        risk_score += 20\n        flags.append(\"large_non_card_payment\")\n    if payment == \"crypto\":\n        risk_score += 15\n        flags.append(\"crypto_payment\")\n    if not customer or len(customer) < 2:\n        risk_score += 25\n        flags.append(\"suspicious_customer_name\")\n    # Deterministic check ID\n    check_id = hashlib.sha256(f\"{order_id}:{total}:{customer}\".encode()).hexdigest()[:12]\n    approved = risk_score <= 50\n    return {\n        \"order_id\": order_id,\n        \"risk_score\": risk_score,\n        \"risk_level\": \"high\" if risk_score > 50 else \"medium\" if risk_score > 20 else \"low\",\n        \"flags\": flags,\n        \"approved\": approved,\n        \"check_id\": check_id,\n        \"stage\": \"fraud_check\",\n    }"'

    create_function "wf-fulfill-order" "ruby" '"def handler(event, context)\n  inventory = event[\"wf_check_inventory\"] || event[\"check_inventory\"] || {}\n  fraud = event[\"wf_fraud_check\"] || event[\"fraud_check\"] || {}\n\n  inv_ok = inventory[\"all_available\"] != false\n  fraud_ok = fraud[\"approved\"] != false\n  can_fulfill = inv_ok && fraud_ok\n\n  order_id = inventory[\"order_id\"] || fraud[\"order_id\"] || \"UNKNOWN\"\n  warehouse = inventory[\"warehouse\"] || \"WH-DEFAULT\"\n  weight = inventory[\"total_weight_kg\"] || 0\n\n  reasons = []\n  reasons << \"inventory_unavailable\" unless inv_ok\n  reasons << \"fraud_check_failed (score: #{fraud[\"risk_score\"]})\" unless fraud_ok\n\n  tracking = can_fulfill ? \"TRK-#{order_id.gsub(/[^A-Z0-9]/i, \"\")}-#{rand(10000..99999)}\" : nil\n  carrier = weight > 10 ? \"freight\" : weight > 2 ? \"express\" : \"standard\"\n\n  {\n    order_id: order_id,\n    fulfillment_status: can_fulfill ? \"processing\" : \"rejected\",\n    inventory_ok: inv_ok,\n    fraud_ok: fraud_ok,\n    risk_level: fraud[\"risk_level\"] || \"unknown\",\n    warehouse: warehouse,\n    carrier: can_fulfill ? carrier : nil,\n    tracking_id: tracking,\n    rejection_reasons: reasons.empty? ? nil : reasons,\n    stage: \"fulfillment\",\n  }\nend"'

    create_function "wf-send-notification" "deno" '"export function handler(event, context) {\n  const status = event.fulfillment_status || \"unknown\";\n  const orderId = event.order_id || \"UNKNOWN\";\n  const tracking = event.tracking_id;\n  const carrier = event.carrier;\n\n  let subject, body, priority;\n  if (status === \"processing\") {\n    subject = `Order ${orderId} Confirmed`;\n    body = `Your order has been confirmed and is being processed.\\n` +\n           `Carrier: ${carrier}\\nTracking: ${tracking}\\n` +\n           `Warehouse: ${event.warehouse}`;\n    priority = \"normal\";\n  } else {\n    subject = `Order ${orderId} Could Not Be Processed`;\n    body = `We were unable to process your order.\\n` +\n           `Reasons: ${(event.rejection_reasons || [\"unknown\"]).join(\", \")}`;\n    priority = \"high\";\n  }\n\n  return {\n    notification_type: status === \"processing\" ? \"order_confirmation\" : \"order_rejection\",\n    channel: \"email\",\n    priority: priority,\n    subject: subject,\n    body: body,\n    order_id: orderId,\n    sent_at: new Date().toISOString(),\n    stage: \"notification\",\n    pipeline: \"order-processing\",\n  };\n}"'
}

# ═══════════════════════════════════════════════════════════════════
# Workflow Definitions
# ═══════════════════════════════════════════════════════════════════

create_workflows() {
    echo ""
    info "Creating DAG workflows..."
    echo ""

    # ── Workflow 1: data-pipeline ─────────────────────────────────
    # Linear ETL: Python → Node.js → Ruby
    #
    #   [extract (Python)] → [transform (Node.js)] → [report (Ruby)]
    #
    step "Workflow: data-pipeline (Python → Node.js → Ruby)"

    create_workflow "data-pipeline" "Sequential ETL pipeline: extract user data (Python), transform and enrich (Node.js), generate report (Ruby). Demonstrates linear cross-language data flow."

    publish_version "data-pipeline" '{
        "nodes": [
            {
                "node_key": "extract",
                "function_name": "wf-extract-data",
                "timeout_s": 30,
                "retry_policy": {"max_attempts": 2, "base_ms": 500, "max_backoff_ms": 5000}
            },
            {
                "node_key": "transform",
                "function_name": "wf-transform-data",
                "timeout_s": 30,
                "retry_policy": {"max_attempts": 2, "base_ms": 500, "max_backoff_ms": 5000}
            },
            {
                "node_key": "report",
                "function_name": "wf-generate-report",
                "timeout_s": 30
            }
        ],
        "edges": [
            {"from": "extract", "to": "transform"},
            {"from": "transform", "to": "report"}
        ]
    }'

    # ── Workflow 2: parallel-compute ──────────────────────────────
    # Fan-out/Fan-in: Node.js → (Python | PHP | Deno) → Python
    #
    #                           ┌→ [fibonacci (Python)] ──┐
    #   [split (Node.js)] ─────┼→ [stats (PHP)]         ─┼→ [aggregate (Python)]
    #                           └→ [encode (Deno)]       ─┘
    #
    step "Workflow: parallel-compute (Node.js → Python|PHP|Deno → Python)"

    create_workflow "parallel-compute" "Fan-out/fan-in computation: split input (Node.js), parallel branches compute fibonacci (Python), statistics (PHP), and encoding (Deno), then aggregate results (Python). Demonstrates parallel DAG execution across 5 languages."

    publish_version "parallel-compute" '{
        "nodes": [
            {
                "node_key": "split_input",
                "function_name": "wf-split-input",
                "timeout_s": 15
            },
            {
                "node_key": "compute_fib",
                "function_name": "wf-compute-fib",
                "timeout_s": 30,
                "retry_policy": {"max_attempts": 3, "base_ms": 200, "max_backoff_ms": 3000}
            },
            {
                "node_key": "compute_stats",
                "function_name": "wf-compute-stats",
                "timeout_s": 30,
                "retry_policy": {"max_attempts": 3, "base_ms": 200, "max_backoff_ms": 3000}
            },
            {
                "node_key": "encode_payload",
                "function_name": "wf-encode-payload",
                "timeout_s": 30,
                "retry_policy": {"max_attempts": 3, "base_ms": 200, "max_backoff_ms": 3000}
            },
            {
                "node_key": "aggregate",
                "function_name": "wf-aggregate-results",
                "timeout_s": 30
            }
        ],
        "edges": [
            {"from": "split_input", "to": "compute_fib"},
            {"from": "split_input", "to": "compute_stats"},
            {"from": "split_input", "to": "encode_payload"},
            {"from": "compute_fib", "to": "aggregate"},
            {"from": "compute_stats", "to": "aggregate"},
            {"from": "encode_payload", "to": "aggregate"}
        ]
    }'

    # ── Workflow 3: order-processing ──────────────────────────────
    # Complex business pipeline with diamond pattern (6 languages)
    #
    #                                          ┌→ [inventory (PHP)]  ──┐
    #   [validate (Python)] → [pricing (Node)] ┤                       ├→ [fulfill (Ruby)] → [notify (Deno)]
    #                                          └→ [fraud (Python)]   ──┘
    #
    step "Workflow: order-processing (Python → Node.js → PHP|Python → Ruby → Deno)"

    create_workflow "order-processing" "E-commerce order pipeline: validate order (Python), calculate pricing (Node.js), parallel inventory check (PHP) and fraud detection (Python), fulfill order (Ruby), send notification (Deno). Demonstrates complex diamond DAG with 6 languages."

    publish_version "order-processing" '{
        "nodes": [
            {
                "node_key": "validate_order",
                "function_name": "wf-validate-order",
                "timeout_s": 15,
                "retry_policy": {"max_attempts": 2, "base_ms": 300, "max_backoff_ms": 3000}
            },
            {
                "node_key": "calc_pricing",
                "function_name": "wf-calc-pricing",
                "timeout_s": 15,
                "retry_policy": {"max_attempts": 2, "base_ms": 300, "max_backoff_ms": 3000}
            },
            {
                "node_key": "wf_check_inventory",
                "function_name": "wf-check-inventory",
                "timeout_s": 30,
                "retry_policy": {"max_attempts": 3, "base_ms": 500, "max_backoff_ms": 10000}
            },
            {
                "node_key": "wf_fraud_check",
                "function_name": "wf-fraud-check",
                "timeout_s": 30,
                "retry_policy": {"max_attempts": 2, "base_ms": 500, "max_backoff_ms": 5000}
            },
            {
                "node_key": "fulfill_order",
                "function_name": "wf-fulfill-order",
                "timeout_s": 30,
                "retry_policy": {"max_attempts": 2, "base_ms": 1000, "max_backoff_ms": 10000}
            },
            {
                "node_key": "send_notification",
                "function_name": "wf-send-notification",
                "timeout_s": 15
            }
        ],
        "edges": [
            {"from": "validate_order", "to": "calc_pricing"},
            {"from": "calc_pricing", "to": "wf_check_inventory"},
            {"from": "calc_pricing", "to": "wf_fraud_check"},
            {"from": "wf_check_inventory", "to": "fulfill_order"},
            {"from": "wf_fraud_check", "to": "fulfill_order"},
            {"from": "fulfill_order", "to": "send_notification"}
        ]
    }'
}

# ═══════════════════════════════════════════════════════════════════
# Main
# ═══════════════════════════════════════════════════════════════════

main() {
    echo ""
    echo -e "${GREEN}════════════════════════════════════════════${NC}"
    echo -e "${GREEN}  Nova Workflow Seeder${NC}"
    echo -e "${GREEN}  Multi-Language DAG Orchestration Demos${NC}"
    echo -e "${GREEN}════════════════════════════════════════════${NC}"
    echo ""

    wait_for_api

    create_pipeline_functions
    create_workflows

    echo ""
    echo -e "${GREEN}════════════════════════════════════════════${NC}"
    log "Workflow seeding complete!"
    echo -e "${GREEN}════════════════════════════════════════════${NC}"
    echo ""
    echo "  Workflows created:"
    echo "  ───────────────────"
    echo "  1. data-pipeline      Python → Node.js → Ruby"
    echo "  2. parallel-compute   Node.js → (Python | PHP | Deno) → Python"
    echo "  3. order-processing   Python → Node.js → (PHP | Python) → Ruby → Deno"
    echo ""
    echo "  Try running:"
    echo "  ─────────────"
    echo "  # ETL pipeline"
    echo "  curl -X POST ${API_URL}/workflows/data-pipeline/runs -H 'Content-Type: application/json' \\"
    echo '    -d '"'"'{"input":{"users":[{"name":"Alice Smith","email":"alice@example.com","age":28},{"name":"Bob Jones","email":"bob@corp.io","age":45},{"name":"Charlie","email":"charlie@example.com","age":17}]}}'"'"''
    echo ""
    echo "  # Parallel computation"
    echo "  curl -X POST ${API_URL}/workflows/parallel-compute/runs -H 'Content-Type: application/json' \\"
    echo '    -d '"'"'{"input":{"numbers":[5,8,13,21,34,55]}}'"'"''
    echo ""
    echo "  # Order processing"
    echo "  curl -X POST ${API_URL}/workflows/order-processing/runs -H 'Content-Type: application/json' \\"
    echo '    -d '"'"'{"input":{"order_id":"ORD-2024-001","customer":"Alice Smith","items":[{"name":"Mechanical Keyboard","price":149.99,"qty":1},{"name":"USB-C Cable","price":12.99,"qty":3}],"payment_method":"credit_card"}}'"'"''
    echo ""
    echo "  Check run status:"
    echo "  curl ${API_URL}/workflows/data-pipeline/runs | jq '.[0]'"
    echo ""
}

main "$@"
