#!/bin/bash
# Build rootfs images using Docker (no root required on host)
# Usage: ./build-rootfs-docker.sh [python|go|rust|wasm|all]

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOTFS_DIR="${SCRIPT_DIR}/rootfs"
IMAGE_SIZE_MB=512

log() { echo "[+] $1"; }

build_image() {
    local runtime=$1
    local dockerfile="${SCRIPT_DIR}/Dockerfile.${runtime}"
    local image_name="nova-rootfs-${runtime}"
    local output="${ROOTFS_DIR}/${runtime}.ext4"

    log "Building ${runtime} rootfs..."

    # Build Docker image
    docker build -t "${image_name}" -f "${dockerfile}" "${SCRIPT_DIR}"

    # Create container and export filesystem
    local container_id=$(docker create "${image_name}")

    # Create ext4 image
    dd if=/dev/zero of="${output}" bs=1M count=${IMAGE_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F "${output}" 2>/dev/null

    # Mount and copy files
    local mount_dir=$(mktemp -d)

    if [[ "$(uname)" == "Linux" ]]; then
        sudo mount -o loop "${output}" "${mount_dir}"
        docker export "${container_id}" | sudo tar -xf - -C "${mount_dir}"
        sudo umount "${mount_dir}"
    else
        # macOS - export to tar and use Docker to create ext4
        docker export "${container_id}" -o "/tmp/${runtime}.tar"

        # Use Docker to create ext4 on Linux
        docker run --rm --privileged \
            -v "${ROOTFS_DIR}:/output" \
            -v "/tmp/${runtime}.tar:/input.tar" \
            ubuntu:24.04 bash -c "
                apt-get update && apt-get install -y e2fsprogs
                dd if=/dev/zero of=/output/${runtime}.ext4 bs=1M count=${IMAGE_SIZE_MB}
                mkfs.ext4 -F /output/${runtime}.ext4
                mkdir -p /mnt/rootfs
                mount -o loop /output/${runtime}.ext4 /mnt/rootfs
                tar -xf /input.tar -C /mnt/rootfs
                umount /mnt/rootfs
            "
        rm -f "/tmp/${runtime}.tar"
    fi

    rmdir "${mount_dir}" 2>/dev/null || true
    docker rm "${container_id}" > /dev/null

    log "Created: ${output}"
}

mkdir -p "${ROOTFS_DIR}"

case "${1:-all}" in
    python|go|rust|wasm)
        build_image "$1"
        ;;
    all)
        for rt in python go rust wasm; do
            build_image "$rt"
        done
        ;;
    *)
        echo "Usage: $0 [python|go|rust|wasm|all]"
        exit 1
        ;;
esac

log "Done! Rootfs images:"
ls -lh "${ROOTFS_DIR}"/*.ext4 2>/dev/null || echo "No images found"
