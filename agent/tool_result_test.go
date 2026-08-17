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
		MutationScope: ToolMutationNone, PostCheck: ToolPostCheckNone,
		Recovery: ToolRecoveryReadOnly, ResultRecoveryKind: ToolResultRecoveryRead,
		ResultProjection: ToolResultBoundedModelContext,
		ResultRetention:  ToolResultDeferred,
		Steering:         SteeringFinishCurrent, MaxResultBytes: limit,
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
	message := ToolMessage(normalized, "call-1", WithToolName("read"))
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "sha256:after") || strings.Contains(message.Content, display) {
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
		{name: "invalid artifact digest", result: ToolResult{Status: ToolResultSuccess, Artifacts: []ToolArtifactRef{{
			ID: "artifact", ReadablePath: "memory://artifact", ContentType: "text/plain", EstimatedBytes: 1, SHA256: "invalid",
		}}}},
		{name: "invalid artifact purpose", result: ToolResult{Status: ToolResultSuccess, Artifacts: []ToolArtifactRef{{
			ID: "artifact", Purpose: ToolArtifactPurpose("future"), ReadablePath: ".denova/artifacts/item.log",
			ContentType: "text/plain", Complete: true,
		}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeToolResult(test.result, resultTestDescriptor(16)); err == nil {
				t.Fatalf("NormalizeToolResult(%#v) succeeded", test.result)
			}
		})
	}
}

func TestNormalizeToolResultBoundsAndRedactsContextHints(t *testing.T) {
	descriptor := resultTestDescriptor(16 * 1024)
	descriptor.ResultRetention = ToolResultEagerCandidate
	reference := map[string]any{
		"path":          "chapters/one.md",
		"authorization": "Bearer never-persist-this",
		"headers": map[string]any{
			"X-Custom-Auth": "Bearer custom-header-secret",
		},
		"env": map[string]any{
			"MY_CREDENTIAL": "environment-secret",
			"PRIVATE_KEY":   "private-key-secret",
			"ACCESS_KEY":    "access-key-secret",
			"SESSION_KEY":   "session-key-secret",
		},
		"opaque_header": "Bearer scalar-secret",
		"nested": map[string]any{
			"api_token": "also-secret",
			"query":     strings.Repeat("x", toolResultHintMaxStringBytes*2),
		},
	}
	result := TextToolResult("result")
	result.ContextHints = &ToolResultContextHints{
		Recovery: ToolResultRecoveryHint{
			Kind: ToolResultRecoveryRead, Reference: reference,
			EstimatedBytes: 4096, EstimatedTokens: 1024,
		},
		ContextValue:    ToolResultContextDiscardable,
		SupersessionKey: "read:chapters/one.md",
	}

	normalized, err := NormalizeToolResult(result, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ResultRetention != ToolResultEagerCandidate || normalized.ContextHints == nil {
		t.Fatalf("normalized contract = %#v", normalized)
	}
	if got := normalized.ContextHints.Recovery.Reference["authorization"]; got != toolResultHintRedactedValue {
		t.Fatalf("authorization was not redacted: %#v", got)
	}
	nested := normalized.ContextHints.Recovery.Reference["nested"].(map[string]any)
	if got := nested["api_token"]; got != toolResultHintRedactedValue {
		t.Fatalf("api token was not redacted: %#v", got)
	}
	if got := nested["query"].(string); len(got) > toolResultHintMaxStringBytes {
		t.Fatalf("hint string exceeded bound: %d", len(got))
	}
	headers := normalized.ContextHints.Recovery.Reference["headers"].(map[string]any)
	environment := normalized.ContextHints.Recovery.Reference["env"].(map[string]any)
	for key, got := range map[string]any{
		"custom auth": headers["X-Custom-Auth"],
		"credential":  environment["MY_CREDENTIAL"],
		"private key": environment["PRIVATE_KEY"],
		"access key":  environment["ACCESS_KEY"],
		"session key": environment["SESSION_KEY"],
		"bare bearer": normalized.ContextHints.Recovery.Reference["opaque_header"],
	} {
		if got != toolResultHintRedactedValue {
			t.Fatalf("%s was not redacted: %#v", key, got)
		}
	}
	if reference["authorization"] != "Bearer never-persist-this" {
		t.Fatal("normalization mutated the caller's recovery map")
	}
	encoded, err := json.Marshal(normalized.ContextHints)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"never-persist-this", "also-secret", "custom-header-secret", "environment-secret",
		"private-key-secret", "access-key-secret", "session-key-secret", "scalar-secret",
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("unsafe context hints leaked %q: %s", secret, encoded)
		}
	}
	if len(encoded) > toolResultHintMaxBytes {
		t.Fatalf("unsafe context hints: %s", encoded)
	}
}

func TestNormalizeToolResultRejectsUnsafeContextHints(t *testing.T) {
	descriptor := resultTestDescriptor(1024)
	descriptor.ResultRetention = ToolResultDeferred
	tests := []ToolResultContextHints{
		{Recovery: ToolResultRecoveryHint{Kind: ToolResultRecoveryKind("future")}},
		{Recovery: ToolResultRecoveryHint{Kind: ToolResultRecoveryArtifact}},
		{Recovery: ToolResultRecoveryHint{Kind: ToolResultRecoveryRead, EstimatedBytes: -1}},
		{ContextValue: ToolResultContextValue("future")},
	}
	for _, hints := range tests {
		result := TextToolResult("result")
		result.ContextHints = &hints
		if _, err := NormalizeToolResult(result, descriptor); err == nil {
			t.Fatalf("accepted unsafe hints: %#v", hints)
		}
	}
}

func TestNormalizeToolResultMarksTruncatedReferenceCollections(t *testing.T) {
	object := make(map[string]any, toolResultHintMaxCollectionEntries+2)
	items := make([]any, toolResultHintMaxCollectionEntries+2)
	for index := range items {
		object["field_"+string(rune(0x100+index))] = index
		items[index] = index
	}
	result := TextToolResult("result")
	result.ContextHints = &ToolResultContextHints{Recovery: ToolResultRecoveryHint{
		Kind: ToolResultRecoveryRerun,
		Reference: map[string]any{
			"object": object,
			"items":  items,
		},
	}}
	normalized, err := NormalizeToolResult(result, resultTestDescriptor(1024))
	if err != nil {
		t.Fatal(err)
	}
	reference := normalized.ContextHints.Recovery.Reference
	normalizedObject := reference["object"].(map[string]any)
	normalizedItems := reference["items"].([]any)
	if normalizedObject["_truncated"] != toolResultHintTruncatedValue ||
		len(normalizedItems) != toolResultHintMaxCollectionEntries+1 ||
		normalizedItems[len(normalizedItems)-1] != toolResultHintTruncatedValue {
		t.Fatalf("truncation markers = object:%#v items:%#v", normalizedObject, normalizedItems)
	}
}

func TestNormalizeToolResultArtifactContractDoesNotRequireDigest(t *testing.T) {
	result := TextToolResult("preview")
	result.Artifacts = []ToolArtifactRef{{
		ID: "artifact-1", Purpose: ToolArtifactPurposeAttachment,
		ReadablePath: ".denova/artifacts/session/call.log",
		ContentType:  "text/plain; charset=utf-8", EstimatedBytes: 4096,
		EstimatedTokens: 1024, Complete: true,
	}}
	normalized, err := NormalizeToolResult(result, resultTestDescriptor(1024))
	if err != nil {
		t.Fatal(err)
	}
	artifact := normalized.Artifacts[0]
	if artifact.Purpose != ToolArtifactPurposeAttachment || artifact.ReadablePath == "" ||
		artifact.ContentType == "" || artifact.EstimatedBytes != 4096 || artifact.SHA256 != "" {
		t.Fatalf("artifact was not normalized: %#v", artifact)
	}
}

func TestNormalizeToolResultBoundsArtifactMetadata(t *testing.T) {
	valid := ToolArtifactRef{
		ID: "artifact", Purpose: ToolArtifactPurposeAttachment,
		ReadablePath: ".denova/artifacts/session/item.log", ContentType: "text/plain", Complete: true,
	}
	tooMany := TextToolResult("preview")
	tooMany.Artifacts = make([]ToolArtifactRef, maxToolResultArtifacts+1)
	for index := range tooMany.Artifacts {
		tooMany.Artifacts[index] = valid
		tooMany.Artifacts[index].ID += string(rune('a' + index%26))
	}
	if _, err := NormalizeToolResult(tooMany, resultTestDescriptor(1024)); err == nil {
		t.Fatal("unbounded artifact count was accepted")
	}

	oversized := TextToolResult("preview")
	oversized.Artifacts = []ToolArtifactRef{valid}
	oversized.Artifacts[0].ReadablePath = strings.Repeat("p", maxToolResultArtifactMetadataBytes+1)
	if _, err := NormalizeToolResult(oversized, resultTestDescriptor(1024)); err == nil {
		t.Fatal("unbounded artifact metadata was accepted")
	}

	normal := TextToolResult("preview")
	normal.Artifacts = []ToolArtifactRef{valid, {
		ID: "raw-output", Purpose: ToolArtifactPurposeCompleteToolOutput,
		ReadablePath: ".denova/artifacts/session/raw.log", ContentType: "text/plain", Complete: true,
	}}
	normalized, err := NormalizeToolResult(normal, resultTestDescriptor(1024))
	if err != nil || len(normalized.Artifacts) != 2 || normalized.Artifacts[1].Purpose != ToolArtifactPurposeCompleteToolOutput {
		t.Fatalf("normal multi-artifact result failed to round-trip: result=%#v err=%v", normalized, err)
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

func TestToolMessagePersistsNewResultContextContract(t *testing.T) {
	result := TextToolResult("bounded preview")
	result.ResultRetention = ToolResultEagerCandidate
	result.ContextHints = &ToolResultContextHints{
		Recovery: ToolResultRecoveryHint{
			Kind: ToolResultRecoveryArtifact, ArtifactPath: ".denova/artifacts/session/call.log",
			EstimatedBytes: 8192, EstimatedTokens: 2048,
		},
		ContextValue: ToolResultContextDiscardable, SupersessionKey: "read:chapter.md",
	}
	result.Metadata.ArtifactPersistence = &ToolArtifactPersistence{Attempted: true, Complete: true}
	result.Artifacts = []ToolArtifactRef{{
		ID: "artifact", ReadablePath: ".denova/artifacts/session/call.log",
		ContentType: "text/plain", EstimatedBytes: 8192, EstimatedTokens: 2048, Complete: true,
	}}

	message := ToolMessage(result, "call-1", WithToolName("read"))
	result.ContextHints.Recovery.ArtifactPath = "changed"
	result.Metadata.ArtifactPersistence.Complete = false
	restored := message.EffectiveToolResult()
	if restored.ResultRetention != ToolResultEagerCandidate || restored.ContextHints == nil ||
		restored.ContextHints.Recovery.ArtifactPath != ".denova/artifacts/session/call.log" ||
		restored.Metadata.ArtifactPersistence == nil || !restored.Metadata.ArtifactPersistence.Complete ||
		len(restored.Artifacts) != 1 || !restored.Artifacts[0].Complete {
		t.Fatalf("restored result = %#v", restored)
	}
	restored.ContextHints.Recovery.ArtifactPath = "mutated"
	if message.ToolResult.ContextHints.Recovery.ArtifactPath != ".denova/artifacts/session/call.log" {
		t.Fatal("EffectiveToolResult shared context hints with persisted message")
	}
}
