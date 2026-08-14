package interactiveapp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	agentexecution "denova/internal/agents/execution"
	agentinteractive "denova/internal/agents/interactive"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/book"
	"denova/internal/book/lore"
	"denova/internal/interactive"
	"denova/internal/interactive/director"
)

func TestInteractiveDirectorPublicRunCommitsCanonicalOutput(t *testing.T) {
	var providerCalls atomic.Int32
	var providerInput atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read provider request: %v", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		providerInput.Store(string(body))
		call := providerCalls.Add(1)
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Error("test provider does not support streaming")
			return
		}
		if call == 1 {
			arguments := `{"decision":{"mode":"keep","reason":"public runtime canonical Director"},"finalize":true}`
			_, _ = fmt.Fprintf(writer, "data: {\"id\":\"director-tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"director-test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call-plan\",\"type\":\"function\",\"function\":{\"name\":\"submit_director_plan_update\",\"arguments\":%q}}]},\"finish_reason\":\"tool_calls\"}]}\n\n", arguments)
		} else {
			_, _ = io.WriteString(writer, `data: {"id":"director-final","object":"chat.completion.chunk","created":1,"model":"director-test","choices":[{"index":0,"delta":{"role":"assistant","content":"Director plan finalized."},"finish_reason":"stop"}]}`+"\n\n")
		}
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Director public lifecycle", Origin: "A public trial begins."})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "I enter the trial.", Narrative: "The witnesses turn toward the arena.",
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.DirectorPlanRunToken(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, turn.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	draft := interactive.NewDirectorPlanUpdateDraft(plan.Docs, token)
	loreRevision, err := lore.NewStore(workspace).Revision()
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, runtimeRoot, workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{Workspace: workspace})
	stable, instruction, err := conversation.BuildDirectorModelInput(turn)
	if err != nil {
		t.Fatal(err)
	}
	commandID, err := DirectorCommandID(token, turn.ID, DirectorTaskPlanUpdate)
	if err != nil {
		t.Fatal(err)
	}
	var draftMu sync.Mutex
	planCommit := newInteractiveDirectorPlanCommit(store, story.ID, "main", turn.ID, token, draft, &draftMu)
	var submitCalls atomic.Int32
	toolContext := agentinteractive.InteractiveStoryToolContext{
		Store: store, CommandID: commandID, StoryID: story.ID, BranchID: "main", TurnID: turn.ID,
		MaintenanceTask: DirectorTaskPlanUpdate, StableContextTitle: stable.Title,
		StableContext: stable.Content, StableContextMaxBytes: stable.MaxBytes,
		CanonicalOutput: planCommit,
		SubmitDirectorPlanUpdate: func(callCtx context.Context, submission interactive.DirectorPlanUpdateSubmission) (interactive.DirectorPlanUpdateReceipt, error) {
			submitCalls.Add(1)
			draftMu.Lock()
			defer draftMu.Unlock()
			submission.SourceLoreRevision = loreRevision
			return store.StageDirectorPlanRunUpdate(story.ID, "main", token, turn.ID, draft, submission)
		},
	}
	cfg := &config.Config{
		Workspace: workspace, NovaDir: runtimeRoot, OpenAIBaseURL: server.URL,
		OpenAIAPIKey: "test-key", OpenAIModel: "director-test", AgentApprovalMode: config.AgentApprovalFullAccess,
	}
	newRuntime := func() *agentexecution.Runtime {
		runtime, runtimeErr := agentexecution.NewAgentRuntime(ctx, runtimeRoot, agentexecution.WithToolMutationApplier(
			func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
		))
		if runtimeErr != nil {
			t.Fatal(runtimeErr)
		}
		return runtime
	}
	live := newRuntime()
	if _, err := agents.GenerateInteractiveDirectorWithTools(ctx, live, cfg, book.NewState(workspace), toolContext, instruction); err != nil {
		t.Fatal(err)
	}
	if err := live.Close(ctx); err != nil {
		t.Fatal(err)
	}
	committed, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if committed.Metadata.LastRun == nil || committed.Metadata.LastRun.Status != director.PlanStatusReady ||
		committed.Metadata.LastRun.DomainCommit == nil || strings.TrimSpace(committed.Metadata.LastRun.DomainCommit.AgentOutputHash) == "" {
		t.Fatalf("public Director Run did not publish an exact canonical receipt: %#v", committed.Metadata.LastRun)
	}
	if submitCalls.Load() != 1 {
		t.Fatalf("Director submit calls = %d, want 1", submitCalls.Load())
	}
	requestBody, _ := providerInput.Load().(string)
	for _, want := range []string{"submit_director_plan_update", "agent-brief.md", turn.ID} {
		if !strings.Contains(requestBody, want) {
			t.Fatalf("real Director provider request is missing %q: %s", want, requestBody)
		}
	}
}
