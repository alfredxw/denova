package codex

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/runtime/external"
)

type steeringHost struct {
	testHost
	committed atomic.Int32
}

func (host *steeringHost) PrepareSteer(_ context.Context, guidance external.Guidance) (external.PreparedSteer, error) {
	return external.PreparedSteer{Input: external.Input{Text: guidance.Request.Message}, Commit: func(context.Context) error {
		host.committed.Add(1)
		return nil
	}}, nil
}

func TestSteeringUsesCurrentNativeTurnAndCommitsOnlyAcceptedInput(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		t.Run(map[bool]string{true: "accepted", false: "turn_ended"}[accepted], func(t *testing.T) {
			fixture := newProtocolFixture(t)
			host := &steeringHost{}
			changed := make(chan struct{}, 1)
			delivered := make(chan struct{}, 1)
			var consumed atomic.Bool
			ctx := external.WithSteering(t.Context(), &external.Steering{
				Changed: changed,
				Next: func(context.Context) (external.Guidance, bool, error) {
					return external.Guidance{Request: agentchat.ChatRequest{CommandID: "guidance", Message: "Keep the gate intact"}, Count: 1}, !consumed.Load(), nil
				},
				Delivered: func(external.Guidance) { consumed.Store(true); delivered <- struct{}{} },
			})
			done := startFixtureTurn(t, ctx, fixture, host)
			changed <- struct{}{}
			request := fixture.receive(t)
			var params struct {
				ThreadID, ExpectedTurnID string
				Input                    []struct{ Type, Text string }
			}
			if err := json.Unmarshal(request.Params, &params); err != nil || request.Method != "turn/steer" || params.ThreadID != "thread" || params.ExpectedTurnID != "turn" || len(params.Input) != 1 || params.Input[0].Text != "Keep the gate intact" {
				t.Fatalf("wrong native steering target: %s %s (%v)", request.Method, request.Params, err)
			}
			if host.committed.Load() != 0 {
				t.Fatal("guidance committed before native acceptance")
			}
			reply := packet{ID: request.ID, Result: json.RawMessage(`{"turnId":"turn"}`)}
			if !accepted {
				reply.Result, reply.Error = nil, &rpcError{Code: -32600, Message: "No active turn to steer"}
			}
			if err := json.NewEncoder(fixture.server).Encode(reply); err != nil {
				t.Fatal(err)
			}
			if accepted {
				select {
				case <-delivered:
				case <-time.After(time.Second):
					t.Fatal("accepted input was not committed")
				}
			}
			fixture.send(t, "", "turn/completed", map[string]any{"threadId": "thread", "turn": map[string]string{"id": "turn", "status": "completed"}})
			select {
			case result := <-done:
				if result.err != nil || !result.result.Settled || consumed.Load() != accepted || (host.committed.Load() == 1) != accepted {
					t.Fatalf("steering changed terminal outcome: %+v", result)
				}
			case <-time.After(time.Second):
				t.Fatal("steered turn did not finish")
			}
		})
	}
}
