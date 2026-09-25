package agent

import (
	"context"
	"testing"
	"time"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestTreeResumeReconcilesSettledRunAndPendingWork(t *testing.T) {
	for _, participant := range []string{"root", "child"} {
		for _, status := range []ResultStatus{ResultCompleted, ResultFailed} {
			t.Run(participant+"/"+string(status), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				store := agentsession.Memory()
				var err error
				rootKey, key := NamedSession("root"), NamedSession(participant)
				if participant == "child" {
					key.Attributes, err = ChildSessionAttributes(rootKey)
					if err != nil {
						t.Fatal(err)
					}
				}
				model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
				owner, err := New(ctx, Definition{Name: "test", Model: model}, WithSessionStore(store))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = owner.Close(context.Background()) })
				session, err := owner.Session(ctx, key)
				if err != nil {
					t.Fatal(err)
				}
				active, err := session.Run(ctx, Text("first task"))
				if err != nil {
					t.Fatal(err)
				}
				<-model.started
				pending, err := session.FollowUp(ctx, Input{Text: "queued task", IdempotencyKey: "next"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := owner.SuspendTree(ctx, rootKey, SuspendRequest{IdempotencyKey: "pause-tree"}); err != nil {
					t.Fatal(err)
				}
				if err := owner.Close(ctx); err != nil {
					t.Fatal(err)
				}
				resumedModel := &lifecycleModel{}
				if status == ResultCompleted {
					resumedModel.responses = []*Message{AssistantMessage("first", nil), AssistantMessage("second", nil)}
				}
				owner, err = New(ctx, Definition{Name: "test", Model: resumedModel}, WithSessionStore(store))
				if err != nil {
					t.Fatal(err)
				}
				session, err = owner.Session(ctx, key)
				if err != nil {
					t.Fatal(err)
				}
				// Force the legal ResumeTree interleaving: its resumed task settles
				// before the subsequent resume_tree record releases the queue fence.
				resumed, err := session.resumeRun(ctx, ResumeRequest{RunID: active.ID(), IdempotencyKey: "resume-tree:run"}, "pause-tree")
				if err != nil {
					t.Fatal(err)
				}
				if result, err := resumed.Wait(ctx); result.Status != status || (err == nil) != (status == ResultCompleted) {
					t.Fatalf("resumed=%#v err=%v", result, err)
				}
				snapshot, err := session.Snapshot(ctx)
				if err != nil || len(snapshot.QueuedRuns) != 1 || snapshot.QueuedRuns[0].ID != pending.RunID {
					t.Fatalf("fenced queue=%#v err=%v", snapshot.QueuedRuns, err)
				}
				for range 2 {
					if _, err := session.acceptTreeControl(ctx, "resume_tree", "pause-tree", "resume-tree", ""); err != nil {
						t.Fatal(err)
					}
				}
				if status == ResultFailed {
					snapshot, err = session.Snapshot(ctx)
					if err != nil || snapshot.ActiveRunID != "" || len(snapshot.QueuedRuns) != 1 || snapshot.QueuedRuns[0].ID != pending.RunID {
						t.Fatalf("failed task advanced the queue: snapshot=%+v err=%v", snapshot, err)
					}
					if len(resumedModel.calls()) != 1 {
						t.Fatal("pending task ran after failure")
					}
					return
				}
				next, found, err := session.AttachRun(ctx, pending.RunID)
				if err != nil || !found {
					t.Fatalf("pending task missing: %v", err)
				}
				waitCtx, stopWaiting := context.WithTimeout(ctx, time.Second)
				defer stopWaiting()
				if result, err := next.Wait(waitCtx); err != nil || result.Status != ResultCompleted {
					t.Fatalf("pending=%#v err=%v", result, err)
				}
				if len(resumedModel.calls()) != 2 {
					t.Fatal("resumed tasks did not execute exactly once")
				}
			})
		}
	}
}
