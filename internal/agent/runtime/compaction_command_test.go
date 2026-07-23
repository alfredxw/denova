package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	runstate "denova/internal/agent/runtime"
)

type compactionEngine struct {
	started chan runstate.StructuralEngineRequest
	release chan struct{}
	onRun   func(runstate.StructuralEngineRequest, runstate.EngineEventSink) error
}

func (e *compactionEngine) Run(context.Context, runstate.EngineRequest, runstate.EngineEventSink) (runstate.EngineResult, error) {
	return runstate.EngineResult{}, errors.New("turn engine must not run for a compaction command")
}

func (e *compactionEngine) RunStructural(ctx context.Context, request runstate.StructuralEngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
	if e.started != nil {
		e.started <- request
	}
	if e.onRun != nil {
		if err := e.onRun(request, emit); err != nil {
			return runstate.EngineResult{}, err
		}
	}
	if e.release != nil {
		select {
		case <-e.release:
		case <-ctx.Done():
			return runstate.EngineResult{Status: runstate.EngineAborted}, nil
		}
	}
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}

func TestCompactIfNeededIsACompactingDurableOperation(t *testing.T) {
	engine := &compactionEngine{started: make(chan runstate.StructuralEngineRequest, 1), release: make(chan struct{})}
	runtime, err := runstate.NewRuntime(
		runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) { return engine, nil }),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	command := runstate.CompactIfNeeded{ID: "compact-1", Ref: runstate.ContextCompactionRef{
		SpecRef: "spec-1", Source: "session.effective_messages", Purpose: "bounded model history checkpoint",
		Resource: "session", ExpectedRevision: "context:7", Force: true,
	}}
	receipt, err := harness.Submit(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	request := <-engine.started
	if request.Snapshot.Kind != runstate.StructuralCompactContext || !reflect.DeepEqual(request.Snapshot.Ref, command.Ref) {
		t.Fatalf("unexpected structural request: %#v", request)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseCompacting || status.ActiveOperation != receipt.OperationID {
		t.Fatalf("status while compacting = %#v", status)
	}
	close(engine.release)
	waitForOperationStatus(t, harness, receipt.OperationID, runstate.OperationSucceeded)
	replayed, err := harness.Submit(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.OperationID != receipt.OperationID {
		t.Fatalf("idempotent replay = %#v, want operation %s", replayed, receipt.OperationID)
	}
}

func TestContextCompactionRestoreDescriptorIsBoundedAndValidJSON(t *testing.T) {
	for name, descriptor := range map[string]json.RawMessage{
		"oversized": json.RawMessage(`{"plan":1}`),
		"invalid":   json.RawMessage(`{"plan":`),
	} {
		t.Run(name, func(t *testing.T) {
			engine := &compactionEngine{started: make(chan runstate.StructuralEngineRequest, 1)}
			runtime, err := runstate.NewRuntime(
				runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) { return engine, nil }),
				runstate.NewMemoryJournalStore(),
				runstate.RuntimeConfig{InputLimits: runstate.InputLimits{MaxRestoreDescriptorBytes: 8}},
			)
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close(context.Background())
			harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "descriptor-" + name})
			if err != nil {
				t.Fatal(err)
			}
			_, err = harness.Submit(context.Background(), runstate.CompactIfNeeded{ID: runstate.CommandID("compact-" + name), Ref: runstate.ContextCompactionRef{
				SpecRef: "spec-" + name, Source: "session.effective_messages", Purpose: "checkpoint",
				Resource: "descriptor-" + name, ExpectedRevision: "context:1", RestoreDescriptor: descriptor,
			}})
			if !errors.Is(err, runstate.ErrInvalidCommand) {
				t.Fatalf("descriptor error = %v, want ErrInvalidCommand", err)
			}
			select {
			case request := <-engine.started:
				t.Fatalf("engine unexpectedly started: %#v", request)
			default:
			}
		})
	}
}

func TestContextCompactionRestoreDescriptorIsDeepClonedAndFingerprinted(t *testing.T) {
	engine := &compactionEngine{started: make(chan runstate.StructuralEngineRequest, 1), release: make(chan struct{})}
	runtime, err := runstate.NewRuntime(
		runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) { return engine, nil }),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "descriptor-clone"})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := json.RawMessage(`{"version":1,"mutation":{"id":"cc-1"}}`)
	command := runstate.CompactIfNeeded{ID: "compact-clone", Ref: runstate.ContextCompactionRef{
		SpecRef: "spec-clone", Source: "session.effective_messages", Purpose: "checkpoint",
		Resource: "descriptor-clone", ExpectedRevision: "context:1", RestoreDescriptor: descriptor,
	}}
	receipt, err := harness.Submit(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	request := <-engine.started
	wantDescriptor := string(descriptor)
	descriptor[0] = '['
	if got := string(request.Snapshot.Ref.RestoreDescriptor); got != wantDescriptor {
		t.Fatalf("engine descriptor changed through caller alias: %q, want %q", got, wantDescriptor)
	}
	request.Snapshot.Ref.RestoreDescriptor[0] = '['
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(status.ActiveStructural.Ref.RestoreDescriptor); got != wantDescriptor {
		t.Fatalf("active descriptor changed through engine snapshot alias: %q, want %q", got, wantDescriptor)
	}
	status.ActiveStructural.Ref.RestoreDescriptor[0] = '['
	statusAgain, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(statusAgain.ActiveStructural.Ref.RestoreDescriptor); got != wantDescriptor {
		t.Fatalf("status descriptor changed through snapshot alias: %q, want %q", got, wantDescriptor)
	}
	conflict := command
	conflict.Ref.RestoreDescriptor = json.RawMessage(`{"version":1,"mutation":{"id":"cc-2"}}`)
	if _, err := harness.Submit(context.Background(), conflict); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("changed descriptor replay error = %v, want ErrInvalidCommand", err)
	}
	close(engine.release)
	waitForOperationStatus(t, harness, receipt.OperationID, runstate.OperationSucceeded)
}

func TestRemoveCompactionAbortSettlesWithoutCanonicalCommit(t *testing.T) {
	engine := &compactionEngine{started: make(chan runstate.StructuralEngineRequest, 1), release: make(chan struct{})}
	runtime, err := runstate.NewRuntime(
		runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) { return engine, nil }),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), runstate.GameBinding{Workspace: "/book", StoryID: "story", BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), runstate.RemoveCompaction{ID: "remove-1", Ref: runstate.ContextCompactionRef{
		SpecRef: "spec-remove", Source: "story.context_compaction", Purpose: "restore canonical turn history",
		Resource: "story/main", ExpectedRevision: "head:cc-1", CompactionID: "cc-1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := <-engine.started
	if request.Snapshot.Kind != runstate.StructuralRemoveCompaction {
		t.Fatalf("structural kind = %q", request.Snapshot.Kind)
	}
	if _, err := harness.Submit(context.Background(), runstate.Abort{ID: "abort-remove", OperationID: receipt.OperationID, Reason: "caller canceled"}); err != nil {
		t.Fatal(err)
	}
	waitForOperationStatus(t, harness, receipt.OperationID, runstate.OperationAborted)
}

func TestCompactionRecoveryHonorsAcknowledgedCommitReceipt(t *testing.T) {
	store := runstate.NewMemoryJournalStore()
	var once sync.Once
	engine := &compactionEngine{onRun: func(request runstate.StructuralEngineRequest, emit runstate.EngineEventSink) error {
		identity := runstate.DomainCommitIdentity{
			CommandID: request.Snapshot.CommandID, OperationID: request.Snapshot.OperationID,
			Cycle: request.Snapshot.Cycle, Stage: runstate.DomainCommitOutput,
		}
		if err := emit(runstate.EngineDomainCommitIntent{Identity: identity, Hash: "sha256:compaction"}); err != nil {
			return err
		}
		if err := emit(runstate.EngineDomainCommitReceipt{Identity: identity, Hash: "sha256:compaction", Revision: "context:8"}); err != nil {
			return err
		}
		once.Do(func() {})
		return nil
	}}
	runtime, err := runstate.NewRuntime(
		runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) { return engine, nil }), store, runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), runstate.CompactIfNeeded{ID: "compact-receipt", Ref: runstate.ContextCompactionRef{
		SpecRef: "spec-receipt", Source: "session.effective_messages", Purpose: "checkpoint", Resource: "session", ExpectedRevision: "context:7",
	}})
	if err != nil {
		t.Fatal(err)
	}
	waitForOperationStatus(t, harness, receipt.OperationID, runstate.OperationSucceeded)
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LastDomainCommit == nil || status.LastDomainCommit.Revision != "context:8" {
		t.Fatalf("missing structural commit receipt: %#v", status)
	}
}

func TestCompactionCrashRecoverySettlesAcknowledgedCanonicalWrite(t *testing.T) {
	store := runstate.NewMemoryJournalStore()
	ref := runstate.BindingRef{
		Kind: runstate.BindingWriting, Profile: runstate.ProfileWriting,
		Workspace: "/book", SessionID: "compaction-crash",
	}
	key, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(key))
	if err != nil {
		t.Fatal(err)
	}
	structural := runstate.StructuralOperationSnapshot{
		Binding: ref, CommandID: "compact-crash", OperationID: "operation-crash", Cycle: 1,
		Kind: runstate.StructuralCompactContext,
		Ref: runstate.ContextCompactionRef{
			SpecRef: "spec-crash", Source: "session.effective_messages", Purpose: "checkpoint",
			Resource: "compaction-crash", ExpectedRevision: "context:7",
		},
		ContextCursor: 2,
	}
	identity := runstate.DomainCommitIdentity{
		CommandID: structural.CommandID, OperationID: structural.OperationID, Cycle: 1, Stage: runstate.DomainCommitOutput,
	}
	if _, err := journal.Append(context.Background(), 0, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: structural.CommandID, CommandKind: "compact_context", OperationID: structural.OperationID, Fingerprint: "seed"},
		runstate.OperationStartedEvent{OperationID: structural.OperationID, Phase: runstate.PhaseCompacting, Structural: &structural},
		runstate.DomainCommitIntentAcceptedEvent{Identity: identity, Hash: "sha256:checkpoint"},
		runstate.DomainCommitReceiptEvent{Identity: identity, Hash: "sha256:checkpoint", Revision: "context:8"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	engine := &compactionEngine{}
	runtime, err := runstate.NewRuntime(
		runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) { return engine, nil }), store, runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "compaction-crash"})
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseIdle || status.LastOperation == nil || status.LastOperation.Status != runstate.OperationSucceeded {
		t.Fatalf("acknowledged structural write was not recovered as success: %#v", status)
	}
	if status.LastDomainCommit == nil || status.LastDomainCommit.Revision != "context:8" {
		t.Fatalf("recovered receipt missing: %#v", status)
	}
}

func waitForOperationStatus(t *testing.T, harness *runstate.Harness, operationID runstate.OperationID, want runstate.OperationStatus) {
	t.Helper()
	observation, err := harness.Observe(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Snapshot.LastOperation != nil && observation.Snapshot.LastOperation.OperationID == operationID {
		if observation.Snapshot.LastOperation.Status != want {
			t.Fatalf("operation status = %s, want %s", observation.Snapshot.LastOperation.Status, want)
		}
		return
	}
	for event := range observation.Events {
		if settled, ok := event.Payload.(runstate.OperationSettledEvent); ok && settled.OperationID == operationID {
			if settled.Status != want {
				t.Fatalf("operation status = %s, want %s", settled.Status, want)
			}
			return
		}
	}
	t.Fatal("observation closed before operation settled")
}
