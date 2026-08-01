package harness

import (
	"context"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestContextStructuralRestorePlanStrictRoundTrip(t *testing.T) {
	binding := agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "restore-plan"}
	mutation := json.RawMessage(` { "summary" : "bounded", "counter": 9007199254740993, "id" : "cc-1" } `)
	hash, err := agentstructural.IntentHash(agentstructural.Compact, binding, "context:7", "cc-1", mutation)
	if err != nil {
		t.Fatal(err)
	}
	compaction := agentcompaction.ResultFromCheckpoint(agentcompaction.NewCheckpoint("", agentcompaction.Result{
		Phase: "mid_run", TriggerReason: "compaction_capacity_reserve",
		EstimatedTokensBefore: 1800, ObservedPromptTokens: 1900, ObservedEstimateTokens: 1700,
		TokensBefore: 2000, TokensAfter: 400, ContextWindowTokens: 2400,
		Strategy: "summary", Threshold: 0.85, RecoveryBand: 0.75,
		Epoch: 2, Summary: "bounded", TargetRatio: 0.22, RetainedTurns: 2,
		CandidateFingerprint: "sha256:candidate", CandidateGeneration: 7,
	}))
	compaction.SourceMessageCount = 8
	compaction.ProjectedTokensBefore = 2300
	compaction.ProjectedTokensAfter = 700
	compaction.ReservedCompletionTokens = 200
	compaction.ReservedToolResultTokens = 100
	compaction.MessageCountBefore = 12
	compaction.MessageCountAfter = 5
	plan := agentstructural.RestorePlan{
		Version: agentstructural.RestorePlanVersion, Domain: agentstructural.DomainSession,
		Action: agentstructural.Compact, Commit: true, IntentHash: hash, RecordID: "cc-1",
		Result:   agentstructural.Result{Compaction: compaction},
		Mutation: mutation,
	}
	encoded, err := agentstructural.EncodeRestorePlan(plan, binding, "context:7")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := agentstructural.DecodeRestorePlan(encoded, binding, "context:7")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Result, plan.Result) || decoded.IntentHash != hash || decoded.RecordID != "cc-1" {
		t.Fatalf("decoded plan = %#v, want %#v", decoded, plan)
	}
	if got, want := string(decoded.Mutation), `{"counter":9007199254740993,"id":"cc-1","summary":"bounded"}`; got != want {
		t.Fatalf("canonical mutation = %s, want %s", got, want)
	}
	encoded[0] = '['
	if got := string(decoded.Mutation); got != `{"counter":9007199254740993,"id":"cc-1","summary":"bounded"}` {
		t.Fatalf("decoded mutation aliases descriptor: %s", got)
	}

	reorderedHash, err := agentstructural.IntentHash(
		agentstructural.Compact, binding, "context:7", "cc-1",
		json.RawMessage(`{"id":"cc-1","counter":9007199254740993,"summary":"bounded"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if reorderedHash != hash {
		t.Fatalf("canonical hashes differ: %q != %q", reorderedHash, hash)
	}
}

func TestContextStructuralBindingOptionsKeepStableProjectIdentity(t *testing.T) {
	t.Parallel()

	for _, agentKind := range []string{agentrun.AgentKindGeneral, agentrun.AgentKindIDE} {
		agentKind := agentKind
		t.Run(agentKind, func(t *testing.T) {
			t.Parallel()
			ref, err := (agentrun.RuntimeBinding{
				AgentKind: agentKind, ProjectID: "project-1", Mode: agentrun.ModeAgentChat,
				Workspace: "/workspace/old", SessionID: "session-1",
			}).Ref()
			if err != nil {
				t.Fatal(err)
			}
			options, err := contextStructuralBindingOptions(ref, agentrun.Options{
				Workspace: "/workspace/current", StateRoot: "/state/current",
			})
			if err != nil {
				t.Fatal(err)
			}
			if options.AgentKind != agentKind || options.ProjectID != "project-1" ||
				options.SessionID != "session-1" || options.Mode != agentrun.ModeAgentChat ||
				options.Workspace != "/workspace/current" || options.StateRoot != "/state/current" {
				t.Fatalf("Project structural options = %#v", options)
			}
		})
	}
}

func TestGeneralProjectStructuralRestorePlanRoundTrip(t *testing.T) {
	t.Parallel()
	binding := agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindGeneral, ProjectID: "project-1",
		Mode: agentrun.ModeAgentChat, SessionID: "session-1",
	}
	mutation := json.RawMessage(`{"id":"cc-general"}`)
	hash, err := agentstructural.IntentHash(
		agentstructural.Compact, binding, "session-context:1", "cc-general", mutation,
	)
	if err != nil {
		t.Fatal(err)
	}
	plan := agentstructural.RestorePlan{
		Version: agentstructural.RestorePlanVersion, Domain: agentstructural.DomainSession,
		Action: agentstructural.Compact, Commit: true, IntentHash: hash, RecordID: "cc-general",
		Result:   agentstructural.Result{Compaction: agentcompaction.Result{Triggered: true}},
		Mutation: mutation,
	}
	descriptor, err := agentstructural.EncodeRestorePlan(plan, binding, "session-context:1")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := agentstructural.DecodeRestorePlan(descriptor, binding, "session-context:1")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.IntentHash != plan.IntentHash || decoded.RecordID != plan.RecordID {
		t.Fatalf("General structural restore plan = %#v", decoded)
	}
}

func TestContextStructuralRestorePlanRejectsInvalidDescriptors(t *testing.T) {
	binding := agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "invalid-plan"}
	mutation := json.RawMessage(`{"id":"cc-1"}`)
	hash, err := agentstructural.IntentHash(agentstructural.Compact, binding, "context:7", "cc-1", mutation)
	if err != nil {
		t.Fatal(err)
	}
	valid := agentstructural.RestorePlan{
		Version: agentstructural.RestorePlanVersion, Domain: agentstructural.DomainSession,
		Action: agentstructural.Compact, Commit: true, IntentHash: hash, RecordID: "cc-1",
		Result: agentstructural.Result{Compaction: agentcompaction.Result{Triggered: true}}, Mutation: mutation,
	}
	encoded, err := agentstructural.EncodeRestorePlan(valid, binding, "context:7")
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	if err := json.Unmarshal(encoded, &base); err != nil {
		t.Fatal(err)
	}
	invalid := map[string]func(map[string]any){
		"unknown field":  func(value map[string]any) { value["unexpected"] = true },
		"missing commit": func(value map[string]any) { delete(value, "commit") },
		"null commit":    func(value map[string]any) { value["commit"] = nil },
		"missing result": func(value map[string]any) { delete(value, "result") },
		"null result":    func(value map[string]any) { value["result"] = nil },
		"missing removed": func(value map[string]any) {
			delete(value["result"].(map[string]any), "removed")
		},
		"null removed": func(value map[string]any) {
			value["result"].(map[string]any)["removed"] = nil
		},
		"missing triggered": func(value map[string]any) {
			result := value["result"].(map[string]any)
			delete(result["compaction"].(map[string]any), "triggered")
		},
		"null triggered": func(value map[string]any) {
			result := value["result"].(map[string]any)
			result["compaction"].(map[string]any)["triggered"] = nil
		},
		"version": func(value map[string]any) { value["version"] = float64(99) },
		"domain":  func(value map[string]any) { value["domain"] = "story" },
		"action":  func(value map[string]any) { value["action"] = "rewrite_everything" },
		"commit result": func(value map[string]any) {
			value["commit"] = false
		},
		"intent hash": func(value map[string]any) { value["intent_hash"] = "sha256:tampered" },
		"mutation":    func(value map[string]any) { value["mutation"] = []any{"not", "an", "object"} },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			copyBytes, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(copyBytes, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			descriptor, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := agentstructural.DecodeRestorePlan(descriptor, binding, "context:7"); err == nil {
				t.Fatalf("invalid descriptor was accepted: %s", descriptor)
			}
		})
	}
	bindingRef, err := binding.Ref()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentstructural.EncodeRuntimeRestorePlan(valid, bindingRef, "context:7", 32); err == nil || !strings.Contains(err.Error(), "exceeds 32 bytes") {
		t.Fatalf("bounded encode error = %v", err)
	}
	duplicate := json.RawMessage(strings.TrimSuffix(string(encoded), "}") + `,"version":1}`)
	if _, err := agentstructural.DecodeRestorePlan(duplicate, binding, "context:7"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate descriptor field error = %v", err)
	}
	if _, err := agentstructural.IntentHash(
		agentstructural.Compact, binding, "context:7", "cc-1", json.RawMessage(`{"id":"cc-1","id":"cc-2"}`),
	); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate mutation field error = %v", err)
	}
}

type recoveredFixedStructuralOperation struct {
	mu        sync.Mutex
	identity  agentstructural.Identity
	plan      agentstructural.RestorePlan
	prepared  int
	committed int
	receipt   agentstructural.Receipt
}

func (o *recoveredFixedStructuralOperation) Prepare(
	_ context.Context,
	identity agentstructural.Identity,
	_ func(agentrun.Event),
) (agentstructural.Intent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.prepared++
	o.identity = identity
	return agentstructural.Intent{Hash: o.plan.IntentHash, Commit: o.plan.Commit, Result: o.plan.Result}, nil
}

func (o *recoveredFixedStructuralOperation) Commit(
	_ context.Context,
	identity agentstructural.Identity,
	intent agentstructural.Intent,
) (agentstructural.Receipt, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if identity != o.identity || intent.Hash != o.plan.IntentHash || !reflect.DeepEqual(intent.Result, o.plan.Result) {
		return agentstructural.Receipt{}, errors.New("restored structural commit identity changed")
	}
	o.committed++
	return o.receipt, nil
}

func (o *recoveredFixedStructuralOperation) Reconcile(context.Context) (agentstructural.Result, agentstructural.Receipt, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.plan.Result, o.receipt, o.committed > 0, nil
}

func TestResumeRecoveredContextStructuralOperationColdRestoresExactPlanOnce(t *testing.T) {
	store := runstate.NewMemoryJournalStore()
	options := agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "cold-structural", Mode: "ide"}.Normalize("")
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	mutation := json.RawMessage(`{"id":"cc-cold","summary":"checkpoint"}`)
	productBinding, err := agentrun.ParseRuntimeBinding(bindingRef)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := agentstructural.IntentHash(agentstructural.Compact, productBinding, "context:7", "cc-cold", mutation)
	if err != nil {
		t.Fatal(err)
	}
	plan := agentstructural.RestorePlan{
		Version: agentstructural.RestorePlanVersion, Domain: agentstructural.DomainSession,
		Action: agentstructural.Compact, Commit: true, IntentHash: hash, RecordID: "cc-cold",
		Result:   agentstructural.Result{Compaction: agentcompaction.Result{Triggered: true, Epoch: 3, Summary: "checkpoint"}},
		Mutation: mutation,
	}
	descriptor, err := agentstructural.EncodeRestorePlan(plan, productBinding, "context:7")
	if err != nil {
		t.Fatal(err)
	}
	command := runstate.CompactIfNeeded{ID: "cold-structural-command", Ref: runstate.ContextCompactionRef{
		SpecRef: "cold-structural-spec", Source: "session.effective_messages", Purpose: "checkpoint",
		Resource: options.SessionID, ExpectedRevision: "context:7", RestoreDescriptor: descriptor,
	}}
	operationID := runstate.OperationID("cold-structural-operation")
	snapshot := runstate.StructuralOperationSnapshot{
		Binding: bindingRef, CommandID: command.ID, OperationID: operationID, Cycle: 1,
		Kind: runstate.StructuralCompactContext, Ref: command.Ref, ContextCursor: 2,
	}
	key, err := json.Marshal(bindingRef)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(key))
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := runstate.CommandFingerprint(command)
	if err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if _, err := journal.Append(context.Background(), 0, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{
			CommandID: command.ID, CommandKind: "compact_context", OperationID: operationID,
			Fingerprint: fingerprint,
		},
		runstate.OperationStartedEvent{OperationID: operationID, Phase: runstate.PhaseCompacting, Structural: &snapshot},
	}); err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	operation := &recoveredFixedStructuralOperation{plan: plan, receipt: agentstructural.Receipt{Revision: "session-context:8"}}
	wantSnapshot, err := agentrun.StructuralOperationFromRuntime(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restoreCalls := 0
	service, err := newServiceWithOptions(context.Background(), agentrun.DefaultLoopPolicy(), store, serviceOptions{
		structuralRestorer: func(_ context.Context, request StructuralRestoreRequest) (agentstructural.Spec, error) {
			restoreCalls++
			if request.Binding != productBinding || !reflect.DeepEqual(request.Snapshot, wantSnapshot) || !reflect.DeepEqual(request.Plan, plan) {
				return agentstructural.Spec{}, errors.New("cold structural restore request changed")
			}
			request.Snapshot.Ref.RestoreDescriptor[0] = '['
			request.Plan.Mutation[0] = '['
			// Deliberately return conflicting transport identity. The adapter must
			// replace all of it with the durable snapshot before registration; the
			// request mutations above must not alias its retained plan either.
			return agentstructural.Spec{
				CommandID: "wrong", Action: agentstructural.Remove,
				Ref:       agentrun.ContextCompactionRef{SpecRef: "wrong"},
				Options:   agentrun.Options{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: "/wrong", StoryID: "wrong", BranchID: "wrong"},
				Operation: operation,
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())
	harness, err := service.coordinator.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.RecoveryPaused || status.Phase != runstate.PhaseCompacting || restoreCalls != 0 {
		t.Fatalf("open must only pause cold structural recovery: status=%#v restore_calls=%d", status, restoreCalls)
	}
	result, resumed, err := service.ResumeRecoveredStructuralOperation(context.Background(), options, agentstructural.Compact)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed || !reflect.DeepEqual(result, plan.Result) || restoreCalls != 1 {
		t.Fatalf("resume = result %#v resumed=%t restore_calls=%d", result, resumed, restoreCalls)
	}
	operation.mu.Lock()
	prepared, committed, identity := operation.prepared, operation.committed, operation.identity
	operation.mu.Unlock()
	if prepared != 1 || committed != 1 || identity.CommandID != agentrun.CommandID(command.ID) || identity.OperationID != agentrun.OperationID(operationID) || identity.Cycle != 1 {
		t.Fatalf("restored execution = prepare/commit %d/%d identity %#v", prepared, committed, identity)
	}
	if _, resumedAgain, err := service.ResumeRecoveredStructuralOperation(context.Background(), options); err != nil || resumedAgain {
		t.Fatalf("second resume = resumed=%t err=%v", resumedAgain, err)
	}
	operation.mu.Lock()
	committed = operation.committed
	operation.mu.Unlock()
	if committed != 1 || restoreCalls != 1 {
		t.Fatalf("duplicate canonical commit: commits=%d restore_calls=%d", committed, restoreCalls)
	}
}
