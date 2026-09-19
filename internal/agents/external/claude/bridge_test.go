package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"denova/internal/agents/external"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type authorizedTransport struct{ token string }

func (t authorizedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestBridgeScopesToolsAndCancelsPendingQuestion(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan external.ToolCall, 1)
	host := &testHost{call: func(ctx context.Context, call external.ToolCall) (external.ToolResult, error) {
		started <- call
		<-ctx.Done()
		return external.ToolResult{}, ctx.Err()
	}}
	bridge, err := startBridge(ctx, []external.Tool{{Name: "ask", Description: "Ask", Schema: json.RawMessage(`{"type":"object"}`)}}, host, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	response, err := http.Get(bridge.url)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("unscoped client accessed bridge")
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: bridge.url, HTTPClient: &http.Client{Transport: authorizedTransport{bridge.token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	list, err := session.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 1 || list.Tools[0].Name != "ask" {
		t.Fatalf("tools: %#v %v", list, err)
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "write", Arguments: map[string]any{}}); err == nil {
		t.Fatal("unregistered tool was accepted")
	}
	done := make(chan error, 1)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		defer func() {
			if v := recover(); v != nil {
				done <- fmt.Errorf("test call panic: %v", v)
			}
		}()
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Meta: mcp.Meta{"claudecode/toolUseId": "ask-1"}, Name: "ask", Arguments: map[string]any{"question": "Wait"}})
		done <- err
	}()
	select {
	case call := <-started:
		if call.ID != "ask-1" || call.Name != "ask" {
			t.Fatal("missing scoped call identity")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("question did not arrive")
	}
	select {
	case err := <-done:
		t.Fatalf("question returned before answer or cancellation: %v", err)
	default:
	}
	cancel()
	workers.Wait()
	if err := <-done; err == nil {
		t.Fatal("cancelled question succeeded")
	}
	if err := bridge.close(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
