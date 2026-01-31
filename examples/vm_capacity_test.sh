#!/bin/bash
# vm_capacity_test.sh - Find the maximum number of concurrent VMs
#
# Each VM performs RSA key generation (CPU-intensive) to test real capacity.
#
# Usage: ./vm_capacity_test.sh [batch_size] [rsa_bits]
#   batch_size: Number of concurrent requests per batch (default: 10)
#   rsa_bits: RSA key bits for computation (default: 512)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BATCH_SIZE=${1:-10}
RSA_BITS=${2:-512}

echo "=========================================="
echo "  Nova VM Capacity Test (RSA Workload)"
echo "=========================================="
echo ""
echo "System info:"
echo "  Memory: $(free -h | awk '/^Mem:/{print $2}') total, $(free -h | awk '/^Mem:/{print $7}') available"
echo "  CPUs: $(nproc)"
echo "  Kernel: $(uname -r)"
echo ""
echo "Test config:"
echo "  Batch size: ${BATCH_SIZE}"
echo "  RSA bits: ${RSA_BITS}"
echo ""

# Check daemon
if ! curl -s http://127.0.0.1:8080/health > /dev/null 2>&1; then
    echo "Error: Nova daemon not running"
    exit 1
fi

# Register RSA test function
echo "=== Registering RSA test function ==="
nova register rsa-capacity-test \
    --runtime python \
    --code "${SCRIPT_DIR}/rsa_test.py" \
    --memory 128 \
    --timeout 60 \
    2>/dev/null || nova update rsa-capacity-test --code "${SCRIPT_DIR}/rsa_test.py"

echo ""
echo "=== Starting capacity test ==="
echo "Each VM will generate ${RSA_BITS}-bit RSA keys"
echo "Press Ctrl+C to stop"
echo ""

VM_COUNT=0
LAST_SUCCESS=0
START_TIME=$(date +%s)

# Trap to show final stats
show_stats() {
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    echo ""
    echo "=========================================="
    echo "  Final Results"
    echo "=========================================="

    # Get actual VM count from daemon
    STATS=$(curl -s http://127.0.0.1:8080/stats 2>/dev/null || echo '{}')
    ACTIVE=$(echo "$STATS" | grep -o '"active_vms":[0-9]*' | cut -d: -f2 || echo "0")

    echo "  Duration: ${DURATION}s"
    echo "  Total invocations: ${TOTAL_INVOKES}"
    echo "  Successful: ${LAST_SUCCESS}"
    echo "  Failed: ${ERRORS}"
    echo "  Active VMs (from daemon): ${ACTIVE}"
    echo ""
    echo "  Memory:"
    free -h | grep "^Mem:"
    echo ""
    echo "  Firecracker processes: $(pgrep -c firecracker 2>/dev/null || echo 0)"
    echo ""

    # Per-VM memory estimate
    if [ "$ACTIVE" -gt 0 ]; then
        USED_MB=$(free -m | awk '/^Mem:/{print $3}')
        PER_VM=$((USED_MB / ACTIVE))
        echo "  Estimated memory per VM: ~${PER_VM}MB"
        echo "  Max VMs (theoretical): ~$(($(free -m | awk '/^Mem:/{print $2}') / PER_VM))"
    fi

    echo "=========================================="

    # Cleanup
    echo ""
    echo "Cleaning up..."
    nova delete rsa-capacity-test 2>/dev/null || true
}

trap show_stats EXIT

# Concurrent invocations to create multiple VMs
echo "Starting concurrent RSA computations..."
echo ""

TOTAL_INVOKES=0
LAST_SUCCESS=0
ERRORS=0
START_TIME=$(date +%s)

while true; do
    # Launch batch of concurrent invocations
    PIDS=""
    for i in $(seq 1 $BATCH_SIZE); do
        (
            result=$(curl -s -X POST http://127.0.0.1:8080/functions/rsa-capacity-test/invoke \
                -H 'Content-Type: application/json' \
                -d "{\"bits\": ${RSA_BITS}}" \
                --max-time 120 2>&1)
            if echo "$result" | grep -q '"success"'; then
                # Extract elapsed time
                elapsed=$(echo "$result" | grep -o '"elapsed_ms":[0-9]*' | cut -d: -f2)
                echo "  RSA ${RSA_BITS}-bit: ${elapsed}ms"
                exit 0
            else
                echo "  Error: $result" >&2
                exit 1
            fi
        ) &
        PIDS="$PIDS $!"
    done

    # Wait for batch and count results
    BATCH_SUCCESS=0
    BATCH_FAIL=0
    for pid in $PIDS; do
        if wait $pid 2>/dev/null; then
            BATCH_SUCCESS=$((BATCH_SUCCESS + 1))
        else
            BATCH_FAIL=$((BATCH_FAIL + 1))
        fi
    done

    TOTAL_INVOKES=$((TOTAL_INVOKES + BATCH_SIZE))
    LAST_SUCCESS=$((LAST_SUCCESS + BATCH_SUCCESS))
    ERRORS=$((ERRORS + BATCH_FAIL))

    # Get current VM count
    ACTIVE=$(curl -s http://127.0.0.1:8080/stats 2>/dev/null | grep -o '"active_vms":[0-9]*' | cut -d: -f2 || echo "?")
    MEM_AVAIL=$(free -m | awk '/^Mem:/{print $7}')

    echo ""
    echo "=== Batch ${TOTAL_INVOKES}/${BATCH_SIZE}: +${BATCH_SUCCESS} ok, +${BATCH_FAIL} fail | VMs: ${ACTIVE} | Mem avail: ${MEM_AVAIL}MB ==="
    echo ""

    # Stop if too many errors or low memory
    if [ "$BATCH_FAIL" -ge "$BATCH_SIZE" ]; then
        echo ""
        echo "All invocations in batch failed, stopping..."
        break
    fi

    if [ "$MEM_AVAIL" -lt 500 ]; then
        echo ""
        echo "Low memory (${MEM_AVAIL}MB available), stopping..."
        break
    fi

    # Small delay between batches
    sleep 2
done
