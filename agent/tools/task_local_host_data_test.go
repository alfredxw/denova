package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

func TestLocalTasksColdResumeKeepsOriginalHostAndFollowUpUsesCurrentHost(t *testing.T) {
	for _, product := range []string{"writing", "game"} {
		t.Run(product, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			store, err := sessionfile.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			host := func(parent string) *agent.HostData {
				return &agent.HostData{Type: "test." + product, Version: 1, Data: json.RawMessage(`{"parent":"` + parent + `"}`)}
			}
			prepared := make(chan agent.PrepareRequest, 8)
			newOwner := func(model agent.BaseChatModel) *agent.Agent {
				owner, err := agent.New(ctx, agent.SourceFunc(func(_ context.Context, request agent.PrepareRequest) (agent.Definition, error) {
					prepared <- request
					return agent.Definition{Key: "same-child", Name: "researcher", Model: model,
						ModelIdentity: agent.CapabilityIdentity{Kind: "test.host-routing", Version: 1}}, nil
				}), agent.WithSessionStore(store))
				if err != nil {
					t.Fatal(err)
				}
				return owner
			}
			newExecutor := func(owner *agent.Agent, parent string) *LocalTasks {
				executor, err := NewLocalTasks(LocalTaskOptions{Parallelism: 1}, LocalTaskAgent{
					Name: "researcher", Opener: owner, Identity: agent.CapabilityIdentity{Kind: "test.child", Version: 1},
					Attributes:       map[string]string{"parent": "stable", "parent_route": "released-route-A"},
					LookupAttributes: map[string]string{"parent": "stable"}, HostData: host(parent),
				})
				if err != nil {
					t.Fatal(err)
				}
				return executor
			}
			first := newOwner(&blockingTaskModel{release: make(chan struct{})})
			defer first.Close(context.Background())
			taskA, err := newExecutor(first, "A").Start(ctx, TaskRequest{Agent: "researcher", Prompt: "First task", IdempotencyKey: "first"})
			if err != nil {
				t.Fatal(err)
			}
			var firstRequest agent.PrepareRequest
			select {
			case firstRequest = <-prepared:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			key := firstRequest.Session.Key
			sess, err := first.Session(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := sess.SuspendAndClose(ctx, agent.SuspendRequest{RunID: taskA.Ref.Run, IdempotencyKey: "pause-A"}); err != nil {
				t.Fatal(err)
			}
			if err := first.Close(ctx); err != nil {
				t.Fatal(err)
			}
			cold := newOwner(&taskModel{responses: []*agent.Message{agent.AssistantMessage("A done", nil), agent.AssistantMessage("B done", nil)}})
			defer cold.Close(context.Background())
			executorB := newExecutor(cold, "B")
			resumed, err := executorB.Resume(ctx, taskA.Ref, agent.ResumeRequest{IdempotencyKey: "resume-A"})
			if err != nil {
				t.Fatal(err)
			}
			if resumed.Ref != taskA.Ref {
				t.Fatalf("resume changed task identity: %+v", resumed.Ref)
			}
			sess, err = cold.Session(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			runA, found, err := sess.AttachRun(ctx, taskA.Ref.Run)
			if err != nil || !found {
				t.Fatalf("attach A: found=%t error=%v", found, err)
			}
			if result, err := runA.Wait(ctx); err != nil || result.Status != agent.ResultCompleted {
				t.Fatalf("A=%+v error=%v", result, err)
			}
			taskB, err := executorB.FollowUp(ctx, taskA.Ref, agent.Input{Text: "Second task", IdempotencyKey: "second"})
			if err != nil {
				t.Fatal(err)
			}
			runB, found, err := sess.AttachRun(ctx, taskB.Ref.Run)
			if err != nil || !found {
				t.Fatalf("attach B: found=%t error=%v", found, err)
			}
			if result, err := runB.Wait(ctx); err != nil || result.Status != agent.ResultCompleted {
				t.Fatalf("B=%+v error=%v", result, err)
			}
			if taskB.Ref.Session != taskA.Ref.Session || taskB.Ref.Run == taskA.Ref.Run {
				t.Fatalf("follow-up identity=%+v", taskB.Ref)
			}
			for _, item := range []struct{ run, parent string }{{taskA.Ref.Run, "A"}, {taskB.Ref.Run, "B"}} {
				input, found, err := sess.RunInput(ctx, item.run)
				if err != nil || !found || !reflect.DeepEqual(input.HostData, host(item.parent)) {
					t.Fatalf("%s input=%+v error=%v", item.parent, input, err)
				}
				select {
				case request := <-prepared:
					if request.Run.ID != item.run || !reflect.DeepEqual(request.HostData, host(item.parent)) || !reflect.DeepEqual(request.Session.Key, key) {
						t.Fatalf("%s preparation=%+v", item.parent, request)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
		})
	}
}
