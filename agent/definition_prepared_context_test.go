package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

func TestPreparedContextRepeatedSuspendPreservesActiveInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
	contextSource := &mutablePreparedContext{body: "Accepted project instruction"}
	definition := Definition{Name: "test", Model: model, Context: contextSource}
	key := NamedSession("repeated-preparation-suspend")
	prepared, err := prepareDefinition(ctx, definition, PrepareRequest{Session: SessionView{Key: key}, Input: Text("original input"), Reason: TurnReasonStart})
	if err != nil {
		t.Fatal(err)
	}
	prepared.definitionOperationID, prepared.definitionCommandID, prepared.definitionCycle = "run", "command", 1
	prepared.preparationStage = enginePreparationMaterialized
	prepared.materializedFingerprint, err = materializedDefinitionFingerprint(prepared)
	if err != nil {
		t.Fatal(err)
	}
	original := UserMessage("original input")
	raw, err := encodeActiveEngineTranscript(prepared, []*Message{original}, original, 0)
	if err != nil {
		t.Fatal(err)
	}
	contextSource.unavailable = true
	var preparationContext context.Context
	engine := &definitionEngine{key: key, cacheKeys: defaultCacheKey, source: SourceFunc(func(ctx context.Context, _ PrepareRequest) (Definition, error) {
		preparationContext = ctx
		return definition, nil
	})}
	snapshot := runstate.TurnSnapshot{OperationID: "run", CommandID: "command", Cycle: 1, Delivery: runstate.DeliveryStart,
		Input: runstate.UserInput{Text: "original input"}, State: raw}
	for attempt := 0; attempt < 2; attempt++ {
		controls := make(chan runstate.EngineControl, 1)
		interrupted := false
		result, err := engine.Run(ctx, runstate.EngineRequest{Snapshot: snapshot, Controls: controls}, func(event runstate.EngineEvent) error {
			if update, ok := event.(runstate.EngineTranscriptUpdated); ok {
				snapshot.State = append(json.RawMessage(nil), update.State...)
				if !interrupted {
					interrupted = true
					controls <- runstate.EngineControl{Kind: runstate.EngineControlSuspend}
					<-preparationContext.Done()
					return preparationContext.Err()
				}
			}
			return nil
		})
		if err != nil || result.Status != runstate.EngineSuspended {
			t.Fatalf("suspend %d: result=%+v err=%v", attempt, result, err)
		}
		state, err := decodeEngineTranscript(snapshot.State)
		if err != nil || state.ActiveModelUser == nil || state.ActiveUserIndex != 0 || !reflect.DeepEqual(state.Messages, []*Message{original}) {
			t.Fatalf("suspend %d lost accepted input boundary: state=%+v err=%v", attempt, state, err)
		}
	}
	result, err := engine.Run(ctx, runstate.EngineRequest{Snapshot: snapshot}, func(runstate.EngineEvent) error { return nil })
	if err != nil || result.Status != runstate.EngineCompleted {
		t.Fatalf("resume: result=%+v err=%v", result, err)
	}
	want := append(leadingContextMessages(prepared.fragments), original)
	if calls := model.calls(); len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("repeated suspension changed model input: %#v", calls)
	}
}

type mutablePreparedContext struct {
	body        string
	unavailable bool
	calls       int
}

func (*mutablePreparedContext) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "test.prepared-context", Version: 1}
}
func (source *mutablePreparedContext) Materialize(context.Context, ContextRequest) ([]ContextFragment, error) {
	source.calls++
	if source.unavailable {
		return nil, errors.New("source unavailable")
	}
	return []ContextFragment{{Source: "project", Purpose: "accepted project instruction", Resource: "AGENTS.md",
		Stability: ContextStablePrefix, Placement: ContextLeadingMessage, Content: source.body, HardLimit: 4096}}, nil
}

func TestPreparedContextResumePreservesContextAndExecutionFences(t *testing.T) {
	for _, scenario := range []string{"changed_context", "unavailable_context", "tool_schema", "tool_descriptor", "tool_removed", "behavior", "legacy_unchanged", "legacy_changed", "invalid_version", "invalid_bound", "tampered_context"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			source := &mutablePreparedContext{body: "Original project instruction"}
			model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
			tool, err := InferTool("inspect", "Inspect a chapter", func(context.Context, struct{}) (string, error) {
				t.Error("resume executed an unrequested tool")
				return "", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			tools := &permissionContractTools{definitions: []ToolDefinition{testToolDefinition(tool)}}
			definition := Definition{Name: "test", Model: model, Context: source, Tools: tools}
			key := NamedSession("restore-context")
			request := PrepareRequest{Session: SessionView{Key: key}, Run: RunView{ID: "run", CommandID: "command", Cycle: 1}, Input: Text("original input"), Reason: TurnReasonStart}
			prepared, err := prepareDefinition(ctx, definition, request)
			if err != nil {
				t.Fatal(err)
			}
			prepared.definitionOperationID, prepared.definitionCommandID, prepared.definitionCycle = "run", "command", 1
			prepared.preparationStage = enginePreparationMaterialized
			prepared.materializedFingerprint, err = materializedDefinitionFingerprint(prepared)
			if err != nil {
				t.Fatal(err)
			}
			original := UserMessage("original input")
			raw, err := encodeActiveEngineTranscript(prepared, []*Message{original}, original, 0)
			if err != nil {
				t.Fatal(err)
			}
			state, err := decodeEngineTranscript(raw)
			if err != nil {
				t.Fatal(err)
			}
			wantError := true
			switch scenario {
			case "changed_context":
				source.body, wantError = "Changed project instruction", false
			case "unavailable_context":
				source.unavailable, wantError = true, false
			case "tool_schema":
				changed, err := InferTool("inspect", "Inspect a chapter", func(context.Context, struct {
					Path string `json:"path"`
				}) (string, error) {
					return "", nil
				})
				if err != nil {
					t.Fatal(err)
				}
				tools.definitions[0].Tool = changed
			case "tool_descriptor":
				tools.definitions[0].Descriptor.MaxResultBytes++
			case "tool_removed":
				tools.definitions = nil
			case "behavior":
				definition.Instructions = "Changed executable behavior"
			case "legacy_unchanged":
				state.PreparedContext, wantError = nil, false
			case "legacy_changed":
				state.PreparedContext, source.body = nil, "Changed project instruction"
			case "invalid_version":
				state.PreparedContext.Version++
			case "invalid_bound":
				state.PreparedContext.Fragments[0].HardLimit = 1
			case "tampered_context":
				state.PreparedContext.Fragments[0].Content = "Unaccepted context"
			}
			raw, err = json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			source.calls = 0
			engine := &definitionEngine{source: definition, key: key, cacheKeys: defaultCacheKey}
			var checkpoints []engineTranscript
			result, err := engine.Run(ctx, runstate.EngineRequest{Snapshot: runstate.TurnSnapshot{
				OperationID: "run", CommandID: "command", Cycle: 1, Delivery: runstate.DeliveryStart,
				Input: runstate.UserInput{Text: "original input"}, State: raw,
			}}, func(event runstate.EngineEvent) error {
				if update, ok := event.(runstate.EngineTranscriptUpdated); ok {
					decoded, decodeErr := decodeEngineTranscript(update.State)
					if decodeErr != nil {
						return decodeErr
					}
					checkpoints = append(checkpoints, decoded)
				}
				return nil
			})
			if wantError {
				if err == nil || len(model.calls()) != 0 {
					t.Fatalf("changed contract reached model: result=%+v err=%v", result, err)
				}
				if len(checkpoints) != 0 {
					t.Fatal("failed resume replaced the last valid recovery checkpoint")
				}
				return
			}
			if err != nil || result.Status != runstate.EngineCompleted {
				t.Fatalf("resume=%+v error=%v", result, err)
			}
			calls := model.calls()
			expected := append(leadingContextMessages(prepared.fragments), original)
			if len(calls) != 1 || !reflect.DeepEqual(calls[0], expected) {
				t.Fatalf("resume changed accepted context: %#v", calls)
			}
			wantReads := 0
			if scenario == "legacy_unchanged" {
				wantReads = 1
			}
			if source.calls != wantReads {
				t.Fatalf("source reads=%d, want %d", source.calls, wantReads)
			}
			if len(checkpoints) == 0 || checkpoints[0].PreparedContext == nil {
				t.Fatal("successful resume omitted prepared context")
			}
		})
	}
}

func TestPreparedContextRematerializationUpdatesRecoveryIdentity(t *testing.T) {
	source := &mutablePreparedContext{body: "Original instruction"}
	request := PrepareRequest{Session: SessionView{Key: NamedSession("refresh")}, Input: Text("work"), Reason: TurnReasonStart}
	prepared, err := prepareDefinition(t.Context(), Definition{Name: "test", Model: &lifecycleModel{}, Context: source}, request)
	if err != nil {
		t.Fatal(err)
	}
	prepared.preparationStage = enginePreparationMaterialized
	prepared.materializedFingerprint, err = materializedDefinitionFingerprint(prepared)
	if err != nil {
		t.Fatal(err)
	}
	previous := prepared.materializedFingerprint
	source.body = "Accepted post-compaction instruction"
	if err := rematerializeDefinitionContext(t.Context(), request, &prepared); err != nil {
		t.Fatal(err)
	}
	raw, err := encodeEngineTranscript(prepared, nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := decodeEngineTranscript(raw)
	if err != nil {
		t.Fatal(err)
	}
	if previous == state.MaterializedFingerprint || state.PreparedContext == nil || !strings.Contains(state.PreparedContext.Fragments[0].Content, "post-compaction") {
		t.Fatalf("context refresh did not update recovery baseline: %+v", state)
	}
	restored := prepared
	source.unavailable = true
	engine := &definitionEngine{}
	if err := engine.materializeCycleCapabilities(t.Context(), request, runstate.TurnSnapshot{}, state.PreparedContext, &restored); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := materializedDefinitionFingerprint(restored)
	if err != nil || fingerprint != state.MaterializedFingerprint {
		t.Fatalf("refreshed context could not restore: %s %v", fingerprint, err)
	}
}
