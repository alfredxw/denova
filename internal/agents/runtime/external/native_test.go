package external

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

func TestNativeApprovalRequiresExplicitChoiceAndSettlesWithdrawal(t *testing.T) {
	for _, action := range []string{"allow", "deny", "free_text", "withdraw"} {
		t.Run(action, func(t *testing.T) {
			service, request, _ := operationFixture(t)
			promptCtx, withdraw := context.WithCancelCause(t.Context())
			defer withdraw(nil)
			request.Emit = func(event agentrun.Event) {
				if event.Type != "ask_pending" {
					return
				}
				pending := event.Data.(*session.AskInteraction)
				if action == "withdraw" {
					withdraw(ErrInteractionWithdrawn)
					return
				}
				answer := conversation.HostAskAnswer{QuestionID: "approval", SelectedOptionIDs: []string{action}}
				if action == "free_text" {
					answer.SelectedOptionIDs = nil
					answer.CustomInput = "allow"
				}
				if _, err := service.Interactions.Resolve(t.Context(), request.ProjectID, request.Session, pending.ID, []conversation.HostAskAnswer{answer}, nil); err != nil {
					t.Error(err)
				}
			}
			request.Adapter = adapterFunc(func(_ context.Context, input Input, host Host) (Result, error) {
				allowed, err := ApproveNative(promptCtx, host, input, "approval-fixture", "Open the configured browser integration.")
				if err != nil {
					return Result{}, err
				}
				if allowed != (action == "allow") {
					return Result{}, errors.New("approval did not match the explicit choice")
				}
				return Result{Text: "Permission resolved."}, nil
			})
			operation, err := service.Start(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if outcome := operation.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("outcome=%+v", outcome)
			}
			if _, err := PrepareHistory(t.Context(), request.Session); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNativeObservationsRecoverFromCanonicalJournal(t *testing.T) {
	for _, disconnected := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "disconnected"}[disconnected], func(t *testing.T) {
			service, request, workspace := operationFixture(t)
			request.Input.Workspace = workspace
			request.Adapter = adapterFunc(func(ctx context.Context, input Input, host Host) (Result, error) {
				observer := NativeObserver{Input: input, Host: host}
				args, _ := json.Marshal(map[string]string{"command": "cat " + workspace + "/draft.md"})
				if err := observer.Start("native-read", "shell_command", args); err != nil {
					return Result{}, err
				}
				if disconnected {
					return Result{}, errors.New("process lost")
				}
				if err := observer.Finish("native-read", "Confirmed evidence from "+workspace+"/draft.md", true); err != nil {
					return Result{}, err
				}
				return Result{Text: "Evidence read."}, observer.Close()
			})
			operation, err := service.Start(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			outcome := operation.Wait(t.Context())
			if !disconnected && outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("outcome=%+v", outcome)
			}
			if err := Reconcile(t.Context(), request.Session, workspace, request.AttachmentRoot); err != nil {
				t.Fatal(err)
			}
			history, err := PrepareHistory(t.Context(), request.Session)
			if err != nil {
				t.Fatal(err)
			}
			messages, err := history.Messages(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(messages)
			want := "Confirmed evidence"
			if disconnected {
				want = "external effects are unknown"
			}
			if !strings.Contains(string(body), want) || strings.Contains(string(body), workspace) {
				t.Fatalf("nonportable or missing observation: %s", body)
			}
			if err := request.Session.ReadExternal(t.Context(), func(state session.ExternalState) error {
				for _, operation := range state.Projection.Operations {
					for _, tool := range operation.Tools {
						if tool.Finished == nil {
							return errors.New("native observation was not settled")
						}
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
