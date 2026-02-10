#!/bin/bash
# Deploy Nova backend binaries on local Linux machine
# Usage: sudo ./scripts/deploy.sh

set -e

INSTALL_DIR="/opt/nova"

pick_bin() {
    local linux_path="$1"
    local native_path="$2"
    if [[ -f "${linux_path}" ]]; then
        echo "${linux_path}"
    elif [[ -f "${native_path}" ]]; then
        echo "${native_path}"
    else
        echo ""
    fi
}

log() { echo "[+] $1"; }

# Check root
[[ $EUID -eq 0 ]] || { echo "Run as root: sudo $0"; exit 1; }

NOVA_SRC="$(pick_bin bin/nova-linux bin/nova)"
COMET_SRC="$(pick_bin bin/comet-linux bin/comet)"
ZENITH_SRC="$(pick_bin bin/zenith-linux bin/zenith)"
AGENT_SRC="$(pick_bin bin/nova-agent bin/nova-agent)"

[[ -n "${NOVA_SRC}" ]] || { echo "Missing binary: bin/nova-linux or bin/nova"; exit 1; }
[[ -n "${COMET_SRC}" ]] || { echo "Missing binary: bin/comet-linux or bin/comet"; exit 1; }
[[ -n "${ZENITH_SRC}" ]] || { echo "Missing binary: bin/zenith-linux or bin/zenith"; exit 1; }
[[ -n "${AGENT_SRC}" ]] || { echo "Missing binary: bin/nova-agent"; exit 1; }

# Copy binaries
log "Installing binaries to ${INSTALL_DIR}/bin/..."
mkdir -p "${INSTALL_DIR}/bin"
install -m 0755 "${NOVA_SRC}" "${INSTALL_DIR}/bin/nova"
install -m 0755 "${COMET_SRC}" "${INSTALL_DIR}/bin/comet"
install -m 0755 "${ZENITH_SRC}" "${INSTALL_DIR}/bin/zenith"
install -m 0755 "${AGENT_SRC}" "${INSTALL_DIR}/bin/nova-agent"

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
echo "  ${INSTALL_DIR}/bin/comet --help"
echo "  ${INSTALL_DIR}/bin/zenith --help"
