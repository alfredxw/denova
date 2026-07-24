package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageLegacyWireGolden(t *testing.T) {
	var message Message
	legacy := `{"role":"user","content":"legacy"}`
	if err := json.Unmarshal([]byte(legacy), &message); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(&message)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != legacy {
		t.Fatalf("wire changed:\n got %s\nwant %s", encoded, legacy)
	}
}

func TestMessageFullWireRoundTripAndClone(t *testing.T) {
	index := 1
	message := &Message{
		Role:                     Assistant,
		Content:                  "answer",
		MultiContent:             []json.RawMessage{json.RawMessage(`{"type":"text","text":"old"}`)},
		UserInputMultiContent:    []json.RawMessage{json.RawMessage(`{"type":"image_url","vendor":{"x":1}}`)},
		AssistantGenMultiContent: []json.RawMessage{json.RawMessage(`{"type":"audio_url","unknown":[1,2]}`)},
		Name:                     "writer",
		ToolCalls: []ToolCall{{
			Index: &index,
			ID:    "call-1",
			Type:  "function",
			Function: FunctionCall{
				Name:      "lookup",
				Arguments: `{"q":"x"}`,
			},
			Extra: map[string]any{"provider": "p"},
		}},
		ToolCallID:       "parent-call",
		ToolName:         "lookup",
		ReasoningContent: "reason",
		ResponseMeta: &ResponseMeta{
			FinishReason: "tool_calls",
			Usage: &TokenUsage{
				PromptTokens:       10,
				PromptTokenDetails: PromptTokenDetails{CachedTokens: 2},
				CompletionTokens:   3,
				TotalTokens:        13,
				CompletionTokensDetails: CompletionTokensDetails{
					ReasoningTokens: 1,
				},
			},
			LogProbs: &LogProbs{Content: []LogProb{{
				Token: "a", LogProb: -0.1, Bytes: []int64{97},
				TopLogProbs: []TopLogProb{{Token: "a", LogProb: -0.1}},
			}}},
		},
		Extra: map[string]any{"nested": map[string]any{"items": []any{"a", "b"}}},
	}

	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"role":"assistant"`, `"content":"answer"`, `"multi_content"`,
		`"user_input_multi_content"`, `"assistant_output_multi_content"`,
		`"tool_calls"`, `"response_meta"`, `"reasoning_content"`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("full wire omitted %s: %s", field, encoded)
		}
	}
	var decoded Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	reencoded, err := json.Marshal(&decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(reencoded) {
		t.Fatalf("round trip changed wire:\nfirst  %s\nsecond %s", encoded, reencoded)
	}

	clone := message.Clone()
	clone.MultiContent[0][0] = '['
	clone.ToolCalls[0].Function.Name = "changed"
	clone.ToolCalls[0].Extra["provider"] = "changed"
	clone.Extra["nested"].(map[string]any)["items"].([]any)[0] = "changed"
	clone.ResponseMeta.Usage.TotalTokens = 99
	if message.MultiContent[0][0] == '[' || message.ToolCalls[0].Function.Name != "lookup" ||
		message.ToolCalls[0].Extra["provider"] != "p" ||
		message.Extra["nested"].(map[string]any)["items"].([]any)[0] != "a" ||
		message.ResponseMeta.Usage.TotalTokens != 13 {
		t.Fatal("Clone shared mutable storage with the source")
	}
}

func TestConcatMessagesInterleavedToolCallsAndUsageTail(t *testing.T) {
	zero, one := 0, 1
	chunks := []*Message{
		{
			Role: Assistant, Content: "hel", ReasoningContent: "rea",
			ToolCalls: []ToolCall{
				{Index: &one, ID: "call-b", Type: "function", Function: FunctionCall{Name: "beta", Arguments: `{"b":`}, Extra: map[string]any{"trace": "x"}},
				{Index: &zero, ID: "call-a", Type: "function", Function: FunctionCall{Name: "alpha", Arguments: `{"a":`}, Extra: map[string]any{"nested": map[string]any{"n": 1}}},
			},
		},
		{
			Content: "lo", ReasoningContent: "son",
			ToolCalls: []ToolCall{
				{Index: &zero, Function: FunctionCall{Arguments: `1}`}, Extra: map[string]any{"nested": map[string]any{"m": true}}},
				{Index: &one, Function: FunctionCall{Arguments: `2}`}, Extra: map[string]any{"trace": "y"}},
			},
		},
		{
			ResponseMeta: &ResponseMeta{
				FinishReason: "tool_calls",
				Usage: &TokenUsage{
					PromptTokens:       20,
					PromptTokenDetails: PromptTokenDetails{CachedTokens: 5},
					CompletionTokens:   7,
					TotalTokens:        27,
					CompletionTokensDetails: CompletionTokensDetails{
						ReasoningTokens: 3,
					},
				},
			},
		},
	}
	message, err := ConcatMessages(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "hello" || message.ReasoningContent != "reason" {
		t.Fatalf("unexpected concatenation: %#v", message)
	}
	if len(message.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls", len(message.ToolCalls))
	}
	if *message.ToolCalls[0].Index != 0 || message.ToolCalls[0].Function.Arguments != `{"a":1}` {
		t.Fatalf("call 0 not source merged: %#v", message.ToolCalls[0])
	}
	if *message.ToolCalls[1].Index != 1 || message.ToolCalls[1].Function.Arguments != `{"b":2}` {
		t.Fatalf("call 1 not source merged: %#v", message.ToolCalls[1])
	}
	if message.ToolCalls[1].Type != "function" || message.ToolCalls[1].Extra["trace"] != "xy" {
		t.Fatalf("tool type/extra lost: %#v", message.ToolCalls[1])
	}
	if message.ResponseMeta == nil || message.ResponseMeta.Usage == nil ||
		message.ResponseMeta.FinishReason != "tool_calls" || message.ResponseMeta.Usage.TotalTokens != 27 {
		t.Fatalf("usage-only tail lost: %#v", message.ResponseMeta)
	}
}

func TestConcatMessagesRejectsConflictsWithoutMutatingAssembler(t *testing.T) {
	tests := []struct {
		name   string
		first  ToolCall
		second ToolCall
	}{
		{name: "id", first: ToolCall{ID: "a"}, second: ToolCall{ID: "b"}},
		{name: "type", first: ToolCall{Type: "function"}, second: ToolCall{Type: "other"}},
		{name: "name", first: ToolCall{Function: FunctionCall{Name: "a"}}, second: ToolCall{Function: FunctionCall{Name: "b"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index := 0
			test.first.Index = &index
			test.second.Index = &index
			assembler := NewMessageAssembler()
			if err := assembler.Append(&Message{Role: Assistant, Content: "ok", ToolCalls: []ToolCall{test.first}}); err != nil {
				t.Fatal(err)
			}
			if err := assembler.Append(&Message{ToolCalls: []ToolCall{test.second}}); err == nil {
				t.Fatal("expected conflict")
			}
			message, err := assembler.Message()
			if err != nil {
				t.Fatal(err)
			}
			if message.Content != "ok" || len(message.ToolCalls) != 1 {
				t.Fatalf("failed append mutated assembler: %#v", message)
			}
		})
	}

	if _, err := ConcatMessages([]*Message{{Role: User}, {Role: Assistant}}); err == nil {
		t.Fatal("expected role conflict")
	}
}
