package automation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSemanticTriggerEvaluationPersistsClaimDecisionAndCompletion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	task, err := store.Create(TaskDefinition{
		Scope:    ScopeWorkspace,
		Enabled:  true,
		Name:     "Durable semantic trigger",
		Template: TemplateReview,
		Triggers: []TriggerDefinition{{
			ID:                "semantic_1",
			Type:              TriggerTypeSemantic,
			Enabled:           true,
			SemanticCondition: "a character changed allegiance",
		}},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	now := time.Date(2026, 7, 22, 8, 30, 0, 0, time.UTC)
	claim, err := NewSemanticTriggerEvaluation(SemanticTriggerIntent{
		Scope:                  ScopeWorkspace,
		Workspace:              workspace,
		TaskID:                 task.ID,
		TriggerID:              "semantic_1",
		Condition:              "a character changed allegiance",
		ObservationFingerprint: "batch-1",
		Instruction:            "bounded semantic instruction",
		Context: TriggerContext{Source: "test", Evidence: []TriggerEvidence{{
			Source: "chapter", Ref: "chapters/ch01.md", Snippet: "A joined B.",
		}}},
		ActionPolicy: ActionPolicyAutoRun,
		NotifyPolicy: NotifyPolicyInbox,
	}, now)
	if err != nil {
		t.Fatalf("NewSemanticTriggerEvaluation failed: %v", err)
	}

	persisted, disposition, err := store.ClaimTriggerEvaluation(context.Background(), task.ID, "semantic_1", claim)
	if err != nil {
		t.Fatalf("ClaimTriggerEvaluation failed: %v", err)
	}
	if disposition != TriggerEvaluationClaimed || persisted.Status != TriggerEvaluationStatusClaimed {
		t.Fatalf("claim disposition/status = %q/%q", disposition, persisted.Status)
	}

	// A new Store models a process restart. The claimed prompt and bounded
	// context must be available before the evaluator is called again.
	restarted := NewStore(filepath.Join(root, "user"), workspace)
	replayed, disposition, err := restarted.ClaimTriggerEvaluation(context.Background(), task.ID, "semantic_1", claim)
	if err != nil {
		t.Fatalf("replay ClaimTriggerEvaluation failed: %v", err)
	}
	if disposition != TriggerEvaluationResumed || replayed.Instruction != claim.Instruction || replayed.IntentHash != claim.IntentHash {
		t.Fatalf("replayed claim lost durable input: disposition=%q record=%#v", disposition, replayed)
	}

	match := TriggerMatch{
		TaskID:      task.ID,
		TriggerID:   "semantic_1",
		Title:       "Allegiance changed",
		Summary:     "The bounded chapter evidence shows the change.",
		Fingerprint: "match-1",
		Evidence:    claim.Context.Evidence,
	}
	decided, err := restarted.DecideTriggerEvaluation(
		context.Background(), task.ID, "semantic_1", claim.ID, claim.IntentHash,
		SemanticEvaluation{Matched: true, Confidence: 0.91, Reason: "explicit change"}, &match, now.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("DecideTriggerEvaluation failed: %v", err)
	}
	if decided.Status != TriggerEvaluationStatusDecided || decided.Action == nil {
		t.Fatalf("decided record = %#v", decided)
	}
	if decided.Action.InboxID == "" || decided.Action.RunID == "" {
		t.Fatalf("auto-run + inbox action lacks stable identities: %#v", decided.Action)
	}

	completed, err := restarted.CompleteTriggerEvaluation(context.Background(), task.ID, "semantic_1", claim.ID, claim.IntentHash, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("CompleteTriggerEvaluation failed: %v", err)
	}
	if completed.Status != TriggerEvaluationStatusCompleted {
		t.Fatalf("completed status = %q", completed.Status)
	}
	saved, err := restarted.Get(task.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	state := saved.TriggerState["semantic_1"]
	if state.LastObservationFingerprint != claim.ObservationFingerprint || state.LastEvidenceFingerprint != match.Fingerprint || state.LastMatchedAt.IsZero() {
		t.Fatalf("completed trigger state = %#v", state)
	}

	exact, disposition, err := restarted.ClaimTriggerEvaluation(context.Background(), task.ID, "semantic_1", claim)
	if err != nil {
		t.Fatalf("completed replay failed: %v", err)
	}
	if disposition != TriggerEvaluationReplayed || exact.Status != TriggerEvaluationStatusCompleted {
		t.Fatalf("completed replay disposition/status = %q/%q", disposition, exact.Status)
	}
}

func TestSemanticTriggerEvaluationRejectsConflictingReplay(t *testing.T) {
	t.Parallel()

	store, task := newTriggerEvaluationTestStore(t)
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	claim := mustSemanticTriggerEvaluation(t, task, now)
	if _, _, err := store.ClaimTriggerEvaluation(context.Background(), task.ID, "semantic_1", claim); err != nil {
		t.Fatalf("initial claim failed: %v", err)
	}
	conflict := claim
	conflict.IntentHash = EvidenceFingerprint("different bounded semantics")
	if _, _, err := store.ClaimTriggerEvaluation(context.Background(), task.ID, "semantic_1", conflict); !errors.Is(err, ErrTriggerEvaluationConflict) {
		t.Fatalf("conflicting claim error = %v, want ErrTriggerEvaluationConflict", err)
	}
	if _, err := store.DecideTriggerEvaluation(context.Background(), task.ID, "semantic_1", claim.ID, "wrong-intent", SemanticEvaluation{}, nil, now); !errors.Is(err, ErrTriggerEvaluationConflict) {
		t.Fatalf("conflicting decision error = %v, want ErrTriggerEvaluationConflict", err)
	}
}

func TestSemanticTriggerEvaluationConcurrentClaimIsCASProtected(t *testing.T) {
	t.Parallel()

	store, task := newTriggerEvaluationTestStore(t)
	claim := mustSemanticTriggerEvaluation(t, task, time.Date(2026, 7, 22, 9, 30, 0, 0, time.UTC))
	stores := []*Store{
		NewStore(store.userDir, store.workspace),
		NewStore(store.userDir, store.workspace),
	}

	var wg sync.WaitGroup
	dispositions := make(chan TriggerEvaluationClaimDisposition, len(stores))
	errs := make(chan error, len(stores))
	for _, candidate := range stores {
		wg.Add(1)
		go func(candidate *Store) {
			defer wg.Done()
			var disposition TriggerEvaluationClaimDisposition
			var claimErr error
			defer func() {
				if recovered := recover(); recovered != nil {
					claimErr = fmt.Errorf("concurrent trigger claim panic: %v", recovered)
				}
				dispositions <- disposition
				errs <- claimErr
			}()
			_, disposition, claimErr = candidate.ClaimTriggerEvaluation(context.Background(), task.ID, "semantic_1", claim)
		}(candidate)
	}
	wg.Wait()
	close(dispositions)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent claim failed: %v", err)
		}
	}
	counts := map[TriggerEvaluationClaimDisposition]int{}
	for disposition := range dispositions {
		counts[disposition]++
	}
	if counts[TriggerEvaluationClaimed] != 1 || counts[TriggerEvaluationResumed] != 1 {
		t.Fatalf("claim dispositions = %#v, want one claim and one resume", counts)
	}
}

func TestTriggerActionIdentityIsDeterministicAndSilentAutoRunHasNoInbox(t *testing.T) {
	t.Parallel()

	claim := TriggerEvaluationRecord{ID: "trigger-eval-stable", IntentHash: "intent-stable", ActionPolicy: ActionPolicyAutoRun, NotifyPolicy: NotifyPolicySilent}
	match := TriggerMatch{TaskID: "task-1", TriggerID: "semantic-1", Fingerprint: "match-1"}
	first, err := NewTriggerActionPlan(claim, match)
	if err != nil {
		t.Fatalf("NewTriggerActionPlan failed: %v", err)
	}
	second, err := NewTriggerActionPlan(claim, match)
	if err != nil {
		t.Fatalf("NewTriggerActionPlan replay failed: %v", err)
	}
	if first != second || first.ID == "" || first.RunID == "" {
		t.Fatalf("action identity is not deterministic: first=%#v second=%#v", first, second)
	}
	if first.InboxID != "" {
		t.Fatalf("silent auto-run must not allocate inbox identity: %#v", first)
	}
}

func TestMatchedTriggerEvaluationResumesClaimedCandidateAndCompletes(t *testing.T) {
	t.Parallel()

	store, task := newTriggerEvaluationTestStore(t)
	now := time.Date(2026, 7, 22, 9, 45, 0, 0, time.UTC)
	match := TriggerMatch{
		TaskID: task.ID, TriggerID: "schedule", Title: "Scheduled review",
		Summary: "The hourly schedule is due.", Fingerprint: "schedule-hour-10",
		Evidence: []TriggerEvidence{{Source: "schedule", Title: "hourly", Ref: "schedule:hourly"}},
	}
	claim, err := NewMatchedTriggerEvaluation(MatchedTriggerIntent{
		Scope: task.Scope, Workspace: task.Target.Workspace, TaskID: task.ID,
		TriggerID: "schedule", TriggerType: TriggerTypeSchedule, Match: match,
		ActionPolicy: ActionPolicyAutoRun, NotifyPolicy: NotifyPolicySilent,
	}, now)
	if err != nil {
		t.Fatalf("NewMatchedTriggerEvaluation failed: %v", err)
	}
	if _, disposition, err := store.ClaimTriggerEvaluation(context.Background(), task.ID, "schedule", claim); err != nil || disposition != TriggerEvaluationClaimed {
		t.Fatalf("claim disposition=%q err=%v", disposition, err)
	}

	restarted := NewStore(store.userDir, store.workspace)
	resumed, disposition, err := restarted.ClaimTriggerEvaluation(context.Background(), task.ID, "schedule", claim)
	if err != nil || disposition != TriggerEvaluationResumed || resumed.CandidateMatch == nil || resumed.CandidateMatch.Fingerprint != match.Fingerprint {
		t.Fatalf("resumed deterministic claim=%#v disposition=%q err=%v", resumed, disposition, err)
	}
	decision, persistedMatch, err := DeterministicTriggerDecision(resumed)
	if err != nil {
		t.Fatalf("DeterministicTriggerDecision failed: %v", err)
	}
	decided, err := restarted.DecideTriggerEvaluation(context.Background(), task.ID, "schedule", resumed.ID, resumed.IntentHash, decision, &persistedMatch, now.Add(time.Second))
	if err != nil {
		t.Fatalf("DecideTriggerEvaluation failed: %v", err)
	}
	if decided.Action == nil || decided.Action.RunID == "" || decided.Action.InboxID != "" {
		t.Fatalf("silent schedule action plan=%#v", decided.Action)
	}
	completed, err := restarted.CompleteTriggerEvaluation(context.Background(), task.ID, "schedule", resumed.ID, resumed.IntentHash, now.Add(2*time.Second))
	if err != nil || completed.Status != TriggerEvaluationStatusCompleted {
		t.Fatalf("completion=%#v err=%v", completed, err)
	}
}

func TestEnsureInboxItemIsIdempotentAcrossRestart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	want := TriggerInboxItem{
		ID: "inbox-deterministic", TaskID: "task-1", TriggerID: "semantic-1",
		Scope: ScopeWorkspace, Workspace: workspace, Status: InboxStatusPending,
		ActionPolicy: ActionPolicyAutoRun, NotifyPolicy: NotifyPolicyInbox,
		Title: "Matched", Summary: "Bounded evidence matched", Fingerprint: "match-1",
		CreatedAt: now, UpdatedAt: now,
	}
	first, created, err := store.EnsureInboxItem(context.Background(), want)
	if err != nil || !created {
		t.Fatalf("first EnsureInboxItem = %#v created=%v err=%v", first, created, err)
	}
	restarted := NewStore(filepath.Join(root, "user"), workspace)
	second, created, err := restarted.EnsureInboxItem(context.Background(), want)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("replayed EnsureInboxItem = %#v created=%v err=%v", second, created, err)
	}
	items, err := restarted.ListInbox()
	if err != nil {
		t.Fatalf("ListInbox failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("inbox item count = %d, want 1", len(items))
	}

	conflict := want
	conflict.TaskID = "different-task"
	if _, _, err := restarted.EnsureInboxItem(context.Background(), conflict); !errors.Is(err, ErrTriggerActionConflict) {
		t.Fatalf("conflicting inbox replay error = %v, want ErrTriggerActionConflict", err)
	}
}

func TestInboxConfirmationRunClaimSurvivesStartBeforeCompletion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	item, err := store.CreateInboxItem(TriggerInboxItem{
		TaskID: "task-confirm", TriggerID: "chapter-1", Scope: ScopeWorkspace, Workspace: workspace,
		Status: InboxStatusPending, ActionPolicy: ActionPolicyConfirm, NotifyPolicy: NotifyPolicyInbox,
		Title: "Confirm", Summary: "Run once", Fingerprint: "confirm-fingerprint",
	})
	if err != nil {
		t.Fatalf("CreateInboxItem failed: %v", err)
	}
	runID, err := InboxConfirmationRunID(item)
	if err != nil {
		t.Fatalf("InboxConfirmationRunID failed: %v", err)
	}
	claimed, disposition, err := store.ClaimInboxRun(context.Background(), item.ID, runID)
	if err != nil || disposition != InboxRunClaimed || claimed.RunID != runID || claimed.Status != InboxStatusPending {
		t.Fatalf("claim=%#v disposition=%q err=%v", claimed, disposition, err)
	}

	// The process crashes after admitting the run but before confirming the
	// inbox. A fresh Store must resume the same claim, never allocate another ID.
	restarted := NewStore(filepath.Join(root, "user"), workspace)
	resumed, disposition, err := restarted.ClaimInboxRun(context.Background(), item.ID, runID)
	if err != nil || disposition != InboxRunResumed || resumed.RunID != runID {
		t.Fatalf("resume=%#v disposition=%q err=%v", resumed, disposition, err)
	}
	completed, err := restarted.CompleteInboxRun(context.Background(), item.ID, runID)
	if err != nil || completed.Status != InboxStatusConfirmed || completed.RunID != runID {
		t.Fatalf("complete=%#v err=%v", completed, err)
	}
	if _, _, err := restarted.ClaimInboxRun(context.Background(), item.ID, "different-run"); !errors.Is(err, ErrTriggerActionConflict) {
		t.Fatalf("conflicting confirmation error=%v", err)
	}
}

func TestInboxConfirmationClaimAndDismissHaveOneWinner(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	item, err := store.CreateInboxItem(TriggerInboxItem{
		TaskID: "task-race", TriggerID: "chapter-1", Scope: ScopeWorkspace, Workspace: workspace,
		Status: InboxStatusPending, ActionPolicy: ActionPolicyConfirm, NotifyPolicy: NotifyPolicyInbox,
		Title: "Confirm or dismiss", Summary: "One action wins", Fingerprint: "confirm-dismiss-race",
	})
	if err != nil {
		t.Fatal(err)
	}
	runID, err := InboxConfirmationRunID(item)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan string, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		if _, _, err := store.ClaimInboxRun(context.Background(), item.ID, runID); err == nil {
			results <- "claim"
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		if _, err := store.DismissInboxItem(item.ID); err == nil {
			results <- "dismiss"
		}
	}()
	close(start)
	wg.Wait()
	close(results)
	winners := make([]string, 0, 1)
	for winner := range results {
		winners = append(winners, winner)
	}
	if len(winners) != 1 {
		t.Fatalf("semantic winner count = %d (%v), want exactly one", len(winners), winners)
	}
	final, err := store.GetInboxItem(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	switch winners[0] {
	case "claim":
		if final.Status != InboxStatusPending || final.RunID != runID {
			t.Fatalf("claimed item became dismissible: %#v", final)
		}
		if _, err := store.CompleteInboxRun(context.Background(), item.ID, runID); err != nil {
			t.Fatalf("claimed winner could not complete: %v", err)
		}
	case "dismiss":
		if final.Status != InboxStatusDismissed || final.RunID != "" {
			t.Fatalf("dismissed item retained a runnable claim: %#v", final)
		}
	default:
		t.Fatalf("unknown winner %q", winners[0])
	}
}

func TestSemanticEvaluationBoundsAndEvidenceMembership(t *testing.T) {
	t.Parallel()

	if _, err := ParseSemanticEvaluation(`{"matched":true,"confidence":1.1,"reason":"bad","title":"bad","evidence_refs":[]}`); err == nil {
		t.Fatal("out-of-range confidence was accepted")
	}
	tooLong := `{"matched":true,"confidence":0.9,"reason":"` + strings.Repeat("x", MaxSemanticEvaluationReasonChars+1) + `","title":"bad","evidence_refs":[]}`
	if _, err := ParseSemanticEvaluation(tooLong); err == nil {
		t.Fatal("oversized decision reason was accepted")
	}
	evaluation, err := ParseSemanticEvaluation(`{"matched":true,"confidence":0.9,"reason":"ok","title":"hit","evidence_refs":["invented.md"]}`)
	if err != nil {
		t.Fatalf("ParseSemanticEvaluation failed: %v", err)
	}
	if _, err := ValidateSemanticEvaluationEvidence(evaluation, TriggerContext{Evidence: []TriggerEvidence{{Ref: "chapters/ch01.md"}}}); err == nil {
		t.Fatal("model evidence reference outside the bounded claim was accepted")
	}

	store, task := newTriggerEvaluationTestStore(t)
	_ = store
	_, err = NewSemanticTriggerEvaluation(SemanticTriggerIntent{
		Scope: task.Scope, Workspace: task.Target.Workspace, TaskID: task.ID, TriggerID: "semantic_1",
		Condition: "changed", ObservationFingerprint: "batch-1",
		Instruction:  strings.Repeat("i", MaxSemanticTriggerInstructionChars+1),
		Context:      TriggerContext{Evidence: []TriggerEvidence{{Ref: "chapters/ch01.md"}}},
		ActionPolicy: ActionPolicyAutoRun, NotifyPolicy: NotifyPolicyInbox,
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("oversized semantic instruction was accepted")
	}
}

func newTriggerEvaluationTestStore(t *testing.T) (*Store, Task) {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	task, err := store.Create(TaskDefinition{
		Scope: ScopeWorkspace, Enabled: true, Name: "Semantic", Template: TemplateReview,
		Triggers: []TriggerDefinition{{ID: "semantic_1", Type: TriggerTypeSemantic, Enabled: true, SemanticCondition: "changed"}},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	return store, task
}

func mustSemanticTriggerEvaluation(t *testing.T, task Task, now time.Time) TriggerEvaluationRecord {
	t.Helper()
	claim, err := NewSemanticTriggerEvaluation(SemanticTriggerIntent{
		Scope: task.Scope, Workspace: task.Target.Workspace, TaskID: task.ID, TriggerID: "semantic_1",
		Condition: "changed", ObservationFingerprint: "batch-1", Instruction: "bounded instruction",
		Context:      TriggerContext{Source: "test", Evidence: []TriggerEvidence{{Ref: "chapters/ch01.md"}}},
		ActionPolicy: ActionPolicyAutoRun, NotifyPolicy: NotifyPolicyInbox,
	}, now)
	if err != nil {
		t.Fatalf("NewSemanticTriggerEvaluation failed: %v", err)
	}
	return claim
}
