package claude

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"denova/internal/agents/runtime/external"
)

func TestNativeElicitationControlRoundTrip(t *testing.T) {
	host := &testHost{call: func(_ context.Context, call external.ToolCall) (external.ToolResult, error) {
		if call.Name != "ask" || !strings.Contains(string(call.Arguments), "Include references") {
			t.Fatalf("elicitation did not reach the product question: %+v", call)
		}
		return external.ToolResult{Success: true, Text: `{"status":"answered","answers":[{"question_id":"question_0","custom_input":"true"}]}`}, nil
	}}
	run := streamRun{host: host, input: external.Input{Tools: []external.Tool{{Name: "ask"}}}}
	// This is the Claude control envelope, not the camelCase MCP wire format.
	var frame streamFrame
	if err := json.Unmarshal([]byte(`{"type":"control_request","request_id":"form-1","request":{"subtype":"elicitation","mcp_server_name":"research","message":"Research options","requested_schema":{"type":"object","properties":{"references":{"type":"boolean","title":"Include references"}},"required":["references"]}}}`), &frame); err != nil {
		t.Fatal(err)
	}
	body, err := run.control(t.Context(), frame)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"type":"control_response","response":{"subtype":"success","request_id":"form-1","response":{"action":"accept","content":{"references":true}}}}`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || len(host.calls) != 1 {
		t.Fatalf("elicitation response=%s calls=%d", body, len(host.calls))
	}
}
