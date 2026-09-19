package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"denova/internal/agents/external"
	agentrun "denova/internal/agents/run"
)

type protocolFixture struct {
	client   *Client
	server   net.Conn
	received chan packet
	done     chan struct{}
}

func newProtocolFixture(t *testing.T) *protocolFixture {
	t.Helper()
	connection, server := net.Pipe()
	fixture := &protocolFixture{client: newClient(connection, connection, func() { _ = connection.Close() }), server: server, received: make(chan packet, 32), done: make(chan struct{})}
	go func() {
		defer close(fixture.done)
		defer func() {
			if recovered := recover(); recovered != nil {
				fixture.client.fail(fmt.Errorf("fixture reader panic: %v", recovered))
			}
		}()
		decoder := json.NewDecoder(server)
		for {
			var msg packet
			if err := decoder.Decode(&msg); err != nil {
				return
			}
			select {
			case fixture.received <- msg:
			case <-fixture.client.done:
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = fixture.client.Close()
		_ = server.Close()
		select {
		case <-fixture.done:
		case <-time.After(time.Second):
			t.Error("fixture reader did not stop")
		}
	})
	return fixture
}

func (fixture *protocolFixture) send(t *testing.T, id, method string, params any) {
	t.Helper()
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(fixture.server).Encode(packet{ID: json.RawMessage(id), Method: method, Params: body}); err != nil {
		t.Fatal(err)
	}
}

func (fixture *protocolFixture) receive(t *testing.T) packet {
	t.Helper()
	select {
	case msg := <-fixture.received:
		return msg
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for protocol packet")
		return packet{}
	}
}

type testHost struct {
	calls   atomic.Int32
	started chan struct{}
	answer  chan struct{}
	output  strings.Builder
	events  chan agentrun.Event
}

func (host *testHost) Emit(event agentrun.Event) error {
	if event.Type == "chunk" {
		host.output.WriteString(event.DataString("content"))
	}
	if host.events != nil {
		host.events <- event
	}
	return nil
}
func (host *testHost) CallTool(ctx context.Context, _ external.ToolCall) (external.ToolResult, error) {
	host.calls.Add(1)
	host.started <- struct{}{}
	select {
	case <-ctx.Done():
		return external.ToolResult{}, ctx.Err()
	case <-host.answer:
		return external.ToolResult{Success: true, Text: "Saved answer"}, nil
	}
}

type attemptResult struct {
	result external.Result
	err    error
}

func startFixtureTurn(t *testing.T, ctx context.Context, fixture *protocolFixture, host *testHost) <-chan attemptResult {
	t.Helper()
	sub, release, err := fixture.client.subscribe("thread")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan attemptResult, 1)
	go func() {
		var completed attemptResult
		defer func() {
			release()
			if recovered := recover(); recovered != nil {
				completed.err = fmt.Errorf("fixture turn panic: %v", recovered)
			}
			done <- completed
		}()
		completed.result, completed.err = fixture.client.runTurn(ctx, sub, "thread", "turn", map[string]bool{"ask": true}, host)
	}()
	return done
}

func TestProtocolDuplicateCallsAndFinalRepairDoNotDuplicateWork(t *testing.T) {
	fixture := newProtocolFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	host := &testHost{started: make(chan struct{}, 4), answer: make(chan struct{})}
	done := startFixtureTurn(t, ctx, fixture, host)
	call := map[string]any{"threadId": "thread", "turnId": "turn", "callId": "call-1", "tool": "ask", "arguments": map[string]any{"questions": []any{}}}
	fixture.send(t, `1`, "item/tool/call", call)
	select {
	case <-host.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	fixture.send(t, `2`, "item/tool/call", call)
	// Stream events remain observable while a tool is waiting.
	fixture.send(t, "", "item/agentMessage/delta", map[string]any{"threadId": "thread", "turnId": "turn", "itemId": "message", "delta": "Draft"})
	close(host.answer)
	first, second := fixture.receive(t), fixture.receive(t)
	if string(first.Result) != string(second.Result) || first.Error != nil || second.Error != nil || string(first.ID) == string(second.ID) {
		t.Fatal("duplicate callback did not receive the original result")
	}
	call["callId"] = "call-2"
	fixture.send(t, `3`, "item/tool/call", call)
	if reply := fixture.receive(t); reply.Error != nil {
		t.Fatal(reply.Error)
	}
	for range 2 {
		fixture.send(t, "", "item/completed", map[string]any{"threadId": "thread", "turnId": "turn", "item": map[string]any{"type": "agentMessage", "id": "message", "text": "Draft complete."}})
	}
	fixture.send(t, "", "turn/completed", map[string]any{"threadId": "thread", "turn": map[string]any{"id": "turn", "status": "completed"}})
	select {
	case result := <-done:
		if result.err != nil || result.result.Text != "Draft complete." || host.output.String() != result.result.Text || host.calls.Load() != 2 {
			t.Fatalf("work/output duplicated: %#v, output=%q calls=%d", result, host.output.String(), host.calls.Load())
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestProtocolCancelStopsQuestionAndRejectsLateCallback(t *testing.T) {
	fixture := newProtocolFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host := &testHost{started: make(chan struct{}, 2), answer: make(chan struct{})}
	done := startFixtureTurn(t, ctx, fixture, host)
	call := map[string]any{"threadId": "thread", "turnId": "turn", "callId": "call-1", "tool": "ask", "arguments": map[string]any{}}
	fixture.send(t, `1`, "item/tool/call", call)
	select {
	case <-host.started:
	case <-time.After(time.Second):
		t.Fatal("question did not start")
	}
	cancel()
	select {
	case result := <-done:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("question did not cancel: %v", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled question did not release its worker")
	}
	fixture.send(t, `2`, "item/tool/call", call)
	if reply := fixture.receive(t); reply.Error == nil || host.calls.Load() != 1 {
		t.Fatal("expired callback reached the host")
	}
}

func TestProtocolMalformedInputUnblocksPendingRPC(t *testing.T) {
	fixture := newProtocolFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- fmt.Errorf("fixture RPC panic: %v", recovered)
			}
		}()
		done <- fixture.client.call(ctx, "thread/list", map[string]any{}, nil)
	}()
	_ = fixture.receive(t)
	if _, err := fixture.server.Write([]byte("{broken-json}\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("protocol corruption did not fail the call: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
