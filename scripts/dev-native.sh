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
GRPC_HEALTH_PROBE="$ROOT_DIR/bin/grpc-health-check"

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

service_label() {
    local name="$1"
    echo "com.oriys.nova.native.$name"
}

service_info() {
    local name="$1"
    launchctl list "$(service_label "$name")" 2>/dev/null || true
}

service_pid() {
    local name="$1"
    service_info "$name" | awk -F' = ' '/"PID"/ {
        gsub(/^[[:space:]]+/, "", $2)
        gsub(/;$/, "", $2)
        print $2
        exit
    }'
}

service_exists() {
    local name="$1"
    launchctl list "$(service_label "$name")" >/dev/null 2>&1
}

write_pid_file() {
    local name="$1"
    local pid_file="$PID_DIR/$name.pid"
    local pid
    pid="$(service_pid "$name")"
    if [ -n "$pid" ]; then
        echo "$pid" > "$pid_file"
    else
        rm -f "$pid_file"
    fi
}

resolve_command_path() {
    local cmd="$1"

    if [[ "$cmd" == /* ]]; then
        echo "$cmd"
    elif [[ "$cmd" == */* ]]; then
        echo "$ROOT_DIR/${cmd#./}"
    else
        command -v "$cmd"
    fi
}

start_service() {
    local name="$1"
    shift
    local pid_file="$PID_DIR/$name.pid"
    local log_file="$LOG_DIR/$name.log"
    local label
    local cmd=("$@")
    local launch_cmd=(/usr/bin/env "PATH=$PATH" "HOME=$HOME")
    label="$(service_label "$name")"

    if service_exists "$name"; then
        local current_pid
        current_pid="$(service_pid "$name")"
        if [ -n "$current_pid" ]; then
            warn "$name already running (PID $current_pid); restarting to pick up the latest build"
        else
            warn "$name already registered with launchd; restarting to pick up the latest build"
        fi
        stop_service "$name"
    elif [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        warn "$name already running (PID $(cat "$pid_file")); restarting to pick up the latest build"
        stop_service "$name"
    fi

    log "Starting $name..."
    cmd[0]="$(resolve_command_path "${cmd[0]}")"
    if [ -n "${NOVA_AGENT_PATH:-}" ]; then
        launch_cmd+=("NOVA_AGENT_PATH=$NOVA_AGENT_PATH")
    fi
    launch_cmd+=("${cmd[@]}")
    launchctl submit -l "$label" -o "$log_file" -e "$log_file" -- "${launch_cmd[@]}"

    local pid=""
    for i in $(seq 1 20); do
        pid="$(service_pid "$name")"
        if [ -n "$pid" ]; then
            break
        fi
        sleep 0.1
    done

    write_pid_file "$name"
    if [ -n "$pid" ]; then
        info "$name started (PID $pid, log: $log_file)"
    else
        info "$name submitted to launchd (label: $label, log: $log_file)"
    fi
}

stop_service() {
    local name="$1"
    local pid_file="$PID_DIR/$name.pid"
    local label
    label="$(service_label "$name")"

    if service_exists "$name"; then
        local pid
        pid="$(service_pid "$name")"
        if [ -n "$pid" ]; then
            log "Stopping $name (PID $pid)..."
        else
            log "Stopping $name..."
        fi

        launchctl remove "$label" >/dev/null 2>&1 || true

        for i in $(seq 1 50); do
            service_exists "$name" || break
            sleep 0.1
        done
    elif [ -f "$pid_file" ]; then
        local pid
        local pgid
        pid=$(cat "$pid_file")
        pgid="$(ps -o pgid= -p "$pid" 2>/dev/null | tr -d '[:space:]')"
        if kill -0 "$pid" 2>/dev/null; then
            log "Stopping $name (PID $pid)..."
            if [ -n "$pgid" ]; then
                kill -TERM -- "-$pgid" 2>/dev/null || kill "$pid" 2>/dev/null || true
            else
                kill "$pid" 2>/dev/null || true
            fi
            for i in $(seq 1 50); do
                kill -0 "$pid" 2>/dev/null || break
                sleep 0.1
            done
            if kill -0 "$pid" 2>/dev/null; then
                if [ -n "$pgid" ]; then
                    kill -KILL -- "-$pgid" 2>/dev/null || kill -9 "$pid" 2>/dev/null || true
                else
                    kill -9 "$pid" 2>/dev/null || true
                fi
            fi
        fi
    fi

    rm -f "$pid_file"
}

wait_for_grpc_health() {
    local addr="$1"
    local name="$2"
    local timeout="${3:-30}"
    local start=$SECONDS
    while ! "$GRPC_HEALTH_PROBE" -timeout 2s "$addr" >/dev/null 2>&1; do
        if (( SECONDS - start >= timeout )); then
            err "$name gRPC health check failed within ${timeout}s"
            return 1
        fi
        sleep 0.5
    done
}

wait_for_http_health() {
    local url="$1"
    local name="$2"
    local timeout="${3:-30}"
    local start=$SECONDS
    while ! curl -sf "$url" >/dev/null 2>&1; do
        if (( SECONDS - start >= timeout )); then
            err "$name HTTP health check failed within ${timeout}s"
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
    if [ ! -x "$GRPC_HEALTH_PROBE" ] || [ "$ROOT_DIR/scripts/grpc-health-check.go" -nt "$GRPC_HEALTH_PROBE" ]; then
        log "Building gRPC health probe..."
        go build -o "$GRPC_HEALTH_PROBE" "$ROOT_DIR/scripts/grpc-health-check.go"
    fi
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

    # Nova (control plane) - gRPC port 9001
    # Start nova first to run DB migrations before other services
    start_service nova \
        bin/nova daemon --config "$CONFIG" --grpc :9001 --pg-dsn "$PG_DSN"
    wait_for_grpc_health 127.0.0.1:9001 "nova" 30

    # Comet (data plane) - gRPC port 9090
    start_service comet \
        bin/comet daemon --config "$CONFIG" --grpc :9090 --pg-dsn "$PG_DSN"
    wait_for_grpc_health 127.0.0.1:9090 "comet" 15

    # Aurora (observability) - gRPC port 9002
    start_service aurora \
        bin/aurora daemon --config "$CONFIG" --grpc :9002 --pg-dsn "$PG_DSN"
    wait_for_grpc_health 127.0.0.1:9002 "aurora" 15

    # Corona (scheduler) - gRPC port 9003
    start_service corona \
        bin/corona daemon --config "$CONFIG" --comet-grpc 127.0.0.1:9090 --grpc :9003 --pg-dsn "$PG_DSN"
    wait_for_grpc_health 127.0.0.1:9003 "corona" 15

    # Nebula (event bus) - gRPC port 9004
    start_service nebula \
        bin/nebula daemon --config "$CONFIG" --comet-grpc 127.0.0.1:9090 --grpc :9004 --pg-dsn "$PG_DSN"
    wait_for_grpc_health 127.0.0.1:9004 "nebula" 15

    # Zenith (gateway) - HTTP port 9000
    start_service zenith \
        bin/zenith serve \
        --listen :9000 \
        --nova-grpc 127.0.0.1:9001 \
        --comet-grpc 127.0.0.1:9090 \
        --corona-grpc 127.0.0.1:9003 \
        --nebula-grpc 127.0.0.1:9004 \
        --aurora-grpc 127.0.0.1:9002

    wait_for_http_health http://127.0.0.1:9000/health "zenith" 10

    # Lumen frontend (unless --no-frontend)
    if [ "${NO_FRONTEND:-0}" != "1" ]; then
        start_lumen
    fi

    echo ""
    log "All services started! 🚀"
    echo ""
    info "  Gateway:    http://localhost:9000"
    info "  Nova gRPC:  localhost:9001"
    info "  Comet gRPC: localhost:9090"
    info "  Aurora gRPC: localhost:9002"
    info "  Corona gRPC: localhost:9003"
    info "  Nebula gRPC: localhost:9004"
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

    if service_exists "lumen"; then
        local pid
        pid="$(service_pid "lumen")"
        if [ -n "$pid" ]; then
            warn "lumen already running (PID $pid)"
            write_pid_file "lumen"
            return 0
        fi
        stop_service "lumen"
    elif [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        warn "lumen already running (PID $(cat "$pid_file"))"
        return 0
    else
        rm -f "$pid_file"
    fi

    # Install dependencies if needed
    if [ ! -d "$ROOT_DIR/lumen/node_modules" ]; then
        log "Installing Lumen dependencies..."
        (cd "$ROOT_DIR/lumen" && npm install --silent) >> "$log_file" 2>&1
    fi

    start_service lumen env BACKEND_URL=http://localhost:9000 bash -lc "cd \"$ROOT_DIR/lumen\" && npx next dev --port 3000"
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

        if service_exists "$svc"; then
            pid="$(service_pid "$svc")"
        else
            pid=""
        fi

        if [ -n "$pid" ]; then
            echo "$pid" > "$pid_file"
            status="${GREEN}running${NC}"
        else
            rm -f "$pid_file"
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
