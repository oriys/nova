#!/bin/bash
# download_assets.sh - Download assets for offline rootfs building.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ASSETS_DIR="${REPO_ROOT}/assets/downloads"

# Versions (must match build_rootfs.sh defaults or be overridden)
ALPINE_VERSION="3.23.3"
WASMTIME_VERSION="${WASMTIME_VERSION:-v41.0.1}"
DENO_VERSION="${DENO_VERSION:-v2.6.7}"
BUN_VERSION="${BUN_VERSION:-bun-v1.3.8}"

# URLs
ALPINE_URL="https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/x86_64/alpine-minirootfs-${ALPINE_VERSION}-x86_64.tar.gz"
WASMTIME_URL="https://github.com/bytecodealliance/wasmtime/releases/download/${WASMTIME_VERSION}/wasmtime-${WASMTIME_VERSION}-x86_64-linux.tar.xz"
DENO_URL="https://github.com/denoland/deno/releases/download/${DENO_VERSION}/deno-x86_64-unknown-linux-gnu.zip"
BUN_URL="https://github.com/oven-sh/bun/releases/download/${BUN_VERSION}/bun-linux-x64-musl.zip"

mkdir -p "${ASSETS_DIR}"

download_if_missing() {
  local url="$1"
  local filename="$2"
  local filepath="${ASSETS_DIR}/${filename}"

  if [[ -f "${filepath}" ]]; then
    echo "[+] ${filename} already exists, skipping."
  else
    echo "[+] Downloading ${filename}..."
    curl -fsSL "${url}" -o "${filepath}"
  fi
}

echo "Downloading assets to ${ASSETS_DIR}..."

download_if_missing "${ALPINE_URL}" "alpine-minirootfs.tar.gz"
download_if_missing "${WASMTIME_URL}" "wasmtime.tar.xz"
download_if_missing "${DENO_URL}" "deno.zip"
download_if_missing "${BUN_URL}" "bun.zip"

echo "All assets downloaded."
