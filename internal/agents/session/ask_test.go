package session

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestAwaitAskAnswersAndCancelsWithoutInterruption(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("ask-answer")
	if err != nil {
		t.Fatal(err)
	}
	interaction := testAskInteraction("call-ask")
	result := make(chan AskResolution, 1)
	errs := make(chan error, 1)
	go func() {
		resolved, awaitErr := sess.AwaitAsk(context.Background(), interaction)
		result <- resolved
		errs <- awaitErr
	}()
	waitForPendingAsk(t, sess, interaction.ID)

	resolved, err := sess.ResolveAsk(context.Background(), interaction.ID, AskAnswered, []AskAnswer{
		{QuestionID: "format", SelectedOptionIDs: []string{"markdown"}},
		{QuestionID: "notes", CustomInput: "Keep the examples concise."},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != AskAnswered || len(resolved.Answers) != 2 || resolved.Answers[0].SelectedOptions[0].Label != "Markdown" {
		t.Fatalf("resolved ask = %#v", resolved)
	}
	select {
	case awaitErr := <-errs:
		if awaitErr != nil {
			t.Fatal(awaitErr)
		}
	case <-time.After(time.Second):
		t.Fatal("ask waiter did not resume")
	}
	if awaited := <-result; !sameAskResolution(awaited, resolved) {
		t.Fatalf("awaited = %#v, resolved = %#v", awaited, resolved)
	}
	if sess.PendingAsk(interaction.ID) != nil || sess.PendingInterruption() != nil {
		t.Fatal("answer left pending interaction or fabricated an interruption")
	}
	// Every retry returns the canonical terminal resolution, even if a stale UI
	// submits a different action after another client already answered.
	if replay, replayErr := sess.ResolveAsk(context.Background(), interaction.ID, AskAnswered, []AskAnswer{
		{QuestionID: "format", SelectedOptionIDs: []string{"plain"}},
		{QuestionID: "notes", CustomInput: "A conflicting retry."},
	}, ""); replayErr != nil || !sameAskResolution(replay, resolved) {
		t.Fatalf("resolution replay = %#v err=%v", replay, replayErr)
	}
	if replay, replayErr := sess.ResolveAsk(context.Background(), interaction.ID, AskCancelled, nil, "late_cancel"); replayErr != nil || !sameAskResolution(replay, resolved) {
		t.Fatalf("cross-action replay = %#v err=%v", replay, replayErr)
	}
}

func TestResolveAskReturnsNotFoundForUnknownID(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("ask-not-found")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.ResolveAskFromHost(context.Background(), "missing", AskCancelled, nil, "user_cancelled"); !errors.Is(err, ErrAskNotFound) {
		t.Fatalf("unknown Ask error = %v, want %v", err, ErrAskNotFound)
	}
}

func TestAwaitAskResolutionWakesEveryAttachedWaiter(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("ask-multiple-waiters")
	if err != nil {
		t.Fatal(err)
	}
	interaction := testAskInteraction("call-shared")
	attached := make(chan struct{}, 2)
	type outcome struct {
		resolution AskResolution
		err        error
	}
	outcomes := make(chan outcome, 2)
	for range 2 {
		go func() {
			resolution, awaitErr := sess.AwaitAskWithPending(context.Background(), interaction, func(AskInteraction) {
				attached <- struct{}{}
			})
			outcomes <- outcome{resolution: resolution, err: awaitErr}
		}()
	}
	for range 2 {
		select {
		case <-attached:
		case <-time.After(time.Second):
			t.Fatal("ask waiter did not attach")
		}
	}
	want, err := sess.ResolveAsk(context.Background(), interaction.ID, AskCancelled, nil, "user_cancelled")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case got := <-outcomes:
			if got.err != nil || !sameAskResolution(got.resolution, want) {
				t.Fatalf("waiter outcome = %#v, want %#v", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("resolving one Ask did not wake every attached waiter")
		}
	}
}

func TestNormalizeAskInteractionTreatsInputAsReadOnly(t *testing.T) {
	interaction := testAskInteraction("call-read-only")
	interaction.Schema = "caller.schema"
	interaction.Status = AskAnswered
	interaction.Questions[0].ID = " format "
	interaction.Questions[0].Question = " Which format? "
	interaction.Questions[0].RecommendedOptionID = " markdown "
	interaction.Questions[0].Options[0].ID = " markdown "
	interaction.Questions[0].Options[0].Label = " Markdown "
	interaction.Questions[0].Options[0].Description = " Recommended "
	interaction.Answers = []AskAnswerResult{{
		QuestionID: "format",
		SelectedOptions: []AskSelectedOption{{
			ID:    "markdown",
			Label: "Markdown",
		}},
	}}
	resolvedAt := time.Now().UTC()
	interaction.ResolvedAt = &resolvedAt

	before, err := json.Marshal(interaction)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalizeAskInteraction(interaction)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(interaction)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("normalization mutated caller-owned input:\nbefore: %s\nafter:  %s", before, after)
	}
	if normalized.Questions[0].ID != "format" ||
		normalized.Questions[0].Options[0].ID != "markdown" ||
		normalized.Questions[0].Options[0].Label != "Markdown" {
		t.Fatalf("normalized interaction = %#v", normalized)
	}

	normalized.Questions[0].Options[0].Label = "changed"
	if interaction.Questions[0].Options[0].Label != " Markdown " {
		t.Fatal("normalized nested option still aliases caller-owned input")
	}
}

func TestAskValidationRejectsMalformedQuestionsAndAnswers(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("ask-validation")
	if err != nil {
		t.Fatal(err)
	}

	invalid := testAskInteraction("call-invalid")
	invalid.Questions[0].Options = []AskOption{{ID: "only", Label: "Only"}}
	if _, err := sess.AwaitAsk(context.Background(), invalid); err == nil {
		t.Fatal("ask accepted a one-option question")
	}

	interaction := testAskInteraction("call-valid")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, awaitErr := sess.AwaitAsk(ctx, interaction)
		done <- awaitErr
	}()
	waitForPendingAsk(t, sess, interaction.ID)
	if _, err := sess.ResolveAsk(context.Background(), interaction.ID, AskAnswered, []AskAnswer{
		{QuestionID: "format", SelectedOptionIDs: []string{"missing"}},
		{QuestionID: "notes", CustomInput: "ok"},
	}, ""); err == nil {
		t.Fatal("ask accepted an unknown option")
	}
	if sess.PendingAsk(interaction.ID) == nil {
		t.Fatal("invalid answer consumed the pending ask")
	}
	if _, err := sess.ResolveAsk(context.Background(), interaction.ID, AskCancelled, nil, "user_cancelled"); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled ask did not resume")
	}
}

func TestAwaitAskPublishesOnlyAfterDurablePendingHistoryExists(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("ask-publish-order")
	if err != nil {
		t.Fatal(err)
	}
	interaction := testAskInteraction("call-publish")
	published := make(chan AskInteraction, 1)
	done := make(chan error, 1)
	go func() {
		_, awaitErr := sess.AwaitAskWithPending(context.Background(), interaction, func(pending AskInteraction) {
			published <- pending
		})
		done <- awaitErr
	}()

	select {
	case pending := <-published:
		if pending.ID != interaction.ID || sess.PendingAsk(interaction.ID) == nil {
			t.Fatalf("published before durable pending state: %#v", pending)
		}
		history := sess.History()
		if len(history) != 1 || history[0].Role != historyTypeAsk || history[0].Ask == nil || history[0].Ask.ID != interaction.ID {
			t.Fatalf("pending Ask missing from canonical history: %#v", history)
		}
	case <-time.After(time.Second):
		t.Fatal("pending Ask was not published")
	}
	if _, err := sess.ResolveAsk(context.Background(), interaction.ID, AskCancelled, nil, "user_cancelled"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPendingAskSurvivesSessionReloadAndResolvedReplay(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("ask-reload")
	if err != nil {
		t.Fatal(err)
	}
	interaction := testAskInteraction("call-reload")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, awaitErr := sess.AwaitAsk(ctx, interaction)
		done <- awaitErr
	}()
	waitForPendingAsk(t, sess, interaction.ID)
	if live := sess.LivePendingAsk(interaction.ID); live == nil || live.ID != interaction.ID {
		t.Fatalf("same-process live pending Ask = %#v", live)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("closed waiter error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("closed ask waiter did not stop")
	}

	reopenedStore, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedStore.Close()
	reopened, err := reopenedStore.Get("ask-reload")
	if err != nil {
		t.Fatal(err)
	}
	if pending := reopened.PendingAsk(interaction.ID); pending == nil || len(pending.Questions) != 2 {
		t.Fatalf("reloaded pending ask = %#v", pending)
	}
	if history := reopened.History(); len(history) != 1 || history[0].Ask == nil || history[0].Ask.Status != AskPending {
		t.Fatalf("reloaded Ask history = %#v", history)
	}
	want, err := reopened.ResolveAsk(context.Background(), interaction.ID, AskCancelled, nil, "user_cancelled")
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.AwaitAsk(context.Background(), interaction)
	if err != nil || !sameAskResolution(got, want) {
		t.Fatalf("resolved replay = %#v err=%v, want %#v", got, err, want)
	}
}

func TestColdReloadHostAnswerCancelsOrphanedAskAndUnblocksNextAsk(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("ask-cold-host")
	if err != nil {
		t.Fatal(err)
	}
	interaction := testAskInteraction("call-cold-host")
	interaction.AgentCommandID = "command-cold"
	interaction.AgentOperationID = "operation-cold"
	interaction.AgentCycle = 2
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, awaitErr := sess.AwaitAsk(ctx, interaction)
		done <- awaitErr
	}()
	waitForPendingAsk(t, sess, interaction.ID)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case awaitErr := <-done:
		if !errors.Is(awaitErr, context.Canceled) {
			t.Fatalf("closed waiter error = %v", awaitErr)
		}
	case <-time.After(time.Second):
		t.Fatal("closed Ask waiter did not stop")
	}

	reopenedStore, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedStore.Close()
	reopened, err := reopenedStore.Get("ask-cold-host")
	if err != nil {
		t.Fatal(err)
	}
	if pending := reopened.PendingAsk(interaction.ID); pending == nil || pending.Status != AskPending {
		t.Fatalf("cold durable pending Ask = %#v", pending)
	}
	if live := reopened.LivePendingAsk(interaction.ID); live != nil {
		t.Fatalf("cold pending Ask was exposed as live: %#v", live)
	}
	resolution, err := reopened.ResolveAskFromHost(context.Background(), interaction.ID, AskAnswered, []AskAnswer{
		{QuestionID: "format", SelectedOptionIDs: []string{"markdown"}},
		{QuestionID: "notes", CustomInput: "This answer has no continuation."},
	}, "")
	if err != nil || resolution.Status != AskCancelled || resolution.CancelReason != askContinuationLostReason {
		t.Fatalf("cold host resolution = %#v error=%v", resolution, err)
	}
	if pending := reopened.PendingAsk(""); pending != nil {
		t.Fatalf("orphaned Ask remained pending: %#v", pending)
	}
	history := reopened.History()
	if len(history) != 1 || history[0].Ask == nil || history[0].Ask.Status != AskCancelled || history[0].Ask.CancelReason != askContinuationLostReason {
		t.Fatalf("cold Ask terminal history = %#v", history)
	}

	next := testAskInteraction("call-after-cold")
	next.TaskID = "task-2"
	ready := make(chan struct{}, 1)
	nextDone := make(chan error, 1)
	go func() {
		_, awaitErr := reopened.AwaitAskWithPending(context.Background(), next, func(AskInteraction) { ready <- struct{}{} })
		nextDone <- awaitErr
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("next Ask remained blocked by the orphaned interaction")
	}
	if _, err := reopened.ResolveAskFromHost(context.Background(), next.ID, AskCancelled, nil, "user_cancelled"); err != nil {
		t.Fatal(err)
	}
	if err := <-nextDone; err != nil {
		t.Fatal(err)
	}
}

func TestReconcileStalePendingAskMatchesCycleAndPreservesLiveWaiter(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("ask-live-reconcile")
	if err != nil {
		t.Fatal(err)
	}
	interaction := testAskInteraction("call-live-reconcile")
	interaction.AgentCommandID = "command-live"
	interaction.AgentOperationID = "operation-live"
	interaction.AgentCycle = 3
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, awaitErr := sess.AwaitAskWithPending(context.Background(), interaction, func(AskInteraction) { ready <- struct{}{} })
		done <- awaitErr
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("live Ask did not become pending")
	}

	reconcileResult := make(chan struct {
		reconciled bool
		err        error
	}, 1)
	go func() {
		reconciled, reconcileErr := sess.ReconcileStalePendingAsk(context.Background(), AskCycleIdentity{
			CommandID: "command-live", OperationID: "operation-live", Cycle: 3,
		})
		reconcileResult <- struct {
			reconciled bool
			err        error
		}{reconciled: reconciled, err: reconcileErr}
	}()
	answerResult := make(chan error, 1)
	go func() {
		_, answerErr := sess.ResolveAskFromHost(context.Background(), interaction.ID, AskCancelled, nil, "user_cancelled")
		answerResult <- answerErr
	}()

	if result := <-reconcileResult; result.err != nil || result.reconciled {
		t.Fatalf("live Ask reconciliation = reconciled:%t err:%v", result.reconciled, result.err)
	}
	if err := <-answerResult; err != nil {
		t.Fatalf("live host resolution failed: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("live waiter failed: %v", err)
	}
}

func TestResolvedAskDoesNotReplayAcrossTasksWithSameProviderCallID(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("ask-task-identity")
	if err != nil {
		t.Fatal(err)
	}
	interaction := testAskInteraction("call-reused")
	done := make(chan error, 1)
	go func() {
		_, awaitErr := sess.AwaitAsk(context.Background(), interaction)
		done <- awaitErr
	}()
	waitForPendingAsk(t, sess, interaction.ID)
	if _, err := sess.ResolveAsk(context.Background(), interaction.ID, AskCancelled, nil, "user_cancelled"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	anotherTask := interaction
	anotherTask.TaskID = "task-2"
	if _, err := sess.AwaitAsk(context.Background(), anotherTask); !errors.Is(err, ErrAskAlreadyResolved) {
		t.Fatalf("different task replay error = %v, want %v", err, ErrAskAlreadyResolved)
	}
}

func testAskInteraction(id string) AskInteraction {
	return AskInteraction{
		ID: id, ToolCallID: id, TaskID: "task-1", AgentKind: "ide",
		Questions: []AskQuestion{
			{
				ID: "format", Question: "Which format?", RecommendedOptionID: "markdown",
				Options: []AskOption{{ID: "markdown", Label: "Markdown"}, {ID: "plain", Label: "Plain text"}},
			},
			{ID: "notes", Question: "Any additional notes?"},
		},
	}
}

func waitForPendingAsk(t *testing.T, sess *Session, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sess.PendingAsk(id) != nil {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("ask %q did not become pending", id)
}
