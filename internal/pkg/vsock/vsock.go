package vsock

import (
	"fmt"
	"net"
)

// Listen is a stub for the mdlayher/vsock.Listen function.
// In a real environment, this would use the kernel's vsock capabilities.
// For this disconnected environment, it returns an error to force fallback to Unix sockets,
// or we could implement a mock if needed.
func Listen(port uint32, config interface{}) (net.Listener, error) {
	return nil, fmt.Errorf("vsock not implemented in this environment")
}
