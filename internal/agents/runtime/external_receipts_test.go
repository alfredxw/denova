package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	"denova/internal/agents/sessionjournal"
	"denova/internal/interactive"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

type receiptJournal struct {
	state   ProductState
	options agentrun.Options
	path    string
	reopen  func() ProductState
}

// Reopen copies only the canonical journal, exercising recovery without indexes.
func newReceiptJournal(t *testing.T, product string) receiptJournal {
	t.Helper()
	root := t.TempDir()
	fixture := receiptJournal{}
	var closeStore func() error
	var openStore func(string) ProductState
	var relative string
	if product == "writing" {
		store, err := session.NewStore(root)
		if err != nil {
			t.Fatal(err)
		}
		sess, err := store.GetOrCreate("control")
		if err != nil {
			t.Fatal(err)
		}
		fixture.options = agentrun.Options{ProjectID: "stable-project", SessionID: sess.ID, AgentKind: agentrun.AgentKindIDE}
		fixture.state, err = SessionState(fixture.options, sess)
		if err != nil {
			t.Fatal(err)
		}
		closeStore = store.Close
		relative = "control.jsonl"
		openStore = func(root string) ProductState {
			store, err := session.NewStore(root)
			if err != nil {
				t.Fatal(err)
			}
			closeStore = store.Close
			sess, err := store.Get("control")
			if err != nil {
				t.Fatal(err)
			}
			state, err := SessionState(fixture.options, sess)
			if err != nil {
				t.Fatal(err)
			}
			return state
		}
	} else {
		store := interactive.NewStore(root)
		story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Control receipts", StoryTellerID: "classic"})
		if err != nil {
			t.Fatal(err)
		}
		fixture.options = agentrun.Options{ProjectID: "stable-project", AgentKind: "interactive_story", StoryID: story.ID, BranchID: "main", Mode: "interactive"}
		fixture.state, err = GameState(fixture.options, store)
		if err != nil {
			t.Fatal(err)
		}
		closeStore = store.Close
		relative = filepath.Join("interactive", "story", "story-"+story.ID+".jsonl")
		openStore = func(root string) ProductState {
			store := interactive.NewStore(root)
			closeStore = store.Close
			state, err := GameState(fixture.options, store)
			if err != nil {
				t.Fatal(err)
			}
			return state
		}
	}
	t.Cleanup(func() { _ = closeStore() })
	fixture.path = filepath.Join(root, relative)
	currentPath := fixture.path
	fixture.reopen = func() ProductState {
		if err := closeStore(); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(currentPath)
		if err != nil {
			t.Fatal(err)
		}
		nextRoot := t.TempDir()
		currentPath = filepath.Join(nextRoot, relative)
		if err := os.MkdirAll(filepath.Dir(currentPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(currentPath, body, 0600); err != nil {
			t.Fatal(err)
		}
		return openStore(nextRoot)
	}
	return fixture
}

func TestExternalControlHistoryDoesNotGrowActiveSnapshot(t *testing.T) {
	for _, product := range []string{"writing", "game"} {
		t.Run(product, func(t *testing.T) {
			fixture := newReceiptJournal(t, product)
			state, options, journal := fixture.state, fixture.options, fixture.path
			engines := NewEngines()
			t.Cleanup(func() { _ = engines.Close() })
			control, err := engines.ExternalControl(options, state)
			if err != nil {
				t.Fatal(err)
			}
			run, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "initial", Message: "Work"}}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			var first Command
			var firstReceipt agentrun.CommandReceipt
			for i := 0; i < 20; i++ {
				command := Command{Kind: agentexecution.CommandFollowUp, CommandID: fmt.Sprintf("follow-%d", i), OperationID: run.Receipt().OperationID, Input: agentchat.ChatRequest{Message: strings.Repeat("Historical instruction. ", 700)}}
				receipt, err := control.Submit(t.Context(), command)
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					first, firstReceipt = command, receipt
				}
				_, err = control.Submit(t.Context(), Command{Kind: agentexecution.CommandCancelQueued, CommandID: fmt.Sprintf("cancel-%d", i), OperationID: run.Receipt().OperationID, TargetCommandID: receipt.CommandID})
				if err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Stat(journal)
			if err != nil {
				t.Fatal(err)
			}
			_, err = control.Submit(t.Context(), Command{Kind: agentexecution.CommandSuspend, CommandID: "pause", OperationID: run.Receipt().OperationID})
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(journal)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("pause append: %d bytes", after.Size()-before.Size())
			if delta := after.Size() - before.Size(); delta > 8<<10 {
				t.Fatalf("pause rewrote historical commands: appended %d bytes", delta)
			}
			raw, _, err := state.Read(t.Context(), externalControlCapability)
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) > 4<<10 || strings.Contains(string(raw), "Historical instruction.") {
				t.Fatalf("active control retains historical bodies: %d bytes", len(raw))
			}
			again, err := control.Submit(t.Context(), first)
			if err != nil || again != firstReceipt {
				t.Fatalf("historical retry: %+v, %v", again, err)
			}
			state = fixture.reopen()
			control = &ExternalController{store: state}
			again, err = control.Submit(t.Context(), first)
			if err != nil || again != firstReceipt {
				t.Fatalf("cold historical retry: %+v, %v", again, err)
			}
			first.Input.Message = "Conflicting retry"
			if _, err := control.Submit(t.Context(), first); err == nil {
				t.Fatal("conflicting historical retry accepted")
			}
		})
	}
}

func TestExternalControlReleasedReceiptsMigrateAndResume(t *testing.T) {
	for _, product := range []string{"writing", "game"} {
		t.Run(product, func(t *testing.T) {
			fixture := newReceiptJournal(t, product)
			original := ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "original", Message: "Accepted input", Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly, AttachedFiles: []agentschema.Attachment{{ID: "image", Path: "attachments/image.png"}}}, Delivery: agentrun.DeliveryFollowUp}
			rootReceipt := agentrun.CommandReceipt{CommandID: "original", OperationID: "original-operation", Cursor: 1}
			follow := Command{Kind: agentexecution.CommandFollowUp, CommandID: "follow", OperationID: rootReceipt.OperationID, Input: agentchat.ChatRequest{Message: "Accepted queued input", Locale: "en-US"}}
			followReceipt := agentrun.CommandReceipt{CommandID: "follow", OperationID: rootReceipt.OperationID, Cursor: 2}
			encoded, err := json.Marshal(follow)
			if err != nil {
				t.Fatal(err)
			}
			queued := ExternalCycleInput{Request: follow.Input, Delivery: agentrun.DeliveryFollowUp}
			queued.Request.CommandID = follow.CommandID
			current := original
			current.Resume = true
			released := map[string]any{
				"version": 1, "revision": 41, "operation_id": rootReceipt.OperationID, "command_id": rootReceipt.CommandID, "phase": agentrun.RunPhaseSuspended,
				"current": current, "queue": []ExternalCycleInput{queued},
				"receipts": map[string]controlReceipt{
					"original": {Fingerprint: externalInputFingerprint(original), Receipt: rootReceipt},
					"follow":   {Fingerprint: string(encoded), Receipt: followReceipt},
				},
			}
			if err := fixture.state.Update(t.Context(), externalControlCapability, func(json.RawMessage, bool) (json.RawMessage, error) { return json.Marshal(released) }); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			control := &ExternalController{store: fixture.state, binding: agentrun.RuntimeBinding{AgentKind: fixture.options.AgentKind}}
			backupPath := fixture.path + ".pre-" + controlReceiptUpgrade + ".bak"
			if err := os.Mkdir(backupPath, 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := control.Status(t.Context()); err == nil {
				t.Fatal("converted released state without required backup")
			}
			failed, err := os.ReadFile(fixture.path)
			if err != nil || !bytes.Equal(before, failed) {
				t.Fatalf("failed migration changed journal: %v", err)
			}
			if err := os.Remove(backupPath); err != nil {
				t.Fatal(err)
			}
			status, err := control.Status(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if status.Cursor != 41 || status.Phase != agentrun.RunPhaseSuspended || len(status.Queue) != 1 {
				t.Fatalf("migration changed task: %+v", status)
			}
			backup, err := os.ReadFile(backupPath)
			if err != nil || !bytes.Equal(backup, before) {
				t.Fatalf("original journal backup: %v", err)
			}
			after, err := os.ReadFile(fixture.path)
			if err != nil || !bytes.HasPrefix(after, before) {
				t.Fatalf("migration rewrote history: %v", err)
			}
			if bytes.Count(after[len(before):], []byte("\n")) != 1 {
				t.Fatal("migration did not commit as one transaction")
			}
			if _, err := control.Status(t.Context()); err != nil {
				t.Fatal(err)
			}
			unchanged, _ := os.ReadFile(fixture.path)
			if !bytes.Equal(after, unchanged) {
				t.Fatal("migration repeated on read")
			}
			control = &ExternalController{store: fixture.reopen(), binding: control.binding}
			if got, err := control.Submit(t.Context(), follow); err != nil || got != followReceipt {
				t.Fatalf("released retry: %+v, %v", got, err)
			}
			conflict := follow
			conflict.Input.Message = "Changed instruction"
			if _, err := control.Submit(t.Context(), conflict); !errors.Is(err, agentrun.ErrInvalidCommand) {
				t.Fatalf("released conflicting retry: %v", err)
			}
			var inputs []ExternalCycleInput
			action := agentexecution.RuntimeRecoveryActions(status)[0]
			run, err := control.Resume(t.Context(), action, func(_ context.Context, input ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
				input.OperationID = ""
				inputs = append(inputs, input)
				return externalCycleFunc(func(context.Context) agentrun.Outcome { return agentrun.Outcome{Status: agentrun.OutcomeCompleted} }), nil
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if outcome := run.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("resume outcome: %+v", outcome)
			}
			got, _ := json.Marshal(inputs)
			want, _ := json.Marshal([]ExternalCycleInput{current, queued})
			if !bytes.Equal(got, want) {
				t.Fatalf("accepted inputs changed:\ngot %s\nwant %s", got, want)
			}
			control = &ExternalController{store: fixture.reopen(), binding: control.binding}
			replayed, err := control.Start(t.Context(), original, nil, nil)
			if err != nil || replayed.Receipt() != rootReceipt || replayed.Wait(t.Context()).Status != agentrun.OutcomeCompleted {
				t.Fatalf("completed root retry: %v", err)
			}
			replayed, err = control.Resume(t.Context(), action, nil, nil)
			if err != nil || replayed.Wait(t.Context()).Status != agentrun.OutcomeCompleted {
				t.Fatalf("completed recovery retry: %v", err)
			}
			if len(inputs) != 2 {
				t.Fatalf("historical retry re-executed %d inputs", len(inputs))
			}
		})
	}
}

func TestExternalControlFailedBackupDoesNotAcceptCommand(t *testing.T) {
	for _, product := range []string{"writing", "game"} {
		t.Run(product, func(t *testing.T) {
			fixture := newReceiptJournal(t, product)
			backup := fixture.path + ".pre-" + controlReceiptUpgrade + ".bak"
			if err := os.Mkdir(backup, 0700); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			control := &ExternalController{store: fixture.state}
			input := ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "start", Message: "Work"}}
			if _, err := control.Start(t.Context(), input, nil, nil); err == nil {
				t.Fatal("accepted command without required backup")
			}
			after, err := os.ReadFile(fixture.path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("failed transaction changed journal: %v", err)
			}
			if _, found, err := control.Receipt(t.Context(), "start"); err != nil || found {
				t.Fatalf("failed command left receipt: found=%v error=%v", found, err)
			}
			if err := os.Remove(backup); err != nil {
				t.Fatal(err)
			}
			run, err := control.Start(t.Context(), input, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			abort := Command{Kind: agentexecution.CommandAbort, CommandID: "abort", OperationID: run.Receipt().OperationID}
			receipt, err := control.Submit(t.Context(), abort)
			if err != nil {
				t.Fatal(err)
			}
			control = &ExternalController{store: fixture.reopen()}
			if got, err := control.Submit(t.Context(), abort); err != nil || got != receipt {
				t.Fatalf("cold abort retry: %+v %v", got, err)
			}
			got, err := control.Start(t.Context(), input, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if outcome := got.Wait(t.Context()); outcome.Status != agentrun.OutcomeAborted || outcome.Error != nil {
				t.Fatalf("aborted root retry: %+v", outcome)
			}
		})
	}
}

func TestExternalControlCostIndependentOfReceiptHistory(t *testing.T) {
	for _, product := range []string{"writing", "game"} {
		t.Run(product, func(t *testing.T) {
			fixture := newReceiptJournal(t, product)
			control := &ExternalController{store: fixture.state}
			run, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "start", Message: "Active input"}}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			var measurements [][2]int64
			for _, count := range []int{10, 100} {
				err := fixture.state.Transact(t.Context(), controlReceiptUpgrade, func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error) {
					changes := make(map[string]json.RawMessage)
					for i := 0; i < count; i++ {
						id := fmt.Sprintf("old-%d", i)
						body, err := json.Marshal(controlReceipt{Fingerprint: controlFingerprint([]byte(id)), Receipt: agentrun.CommandReceipt{CommandID: agentrun.CommandID(id), OperationID: "old-operation", Cursor: 1}, Outcome: agentrun.OutcomeCompleted})
						if err != nil {
							return nil, err
						}
						changes[controlReceiptPrefix+id] = body
					}
					return changes, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				var readBytes int64
				control.store.Transact = func(ctx context.Context, upgrade string, mutate func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error)) error {
					return fixture.state.Transact(ctx, upgrade, func(read sessionjournal.CapabilityReader) (map[string]json.RawMessage, error) {
						return mutate(func(key string) (json.RawMessage, bool, error) {
							raw, found, err := read(key)
							readBytes += int64(len(raw))
							return raw, found, err
						})
					})
				}
				before, _ := os.Stat(fixture.path)
				if _, err := control.Submit(t.Context(), Command{Kind: agentexecution.CommandSuspend, CommandID: fmt.Sprintf("pause-%d", count), OperationID: run.Receipt().OperationID}); err != nil {
					t.Fatal(err)
				}
				after, _ := os.Stat(fixture.path)
				measurements = append(measurements, [2]int64{readBytes, after.Size() - before.Size()})
			}
			for dimension := range 2 {
				if measurements[1][dimension] > measurements[0][dimension]+128 {
					t.Fatalf("10x history grew hot-path read/write bytes: %v", measurements)
				}
			}
			t.Logf("10 vs 100 receipts, [state read bytes, canonical append bytes]: %v", measurements)
		})
	}
}

func TestExternalControlGameReceiptBranchIsolation(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Branch receipts", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", User: "Choose", Narrative: "A fork"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID, Title: "Alternative"})
	if err != nil {
		t.Fatal(err)
	}
	var controls []*ExternalController
	var receipts []agentrun.CommandReceipt
	for _, branchID := range []string{"main", branch.ID} {
		options := agentrun.Options{ProjectID: "stable-project", AgentKind: "interactive_story", StoryID: story.ID, BranchID: branchID, Mode: "interactive"}
		state, err := GameState(options, store)
		if err != nil {
			t.Fatal(err)
		}
		control := &ExternalController{store: state}
		if _, found, err := control.Receipt(t.Context(), "same-command"); err != nil || found {
			t.Fatalf("branch inherited another receipt: %v", err)
		}
		run, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "same-command", Message: branchID}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		controls = append(controls, control)
		receipts = append(receipts, run.Receipt())
	}
	if receipts[0].OperationID == receipts[1].OperationID {
		t.Fatal("branches share a control operation")
	}
	for i, control := range controls {
		if got, found, err := control.Receipt(t.Context(), "same-command"); err != nil || !found || got != receipts[i] {
			t.Fatalf("branch receipt: %+v %v %v", got, found, err)
		}
	}
}
