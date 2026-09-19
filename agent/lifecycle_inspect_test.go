package agent

import (
	"context"
	"fmt"
	"testing"
)

func TestCommandRunSurvivesRecentHistoryEvictionAndColdReopen(t *testing.T) {
	ctx := t.Context()
	store := newObservingSessionStore()
	model := &lifecycleModel{}
	for i := range 40 {
		model.responses = append(model.responses, AssistantMessage(fmt.Sprintf("answer-%d", i), nil))
	}
	owner, err := New(ctx, Definition{Name: "inspect", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	sess, err := owner.Session(ctx, NamedSession("exact-command"))
	if err != nil {
		t.Fatal(err)
	}
	var first CommandReceipt
	for i := range 40 {
		run, err := sess.Run(ctx, Input{Text: fmt.Sprintf("message-%d", i), IdempotencyKey: fmt.Sprintf("command-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if result, err := run.Wait(ctx); err != nil || result.Status != ResultCompleted {
			t.Fatalf("run %d: %+v %v", i, result, err)
		}
		if i == 0 {
			first = run.Receipt()
		}
	}
	recent, err := sess.Snapshot(ctx)
	if err != nil || len(recent.RecentRuns) != 32 {
		t.Fatalf("recent: %+v %v", recent, err)
	}
	assertExact := func(sess *Session) {
		t.Helper()
		run, found, err := sess.CommandRun(ctx, "command-0")
		if err != nil || !found {
			t.Fatalf("old command lost: %v %v", found, err)
		}
		snapshot := run.Snapshot()
		if snapshot.Receipt != first || snapshot.Result == nil || snapshot.Result.Status != ResultCompleted || snapshot.Output != "answer-0" {
			t.Fatalf("exact snapshot: %+v", snapshot)
		}
		if _, found, err := sess.CommandRun(ctx, "missing"); found || err != nil {
			t.Fatalf("missing lookup: %v %v", found, err)
		}
	}
	assertExact(sess)
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner, err = New(ctx, Definition{Name: "inspect", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	sess, err = owner.Session(ctx, NamedSession("exact-command"))
	if err != nil {
		t.Fatal(err)
	}
	assertExact(sess)
	if len(model.calls()) != 40 {
		t.Fatalf("lookup executed model: %d", len(model.calls()))
	}
}
