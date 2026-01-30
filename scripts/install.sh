#!/bin/bash
# Nova Serverless Platform - Linux Server Setup
# Run this script on your Linux server to install all dependencies
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/oriys/nova/main/scripts/install.sh | bash
# Or:
#   scp scripts/install.sh user@server:/tmp/ && ssh user@server 'bash /tmp/install.sh'

set -e

NOVA_VERSION="${NOVA_VERSION:-latest}"
INSTALL_DIR="/opt/nova"
FC_VERSION="v1.7.0"
KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.11/x86_64/vmlinux-5.10.225"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[!]${NC} $1" >&2; exit 1; }

check_root() {
    [[ $EUID -eq 0 ]] || err "Please run as root: sudo $0"
}

check_system() {
    [[ "$(uname)" == "Linux" ]] || err "This script only works on Linux"
    [[ "$(uname -m)" == "x86_64" ]] || err "Only x86_64 architecture is supported"

    # Check KVM support
    if [[ ! -e /dev/kvm ]]; then
        warn "/dev/kvm not found. Firecracker requires KVM support."
        warn "If running in a VM, enable nested virtualization."
    fi
}

install_deps() {
    log "Installing dependencies..."
    if command -v apt-get &>/dev/null; then
        apt-get update
        apt-get install -y curl wget e2fsprogs debootstrap
    elif command -v yum &>/dev/null; then
        yum install -y curl wget e2fsprogs
    elif command -v apk &>/dev/null; then
        apk add curl wget e2fsprogs
    fi
}

install_firecracker() {
    log "Installing Firecracker ${FC_VERSION}..."
    local tmp_dir=$(mktemp -d)
    cd "${tmp_dir}"

    curl -fsSL -o firecracker.tgz \
        "https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-x86_64.tgz"
    tar -xzf firecracker.tgz

    mv release-*/firecracker-* /usr/local/bin/firecracker
    mv release-*/jailer-* /usr/local/bin/jailer
    chmod +x /usr/local/bin/firecracker /usr/local/bin/jailer

    cd /
    rm -rf "${tmp_dir}"

    log "Firecracker installed: $(firecracker --version)"
}

setup_dirs() {
    log "Creating directories..."
    mkdir -p ${INSTALL_DIR}/{kernel,rootfs,bin}
    mkdir -p /tmp/nova/{sockets,vsock,logs}
    chmod 755 ${INSTALL_DIR} /tmp/nova
}

download_kernel() {
    log "Downloading kernel..."
    curl -fsSL -o ${INSTALL_DIR}/kernel/vmlinux "${KERNEL_URL}"
    chmod 644 ${INSTALL_DIR}/kernel/vmlinux
    log "Kernel ready: ${INSTALL_DIR}/kernel/vmlinux"
}

build_rootfs() {
    local runtime=$1
    local output="${INSTALL_DIR}/rootfs/${runtime}.ext4"
    local size_mb=256
    local mount_dir=$(mktemp -d)

    log "Building ${runtime} rootfs..."

    # Create ext4 image
    dd if=/dev/zero of="${output}" bs=1M count=${size_mb} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mount_dir}"

    # Download Alpine minirootfs
    local alpine_url="https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/x86_64/alpine-minirootfs-3.19.0-x86_64.tar.gz"
    curl -fsSL "${alpine_url}" | tar -xzf - -C "${mount_dir}"

    # Install runtime
    case "${runtime}" in
        python)
            chroot "${mount_dir}" /bin/sh -c "apk add --no-cache python3 py3-pip"
            ;;
        go)
            chroot "${mount_dir}" /bin/sh -c "apk add --no-cache go"
            ;;
        rust)
            chroot "${mount_dir}" /bin/sh -c "apk add --no-cache libgcc libstdc++"
            ;;
        wasm)
            # Install wasmtime
            curl -fsSL "https://github.com/bytecodealliance/wasmtime/releases/download/v18.0.2/wasmtime-v18.0.2-x86_64-linux.tar.xz" \
                | tar -xJf - -C "${mount_dir}/usr/local/bin" --strip-components=1 --wildcards '*/wasmtime'
            ;;
    esac

    # Create init script
    cat > "${mount_dir}/init" << 'INIT'
#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
ip link set lo up
ip link set eth0 up 2>/dev/null
exec /usr/local/bin/nova-agent
INIT
    chmod +x "${mount_dir}/init"

    # Copy nova-agent if exists
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && cp ${INSTALL_DIR}/bin/nova-agent "${mount_dir}/usr/local/bin/"

    umount "${mount_dir}"
    rmdir "${mount_dir}"
    log "Created: ${output} ($(du -h ${output} | cut -f1))"
}

install_redis() {
    log "Checking Redis..."
    if command -v redis-server &>/dev/null; then
        log "Redis already installed"
    else
        if command -v apt-get &>/dev/null; then
            apt-get install -y redis-server
            systemctl enable redis-server
            systemctl start redis-server
        elif command -v yum &>/dev/null; then
            yum install -y redis
            systemctl enable redis
            systemctl start redis
        else
            warn "Please install Redis manually"
        fi
    fi
}

setup_permissions() {
    log "Setting up permissions..."
    # Allow non-root users to use KVM
    if getent group kvm > /dev/null; then
        chmod 666 /dev/kvm 2>/dev/null || true
    fi
}

print_summary() {
    echo ""
    echo "============================================"
    echo "  Nova Serverless Platform - Setup Complete"
    echo "============================================"
    echo ""
    echo "Installed components:"
    echo "  Firecracker: $(which firecracker)"
    echo "  Kernel:      ${INSTALL_DIR}/kernel/vmlinux"
    echo "  Rootfs:      ${INSTALL_DIR}/rootfs/*.ext4"
    echo ""
    echo "Next steps:"
    echo "  1. Copy nova binary:  scp bin/nova server:${INSTALL_DIR}/bin/"
    echo "  2. Copy nova-agent:   scp bin/nova-agent server:${INSTALL_DIR}/bin/"
    echo "  3. Start Redis:       systemctl start redis"
    echo "  4. Run nova:          ${INSTALL_DIR}/bin/nova --help"
    echo ""
}

main() {
    check_root
    check_system
    install_deps
    setup_dirs
    install_firecracker
    download_kernel

    for runtime in python go rust wasm; do
        build_rootfs "${runtime}"
    done

    install_redis
    setup_permissions
    print_summary
}

main "$@"
