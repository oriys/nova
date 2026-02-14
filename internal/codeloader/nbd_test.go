package codeloader

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestNBDServer_ReadBlocks(t *testing.T) {
	// Create test data (simulating a small ext4 image)
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	server := NewNBDServer(data)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := server.ListenAndServe(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenAndServe failed: %v", err)
	}
	defer server.Close()

	// Connect as NBD client
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// Read handshake (152 bytes)
	handshake := make([]byte, 152)
	if _, err := io.ReadFull(conn, handshake); err != nil {
		t.Fatalf("read handshake: %v", err)
	}

	// Verify magic
	if string(handshake[0:8]) != "NBDMAGIC" {
		t.Fatal("invalid handshake magic")
	}

	// Verify export size
	exportSize := binary.BigEndian.Uint64(handshake[16:24])
	if exportSize != uint64(len(data)) {
		t.Fatalf("expected size %d, got %d", len(data), exportSize)
	}

	// Send a read request for 512 bytes at offset 0
	readReq := make([]byte, 28)
	binary.BigEndian.PutUint32(readReq[0:4], nbdRequestMagic)
	binary.BigEndian.PutUint32(readReq[4:8], nbdCmdRead) // read
	// handle (8 bytes) - use a simple handle
	readReq[8] = 0x01
	binary.BigEndian.PutUint64(readReq[16:24], 0) // offset
	binary.BigEndian.PutUint32(readReq[24:28], 512) // length

	if _, err := conn.Write(readReq); err != nil {
		t.Fatalf("write read request: %v", err)
	}

	// Read reply header (16 bytes) + data (512 bytes)
	reply := make([]byte, 16+512)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("read reply: %v", err)
	}

	// Verify reply magic
	replyMagic := binary.BigEndian.Uint32(reply[0:4])
	if replyMagic != nbdReplyMagic {
		t.Fatalf("invalid reply magic: 0x%x", replyMagic)
	}

	// Verify no error
	replyErr := binary.BigEndian.Uint32(reply[4:8])
	if replyErr != 0 {
		t.Fatalf("unexpected error: %d", replyErr)
	}

	// Verify data matches
	for i := 0; i < 512; i++ {
		if reply[16+i] != data[i] {
			t.Fatalf("data mismatch at byte %d: got %d, want %d", i, reply[16+i], data[i])
		}
	}
}

func TestNBDServer_Disconnect(t *testing.T) {
	data := make([]byte, 1024)
	server := NewNBDServer(data)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := server.ListenAndServe(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Read handshake
	handshake := make([]byte, 152)
	io.ReadFull(conn, handshake)

	// Send disconnect
	discReq := make([]byte, 28)
	binary.BigEndian.PutUint32(discReq[0:4], nbdRequestMagic)
	binary.BigEndian.PutUint32(discReq[4:8], nbdCmdDisc)
	conn.Write(discReq)

	// Connection should close gracefully
	time.Sleep(50 * time.Millisecond)
}

func TestNBDServer_Close(t *testing.T) {
	data := make([]byte, 1024)
	server := NewNBDServer(data)
	ctx := context.Background()

	_, err := server.ListenAndServe(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	if err := server.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
