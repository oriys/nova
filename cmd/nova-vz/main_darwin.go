// nova-vz: Lightweight VM manager using Apple Virtualization.framework.
// Boots a Linux VM with virtio-blk, VirtioFS, vsock, and serial console.
// Provides vsock proxy (UNIX socket ↔ guest vsock) and lifecycle control socket.
//
// Build: CGO_ENABLED=1 go build -o bin/nova-vz ./cmd/nova-vz
// Requires codesign: codesign --entitlements cmd/nova-vz/nova-vz.entitlements --force -s - bin/nova-vz

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/Code-Hex/vz/v3"
)

type config struct {
	kernel        string
	initrd        string
	cmdline       string
	rootfs        string
	cpus          uint
	memoryMB      uint64
	sharedDir     string
	mountTag      string
	vsockPort     uint32
	vsockSocket   string
	controlSocket string
	serialLog     string
	restore       string
}

func main() {
	var cfg config
	flag.StringVar(&cfg.kernel, "kernel", "", "Linux kernel image (required)")
	flag.StringVar(&cfg.initrd, "initrd", "", "Initial ramdisk")
	flag.StringVar(&cfg.cmdline, "cmdline", "console=hvc0 root=/dev/vda rw", "Kernel command line")
	flag.StringVar(&cfg.rootfs, "rootfs", "", "Root filesystem image (required)")
	flag.UintVar(&cfg.cpus, "cpus", 1, "CPU count")
	flag.Uint64Var(&cfg.memoryMB, "memory", 256, "Memory in MB")
	flag.StringVar(&cfg.sharedDir, "shared-dir", "", "Host directory to share via VirtioFS")
	flag.StringVar(&cfg.mountTag, "mount-tag", "code", "VirtioFS mount tag")
	var vsockPort uint
	flag.UintVar(&vsockPort, "vsock-port", 9999, "Vsock port for guest agent")
	flag.StringVar(&cfg.vsockSocket, "vsock-socket", "", "UNIX socket path for vsock proxy")
	flag.StringVar(&cfg.controlSocket, "control-socket", "", "UNIX socket for lifecycle commands")
	flag.StringVar(&cfg.serialLog, "serial-log", "", "Serial console log file path")
	flag.StringVar(&cfg.restore, "restore", "", "Restore VM from saved state")
	flag.Parse()

	cfg.vsockPort = uint32(vsockPort)

	if cfg.kernel == "" || cfg.rootfs == "" {
		flag.Usage()
		os.Exit(1)
	}

	if err := run(cfg); err != nil {
		log.Fatalf("nova-vz: %v", err)
	}
}

func run(cfg config) error {
	// Build VM configuration
	vmConfig, err := vz.NewVirtualMachineConfiguration(
		mustBootLoader(cfg),
		cfg.cpus,
		cfg.memoryMB*1024*1024,
	)
	if err != nil {
		return fmt.Errorf("vm config: %w", err)
	}

	// Platform
	machineID, err := vz.NewGenericMachineIdentifier()
	if err != nil {
		return fmt.Errorf("machine id: %w", err)
	}
	platform, err := vz.NewGenericPlatformConfiguration(vz.WithGenericMachineIdentifier(machineID))
	if err != nil {
		return fmt.Errorf("platform: %w", err)
	}
	vmConfig.SetPlatformVirtualMachineConfiguration(platform)

	// Storage (virtio-blk)
	diskAttach, err := vz.NewDiskImageStorageDeviceAttachmentWithCacheAndSync(
		cfg.rootfs, false,
		vz.DiskImageCachingModeCached,
		vz.DiskImageSynchronizationModeFsync,
	)
	if err != nil {
		return fmt.Errorf("disk: %w", err)
	}
	blockDev, err := vz.NewVirtioBlockDeviceConfiguration(diskAttach)
	if err != nil {
		return fmt.Errorf("block dev: %w", err)
	}
	vmConfig.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{blockDev})

	// VirtioFS
	if cfg.sharedDir != "" {
		sd, err := vz.NewSharedDirectory(cfg.sharedDir, false)
		if err != nil {
			return fmt.Errorf("shared dir: %w", err)
		}
		share, err := vz.NewSingleDirectoryShare(sd)
		if err != nil {
			return fmt.Errorf("single dir share: %w", err)
		}
		fsConfig, err := vz.NewVirtioFileSystemDeviceConfiguration(cfg.mountTag)
		if err != nil {
			return fmt.Errorf("virtiofs: %w", err)
		}
		fsConfig.SetDirectoryShare(share)
		vmConfig.SetDirectorySharingDevicesVirtualMachineConfiguration([]vz.DirectorySharingDeviceConfiguration{fsConfig})
	}

	// Vsock
	vsockConfig, err := vz.NewVirtioSocketDeviceConfiguration()
	if err != nil {
		return fmt.Errorf("vsock: %w", err)
	}
	vmConfig.SetSocketDevicesVirtualMachineConfiguration([]vz.SocketDeviceConfiguration{vsockConfig})

	// Network (NAT)
	natAttach, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		return fmt.Errorf("nat: %w", err)
	}
	netConfig, err := vz.NewVirtioNetworkDeviceConfiguration(natAttach)
	if err != nil {
		return fmt.Errorf("net: %w", err)
	}
	vmConfig.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{netConfig})

	// Serial console
	serialAttach, err := newSerialAttachment(cfg)
	if err != nil {
		return fmt.Errorf("serial: %w", err)
	}
	consoleConfig, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(serialAttach)
	if err != nil {
		return fmt.Errorf("console: %w", err)
	}
	vmConfig.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{consoleConfig})

	// Entropy
	entropy, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return fmt.Errorf("entropy: %w", err)
	}
	vmConfig.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{entropy})

	// Memory balloon
	balloon, err := vz.NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
	if err != nil {
		return fmt.Errorf("balloon: %w", err)
	}
	vmConfig.SetMemoryBalloonDevicesVirtualMachineConfiguration([]vz.MemoryBalloonDeviceConfiguration{balloon})

	// Validate
	valid, err := vmConfig.Validate()
	if !valid || err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	// Create VM
	vm, err := vz.NewVirtualMachine(vmConfig)
	if err != nil {
		return fmt.Errorf("create vm: %w", err)
	}

	// Start or restore
	if cfg.restore != "" {
		if err := vm.RestoreMachineStateFromURL(cfg.restore); err != nil {
			return fmt.Errorf("restore: %w", err)
		}
		if err := vm.Resume(); err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		log.Println("nova-vz: VM restored and resumed")
	} else {
		if err := vm.Start(); err != nil {
			return fmt.Errorf("start: %w", err)
		}
		log.Println("nova-vz: VM started")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Vsock proxy
	if cfg.vsockSocket != "" {
		go startVsockProxy(ctx, vm, cfg.vsockSocket, cfg.vsockPort)
	}

	// Control socket
	if cfg.controlSocket != "" {
		go startControlServer(ctx, vm, cfg.controlSocket)
	}

	// Wait for VM state changes or signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	for {
		select {
		case sig := <-sigCh:
			log.Printf("nova-vz: received %v, stopping VM", sig)
			if vm.CanRequestStop() {
				if _, err := vm.RequestStop(); err != nil {
					log.Printf("nova-vz: request stop: %v", err)
					_ = vm.Stop()
				}
			} else {
				_ = vm.Stop()
			}
			cleanup(cfg)
			return nil
		case state := <-vm.StateChangedNotify():
			switch state {
			case vz.VirtualMachineStateRunning:
				log.Println("nova-vz: VM running")
			case vz.VirtualMachineStateStopped:
				log.Println("nova-vz: VM stopped")
				cleanup(cfg)
				return nil
			case vz.VirtualMachineStateError:
				cleanup(cfg)
				return fmt.Errorf("VM entered error state")
			}
		}
	}
}

func mustBootLoader(cfg config) vz.BootLoader {
	var opts []vz.LinuxBootLoaderOption
	opts = append(opts, vz.WithCommandLine(cfg.cmdline))
	if cfg.initrd != "" {
		opts = append(opts, vz.WithInitrd(cfg.initrd))
	}
	bl, err := vz.NewLinuxBootLoader(cfg.kernel, opts...)
	if err != nil {
		log.Fatalf("nova-vz: bootloader: %v", err)
	}
	return bl
}

func newSerialAttachment(cfg config) (vz.SerialPortAttachment, error) {
	if cfg.serialLog != "" {
		return vz.NewFileSerialPortAttachment(cfg.serialLog, false)
	}
	return vz.NewFileHandleSerialPortAttachment(os.Stdin, os.Stdout)
}

func cleanup(cfg config) {
	if cfg.vsockSocket != "" {
		os.Remove(cfg.vsockSocket)
	}
	if cfg.controlSocket != "" {
		os.Remove(cfg.controlSocket)
	}
}

// startVsockProxy creates a UNIX socket server; each accepted connection is proxied to guest vsock.
func startVsockProxy(ctx context.Context, vm *vz.VirtualMachine, sockPath string, port uint32) {
	os.Remove(sockPath)
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Printf("nova-vz: vsock proxy listen: %v", err)
		return
	}
	defer l.Close()
	log.Printf("nova-vz: Vsock proxy on %s → guest port %d", sockPath, port)

	go func() {
		<-ctx.Done()
		l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("nova-vz: vsock proxy accept: %v", err)
				continue
			}
		}
		go proxyVsock(vm, conn, port)
	}
}

func proxyVsock(vm *vz.VirtualMachine, hostConn net.Conn, port uint32) {
	defer hostConn.Close()

	var guestConn *vz.VirtioSocketConnection
	var err error
	for _, sock := range vm.SocketDevices() {
		guestConn, err = sock.Connect(port)
		if err == nil {
			break
		}
	}
	if err != nil {
		log.Printf("nova-vz: vsock connect port %d: %v", port, err)
		return
	}
	defer guestConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(guestConn, hostConn) }()
	go func() { defer wg.Done(); io.Copy(hostConn, guestConn) }()
	wg.Wait()
}

// Control socket: JSON request/response for VM lifecycle.
type controlRequest struct {
	Action string `json:"action"` // save, restore, pause, resume, stop, status
	Path   string `json:"path,omitempty"`
}

type controlResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	State string `json:"state,omitempty"`
}

func startControlServer(ctx context.Context, vm *vz.VirtualMachine, sockPath string) {
	os.Remove(sockPath)
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Printf("nova-vz: control listen: %v", err)
		return
	}
	defer l.Close()
	log.Printf("nova-vz: Control server on %s", sockPath)

	go func() {
		<-ctx.Done()
		l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		go handleControl(vm, conn)
	}
}

func handleControl(vm *vz.VirtualMachine, conn net.Conn) {
	defer conn.Close()

	var req controlRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		writeControlResp(conn, controlResponse{Error: err.Error()})
		return
	}

	var resp controlResponse
	switch req.Action {
	case "save":
		if req.Path == "" {
			resp = controlResponse{Error: "path required"}
		} else if err := vm.Pause(); err != nil {
			resp = controlResponse{Error: fmt.Sprintf("pause: %v", err)}
		} else if err := vm.SaveMachineStateToPath(req.Path); err != nil {
			_ = vm.Resume()
			resp = controlResponse{Error: fmt.Sprintf("save: %v", err)}
		} else if err := vm.Resume(); err != nil {
			resp = controlResponse{Error: fmt.Sprintf("resume after save: %v", err)}
		} else {
			resp = controlResponse{OK: true}
		}
	case "pause":
		if err := vm.Pause(); err != nil {
			resp = controlResponse{Error: err.Error()}
		} else {
			resp = controlResponse{OK: true}
		}
	case "resume":
		if err := vm.Resume(); err != nil {
			resp = controlResponse{Error: err.Error()}
		} else {
			resp = controlResponse{OK: true}
		}
	case "stop":
		if vm.CanRequestStop() {
			if _, err := vm.RequestStop(); err != nil {
				_ = vm.Stop()
			}
		} else {
			_ = vm.Stop()
		}
		resp = controlResponse{OK: true}
	case "status":
		resp = controlResponse{OK: true, State: vmStateString(vm.State())}
	default:
		resp = controlResponse{Error: fmt.Sprintf("unknown action: %s", req.Action)}
	}

	writeControlResp(conn, resp)
}

func writeControlResp(conn net.Conn, resp controlResponse) {
	json.NewEncoder(conn).Encode(resp)
}

func vmStateString(s vz.VirtualMachineState) string {
	switch s {
	case vz.VirtualMachineStateStopped:
		return "stopped"
	case vz.VirtualMachineStateRunning:
		return "running"
	case vz.VirtualMachineStatePaused:
		return "paused"
	case vz.VirtualMachineStateError:
		return "error"
	case vz.VirtualMachineStateStarting:
		return "starting"
	case vz.VirtualMachineStatePausing:
		return "pausing"
	case vz.VirtualMachineStateResuming:
		return "resuming"
	case vz.VirtualMachineStateStopping:
		return "stopping"
	case vz.VirtualMachineStateSaving:
		return "saving"
	case vz.VirtualMachineStateRestoring:
		return "restoring"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}
