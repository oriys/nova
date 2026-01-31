package vsock

import (
	"net"

	"github.com/mdlayher/vsock"
)

// Listen creates a vsock listener on the specified port.
// Uses the mdlayher/vsock package for AF_VSOCK support.
func Listen(port uint32, config interface{}) (net.Listener, error) {
	return vsock.Listen(port, nil)
}
