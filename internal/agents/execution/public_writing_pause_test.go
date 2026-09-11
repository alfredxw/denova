package execution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/internal/agents/canonicalstore"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/project"

	agent "github.com/alfredxw/denova/agent"
	publictools "github.com/alfredxw/denova/agent/tools"
)

func TestWritingCanonicalPauseAnswerAndColdResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	workspace, dataDir := t.TempDir(), t.TempDir()
	registry := project.NewRegistry(dataDir)
	record, err := registry.Add(workspace, project.TypeGeneral, "Writing recovery")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureStore(record)
	if err != nil {
		t.Fatal(err)
	}
	productStore, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := productStore.GetOrCreate("writing-pause")
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendTestModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "ask-original", Type: "function", Function: agent.FunctionCall{
			Name: "ask", Arguments: `{"questions":[{"id":"scope","prompt":"Choose scope","options":[{"value":"minimal","label":"Minimal","recommended":true},{"value":"full","label":"Full"}]}]}`,
		}}}),
	}}
	options := agentrun.Options{ProjectID: record.ID, AgentKind: agentrun.AgentKindIDE, Workspace: workspace, StateRoot: layout.StoreRoot, SessionID: sess.ID}
	newCycle := func(request agentchat.ChatRequest) Cycle {
		return Cycle{Definition: agent.Definition{Key: "writing-pause", Name: "writer", Model: model, Tools: publictools.Ask(),
			ModelIdentity: agent.CapabilityIdentity{Kind: "test.writing-pause", Version: 1}},
			Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE), Request: request, Options: options}
	}
	newRuntime := func() *Runtime {
		journalStore, err := canonicalstore.New(dataDir, registry)
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := NewAgentRuntime(ctx, dataDir, WithSessionStore(journalStore),
			WithProfiles(publicBackendTestProfile{prepare: func(_ context.Context, request CycleRestoreRequest) (Cycle, error) {
				return newCycle(request.Request), nil
			}, canonical: publicBackendTestSessionCanonical(sess)}),
			WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }))
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	}
	runtime := newRuntime()
	t.Cleanup(func() { _ = runtime.Close(context.Background()); _ = productStore.Close() })
	pending := make(chan string, 1)
	operation, err := runtime.Start(ctx, StartRequest{Cycle: newCycle(agentchatRequest("original-command", "Keep this original input")), Emit: func(event agentrun.Event) {
		if event.Type == "ask_pending" {
			pending <- event.Data.(map[string]any)["id"].(string)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan agentrun.Outcome, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Errorf("Writing observation panicked: %v", recovered)
			}
		}()
		outcomes <- operation.Wait(ctx)
	}()
	var askID string
	select {
	case askID = <-pending:
	case outcome := <-outcomes:
		for _, input := range model.inputs {
			for _, message := range input {
				if message.Role == agent.ToolRole {
					t.Logf("tool feedback: %s", message.Content)
				}
			}
		}
		t.Fatalf("Writing ended before Ask: %+v", outcome)
	case <-ctx.Done():
		t.Fatal("Ask did not become durable")
	}
	runID := operation.Receipt().OperationID
	if _, err := runtime.SubmitCommand(ctx, CommandRequest{Kind: CommandSuspend, CommandID: "pause", OperationID: runID, Options: options}); err != nil {
		t.Fatal(err)
	}
	if outcome := <-outcomes; outcome.Status != agentrun.OutcomeSuspended {
		t.Fatalf("pause=%+v", outcome)
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := productStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(dataDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".idx.json") {
			return os.Remove(path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	productStore, err = session.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err = productStore.Get("writing-pause")
	if err != nil {
		t.Fatal(err)
	}
	model = &publicBackendTestModel{responses: []*agent.Message{agent.AssistantMessage("Finished once", nil)}}
	runtime = newRuntime()
	answers := []agentconversation.HostAskAnswer{{QuestionID: "scope", SelectedOptionIDs: []string{"minimal"}}}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := runtime.ResolveAsk(ctx, options, askID, session.AskAnswered, answers, "")
		if err != nil || result.Status != session.AskAnswered {
			t.Fatalf("answer=%+v error=%v", result, err)
		}
	}
	page, err := sess.ReadHistoryPage(ctx, -1, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundAnswer := false
	for _, entry := range page.Entries {
		if entry.Ask != nil && entry.Ask.ID == askID {
			foundAnswer = entry.Ask.Status == session.AskAnswered && len(entry.Ask.Answers) == 1
		}
	}
	if !foundAnswer {
		t.Fatalf("cold history did not project the durable answer: %+v", page.Entries)
	}
	observation, err := runtime.OpenRecoveryObservation(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer observation.Close()
	status := observation.InitialStatus()
	if status.Phase != agentrun.RunPhaseSuspended || status.ActiveOperation != runID || len(model.inputs) != 0 {
		t.Fatalf("cold status=%+v", status)
	}
	if _, err := observation.Resume(ctx, RuntimeRecoveryActions(status)[0], "new-display", nil); err != nil {
		t.Fatal(err)
	}
	if outcome := observation.Wait(ctx, nil); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("resume=%+v", outcome)
	}
	messages, err := sess.ReadCanonicalMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inputs, outputs := 0, 0
	for _, message := range messages {
		if message.Role == agent.User && message.Content == "Keep this original input" {
			inputs++
		}
		if message.Role == agent.Assistant && message.Content == "Finished once" {
			outputs++
		}
	}
	if inputs != 1 || outputs != 1 || len(model.inputs) != 1 {
		t.Fatalf("canonical inputs=%d outputs=%d model calls=%d", inputs, outputs, len(model.inputs))
	}
}
