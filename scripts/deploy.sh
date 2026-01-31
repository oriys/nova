#!/bin/bash
# Deploy Nova on local Linux machine
# Usage: sudo ./scripts/deploy.sh

set -e

INSTALL_DIR="/opt/nova"

log() { echo "[+] $1"; }

# Check root
[[ $EUID -eq 0 ]] || { echo "Run as root: sudo $0"; exit 1; }

# Build binaries
log "Building binaries..."
CGO_ENABLED=0 go build -o bin/nova ./cmd/nova
CGO_ENABLED=0 go build -o bin/nova-agent ./cmd/agent

# Copy binaries
log "Installing binaries to ${INSTALL_DIR}/bin/..."
mkdir -p ${INSTALL_DIR}/bin
cp bin/nova bin/nova-agent ${INSTALL_DIR}/bin/
chmod +x ${INSTALL_DIR}/bin/*

# Update rootfs images with nova-agent as /init
log "Updating rootfs with nova-agent..."
for img in ${INSTALL_DIR}/rootfs/*.ext4; do
    [ -f "$img" ] || continue
    mnt=$(mktemp -d)
    mount -o loop "$img" "$mnt"
    cp ${INSTALL_DIR}/bin/nova-agent "$mnt/init"
    chmod +x "$mnt/init"
    umount "$mnt"
    rmdir "$mnt"
    log "  Updated: $(basename $img)"
done

log "Done!"
echo "  ${INSTALL_DIR}/bin/nova --help"
echo "  ${INSTALL_DIR}/bin/nova list"
