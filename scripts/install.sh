#!/bin/bash
# Nova Serverless Platform - Linux Server Setup
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/oriys/nova/main/scripts/install.sh | sudo bash
# Or:
#   scp scripts/install.sh user@server:/tmp/ && ssh user@server 'sudo bash /tmp/install.sh'

set -e

INSTALL_DIR="/opt/nova"
FC_VERSION="v1.7.0"
KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.11/x86_64/vmlinux-5.10.225"
ALPINE_URL="https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/x86_64/alpine-minirootfs-3.19.0-x86_64.tar.gz"
WASMTIME_VERSION="v18.0.2"
ROOTFS_SIZE_MB=256

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log()  { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[!]${NC} $1" >&2; exit 1; }

# ─── Checks ──────────────────────────────────────────────
check_root()   { [[ $EUID -eq 0 ]] || err "Run as root: sudo $0"; }
check_system() {
    [[ "$(uname)" == "Linux" ]]   || err "Linux only"
    [[ "$(uname -m)" == "x86_64" ]] || err "x86_64 only"
    [[ -e /dev/kvm ]] || warn "/dev/kvm not found - Firecracker needs KVM"
}

install_deps() {
    log "Installing dependencies..."
    if command -v apt-get &>/dev/null; then
        apt-get update -qq
        apt-get install -y -qq curl e2fsprogs >/dev/null
    elif command -v yum &>/dev/null; then
        yum install -y -q curl e2fsprogs
    fi
}

# ─── Firecracker ─────────────────────────────────────────
install_firecracker() {
    if command -v firecracker &>/dev/null; then
        log "Firecracker already installed: $(firecracker --version)"
        return
    fi
    log "Installing Firecracker ${FC_VERSION}..."
    local tmp=$(mktemp -d)
    curl -fsSL -o "${tmp}/fc.tgz" \
        "https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-x86_64.tgz"
    tar -xzf "${tmp}/fc.tgz" -C "${tmp}"
    mv ${tmp}/release-*/firecracker-* /usr/local/bin/firecracker
    mv ${tmp}/release-*/jailer-*      /usr/local/bin/jailer
    chmod +x /usr/local/bin/firecracker /usr/local/bin/jailer
    rm -rf "${tmp}"
    log "Firecracker $(firecracker --version)"
}

# ─── Kernel ──────────────────────────────────────────────
download_kernel() {
    log "Downloading kernel..."
    mkdir -p ${INSTALL_DIR}/kernel
    curl -fsSL -o ${INSTALL_DIR}/kernel/vmlinux "${KERNEL_URL}"
    log "Kernel: ${INSTALL_DIR}/kernel/vmlinux ($(du -h ${INSTALL_DIR}/kernel/vmlinux | cut -f1))"
}

# ─── Rootfs builder ─────────────────────────────────────
#
# Only 3 images:
#   base.ext4   - Alpine + init + nova-agent (for Go/Rust)
#   python.ext4 - base + python3
#   wasm.ext4   - base + wasmtime
#
build_base_rootfs() {
    local output="${INSTALL_DIR}/rootfs/base.ext4"
    local mnt=$(mktemp -d)

    log "Building base rootfs..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"

    # DNS
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    # init
    cat > "${mnt}/init" << 'INIT'
#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
ip link set lo up
ip link set eth0 up 2>/dev/null
exec /usr/local/bin/nova-agent
INIT
    chmod +x "${mnt}/init"

    # Copy nova-agent if already present
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/usr/local/bin/" && \
        chmod +x "${mnt}/usr/local/bin/nova-agent"

    umount "${mnt}" && rmdir "${mnt}"
    log "base.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_python_rootfs() {
    local base="${INSTALL_DIR}/rootfs/base.ext4"
    local output="${INSTALL_DIR}/rootfs/python.ext4"
    local mnt=$(mktemp -d)

    log "Building python rootfs (base + python3)..."
    cp "${base}" "${output}"
    mount -o loop "${output}" "${mnt}"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache python3 py3-pip" >/dev/null 2>&1

    umount "${mnt}" && rmdir "${mnt}"
    log "python.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_wasm_rootfs() {
    local base="${INSTALL_DIR}/rootfs/base.ext4"
    local output="${INSTALL_DIR}/rootfs/wasm.ext4"
    local mnt=$(mktemp -d)

    log "Building wasm rootfs (base + wasmtime)..."
    cp "${base}" "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL \
        "https://github.com/bytecodealliance/wasmtime/releases/download/${WASMTIME_VERSION}/wasmtime-${WASMTIME_VERSION}-x86_64-linux.tar.xz" \
        | tar -xJf - -C "${mnt}/usr/local/bin" --strip-components=1 --wildcards '*/wasmtime'

    umount "${mnt}" && rmdir "${mnt}"
    log "wasm.ext4 ready ($(du -h ${output} | cut -f1))"
}

# ─── Redis ───────────────────────────────────────────────
install_redis() {
    if command -v redis-server &>/dev/null; then
        log "Redis already installed"
        return
    fi
    log "Installing Redis..."
    if command -v apt-get &>/dev/null; then
        apt-get install -y -qq redis-server >/dev/null
        systemctl enable --now redis-server
    elif command -v yum &>/dev/null; then
        yum install -y -q redis
        systemctl enable --now redis
    else
        warn "Install Redis manually"
    fi
}

# ─── Main ────────────────────────────────────────────────
main() {
    check_root
    check_system
    install_deps

    mkdir -p ${INSTALL_DIR}/{kernel,rootfs,bin}
    mkdir -p /tmp/nova/{sockets,vsock,logs}

    install_firecracker
    download_kernel

    build_base_rootfs
    build_python_rootfs
    build_wasm_rootfs

    install_redis

    # Permissions
    chmod 666 /dev/kvm 2>/dev/null || true

    echo ""
    echo "========================================"
    echo "  Nova Setup Complete"
    echo "========================================"
    echo ""
    echo "  ${INSTALL_DIR}/kernel/vmlinux"
    echo "  ${INSTALL_DIR}/rootfs/base.ext4     (Go, Rust)"
    echo "  ${INSTALL_DIR}/rootfs/python.ext4   (Python)"
    echo "  ${INSTALL_DIR}/rootfs/wasm.ext4     (WASM)"
    echo ""
    echo "  Next: copy bin/nova + bin/nova-agent to ${INSTALL_DIR}/bin/"
    echo ""
}

main "$@"
