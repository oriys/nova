#!/bin/bash
# Deploy Nova to Linux server from macOS
# Usage: ./scripts/deploy.sh user@server

set -e

SERVER="${1:?Usage: $0 user@server}"
INSTALL_DIR="/opt/nova"

log() { echo "[+] $1"; }

# Build binaries for Linux
log "Building binaries for Linux..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/nova-linux ./cmd/nova
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/nova-agent ./cmd/agent

# Deploy
log "Deploying to ${SERVER}..."

# Copy install script and run setup (if first time)
ssh "${SERVER}" "test -f ${INSTALL_DIR}/kernel/vmlinux" || {
    log "First time setup - running install.sh..."
    scp scripts/install.sh "${SERVER}":/tmp/
    ssh "${SERVER}" "sudo bash /tmp/install.sh"
}

# Copy binaries
log "Copying binaries..."
ssh "${SERVER}" "sudo mkdir -p ${INSTALL_DIR}/bin"
scp bin/nova-linux "${SERVER}":/tmp/nova
scp bin/nova-agent "${SERVER}":/tmp/nova-agent
ssh "${SERVER}" "sudo mv /tmp/nova /tmp/nova-agent ${INSTALL_DIR}/bin/ && sudo chmod +x ${INSTALL_DIR}/bin/*"

# Copy nova-agent to rootfs images
log "Updating rootfs with nova-agent..."
ssh "${SERVER}" << 'EOF'
for img in /opt/nova/rootfs/*.ext4; do
    mnt=$(mktemp -d)
    sudo mount -o loop "$img" "$mnt"
    sudo cp /opt/nova/bin/nova-agent "$mnt/usr/local/bin/"
    sudo chmod +x "$mnt/usr/local/bin/nova-agent"
    sudo umount "$mnt"
    rmdir "$mnt"
done
EOF

log "Done! Run on server:"
echo "  ${INSTALL_DIR}/bin/nova --help"
echo "  ${INSTALL_DIR}/bin/nova list"
