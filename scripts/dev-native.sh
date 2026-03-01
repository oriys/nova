#!/bin/bash
# Native macOS development environment for Nova.
# Starts Postgres in Docker, then runs all Go services natively on macOS.
# This enables Apple VZ backend detection and usage.
#
# Usage:
#   scripts/dev-native.sh          # start all services
#   scripts/dev-native.sh stop     # stop all services
#   scripts/dev-native.sh status   # show service status
#   scripts/dev-native.sh logs     # tail all logs
#   scripts/dev-native.sh seed     # seed sample functions
#   scripts/dev-native.sh no-frontend  # start backend only (no Lumen)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG="$ROOT_DIR/configs/nova-native.json"
PG_DSN="postgres://nova:nova@localhost:5432/nova?sslmode=disable"
LOG_DIR="/tmp/nova/logs/native"
PID_DIR="/tmp/nova/pids"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()  { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[-]${NC} $1"; }
info() { echo -e "${BLUE}[*]${NC} $1"; }

mkdir -p "$LOG_DIR" "$PID_DIR"

# ── Helpers ───────────────────────────────────────────────────────────────────

spawn_detached() {
    local log_file="$1"
    shift

    if command -v python3 >/dev/null 2>&1; then
        python3 - "$log_file" "$@" <<'PY'
import subprocess
import sys

log_path = sys.argv[1]
cmd = sys.argv[2:]

with open(log_path, "ab", buffering=0) as log:
    proc = subprocess.Popen(
        cmd,
        stdin=subprocess.DEVNULL,
        stdout=log,
        stderr=subprocess.STDOUT,
        start_new_session=True,
        close_fds=True,
    )

print(proc.pid)
PY
    else
        nohup "$@" > "$log_file" 2>&1 < /dev/null &
        echo "$!"
    fi
}

start_service() {
    local name="$1"
    shift
    local pid_file="$PID_DIR/$name.pid"
    local log_file="$LOG_DIR/$name.log"

    if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        warn "$name already running (PID $(cat "$pid_file")); restarting to pick up the latest build"
        stop_service "$name"
    fi

    log "Starting $name..."
    local pid
    pid="$(spawn_detached "$log_file" "$@")"
    echo "$pid" > "$pid_file"
    info "$name started (PID $pid, log: $log_file)"
}

stop_service() {
    local name="$1"
    local pid_file="$PID_DIR/$name.pid"

    if [ -f "$pid_file" ]; then
        local pid
        pid=$(cat "$pid_file")
        if kill -0 "$pid" 2>/dev/null; then
            log "Stopping $name (PID $pid)..."
            kill "$pid" 2>/dev/null || true
            # Wait up to 5s for graceful shutdown
            for i in $(seq 1 50); do
                kill -0 "$pid" 2>/dev/null || break
                sleep 0.1
            done
            kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null
        fi
        rm -f "$pid_file"
    fi
}

wait_for_port() {
    local port="$1"
    local name="$2"
    local timeout="${3:-30}"
    local start=$SECONDS
    while ! nc -z 127.0.0.1 "$port" 2>/dev/null; do
        if (( SECONDS - start >= timeout )); then
            err "$name failed to start within ${timeout}s"
            return 1
        fi
        sleep 0.5
    done
}

# ── Commands ──────────────────────────────────────────────────────────────────

do_build() {
    log "Building native binaries..."
    cd "$ROOT_DIR"
    make build nova-vz 2>&1 | tail -5
    log "Build complete"
}

do_postgres() {
    # Start postgres via docker, reusing the compose volume
    if docker ps --format '{{.Names}}' | grep -q '^nova-postgres$'; then
        info "Postgres already running"
    else
        log "Starting Postgres via Docker..."
        docker run -d --name nova-postgres \
            -e POSTGRES_USER=nova \
            -e POSTGRES_PASSWORD=nova \
            -e POSTGRES_DB=nova \
            -p 127.0.0.1:5432:5432 \
            -v nova_pg_data:/var/lib/postgresql/data \
            -v "$ROOT_DIR/scripts/init-db.sql:/docker-entrypoint-initdb.d/init-db.sql:ro" \
            postgres:16-alpine >/dev/null
        info "Postgres starting..."
    fi
    # Wait for postgres to be ready
    log "Waiting for Postgres..."
    for i in $(seq 1 60); do
        if docker exec nova-postgres pg_isready -U nova -d nova >/dev/null 2>&1; then
            log "Postgres ready"
            return 0
        fi
        sleep 0.5
    done
    err "Postgres failed to start"
    return 1
}

do_start() {
    cd "$ROOT_DIR"

    # Build
    do_build

    # Postgres
    do_postgres

    # Create working directories
    mkdir -p /tmp/nova/code /tmp/nova/wasm-code /tmp/nova/applevz-code /tmp/nova/applevz-socks /tmp/nova/applevz-snapshots

    # Build macOS-native agent for WASM backend (nova-agent default is linux/amd64)
    if [ ! -f "$ROOT_DIR/bin/nova-agent-darwin" ]; then
        log "Building macOS-native agent for WASM backend..."
        CGO_ENABLED=0 go build -o "$ROOT_DIR/bin/nova-agent-darwin" ./cmd/agent
    fi
    export NOVA_AGENT_PATH="$ROOT_DIR/bin/nova-agent-darwin"

    # Nova (control plane) - port 9001
    # Start nova first to run DB migrations before other services
    start_service nova \
        bin/nova daemon --config "$CONFIG" --http :9001 --pg-dsn "$PG_DSN"
    wait_for_port 9001 "nova" 30

    # Comet (data plane) - port 9090
    start_service comet \
        bin/comet daemon --config "$CONFIG" --grpc :9090 --pg-dsn "$PG_DSN"
    wait_for_port 9090 "comet" 15

    # Aurora (observability) - port 9002
    start_service aurora \
        bin/aurora daemon --config "$CONFIG" --listen :9002 --pg-dsn "$PG_DSN"
    wait_for_port 9002 "aurora" 15

    # Corona (scheduler) - port 9003
    start_service corona \
        bin/corona daemon --config "$CONFIG" --comet-grpc 127.0.0.1:9090 --listen :9003 --pg-dsn "$PG_DSN"
    wait_for_port 9003 "corona" 15

    # Nebula (event bus) - port 9004
    start_service nebula \
        bin/nebula daemon --config "$CONFIG" --comet-grpc 127.0.0.1:9090 --listen :9004 --pg-dsn "$PG_DSN"
    wait_for_port 9004 "nebula" 15

    # Zenith (gateway) - port 9000
    start_service zenith \
        bin/zenith serve \
        --listen :9000 \
        --nova-url http://127.0.0.1:9001 \
        --comet-grpc 127.0.0.1:9090 \
        --corona-url http://127.0.0.1:9003 \
        --nebula-url http://127.0.0.1:9004 \
        --aurora-url http://127.0.0.1:9002

    wait_for_port 9000 "zenith" 10

    # Lumen frontend (unless --no-frontend)
    if [ "${NO_FRONTEND:-0}" != "1" ]; then
        start_lumen
    fi

    echo ""
    log "All services started! 🚀"
    echo ""
    info "  Gateway:    http://localhost:9000"
    info "  Nova API:   http://localhost:9001"
    info "  Comet gRPC: localhost:9090"
    info "  Aurora:     http://localhost:9002"
    info "  Corona:     http://localhost:9003"
    info "  Nebula:     http://localhost:9004"
    info "  Postgres:   localhost:5432"
    if [ "${NO_FRONTEND:-0}" != "1" ]; then
        info "  Lumen:      http://localhost:3000"
    fi
    echo ""
    info "  Logs: $LOG_DIR/"
    info "  PIDs: $PID_DIR/"
    echo ""
    info "  Stop:   scripts/dev-native.sh stop"
    info "  Status: scripts/dev-native.sh status"
    info "  Seed:   scripts/dev-native.sh seed"
}

start_lumen() {
    local pid_file="$PID_DIR/lumen.pid"
    local log_file="$LOG_DIR/lumen.log"

    if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        warn "lumen already running (PID $(cat "$pid_file"))"
        return 0
    fi

    # Install dependencies if needed
    if [ ! -d "$ROOT_DIR/lumen/node_modules" ]; then
        log "Installing Lumen dependencies..."
        (cd "$ROOT_DIR/lumen" && npm install --silent) >> "$log_file" 2>&1
    fi

    log "Starting lumen..."
    local pid
    pid="$(spawn_detached "$log_file" env BACKEND_URL=http://localhost:9000 bash -lc "cd \"$ROOT_DIR/lumen\" && npx next dev --port 3000")"
    echo "$pid" > "$pid_file"
    info "lumen started (PID $pid, log: $log_file)"
}

do_stop() {
    log "Stopping all native services..."
    for svc in lumen zenith nebula corona aurora comet nova; do
        stop_service "$svc"
    done
    log "All services stopped"
    echo ""
    info "Postgres left running (docker stop nova-postgres to stop)"
}

do_status() {
    echo ""
    printf "  %-12s %-8s %-6s %s\n" "SERVICE" "STATUS" "PID" "PORT"
    printf "  %-12s %-8s %-6s %s\n" "-------" "------" "---" "----"
    for svc_port in nova:9001 comet:9090 aurora:9002 corona:9003 nebula:9004 zenith:9000 lumen:3000; do
        svc="${svc_port%%:*}"
        port="${svc_port##*:}"
        pid_file="$PID_DIR/$svc.pid"
        if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
            status="${GREEN}running${NC}"
            pid=$(cat "$pid_file")
        else
            status="${RED}stopped${NC}"
            pid="-"
        fi
        printf "  %-12s %-18b %-6s %s\n" "$svc" "$status" "$pid" "$port"
    done
    # Postgres
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q '^nova-postgres$'; then
        printf "  %-12s %-18b %-6s %s\n" "postgres" "${GREEN}running${NC}" "docker" "5432"
    else
        printf "  %-12s %-18b %-6s %s\n" "postgres" "${RED}stopped${NC}" "-" "5432"
    fi
    echo ""
}

do_logs() {
    tail -f "$LOG_DIR"/*.log
}

do_seed() {
    log "Seeding functions..."
    "$ROOT_DIR/scripts/seed-functions.sh" http://localhost:9000
    log "Seeding workflows..."
    "$ROOT_DIR/scripts/seed-workflows.sh" http://localhost:9000
}

# ── Main ──────────────────────────────────────────────────────────────────────

case "${1:-start}" in
    start)  do_start ;;
    no-frontend) NO_FRONTEND=1 do_start ;;
    stop)   do_stop ;;
    status) do_status ;;
    logs)   do_logs ;;
    seed)   do_seed ;;
    build)  do_build ;;
    restart)
        do_stop
        sleep 1
        do_start
        ;;
    *)
        echo "Usage: $0 {start|no-frontend|stop|status|logs|seed|build|restart}"
        exit 1
        ;;
esac
