#!/bin/bash
# build_rootfs.sh - Build Nova rootfs images for all supported runtimes.
#
# Images produced (same naming as internal/firecracker/rootfsForRuntime):
#   base.ext4   - Go/Rust (static binaries)
#   python.ext4 - Python (apk python3)
#   node.ext4   - Node.js (apk nodejs)
#   ruby.ext4   - Ruby (apk ruby)
#   java.ext4   - Java (apk openjdk21-jre-headless)
#   wasm.ext4   - WASM (wasmtime + glibc compat)
#   php.ext4    - PHP (apk php)
#   dotnet.ext4 - .NET (dotnet runtime + dependencies)
#   deno.ext4   - Deno (deno binary + glibc compat)
#   bun.ext4    - Bun (bun binary, musl)
#
# This script avoids loop-mount by using `mkfs.ext4 -d <dir>`.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

ALPINE_URL="${ALPINE_URL:-https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/x86_64/alpine-minirootfs-3.23.3-x86_64.tar.gz}"
WASMTIME_VERSION="${WASMTIME_VERSION:-v41.0.1}"
DENO_VERSION="${DENO_VERSION:-v2.6.7}"
BUN_VERSION="${BUN_VERSION:-bun-v1.3.8}"
DOTNET_VERSION="${DOTNET_VERSION:-8.0.23}"

ROOTFS_SIZE_MB="${ROOTFS_SIZE_MB:-256}"
ROOTFS_SIZE_JAVA_MB="${ROOTFS_SIZE_JAVA_MB:-512}"
BASE_ROOTFS_SIZE_MB="${BASE_ROOTFS_SIZE_MB:-32}"

OUT_DIR="${OUT_DIR:-/opt/nova/rootfs}"
AGENT_BIN="${AGENT_BIN:-}"

usage() {
  cat <<EOF
Usage: $0 [--out-dir DIR] [--agent PATH]

Env vars:
  OUT_DIR                Default: /opt/nova/rootfs
  AGENT_BIN              Path to nova-agent binary (linux/amd64)
  ALPINE_URL             Alpine minirootfs tarball URL
  WASMTIME_VERSION       Default: v41.0.1
  DENO_VERSION           Default: v2.6.7
  BUN_VERSION            Default: bun-v1.3.8
  DOTNET_VERSION         Default: 8.0.23
  ROOTFS_SIZE_MB         Default: 256
  ROOTFS_SIZE_JAVA_MB    Default: 512
  BASE_ROOTFS_SIZE_MB    Default: 32
EOF
}

log() { echo "[+] $*"; }
warn() { echo "[!] $*" >&2; }
die() { echo "[x] $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out-dir)
      OUT_DIR="$2"
      shift 2
      ;;
    --agent)
      AGENT_BIN="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "Unknown arg: $1 (use --help)"
      ;;
  esac
done

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"
}

check_platform() {
  [[ "$(uname)" == "Linux" ]] || die "Linux only"
  [[ "$(uname -m)" == "x86_64" ]] || die "x86_64 only"
}

resolve_agent() {
  if [[ -n "${AGENT_BIN}" ]]; then
    [[ -f "${AGENT_BIN}" ]] || die "AGENT_BIN not found: ${AGENT_BIN}"
    return
  fi

  if [[ -f "${REPO_ROOT}/bin/nova-agent" ]]; then
    AGENT_BIN="${REPO_ROOT}/bin/nova-agent"
    return
  fi

  if [[ -f "/opt/nova/bin/nova-agent" ]]; then
    AGENT_BIN="/opt/nova/bin/nova-agent"
    return
  fi

  die "nova-agent not found. Build it with: make build-linux (produces bin/nova-agent), or pass --agent PATH"
}

make_dev_nodes() {
  local root="$1"
  mkdir -p "${root}/dev"
  # Best-effort: some filesystems may block mknod (e.g. certain container mounts)
  mknod -m 666 "${root}/dev/null" c 1 3 2>/dev/null || true
  mknod -m 666 "${root}/dev/zero" c 1 5 2>/dev/null || true
  mknod -m 666 "${root}/dev/random" c 1 8 2>/dev/null || true
  mknod -m 666 "${root}/dev/urandom" c 1 9 2>/dev/null || true
  mknod -m 666 "${root}/dev/tty" c 5 0 2>/dev/null || true
}

build_image_from_dir() {
  local output="$1"
  local size_mb="$2"
  local rootdir="$3"

  rm -f "${output}"
  dd if=/dev/zero of="${output}" bs=1M count="${size_mb}" status=none
  mkfs.ext4 -F -q -d "${rootdir}" "${output}" >/dev/null
}

stage_alpine_root() {
  local root="$1"
  curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${root}"
  mkdir -p "${root}"/{code,tmp,usr/local/bin,proc,sys}
  make_dev_nodes "${root}"
  echo "nameserver 8.8.8.8" > "${root}/etc/resolv.conf"
}

apk_add() {
  local root="$1"
  shift
  local pkgs=("$@")
  chroot "${root}" /bin/sh -c "apk add --no-cache ${pkgs[*]}" >/dev/null 2>&1
}

inject_agent_init() {
  local root="$1"
  cp "${AGENT_BIN}" "${root}/init"
  chmod +x "${root}/init"
}

build_base_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  mkdir -p "${tmp}"/{dev,proc,sys,tmp,code,usr/local/bin}
  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/base.ext4" "${BASE_ROOTFS_SIZE_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "base.ext4 ready -> ${OUT_DIR}/base.ext4"
}

build_python_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  stage_alpine_root "${tmp}"
  apk_add "${tmp}" python3
  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/python.ext4" "${ROOTFS_SIZE_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "python.ext4 ready -> ${OUT_DIR}/python.ext4"
}

build_node_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  stage_alpine_root "${tmp}"
  apk_add "${tmp}" nodejs npm
  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/node.ext4" "${ROOTFS_SIZE_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "node.ext4 ready -> ${OUT_DIR}/node.ext4"
}

build_ruby_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  stage_alpine_root "${tmp}"
  apk_add "${tmp}" ruby
  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/ruby.ext4" "${ROOTFS_SIZE_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "ruby.ext4 ready -> ${OUT_DIR}/ruby.ext4"
}

build_java_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  stage_alpine_root "${tmp}"
  apk_add "${tmp}" openjdk21-jre-headless
  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/java.ext4" "${ROOTFS_SIZE_JAVA_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "java.ext4 ready -> ${OUT_DIR}/java.ext4"
}

build_wasm_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  stage_alpine_root "${tmp}"

  # wasmtime release is glibc-linked; add compatibility layer.
  apk_add "${tmp}" gcompat libstdc++

  local wasmtime_tmp
  wasmtime_tmp="$(mktemp -d)"
  curl -fsSL \
    "https://github.com/bytecodealliance/wasmtime/releases/download/${WASMTIME_VERSION}/wasmtime-${WASMTIME_VERSION}-x86_64-linux.tar.xz" \
    -o "${wasmtime_tmp}/wasmtime.tar.xz"
  tar -xJf "${wasmtime_tmp}/wasmtime.tar.xz" -C "${wasmtime_tmp}"
  cp "${wasmtime_tmp}/wasmtime-${WASMTIME_VERSION}-x86_64-linux/wasmtime" "${tmp}/usr/local/bin/wasmtime"
  chmod +x "${tmp}/usr/local/bin/wasmtime"
  rm -rf "${wasmtime_tmp}"

  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/wasm.ext4" "${ROOTFS_SIZE_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "wasm.ext4 ready -> ${OUT_DIR}/wasm.ext4"
}

build_php_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  stage_alpine_root "${tmp}"
  apk_add "${tmp}" php
  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/php.ext4" "${ROOTFS_SIZE_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "php.ext4 ready -> ${OUT_DIR}/php.ext4"
}

build_dotnet_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  stage_alpine_root "${tmp}"

  # Match dotnet runtime-deps for Alpine (https://github.com/dotnet/dotnet-docker)
  apk_add "${tmp}" ca-certificates-bundle libgcc libssl3 libstdc++ zlib

  local dotnet_tmp
  dotnet_tmp="$(mktemp -d)"
  curl -fsSL \
    "https://builds.dotnet.microsoft.com/dotnet/Runtime/${DOTNET_VERSION}/dotnet-runtime-${DOTNET_VERSION}-linux-musl-x64.tar.gz" \
    -o "${dotnet_tmp}/dotnet-runtime.tar.gz"
  mkdir -p "${tmp}/usr/share/dotnet"
  tar -xzf "${dotnet_tmp}/dotnet-runtime.tar.gz" -C "${tmp}/usr/share/dotnet"
  mkdir -p "${tmp}/usr/bin"
  ln -sf ../share/dotnet/dotnet "${tmp}/usr/bin/dotnet"
  rm -rf "${dotnet_tmp}"

  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/dotnet.ext4" "${ROOTFS_SIZE_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "dotnet.ext4 ready -> ${OUT_DIR}/dotnet.ext4"
}

build_deno_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  stage_alpine_root "${tmp}"

  # deno release is glibc-linked; add compatibility layer.
  apk_add "${tmp}" gcompat libstdc++

  local deno_tmp
  deno_tmp="$(mktemp -d)"
  curl -fsSL \
    "https://github.com/denoland/deno/releases/download/${DENO_VERSION}/deno-x86_64-unknown-linux-gnu.zip" \
    -o "${deno_tmp}/deno.zip"
  unzip -q -o "${deno_tmp}/deno.zip" -d "${deno_tmp}"
  cp "${deno_tmp}/deno" "${tmp}/usr/local/bin/deno"
  chmod +x "${tmp}/usr/local/bin/deno"
  rm -rf "${deno_tmp}"

  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/deno.ext4" "${ROOTFS_SIZE_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "deno.ext4 ready -> ${OUT_DIR}/deno.ext4"
}

build_bun_rootfs() {
  local tmp
  tmp="$(mktemp -d)"
  stage_alpine_root "${tmp}"

  # bun provides musl builds; only C++ runtime needed.
  apk_add "${tmp}" libgcc libstdc++

  local bun_tmp
  bun_tmp="$(mktemp -d)"
  curl -fsSL \
    "https://github.com/oven-sh/bun/releases/download/${BUN_VERSION}/bun-linux-x64-musl.zip" \
    -o "${bun_tmp}/bun.zip"
  unzip -q -o "${bun_tmp}/bun.zip" -d "${bun_tmp}"
  cp "${bun_tmp}/bun-linux-x64-musl/bun" "${tmp}/usr/local/bin/bun"
  chmod +x "${tmp}/usr/local/bin/bun"
  rm -rf "${bun_tmp}"

  inject_agent_init "${tmp}"
  build_image_from_dir "${OUT_DIR}/bun.ext4" "${ROOTFS_SIZE_MB}" "${tmp}"
  rm -rf "${tmp}"
  log "bun.ext4 ready -> ${OUT_DIR}/bun.ext4"
}

main() {
  check_platform
  require_cmd dd
  require_cmd mkfs.ext4
  require_cmd curl
  require_cmd tar
  require_cmd unzip
  require_cmd chroot

  resolve_agent

  mkdir -p "${OUT_DIR}"
  log "Output dir: ${OUT_DIR}"
  log "Using agent: ${AGENT_BIN}"

  build_base_rootfs
  build_python_rootfs
  build_node_rootfs
  build_ruby_rootfs
  build_java_rootfs
  build_wasm_rootfs
  build_php_rootfs
  build_dotnet_rootfs
  build_deno_rootfs
  build_bun_rootfs

  log "Done"
}

main "$@"
