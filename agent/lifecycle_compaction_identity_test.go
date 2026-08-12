package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type identityCompactionManager struct {
	kind             string
	mu               sync.Mutex
	sawExactSnapshot bool
	cacheKey         string
	leadingRole      RoleType
	stablePrefix     int
}

func (manager *identityCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.identity-test." + manager.kind, Version: 1}
}

func (*identityCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (*identityCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if !request.Force || len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, SourceFrom: 0, SourceTo: 2,
		Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20}}, nil
}

func (manager *identityCompactionManager) Compact(_ context.Context, request CompactionCompactRequest) (CompactionCheckpoint, error) {
	sawMarker := false
	var leadingRole RoleType
	stablePrefix := 0
	cacheKey := ""
	if request.ModelSnapshot != nil {
		cacheKey = request.ModelSnapshot.ResolvedOptions().SessionKey
		stablePrefix = request.ModelSnapshot.StablePrefixMessages()
		for _, message := range request.ModelSnapshot.Messages() {
			if message != nil && message.Content == "FINAL_MIDDLEWARE_CONTEXT" {
				sawMarker = true
			}
			if message != nil && strings.Contains(message.Content, "stable user context") {
				leadingRole = message.Role
			}
		}
	}
	manager.mu.Lock()
	manager.sawExactSnapshot = sawMarker
	manager.cacheKey = cacheKey
	manager.leadingRole = leadingRole
	manager.stablePrefix = stablePrefix
	manager.mu.Unlock()
	return CompactionCheckpoint{Summary: "summary from " + manager.kind, TokenEstimate: 4}, nil
}

func (manager *identityCompactionManager) sawSnapshot() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.sawExactSnapshot
}

func (manager *identityCompactionManager) snapshotCacheKey() string {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.cacheKey
}

func (manager *identityCompactionManager) snapshotPrefix() (RoleType, int) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.leadingRole, manager.stablePrefix
}

func TestManualCompactionSelectsCurrentDefinitionAndCapturesExactModelRequest(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("answer one", nil), AssistantMessage("answer two", nil),
	}}
	firstManager := &identityCompactionManager{kind: "first"}
	currentManager := firstManager
	source := SourceFunc(func(context.Context, PrepareRequest) (Definition, error) {
		return Definition{
			Key: "dynamic-compaction", Model: model,
			Instructions: "current instructions", Context: userLeadingPrefixSource{}, Compaction: currentManager,
			Middlewares: []Middleware{&appendFinalContextMiddleware{}},
		}, nil
	})
	owner, err := New(context.Background(), source, WithCacheKeyGenerator(func(SessionKey) (string, error) {
		return "manual-cache-key", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("dynamic-manual-compaction"))
	if err != nil {
		t.Fatal(err)
	}
	for index := range 2 {
		run, runErr := session.Run(context.Background(), Input{
			Text: strings.Repeat(fmt.Sprintf("question %d with context ", index+1), 80), IdempotencyKey: fmt.Sprintf("dynamic-turn-%d", index),
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("run=%#v err=%v", result, waitErr)
		}
	}

	secondManager := &identityCompactionManager{kind: "second"}
	currentManager = secondManager
	result, err := session.Compact(context.Background(), CompactionRequest{
		Force: true, IdempotencyKey: "dynamic-current-definition",
	})
	if err != nil || !result.Changed || result.State.Summary != "summary from second" {
		t.Fatalf("manual Compaction=%#v err=%v", result, err)
	}
	if !secondManager.sawSnapshot() || firstManager.sawSnapshot() {
		t.Fatalf("exact snapshot current=%v previous=%v", secondManager.sawSnapshot(), firstManager.sawSnapshot())
	}
	if secondManager.snapshotCacheKey() != "manual-cache-key" {
		t.Fatalf("manual Compaction cache key=%q", secondManager.snapshotCacheKey())
	}
	if role, stable := secondManager.snapshotPrefix(); role != User || stable != 2 {
		t.Fatalf("manual Compaction stable leading fragment role=%q boundary=%d, want user/2", role, stable)
	}
}

func TestManualCompactionCannotRaceAnActiveRun(t *testing.T) {
	model := newGatedLifecycleModel()
	manager := &identityCompactionManager{kind: "active-run-fence"}
	owner, err := New(context.Background(), Definition{
		Key: "manual-compaction-active-run-fence", Model: model,
		ModelIdentity: CapabilityIdentity{Kind: "model.test.manual-compaction-fence", Version: 1},
		Compaction:    manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("manual-compaction-active-run-fence"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "block provider", IdempotencyKey: "active-run"})
	if err != nil {
		t.Fatal(err)
	}
	<-model.calls
	if _, err := session.Compact(context.Background(), CompactionRequest{
		Force: true, IdempotencyKey: "must-not-race",
	}); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Compact error = %v, want ErrSessionBusy", err)
	}
	snapshot, err := session.Snapshot(context.Background())
	if err != nil || snapshot.Compaction != nil {
		t.Fatalf("racing Compact mutated Session: snapshot=%#v err=%v", snapshot, err)
	}
	if _, err := run.Abort(context.Background(), AbortRequest{Reason: "finish active-run fence test"}); err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultAborted {
		t.Fatalf("aborted Run=%#v err=%v", result, waitErr)
	}
}

func TestStructuralCompactionRecoveryRejectsCommandOwnedIdentityMismatch(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("answer", nil)}}
	currentManager := &identityCompactionManager{kind: "admitted"}
	source := SourceFunc(func(context.Context, PrepareRequest) (Definition, error) {
		return Definition{Key: "structural-recovery", Model: model, Compaction: currentManager}, nil
	})
	owner, err := New(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("structural-recovery"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "seed", IdempotencyKey: "structural-seed"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("seed=%#v err=%v", result, waitErr)
	}

	commandID := runstate.CommandID("frozen-command")
	preparation, err := session.prepareStructuralDefinition(context.Background(), commandID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepareStructuralCompactionSnapshot(
		context.Background(), preparation.prepared,
		SessionView{Key: session.key, Revision: uint64(preparation.checkpoint.Cursor)},
		structuralDefinitionRun(commandID), preparation.transcript.Messages,
		preparation.cleanup, preparation.cleanupPresent, preparation.compaction, preparation.compactionPresent,
	)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := modelRequestSnapshotFingerprint(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	request := CompactionRequest{Force: true, IdempotencyKey: string(commandID)}
	envelope, err := json.Marshal(compactionCommandEnvelope{
		Version:       compactionCommandVersion,
		DefinitionKey: preparation.prepared.definitionKey, RestoreKey: preparation.prepared.restoreKey,
		MaterializedFingerprint: preparation.prepared.materializedFingerprint,
		ModelRequestFingerprint: fingerprint,
		Manager:                 preparation.prepared.definition.Compaction.Identity(), Compact: &request,
	})
	if err != nil {
		t.Fatal(err)
	}
	currentManager = &identityCompactionManager{kind: "changed-after-admission"}
	engine := &definitionEngine{source: source, key: session.key}
	_, err = engine.RunStructural(context.Background(), runstate.StructuralEngineRequest{
		Snapshot: runstate.StructuralOperationSnapshot{
			CommandID: commandID, OperationID: "frozen-operation", Cycle: 1,
			Kind: runstate.StructuralCompactContext, ContextCursor: preparation.checkpoint.Cursor,
			Ref: runstate.ContextCompactionRef{RestoreDescriptor: envelope},
		},
		State: preparation.checkpoint.State, Capabilities: preparation.checkpoint.Capabilities,
	}, func(runstate.EngineEvent) error { return nil })
	if !errors.Is(err, ErrDefinitionMismatch) {
		t.Fatalf("RunStructural error=%v, want command-owned Definition mismatch", err)
	}
}

func TestCompactionClearCreatesVisibleNewGenerationAndSurvivesReopen(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := CapabilityIdentity{Kind: "model.compaction-clear-test", Version: 1}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("one", nil), AssistantMessage("two", nil), AssistantMessage("three", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Key: "compaction-clear-test", Model: model, ModelIdentity: identity, Compaction: fixedCompactionManager{},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), NamedSession("compaction-clear"))
	if err != nil {
		t.Fatal(err)
	}
	for generation := 1; generation <= 3; generation++ {
		if generation > 1 {
			if clearErr := session.Clear(context.Background()); clearErr != nil {
				t.Fatal(clearErr)
			}
			if hidden, present, stateErr := session.compactionState(context.Background()); stateErr != nil || present {
				t.Fatalf("generation %d Clear left Compaction visible: %#v present=%v err=%v", generation, hidden, present, stateErr)
			}
		}
		run, runErr := session.Run(context.Background(), Input{
			Text: strings.Repeat(fmt.Sprintf("generation %d context ", generation), 80), IdempotencyKey: fmt.Sprintf("compaction-generation-turn-%d", generation),
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("generation %d run=%#v err=%v", generation, result, waitErr)
		}
		compacted, compactErr := session.Compact(context.Background(), CompactionRequest{
			Force: true, IdempotencyKey: fmt.Sprintf("compaction-generation-command-%d", generation),
		})
		if compactErr != nil || !compacted.Changed || compacted.State.Revision != uint64(generation) {
			t.Fatalf("generation %d Compaction=%#v err=%v", generation, compacted, compactErr)
		}
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(context.Background(), Definition{
		Key: "compaction-clear-test", Model: &lifecycleModel{}, ModelIdentity: identity, Compaction: fixedCompactionManager{},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close(context.Background()) })
	reopenedSession, err := reopened.Session(context.Background(), NamedSession("compaction-clear"))
	if err != nil {
		t.Fatal(err)
	}
	state, present, err := reopenedSession.compactionState(context.Background())
	if err != nil || !present || state.Revision != 3 || !strings.Contains(state.Summary, "first turn") {
		t.Fatalf("reopened Compaction=%#v present=%v err=%v", state, present, err)
	}
}
