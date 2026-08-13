package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentsession "github.com/alfredxw/denova/agent/session"
)

type inspectionCleanupManager struct{}

func (inspectionCleanupManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "cleanup.inspection-test", Version: 1}
}

func (inspectionCleanupManager) Plan(context.Context, CleanupPlanRequest) (CleanupPlan, error) {
	return CleanupPlan{Action: CleanupNone}, nil
}

type inspectionContextSource struct{ called atomic.Bool }

func (*inspectionContextSource) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "context.inspection-test", Version: 1}
}

func (source *inspectionContextSource) Materialize(ctx context.Context, _ ContextRequest) ([]ContextFragment, error) {
	if !IsInspection(ctx) {
		return nil, errors.New("inspection ContextSource did not receive an inspection context")
	}
	source.called.Store(true)
	return []ContextFragment{{
		Source: "test.inspection", Purpose: "verify exact read-only assembly", Resource: "stable-context",
		Revision: "1", Stability: ContextStablePrefix, Placement: ContextLeadingMessage, Content: "INSPECTION_STABLE_CONTEXT", HardLimit: 64 << 10,
	}}, nil
}

type inspectionMiddleware struct{ BaseMiddleware }

func (*inspectionMiddleware) BeforeModelCall(
	ctx context.Context,
	call *ModelCall,
	_ *ModelContext,
) (context.Context, *ModelCall, error) {
	if !IsInspection(ctx) {
		return ctx, nil, errors.New("inspection Middleware did not receive an inspection context")
	}
	next := *call
	next.Messages = append(cloneMessages(call.Messages), UserMessage("INSPECTION_MIDDLEWARE_SUFFIX"))
	return ctx, &next, nil
}

func TestSessionInspectUsesExactActiveMaintenanceAndMiddlewareWithoutSideEffects(t *testing.T) {
	model := &lifecycleModel{}
	contextSource := &inspectionContextSource{}
	tool := testToolDefinition(&functionTool{
		name: "inspect_read",
		run:  func(context.Context, string) (string, error) { return "unexpected", nil },
	})
	store := &persistentMemoryStore{Store: agentsession.Memory()}
	owner, err := New(context.Background(), Definition{
		Key: "inspection-definition", Name: "inspection-agent", Model: model,
		ModelIdentity: CapabilityIdentity{Kind: "model.inspection-test", Version: 1},
		Instructions:  "INSPECTION_SYSTEM_INSTRUCTION",
		Tools: mustStaticToolsIdentified(t,
			CapabilityIdentity{Kind: "tools.inspection-test", Version: 1},
			tool,
		),
		Context:    contextSource,
		Cleanup:    inspectionCleanupManager{},
		Compaction: fixedCompactionManager{},
		Middlewares: []Middleware{IdentifyMiddleware(
			&inspectionMiddleware{},
			CapabilityIdentity{Kind: "middleware.inspection-test", Version: 1},
		)},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("inspect-exact"))
	if err != nil {
		t.Fatal(err)
	}
	callIndex := 0
	raw := []*Message{
		UserMessage("old turn"),
		AssistantMessage("old answer", nil),
		UserMessage("tool turn"),
		AssistantMessage("", []ToolCall{{
			Index: &callIndex, ID: "read-1", Type: "function",
			Function: FunctionCall{Name: "inspect_read", Arguments: `{}`},
		}}),
		ToolMessage(TextToolResult("RICH_TOOL_BODY"), "read-1", WithToolName("inspect_read")),
		AssistantMessage("tool answer", nil),
	}
	if _, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source:         CapabilityIdentity{Kind: "transcript.inspection-test", Version: 1},
		SourceRevision: 1,
		Messages:       raw,
	}); err != nil {
		t.Fatal(err)
	}
	cleanupHash, err := hashCanonical(raw[:5])
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	setClearTestCapability(t, session, cleanupCapability, CleanupState{
		ID: "inspection-cleanup", Revision: 2, SourceRevision: "transcript:1", SourceHash: cleanupHash,
		SourceStart: 4, SourceEnd: 5,
		Replacements: []CleanupReplacement{{
			MessageIndex: 4, ToolCallID: "read-1", Placeholder: "[INSPECTION_CLEANUP_PLACEHOLDER]",
		}},
		Renderer: "inspection.v1", CreatedAt: now, UpdatedAt: now,
	})
	setClearTestCapability(t, session, compactionCapability, CompactionState{
		ID: "inspection-compaction", Revision: 1, SourceRevision: "transcript:1", SourceHash: "inspection-source",
		Summary: "INSPECTION_COMPACTION_SUMMARY", ReplacementFrom: 0, ReplacementTo: 2,
		CleanupRevisionAtCompaction: 1, CreatedAt: now,
	})
	before, err := session.harness.IdleEngineCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := session.Inspect(context.Background(), Text("current request"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := session.harness.IdleEngineCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if before.Cursor != after.Cursor || before.StateDescriptor != after.StateDescriptor {
		t.Fatalf("inspection mutated Session checkpoint: before=%#v after=%#v", before, after)
	}
	if !contextSource.called.Load() {
		t.Fatal("inspection did not materialize the configured ContextSource")
	}
	if calls := model.calls(); len(calls) != 0 {
		t.Fatalf("inspection invoked the model: %#v", calls)
	}
	if inspection.DefinitionKey != "inspection-definition" || inspection.RestoreKey == "" ||
		inspection.MaterializedFingerprint == "" || inspection.PrefixFingerprint == "" {
		t.Fatalf("inspection identities=%#v", inspection)
	}
	if inspection.Cleanup == nil || inspection.Cleanup.ID != "inspection-cleanup" ||
		inspection.Compaction == nil || inspection.Compaction.ID != "inspection-compaction" {
		t.Fatalf("inspection maintenance=%#v", inspection)
	}
	if len(inspection.ContextFragments) != 1 || inspection.ContextFragments[0].Source != "test.inspection" ||
		inspection.ContextFragments[0].Content != "INSPECTION_STABLE_CONTEXT" {
		t.Fatalf("inspection context provenance=%#v", inspection.ContextFragments)
	}
	if len(inspection.ModelRequest.Options.Tools) != 1 || inspection.ModelRequest.Options.Tools[0].Name != "inspect_read" ||
		inspection.ModelRequest.Options.SessionKey == "" {
		t.Fatalf("inspection options=%#v", inspection.ModelRequest.Options)
	}
	if inspection.ModelRequest.StablePrefixMessages != 3 {
		t.Fatalf("stable prefix messages=%d, want instruction + context + checkpoint", inspection.ModelRequest.StablePrefixMessages)
	}
	joined := inspectionMessageText(inspection.ModelRequest.Messages)
	for _, required := range []string{
		"INSPECTION_SYSTEM_INSTRUCTION",
		"INSPECTION_STABLE_CONTEXT",
		"INSPECTION_COMPACTION_SUMMARY",
		"[INSPECTION_CLEANUP_PLACEHOLDER]",
		"tool answer",
		"current request",
		"INSPECTION_MIDDLEWARE_SUFFIX",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("inspection request is missing %q:\n%s", required, joined)
		}
	}
	for _, hidden := range []string{"old turn", "old answer", "RICH_TOOL_BODY"} {
		if strings.Contains(joined, hidden) {
			t.Fatalf("inspection request resurrected %q:\n%s", hidden, joined)
		}
	}
}

func TestSessionInspectRejectsBusySessionAndUncommittedGoalMutation(t *testing.T) {
	model := newGatedLifecycleModel()
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("inspect-busy"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Inspect(context.Background(), Input{
		Text: "preview", Goal: &GoalMutation{Kind: GoalSet, Objective: "not durable"},
	}); err == nil || !strings.Contains(err.Error(), "Goal mutation") {
		t.Fatalf("uncommitted Goal inspection error=%v", err)
	}
	run, err := session.Run(context.Background(), Input{Text: "block", IdempotencyKey: "inspect-busy-run"})
	if err != nil {
		t.Fatal(err)
	}
	<-model.calls
	if _, err := session.Inspect(context.Background(), Text("preview")); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("busy inspection error=%v, want ErrSessionBusy", err)
	}
	if _, err := run.Abort(context.Background(), AbortRequest{Reason: "test cleanup", IdempotencyKey: "inspect-busy-abort"}); err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultAborted {
		t.Fatalf("aborted result=%#v error=%v", result, err)
	}
}

func inspectionMessageText(messages []*Message) string {
	var result strings.Builder
	for _, message := range messages {
		if message != nil {
			result.WriteString(message.Content)
			result.WriteByte('\n')
		}
	}
	return result.String()
}
