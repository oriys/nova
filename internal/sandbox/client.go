// Package sandbox provides the sandbox execution client that communicates
// with the guest agent over vsock for shell execution, file operations,
// and process management.
package sandbox

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/oriys/nova/internal/backend"
	fc "github.com/oriys/nova/internal/firecracker"
)

// Client wraps a backend.Client (typically a VsockClient) and provides
// sandbox-specific operations: shell exec, file I/O, process management.
type Client struct {
	raw backend.Client
	vm  *backend.VM
}

// NewClient creates a sandbox client from an existing backend client and VM.
func NewClient(vm *backend.VM, raw backend.Client) *Client {
	return &Client{raw: raw, vm: vm}
}

// Close closes the underlying client connection.
func (c *Client) Close() error {
	return c.raw.Close()
}

// Ping checks if the agent is alive.
func (c *Client) Ping() error {
	return c.raw.Ping()
}

// ShellExec executes a shell command and returns stdout/stderr.
func (c *Client) ShellExec(command string, timeoutS int, workDir string) (*fc.ShellExecRespPayload, error) {
	payload, err := json.Marshal(&fc.ShellExecPayload{
		Command:  command,
		TimeoutS: timeoutS,
		WorkDir:  workDir,
	})
	if err != nil {
		return nil, err
	}

	resp, err := c.sendAndReceive(fc.MsgTypeShellExec, payload)
	if err != nil {
		return nil, err
	}

	var result fc.ShellExecRespPayload
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal shell exec response: %w", err)
	}
	return &result, nil
}

// FileRead reads a file from the sandbox.
func (c *Client) FileRead(path string) (*fc.FileRespPayload, error) {
	payload, err := json.Marshal(&fc.FileReadPayload{Path: path})
	if err != nil {
		return nil, err
	}

	resp, err := c.sendAndReceive(fc.MsgTypeFileRead, payload)
	if err != nil {
		return nil, err
	}

	var result fc.FileRespPayload
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal file read response: %w", err)
	}
	return &result, nil
}

// FileWrite writes a file in the sandbox.
func (c *Client) FileWrite(path string, content string, perm int) (*fc.FileRespPayload, error) {
	payload, err := json.Marshal(&fc.FileWritePayload{
		Path:    path,
		Content: content,
		Perm:    perm,
	})
	if err != nil {
		return nil, err
	}

	resp, err := c.sendAndReceive(fc.MsgTypeFileWrite, payload)
	if err != nil {
		return nil, err
	}

	var result fc.FileRespPayload
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal file write response: %w", err)
	}
	return &result, nil
}

// FileList lists directory contents in the sandbox.
func (c *Client) FileList(path string) (*fc.FileRespPayload, error) {
	payload, err := json.Marshal(&fc.FileListPayload{Path: path})
	if err != nil {
		return nil, err
	}

	resp, err := c.sendAndReceive(fc.MsgTypeFileList, payload)
	if err != nil {
		return nil, err
	}

	var result fc.FileRespPayload
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal file list response: %w", err)
	}
	return &result, nil
}

// FileDelete deletes a file or directory in the sandbox.
func (c *Client) FileDelete(path string) (*fc.FileRespPayload, error) {
	payload, err := json.Marshal(&fc.FileDeletePayload{Path: path})
	if err != nil {
		return nil, err
	}

	resp, err := c.sendAndReceive(fc.MsgTypeFileDelete, payload)
	if err != nil {
		return nil, err
	}

	var result fc.FileRespPayload
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal file delete response: %w", err)
	}
	return &result, nil
}

// ProcessList returns all running processes in the sandbox.
func (c *Client) ProcessList() (*fc.ProcessListRespPayload, error) {
	resp, err := c.sendAndReceive(fc.MsgTypeProcessList, json.RawMessage("{}"))
	if err != nil {
		return nil, err
	}

	var result fc.ProcessListRespPayload
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal process list response: %w", err)
	}
	return &result, nil
}

// ProcessKill kills a process by PID.
func (c *Client) ProcessKill(pid int, signal int) (*fc.ProcessKillRespPayload, error) {
	payload, err := json.Marshal(&fc.ProcessKillPayload{PID: pid, Signal: signal})
	if err != nil {
		return nil, err
	}

	resp, err := c.sendAndReceive(fc.MsgTypeProcessKill, payload)
	if err != nil {
		return nil, err
	}

	var result fc.ProcessKillRespPayload
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal process kill response: %w", err)
	}
	return &result, nil
}

// sendAndReceive sends a vsock message and returns the response payload.
// It uses the underlying VsockClient's Send/Receive methods.
func (c *Client) sendAndReceive(msgType int, payload json.RawMessage) (json.RawMessage, error) {
	// Try to extract the inner VsockClient from the adapter wrapper
	type vsockClientUnwrapper interface {
		InnerVsockClient() *fc.VsockClient
	}

	var vc *fc.VsockClient
	if unwrapper, ok := c.raw.(vsockClientUnwrapper); ok {
		vc = unwrapper.InnerVsockClient()
	} else {
		return nil, fmt.Errorf("sandbox client requires VsockClient backend (via adapter), got %T", c.raw)
	}

	// Ensure we have a connection (connect only if not already connected)
	if err := vc.ConnectIfNeeded(5 * time.Second); err != nil {
		return nil, fmt.Errorf("connect to sandbox agent: %w", err)
	}

	msg := &fc.VsockMessage{Type: msgType, Payload: payload}
	if err := vc.Send(msg); err != nil {
		return nil, fmt.Errorf("send message type %d: %w", msgType, err)
	}

	resp, err := vc.Receive()
	if err != nil {
		return nil, fmt.Errorf("receive response for type %d: %w", msgType, err)
	}

	return resp.Payload, nil
}
