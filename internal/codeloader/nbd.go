package codeloader

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/oriys/nova/internal/logging"
)

// NBD protocol constants
const (
	nbdRequestMagic  = 0x25609513
	nbdReplyMagic    = 0x67446698
	nbdCmdRead       = 0
	nbdCmdWrite      = 1
	nbdCmdDisc       = 2
	nbdFlagHasFlags  = 1 << 0
	nbdFlagReadOnly  = 1 << 1
	nbdFlagSendFlush = 1 << 2
)

// NBDServer serves code blocks on demand over the NBD (Network Block Device)
// protocol. Instead of writing the entire code image to disk upfront, blocks
// are served lazily from an in-memory or cached backing store.
//
// This reduces cold-start I/O: only the blocks the VM actually reads are
// transferred, which is typically a fraction of the total image size.
type NBDServer struct {
	mu       sync.Mutex
	data     []byte // backing data (code image in memory)
	size     int64  // exposed device size
	listener net.Listener
	done     chan struct{}
	conns    []net.Conn
}

// NewNBDServer creates a new NBD server backed by the given data.
// The data represents the full ext4 image contents.
func NewNBDServer(data []byte) *NBDServer {
	return &NBDServer{
		data: data,
		size: int64(len(data)),
		done: make(chan struct{}),
	}
}

// ListenAndServe starts the NBD server on the given address (e.g., "127.0.0.1:0").
// Returns the address the server is listening on.
func (s *NBDServer) ListenAndServe(ctx context.Context, addr string) (string, error) {
	var err error
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("nbd listen: %w", err)
	}

	listenAddr := s.listener.Addr().String()
	logging.Op().Info("NBD server started", "addr", listenAddr, "size", s.size)

	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.done:
					return
				default:
					continue
				}
			}
			s.mu.Lock()
			s.conns = append(s.conns, conn)
			s.mu.Unlock()

			go s.handleConnection(ctx, conn)
		}
	}()

	return listenAddr, nil
}

// Close shuts down the NBD server and all connections.
func (s *NBDServer) Close() error {
	close(s.done)
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, conn := range s.conns {
		conn.Close()
	}
	s.conns = nil
	return nil
}

// handleConnection performs the NBD handshake and serves block requests.
func (s *NBDServer) handleConnection(_ context.Context, conn net.Conn) {
	defer conn.Close()

	// Simplified NBD oldstyle handshake
	if err := s.sendHandshake(conn); err != nil {
		logging.Op().Warn("NBD handshake failed", "error", err)
		return
	}

	// Serve requests
	for {
		select {
		case <-s.done:
			return
		default:
		}

		if err := s.handleRequest(conn); err != nil {
			if err == io.EOF {
				return
			}
			return
		}
	}
}

// sendHandshake sends the NBD old-style negotiation.
func (s *NBDServer) sendHandshake(conn net.Conn) error {
	// NBD old-style handshake: magic + size + flags
	buf := make([]byte, 152)
	// init passwd: "NBDMAGIC"
	copy(buf[0:8], []byte("NBDMAGIC"))
	// cliserv_magic: 0x00420281861253
	binary.BigEndian.PutUint64(buf[8:16], 0x00420281861253)
	// export size
	binary.BigEndian.PutUint64(buf[16:24], uint64(s.size))
	// flags: read only + has flags
	binary.BigEndian.PutUint32(buf[24:28], nbdFlagHasFlags|nbdFlagReadOnly)
	// rest is zero padding (124 bytes reserved)

	_, err := conn.Write(buf)
	return err
}

// handleRequest reads and processes a single NBD request.
func (s *NBDServer) handleRequest(conn net.Conn) error {
	// NBD request header: magic(4) + type(4) + handle(8) + offset(8) + length(4) = 28 bytes
	header := make([]byte, 28)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}

	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != nbdRequestMagic {
		return fmt.Errorf("invalid NBD request magic: 0x%x", magic)
	}

	reqType := binary.BigEndian.Uint32(header[4:8])
	handle := header[8:16] // 8-byte opaque handle
	offset := binary.BigEndian.Uint64(header[16:24])
	length := binary.BigEndian.Uint32(header[24:28])

	switch reqType {
	case nbdCmdRead:
		return s.handleRead(conn, handle, offset, length)
	case nbdCmdDisc:
		return io.EOF // disconnect
	default:
		return s.sendError(conn, handle, 1) // EPERM
	}
}

// handleRead serves a block read request from the backing data.
func (s *NBDServer) handleRead(conn net.Conn, handle []byte, offset uint64, length uint32) error {
	// Build reply: magic(4) + error(4) + handle(8) + data
	reply := make([]byte, 16+length)
	binary.BigEndian.PutUint32(reply[0:4], nbdReplyMagic)
	binary.BigEndian.PutUint32(reply[4:8], 0) // no error
	copy(reply[8:16], handle)

	// Copy data from backing store
	end := offset + uint64(length)
	if end > uint64(len(s.data)) {
		end = uint64(len(s.data))
	}
	if offset < uint64(len(s.data)) {
		copy(reply[16:], s.data[offset:end])
	}
	// Any bytes beyond data are already zero-filled

	_, err := conn.Write(reply)
	return err
}

// sendError sends an error reply for unsupported operations.
func (s *NBDServer) sendError(conn net.Conn, handle []byte, errCode uint32) error {
	reply := make([]byte, 16)
	binary.BigEndian.PutUint32(reply[0:4], nbdReplyMagic)
	binary.BigEndian.PutUint32(reply[4:8], errCode)
	copy(reply[8:16], handle)
	_, err := conn.Write(reply)
	return err
}
