// Package vsockpb provides a Protobuf-based codec for the vsock communication
// protocol between the host and guest agent. It complements the existing
// JSON-based protocol, offering reduced CPU overhead and smaller message sizes.
//
// The codec uses the same 4-byte big-endian length prefix framing, but
// encodes the payload using Protocol Buffers instead of JSON.
//
// Usage:
//
//	codec := vsockpb.NewCodec(conn)
//	// Send a message
//	msg := &agentpb.VsockMessage{Type: agentpb.VsockMessage_TYPE_PING}
//	codec.Send(msg)
//	// Receive a message
//	resp, err := codec.Receive()
package vsockpb

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/oriys/nova/api/proto/agentpb"
	"google.golang.org/protobuf/proto"
)

const maxMessageBytes = 8 * 1024 * 1024 // 8MB, same as JSON protocol

// Codec handles protobuf serialization over a length-prefixed connection.
type Codec struct {
	conn net.Conn
}

// NewCodec creates a new protobuf codec wrapping the given connection.
func NewCodec(conn net.Conn) *Codec {
	return &Codec{conn: conn}
}

// Send marshals a VsockMessage to protobuf and writes it with a 4-byte
// big-endian length prefix.
func (c *Codec) Send(msg *agentpb.VsockMessage) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("protobuf marshal: %w", err)
	}

	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(data)))
	copy(buf[4:], data)

	_, err = c.conn.Write(buf)
	return err
}

// Receive reads a length-prefixed protobuf message from the connection.
func (c *Codec) Receive() (*agentpb.VsockMessage, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)
	if msgLen > maxMessageBytes {
		return nil, fmt.Errorf("vsock protobuf message too large: %d bytes", msgLen)
	}

	data := make([]byte, msgLen)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return nil, err
	}

	msg := &agentpb.VsockMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("protobuf unmarshal: %w", err)
	}
	return msg, nil
}
