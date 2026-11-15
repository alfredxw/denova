package antigravity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func TestBridgeReplayIdentityBindsPayload(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	host := &testHost{}
	bridge, err := startBridge(ctx, []external.Tool{
		{Name: "record", Schema: json.RawMessage(`{"type":"object"}`)},
		{Name: "other", Schema: json.RawMessage(`{"type":"object"}`)},
	}, host, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	close(bridge.ready)
	client := &http.Client{Timeout: 2 * time.Second}
	sessionID := ""
	post := func(body string, authorized bool) (string, int) {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, bridge.url, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("MCP-Protocol-Version", "2025-03-26")
		if sessionID != "" {
			request.Header.Set("Mcp-Session-Id", sessionID)
		}
		if authorized {
			request.Header.Set("Authorization", "Bearer "+bridge.token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if id := response.Header.Get("Mcp-Session-Id"); id != "" {
			sessionID = id
		}
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return string(data), response.StatusCode
	}
	if _, status := post(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`, false); status != http.StatusUnauthorized {
		t.Fatal("unauthorized client reached the host bridge")
	}
	if body, status := post(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`, true); status != http.StatusOK || sessionID == "" {
		t.Fatalf("initialize: %d %s", status, body)
	}
	post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`, true)
	request := `{"jsonrpc":"2.0","id":"same","method":"tools/call","params":{"name":"record","arguments":{"value":"one"}}}`
	for range 2 {
		body, status := post(request, true)
		if status != http.StatusOK || !strings.Contains(body, "Committed product outcome") {
			t.Fatalf("tool result: %d %s", status, body)
		}
	}
	for _, changed := range []string{
		`{"jsonrpc":"2.0","id":"same","method":"tools/call","params":{"name":"record","arguments":{"value":"two"}}}`,
		`{"jsonrpc":"2.0","id":"same","method":"tools/call","params":{"name":"other","arguments":{"value":"one"}}}`,
	} {
		body, _ := post(changed, true)
		if !strings.Contains(body, "different payload") {
			t.Fatalf("request identity collision accepted: %s", body)
		}
	}
	if len(host.calls) != 1 {
		t.Fatalf("replayed deliveries repeated effects: %#v", host.calls)
	}
}

// A product tool that returns an image must reach the CLI as MCP image content
// rather than failing the whole turn.
func TestBridgeReturnsImageToolResults(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	attachment := referenceAttachment(t)
	host := &testHost{onCall: func(context.Context, external.ToolCall) (external.ToolResult, error) {
		return external.ToolResult{Text: "Generated", Success: true, Images: []agentschema.Attachment{attachment}}, nil
	}}
	bridge, err := startBridge(ctx, []external.Tool{{Name: "record", Schema: json.RawMessage(`{"type":"object"}`)}}, host, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	close(bridge.ready)
	client := &http.Client{Timeout: 2 * time.Second}
	sessionID := ""
	post := func(body string) string {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, bridge.url, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("MCP-Protocol-Version", "2025-03-26")
		request.Header.Set("Authorization", "Bearer "+bridge.token)
		if sessionID != "" {
			request.Header.Set("Mcp-Session-Id", sessionID)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if id := response.Header.Get("Mcp-Session-Id"); id != "" {
			sessionID = id
		}
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if body := post(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`); sessionID == "" {
		t.Fatalf("initialize did not establish a session: %s", body)
	}
	post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	body := post(`{"jsonrpc":"2.0","id":"img","method":"tools/call","params":{"name":"record","arguments":{}}}`)
	if !strings.Contains(body, `"type":"image"`) || !strings.Contains(body, `"mimeType":"image/png"`) {
		t.Fatalf("image tool result was not mirrored: %s", body)
	}
}
