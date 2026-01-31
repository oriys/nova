#!/bin/bash
# Nova Serverless Platform - Linux Server Setup
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/oriys/nova/main/scripts/install.sh | sudo bash
# Or:
#   scp scripts/install.sh user@server:/tmp/ && ssh user@server 'sudo bash /tmp/install.sh'

set -e

PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
export PATH

INSTALL_DIR="/opt/nova"
FC_VERSION="latest"
ALPINE_URL="https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.3-x86_64.tar.gz"
WASMTIME_VERSION="v29.0.1"
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

latest_firecracker_version() {
    local release_url="https://github.com/firecracker-microvm/firecracker/releases"
    basename "$(curl -fsSLI -o /dev/null -w "%{url_effective}" ${release_url}/latest)"
}

# ─── Firecracker ─────────────────────────────────────────
install_firecracker() {
    local fc_bin="${INSTALL_DIR}/bin/firecracker"
    local jailer_bin="${INSTALL_DIR}/bin/jailer"

    if [[ -x "${fc_bin}" ]]; then
        warn "Existing Firecracker detected: $(${fc_bin} --version) - overwriting"
    fi
    if [[ "${FC_VERSION}" == "latest" || -z "${FC_VERSION}" ]]; then
        FC_VERSION="$(latest_firecracker_version)"
    fi
    log "Installing Firecracker ${FC_VERSION}..."
    local tmp=$(mktemp -d)
    local arch="$(uname -m)"
    local fc_url="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${arch}.tgz"
    curl -fsSL -o "${tmp}/fc.tgz" "${fc_url}"
    tar -xzf "${tmp}/fc.tgz" -C "${tmp}"
    # Handle both old (release-*/) and new (flat) archive structures
    if ls ${tmp}/release-*/firecracker-* &>/dev/null 2>&1; then
        install -m 0755 ${tmp}/release-*/firecracker-* "${fc_bin}"
        install -m 0755 ${tmp}/release-*/jailer-*      "${jailer_bin}"
    else
        install -m 0755 ${tmp}/firecracker-${FC_VERSION}-${arch} "${fc_bin}"
        install -m 0755 ${tmp}/jailer-${FC_VERSION}-${arch}      "${jailer_bin}"
    fi
    rm -rf "${tmp}"
    # Symlink to /usr/local/bin for convenience
    ln -sf "${fc_bin}" /usr/local/bin/firecracker
    ln -sf "${jailer_bin}" /usr/local/bin/jailer
    log "Firecracker $(${fc_bin} --version)"
}

# ─── Kernel ──────────────────────────────────────────────
download_kernel() {
    log "Downloading kernel..."
    mkdir -p ${INSTALL_DIR}/kernel
    local arch
    local latest_version
    local ci_version
    local kernel_key
    arch="$(uname -m)"
    latest_version="$(latest_firecracker_version)"
    # Extract major.minor from version (e.g., v1.14.1 -> v1.14)
    ci_version="${latest_version%.*}"

    # Try to find kernel from Firecracker CI bucket
    kernel_key=$(curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${ci_version}/${arch}/vmlinux-&list-type=2" 2>/dev/null \
        | grep -oP "(?<=<Key>)(firecracker-ci/${ci_version}/${arch}/vmlinux-[0-9]+\\.[0-9]+\\.[0-9]{1,3})(?=</Key>)" \
        | sort -V | tail -1)

    # Fallback: try previous minor version if current not found
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

    log "Building base rootfs (minimal, no distro)..."
    dd if=/dev/zero of="${output}" bs=1M count=32 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    # Minimal directory structure
    mkdir -p "${mnt}"/{dev,proc,sys,tmp,code,usr/local/bin}

    # init = nova-agent (static binary)
    if [[ -f ${INSTALL_DIR}/bin/nova-agent ]]; then
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init"
        chmod +x "${mnt}/init"
    else
        # Placeholder init that waits for agent to be injected
        cat > "${mnt}/init" << 'INIT'
#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
echo "ERROR: nova-agent not found, halting"
sleep infinity
INIT
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

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
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
    mkdir -p "${mnt}"/{code,tmp}

    curl -fsSL \
        "https://github.com/bytecodealliance/wasmtime/releases/download/${WASMTIME_VERSION}/wasmtime-${WASMTIME_VERSION}-x86_64-linux.tar.xz" \
        | tar -xJf - -C "${mnt}/usr/local/bin" --strip-components=1 --wildcards '*/wasmtime'

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

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

    # Create directories (idempotent - creates if not exist, no error if exist)
    log "Setting up directories..."
    mkdir -p ${INSTALL_DIR}/{kernel,rootfs,bin,snapshots}
    mkdir -p /tmp/nova/{sockets,vsock,logs}
    chmod 755 ${INSTALL_DIR} ${INSTALL_DIR}/{kernel,rootfs,bin,snapshots}

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
    echo "  Installed to: ${INSTALL_DIR}"
    echo ""
    echo "  ${INSTALL_DIR}/bin/           (nova, nova-agent)"
    echo "  ${INSTALL_DIR}/kernel/vmlinux"
    echo "  ${INSTALL_DIR}/rootfs/base.ext4     (Go, Rust)"
    echo "  ${INSTALL_DIR}/rootfs/python.ext4   (Python)"
    echo "  ${INSTALL_DIR}/rootfs/wasm.ext4     (WASM)"
    echo "  ${INSTALL_DIR}/snapshots/     (VM snapshots)"
    echo ""
    echo "  Next: copy nova and nova-agent binaries to ${INSTALL_DIR}/bin/"
    echo ""
}

main "$@"
