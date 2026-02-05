#!/bin/bash
# Nova Serverless Platform - One-Click Deployment Script
#
# This script deploys the complete Nova platform on a Linux x86_64 server:
# - PostgreSQL database + schema initialization
# - Nova backend (daemon mode, Firecracker backend)
# - Lumen frontend (Next.js standalone)
# - Three systemd services, enabled at boot
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
    local agent_bin=""

    # Look for binaries in deployment directory first
    if [[ -f "${DEPLOY_DIR}/bin/nova-linux" ]]; then
        nova_bin="${DEPLOY_DIR}/bin/nova-linux"
    elif [[ -f "${DEPLOY_DIR}/bin/nova" ]]; then
        nova_bin="${DEPLOY_DIR}/bin/nova"
    fi

    if [[ -f "${DEPLOY_DIR}/bin/nova-agent" ]]; then
        agent_bin="${DEPLOY_DIR}/bin/nova-agent"
    fi

    if [[ -z "${nova_bin}" ]] || [[ -z "${agent_bin}" ]]; then
        err "Nova binaries not found. Please run 'make build-linux' first.
Expected binaries at:
  ${DEPLOY_DIR}/bin/nova-linux (or nova)
  ${DEPLOY_DIR}/bin/nova-agent"
    fi

    log "Found Nova binary: ${nova_bin}"
    log "Found Agent binary: ${agent_bin}"

    # Export for later use
    export NOVA_BIN="${nova_bin}"
    export AGENT_BIN="${agent_bin}"
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
    log "Setting up Nova database..."

    # Create user if not exists
    if ! su - postgres -c "psql -tAc \"SELECT 1 FROM pg_roles WHERE rolname='nova'\"" | grep -q 1; then
        su - postgres -c "psql -c \"CREATE USER nova WITH PASSWORD 'nova';\""
        log "Created database user: nova"
    fi

    # Create database if not exists
    if ! su - postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='nova'\"" | grep -q 1; then
        su - postgres -c "psql -c \"CREATE DATABASE nova OWNER nova;\""
        log "Created database: nova"
    fi

    # Run schema initialization
    if [[ -f "${SCRIPT_DIR}/init-db.sql" ]]; then
        su - postgres -c "psql -d nova" < "${SCRIPT_DIR}/init-db.sql" >/dev/null 2>&1
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

    curl -fsSL \
        "https://builds.dotnet.microsoft.com/dotnet/Runtime/${DOTNET_VERSION}/dotnet-runtime-${DOTNET_VERSION}-linux-musl-x64.tar.gz" \
        -o /tmp/dotnet-runtime.tar.gz
    tar -xzf /tmp/dotnet-runtime.tar.gz -C "${mnt}/usr/share/dotnet"
    ln -sf /usr/share/dotnet/dotnet "${mnt}/usr/bin/dotnet"
    rm -f /tmp/dotnet-runtime.tar.gz

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
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp,usr/local/bin}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache libstdc++ gcompat" >/dev/null 2>&1

    curl -fsSL \
        "https://github.com/denoland/deno/releases/download/${DENO_VERSION}/deno-x86_64-unknown-linux-gnu.zip" \
        -o /tmp/deno.zip
    unzip -q -o /tmp/deno.zip -d "${mnt}/usr/local/bin"
    chmod +x "${mnt}/usr/local/bin/deno"
    rm -f /tmp/deno.zip

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
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

    curl -fsSL \
        "https://github.com/oven-sh/bun/releases/download/${BUN_VERSION}/bun-linux-x64-musl.zip" \
        -o /tmp/bun.zip
    unzip -q -o /tmp/bun.zip -d /tmp/bun-extract
    cp /tmp/bun-extract/bun-linux-x64-musl/bun "${mnt}/usr/local/bin/bun"
    chmod +x "${mnt}/usr/local/bin/bun"
    rm -rf /tmp/bun.zip /tmp/bun-extract

    [[ -f "${INSTALL_DIR}/bin/nova-agent" ]] && \
        cp "${INSTALL_DIR}/bin/nova-agent" "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "bun.ext4 ready ($(du -h ${output} | cut -f1))"
}

# ─── Nova Backend ────────────────────────────────────────
deploy_nova_backend() {
    log "Deploying Nova backend..."

    # Copy binaries
    install -m 0755 "${NOVA_BIN}" "${INSTALL_DIR}/bin/nova"
    install -m 0755 "${AGENT_BIN}" "${INSTALL_DIR}/bin/nova-agent"
    log "Installed Nova binaries to ${INSTALL_DIR}/bin/"

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

    # Create systemd service
    cat > /etc/systemd/system/nova.service << 'EOF'
[Unit]
Description=Nova Serverless Platform
After=postgresql.service network.target
Requires=postgresql.service

[Service]
Type=simple
ExecStart=/opt/nova/bin/nova daemon --config /opt/nova/configs/nova.json --http :9000
Restart=on-failure
RestartSec=5
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt/nova/bin

[Install]
WantedBy=multi-user.target
EOF
    log "Created systemd service: nova.service"
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
    cat > /etc/systemd/system/nova-lumen.service << 'EOF'
[Unit]
Description=Nova Lumen Dashboard
After=nova.service network.target

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
    log "Created systemd service: nova-lumen.service"
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
    while ! curl -sf http://localhost:9000/health >/dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ ${retries} -eq 0 ]]; then
            warn "Nova health check failed - check logs with: journalctl -u nova"
            break
        fi
        sleep 1
    done

    # Enable and start Lumen
    systemctl enable nova-lumen >/dev/null 2>&1
    systemctl start nova-lumen

    # Wait for Lumen to be ready
    retries=10
    while ! curl -sf http://localhost:3000 >/dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ ${retries} -eq 0 ]]; then
            warn "Lumen health check failed - check logs with: journalctl -u nova-lumen"
            break
        fi
        sleep 1
    done
}

# ─── Create Sample Functions ─────────────────────────────
create_sample_functions() {
    log "Creating sample functions for all runtimes..."

    local api="http://localhost:9000"

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

    # Python functions
    info "  Creating Python functions..."
    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"hello-python","runtime":"python","handler":"handler","memory_mb":128,"timeout_s":30,"code":"import json\nimport sys\n\ndef handler(event):\n    name = event.get(\"name\", \"World\")\n    return {\"message\": f\"Hello, {name}!\", \"runtime\": \"python\"}\n\nif __name__ == \"__main__\":\n    with open(sys.argv[1]) as f:\n        event = json.load(f)\n    result = handler(event)\n    print(json.dumps(result))"}' >/dev/null 2>&1 && log "    hello-python" || true

    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"fibonacci","runtime":"python","handler":"handler","memory_mb":128,"timeout_s":30,"code":"import json\nimport sys\n\ndef fib(n):\n    if n <= 1:\n        return n\n    a, b = 0, 1\n    for _ in range(2, n + 1):\n        a, b = b, a + b\n    return b\n\ndef main():\n    with open(sys.argv[1]) as f:\n        event = json.load(f)\n    n = event.get(\"n\", 10)\n    print(json.dumps({\"n\": n, \"fibonacci\": fib(n)}))\n\nif __name__ == \"__main__\":\n    main()"}' >/dev/null 2>&1 && log "    fibonacci" || true

    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"prime-checker","runtime":"python","handler":"handler","memory_mb":128,"timeout_s":30,"code":"import json\nimport sys\nimport math\n\ndef is_prime(n):\n    if n < 2:\n        return False\n    if n == 2:\n        return True\n    if n % 2 == 0:\n        return False\n    for i in range(3, int(math.sqrt(n)) + 1, 2):\n        if n % i == 0:\n            return False\n    return True\n\ndef main():\n    with open(sys.argv[1]) as f:\n        event = json.load(f)\n    n = event.get(\"n\", 17)\n    print(json.dumps({\"n\": n, \"is_prime\": is_prime(n)}))\n\nif __name__ == \"__main__\":\n    main()"}' >/dev/null 2>&1 && log "    prime-checker" || true

    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"echo","runtime":"python","handler":"handler","memory_mb":128,"timeout_s":30,"code":"import json\nimport sys\nimport time\n\ndef main():\n    with open(sys.argv[1]) as f:\n        event = json.load(f)\n    print(json.dumps({\"echo\": event, \"timestamp\": time.time()}))\n\nif __name__ == \"__main__\":\n    main()"}' >/dev/null 2>&1 && log "    echo" || true

    # Node.js functions
    info "  Creating Node.js functions..."
    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"hello-node","runtime":"node","handler":"handler","memory_mb":256,"timeout_s":30,"code":"const fs = require(\"fs\");\n\nfunction handler(event) {\n  const name = event.name || \"World\";\n  return { message: `Hello, ${name}!`, runtime: \"node\" };\n}\n\nconst event = JSON.parse(fs.readFileSync(process.argv[2], \"utf8\"));\nconsole.log(JSON.stringify(handler(event)));"}' >/dev/null 2>&1 && log "    hello-node" || true

    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"uuid-generator","runtime":"node","handler":"handler","memory_mb":256,"timeout_s":30,"code":"const fs = require(\"fs\");\nconst crypto = require(\"crypto\");\nconst event = JSON.parse(fs.readFileSync(process.argv[2], \"utf8\"));\nconst count = event.count || 1;\nconst uuids = [];\nfor (let i = 0; i < count; i++) {\n  uuids.push(crypto.randomUUID());\n}\nconsole.log(JSON.stringify({ uuids }));"}' >/dev/null 2>&1 && log "    uuid-generator" || true

    # Ruby functions
    info "  Creating Ruby functions..."
    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"hello-ruby","runtime":"ruby","handler":"handler","memory_mb":128,"timeout_s":30,"code":"require \"json\"\n\ndef handler(event)\n  name = event[\"name\"] || \"World\"\n  { message: \"Hello, #{name}!\", runtime: \"ruby\" }\nend\n\nevent = JSON.parse(File.read(ARGV[0]))\nputs JSON.generate(handler(event))"}' >/dev/null 2>&1 && log "    hello-ruby" || true

    # PHP functions
    info "  Creating PHP functions..."
    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"hello-php","runtime":"php","handler":"handler","memory_mb":128,"timeout_s":30,"code":"<?php\n$event = json_decode(file_get_contents($argv[1]), true);\n$name = $event[\"name\"] ?? \"World\";\necho json_encode([\"message\" => \"Hello, $name!\", \"runtime\" => \"php\"]);"}' >/dev/null 2>&1 && log "    hello-php" || true

    # Deno functions
    info "  Creating Deno functions..."
    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"hello-deno","runtime":"deno","handler":"handler","memory_mb":128,"timeout_s":30,"code":"const event = JSON.parse(await Deno.readTextFile(Deno.args[0]));\nconst name = event.name || \"World\";\nconsole.log(JSON.stringify({ message: `Hello, ${name}!`, runtime: \"deno\" }));"}' >/dev/null 2>&1 && log "    hello-deno" || true

    # Bun functions
    info "  Creating Bun functions..."
    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"hello-bun","runtime":"bun","handler":"handler","memory_mb":128,"timeout_s":30,"code":"const event = JSON.parse(await Bun.file(Bun.argv[2]).text());\nconst name = event.name || \"World\";\nconsole.log(JSON.stringify({ message: `Hello, ${name}!`, runtime: \"bun\" }));"}' >/dev/null 2>&1 && log "    hello-bun" || true

    # Go functions (compiled)
    info "  Creating Go functions (will be compiled)..."
    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"hello-go","runtime":"go","handler":"handler","memory_mb":128,"timeout_s":30,"code":"package main\n\nimport (\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"os\"\n)\n\ntype Event struct {\n\tName string `json:\"name\"`\n}\n\ntype Response struct {\n\tMessage string `json:\"message\"`\n\tRuntime string `json:\"runtime\"`\n}\n\nfunc main() {\n\tdata, _ := os.ReadFile(os.Args[1])\n\tvar event Event\n\tjson.Unmarshal(data, &event)\n\tif event.Name == \"\" {\n\t\tevent.Name = \"World\"\n\t}\n\tresp := Response{Message: fmt.Sprintf(\"Hello, %s!\", event.Name), Runtime: \"go\"}\n\tout, _ := json.Marshal(resp)\n\tfmt.Println(string(out))\n}"}' >/dev/null 2>&1 && log "    hello-go" || true

    # Java functions (compiled)
    info "  Creating Java functions (will be compiled)..."
    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"hello-java","runtime":"java","handler":"handler","memory_mb":256,"timeout_s":60,"code":"import java.nio.file.*;\nimport java.util.regex.*;\n\npublic class Handler {\n    public static void main(String[] args) throws Exception {\n        String content = Files.readString(Path.of(args[0]));\n        Pattern p = Pattern.compile(\"\\\"name\\\"\\\\s*:\\\\s*\\\"([^\\\"]+)\\\"\");\n        Matcher m = p.matcher(content);\n        String name = m.find() ? m.group(1) : \"World\";\n        System.out.println(\"{\\\"message\\\": \\\"Hello, \" + name + \"!\\\", \\\"runtime\\\": \\\"java\\\"}\");\n    }\n}"}' >/dev/null 2>&1 && log "    hello-java" || true

    # .NET functions (compiled)
    info "  Creating .NET functions (will be compiled)..."
    curl -sf -X POST "${api}/functions" -H "Content-Type: application/json" \
        -d '{"name":"hello-dotnet","runtime":"dotnet","handler":"handler","memory_mb":256,"timeout_s":60,"code":"using System.Text.Json;\n\nvar json = File.ReadAllText(args[0]);\nvar doc = JsonDocument.Parse(json);\nvar name = doc.RootElement.TryGetProperty(\"name\", out var n) ? n.GetString() : \"World\";\nConsole.WriteLine(JsonSerializer.Serialize(new { message = $\"Hello, {name}!\", runtime = \"dotnet\" }));"}' >/dev/null 2>&1 && log "    hello-dotnet" || true

    log "Sample functions created (compiled languages may take a moment to build)"
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

    local pg_status nova_status lumen_status
    pg_status=$(systemctl is-active postgresql 2>/dev/null || echo "unknown")
    nova_status=$(systemctl is-active nova 2>/dev/null || echo "unknown")
    lumen_status=$(systemctl is-active nova-lumen 2>/dev/null || echo "unknown")

    if [[ "${pg_status}" == "active" ]]; then
        echo -e "  ${GREEN}[OK]${NC} PostgreSQL      - running"
    else
        echo -e "  ${RED}[!!]${NC} PostgreSQL      - ${pg_status}"
    fi

    if [[ "${nova_status}" == "active" ]]; then
        echo -e "  ${GREEN}[OK]${NC} Nova Backend    - running on port 9000"
    else
        echo -e "  ${RED}[!!]${NC} Nova Backend    - ${nova_status}"
    fi

    if [[ "${lumen_status}" == "active" ]]; then
        echo -e "  ${GREEN}[OK]${NC} Lumen Dashboard - running on port 3000"
    else
        echo -e "  ${RED}[!!]${NC} Lumen Dashboard - ${lumen_status}"
    fi

    echo ""
    echo "  Access URLs:"
    echo "  ------------"
    echo "  Dashboard:  http://${server_ip}:3000"
    echo "  API:        http://${server_ip}:9000"
    echo "  Health:     http://${server_ip}:9000/health"
    echo ""
    echo "  Installation Directory: ${INSTALL_DIR}"
    echo ""
    echo "  Useful Commands:"
    echo "  ----------------"
    echo "  journalctl -u nova -f          # View Nova logs"
    echo "  journalctl -u nova-lumen -f    # View Lumen logs"
    echo "  systemctl restart nova         # Restart Nova"
    echo "  systemctl restart nova-lumen   # Restart Lumen"
    echo ""

    # Health check
    if curl -sf http://localhost:9000/health >/dev/null 2>&1; then
        echo -e "  ${GREEN}API Health Check: OK${NC}"
    else
        echo -e "  ${RED}API Health Check: FAILED${NC}"
        echo "  Run 'journalctl -u nova' to check for errors"
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

    # Create directories
    log "Setting up directories..."
    mkdir -p "${INSTALL_DIR}"/{kernel,rootfs,bin,snapshots,configs,lumen}
    mkdir -p /tmp/nova/{sockets,vsock,logs}
    chmod 755 "${INSTALL_DIR}" "${INSTALL_DIR}"/{kernel,rootfs,bin,snapshots,configs,lumen}

    # PostgreSQL
    install_postgres
    setup_database

    # Firecracker + Kernel
    install_firecracker
    download_kernel

    # Deploy Nova binaries first (so agent is available for rootfs)
    deploy_nova_backend

    # Build all rootfs images
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

    # Deploy Lumen
    deploy_lumen_frontend

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
