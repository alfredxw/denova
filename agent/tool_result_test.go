package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func resultTestDescriptor(limit int) ToolDescriptor {
	return ToolDescriptor{
		Source: ToolSourceRead, Execution: ToolExecutionParallelRead,
		Recovery: ToolRecoveryReadOnly, ResultProjection: ToolResultBoundedModelContext,
		Steering: SteeringFinishCurrent, MaxResultBytes: limit,
	}
}

func TestNormalizeToolResultKeepsModelDisplayAndDetailsIsolated(t *testing.T) {
	model := strings.Repeat("模型🙂", 20)
	display := strings.Repeat("界面✨", 20)
	details := json.RawMessage(`{"receipt":{"revision":"sha256:after"}}`)
	result := ToolResult{
		ModelContent: model, DisplayContent: display, Details: details,
		Status:   ToolResultSuccess,
		Metadata: ToolResultMetadata{Target: "chapters/one.md", IdempotencyKey: "call-1"},
	}

	normalized, err := NormalizeToolResult(result, resultTestDescriptor(48))
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.ModelContent) > 48 || len(normalized.DisplayContent) > 48 {
		t.Fatalf("bounded content exceeded limit: model=%d display=%d", len(normalized.ModelContent), len(normalized.DisplayContent))
	}
	if !utf8.ValidString(normalized.ModelContent) || !utf8.ValidString(normalized.DisplayContent) {
		t.Fatalf("truncation broke UTF-8: model=%q display=%q", normalized.ModelContent, normalized.DisplayContent)
	}
	if !normalized.Metadata.ModelTruncated || !normalized.Metadata.DisplayTruncated ||
		normalized.Metadata.OriginalModelBytes != len(model) || normalized.Metadata.OriginalDisplayBytes != len(display) ||
		normalized.Metadata.ReturnedModelBytes != len(normalized.ModelContent) ||
		normalized.Metadata.ReturnedDisplayBytes != len(normalized.DisplayContent) {
		t.Fatalf("unexpected result metadata: %#v", normalized.Metadata)
	}
	if string(normalized.Details) != string(details) || normalized.Metadata.Target != "chapters/one.md" || normalized.Metadata.IdempotencyKey != "call-1" {
		t.Fatalf("details or durable metadata changed: %#v", normalized)
	}
	message := ToolMessage(normalized, "call-1", WithToolName("read_file"))
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "receipt") || strings.Contains(message.Content, display) {
		t.Fatalf("transcript leaked display/details: %s", encoded)
	}
}

func TestNormalizeToolResultRejectsInvalidStructuredFields(t *testing.T) {
	tests := []struct {
		name   string
		result ToolResult
	}{
		{name: "status", result: ToolResult{Status: ToolResultStatus("future")}},
		{name: "synthetic reason", result: ToolResult{Status: ToolResultError, SyntheticReason: ToolSyntheticReason("future")}},
		{name: "success synthetic", result: ToolResult{Status: ToolResultSuccess, SyntheticReason: ToolSyntheticUnknownTool}},
		{name: "invalid details", result: ToolResult{Status: ToolResultSuccess, Details: json.RawMessage(`{"broken"`)}},
		{name: "oversized details", result: ToolResult{Status: ToolResultSuccess, Details: json.RawMessage(`{"value":"0123456789"}`)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeToolResult(test.result, resultTestDescriptor(16)); err == nil {
				t.Fatalf("NormalizeToolResult(%#v) succeeded", test.result)
			}
		})
	}
}

func TestNormalizeToolResultRevalidatesAfterMiddlewareMutation(t *testing.T) {
	first, err := NormalizeToolResult(TextToolResult(strings.Repeat("a", 32)), resultTestDescriptor(8))
	if err != nil {
		t.Fatal(err)
	}
	first.ModelContent = strings.Repeat("b", 64)
	first.DisplayContent = strings.Repeat("c", 64)
	second, err := NormalizeToolResult(first, resultTestDescriptor(16))
	if err != nil {
		t.Fatal(err)
	}
	if len(second.ModelContent) > 16 || len(second.DisplayContent) > 16 ||
		second.Metadata.OriginalModelBytes != 64 || second.Metadata.OriginalDisplayBytes != 64 ||
		!second.Metadata.ModelTruncated || !second.Metadata.DisplayTruncated {
		t.Fatalf("re-normalization was bypassed: %#v", second)
	}
}

func TestMessageEffectiveToolResultReadsLegacyAndStructuredHistory(t *testing.T) {
	legacy := (&Message{Role: ToolRole, Content: "legacy", ToolCallID: "old"}).EffectiveToolResult()
	if legacy.Status != ToolResultSuccess || legacy.ModelContent != "legacy" || legacy.DisplayContent != "legacy" {
		t.Fatalf("legacy result = %#v", legacy)
	}
	structured := (&Message{
		Role: ToolRole, Content: "blocked", ToolCallID: "new",
		ToolResult: &ToolResultSummary{Status: ToolResultBlocked, SyntheticReason: ToolSyntheticPolicyBlocked, ModelTruncated: true},
	}).EffectiveToolResult()
	if structured.Status != ToolResultBlocked || structured.SyntheticReason != ToolSyntheticPolicyBlocked || !structured.Metadata.ModelTruncated {
		t.Fatalf("structured result = %#v", structured)
	}
}
