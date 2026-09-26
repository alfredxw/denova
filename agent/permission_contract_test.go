package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type permissionContractTools struct{ definitions []ToolDefinition }

func (*permissionContractTools) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "test.permission-contract-tools", Version: 1}
}

func (tools *permissionContractTools) PrepareTools(context.Context, ToolRequest) ([]ToolDefinition, error) {
	return append([]ToolDefinition(nil), tools.definitions...), nil
}

type permissionContractContext struct{ unavailable bool }

func (*permissionContractContext) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "test.permission-contract-context", Version: 1}
}

func (source *permissionContractContext) Materialize(context.Context, ContextRequest) ([]ContextFragment, error) {
	if source.unavailable {
		return nil, errors.New("referenced context is no longer readable")
	}
	return []ContextFragment{{Source: "test", Purpose: "referenced document", Resource: "chapter.md", Revision: "1",
		Stability: ContextTurn, Placement: ContextFinalUserPrefix, Content: "Original chapter", HardLimit: 1024}}, nil
}

func TestPermissionContractPreservesAuthorizationFences(t *testing.T) {
	for _, scenario := range []string{"context_unavailable", "schema_changed", "descriptor_changed", "tool_removed", "policy_changed", "legacy_context_changed", "legacy_unchanged"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			tool, err := InferTool("inspect", "Inspect a chapter", func(context.Context, struct{}) (string, error) {
				return "inspected", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			tools := &permissionContractTools{definitions: []ToolDefinition{testToolDefinition(tool)}}
			contextSource := &permissionContractContext{}
			policy := &permissionResolutionInvariantPolicy{decision: PermissionResolvedDecision{Allowed: true}}
			definition := Definition{Key: "permission-contract", Name: "test", Model: &lifecycleModel{},
				Tools: tools, Context: contextSource, Permission: policy}
			key := NamedSession("permission-contract")
			prepared, err := prepareDefinition(ctx, definition, PrepareRequest{
				Session: SessionView{Key: key}, Run: RunView{ID: "run", CommandID: "command", Cycle: 1},
				Input: Text("inspect"), Reason: TurnReasonInteraction,
			})
			if err != nil {
				t.Fatal(err)
			}
			prepared.materializedFingerprint, err = materializedDefinitionFingerprint(prepared)
			if err != nil {
				t.Fatal(err)
			}
			prepared.preparationStage = enginePreparationMaterialized
			state, err := encodeEngineTranscript(prepared, nil)
			if err != nil {
				t.Fatal(err)
			}
			presentation := permissionPresentation(PermissionRequest{Tool: "inspect", CallID: "call", Arguments: json.RawMessage(`{}`)},
				PermissionDecision{Reason: LocalizedText{Chinese: "需要确认", English: "Approval required"}})
			presentation.ToolDefinitionHash, err = hashCanonical(prepared.toolSnapshots[0])
			if err != nil {
				t.Fatal(err)
			}
			wantError := true
			switch scenario {
			case "context_unavailable":
				contextSource.unavailable, wantError = true, false
			case "schema_changed":
				changed, err := InferTool("inspect", "Inspect a chapter", func(context.Context, struct {
					Path string `json:"path"`
				}) (string, error) {
					return "changed", nil
				})
				if err != nil {
					t.Fatal(err)
				}
				tools.definitions[0].Tool = changed
			case "descriptor_changed":
				tools.definitions[0].Descriptor.MaxResultBytes++
			case "tool_removed":
				tools.definitions = nil
			case "policy_changed":
				definition.Permission = safeDefaultPermissionPolicy{}
			case "legacy_context_changed":
				presentation.ToolDefinitionHash = ""
				contextSource.unavailable = true
			case "legacy_unchanged":
				presentation.ToolDefinitionHash, wantError = "", false
			}
			if strings.HasPrefix(scenario, "legacy_") {
				var legacy engineTranscript
				if err := json.Unmarshal(state, &legacy); err != nil {
					t.Fatal(err)
				}
				legacy.PreparedContext = nil
				state, err = json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
			}
			encodedRequest, err := json.Marshal(InteractionRequest{ID: "permission-call", Kind: InteractionPermission, Permission: &presentation})
			if err != nil {
				t.Fatal(err)
			}
			engine := &definitionEngine{source: definition, key: key}
			encoded, err := engine.ResolveInteraction(ctx, runstate.InteractionResolveRequest{
				Snapshot: runstate.TurnSnapshot{CommandID: "command", OperationID: "run", Cycle: 1,
					Input: runstate.UserInput{Text: "inspect"}, State: state},
				Interaction: runstate.InteractionSnapshot{ID: "permission-call", OperationID: "run", Cycle: 1, ToolCallID: "call", Request: encodedRequest},
				Response:    json.RawMessage(`{"permission":"allow_once"}`),
			})
			if wantError {
				if err == nil || scenario != "legacy_context_changed" && !errors.Is(err, ErrDefinitionMismatch) {
					t.Fatalf("error=%v, want authorization rejection", err)
				}
				if policy.resolve.Load() != 0 {
					t.Fatal("changed authorization reached PermissionPolicy.Resolve")
				}
				return
			}
			var resolution InteractionResolution
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &resolution); err != nil || resolution.Permission != PermissionAllowOnce || policy.resolve.Load() != 1 {
				t.Fatalf("resolution=%#v error=%v policy calls=%d", resolution, err, policy.resolve.Load())
			}
		})
	}
}

func TestPermissionContractSurvivesJournalReopen(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	store, err := sessionfile.New(root)
	if err != nil {
		t.Fatal(err)
	}
	tool := testToolDefinition(&functionTool{name: "inspect", run: func(context.Context, string) (string, error) {
		t.Error("answering a suspended approval executed a tool")
		return "unexpected", nil
	}})
	tool.Descriptor.Source = ToolSourceShell
	contextSource := &permissionContractContext{}
	model := &lifecycleModel{responses: []*Message{AssistantMessage("", []ToolCall{{
		ID: "call", Type: "function", Function: FunctionCall{Name: "inspect", Arguments: `{}`},
	}})}}
	definition := Definition{Name: "test", Model: model, Tools: mustStaticTools(t, tool), Context: contextSource}
	owner, err := New(ctx, definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	key := NamedSession("permission-reopen")
	sess, err := owner.Session(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	run, err := sess.Run(ctx, Text("inspect"))
	if err != nil {
		t.Fatal(err)
	}
	id := waitPermissionInteractionID(t, run)
	if _, err := sess.SuspendAndClose(ctx, SuspendRequest{RunID: run.ID(), IdempotencyKey: "pause"}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	store, err = sessionfile.New(root)
	if err != nil {
		t.Fatal(err)
	}
	contextSource.unavailable = true
	owner, err = New(ctx, definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	sess, err = owner.Session(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		request, resolution, err := sess.Respond(ctx, id, InteractionResponse{Permission: PermissionAllowOnce})
		if err != nil || resolution.Permission != PermissionAllowOnce || request.Permission == nil || len(request.Permission.ToolDefinitionHash) != 64 {
			t.Fatalf("request=%#v resolution=%#v error=%v", request, resolution, err)
		}
	}
	if len(model.calls()) != 1 {
		t.Fatal("approval recovery invoked the model")
	}
}

type permissionArgumentRewrite struct{ BaseMiddleware }

func (*permissionArgumentRewrite) WrapToolCall(_ context.Context, next ToolCallEndpoint, _ *ToolContext) (ToolCallEndpoint, error) {
	return func(ctx context.Context, _ string, options ...ToolOption) (ToolResult, error) {
		return next(ctx, `{"value":"changed"}`, options...)
	}, nil
}

func TestPermissionContractDoesNotAuthorizeRewrittenArguments(t *testing.T) {
	policy := &permissionResolutionInvariantPolicy{decision: PermissionResolvedDecision{Allowed: true}}
	run, executions := startPermissionResolutionInvariantRun(t, policy, &permissionArgumentRewrite{})
	id := waitPermissionInteractionID(t, run)
	if err := run.Respond(t.Context(), id, InteractionResponse{Permission: PermissionAllowOnce}); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if executions.Load() != 0 || policy.resolve.Load() != 1 {
		t.Fatalf("executions=%d resolutions=%d", executions.Load(), policy.resolve.Load())
	}
	for event := range run.Events() {
		if finished, ok := event.Payload.(ToolFinished); ok && strings.Contains(finished.Result, ErrPermissionArgumentsChanged.Error()) {
			return
		}
	}
	t.Fatal("missing tool argument authorization failure")
}
