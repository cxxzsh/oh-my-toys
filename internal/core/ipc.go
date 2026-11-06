package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// ErrNoApp means no desktop app is listening, so the caller should run the
// switch tool itself.
var ErrNoApp = errors.New("desktop app is not running")

// dialSwitch is how a command finds the desktop app. Tests replace it.
var dialSwitch = SwitchDial

const dialTimeout = 200 * time.Millisecond

type ipcRequest struct {
	Tool   string `json:"tool"`
	Action string `json:"action"`
}

type ipcResponse struct {
	On     bool   `json:"on"`
	Text   string `json:"text"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ServeSwitch answers start, stop and status for the switch tools in reg
// until ctx is cancelled. Each connection carries one request.
func ServeSwitch(ctx context.Context, reg *Registry, ln net.Listener) error {
	defer func() { _ = ln.Close() }()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept ipc: %w", err)
		}
		replySwitch(ctx, reg, conn)
	}
}

func replySwitch(ctx context.Context, reg *Registry, conn net.Conn) {
	defer func() { _ = conn.Close() }()
	var req ipcRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		writeIPC(conn, ipcResponse{Error: err.Error()})
		return
	}
	writeIPC(conn, switchResult(ctx, reg, req))
}

func switchResult(ctx context.Context, reg *Registry, req ipcRequest) ipcResponse {
	t := reg.Get(req.Tool)
	sw, ok := t.(tool.Switch)
	if !ok {
		return ipcResponse{Error: fmt.Sprintf("no switch tool %q", req.Tool)}
	}
	if err := runSwitch(ctx, sw, req.Action); err != nil {
		return ipcResponse{Error: err.Error()}
	}
	st := sw.Status()
	return ipcResponse{On: st.On, Text: st.Text, Detail: st.Detail}
}

func runSwitch(ctx context.Context, sw tool.Switch, action string) error {
	switch action {
	case tool.ActionStart:
		return sw.Start(ctx)
	case tool.ActionStop:
		return sw.Stop()
	case tool.ActionStatus:
		return nil
	default:
		return fmt.Errorf("unknown action %q", action)
	}
}

// CallSwitch asks the desktop app at network/address to run a switch action.
// A dial failure is ErrNoApp.
func CallSwitch(ctx context.Context, network, address, toolID, action string) (tool.Status, error) {
	conn, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, network, address)
	if err != nil {
		return tool.Status{}, fmt.Errorf("%w: %w", ErrNoApp, err)
	}
	defer func() { _ = conn.Close() }()
	return callSwitch(conn, toolID, action)
}

func callSwitch(conn net.Conn, toolID, action string) (tool.Status, error) {
	if err := json.NewEncoder(conn).Encode(ipcRequest{Tool: toolID, Action: action}); err != nil {
		return tool.Status{}, err
	}
	var resp ipcResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return tool.Status{}, err
	}
	if resp.Error != "" {
		return tool.Status{}, errors.New(resp.Error)
	}
	return tool.Status{On: resp.On, Text: resp.Text, Detail: resp.Detail}, nil
}

func writeIPC(conn net.Conn, resp ipcResponse) {
	_ = json.NewEncoder(conn).Encode(resp)
}
