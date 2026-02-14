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

set -euxo pipefail

PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt/nova/bin"
export PATH

INSTALL_DIR="/opt/nova"
FC_VERSION="latest"
ALPINE_URL="https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/x86_64/alpine-minirootfs-3.23.3-x86_64.tar.gz"
WASMTIME_VERSION="v41.0.1"
DENO_VERSION="v2.6.7"
BUN_VERSION="bun-v1.3.8"
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

hash_stdin() {
    if command -v sha256sum &>/dev/null; then
        sha256sum | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 | awk '{print $1}'
    else
        err "Neither sha256sum nor shasum found"
    fi
}

hash_file() {
    local file="$1"
    [[ -f "${file}" ]] || return 1

    if command -v sha256sum &>/dev/null; then
        sha256sum "${file}" | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "${file}" | awk '{print $1}'
    else
        err "Neither sha256sum nor shasum found"
    fi
}

hash_string() {
    local text="$1"
    printf '%s' "${text}" | hash_stdin
}

hash_directory() {
    local dir="$1"
    [[ -d "${dir}" ]] || return 1

    (
        cd "${dir}" || exit 1
        local files
        files="$(find . -type f | LC_ALL=C sort)"
        if [[ -z "${files}" ]]; then
            printf '__empty__'
            exit 0
        fi

        local file sum
        while IFS= read -r file; do
            [[ -n "${file}" ]] || continue
            sum="$(hash_file "${file}")" || exit 1
            printf '%s  %s\n' "${sum}" "${file}"
        done <<< "${files}"
    ) | hash_stdin
}

install_file_if_changed() {
    local source="$1"
    local target="$2"
    local label="$3"
    local source_hash target_hash

    source_hash="$(hash_file "${source}")" || err "Missing source file for ${label}: ${source}"
    if [[ -f "${target}" ]]; then
        target_hash="$(hash_file "${target}" 2>/dev/null || true)"
    else
        target_hash=""
    fi

    if [[ -n "${target_hash}" && "${source_hash}" == "${target_hash}" ]]; then
        info "  [skip] ${label} unchanged"
        return 0
    fi

    install -m 0755 "${source}" "${target}"
    info "  [update] ${label}"
}

read_manifest() {
    local manifest_file="$1"
    [[ -f "${manifest_file}" ]] || return 1

    cat "${manifest_file}" 2>/dev/null || return 1
}

write_manifest() {
    local manifest_file="$1"
    local value="$2"
    mkdir -p "$(dirname "${manifest_file}")"
    printf '%s\n' "${value}" > "${manifest_file}"
}

# Run functions sequentially and fail fast on first error.
run_sequential_functions() {
    local fn
    for fn in "$@"; do
        info "  [start] ${fn}"
        "${fn}"
        log "  [done] ${fn}"
    done
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
    if [[ "${NOVA_CLEAN_DEPLOY_BINARIES:-0}" != "1" ]]; then
        info "Skipping deployment build binary cleanup (set NOVA_CLEAN_DEPLOY_BINARIES=1 to enable)"
        return 0
    fi

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
    log "Preparing installation state..."

    stop_existing_services

    # Keep deployed artifacts for incremental runs; only clear transient runtime state.
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
prepare_chroot_dev() {
    local root="$1"
    mkdir -p "${root}/dev"
    mknod -m 666 "${root}/dev/null" c 1 3 2>/dev/null || true
    mknod -m 666 "${root}/dev/zero" c 1 5 2>/dev/null || true
    mknod -m 666 "${root}/dev/random" c 1 8 2>/dev/null || true
    mknod -m 666 "${root}/dev/urandom" c 1 9 2>/dev/null || true
    mknod -m 666 "${root}/dev/tty" c 5 0 2>/dev/null || true
}

# Remove device nodes before mkfs.ext4 -d (some e2fsprogs versions
# cannot copy device special files into the new filesystem image).
# The guest VM mounts its own devtmpfs, so these nodes are not needed.
cleanup_chroot_dev() {
    local root="$1"
    rm -f "${root}/dev/null" "${root}/dev/zero" "${root}/dev/random" \
          "${root}/dev/urandom" "${root}/dev/tty" 2>/dev/null || true
}

build_base_rootfs() {
    local output="${INSTALL_DIR}/rootfs/base.ext4"
    local mnt=$(mktemp -d)

    log "Building base rootfs (minimal, no distro)..."

    mkdir -p "${mnt}"/{dev,proc,sys,tmp,code,usr/local/bin}

    if [[ -f "${INSTALL_DIR}/bin/nova-agent" ]]; then
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init"
        chmod +x "${mnt}/init"
    fi

    dd if=/dev/zero of="${output}" bs=1M count=32 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "base.ext4 ready ($(du -h ${output} | cut -f1)) - Go/Rust runtime"
}

build_python_rootfs() {
    local output="${INSTALL_DIR}/rootfs/python.ext4"
    local mnt=$(mktemp -d)

    log "Building python rootfs (Alpine + python3)..."

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    prepare_chroot_dev "${mnt}"
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache python3" >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    cleanup_chroot_dev "${mnt}"
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "python.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_wasm_rootfs() {
    local output="${INSTALL_DIR}/rootfs/wasm.ext4"
    local mnt=$(mktemp -d)

    log "Building wasm rootfs (Alpine + wasmtime)..."

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp,usr/local/bin}
    prepare_chroot_dev "${mnt}"
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache libstdc++ gcompat" >/dev/null 2>&1

    curl -fsSL \
        "https://github.com/bytecodealliance/wasmtime/releases/download/${WASMTIME_VERSION}/wasmtime-${WASMTIME_VERSION}-x86_64-linux.tar.xz" \
        | tar -xJf - -C "${mnt}/usr/local/bin" --strip-components=1 --wildcards '*/wasmtime'

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    cleanup_chroot_dev "${mnt}"
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "wasm.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_node_rootfs() {
    local output="${INSTALL_DIR}/rootfs/node.ext4"
    local mnt=$(mktemp -d)

    log "Building node rootfs (Alpine + nodejs)..."

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    prepare_chroot_dev "${mnt}"
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache nodejs npm" >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    cleanup_chroot_dev "${mnt}"
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "node.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_ruby_rootfs() {
    local output="${INSTALL_DIR}/rootfs/ruby.ext4"
    local mnt=$(mktemp -d)

    log "Building ruby rootfs (Alpine + ruby)..."

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    prepare_chroot_dev "${mnt}"
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache ruby" >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    cleanup_chroot_dev "${mnt}"
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "ruby.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_java_rootfs() {
    local output="${INSTALL_DIR}/rootfs/java.ext4"
    local mnt=$(mktemp -d)

    log "Building java rootfs (Alpine + OpenJDK)..."

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    prepare_chroot_dev "${mnt}"
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache openjdk21-jre-headless" >/dev/null 2>&1

    chroot "${mnt}" /bin/sh -c 'jli="$(find /usr/lib/jvm -name libjli.so | head -n1)"; [ -n "$jli" ] && ln -sf "$jli" /usr/lib/libjli.so' >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    # Java needs more space
    cleanup_chroot_dev "${mnt}"
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_JAVA_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "java.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_php_rootfs() {
    local output="${INSTALL_DIR}/rootfs/php.ext4"
    local mnt=$(mktemp -d)

    log "Building php rootfs (Alpine + php)..."

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    prepare_chroot_dev "${mnt}"
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache php" >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    cleanup_chroot_dev "${mnt}"
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "php.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_lua_rootfs() {
    local output="${INSTALL_DIR}/rootfs/lua.ext4"
    local mnt=$(mktemp -d)

    log "Building lua rootfs (Alpine + lua)..."

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    prepare_chroot_dev "${mnt}"
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache lua5.4" >/dev/null 2>&1

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    cleanup_chroot_dev "${mnt}"
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "lua.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_deno_rootfs() {
    local output="${INSTALL_DIR}/rootfs/deno.ext4"
    local rootfs_dir=$(mktemp -d)

    log "Building deno rootfs (Alpine + deno)..."

    # Build in a temp directory instead of a mounted ext4 image.
    # build-base (gcc) temporarily needs more space than the 256MB rootfs allows.
    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${rootfs_dir}"
    mkdir -p "${rootfs_dir}"/{code,tmp,usr/local/bin}
    prepare_chroot_dev "${rootfs_dir}"
    echo "nameserver 8.8.8.8" > "${rootfs_dir}/etc/resolv.conf"

    chroot "${rootfs_dir}" /bin/sh -c "apk add --no-cache libstdc++ gcompat" >/dev/null 2>&1

    # gcompat does not provide __res_init (glibc resolver symbol);
    # build a minimal stub so the dynamic linker can resolve it.
    chroot "${rootfs_dir}" /bin/sh -c "apk add --no-cache build-base" >/dev/null 2>&1
    printf 'int __res_init(void){return 0;}\n' > "${rootfs_dir}/tmp/res_stub.c"
    chroot "${rootfs_dir}" /bin/sh -c "gcc -shared -o /lib/libresolv_stub.so /tmp/res_stub.c"
    rm -f "${rootfs_dir}/tmp/res_stub.c"
    chroot "${rootfs_dir}" /bin/sh -c "apk del build-base" >/dev/null 2>&1

    local deno_zip
    deno_zip="$(mktemp /tmp/deno.XXXXXX.zip)"

    curl -fsSL \
        "https://github.com/denoland/deno/releases/download/${DENO_VERSION}/deno-x86_64-unknown-linux-gnu.zip" \
        -o "${deno_zip}"
    unzip -q -o "${deno_zip}" -d "${rootfs_dir}/usr/local/bin"
    chmod +x "${rootfs_dir}/usr/local/bin/deno"
    rm -f "${deno_zip}"

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${rootfs_dir}/init" && \
        chmod +x "${rootfs_dir}/init"

    # Create the ext4 image from the populated directory.
    cleanup_chroot_dev "${rootfs_dir}"
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${rootfs_dir}" "${output}" >/dev/null
    rm -rf "${rootfs_dir}"
    log "deno.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_bun_rootfs() {
    local output="${INSTALL_DIR}/rootfs/bun.ext4"
    local mnt=$(mktemp -d)

    log "Building bun rootfs (Alpine + bun)..."

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp,usr/local/bin}
    prepare_chroot_dev "${mnt}"
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

    cleanup_chroot_dev "${mnt}"
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "bun.ext4 ready ($(du -h ${output} | cut -f1))"
}

rootfs_manifest_path() {
    echo "${INSTALL_DIR}/rootfs/.build-manifest"
}

rootfs_build_fingerprint() {
    local agent_hash script_hash
    agent_hash="$(hash_file "${AGENT_BIN}" 2>/dev/null || echo "missing-agent")"
    script_hash="$(hash_file "${SCRIPT_DIR}/setup.sh" 2>/dev/null || echo "missing-script")"

    hash_string "agent=${agent_hash};alpine=${ALPINE_URL};wasmtime=${WASMTIME_VERSION};deno=${DENO_VERSION};bun=${BUN_VERSION};rootfs=${ROOTFS_SIZE_MB};rootfs_java=${ROOTFS_SIZE_JAVA_MB};script=${script_hash}"
}

rootfs_images_exist() {
    local -a images=(
        "base.ext4"
        "python.ext4"
        "wasm.ext4"
        "node.ext4"
        "ruby.ext4"
        "java.ext4"
        "php.ext4"
        "lua.ext4"
        "deno.ext4"
        "bun.ext4"
    )

    local image
    for image in "${images[@]}"; do
        [[ -f "${INSTALL_DIR}/rootfs/${image}" ]] || return 1
    done
    return 0
}

rootfs_is_up_to_date() {
    local manifest_file expected current
    manifest_file="$(rootfs_manifest_path)"
    rootfs_images_exist || return 1

    expected="$(read_manifest "${manifest_file}" || true)"
    [[ -n "${expected}" ]] || return 1
    current="$(rootfs_build_fingerprint)"

    [[ "${expected}" == "${current}" ]]
}

write_rootfs_manifest() {
    write_manifest "$(rootfs_manifest_path)" "$(rootfs_build_fingerprint)"
}

# Build all rootfs images sequentially.
# Dependency order is preserved at stage level (this runs only after binaries are deployed).
build_rootfs_images() {
    if rootfs_is_up_to_date; then
        log "Rootfs artifacts unchanged, skipping rebuild"
        return 0
    fi

    local builders=(
        build_base_rootfs
        build_python_rootfs
        build_wasm_rootfs
        build_node_rootfs
        build_ruby_rootfs
        build_java_rootfs
        build_php_rootfs
        build_lua_rootfs
        build_deno_rootfs
        build_bun_rootfs
    )

    log "Building rootfs images sequentially..."
    local fn
    for fn in "${builders[@]}"; do
        info "  [start] ${fn}"
        "${fn}"
        log "  [done] ${fn}"
    done

    write_rootfs_manifest
    log "Updated rootfs build manifest"
}

# ─── Backend Services ────────────────────────────────────
deploy_backend_services() {
    log "Deploying backend services..."

    # Copy binaries only when content changed.
    install_file_if_changed "${NOVA_BIN}" "${INSTALL_DIR}/bin/nova" "nova binary"
    install_file_if_changed "${COMET_BIN}" "${INSTALL_DIR}/bin/comet" "comet binary"
    install_file_if_changed "${ZENITH_BIN}" "${INSTALL_DIR}/bin/zenith" "zenith binary"
    install_file_if_changed "${AGENT_BIN}" "${INSTALL_DIR}/bin/nova-agent" "nova-agent binary"
    log "Backend binary deployment check complete"

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
lumen_manifest_path() {
    echo "${INSTALL_DIR}/lumen/.build-manifest"
}

lumen_source_fingerprint() {
    local lumen_src="$1"
    [[ -d "${lumen_src}" ]] || return 1

    (
        cd "${lumen_src}" || exit 1
        local files
        files="$(find . \
            -path './node_modules' -prune -o \
            -path './.next' -prune -o \
            -type f -print | LC_ALL=C sort)"

        if [[ -z "${files}" ]]; then
            printf '__empty__'
            exit 0
        fi

        local file sum
        while IFS= read -r file; do
            [[ -n "${file}" ]] || continue
            sum="$(hash_file "${file}")" || exit 1
            printf '%s  %s\n' "${sum}" "${file}"
        done <<< "${files}"
    ) | hash_stdin
}

lumen_is_up_to_date() {
    local lumen_src="$1"
    local manifest_file expected current

    manifest_file="$(lumen_manifest_path)"
    [[ -f "${INSTALL_DIR}/lumen/server.js" ]] || return 1
    expected="$(read_manifest "${manifest_file}" || true)"
    [[ -n "${expected}" ]] || return 1

    current="$(lumen_source_fingerprint "${lumen_src}")" || return 1
    [[ "${expected}" == "${current}" ]]
}

deploy_lumen_frontend() {
    log "Deploying Lumen frontend..."

    local lumen_src="${DEPLOY_DIR}/lumen"
    local source_fingerprint

    if [[ ! -d "${lumen_src}" ]]; then
        err "Lumen source directory not found at ${lumen_src}"
    fi

    if lumen_is_up_to_date "${lumen_src}"; then
        log "Lumen source unchanged, skipping npm build and deploy"
    else
        source_fingerprint="$(lumen_source_fingerprint "${lumen_src}")" || err "Failed to compute Lumen source fingerprint"

        # Build Lumen
        log "Building Lumen (this may take a while)..."
        local npm_log
        npm_log="$(mktemp -t lumen-build.XXXXXX.log)"
        (
            cd "${lumen_src}" || exit 1
            npm install --silent 2>&1 | tee -a "${npm_log}"
            npm run build 2>&1 | tee -a "${npm_log}"
        ) || {
            warn "Lumen build failed. Log output:"
            tail -30 "${npm_log}" >&2
            err "npm build failed for Lumen frontend"
        }
        rm -f "${npm_log}"

        # Deploy standalone build
        rm -rf "${INSTALL_DIR}/lumen"
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
            write_manifest "$(lumen_manifest_path)" "${source_fingerprint}"
            log "Deployed Lumen standalone build to ${INSTALL_DIR}/lumen/"
        else
            err "Next.js standalone build not found. Make sure next.config.ts has output: 'standalone'"
        fi
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

    # Firecracker install and kernel download.
    log "Running Firecracker + kernel setup..."
    run_sequential_functions install_firecracker download_kernel

    # Deploy backend binaries first (so agent is available for rootfs)
    deploy_backend_services
    cleanup_deploy_build_binaries

    # Build steps run sequentially for reliability.
    log "Running rootfs build + Lumen build..."
    run_sequential_functions build_rootfs_images deploy_lumen_frontend

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
