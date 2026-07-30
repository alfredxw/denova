package agents

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestContextStructuralRestorePlanStrictRoundTrip(t *testing.T) {
	binding := RuntimeBinding{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "restore-plan"}
	mutation := json.RawMessage(` { "summary" : "bounded", "counter": 9007199254740993, "id" : "cc-1" } `)
	hash, err := ContextStructuralIntentHash(ContextStructuralCompact, binding, "context:7", "cc-1", mutation)
	if err != nil {
		t.Fatal(err)
	}
	plan := ContextStructuralRestorePlan{
		Version: ContextStructuralRestorePlanVersion, Domain: ContextStructuralDomainSession,
		Action: ContextStructuralCompact, Commit: true, IntentHash: hash, RecordID: "cc-1",
		Result: ContextStructuralResult{Compaction: ContextCompactionResult{
			Triggered: true, Epoch: 2, Summary: "bounded", SourceMessageCount: 8,
		}},
		Mutation: mutation,
	}
	encoded, err := EncodeContextStructuralRestorePlan(plan, binding, "context:7")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeContextStructuralRestorePlan(encoded, binding, "context:7")
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

	reorderedHash, err := ContextStructuralIntentHash(
		ContextStructuralCompact, binding, "context:7", "cc-1",
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

	for _, agentKind := range []string{AgentKindGeneral, AgentKindIDE} {
		agentKind := agentKind
		t.Run(agentKind, func(t *testing.T) {
			t.Parallel()
			ref, err := (RuntimeBinding{
				AgentKind: agentKind, ProjectID: "project-1", Mode: runtimeBindingProfileAgentChat,
				Workspace: "/workspace/old", SessionID: "session-1",
			}).Ref()
			if err != nil {
				t.Fatal(err)
			}
			options, err := contextStructuralBindingOptions(ref, RunOptions{
				Workspace: "/workspace/current", StateRoot: "/state/current",
			})
			if err != nil {
				t.Fatal(err)
			}
			if options.AgentKind != agentKind || options.ProjectID != "project-1" ||
				options.SessionID != "session-1" || options.Mode != runtimeBindingProfileAgentChat ||
				options.Workspace != "/workspace/current" || options.StateRoot != "/state/current" {
				t.Fatalf("Project structural options = %#v", options)
			}
		})
	}
}

func TestGeneralProjectStructuralRestorePlanRoundTrip(t *testing.T) {
	t.Parallel()
	binding := RuntimeBinding{
		AgentKind: AgentKindGeneral, ProjectID: "project-1",
		Mode: runtimeBindingProfileAgentChat, SessionID: "session-1",
	}
	mutation := json.RawMessage(`{"id":"cc-general"}`)
	hash, err := ContextStructuralIntentHash(
		ContextStructuralCompact, binding, "session-context:1", "cc-general", mutation,
	)
	if err != nil {
		t.Fatal(err)
	}
	plan := ContextStructuralRestorePlan{
		Version: ContextStructuralRestorePlanVersion, Domain: ContextStructuralDomainSession,
		Action: ContextStructuralCompact, Commit: true, IntentHash: hash, RecordID: "cc-general",
		Result:   ContextStructuralResult{Compaction: ContextCompactionResult{Triggered: true}},
		Mutation: mutation,
	}
	descriptor, err := EncodeContextStructuralRestorePlan(plan, binding, "session-context:1")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeContextStructuralRestorePlan(descriptor, binding, "session-context:1")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.IntentHash != plan.IntentHash || decoded.RecordID != plan.RecordID {
		t.Fatalf("General structural restore plan = %#v", decoded)
	}
}

func TestContextStructuralRestorePlanRejectsInvalidDescriptors(t *testing.T) {
	binding := RuntimeBinding{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "invalid-plan"}
	mutation := json.RawMessage(`{"id":"cc-1"}`)
	hash, err := ContextStructuralIntentHash(ContextStructuralCompact, binding, "context:7", "cc-1", mutation)
	if err != nil {
		t.Fatal(err)
	}
	valid := ContextStructuralRestorePlan{
		Version: ContextStructuralRestorePlanVersion, Domain: ContextStructuralDomainSession,
		Action: ContextStructuralCompact, Commit: true, IntentHash: hash, RecordID: "cc-1",
		Result: ContextStructuralResult{Compaction: ContextCompactionResult{Triggered: true}}, Mutation: mutation,
	}
	encoded, err := EncodeContextStructuralRestorePlan(valid, binding, "context:7")
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
			if _, err := DecodeContextStructuralRestorePlan(descriptor, binding, "context:7"); err == nil {
				t.Fatalf("invalid descriptor was accepted: %s", descriptor)
			}
		})
	}
	bindingRef, err := binding.Ref()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encodeContextStructuralRestorePlan(valid, bindingRef, "context:7", 32); err == nil || !strings.Contains(err.Error(), "exceeds 32 bytes") {
		t.Fatalf("bounded encode error = %v", err)
	}
	duplicate := json.RawMessage(strings.TrimSuffix(string(encoded), "}") + `,"version":1}`)
	if _, err := DecodeContextStructuralRestorePlan(duplicate, binding, "context:7"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate descriptor field error = %v", err)
	}
	if _, err := ContextStructuralIntentHash(
		ContextStructuralCompact, binding, "context:7", "cc-1", json.RawMessage(`{"id":"cc-1","id":"cc-2"}`),
	); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate mutation field error = %v", err)
	}
}

type recoveredFixedStructuralOperation struct {
	mu        sync.Mutex
	identity  ContextStructuralIdentity
	plan      ContextStructuralRestorePlan
	prepared  int
	committed int
	receipt   ContextStructuralReceipt
}

func (o *recoveredFixedStructuralOperation) Prepare(
	_ context.Context,
	identity ContextStructuralIdentity,
	_ func(Event),
) (ContextStructuralIntent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.prepared++
	o.identity = identity
	return ContextStructuralIntent{Hash: o.plan.IntentHash, Commit: o.plan.Commit, Result: o.plan.Result}, nil
}

func (o *recoveredFixedStructuralOperation) Commit(
	_ context.Context,
	identity ContextStructuralIdentity,
	intent ContextStructuralIntent,
) (ContextStructuralReceipt, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if identity != o.identity || intent.Hash != o.plan.IntentHash || !reflect.DeepEqual(intent.Result, o.plan.Result) {
		return ContextStructuralReceipt{}, errors.New("restored structural commit identity changed")
	}
	o.committed++
	return o.receipt, nil
}

func (o *recoveredFixedStructuralOperation) Reconcile(context.Context) (ContextStructuralResult, ContextStructuralReceipt, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.plan.Result, o.receipt, o.committed > 0, nil
}

func TestResumeRecoveredContextStructuralOperationColdRestoresExactPlanOnce(t *testing.T) {
	store := runstate.NewMemoryJournalStore()
	options := RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "cold-structural", Mode: "ide"}.normalized("")
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	mutation := json.RawMessage(`{"id":"cc-cold","summary":"checkpoint"}`)
	productBinding, err := ParseRuntimeBinding(bindingRef)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := ContextStructuralIntentHash(ContextStructuralCompact, productBinding, "context:7", "cc-cold", mutation)
	if err != nil {
		t.Fatal(err)
	}
	plan := ContextStructuralRestorePlan{
		Version: ContextStructuralRestorePlanVersion, Domain: ContextStructuralDomainSession,
		Action: ContextStructuralCompact, Commit: true, IntentHash: hash, RecordID: "cc-cold",
		Result:   ContextStructuralResult{Compaction: ContextCompactionResult{Triggered: true, Epoch: 3, Summary: "checkpoint"}},
		Mutation: mutation,
	}
	descriptor, err := EncodeContextStructuralRestorePlan(plan, productBinding, "context:7")
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

	operation := &recoveredFixedStructuralOperation{plan: plan, receipt: ContextStructuralReceipt{Revision: "session-context:8"}}
	wantSnapshot, err := structuralOperationFromRuntime(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restoreCalls := 0
	service, err := newHarnessChatServiceWithOptions(context.Background(), DefaultLoopPolicy(), store, durableChatServiceOptions{
		structuralRestorer: func(_ context.Context, request HarnessStructuralRestoreRequest) (ContextStructuralSpec, error) {
			restoreCalls++
			if request.Binding != productBinding || !reflect.DeepEqual(request.Snapshot, wantSnapshot) || !reflect.DeepEqual(request.Plan, plan) {
				return ContextStructuralSpec{}, errors.New("cold structural restore request changed")
			}
			request.Snapshot.Ref.RestoreDescriptor[0] = '['
			request.Plan.Mutation[0] = '['
			// Deliberately return conflicting transport identity. The adapter must
			// replace all of it with the durable snapshot before registration; the
			// request mutations above must not alias its retained plan either.
			return ContextStructuralSpec{
				CommandID: "wrong", Action: ContextStructuralRemove,
				Ref:       ContextCompactionRef{SpecRef: "wrong"},
				Options:   RunOptions{AgentKind: AgentKindInteractiveStory, Workspace: "/wrong", StoryID: "wrong", BranchID: "wrong"},
				Operation: operation,
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())
	harness, err := service.harness.runtime.Open(context.Background(), binding)
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
	result, resumed, err := service.ResumeRecoveredContextStructuralOperation(context.Background(), options, ContextStructuralCompact)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed || !reflect.DeepEqual(result, plan.Result) || restoreCalls != 1 {
		t.Fatalf("resume = result %#v resumed=%t restore_calls=%d", result, resumed, restoreCalls)
	}
	operation.mu.Lock()
	prepared, committed, identity := operation.prepared, operation.committed, operation.identity
	operation.mu.Unlock()
	if prepared != 1 || committed != 1 || identity.CommandID != CommandID(command.ID) || identity.OperationID != OperationID(operationID) || identity.Cycle != 1 {
		t.Fatalf("restored execution = prepare/commit %d/%d identity %#v", prepared, committed, identity)
	}
	if _, resumedAgain, err := service.ResumeRecoveredContextStructuralOperation(context.Background(), options); err != nil || resumedAgain {
		t.Fatalf("second resume = resumed=%t err=%v", resumedAgain, err)
	}
	operation.mu.Lock()
	committed = operation.committed
	operation.mu.Unlock()
	if committed != 1 || restoreCalls != 1 {
		t.Fatalf("duplicate canonical commit: commits=%d restore_calls=%d", committed, restoreCalls)
	}
}
