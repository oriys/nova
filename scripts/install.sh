#!/bin/bash
# Nova Serverless Platform - Linux Server Setup
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/oriys/nova/main/scripts/install.sh | sudo bash
# Or:
#   scp scripts/install.sh user@server:/tmp/ && ssh user@server 'sudo bash /tmp/install.sh'

set -e

PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt/nova/bin"
export PATH

INSTALL_DIR="/opt/nova"
FC_VERSION="latest"
ALPINE_URL="https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/x86_64/alpine-minirootfs-3.23.3-x86_64.tar.gz"
WASMTIME_VERSION="v41.0.1"
DENO_VERSION="v2.6.7"
BUN_VERSION="bun-v1.3.8"
DOTNET_VERSION="8.0.23"
ROOTFS_SIZE_MB=256
ROOTFS_SIZE_JAVA_MB=512

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log()  { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[!]${NC} $1" >&2; exit 1; }

# ─── Checks ──────────────────────────────────────────────
check_root()   { [[ $EUID -eq 0 ]] || err "Run as root: sudo $0"; }
check_system() {
    [[ "$(uname)" == "Linux" ]]   || err "Linux only"
    [[ "$(uname -m)" == "x86_64" ]] || err "x86_64 only"
    [[ -e /dev/kvm ]] || warn "/dev/kvm not found - Firecracker needs KVM"
}

install_deps() {
    log "Installing dependencies..."
    if command -v apt-get &>/dev/null; then
        apt-get update -qq
        apt-get install -y -qq curl e2fsprogs unzip iproute2 >/dev/null
    elif command -v yum &>/dev/null; then
        yum install -y -q curl e2fsprogs unzip iproute
    fi
}

latest_firecracker_version() {
    local release_url="https://github.com/firecracker-microvm/firecracker/releases"
    basename "$(curl -fsSLI -o /dev/null -w "%{url_effective}" ${release_url}/latest)"
}

# ─── Firecracker ─────────────────────────────────────────
install_firecracker() {
    if [[ "${FC_VERSION}" == "latest" || -z "${FC_VERSION}" ]]; then
        FC_VERSION="$(latest_firecracker_version)"
    fi
    local arch="$(uname -m)"
    local fc_bin="${INSTALL_DIR}/bin/firecracker-${FC_VERSION}-${arch}"
    local jailer_bin="${INSTALL_DIR}/bin/jailer-${FC_VERSION}-${arch}"

    if [[ -x "${INSTALL_DIR}/bin/firecracker" ]]; then
        warn "Existing Firecracker detected: $(${INSTALL_DIR}/bin/firecracker --version) - overwriting"
    fi
    log "Installing Firecracker ${FC_VERSION}..."
    local tmp=$(mktemp -d)
    local fc_url="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${arch}.tgz"
    curl -fsSL -o "${tmp}/fc.tgz" "${fc_url}"
    tar -xzf "${tmp}/fc.tgz" -C "${tmp}"
    local installed_fc=""
    local installed_jailer=""
    # Handle both old (release-*/) and new (flat) archive structures
    if ls ${tmp}/release-*/firecracker-* &>/dev/null 2>&1; then
        local fc_src
        local jailer_src
        fc_src="$(ls -1 ${tmp}/release-*/firecracker-* | head -n 1)"
        jailer_src="$(ls -1 ${tmp}/release-*/jailer-* | head -n 1)"
        install -m 0755 "${fc_src}" "${INSTALL_DIR}/bin"
        install -m 0755 "${jailer_src}" "${INSTALL_DIR}/bin"
        installed_fc="${INSTALL_DIR}/bin/$(basename "${fc_src}")"
        installed_jailer="${INSTALL_DIR}/bin/$(basename "${jailer_src}")"
    else
        install -m 0755 "${tmp}/firecracker-${FC_VERSION}-${arch}" "${INSTALL_DIR}/bin"
        install -m 0755 "${tmp}/jailer-${FC_VERSION}-${arch}" "${INSTALL_DIR}/bin"
        installed_fc="${fc_bin}"
        installed_jailer="${jailer_bin}"
    fi
    rm -rf "${tmp}"

    # Stable symlinks used by Nova defaults/configs.
    ln -sf "${installed_fc}" "${INSTALL_DIR}/bin/firecracker"
    ln -sf "${installed_jailer}" "${INSTALL_DIR}/bin/jailer"

    # Also expose in /usr/local/bin for convenience.
    ln -sf "${INSTALL_DIR}/bin/firecracker" /usr/local/bin/firecracker
    ln -sf "${INSTALL_DIR}/bin/jailer" /usr/local/bin/jailer

    log "Firecracker $(${INSTALL_DIR}/bin/firecracker --version)"
}

# ─── Kernel ──────────────────────────────────────────────
download_kernel() {
    log "Downloading kernel..."
    mkdir -p ${INSTALL_DIR}/kernel
    local arch
    local latest_version
    local ci_version
    local kernel_key
    arch="$(uname -m)"
    latest_version="$(latest_firecracker_version)"
    # Extract major.minor from version (e.g., v1.14.1 -> v1.14)
    ci_version="${latest_version%.*}"

    # Try to find kernel from Firecracker CI bucket
    kernel_key=$(curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${ci_version}/${arch}/vmlinux-&list-type=2" 2>/dev/null \
        | grep -oP "(?<=<Key>)(firecracker-ci/${ci_version}/${arch}/vmlinux-[0-9]+\\.[0-9]+\\.[0-9]{1,3})(?=</Key>)" \
        | sort -V | tail -1)

    # Fallback: try previous minor version if current not found
    if [[ -z "${kernel_key}" ]]; then
        local major_minor="${ci_version#v}"
        local major="${major_minor%.*}"
        local minor="${major_minor#*.}"
        if [[ ${minor} -gt 0 ]]; then
            local fallback_version="v${major}.$((minor - 1))"
            warn "Kernel not found for ${ci_version}, trying ${fallback_version}"
            kernel_key=$(curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${fallback_version}/${arch}/vmlinux-&list-type=2" 2>/dev/null \
                | grep -oP "(?<=<Key>)(firecracker-ci/${fallback_version}/${arch}/vmlinux-[0-9]+\\.[0-9]+\\.[0-9]{1,3})(?=</Key>)" \
                | sort -V | tail -1)
        fi
    fi

    if [[ -z "${kernel_key}" ]]; then
        err "Failed to locate Firecracker CI kernel for ${ci_version}/${arch}"
    fi
    curl -fsSL -o "${INSTALL_DIR}/kernel/vmlinux" "https://s3.amazonaws.com/spec.ccfc.min/${kernel_key}"
    log "Kernel: ${INSTALL_DIR}/kernel/vmlinux ($(du -h ${INSTALL_DIR}/kernel/vmlinux | cut -f1))"
}

# ─── Rootfs builder ─────────────────────────────────────
#
# Images produced (shared read-only rootfs per runtime):
#   base.ext4   - minimal rootfs (init + /code) for static binaries (Go/Rust)
#   python.ext4 - Alpine + python3
#   wasm.ext4   - Alpine + wasmtime (+ glibc compat)
#   node.ext4   - Alpine + nodejs
#   ruby.ext4   - Alpine + ruby
#   java.ext4   - Alpine + OpenJDK
#   php.ext4    - Alpine + php
#   dotnet.ext4 - Alpine + .NET runtime (musl)
#   deno.ext4   - Alpine + deno (+ glibc compat)
#   bun.ext4    - Alpine + bun (musl)
#
build_base_rootfs() {
    local output="${INSTALL_DIR}/rootfs/base.ext4"
    local mnt=$(mktemp -d)

    log "Building base rootfs (minimal, no distro)..."
    dd if=/dev/zero of="${output}" bs=1M count=32 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    # Minimal directory structure
    mkdir -p "${mnt}"/{dev,proc,sys,tmp,code,usr/local/bin}

    # init = nova-agent (static binary)
    if [[ -f ${INSTALL_DIR}/bin/nova-agent ]]; then
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init"
        chmod +x "${mnt}/init"
    else
        # Placeholder init that waits for agent to be injected
        cat > "${mnt}/init" << 'INIT'
#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
echo "ERROR: nova-agent not found, halting"
sleep infinity
INIT
        chmod +x "${mnt}/init"
    fi

    umount "${mnt}" && rmdir "${mnt}"
    log "base.ext4 ready ($(du -h ${output} | cut -f1)) - Go/Rust runtime"
}

build_python_rootfs() {
    local output="${INSTALL_DIR}/rootfs/python.ext4"
    local mnt=$(mktemp -d)

    log "Building python rootfs (Alpine + python3)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache python3" >/dev/null 2>&1

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "python.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_wasm_rootfs() {
    local output="${INSTALL_DIR}/rootfs/wasm.ext4"
    local mnt=$(mktemp -d)

    log "Building wasm rootfs (Alpine + wasmtime)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp,usr/local/bin}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    # wasmtime release is glibc-linked; add compatibility layer.
    chroot "${mnt}" /bin/sh -c "apk add --no-cache libstdc++ gcompat" >/dev/null 2>&1

    curl -fsSL \
        "https://github.com/bytecodealliance/wasmtime/releases/download/${WASMTIME_VERSION}/wasmtime-${WASMTIME_VERSION}-x86_64-linux.tar.xz" \
        | tar -xJf - -C "${mnt}/usr/local/bin" --strip-components=1 --wildcards '*/wasmtime'

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "wasm.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_node_rootfs() {
    local output="${INSTALL_DIR}/rootfs/node.ext4"
    local mnt=$(mktemp -d)

    log "Building node rootfs (Alpine + nodejs)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache nodejs npm" >/dev/null 2>&1

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "node.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_ruby_rootfs() {
    local output="${INSTALL_DIR}/rootfs/ruby.ext4"
    local mnt=$(mktemp -d)

    log "Building ruby rootfs (Alpine + ruby)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache ruby" >/dev/null 2>&1

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "ruby.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_java_rootfs() {
    local output="${INSTALL_DIR}/rootfs/java.ext4"
    local mnt=$(mktemp -d)

    log "Building java rootfs (Alpine + OpenJDK)..."
    # Java needs more space
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_JAVA_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    # Use OpenJDK 21 (LTS) headless for smaller size
    chroot "${mnt}" /bin/sh -c "apk add --no-cache openjdk21-jre-headless" >/dev/null 2>&1

    chroot "${mnt}" /bin/sh -c 'jli="$(find /usr/lib/jvm -name libjli.so | head -n1)"; [ -n "$jli" ] && ln -sf "$jli" /usr/lib/libjli.so' >/dev/null 2>&1

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "java.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_php_rootfs() {
    local output="${INSTALL_DIR}/rootfs/php.ext4"
    local mnt=$(mktemp -d)

    log "Building php rootfs (Alpine + php)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    chroot "${mnt}" /bin/sh -c "apk add --no-cache php" >/dev/null 2>&1

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "php.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_dotnet_rootfs() {
    local output="${INSTALL_DIR}/rootfs/dotnet.ext4"
    local mnt=$(mktemp -d)

    log "Building dotnet rootfs (Alpine + .NET runtime)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp,usr/share/dotnet}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    # Match dotnet runtime-deps for Alpine (https://github.com/dotnet/dotnet-docker)
    chroot "${mnt}" /bin/sh -c "apk add --no-cache ca-certificates-bundle libgcc libssl3 libstdc++ zlib" >/dev/null 2>&1

    # Download and install .NET Runtime (musl)
    curl -fsSL \
        "https://builds.dotnet.microsoft.com/dotnet/Runtime/${DOTNET_VERSION}/dotnet-runtime-${DOTNET_VERSION}-linux-musl-x64.tar.gz" \
        -o /tmp/dotnet-runtime.tar.gz
    tar -xzf /tmp/dotnet-runtime.tar.gz -C "${mnt}/usr/share/dotnet"
    ln -sf /usr/share/dotnet/dotnet "${mnt}/usr/bin/dotnet"
    rm -f /tmp/dotnet-runtime.tar.gz

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "dotnet.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_deno_rootfs() {
    local output="${INSTALL_DIR}/rootfs/deno.ext4"
    local mnt=$(mktemp -d)

    log "Building deno rootfs (Alpine + deno)..."

    # Build in a temp directory instead of a mounted ext4 image.
    # build-base (gcc) temporarily needs more space than the 256MB rootfs allows.
    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{dev,code,tmp,usr/local/bin}
    mknod -m 666 "${mnt}/dev/null" c 1 3 2>/dev/null || true
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    # deno release is glibc-linked; add compatibility layer.
    chroot "${mnt}" /bin/sh -c "apk add --no-cache libstdc++ gcompat" >/dev/null 2>&1

    # gcompat does not provide __res_init (glibc resolver symbol);
    # build a minimal stub so the dynamic linker can resolve it.
    chroot "${mnt}" /bin/sh -c "apk add --no-cache build-base" >/dev/null 2>&1
    printf 'int __res_init(void){return 0;}\n' > "${mnt}/tmp/res_stub.c"
    chroot "${mnt}" /bin/sh -c "gcc -shared -o /lib/libresolv_stub.so /tmp/res_stub.c"
    rm -f "${mnt}/tmp/res_stub.c"
    chroot "${mnt}" /bin/sh -c "apk del build-base" >/dev/null 2>&1

    # Download Deno binary
    curl -fsSL \
        "https://github.com/denoland/deno/releases/download/${DENO_VERSION}/deno-x86_64-unknown-linux-gnu.zip" \
        -o /tmp/deno.zip
    unzip -q -o /tmp/deno.zip -d "${mnt}/usr/local/bin"
    chmod +x "${mnt}/usr/local/bin/deno"
    rm -f /tmp/deno.zip

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

    # Create the ext4 image from the populated directory.
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q -d "${mnt}" "${output}" >/dev/null
    rm -rf "${mnt}"
    log "deno.ext4 ready ($(du -h ${output} | cut -f1))"
}

build_bun_rootfs() {
    local output="${INSTALL_DIR}/rootfs/bun.ext4"
    local mnt=$(mktemp -d)

    log "Building bun rootfs (Alpine + bun)..."
    dd if=/dev/zero of="${output}" bs=1M count=${ROOTFS_SIZE_MB} 2>/dev/null
    mkfs.ext4 -F -q "${output}"
    mount -o loop "${output}" "${mnt}"

    curl -fsSL "${ALPINE_URL}" | tar -xzf - -C "${mnt}"
    mkdir -p "${mnt}"/{code,tmp,usr/local/bin}
    echo "nameserver 8.8.8.8" > "${mnt}/etc/resolv.conf"

    # Bun provides musl builds; install runtime deps.
    chroot "${mnt}" /bin/sh -c "apk add --no-cache libgcc libstdc++" >/dev/null 2>&1

    # Download Bun binary
    curl -fsSL \
        "https://github.com/oven-sh/bun/releases/download/${BUN_VERSION}/bun-linux-x64-musl.zip" \
        -o /tmp/bun.zip
    unzip -q -o /tmp/bun.zip -d /tmp/bun-extract
    cp /tmp/bun-extract/bun-linux-x64-musl/bun "${mnt}/usr/local/bin/bun"
    chmod +x "${mnt}/usr/local/bin/bun"
    rm -rf /tmp/bun.zip /tmp/bun-extract

    # init = nova-agent
    [[ -f ${INSTALL_DIR}/bin/nova-agent ]] && \
        cp ${INSTALL_DIR}/bin/nova-agent "${mnt}/init" && \
        chmod +x "${mnt}/init"

    umount "${mnt}" && rmdir "${mnt}"
    log "bun.ext4 ready ($(du -h ${output} | cut -f1))"
}

# ─── Redis ───────────────────────────────────────────────
install_redis() {
    if command -v redis-server &>/dev/null; then
        log "Redis already installed"
        return
    fi
    log "Installing Redis..."
    if command -v apt-get &>/dev/null; then
        apt-get install -y -qq redis-server >/dev/null
        systemctl enable --now redis-server
    elif command -v yum &>/dev/null; then
        yum install -y -q redis
        systemctl enable --now redis
    else
        warn "Install Redis manually"
    fi
}

# ─── Postgres ────────────────────────────────────────────
install_postgres() {
    if command -v psql &>/dev/null; then
        log "Postgres already installed"
    else
        log "Installing Postgres..."
        if command -v apt-get &>/dev/null; then
            apt-get install -y -qq postgresql postgresql-contrib >/dev/null
            systemctl enable --now postgresql
        elif command -v yum &>/dev/null; then
            warn "Postgres install not implemented for yum-based distros. Install Postgres manually."
            return
        else
            warn "Install Postgres manually"
            return
        fi
    fi

    # Bootstrap a local DB/user for nova (best-effort).
    if command -v psql &>/dev/null; then
        if id -u postgres &>/dev/null 2>&1; then
            if ! su - postgres -c "psql -tAc \"SELECT 1 FROM pg_roles WHERE rolname='nova'\"" | grep -q 1; then
                su - postgres -c "psql -c \"CREATE USER nova WITH PASSWORD 'nova';\""
            fi
            if ! su - postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='nova'\"" | grep -q 1; then
                su - postgres -c "psql -c \"CREATE DATABASE nova OWNER nova;\""
            fi
            log "Postgres configured (db=nova user=nova)"
            log "DSN: postgres://nova:nova@localhost:5432/nova?sslmode=disable"
        else
            warn "Postgres installed, but OS user 'postgres' not found. Create db/user manually."
        fi
    fi
}

# ─── Main ────────────────────────────────────────────────
main() {
    check_root
    check_system
    install_deps

    # Create directories (idempotent - creates if not exist, no error if exist)
    log "Setting up directories..."
    mkdir -p ${INSTALL_DIR}/{kernel,rootfs,bin,snapshots}
    mkdir -p /tmp/nova/{sockets,vsock,logs}
    chmod 755 ${INSTALL_DIR} ${INSTALL_DIR}/{kernel,rootfs,bin,snapshots}

    install_firecracker
    download_kernel

    build_base_rootfs
    build_python_rootfs
    build_wasm_rootfs
    build_node_rootfs
    build_ruby_rootfs
    build_java_rootfs
    build_php_rootfs
    build_dotnet_rootfs
    build_deno_rootfs
    build_bun_rootfs

    install_postgres
    install_redis

    # Permissions
    chmod 666 /dev/kvm 2>/dev/null || true

    echo ""
    echo "========================================"
    echo "  Nova Setup Complete"
    echo "========================================"
    echo ""
    echo "  Installed to: ${INSTALL_DIR}"
    echo ""
    echo "  ${INSTALL_DIR}/bin/           (nova, nova-agent)"
    echo "  ${INSTALL_DIR}/kernel/vmlinux"
    echo "  ${INSTALL_DIR}/rootfs/base.ext4     (Go, Rust)"
    echo "  ${INSTALL_DIR}/rootfs/python.ext4   (Python)"
    echo "  ${INSTALL_DIR}/rootfs/wasm.ext4     (WASM)"
    echo "  ${INSTALL_DIR}/rootfs/node.ext4     (Node.js)"
    echo "  ${INSTALL_DIR}/rootfs/ruby.ext4     (Ruby)"
    echo "  ${INSTALL_DIR}/rootfs/java.ext4     (Java)"
    echo "  ${INSTALL_DIR}/rootfs/php.ext4      (PHP)"
    echo "  ${INSTALL_DIR}/rootfs/dotnet.ext4   (.NET)"
    echo "  ${INSTALL_DIR}/rootfs/deno.ext4     (Deno)"
    echo "  ${INSTALL_DIR}/rootfs/bun.ext4      (Bun)"
    echo "  ${INSTALL_DIR}/snapshots/     (VM snapshots)"
    echo ""
    echo "  Next: copy nova and nova-agent binaries to ${INSTALL_DIR}/bin/"
    echo ""
}

main "$@"
