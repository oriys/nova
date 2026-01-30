#!/bin/bash
# Build Ubuntu 24.04 rootfs images for Nova serverless platform
# This script creates ext4 images with different runtime environments
# Run on Linux with root privileges

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOTFS_DIR="${SCRIPT_DIR}/rootfs"
BUILD_DIR="${SCRIPT_DIR}/build"
IMAGE_SIZE="1G"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
err() { echo -e "${RED}[!]${NC} $1"; exit 1; }

check_root() {
    if [[ $EUID -ne 0 ]]; then
        err "This script must be run as root"
    fi
}

check_deps() {
    local deps="debootstrap qemu-img e2fsprogs"
    for dep in $deps; do
        command -v $dep &>/dev/null || err "Missing dependency: $dep"
    done
}

create_base_rootfs() {
    local name=$1
    local rootfs_path="${BUILD_DIR}/${name}"
    local image_path="${ROOTFS_DIR}/${name}.ext4"

    log "Creating base rootfs for ${name}..."

    # Create mount directory
    mkdir -p "${rootfs_path}"

    # Create ext4 image
    truncate -s ${IMAGE_SIZE} "${image_path}"
    mkfs.ext4 -F "${image_path}"

    # Mount and bootstrap
    mount -o loop "${image_path}" "${rootfs_path}"
    trap "umount ${rootfs_path} 2>/dev/null || true" EXIT

    # Bootstrap Ubuntu 24.04
    debootstrap --arch=amd64 noble "${rootfs_path}" http://archive.ubuntu.com/ubuntu

    # Configure base system
    cat > "${rootfs_path}/etc/fstab" << 'EOF'
/dev/vda / ext4 defaults 0 1
EOF

    # Set hostname
    echo "nova-vm" > "${rootfs_path}/etc/hostname"

    # Configure network
    cat > "${rootfs_path}/etc/hosts" << 'EOF'
127.0.0.1 localhost
127.0.1.1 nova-vm
EOF

    # Configure DNS
    echo "nameserver 8.8.8.8" > "${rootfs_path}/etc/resolv.conf"

    # Setup init script
    cat > "${rootfs_path}/init" << 'EOF'
#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev

# Setup networking (IP passed via kernel args)
ip link set lo up
ip link set eth0 up

# Start nova agent
exec /usr/local/bin/nova-agent
EOF
    chmod +x "${rootfs_path}/init"

    # Copy nova-agent binary (must be built for Linux amd64)
    if [[ -f "${SCRIPT_DIR}/../bin/nova-agent" ]]; then
        cp "${SCRIPT_DIR}/../bin/nova-agent" "${rootfs_path}/usr/local/bin/"
        chmod +x "${rootfs_path}/usr/local/bin/nova-agent"
    fi

    umount "${rootfs_path}"
    trap - EXIT

    log "Base rootfs created: ${image_path}"
}

install_python() {
    local image_path="${ROOTFS_DIR}/python.ext4"
    local mount_path="${BUILD_DIR}/python"

    log "Creating Python runtime rootfs..."

    cp "${ROOTFS_DIR}/base.ext4" "${image_path}"
    mkdir -p "${mount_path}"
    mount -o loop "${image_path}" "${mount_path}"
    trap "umount ${mount_path} 2>/dev/null || true" EXIT

    # Install Python
    chroot "${mount_path}" /bin/bash -c "
        apt-get update
        apt-get install -y --no-install-recommends python3 python3-pip
        apt-get clean
        rm -rf /var/lib/apt/lists/*
    "

    umount "${mount_path}"
    trap - EXIT
    log "Python rootfs created: ${image_path}"
}

install_go() {
    local image_path="${ROOTFS_DIR}/go.ext4"
    local mount_path="${BUILD_DIR}/go"
    local go_version="1.22.0"

    log "Creating Go runtime rootfs..."

    cp "${ROOTFS_DIR}/base.ext4" "${image_path}"
    mkdir -p "${mount_path}"
    mount -o loop "${image_path}" "${mount_path}"
    trap "umount ${mount_path} 2>/dev/null || true" EXIT

    # Install Go
    chroot "${mount_path}" /bin/bash -c "
        apt-get update
        apt-get install -y --no-install-recommends ca-certificates curl
        curl -fsSL https://go.dev/dl/go${go_version}.linux-amd64.tar.gz | tar -C /usr/local -xzf -
        ln -s /usr/local/go/bin/go /usr/local/bin/go
        apt-get clean
        rm -rf /var/lib/apt/lists/*
    "

    umount "${mount_path}"
    trap - EXIT
    log "Go rootfs created: ${image_path}"
}

install_rust() {
    local image_path="${ROOTFS_DIR}/rust.ext4"
    local mount_path="${BUILD_DIR}/rust"

    log "Creating Rust runtime rootfs..."

    cp "${ROOTFS_DIR}/base.ext4" "${image_path}"
    mkdir -p "${mount_path}"
    mount -o loop "${image_path}" "${mount_path}"
    trap "umount ${mount_path} 2>/dev/null || true" EXIT

    # Rust binaries are compiled, just need basic libs
    chroot "${mount_path}" /bin/bash -c "
        apt-get update
        apt-get install -y --no-install-recommends ca-certificates libssl3
        apt-get clean
        rm -rf /var/lib/apt/lists/*
    "

    umount "${mount_path}"
    trap - EXIT
    log "Rust rootfs created: ${image_path}"
}

install_wasm() {
    local image_path="${ROOTFS_DIR}/wasm.ext4"
    local mount_path="${BUILD_DIR}/wasm"
    local wasmtime_version="v18.0.2"

    log "Creating WASM runtime rootfs..."

    cp "${ROOTFS_DIR}/base.ext4" "${image_path}"
    mkdir -p "${mount_path}"
    mount -o loop "${image_path}" "${mount_path}"
    trap "umount ${mount_path} 2>/dev/null || true" EXIT

    # Install wasmtime
    chroot "${mount_path}" /bin/bash -c "
        apt-get update
        apt-get install -y --no-install-recommends ca-certificates curl xz-utils
        curl -fsSL https://github.com/bytecodealliance/wasmtime/releases/download/${wasmtime_version}/wasmtime-${wasmtime_version}-x86_64-linux.tar.xz | tar -C /tmp -xJf -
        mv /tmp/wasmtime-${wasmtime_version}-x86_64-linux/wasmtime /usr/local/bin/
        rm -rf /tmp/wasmtime-*
        apt-get clean
        rm -rf /var/lib/apt/lists/*
    "

    umount "${mount_path}"
    trap - EXIT
    log "WASM rootfs created: ${image_path}"
}

main() {
    check_root
    check_deps

    mkdir -p "${ROOTFS_DIR}" "${BUILD_DIR}"

    log "Building Nova rootfs images (Ubuntu 24.04)..."

    # Create base image first
    create_base_rootfs "base"

    # Create runtime-specific images
    install_python
    install_go
    install_rust
    install_wasm

    # Cleanup
    rm -rf "${BUILD_DIR}"
    rm -f "${ROOTFS_DIR}/base.ext4"

    log "All rootfs images created successfully!"
    ls -lh "${ROOTFS_DIR}"/*.ext4
}

main "$@"
