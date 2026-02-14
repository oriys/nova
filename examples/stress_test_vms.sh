#!/bin/bash
# stress_test_vms.sh - Test how many microVMs can run on this machine
#
# Usage: ./stress_test_vms.sh [max_vms]
#   max_vms: Maximum number of VMs to create (default: 100)

set -euxo pipefail

MAX_VMS=${1:-100}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FUNC_PREFIX="stress-test"

echo "=========================================="
echo "  Nova MicroVM Stress Test"
echo "=========================================="
echo "  Max VMs to create: ${MAX_VMS}"
echo "  Host memory: $(free -h | awk '/^Mem:/{print $2}')"
echo "  Host CPUs: $(nproc)"
echo "=========================================="
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo "=== Cleaning up ==="
    for i in $(seq 1 $CREATED_VMS); do
        nova delete "${FUNC_PREFIX}-${i}" 2>/dev/null || true
    done
    echo "Cleanup complete"
}

trap cleanup EXIT

# Check if nova daemon is running
if ! curl -s http://127.0.0.1:8080/health > /dev/null 2>&1; then
    echo "Error: Nova daemon not running on :8080"
    exit 1
fi

# Register test functions
echo "=== Registering ${MAX_VMS} functions ==="
CREATED_VMS=0

for i in $(seq 1 $MAX_VMS); do
    nova register "${FUNC_PREFIX}-${i}" \
        --runtime python \
        --code "${SCRIPT_DIR}/hello.py" \
        --memory 128 \
        --min-replicas 1 \
        2>/dev/null || {
            echo "Failed to register function ${i}"
            break
        }
    CREATED_VMS=$i

    # Show progress every 10 functions
    if [ $((i % 10)) -eq 0 ]; then
        echo "Registered ${i} functions..."
    fi
done

echo "Registered ${CREATED_VMS} functions"
echo ""

# Wait for VMs to be pre-warmed
echo "=== Waiting for VMs to be ready (30s) ==="
sleep 30

# Check active VMs
echo ""
echo "=== Checking active VMs ==="
ACTIVE_VMS=$(curl -s http://127.0.0.1:8080/stats | grep -o '"active_vms":[0-9]*' | cut -d: -f2)
echo "Active VMs: ${ACTIVE_VMS}"

# Show system stats
echo ""
echo "=== System Stats ==="
echo "Memory usage:"
free -h
echo ""
echo "Firecracker processes:"
ps aux | grep firecracker | grep -v grep | wc -l
echo ""

# Test invocations
echo "=== Testing invocations ==="
SUCCESS=0
FAILED=0

for i in $(seq 1 $CREATED_VMS); do
    result=$(nova invoke "${FUNC_PREFIX}-${i}" --payload '{"name":"StressTest"}' 2>&1)
    if echo "$result" | grep -q '"message"'; then
        SUCCESS=$((SUCCESS + 1))
    else
        FAILED=$((FAILED + 1))
        echo "Failed: ${FUNC_PREFIX}-${i}: $result"
    fi

    # Show progress every 10 invocations
    if [ $((i % 10)) -eq 0 ]; then
        echo "Tested ${i}/${CREATED_VMS} (success: ${SUCCESS}, failed: ${FAILED})"
    fi
done

echo ""
echo "=========================================="
echo "  Results"
echo "=========================================="
echo "  Functions registered: ${CREATED_VMS}"
echo "  Active VMs: ${ACTIVE_VMS}"
echo "  Invocations success: ${SUCCESS}"
echo "  Invocations failed: ${FAILED}"
echo ""
echo "  Memory after test:"
free -h | grep "^Mem:"
echo ""
echo "  Firecracker processes: $(ps aux | grep firecracker | grep -v grep | wc -l)"
echo "=========================================="
