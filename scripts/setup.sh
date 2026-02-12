#!/bin/bash
# Nova Serverless Platform - One-Click Deployment Script
#
# This script deploys the complete Nova platform on a Linux x86_64 server:
# - PostgreSQL database + schema initialization
# - Nova control plane + Comet data plane + Zenith gateway
# - Lumen frontend (Next.js standalone)
# - Five systemd services, enabled at boot
#
# Usage:
#   # Option 1: Build locally and deploy to remote server
#   make build-linux
#   scp -r scripts/ bin/ lumen/ user@server:/tmp/nova-deploy/
#   ssh user@server 'sudo bash /tmp/nova-deploy/scripts/setup.sh'
#
#   # Option 2: Build and deploy on the server directly
#   git clone https://github.com/oriys/nova && cd nova
#   make build-linux
#   sudo bash scripts/setup.sh

set -e

PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt/nova/bin"
export PATH

INSTALL_DIR="/opt/nova"
FC_VERSION="latest"
ALPINE_URL="https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/x86_64/alpine-minirootfs-3.23.3-x86_64.tar.gz"
WASMTIME_VERSION="v41.0.1"
DENO_VERSION="v2.6.7"
BUN_VERSION="bun-v1.3.8"
DOTNET_VERSION="8.0.23"
ROOTFS_SIZE_MB=256
ROOTFS_SIZE_JAVA_MB=512
NODE_VERSION=20

# Detect script directory (where binaries should be)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(dirname "${SCRIPT_DIR}")"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log()  { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[!]${NC} $1" >&2; exit 1; }
info() { echo -e "${BLUE}[*]${NC} $1"; }

# Run independent functions concurrently and fail fast on first error.
run_parallel_functions() {
    local -a pids=()
    local -a names=()
    local fn

    for fn in "$@"; do
        info "  [start] ${fn}"
        "${fn}" &
        pids+=("$!")
        names+=("${fn}")
    done

    local idx
    local failed_count=0
    local failed_names=""
    for idx in "${!pids[@]}"; do
        if wait "${pids[$idx]}"; then
            log "  [done] ${names[$idx]}"
        else
            failed_count=$((failed_count + 1))
            if [[ -z "${failed_names}" ]]; then
                failed_names="${names[$idx]}"
            else
                failed_names="${failed_names}, ${names[$idx]}"
            fi
        fi
    done

    if [[ "${failed_count}" -gt 0 ]]; then
        err "Parallel stage failed (${failed_count} task(s)): ${failed_names}"
    fi
}

# Calculate a safe default parallelism level.
default_parallel_jobs() {
    local cpus
    cpus=$(getconf _NPROCESSORS_ONLN 2>/dev/null || nproc 2>/dev/null || echo 4)
    if ! [[ "${cpus}" =~ ^[0-9]+$ ]] || [[ "${cpus}" -lt 1 ]]; then
        cpus=4
    fi

    # Keep enough parallelism for speed, but avoid overloading low-end hosts.
    if [[ "${cpus}" -lt 2 ]]; then
        echo 1
    elif [[ "${cpus}" -gt 6 ]]; then
        echo 6
    else
        echo "${cpus}"
    fi
}

# ─── Checks ──────────────────────────────────────────────
check_root() {
    [[ $EUID -eq 0 ]] || err "This script must be run as root: sudo $0"
}

check_system() {
    [[ "$(uname)" == "Linux" ]] || err "This script only supports Linux"
    [[ "$(uname -m)" == "x86_64" ]] || err "This script only supports x86_64 architecture"
    [[ -e /dev/kvm ]] || warn "/dev/kvm not found - Firecracker requires KVM. VMs will not work without it."
}

check_binaries() {
    local nova_bin=""
    local comet_bin=""
    local zenith_bin=""
    local agent_bin=""

    # Look for binaries in deployment directory first
    if [[ -f "${DEPLOY_DIR}/bin/nova-linux" ]]; then
        nova_bin="${DEPLOY_DIR}/bin/nova-linux"
    elif [[ -f "${DEPLOY_DIR}/bin/nova" ]]; then
        nova_bin="${DEPLOY_DIR}/bin/nova"
    fi

    if [[ -f "${DEPLOY_DIR}/bin/comet-linux" ]]; then
        comet_bin="${DEPLOY_DIR}/bin/comet-linux"
    elif [[ -f "${DEPLOY_DIR}/bin/comet" ]]; then
        comet_bin="${DEPLOY_DIR}/bin/comet"
    fi

    if [[ -f "${DEPLOY_DIR}/bin/zenith-linux" ]]; then
        zenith_bin="${DEPLOY_DIR}/bin/zenith-linux"
    elif [[ -f "${DEPLOY_DIR}/bin/zenith" ]]; then
        zenith_bin="${DEPLOY_DIR}/bin/zenith"
    fi

    if [[ -f "${DEPLOY_DIR}/bin/nova-agent" ]]; then
        agent_bin="${DEPLOY_DIR}/bin/nova-agent"
    fi

    if [[ -z "${nova_bin}" ]] || [[ -z "${comet_bin}" ]] || [[ -z "${zenith_bin}" ]] || [[ -z "${agent_bin}" ]]; then
        err "Backend binaries not found. Please run 'make build-linux' first.
Expected binaries at:
  ${DEPLOY_DIR}/bin/nova-linux (or nova)
  ${DEPLOY_DIR}/bin/comet-linux (or comet)
  ${DEPLOY_DIR}/bin/zenith-linux (or zenith)
  ${DEPLOY_DIR}/bin/nova-agent"
    fi

    log "Found Nova binary: ${nova_bin}"
    log "Found Comet binary: ${comet_bin}"
    log "Found Zenith binary: ${zenith_bin}"
    log "Found Agent binary: ${agent_bin}"

    # Export for later use
    export NOVA_BIN="${nova_bin}"
    export COMET_BIN="${comet_bin}"
    export ZENITH_BIN="${zenith_bin}"
    export AGENT_BIN="${agent_bin}"
}

# ─── Cleanup ──────────────────────────────────────────────
cleanup_existing_binaries() {
    log "Cleaning existing binary artifacts..."

    mkdir -p "${INSTALL_DIR}/bin"

    local removed=0
    local source_nova source_comet source_zenith source_agent
    source_nova="$(readlink -f "${NOVA_BIN}" 2>/dev/null || echo "${NOVA_BIN}")"
    source_comet="$(readlink -f "${COMET_BIN}" 2>/dev/null || echo "${COMET_BIN}")"
    source_zenith="$(readlink -f "${ZENITH_BIN}" 2>/dev/null || echo "${ZENITH_BIN}")"
    source_agent="$(readlink -f "${AGENT_BIN}" 2>/dev/null || echo "${AGENT_BIN}")"
    local -a targets=(
        "${INSTALL_DIR}/bin/nova"
        "${INSTALL_DIR}/bin/nova-linux"
        "${INSTALL_DIR}/bin/comet"
        "${INSTALL_DIR}/bin/comet-linux"
        "${INSTALL_DIR}/bin/zenith"
        "${INSTALL_DIR}/bin/zenith-linux"
        "${INSTALL_DIR}/bin/nova-agent"
        "${INSTALL_DIR}/bin/firecracker"
        "${INSTALL_DIR}/bin/jailer"
    )

    # Include versioned Firecracker/Jailer binaries from prior installs.
    shopt -s nullglob
    targets+=("${INSTALL_DIR}/bin"/firecracker-*)
    targets+=("${INSTALL_DIR}/bin"/jailer-*)
    shopt -u nullglob

    local f
    for f in "${targets[@]}"; do
        if [[ -e "${f}" || -L "${f}" ]]; then
            local f_real
            f_real="$(readlink -f "${f}" 2>/dev/null || echo "${f}")"
            if [[ "${f_real}" == "${source_nova}" || "${f_real}" == "${source_comet}" || "${f_real}" == "${source_zenith}" || "${f_real}" == "${source_agent}" ]]; then
                info "  [keep] ${f} (deployment source binary)"
                continue
            fi
            rm -f "${f}"
            removed=$((removed + 1))
        fi
    done

    # Remove legacy symlinks only when they point to /opt/nova/bin artifacts.
    local link target
    for link in /usr/local/bin/firecracker /usr/local/bin/jailer; do
        if [[ -L "${link}" ]]; then
            target="$(readlink "${link}" 2>/dev/null || true)"
            case "${target}" in
                "${INSTALL_DIR}/bin/"*)
                    rm -f "${link}"
                    removed=$((removed + 1))
                    ;;
            esac
        fi
    done

    log "Removed ${removed} binary artifact(s)"
}

cleanup_deploy_build_binaries() {
    local deploy_bin="${DEPLOY_DIR}/bin"
    local install_bin="${INSTALL_DIR}/bin"

    [[ -d "${deploy_bin}" ]] || return 0

    # Safety: if deploy dir and install dir are the same path, skip cleanup.
    local deploy_real install_real
    deploy_real="$(readlink -f "${deploy_bin}" 2>/dev/null || echo "${deploy_bin}")"
    install_real="$(readlink -f "${install_bin}" 2>/dev/null || echo "${install_bin}")"
    if [[ "${deploy_real}" == "${install_real}" ]]; then
        warn "Skipping deploy build cleanup: deploy bin equals install bin (${deploy_real})"
        return 0
    fi

    log "Cleaning deployment build binaries from ${deploy_bin}..."

    local removed=0
    local -a files=(
        "${deploy_bin}/nova"
        "${deploy_bin}/nova-linux"
        "${deploy_bin}/comet"
        "${deploy_bin}/comet-linux"
        "${deploy_bin}/zenith"
        "${deploy_bin}/zenith-linux"
        "${deploy_bin}/nova-agent"
    )

    local f
    for f in "${files[@]}"; do
        if [[ -f "${f}" || -L "${f}" ]]; then
            rm -f "${f}"
            removed=$((removed + 1))
        fi
    done

    log "Removed ${removed} deployment build binary file(s)"
}

stop_existing_services() {
    log "Stopping existing Nova services (if any)..."
    systemctl stop zenith >/dev/null 2>&1 || true
    systemctl stop comet >/dev/null 2>&1 || true
    systemctl stop nova >/dev/null 2>&1 || true
    systemctl stop lumen >/dev/null 2>&1 || true
    systemctl stop nova-lumen >/dev/null 2>&1 || true
    systemctl disable zenith >/dev/null 2>&1 || true
    systemctl disable comet >/dev/null 2>&1 || true
    systemctl disable nova >/dev/null 2>&1 || true
    systemctl disable lumen >/dev/null 2>&1 || true
    systemctl disable nova-lumen >/dev/null 2>&1 || true
    systemctl reset-failed zenith >/dev/null 2>&1 || true
    systemctl reset-failed comet >/dev/null 2>&1 || true
    systemctl reset-failed nova >/dev/null 2>&1 || true
    systemctl reset-failed lumen >/dev/null 2>&1 || true
    systemctl reset-failed nova-lumen >/dev/null 2>&1 || true
}

reset_installation_state() {
    log "Resetting installation state (fresh deployment)..."

    stop_existing_services

    # Remove all runtime artifacts under /opt/nova so every run starts clean.
    rm -rf \
        "${INSTALL_DIR}/kernel" \
        "${INSTALL_DIR}/rootfs" \
        "${INSTALL_DIR}/bin" \
        "${INSTALL_DIR}/snapshots" \
        "${INSTALL_DIR}/configs" \
        "${INSTALL_DIR}/lumen"

    # Remove transient runtime state.
    rm -rf /tmp/nova
}

# ─── Dependencies ────────────────────────────────────────
install_deps() {
    log "Installing system dependencies..."
    if command -v apt-get &>/dev/null; then
        apt-get update -qq
        apt-get install -y -qq curl e2fsprogs unzip iproute2 >/dev/null
    elif command -v yum &>/dev/null; then
        yum install -y -q curl e2fsprogs unzip iproute
    elif command -v dnf &>/dev/null; then
        dnf install -y -q curl e2fsprogs unzip iproute
    else
        warn "Unknown package manager. Please install: curl, e2fsprogs, unzip, iproute2"
    fi
}

install_nodejs() {
    if command -v node &>/dev/null; then
        local current_version
        current_version=$(node --version | sed 's/v//' | cut -d. -f1)
        if [[ "${current_version}" -ge "${NODE_VERSION}" ]]; then
            log "Node.js $(node --version) already installed"
            return
        fi
    fi

    log "Installing Node.js ${NODE_VERSION}..."
    if command -v apt-get &>/dev/null; then
        # Debian/Ubuntu: Use NodeSource repository
        curl -fsSL "https://deb.nodesource.com/setup_${NODE_VERSION}.x" | bash - >/dev/null 2>&1
        apt-get install -y -qq nodejs >/dev/null
    elif command -v yum &>/dev/null; then
        # RHEL/CentOS: Use NodeSource repository
        curl -fsSL "https://rpm.nodesource.com/setup_${NODE_VERSION}.x" | bash - >/dev/null 2>&1
        yum install -y -q nodejs
    elif command -v dnf &>/dev/null; then
        # Fedora: Use NodeSource repository
        curl -fsSL "https://rpm.nodesource.com/setup_${NODE_VERSION}.x" | bash - >/dev/null 2>&1
        dnf install -y -q nodejs
    else
        err "Cannot install Node.js automatically. Please install Node.js ${NODE_VERSION}+ manually."
    fi
    log "Node.js $(node --version) installed"
}

# ─── PostgreSQL ──────────────────────────────────────────
install_postgres() {
    if command -v psql &>/dev/null; then
        log "PostgreSQL already installed"
    else
        log "Installing PostgreSQL..."
        if command -v apt-get &>/dev/null; then
            apt-get install -y -qq postgresql postgresql-contrib >/dev/null
        elif command -v yum &>/dev/null; then
            yum install -y -q postgresql-server postgresql-contrib
            postgresql-setup --initdb 2>/dev/null || true
        elif command -v dnf &>/dev/null; then
            dnf install -y -q postgresql-server postgresql-contrib
            postgresql-setup --initdb 2>/dev/null || true
        else
            err "Cannot install PostgreSQL automatically. Please install PostgreSQL manually."
        fi
    fi

    # Ensure PostgreSQL is running
    systemctl enable postgresql >/dev/null 2>&1 || true
    systemctl start postgresql || true

    # Wait for PostgreSQL to be ready
    local retries=10
    while ! su - postgres -c "psql -c 'SELECT 1'" >/dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ ${retries} -eq 0 ]]; then
            err "PostgreSQL failed to start"
        fi
        sleep 1
    done
}

setup_database() {
    log "Setting up Nova database (fresh start)..."

    info "  Recreating role/database..."
    su - postgres -c "psql -v ON_ERROR_STOP=1" >/dev/null <<'SQL'
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nova') THEN
        ALTER ROLE nova WITH LOGIN PASSWORD 'nova';
    ELSE
        CREATE ROLE nova WITH LOGIN PASSWORD 'nova';
    END IF;
END
$$;

SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = 'nova' AND pid <> pg_backend_pid();

DROP DATABASE IF EXISTS nova;
CREATE DATABASE nova OWNER nova;
SQL

    # Run schema initialization
    if [[ -f "${SCRIPT_DIR}/init-db.sql" ]]; then
        info "  Applying init-db.sql as role nova..."
        # Run schema as role `nova` to ensure created objects are owned by nova.
        su - postgres -c "psql -d nova -v ON_ERROR_STOP=1 -c \"SET lock_timeout = '10s'; SET statement_timeout = '10min'; SET ROLE nova;\" -f '${SCRIPT_DIR}/init-db.sql'" >/dev/null
        log "Database schema initialized"
    else
        warn "init-db.sql not found at ${SCRIPT_DIR}/init-db.sql - skipping schema initialization"
    fi

    log "PostgreSQL configured (db=nova user=nova)"
}

# ─── Firecracker ─────────────────────────────────────────
latest_firecracker_version() {
    local release_url="https://github.com/firecracker-microvm/firecracker/releases"
    basename "$(curl -fsSLI -o /dev/null -w "%{url_effective}" ${release_url}/latest)"
}

install_firecracker() {
    if [[ "${FC_VERSION}" == "latest" || -z "${FC_VERSION}" ]]; then
        FC_VERSION="$(latest_firecracker_version)"
    fi
    local arch="$(uname -m)"

    if [[ -x "${INSTALL_DIR}/bin/firecracker" ]]; then
        local existing_version
        existing_version=$(${INSTALL_DIR}/bin/firecracker --version 2>/dev/null | head -1 || echo "unknown")
        warn "Existing Firecracker detected: ${existing_version} - overwriting"
    fi

    log "Installing Firecracker ${FC_VERSION}..."
    local tmp=$(mktemp -d)
    local fc_url="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${arch}.tgz"
    curl -fsSL -o "${tmp}/fc.tgz" "${fc_url}"
    tar -xzf "${tmp}/fc.tgz" -C "${tmp}"

    local installed_fc=""
    local installed_jailer=""

    # Handle both old (release-*/) and new (flat) archive structures
    if ls ${tmp}/release-*/firecracker-* &>/dev/null 2>&1; then
        local fc_src jailer_src
        fc_src="$(ls -1 ${tmp}/release-*/firecracker-* | head -n 1)"
        jailer_src="$(ls -1 ${tmp}/release-*/jailer-* | head -n 1)"
        install -m 0755 "${fc_src}" "${INSTALL_DIR}/bin"
        install -m 0755 "${jailer_src}" "${INSTALL_DIR}/bin"
        installed_fc="${INSTALL_DIR}/bin/$(basename "${fc_src}")"
        installed_jailer="${INSTALL_DIR}/bin/$(basename "${jailer_src}")"
    else
        install -m 0755 "${tmp}/firecracker-${FC_VERSION}-${arch}" "${INSTALL_DIR}/bin"
        install -m 0755 "${tmp}/jailer-${FC_VERSION}-${arch}" "${INSTALL_DIR}/bin"
        installed_fc="${INSTALL_DIR}/bin/firecracker-${FC_VERSION}-${arch}"
        installed_jailer="${INSTALL_DIR}/bin/jailer-${FC_VERSION}-${arch}"
    fi
    rm -rf "${tmp}"

    # Stable symlinks
    ln -sf "${installed_fc}" "${INSTALL_DIR}/bin/firecracker"
    ln -sf "${installed_jailer}" "${INSTALL_DIR}/bin/jailer"
    ln -sf "${INSTALL_DIR}/bin/firecracker" /usr/local/bin/firecracker
    ln -sf "${INSTALL_DIR}/bin/jailer" /usr/local/bin/jailer

    log "Firecracker installed: $(${INSTALL_DIR}/bin/firecracker --version 2>/dev/null | head -1)"
}

# ─── Kernel ──────────────────────────────────────────────
download_kernel() {
    log "Downloading kernel..."
    mkdir -p "${INSTALL_DIR}/kernel"
    local arch latest_version ci_version kernel_key
    arch="$(uname -m)"
    latest_version="$(latest_firecracker_version)"
    ci_version="${latest_version%.*}"

    kernel_key=$(curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${ci_version}/${arch}/vmlinux-&list-type=2" 2>/dev/null \
        | grep -oP "(?<=<Key>)(firecracker-ci/${ci_version}/${arch}/vmlinux-[0-9]+\\.[0-9]+\\.[0-9]{1,3})(?=</Key>)" \
        | sort -V | tail -1)

    # Fallback: try previous minor version
    if [[ -z "${kernel_key}" ]]; then
        local major_minor="${ci_version#v}"
        local major="${major_minor%.*}"
        local minor="${major_minor#*.}"
        if [[ ${minor} -gt 0 ]]; then
            local fallback_version="v${major}.$((minor - 1))"
            warn "Kernel not found for ${ci_version}, trying ${fallback_version}"
            kernel_key=$(curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${fallback_version}/${arch}/vmlinux-&list-type=2" 2>/dev/null \
                | grep -oP "(?<=<Key>)(firecracker-ci/${fallback_version}/${arch}/vmlinux-[0-9]+\\.[0-9]+\\.[0-9]{1,3})(?=</Key>)" \
                | sort -V | tail -1)
        fi
    fi

    if [[ -z "${kernel_key}" ]]; then
        err "Failed to locate Firecracker CI kernel for ${ci_version}/${arch}"
    fi

    curl -fsSL -o "${INSTALL_DIR}/kernel/vmlinux" "https://s3.amazonaws.com/spec.ccfc.min/${kernel_key}"
    log "Kernel downloaded: ${INSTALL_DIR}/kernel/vmlinux ($(du -h ${INSTALL_DIR}/kernel/vmlinux | cut -f1))"
}

# ─── Rootfs builders ─────────────────────────────────────
build_base_rootfs() {
    local output="${INSTALL_DIR}/rootfs/base.ext4"
    local mnt=$(mktemp -d)

    log "Building base rootfs (minimal, no distro)..."
    dd if=/dev/zero of="${output}" bs=1M count=32 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    mkdir -p "${mnt}"/{dev,proc,sys,tmp,code,usr/local/bin}

    if [[ -f "${INSTALL_DIR}/bin/nova-agent" ]]; then
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init"
        chmod +x "${mnt}/init"
    fi

    umount "${mnt}" && rmdir "${mnt}"
    log "base.ext4 ready ($(du -h ${output} | cut -f1)) - Go/Rust runtime"
}

build_python_rootfs() {
    local output="${INSTALL_DIR}/rootfs/python.ext4"
    local mnt=$(mktemp -d)

    log "Building python rootfs (Alpine + python3)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache python3" >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "python.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_wasm_rootfs() {
    local output="${INSTALL_DIR}/rootfs/wasm.ext4"
    local mnt=$(mktemp -d)

    log "Building wasm rootfs (Alpine + wasmtime)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp,usr/local/bin}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache libstdc++ gcompat" >/dev/null 2>&1

    curl -fsSL \
        "https://github.com/bytecodealliance/wasmtime/releases/download/${WASMTIME_VERSION}/wasmtime-${WASMTIME_VERSION}-x86_64-linux.tar.xz" \
        | tar -xJf - -C "${mnt}/usr/local/bin" --strip-components=1 --wildcards '*/wasmtime'

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "wasm.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_node_rootfs() {
    local output="${INSTALL_DIR}/rootfs/node.ext4"
    local mnt=$(mktemp -d)

    log "Building node rootfs (Alpine + nodejs)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache nodejs npm" >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "node.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_ruby_rootfs() {
    local output="${INSTALL_DIR}/rootfs/ruby.ext4"
    local mnt=$(mktemp -d)

    log "Building ruby rootfs (Alpine + ruby)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache ruby" >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "ruby.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_java_rootfs() {
    local output="${INSTALL_DIR}/rootfs/java.ext4"
    local mnt=$(mktemp -d)

    log "Building java rootfs (Alpine + OpenJDK)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_JAVA_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache openjdk21-jre-headless" >/dev/null 2>&1

    chroot "${mnt}" /bin/sh -c 'jli="$(find /usr/lib/jvm -name libjli.so | head -n1)"; [ -n "$jli" ] && ln -sf "$jli" /usr/lib/libjli.so' >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "java.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_php_rootfs() {
    local output="${INSTALL_DIR}/rootfs/php.ext4"
    local mnt=$(mktemp -d)

    log "Building php rootfs (Alpine + php)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache php" >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "php.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_dotnet_rootfs() {
    local output="${INSTALL_DIR}/rootfs/dotnet.ext4"
    local mnt=$(mktemp -d)

    log "Building dotnet rootfs (Alpine + .NET runtime)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp,usr/share/dotnet}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache ca-certificates-bundle libgcc libssl3 libstdc++ zlib" >/dev/null 2>&1

    local dotnet_tar
    dotnet_tar="$(mktemp /tmp/dotnet-runtime.XXXXXX.tar.gz)"

    curl -fsSL \
        "https://builds.dotnet.microsoft.com/dotnet/Runtime/${DOTNET_VERSION}/dotnet-runtime-${DOTNET_VERSION}-linux-musl-x64.tar.gz" \
        -o "${dotnet_tar}"
    tar -xzf "${dotnet_tar}" -C "${mnt}/usr/share/dotnet"
    ln -sf /usr/share/dotnet/dotnet "${mnt}/usr/bin/dotnet"
    rm -f "${dotnet_tar}"

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "dotnet.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_deno_rootfs() {
    local output="${INSTALL_DIR}/rootfs/deno.ext4"
    local mnt=$(mktemp -d)

    log "Building deno rootfs (Alpine + deno)..."

    # Build in a temp directory instead of a mounted ext4 image.
    # build-base (gcc) temporarily needs more space than the 256MB rootfs allows.
    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{dev,code,tmp,usr/local/bin}
    mknod -m 666 "${mnt}/dev/null" c 1 3 2>/dev/null || true
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache libstdc++ gcompat" >/dev/null 2>&1

    # gcompat does not provide __res_init (glibc resolver symbol);
    # build a minimal stub so the dynamic linker can resolve it.
    chroot "${mnt}" /bin/sh -c "apk add --no-cache build-base" >/dev/null 2>&1
    printf 'int __res_init(void){return 0;}\n' > "${mnt}/tmp/res_stub.c"
    chroot "${mnt}" /bin/sh -c "gcc -shared -o /lib/libresolv_stub.so /tmp/res_stub.c"
    rm -f "${mnt}/tmp/res_stub.c"
    chroot "${mnt}" /bin/sh -c "apk del build-base" >/dev/null 2>&1

    local deno_zip
    deno_zip="$(mktemp /tmp/deno.XXXXXX.zip)"

    curl -fsSL \
        "https://github.com/denoland/deno/releases/download/${DENO_VERSION}/deno-x86_64-unknown-linux-gnu.zip" \
        -o "${deno_zip}"
    unzip -q -o "${deno_zip}" -d "${mnt}/usr/local/bin"
    chmod +x "${mnt}/usr/local/bin/deno"
    rm -f "${deno_zip}"

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    # Create the ext4 image from the populated directory.
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "deno.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_bun_rootfs() {
    local output="${INSTALL_DIR}/rootfs/bun.ext4"
    local mnt=$(mktemp -d)

    log "Building bun rootfs (Alpine + bun)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp,usr/local/bin}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache libgcc libstdc++" >/dev/null 2>&1

    local bun_zip bun_extract
    bun_zip="$(mktemp /tmp/bun.XXXXXX.zip)"
    bun_extract="$(mktemp -d /tmp/bun-extract.XXXXXX)"

    curl -fsSL \
        "https://github.com/oven-sh/bun/releases/download/${BUN_VERSION}/bun-linux-x64-musl.zip" \
        -o "${bun_zip}"
    unzip -q -o "${bun_zip}" -d "${bun_extract}"
    cp "${bun_extract}/bun-linux-x64-musl/bun" "${mnt}/usr/local/bin/bun"
    chmod +x "${mnt}/usr/local/bin/bun"
    rm -rf "${bun_zip}" "${bun_extract}"

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "bun.ext4 ready ($(du -h ${output} | cut -f1))"
}

# Build all rootfs images in controlled parallel batches.
# Dependency order is preserved at stage level (this runs only after binaries are deployed).
build_rootfs_images() {
    local max_jobs="${NOVA_ROOTFS_JOBS:-$(default_parallel_jobs)}"
    if ! [[ "${max_jobs}" =~ ^[0-9]+$ ]] || [[ "${max_jobs}" -lt 1 ]]; then
        max_jobs="$(default_parallel_jobs)"
    fi

    local builders=(
        build_base_rootfs
        build_python_rootfs
        build_wasm_rootfs
        build_node_rootfs
        build_ruby_rootfs
        build_java_rootfs
        build_php_rootfs
        build_dotnet_rootfs
        build_deno_rootfs
        build_bun_rootfs
    )

    if [[ "${max_jobs}" -eq 1 ]]; then
        log "Building rootfs images sequentially (NOVA_ROOTFS_JOBS=1)..."
        local fn
        for fn in "${builders[@]}"; do
            "${fn}"
        done
        return
    fi

    log "Building rootfs images with concurrency=${max_jobs}..."
    local total=${#builders[@]}
    local i=0

    while [[ ${i} -lt ${total} ]]; do
        local -a pids=()
        local -a names=()
        local launched=0

        while [[ ${launched} -lt ${max_jobs} && ${i} -lt ${total} ]]; do
            local fn="${builders[$i]}"
            info "  [start] ${fn}"
            "${fn}" &
            pids+=("$!")
            names+=("${fn}")
            i=$((i + 1))
            launched=$((launched + 1))
        done

        local idx
        local batch_failed=0
        local batch_failed_names=""
        for idx in "${!pids[@]}"; do
            if wait "${pids[$idx]}"; then
                log "  [done] ${names[$idx]}"
            else
                batch_failed=$((batch_failed + 1))
                if [[ -z "${batch_failed_names}" ]]; then
                    batch_failed_names="${names[$idx]}"
                else
                    batch_failed_names="${batch_failed_names}, ${names[$idx]}"
                fi
            fi
        done
        if [[ "${batch_failed}" -gt 0 ]]; then
            err "Rootfs build failed (${batch_failed} task(s)): ${batch_failed_names}"
        fi
    done
}

# ─── Backend Services ────────────────────────────────────
deploy_backend_services() {
    log "Deploying backend services..."

    # Copy binaries
    install -m 0755 "${NOVA_BIN}" "${INSTALL_DIR}/bin/nova"
    install -m 0755 "${COMET_BIN}" "${INSTALL_DIR}/bin/comet"
    install -m 0755 "${ZENITH_BIN}" "${INSTALL_DIR}/bin/zenith"
    install -m 0755 "${AGENT_BIN}" "${INSTALL_DIR}/bin/nova-agent"
    log "Installed backend binaries to ${INSTALL_DIR}/bin/"

    # Generate config
    mkdir -p "${INSTALL_DIR}/configs"
    cat > "${INSTALL_DIR}/configs/nova.json" << 'EOF'
{
  "postgres": {
    "dsn": "postgres://nova:nova@localhost:5432/nova?sslmode=disable"
  },
  "firecracker": {
    "backend": "firecracker",
    "binary": "/opt/nova/bin/firecracker",
    "kernel": "/opt/nova/kernel/vmlinux",
    "rootfs_dir": "/opt/nova/rootfs",
    "snapshot_dir": "/opt/nova/snapshots",
    "socket_dir": "/tmp/nova/sockets",
    "vsock_dir": "/tmp/nova/vsock",
    "log_dir": "/tmp/nova/logs"
  },
  "pool": {
    "idle_ttl": 60000000000
  }
}
EOF
    log "Generated config at ${INSTALL_DIR}/configs/nova.json"

    # Create systemd services
    cat > /etc/systemd/system/nova.service << 'EOF'
[Unit]
Description=Nova Control Plane
After=postgresql.service network.target
Requires=postgresql.service

[Service]
Type=simple
ExecStart=/opt/nova/bin/nova daemon --config /opt/nova/configs/nova.json --http 127.0.0.1:9001
Restart=on-failure
RestartSec=5
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt/nova/bin

[Install]
WantedBy=multi-user.target
EOF
    log "Created systemd service: nova.service"

    cat > /etc/systemd/system/comet.service << 'EOF'
[Unit]
Description=Comet Data Plane
After=postgresql.service network.target
Requires=postgresql.service

[Service]
Type=simple
ExecStart=/opt/nova/bin/comet daemon --config /opt/nova/configs/nova.json --grpc 127.0.0.1:9090
Restart=on-failure
RestartSec=5
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt/nova/bin

[Install]
WantedBy=multi-user.target
EOF
    log "Created systemd service: comet.service"

    cat > /etc/systemd/system/zenith.service << 'EOF'
[Unit]
Description=Zenith Gateway
After=nova.service comet.service network.target
Requires=nova.service comet.service

[Service]
Type=simple
ExecStart=/opt/nova/bin/zenith serve --listen :9000 --nova-url http://127.0.0.1:9001 --comet-grpc 127.0.0.1:9090
Restart=on-failure
RestartSec=5
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt/nova/bin

[Install]
WantedBy=multi-user.target
EOF
    log "Created systemd service: zenith.service"
}

# ─── Lumen Frontend ──────────────────────────────────────
deploy_lumen_frontend() {
    log "Deploying Lumen frontend..."

    local lumen_src="${DEPLOY_DIR}/lumen"

    if [[ ! -d "${lumen_src}" ]]; then
        err "Lumen source directory not found at ${lumen_src}"
    fi

    # Build Lumen
    log "Building Lumen (this may take a while)..."
    cd "${lumen_src}"
    npm install --silent 2>/dev/null
    npm run build 2>/dev/null

    # Deploy standalone build
    mkdir -p "${INSTALL_DIR}/lumen"

    # Copy standalone output
    if [[ -d "${lumen_src}/.next/standalone" ]]; then
        cp -r "${lumen_src}/.next/standalone/." "${INSTALL_DIR}/lumen/"
        # Copy static files
        if [[ -d "${lumen_src}/.next/static" ]]; then
            mkdir -p "${INSTALL_DIR}/lumen/.next/static"
            cp -r "${lumen_src}/.next/static/." "${INSTALL_DIR}/lumen/.next/static/"
        fi
        # Copy public files if they exist
        if [[ -d "${lumen_src}/public" ]]; then
            cp -r "${lumen_src}/public" "${INSTALL_DIR}/lumen/"
        fi
        log "Deployed Lumen standalone build to ${INSTALL_DIR}/lumen/"
    else
        err "Next.js standalone build not found. Make sure next.config.ts has output: 'standalone'"
    fi

    # Create systemd service
    rm -f /etc/systemd/system/nova-lumen.service
    cat > /etc/systemd/system/lumen.service << 'EOF'
[Unit]
Description=Lumen Dashboard
After=zenith.service network.target
Requires=zenith.service

[Service]
Type=simple
WorkingDirectory=/opt/nova/lumen
Environment=BACKEND_URL=http://127.0.0.1:9000
Environment=HOSTNAME=0.0.0.0
Environment=PORT=3000
ExecStart=/usr/bin/node server.js
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
    log "Created systemd service: lumen.service"
}

# ─── Start Services ──────────────────────────────────────
start_services() {
    log "Starting services..."

    systemctl daemon-reload

    # Enable and start PostgreSQL (should already be running)
    systemctl enable postgresql >/dev/null 2>&1 || true

    # Enable and start Nova
    systemctl enable nova >/dev/null 2>&1
    systemctl start nova

    # Wait for Nova to be ready
    local retries=10
    while ! curl -sf http://localhost:9001/health >/dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ ${retries} -eq 0 ]]; then
            warn "Nova health check failed - check logs with: journalctl -u nova"
            break
        fi
        sleep 1
    done

    # Enable and start Comet
    systemctl enable comet >/dev/null 2>&1
    systemctl start comet

    # Wait for Comet gRPC to be ready
    retries=10
    while ! ss -lnt | grep -q ':9090 '; do
        retries=$((retries - 1))
        if [[ ${retries} -eq 0 ]]; then
            warn "Comet port check failed - check logs with: journalctl -u comet"
            break
        fi
        sleep 1
    done

    # Enable and start Zenith
    systemctl enable zenith >/dev/null 2>&1
    systemctl start zenith

    # Wait for Zenith to be ready
    retries=10
    while ! curl -sf http://localhost:9000/health >/dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ ${retries} -eq 0 ]]; then
            warn "Zenith health check failed - check logs with: journalctl -u zenith"
            break
        fi
        sleep 1
    done

    # Enable and start Lumen
    systemctl enable lumen >/dev/null 2>&1
    systemctl start lumen

    # Wait for Lumen to be ready
    retries=10
    while ! curl -sf http://localhost:3000 >/dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ ${retries} -eq 0 ]]; then
            warn "Lumen health check failed - check logs with: journalctl -u lumen"
            break
        fi
        sleep 1
    done
}

# ─── Create Sample Functions ─────────────────────────────
create_sample_functions() {
    log "Creating sample functions for all runtimes..."

    local api="http://localhost:9000"
    local seed_script="${SCRIPT_DIR}/seed-functions.sh"
    local skip_compiled="${SKIP_COMPILED:-0}"

    # Wait for API to be ready
    local retries=10
    while ! curl -sf "${api}/health" >/dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ ${retries} -eq 0 ]]; then
            warn "API not ready, skipping sample functions"
            return
        fi
        sleep 2
    done

    if [[ ! -f "${seed_script}" ]]; then
        warn "seed-functions.sh not found at ${seed_script}, skipping sample functions"
        return
    fi

    info "  Seeding bootstrap-compatible sample handlers..."
    SKIP_WORKFLOWS=1 SKIP_COMPILED="${skip_compiled}" bash "${seed_script}" "${api}" || warn "sample function seeding reported errors"

    log "Sample functions created (bootstrap-compatible handler format)"
}

# ─── Print Summary ───────────────────────────────────────
print_summary() {
    local server_ip
    server_ip=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "localhost")

    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  Nova Deployment Complete${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "  Services:"
    echo "  ---------"

    local pg_status nova_status comet_status zenith_status lumen_status
    pg_status=$(systemctl is-active postgresql 2>/dev/null || echo "unknown")
    nova_status=$(systemctl is-active nova 2>/dev/null || echo "unknown")
    comet_status=$(systemctl is-active comet 2>/dev/null || echo "unknown")
    zenith_status=$(systemctl is-active zenith 2>/dev/null || echo "unknown")
    lumen_status=$(systemctl is-active lumen 2>/dev/null || echo "unknown")

    if [[ "${pg_status}" == "active" ]]; then
        echo -e "  ${GREEN}[OK]${NC} PostgreSQL      - running"
    else
        echo -e "  ${RED}[!!]${NC} PostgreSQL      - ${pg_status}"
    fi

    if [[ "${nova_status}" == "active" ]]; then
        echo -e "  ${GREEN}[OK]${NC} nova            - control plane on 127.0.0.1:9001"
    else
        echo -e "  ${RED}[!!]${NC} nova            - ${nova_status}"
    fi

    if [[ "${comet_status}" == "active" ]]; then
        echo -e "  ${GREEN}[OK]${NC} comet           - data plane gRPC on 127.0.0.1:9090"
    else
        echo -e "  ${RED}[!!]${NC} comet           - ${comet_status}"
    fi

    if [[ "${zenith_status}" == "active" ]]; then
        echo -e "  ${GREEN}[OK]${NC} zenith          - gateway on port 9000"
    else
        echo -e "  ${RED}[!!]${NC} zenith          - ${zenith_status}"
    fi

    if [[ "${lumen_status}" == "active" ]]; then
        echo -e "  ${GREEN}[OK]${NC} lumen           - running on port 3000"
    else
        echo -e "  ${RED}[!!]${NC} lumen           - ${lumen_status}"
    fi

    echo ""
    echo "  Access URLs:"
    echo "  ------------"
    echo "  Dashboard:  http://${server_ip}:3000"
    echo "  API:        http://${server_ip}:9000 (Zenith)"
    echo "  Health:     http://${server_ip}:9000/health"
    echo ""
    echo "  Installation Directory: ${INSTALL_DIR}"
    echo ""
    echo "  Useful Commands:"
    echo "  ----------------"
    echo "  journalctl -u nova -f          # View Nova control plane logs"
    echo "  journalctl -u comet -f         # View Comet data plane logs"
    echo "  journalctl -u zenith -f        # View Zenith gateway logs"
    echo "  journalctl -u lumen -f         # View Lumen logs"
    echo "  systemctl restart nova         # Restart Nova"
    echo "  systemctl restart comet        # Restart Comet"
    echo "  systemctl restart zenith       # Restart Zenith"
    echo "  systemctl restart lumen        # Restart Lumen"
    echo ""

    # Health check
    if curl -sf http://localhost:9000/health >/dev/null 2>&1; then
        echo -e "  ${GREEN}API Health Check: OK${NC}"
    else
        echo -e "  ${RED}API Health Check: FAILED${NC}"
        echo "  Run 'journalctl -u zenith' to check for errors"
    fi
    echo ""
}

# ─── Main ────────────────────────────────────────────────
main() {
    echo ""
    echo -e "${BLUE}Nova Serverless Platform - One-Click Deployment${NC}"
    echo ""

    check_root
    check_system
    check_binaries

    install_deps
    install_nodejs

    # Reset previous installation state.
    reset_installation_state

    # Create directories
    log "Setting up directories..."
    mkdir -p "${INSTALL_DIR}"/{kernel,rootfs,bin,snapshots,configs,lumen}
    mkdir -p /tmp/nova/{sockets,vsock,logs}
    chmod 755 "${INSTALL_DIR}" "${INSTALL_DIR}"/{kernel,rootfs,bin,snapshots,configs,lumen}

    # PostgreSQL
    install_postgres
    setup_database

    # Firecracker install and kernel download are independent.
    log "Running Firecracker + kernel setup in parallel..."
    run_parallel_functions install_firecracker download_kernel

    # Deploy backend binaries first (so agent is available for rootfs)
    deploy_backend_services
    cleanup_deploy_build_binaries

    # Build steps are independent at this stage.
    log "Running rootfs build + Lumen build in parallel..."
    run_parallel_functions build_rootfs_images deploy_lumen_frontend

    # Set KVM permissions
    chmod 666 /dev/kvm 2>/dev/null || true

    # Start all services
    start_services

    # Create sample functions via API
    create_sample_functions

    # Print summary
    print_summary
}

main "$@"
