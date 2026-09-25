package external

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	agentrun "denova/internal/agents/run"
)

func TestPreparedHistoryKeepsOnlyInterruptedAncestorEffectsInOrder(t *testing.T) {
	service, request, _ := operationFixture(t)
	for attempt, paths := range [][]string{{"unrelated.md"}, {"z-first.md", "a-second.md"}, {"last.md"}} {
		history, err := PrepareHistory(t.Context(), request.Session)
		if err != nil {
			t.Fatal(err)
		}
		request.PreparedCursor, request.ContinuesOperationID = history.Cursor, history.ContinuesOperationID
		request.LoadHistory, request.PriorMutations = history.Messages, history.PriorMutations
		request.CommandID = fmt.Sprintf("attempt-%d", attempt)
		request.Fingerprint, request.Metadata.MessageID = request.CommandID, request.CommandID+"-input"
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(context.Canceled)
		request.Adapter = adapterFunc(func(ctx context.Context, _ Input, host Host) (Result, error) {
			for _, path := range paths {
				arguments, _ := json.Marshal(map[string]string{"path": path, "content": "Keep this confirmed change."})
				result, err := host.CallTool(ctx, ToolCall{ID: path, Name: "write", Arguments: arguments})
				if err != nil || !result.Success {
					t.Fatalf("write %s: %+v, %v", path, result, err)
				}
			}
			if attempt > 0 {
				cancel(ErrSuspended)
				return Result{Text: "Work interrupted after confirmed writes."}, ctx.Err()
			}
			return Result{Text: "Unrelated task complete."}, nil
		})
		op, err := service.Start(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		want := agentrun.OutcomeCompleted
		if attempt > 0 {
			want = agentrun.OutcomeSuspended
		}
		if outcome := op.Wait(ctx); outcome.Status != want {
			t.Fatalf("attempt %d: %+v", attempt, outcome)
		}
	}
	history, err := PrepareHistory(t.Context(), request.Session)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, mutation := range history.PriorMutations {
		paths = append(paths, mutation.Target)
	}
	if !reflect.DeepEqual(paths, []string{"z-first.md", "a-second.md", "last.md"}) {
		t.Fatalf("continuation effects lost canonical order or included another task: %v", paths)
	}
}
