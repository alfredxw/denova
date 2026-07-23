package agentruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"denova/internal/agentruntime"
)

type compactionEngine struct {
	started chan agentruntime.StructuralEngineRequest
	release chan struct{}
	onRun   func(agentruntime.StructuralEngineRequest, agentruntime.EngineEventSink) error
}

func (e *compactionEngine) Run(context.Context, agentruntime.EngineRequest, agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
	return agentruntime.EngineResult{}, errors.New("turn engine must not run for a compaction command")
}

func (e *compactionEngine) RunStructural(ctx context.Context, request agentruntime.StructuralEngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
	if e.started != nil {
		e.started <- request
	}
	if e.onRun != nil {
		if err := e.onRun(request, emit); err != nil {
			return agentruntime.EngineResult{}, err
		}
	}
	if e.release != nil {
		select {
		case <-e.release:
		case <-ctx.Done():
			return agentruntime.EngineResult{Status: agentruntime.EngineAborted}, nil
		}
	}
	return agentruntime.EngineResult{Status: agentruntime.EngineCompleted}, nil
}

func TestCompactIfNeededIsACompactingDurableOperation(t *testing.T) {
	engine := &compactionEngine{started: make(chan agentruntime.StructuralEngineRequest, 1), release: make(chan struct{})}
	runtime, err := agentruntime.NewRuntime(
		agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) { return engine, nil }),
		agentruntime.NewMemoryJournalStore(),
		agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	command := agentruntime.CompactIfNeeded{ID: "compact-1", Ref: agentruntime.ContextCompactionRef{
		SpecRef: "spec-1", Source: "session.effective_messages", Purpose: "bounded model history checkpoint",
		Resource: "session", ExpectedRevision: "context:7", Force: true,
	}}
	receipt, err := harness.Submit(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	request := <-engine.started
	if request.Snapshot.Kind != agentruntime.StructuralCompactContext || !reflect.DeepEqual(request.Snapshot.Ref, command.Ref) {
		t.Fatalf("unexpected structural request: %#v", request)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentruntime.PhaseCompacting || status.ActiveOperation != receipt.OperationID {
		t.Fatalf("status while compacting = %#v", status)
	}
	close(engine.release)
	waitForOperationStatus(t, harness, receipt.OperationID, agentruntime.OperationSucceeded)
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
			engine := &compactionEngine{started: make(chan agentruntime.StructuralEngineRequest, 1)}
			runtime, err := agentruntime.NewRuntime(
				agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) { return engine, nil }),
				agentruntime.NewMemoryJournalStore(),
				agentruntime.RuntimeConfig{InputLimits: agentruntime.InputLimits{MaxRestoreDescriptorBytes: 8}},
			)
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close(context.Background())
			harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "descriptor-" + name})
			if err != nil {
				t.Fatal(err)
			}
			_, err = harness.Submit(context.Background(), agentruntime.CompactIfNeeded{ID: agentruntime.CommandID("compact-" + name), Ref: agentruntime.ContextCompactionRef{
				SpecRef: "spec-" + name, Source: "session.effective_messages", Purpose: "checkpoint",
				Resource: "descriptor-" + name, ExpectedRevision: "context:1", RestoreDescriptor: descriptor,
			}})
			if !errors.Is(err, agentruntime.ErrInvalidCommand) {
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
	engine := &compactionEngine{started: make(chan agentruntime.StructuralEngineRequest, 1), release: make(chan struct{})}
	runtime, err := agentruntime.NewRuntime(
		agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) { return engine, nil }),
		agentruntime.NewMemoryJournalStore(),
		agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "descriptor-clone"})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := json.RawMessage(`{"version":1,"mutation":{"id":"cc-1"}}`)
	command := agentruntime.CompactIfNeeded{ID: "compact-clone", Ref: agentruntime.ContextCompactionRef{
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
	if _, err := harness.Submit(context.Background(), conflict); !errors.Is(err, agentruntime.ErrInvalidCommand) {
		t.Fatalf("changed descriptor replay error = %v, want ErrInvalidCommand", err)
	}
	close(engine.release)
	waitForOperationStatus(t, harness, receipt.OperationID, agentruntime.OperationSucceeded)
}

func TestRemoveCompactionAbortSettlesWithoutCanonicalCommit(t *testing.T) {
	engine := &compactionEngine{started: make(chan agentruntime.StructuralEngineRequest, 1), release: make(chan struct{})}
	runtime, err := agentruntime.NewRuntime(
		agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) { return engine, nil }),
		agentruntime.NewMemoryJournalStore(),
		agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), agentruntime.GameBinding{Workspace: "/book", StoryID: "story", BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), agentruntime.RemoveCompaction{ID: "remove-1", Ref: agentruntime.ContextCompactionRef{
		SpecRef: "spec-remove", Source: "story.context_compaction", Purpose: "restore canonical turn history",
		Resource: "story/main", ExpectedRevision: "head:cc-1", CompactionID: "cc-1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := <-engine.started
	if request.Snapshot.Kind != agentruntime.StructuralRemoveCompaction {
		t.Fatalf("structural kind = %q", request.Snapshot.Kind)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.Abort{ID: "abort-remove", OperationID: receipt.OperationID, Reason: "caller canceled"}); err != nil {
		t.Fatal(err)
	}
	waitForOperationStatus(t, harness, receipt.OperationID, agentruntime.OperationAborted)
}

func TestCompactionRecoveryHonorsAcknowledgedCommitReceipt(t *testing.T) {
	store := agentruntime.NewMemoryJournalStore()
	var once sync.Once
	engine := &compactionEngine{onRun: func(request agentruntime.StructuralEngineRequest, emit agentruntime.EngineEventSink) error {
		identity := agentruntime.DomainCommitIdentity{
			CommandID: request.Snapshot.CommandID, OperationID: request.Snapshot.OperationID,
			Cycle: request.Snapshot.Cycle, Stage: agentruntime.DomainCommitOutput,
		}
		if err := emit(agentruntime.EngineDomainCommitIntent{Identity: identity, Hash: "sha256:compaction"}); err != nil {
			return err
		}
		if err := emit(agentruntime.EngineDomainCommitReceipt{Identity: identity, Hash: "sha256:compaction", Revision: "context:8"}); err != nil {
			return err
		}
		once.Do(func() {})
		return nil
	}}
	runtime, err := agentruntime.NewRuntime(
		agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) { return engine, nil }), store, agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), agentruntime.CompactIfNeeded{ID: "compact-receipt", Ref: agentruntime.ContextCompactionRef{
		SpecRef: "spec-receipt", Source: "session.effective_messages", Purpose: "checkpoint", Resource: "session", ExpectedRevision: "context:7",
	}})
	if err != nil {
		t.Fatal(err)
	}
	waitForOperationStatus(t, harness, receipt.OperationID, agentruntime.OperationSucceeded)
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LastDomainCommit == nil || status.LastDomainCommit.Revision != "context:8" {
		t.Fatalf("missing structural commit receipt: %#v", status)
	}
}

func TestCompactionCrashRecoverySettlesAcknowledgedCanonicalWrite(t *testing.T) {
	store := agentruntime.NewMemoryJournalStore()
	ref := agentruntime.BindingRef{
		Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileWriting,
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
	structural := agentruntime.StructuralOperationSnapshot{
		Binding: ref, CommandID: "compact-crash", OperationID: "operation-crash", Cycle: 1,
		Kind: agentruntime.StructuralCompactContext,
		Ref: agentruntime.ContextCompactionRef{
			SpecRef: "spec-crash", Source: "session.effective_messages", Purpose: "checkpoint",
			Resource: "compaction-crash", ExpectedRevision: "context:7",
		},
		ContextCursor: 2,
	}
	identity := agentruntime.DomainCommitIdentity{
		CommandID: structural.CommandID, OperationID: structural.OperationID, Cycle: 1, Stage: agentruntime.DomainCommitOutput,
	}
	if _, err := journal.Append(context.Background(), 0, []agentruntime.EventPayload{
		agentruntime.CommandAcceptedEvent{CommandID: structural.CommandID, CommandKind: "compact_context", OperationID: structural.OperationID, Fingerprint: "seed"},
		agentruntime.OperationStartedEvent{OperationID: structural.OperationID, Phase: agentruntime.PhaseCompacting, Structural: &structural},
		agentruntime.DomainCommitIntentAcceptedEvent{Identity: identity, Hash: "sha256:checkpoint"},
		agentruntime.DomainCommitReceiptEvent{Identity: identity, Hash: "sha256:checkpoint", Revision: "context:8"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	engine := &compactionEngine{}
	runtime, err := agentruntime.NewRuntime(
		agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) { return engine, nil }), store, agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "compaction-crash"})
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentruntime.PhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentruntime.OperationSucceeded {
		t.Fatalf("acknowledged structural write was not recovered as success: %#v", status)
	}
	if status.LastDomainCommit == nil || status.LastDomainCommit.Revision != "context:8" {
		t.Fatalf("recovered receipt missing: %#v", status)
	}
}

func waitForOperationStatus(t *testing.T, harness *agentruntime.Harness, operationID agentruntime.OperationID, want agentruntime.OperationStatus) {
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
		if settled, ok := event.Payload.(agentruntime.OperationSettledEvent); ok && settled.OperationID == operationID {
			if settled.Status != want {
				t.Fatalf("operation status = %s, want %s", settled.Status, want)
			}
			return
		}
	}
	t.Fatal("observation closed before operation settled")
}
