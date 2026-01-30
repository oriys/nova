#!/bin/bash
# Download Firecracker kernel and create minimal rootfs images
# Run this on a Linux machine

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
KERNEL_DIR="${SCRIPT_DIR}/kernel"
ROOTFS_DIR="${SCRIPT_DIR}/rootfs"

log() { echo "[+] $1"; }
err() { echo "[!] $1" >&2; exit 1; }

# Firecracker kernel from AWS S3
KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.11/x86_64/vmlinux-5.10.225"

download_kernel() {
    log "Downloading Firecracker kernel..."
    mkdir -p "${KERNEL_DIR}"
    curl -fsSL -o "${KERNEL_DIR}/vmlinux" "${KERNEL_URL}"
    chmod 644 "${KERNEL_DIR}/vmlinux"
    log "Kernel downloaded: ${KERNEL_DIR}/vmlinux ($(du -h ${KERNEL_DIR}/vmlinux | cut -f1))"
}

# Build minimal Alpine-based rootfs (much smaller than Ubuntu)
build_alpine_rootfs() {
    local runtime=$1
    local output="${ROOTFS_DIR}/${runtime}.ext4"
    local size_mb=256

    log "Building ${runtime} rootfs (Alpine-based)..."

    # Create ext4 image
    dd if=/dev/zero of="${output}" bs=1M count=${size_mb} 2>/dev/null
    mkfs.ext4 -F -q "${output}"

    # Mount
    local mount_dir=$(mktemp -d)
    sudo mount -o loop "${output}" "${mount_dir}"

    # Download and extract Alpine minirootfs
    local alpine_version="3.19"
    local alpine_url="https://dl-cdn.alpinelinux.org/alpine/v${alpine_version}/releases/x86_64/alpine-minirootfs-${alpine_version}.0-x86_64.tar.gz"

    curl -fsSL "${alpine_url}" | sudo tar -xzf - -C "${mount_dir}"

    # Install runtime-specific packages
    case "${runtime}" in
        python)
            sudo chroot "${mount_dir}" /bin/sh -c "apk add --no-cache python3"
            ;;
        go)
            sudo chroot "${mount_dir}" /bin/sh -c "apk add --no-cache go"
            ;;
        rust)
            # Rust binaries are statically linked, just need basic libs
            sudo chroot "${mount_dir}" /bin/sh -c "apk add --no-cache libgcc"
            ;;
        wasm)
            # Download wasmtime
            local wt_ver="v18.0.2"
            curl -fsSL "https://github.com/bytecodealliance/wasmtime/releases/download/${wt_ver}/wasmtime-${wt_ver}-x86_64-linux.tar.xz" \
                | sudo tar -xJf - -C "${mount_dir}/usr/local/bin" --strip-components=1 --wildcards '*/wasmtime'
            ;;
    esac

    # Create init script
    sudo tee "${mount_dir}/init" > /dev/null << 'INIT'
#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
ip link set lo up
ip link set eth0 up 2>/dev/null
exec /usr/local/bin/nova-agent
INIT
    sudo chmod +x "${mount_dir}/init"

    # Copy nova-agent if exists
    if [[ -f "${SCRIPT_DIR}/../bin/nova-agent" ]]; then
        sudo cp "${SCRIPT_DIR}/../bin/nova-agent" "${mount_dir}/usr/local/bin/"
        sudo chmod +x "${mount_dir}/usr/local/bin/nova-agent"
    fi

    sudo umount "${mount_dir}"
    rmdir "${mount_dir}"

    log "Created: ${output} ($(du -h ${output} | cut -f1))"
}

main() {
    if [[ "$(uname)" != "Linux" ]]; then
        err "This script must be run on Linux (for loop mount)"
    fi

    mkdir -p "${KERNEL_DIR}" "${ROOTFS_DIR}"

    download_kernel

    for runtime in python go rust wasm; do
        build_alpine_rootfs "${runtime}"
    done

    log "All assets ready!"
    echo ""
    echo "Kernel: ${KERNEL_DIR}/vmlinux"
    echo "Rootfs:"
    ls -lh "${ROOTFS_DIR}"/*.ext4
}

main "$@"
