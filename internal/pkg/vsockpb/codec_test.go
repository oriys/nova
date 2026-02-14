package vsockpb

import (
	"net"
	"testing"

	"github.com/oriys/nova/api/proto/agentpb"
)

func TestCodec_SendReceive(t *testing.T) {
	// Create a pipe to simulate a network connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sendCodec := NewCodec(client)
	recvCodec := NewCodec(server)

	// Send a ping message
	sent := &agentpb.VsockMessage{
		Type: agentpb.VsockMessage_TYPE_PING,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- sendCodec.Send(sent)
	}()

	received, err := recvCodec.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if received.Type != agentpb.VsockMessage_TYPE_PING {
		t.Fatalf("expected TYPE_PING, got %v", received.Type)
	}
}

func TestCodec_ExecPayload(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sendCodec := NewCodec(client)
	recvCodec := NewCodec(server)

	sent := &agentpb.VsockMessage{
		Type: agentpb.VsockMessage_TYPE_EXEC,
		Payload: &agentpb.VsockMessage_Exec{
			Exec: &agentpb.ExecPayload{
				RequestId:   "req-123",
				Input:       []byte(`{"key":"value"}`),
				TimeoutS:    30,
				TraceParent: "00-traceid-spanid-01",
				Stream:      false,
			},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- sendCodec.Send(sent)
	}()

	received, err := recvCodec.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if received.Type != agentpb.VsockMessage_TYPE_EXEC {
		t.Fatalf("expected TYPE_EXEC, got %v", received.Type)
	}

	exec := received.GetExec()
	if exec == nil {
		t.Fatal("expected exec payload")
	}
	if exec.RequestId != "req-123" {
		t.Fatalf("expected request_id 'req-123', got '%s'", exec.RequestId)
	}
	if string(exec.Input) != `{"key":"value"}` {
		t.Fatalf("unexpected input: %s", exec.Input)
	}
	if exec.TimeoutS != 30 {
		t.Fatalf("expected timeout 30, got %d", exec.TimeoutS)
	}
}

func TestCodec_InitPayload(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sendCodec := NewCodec(client)
	recvCodec := NewCodec(server)

	sent := &agentpb.VsockMessage{
		Type: agentpb.VsockMessage_TYPE_INIT,
		Payload: &agentpb.VsockMessage_Init{
			Init: &agentpb.InitPayload{
				Runtime:      "python",
				Handler:      "handler.main",
				EnvVars:      map[string]string{"KEY": "val"},
				Mode:         "process",
				FunctionName: "hello",
				MemoryMb:     128,
				TimeoutS:     30,
			},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- sendCodec.Send(sent)
	}()

	received, err := recvCodec.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	init := received.GetInit()
	if init == nil {
		t.Fatal("expected init payload")
	}
	if init.Runtime != "python" {
		t.Fatalf("expected runtime 'python', got '%s'", init.Runtime)
	}
	if init.FunctionName != "hello" {
		t.Fatalf("expected function_name 'hello', got '%s'", init.FunctionName)
	}
	if init.EnvVars["KEY"] != "val" {
		t.Fatalf("expected env var KEY=val, got %v", init.EnvVars)
	}
}

func TestCodec_RespPayload(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sendCodec := NewCodec(client)
	recvCodec := NewCodec(server)

	sent := &agentpb.VsockMessage{
		Type: agentpb.VsockMessage_TYPE_RESP,
		Payload: &agentpb.VsockMessage_Resp{
			Resp: &agentpb.RespPayload{
				RequestId:  "req-456",
				Output:     []byte(`{"result":42}`),
				DurationMs: 150,
				Stdout:     "some output",
			},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- sendCodec.Send(sent)
	}()

	received, err := recvCodec.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	resp := received.GetResp()
	if resp == nil {
		t.Fatal("expected resp payload")
	}
	if resp.RequestId != "req-456" {
		t.Fatalf("expected request_id 'req-456', got '%s'", resp.RequestId)
	}
	if resp.DurationMs != 150 {
		t.Fatalf("expected duration 150, got %d", resp.DurationMs)
	}
}
