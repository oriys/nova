// nova-vz: Lightweight VM manager using Apple Virtualization.framework.
// Supports direct kernel boot, VirtioFS, vsock, and VM state save/restore.
//
// Usage:
//   nova-vz --kernel PATH --rootfs PATH [options]
//
// Options:
//   --kernel PATH       Linux kernel image (required)
//   --initrd PATH       Initial ramdisk
//   --cmdline STR       Kernel command line (default: "console=hvc0 root=/dev/vda rw")
//   --rootfs PATH       Root filesystem image (required)
//   --cpus N            CPU count (default: 1)
//   --memory N          Memory in MB (default: 256)
//   --shared-dir PATH   Host directory to share via VirtioFS
//   --mount-tag TAG     VirtioFS mount tag (default: "code")
//   --vsock-port N      Vsock port for guest agent (default: 9999)
//   --vsock-socket PATH UNIX socket path for vsock proxy
//   --control-socket P  UNIX socket for lifecycle commands (save/restore/stop)
//   --restore PATH      Restore VM from saved state instead of fresh boot

import Foundation
import Virtualization

// MARK: - Configuration

struct VMConfig {
    var cpus: Int = 1
    var memoryMB: UInt64 = 256
    var kernelPath: String = ""
    var initrdPath: String? = nil
    var cmdline: String = "console=hvc0 root=/dev/vda rw"
    var rootfsPath: String = ""
    var sharedDir: String? = nil
    var mountTag: String = "code"
    var vsockPort: UInt32 = 9999
    var vsockSocket: String = ""
    var controlSocket: String = ""
    var restorePath: String? = nil
}

func parseArgs() -> VMConfig {
    var config = VMConfig()
    var args = CommandLine.arguments.dropFirst().makeIterator()

    while let arg = args.next() {
        switch arg {
        case "--cpus":         config.cpus = Int(args.next() ?? "1") ?? 1
        case "--memory":       config.memoryMB = UInt64(args.next() ?? "256") ?? 256
        case "--kernel":       config.kernelPath = args.next() ?? ""
        case "--initrd":       config.initrdPath = args.next()
        case "--cmdline":      config.cmdline = args.next() ?? config.cmdline
        case "--rootfs":       config.rootfsPath = args.next() ?? ""
        case "--shared-dir":   config.sharedDir = args.next()
        case "--mount-tag":    config.mountTag = args.next() ?? "code"
        case "--vsock-port":   config.vsockPort = UInt32(args.next() ?? "9999") ?? 9999
        case "--vsock-socket": config.vsockSocket = args.next() ?? ""
        case "--control-socket": config.controlSocket = args.next() ?? ""
        case "--restore":      config.restorePath = args.next()
        case "--help", "-h":
            printUsage()
            exit(0)
        default:
            log("Unknown argument: \(arg)")
            exit(1)
        }
    }

    guard !config.kernelPath.isEmpty else { die("--kernel is required") }
    guard !config.rootfsPath.isEmpty else { die("--rootfs is required") }
    return config
}

func printUsage() {
    fputs("""
    nova-vz: VM manager using Apple Virtualization.framework
    Usage: nova-vz --kernel PATH --rootfs PATH [options]
    Run nova-vz --help for full option list.
    \n
    """, stderr)
}

// MARK: - VM Controller

@MainActor
class VMController: NSObject, VZVirtualMachineDelegate {
    let vm: VZVirtualMachine
    let config: VMConfig
    private var vsockDevice: VZVirtioSocketDevice?
    private var proxyServerFD: Int32 = -1
    private var controlServerFD: Int32 = -1

    init(config: VMConfig) throws {
        self.config = config

        let vzConfig = VZVirtualMachineConfiguration()
        vzConfig.cpuCount = config.cpus
        vzConfig.memorySize = config.memoryMB * 1024 * 1024

        // Bootloader
        let bootloader = VZLinuxBootLoader(kernelURL: URL(fileURLWithPath: config.kernelPath))
        bootloader.commandLine = config.cmdline
        if let initrd = config.initrdPath {
            bootloader.initialRamdiskURL = URL(fileURLWithPath: initrd)
        }
        vzConfig.bootLoader = bootloader

        // Root filesystem (virtio-blk)
        let diskAttachment = try VZDiskImageStorageDeviceAttachment(
            url: URL(fileURLWithPath: config.rootfsPath), readOnly: false)
        vzConfig.storageDevices = [VZVirtioBlockDeviceConfiguration(attachment: diskAttachment)]

        // VirtioFS shared directory
        if let dir = config.sharedDir {
            let share = VZSharedDirectory(url: URL(fileURLWithPath: dir), readOnly: false)
            let fsDevice = VZVirtioFileSystemDeviceConfiguration(tag: config.mountTag)
            fsDevice.share = VZSingleDirectoryShare(directory: share)
            vzConfig.directorySharingDevices = [fsDevice]
        }

        // Vsock
        vzConfig.socketDevices = [VZVirtioSocketDeviceConfiguration()]

        // NAT networking
        let netDevice = VZVirtioNetworkDeviceConfiguration()
        netDevice.attachment = VZNATNetworkDeviceAttachment()
        vzConfig.networkDevices = [netDevice]

        // Serial console → stderr (for kernel logs)
        let console = VZVirtioConsoleDeviceSerialPortConfiguration()
        let readPipe = Pipe()  // unused read end; VZ requires a valid file handle
        console.attachment = VZFileHandleSerialPortAttachment(
            fileHandleForReading: readPipe.fileHandleForReading,
            fileHandleForWriting: FileHandle.standardError)
        vzConfig.serialPorts = [console]

        // Entropy source
        vzConfig.entropyDevices = [VZVirtioEntropyDeviceConfiguration()]

        try vzConfig.validate()

        self.vm = VZVirtualMachine(configuration: vzConfig)
        super.init()
        self.vm.delegate = self
    }

    // MARK: - Lifecycle

    func boot() async throws {
        if let restorePath = config.restorePath {
            log("Restoring VM state from \(restorePath)")
            try await vm.restoreMachineStateFrom(url: URL(fileURLWithPath: restorePath))
            try await vm.resume()
            log("VM restored and resumed")
        } else {
            try await vm.start()
            log("VM booted")
        }

        vsockDevice = vm.socketDevices.first as? VZVirtioSocketDevice
        if !config.vsockSocket.isEmpty { startVsockProxy() }
        if !config.controlSocket.isEmpty { startControlServer() }
    }

    // MARK: - Vsock UNIX Socket Proxy
    // Creates a UNIX socket server; each client connection is proxied to the guest's vsock port.

    func startVsockProxy() {
        let path = config.vsockSocket
        unlink(path)

        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { log("socket() failed: \(errno)"); return }

        guard bindUnix(fd: fd, path: path) else { close(fd); return }
        listen(fd, 8)
        proxyServerFD = fd

        let source = DispatchSource.makeReadSource(fileDescriptor: fd, queue: .global())
        source.setEventHandler { [weak self] in self?.acceptProxy(serverFD: fd) }
        source.setCancelHandler { close(fd); unlink(path) }
        source.resume()
        log("Vsock proxy on \(path)")
    }

    private func acceptProxy(serverFD: Int32) {
        let clientFD = accept(serverFD, nil, nil)
        guard clientFD >= 0, let vsock = vsockDevice else { if clientFD >= 0 { close(clientFD) }; return }

        vsock.connect(toPort: config.vsockPort) { result in
            switch result {
            case .success(let conn):
                bidirectionalProxy(fd1: clientFD, fd2: conn.fileDescriptor)
            case .failure(let error):
                log("vsock connect: \(error)")
                close(clientFD)
            }
        }
    }

    // MARK: - Control Server
    // Accepts JSON commands: save, restore, pause, resume, stop, status

    func startControlServer() {
        let path = config.controlSocket
        unlink(path)

        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { log("control socket() failed: \(errno)"); return }

        guard bindUnix(fd: fd, path: path) else { close(fd); return }
        listen(fd, 4)
        controlServerFD = fd

        let source = DispatchSource.makeReadSource(fileDescriptor: fd, queue: .global())
        source.setEventHandler { [weak self] in self?.acceptControl(serverFD: fd) }
        source.setCancelHandler { close(fd); unlink(path) }
        source.resume()
        log("Control server on \(path)")
    }

    private func acceptControl(serverFD: Int32) {
        let clientFD = accept(serverFD, nil, nil)
        guard clientFD >= 0 else { return }

        var buf = [UInt8](repeating: 0, count: 8192)
        let n = read(clientFD, &buf, buf.count)
        guard n > 0, let str = String(bytes: buf[..<n], encoding: .utf8),
              let json = try? JSONSerialization.jsonObject(with: Data(str.utf8)) as? [String: Any],
              let action = json["action"] as? String else {
            sendJSON(fd: clientFD, ["ok": false, "error": "invalid request"])
            return
        }

        Task { @MainActor [weak self] in
            guard let self = self else { return }
            await self.handleAction(action, json: json, clientFD: clientFD)
        }
    }

    private func handleAction(_ action: String, json: [String: Any], clientFD: Int32) async {
        do {
            switch action {
            case "save":
                guard let path = json["path"] as? String else {
                    sendJSON(fd: clientFD, ["ok": false, "error": "missing 'path'"]); return
                }
                try await vm.pause()
                try await vm.saveMachineStateTo(url: URL(fileURLWithPath: path))
                log("State saved to \(path)")
                sendJSON(fd: clientFD, ["ok": true])

            case "restore":
                guard let path = json["path"] as? String else {
                    sendJSON(fd: clientFD, ["ok": false, "error": "missing 'path'"]); return
                }
                try await vm.restoreMachineStateFrom(url: URL(fileURLWithPath: path))
                try await vm.resume()
                log("State restored from \(path)")
                sendJSON(fd: clientFD, ["ok": true])

            case "pause":
                try await vm.pause()
                sendJSON(fd: clientFD, ["ok": true])

            case "resume":
                try await vm.resume()
                sendJSON(fd: clientFD, ["ok": true])

            case "stop":
                sendJSON(fd: clientFD, ["ok": true])
                try vm.requestStop()

            case "status":
                let state: String
                switch vm.state {
                case .stopped: state = "stopped"
                case .running: state = "running"
                case .paused:  state = "paused"
                case .error:   state = "error"
                case .starting: state = "starting"
                case .stopping: state = "stopping"
                case .saving:  state = "saving"
                case .restoring: state = "restoring"
                case .pausing: state = "pausing"
                case .resuming: state = "resuming"
                @unknown default: state = "unknown"
                }
                sendJSON(fd: clientFD, ["ok": true, "state": state])

            default:
                sendJSON(fd: clientFD, ["ok": false, "error": "unknown action: \(action)"])
            }
        } catch {
            log("Control error (\(action)): \(error)")
            sendJSON(fd: clientFD, ["ok": false, "error": "\(error)"])
        }
    }

    // MARK: - VZVirtualMachineDelegate

    func virtualMachine(_ vm: VZVirtualMachine, didStopWithError error: Error) {
        log("VM stopped with error: \(error)")
        cleanup()
        exit(1)
    }

    func guestDidStop(_ vm: VZVirtualMachine) {
        log("Guest stopped")
        cleanup()
        exit(0)
    }

    private func cleanup() {
        if !config.vsockSocket.isEmpty { unlink(config.vsockSocket) }
        if !config.controlSocket.isEmpty { unlink(config.controlSocket) }
    }
}

// MARK: - Helpers

func log(_ msg: String) {
    fputs("nova-vz: \(msg)\n", stderr)
}

func die(_ msg: String) -> Never {
    fputs("nova-vz: error: \(msg)\n", stderr)
    exit(1)
}

func bindUnix(fd: Int32, path: String) -> Bool {
    var addr = sockaddr_un()
    addr.sun_family = sa_family_t(AF_UNIX)
    let maxLen = MemoryLayout.size(ofValue: addr.sun_path) - 1
    guard path.utf8.count < maxLen else { log("Socket path too long"); return false }

    _ = withUnsafeMutablePointer(to: &addr.sun_path) { sunPath in
        path.withCString { cstr in
            memcpy(sunPath, cstr, path.utf8.count + 1)
        }
    }

    let result = withUnsafePointer(to: &addr) { ptr in
        ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { saddr in
            bind(fd, saddr, socklen_t(MemoryLayout<sockaddr_un>.size))
        }
    }
    if result != 0 { log("bind(\(path)) failed: \(errno)"); return false }
    return true
}

func sendJSON(fd: Int32, _ dict: [String: Any]) {
    guard let data = try? JSONSerialization.data(withJSONObject: dict) else { close(fd); return }
    _ = data.withUnsafeBytes { ptr in write(fd, ptr.baseAddress!, data.count) }
    close(fd)
}

/// Bidirectional proxy between two file descriptors using GCD dispatch sources.
func bidirectionalProxy(fd1: Int32, fd2: Int32) {
    let q = DispatchQueue(label: "proxy-\(fd1)-\(fd2)")

    let s1 = DispatchSource.makeReadSource(fileDescriptor: fd1, queue: q)
    let s2 = DispatchSource.makeReadSource(fileDescriptor: fd2, queue: q)

    var alive = true

    func teardown() {
        guard alive else { return }
        alive = false
        s1.cancel(); s2.cancel()
        close(fd1); close(fd2)
    }

    s1.setEventHandler {
        var buf = [UInt8](repeating: 0, count: 65536)
        let n = read(fd1, &buf, buf.count)
        if n <= 0 { teardown(); return }
        _ = buf.withUnsafeBufferPointer { write(fd2, $0.baseAddress!, n) }
    }

    s2.setEventHandler {
        var buf = [UInt8](repeating: 0, count: 65536)
        let n = read(fd2, &buf, buf.count)
        if n <= 0 { teardown(); return }
        _ = buf.withUnsafeBufferPointer { write(fd1, $0.baseAddress!, n) }
    }

    s1.setCancelHandler {} // cleanup handled in teardown
    s2.setCancelHandler {}

    s1.resume()
    s2.resume()
}

// MARK: - Entry Point

let config = parseArgs()

Task { @MainActor in
    do {
        let controller = try VMController(config: config)
        _ = controller  // prevent dealloc
        try await controller.boot()
    } catch {
        die("Failed to start VM: \(error)")
    }
}

dispatchMain()
