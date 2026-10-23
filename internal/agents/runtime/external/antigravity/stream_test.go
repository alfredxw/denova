package antigravity

import (
	"fmt"
	"testing"

	"denova/internal/agents/runtime/external"
)

func TestStreamTerminalReconciliationAndProtocolIdentity(t *testing.T) {
	host := &testHost{}
	// Provider tool names change between CLI releases; an unlisted name is
	// reported but must not abort the turn.
	private := streamOutput{allowed: map[string]bool{"call_mcp_tool": true}}
	if err := private.feed([]byte(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["ask_custom_permission","browser_click_element","run_command","write_to_file","call_mcp_tool"]}}`), host); err != nil {
		t.Fatalf("provider tools were refused: %v", err)
	}
	output := streamOutput{allowed: map[string]bool{}}
	for _, line := range []string{
		`{"event":"init","conversation_id":"conversation-1","init":{"tools":[]}}`,
		`{"event":"step_update","step_update":{"conversation_id":"conversation-1","step_index":1,"state":"ACTIVE","step_type":"thinking","text_delta":"Private reasoning"}}`,
		`{"event":"step_update","step_update":{"conversation_id":"conversation-1","step_index":2,"state":"DONE","step_type":"agent_response","text_delta":"Answer"}}`,
		`{"event":"result","result":{"conversation_id":"conversation-1","status":"SUCCESS","response":"Answer","usage":{"input_tokens":11,"output_tokens":5,"thinking_tokens":3,"cache_read_tokens":7,"total_tokens":16}}}`,
	} {
		if err := output.feed([]byte(line), host); err != nil {
			t.Fatal(err)
		}
	}
	result := output.complete(nil)
	if result.Text != "Answer" || len(host.events) != 1 || result.Usage.PromptTokens != 11 || result.Usage.PromptTokenDetails.CachedTokens != 7 || result.Usage.CompletionTokensDetails.ReasoningTokens != 3 {
		t.Fatalf("reconciled result: %#v events=%#v", result, host.events)
	}
	for _, line := range []string{
		`{"type":"control_response","response":{"subtype":"success"}}`,
		`{"event":"step_update","step_update":{"conversation_id":"uninitialized","step_type":"agent_response","state":"DONE","text_delta":"Unscoped"}}`,
		`{"event":"result","result":{"status":"SUCCESS","response":"Uninitialized"}}`,

	} {
		stream := streamOutput{allowed: map[string]bool{}}
		if err := stream.feed([]byte(line), &testHost{}); err == nil {
			t.Fatalf("unexpected protocol accepted: %s", line)
		}
	}
	missing := streamOutput{allowed: map[string]bool{"call_mcp_tool": true}}
	if err := missing.feed([]byte(`{"event":"init","conversation_id":"conversation-1","init":{"tools":[]}}`), &testHost{}); err == nil {
		t.Fatal("missing host dispatcher accepted")
	}
}

func TestStreamToleratesStepTerminalStates(t *testing.T) {
	host := &testHost{}
	output := streamOutput{allowed: map[string]bool{}}
	if err := output.feed([]byte(`{"event":"init","conversation_id":"conversation-1","init":{"tools":[]}}`), host); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"ERROR", "CANCELED", "INTERRUPTED"} {
		line := fmt.Sprintf(`{"event":"step_update","step_update":{"conversation_id":"conversation-1","step_index":1,"state":%q,"step_type":"agent_response","text_delta":"Partial answer"}}`, state)
		if err := output.feed([]byte(line), host); err != nil {
			t.Fatalf("step state %s must not abort the attempt: %v", state, err)
		}
	}
	// An unknown step state stays a protocol error, never a silent skip.
	unknown := `{"event":"step_update","step_update":{"conversation_id":"conversation-1","step_index":1,"state":"BOGUS","step_type":"agent_response","text_delta":""}}`
	if err := output.feed([]byte(unknown), host); err == nil {
		t.Fatal("unknown step state accepted")
	}
}

func TestSummarizeUsesFreshConversationWithoutTools(t *testing.T) {
	client := fixtureClient(t, "summary")
	input := fixtureInput(t)
	input.Mode, input.SessionID = external.OperationSummarize, "old-session"
	input.History = []external.Message{{Role: "assistant", Text: "Canonical history"}}
	host := &testHost{}
	result, err := client.Run(t.Context(), input, host)
	if err != nil || !result.Settled || result.Text != "Task completed." || len(host.calls) != 0 {
		t.Fatalf("summary result: %#v calls=%#v err=%v", result, host.calls, err)
	}
}
