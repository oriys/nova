#!/bin/bash
# download_assets.sh - Download assets for offline rootfs building.

set -euxo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ASSETS_DIR="${REPO_ROOT}/assets/downloads"
NOVA_CACHE_DIR="${NOVA_CACHE_DIR:-}"

# Versions (must match build_rootfs.sh defaults or be overridden)
ALPINE_VERSION="3.23.3"
WASMTIME_VERSION="${WASMTIME_VERSION:-v41.0.1}"
DENO_VERSION="${DENO_VERSION:-v2.6.7}"
BUN_VERSION="${BUN_VERSION:-bun-v1.3.8}"

# Detect host architecture
HOST_ARCH="$(uname -m)"
case "${HOST_ARCH}" in
  x86_64)  ARCH="x86_64"; BUN_MUSL="bun-linux-x64-musl" ;;
  aarch64|arm64) ARCH="aarch64"; BUN_MUSL="bun-linux-aarch64-musl" ;;
  *) echo "Unsupported architecture: ${HOST_ARCH}"; exit 1 ;;
esac

# URLs (architecture-aware)
ALPINE_URL="https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/${ARCH}/alpine-minirootfs-${ALPINE_VERSION}-${ARCH}.tar.gz"
WASMTIME_URL="https://github.com/bytecodealliance/wasmtime/releases/download/${WASMTIME_VERSION}/wasmtime-${WASMTIME_VERSION}-${ARCH}-linux.tar.xz"
DENO_URL="https://github.com/denoland/deno/releases/download/${DENO_VERSION}/deno-${ARCH}-unknown-linux-gnu.zip"
BUN_URL="https://github.com/oven-sh/bun/releases/download/${BUN_VERSION}/${BUN_MUSL}.zip"

mkdir -p "${ASSETS_DIR}"

download_if_missing() {
  local url="$1"
  local filename="$2"
  local filepath="${ASSETS_DIR}/${filename}"
  local cached="${NOVA_CACHE_DIR}/${filename}"

  if [[ -f "${filepath}" ]]; then
    echo "[+] ${filename} already exists, skipping."
  elif [[ -n "${NOVA_CACHE_DIR}" && -f "${cached}" ]]; then
    echo "[+] ${filename} found in cache, copying."
    cp "${cached}" "${filepath}"
  else
    echo "[+] Downloading ${filename}..."
    local tmp_dl="${filepath}.tmp.$$"
    curl -fsSL "${url}" -o "${tmp_dl}"
    mv "${tmp_dl}" "${filepath}"
    # Also save to global cache for reuse
    if [[ -n "${NOVA_CACHE_DIR}" ]]; then
      mkdir -p "${NOVA_CACHE_DIR}"
      cp "${filepath}" "${cached}"
    fi
  fi
}

echo "Downloading assets to ${ASSETS_DIR}..."

download_if_missing "${ALPINE_URL}" "alpine-minirootfs.tar.gz"
download_if_missing "${WASMTIME_URL}" "wasmtime.tar.xz"
download_if_missing "${DENO_URL}" "deno.zip"
download_if_missing "${BUN_URL}" "bun.zip"

echo "All assets downloaded."
