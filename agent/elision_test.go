package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func elisionFixture(t *testing.T) (preparedDefinition, []*Message) {
	t.Helper()
	policy, err := normalizeElisionPolicy(&ElisionPolicy{ContextWindowTokens: 9000})
	if err != nil {
		t.Fatal(err)
	}
	prepared := preparedDefinition{definition: Definition{Elision: policy}, toolSnapshots: []ToolDefinitionSnapshot{{
		Info: &ToolInfo{Name: "read"}, Descriptor: ToolDescriptor{Recovery: ToolRecoveryReadOnly, MutationScope: ToolMutationNone, ResultRecoveryKind: ToolResultRecoveryRead},
	}}}
	raw := []*Message{SystemMessage("Stable instructions"), UserMessage("Keep the exact budget 72519")}
	for range 6 {
		raw = append(raw, AssistantMessage("Read the evidence", []ToolCall{{ID: "reused", Function: FunctionCall{Name: "read", Arguments: `{"path":"evidence.txt"}`}}}),
			ToolMessage(ToolResult{ModelContent: strings.Repeat("evidence", 512), Status: ToolResultSuccess, ResultRetention: ToolResultDeferred,
				ContextHints: &ToolResultContextHints{Recovery: ToolResultRecoveryHint{Kind: ToolResultRecoveryRead, Reference: map[string]any{"path": "evidence.txt"}}}}, "reused"))
	}
	return prepared, raw
}

func prepareFixtureElision(t *testing.T, prepared preparedDefinition, raw []*Message) (elisionRecord, *preparedModelCall, error) {
	t.Helper()
	return prepareElision(context.Background(), prepared, raw, compactionRecord{}, false, compactionValidationSnapshot(raw, 1), func(next elisionRecord) (*preparedModelCall, error) {
		messages, err := next.project(raw)
		return &preparedModelCall{call: &ModelCall{Model: &lifecycleModel{}, Messages: messages, stablePrefixMessages: 1}}, err
	})
}

func TestElisionProtectsRecentContextAndPersistsOnlyCoordinates(t *testing.T) {
	prepared, raw := elisionFixture(t)
	original := cloneMessages(raw)
	state, call, err := prepareFixtureElision(t, prepared, raw)
	if err != nil || call == nil || state.Metrics.ResultsElided == 0 || state.Metrics.TokensBefore <= state.Metrics.TokensAfter {
		t.Fatalf("elision=%+v call=%v err=%v", state, call, err)
	}
	if !reflect.DeepEqual(raw, original) {
		t.Fatal("Elision mutated the canonical journal")
	}
	projected := call.call.Messages
	if !reflect.DeepEqual(projected[:2], raw[:2]) || !reflect.DeepEqual(projected[len(raw)-6:], raw[len(raw)-6:]) {
		t.Fatal("Elision changed instructions or the recent token-budgeted tail")
	}
	for index := range raw {
		if raw[index].Role != ToolRole && !reflect.DeepEqual(raw[index], projected[index]) {
			t.Fatalf("Elision changed non-tool message %d", index)
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil || strings.Contains(string(encoded), "evidence.txt") || strings.Contains(string(encoded), "Earlier tool") {
		t.Fatalf("recovery record copied runtime content: %s %v", encoded, err)
	}
	restored, err := elisionStateFrom(map[string]json.RawMessage{elisionCapability: encoded})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restored.project(raw)
	if err != nil || !reflect.DeepEqual(replayed, projected) {
		t.Fatalf("cold Elision projection differs: %v", err)
	}
	changed := cloneMessages(raw)
	changed[state.Replacements[0].MessageIndex].Content = "replaced by branch history"
	if _, err := restored.project(changed); err == nil {
		t.Fatal("stale Elision coordinates rewrote changed canonical history")
	}
	prepared.elision = restored
	_, second, err := prepareFixtureElision(t, prepared, projected)
	if err == nil && second != nil {
		t.Fatal("already elided source was accepted as canonical history")
	}
}

func TestElisionKeepsUnrecoverableAndUnsettledGroups(t *testing.T) {
	for _, kind := range []string{"error", "unknown_effect", "protected", "failed_spill", "redacted_reference", "truncated_reference", "image", "missing_recovery_tool", "write_replay", "no_hint", "incomplete"} {
		t.Run(kind, func(t *testing.T) {
			prepared, raw := elisionFixture(t)
			for _, message := range raw {
				if message.Role != ToolRole {
					continue
				}
				switch kind {
				case "error":
					message.ToolResult.Status = ToolResultError
				case "unknown_effect":
					message.ToolResult.SyntheticReason = ToolSyntheticEffectUnknown
				case "protected":
					message.ToolResult.ResultRetention = ToolResultProtected
				case "failed_spill":
					message.ToolResult.ArtifactPersistence = &ToolArtifactPersistence{Attempted: true}
				case "redacted_reference":
					message.ToolResult.ContextHints.Recovery.Reference["nested"] = map[string]any{"secret": "[REDACTED]"}
				case "truncated_reference":
					message.ToolResult.ContextHints.Recovery.Reference["path"] = "...[truncated]"
				case "image":
					message.Attachments = []Attachment{{Path: "images/evidence.png"}}
				case "no_hint":
					message.ToolResult.ContextHints = nil
				}
			}
			switch kind {
			case "missing_recovery_tool":
				prepared.toolSnapshots = nil
			case "write_replay":
				prepared.toolSnapshots[0].Descriptor.Recovery = ToolRecoveryNonIdempotent
			case "incomplete":
				// A malformed first batch prevents later data from authorizing elision.
				raw[3].ToolCallID = "missing"
			}
			_, call, err := prepareFixtureElision(t, prepared, raw)
			if err != nil || call != nil {
				t.Fatalf("protected group was changed: call=%v err=%v", call, err)
			}
		})
	}
}

func TestElisionRequiresPressureAndActualProviderSavings(t *testing.T) {
	prepared, raw := elisionFixture(t)
	prepared.definition.Elision.ContextWindowTokens = 100_000
	if _, call, err := prepareFixtureElision(t, prepared, raw); err != nil || call != nil {
		t.Fatalf("below-threshold elision: call=%v err=%v", call, err)
	}
	prepared.definition.Elision.ContextWindowTokens = 9000
	_, call, err := prepareElision(context.Background(), prepared, raw, compactionRecord{}, false, compactionValidationSnapshot(raw, 1), func(elisionRecord) (*preparedModelCall, error) {
		// A host middleware can hide or replace tool bodies. The rebuilt request,
		// rather than raw journal bytes, decides whether there is any benefit.
		return &preparedModelCall{call: &ModelCall{Model: &lifecycleModel{}, Messages: cloneMessages(raw), stablePrefixMessages: 1}}, nil
	})
	if err != nil || call != nil {
		t.Fatalf("no-benefit projection committed: call=%v err=%v", call, err)
	}
}

func TestElisionPlaceholderPreservesArtifactReceiptAndRejectsAuxiliaryFiles(t *testing.T) {
	_, raw := elisionFixture(t)
	message := raw[3]
	message.ToolResult.ContextHints.Recovery = ToolResultRecoveryHint{Kind: ToolResultRecoveryArtifact, ArtifactPath: "artifacts/output.log"}
	message.ToolResult.Artifacts = []ToolArtifactRef{{ReadablePath: "artifacts/output.log", Complete: true, ContentType: "text/plain", Purpose: ToolArtifactPurposeCompleteToolOutput}}
	message.ToolResult.ProtectedReceipt = &ToolResultProtectedReceipt{Outcome: "Created chapter 7 exactly once", SanitizedArguments: `{"chapter":7}`}
	stub := elisionPlaceholder(message)
	if !strings.Contains(stub, "artifacts/output.log") || !strings.Contains(stub, "Created chapter 7 exactly once") || !strings.Contains(stub, "Do not repeat") {
		t.Fatalf("artifact receipt lost: %s", stub)
	}
	message.ToolResult.Artifacts[0].Purpose = ToolArtifactPurposeAttachment
	if stub := elisionPlaceholder(message); stub != "" {
		t.Fatalf("auxiliary attachment authorized Elision: %s", stub)
	}
}

func TestElisionClearFencesLateCommitAndStablePrefixValidation(t *testing.T) {
	prepared, raw := elisionFixture(t)
	state, _, err := prepareFixtureElision(t, prepared, raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(state)
	clear, _ := json.Marshal(ClearState{Revision: 1, ClearedAt: time.Now().UTC()})
	restored, err := elisionStateFrom(map[string]json.RawMessage{elisionCapability: encoded, clearCapability: clear})
	if err != nil || len(restored.Replacements) != 0 {
		t.Fatalf("late pre-Clear Elision became active again: %+v %v", restored, err)
	}
	covered, err := effectiveHistoryMessages(raw, state, compactionRecord{ReplacementTo: len(raw) - 6, Removed: true}, true, 4096)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range covered[:len(raw)-6] {
		if strings.HasPrefix(message.Content, "[Earlier tool output elided;") {
			t.Fatal("a removed Compaction boundary resurrected an absorbed Elision placeholder")
		}
	}
	_, call, err := prepareElision(context.Background(), prepared, raw, compactionRecord{}, false, compactionValidationSnapshot(raw, 1), func(next elisionRecord) (*preparedModelCall, error) {
		messages, err := next.project(raw)
		messages[0] = SystemMessage("Changed stable instructions")
		return &preparedModelCall{call: &ModelCall{Model: &lifecycleModel{}, Messages: messages, stablePrefixMessages: 1}}, err
	})
	if err == nil || call != nil || !strings.Contains(err.Error(), "stable context prefix") {
		t.Fatalf("Elision allowed a changed cache prefix: call=%v err=%v", call, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, call, err = prepareElision(ctx, prepared, raw, compactionRecord{}, false, compactionValidationSnapshot(raw, 1), func(elisionRecord) (*preparedModelCall, error) {
		t.Fatal("cancelled Elision rebuilt the model request")
		return nil, nil
	})
	if err != context.Canceled || call != nil {
		t.Fatalf("cancelled Elision returned a candidate: %v %v", call, err)
	}
}
