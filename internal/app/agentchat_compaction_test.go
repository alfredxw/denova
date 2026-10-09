package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/conversationconfig"
	agentexecution "denova/internal/agents/execution"
	agentruntime "denova/internal/agents/runtime"
	agentchatapp "denova/internal/app/agentchat"
	appagentruntime "denova/internal/app/agentruntime"
	projectdomain "denova/internal/project"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

// The model waits after real Native compaction reaches the provider boundary.
// Every admission, control and journal operation below uses the production App.
func TestAgentChatCompactionIsolatesConversationAdmission(t *testing.T) {
	for _, projectType := range []projectdomain.Type{projectdomain.TypeGeneral, projectdomain.TypeBook} {
		t.Run(string(projectType), func(t *testing.T) {
			for _, exit := range []string{"complete", "cancel", "shutdown"} {
				t.Run(exit, func(t *testing.T) { testAgentChatCompactionAdmission(t, projectType, exit) })
			}
		})
	}
}

func testAgentChatCompactionAdmission(t *testing.T, projectType projectdomain.Type, exit string) {
	t.Helper()
	started, release := make(chan struct{}), make(chan struct{})
	var startedOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		wait := false
		startedOnce.Do(func() {
			wait = true
			close(started)
		})
		if wait {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		if !request.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"Earlier research is complete. Continue with the latest request."},"finish_reason":"stop"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Completed the independent request.\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	t.Setenv("OPENAI_API_KEY", "fixture-only")
	t.Setenv("OPENAI_BASE_URL", server.URL+"/v1")
	t.Setenv("OPENAI_MODEL", "test-model")
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := config.WriteSettingsFile(config.UserConfigPath(dataDir), config.Settings{
		OpenAIModel: "test-model", OpenAIAPIKey: "fixture-only", OpenAIBaseURL: server.URL + "/v1",
	}); err != nil {
		t.Fatal(err)
	}
	application, err := New(t.Context(), &config.Config{
		NovaDir: dataDir, Workspace: root,
		OpenAIModel: "test-model", OpenAIAPIKey: "fixture-only", OpenAIBaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	t.Cleanup(unblock)
	service := application.AgentChat()
	project, err := application.projectRegistry.Add(t.TempDir(), projectType, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.projectRegistry.EnsureStore(project); err != nil {
		t.Fatal(err)
	}
	first, err := service.CreateSession(project.ID, "Compacting", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateSession(project.ID, "Independent", nil)
	if err != nil {
		t.Fatal(err)
	}
	a := agentchatapp.Binding{ProjectID: project.ID, SessionID: first.ID}
	b := agentchatapp.Binding{ProjectID: project.ID, SessionID: second.ID}
	bound, err := service.ProjectRuntime(t.Context(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := bound.SessionStore.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	for range 12 {
		for _, message := range []*agentschema.Message{
			agentschema.UserMessage(strings.Repeat("Keep the research findings and writing constraints. ", 200)),
			agentschema.AssistantMessage("Research findings retained.", nil),
		} {
			if err := conversation.Append(message); err != nil {
				t.Fatal(err)
			}
		}
	}
	compactCtx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	compacted := make(chan error, 1)
	go func() {
		defer func() {
			if value := recover(); value != nil {
				compacted <- fmt.Errorf("compaction panicked: %v", value)
			}
		}()
		result, err := service.CompactContext(compactCtx, a, "compact-first")
		if err == nil && !result.Triggered {
			err = errors.New("compaction did not create a checkpoint")
		}
		compacted <- err
	}()
	select {
	case <-started:
	case err := <-compacted:
		t.Fatalf("compaction stopped before the provider wait: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("compaction did not reach the provider")
	}

	independent := make(chan error, 1)
	go func() {
		defer func() {
			if value := recover(); value != nil {
				independent <- fmt.Errorf("independent requests panicked: %v", value)
			}
		}()
		if _, err := service.ConversationConfig(t.Context(), b); err != nil {
			independent <- err
			return
		}
		if _, err := service.Recover(t.Context(), b, appagentruntime.RecoveryRequest{}); !errors.Is(err, agentexecution.ErrRecoveryActionChanged) {
			independent <- fmt.Errorf("independent recovery: %w", err)
			return
		}
		task, err := service.StartTask(t.Context(), b, agentchat.ChatRequest{CommandID: "independent-turn", Message: "Continue independently."})
		if err == nil {
			select {
			case <-task.Done():
				if task.Status() != "done" {
					err = fmt.Errorf("independent task failed: %+v", task.Snapshot())
				}
			case <-time.After(5 * time.Second):
				err = errors.New("independent turn did not finish")
			}
		}
		independent <- err
	}()
	select {
	case err := <-independent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("another conversation waited for compaction")
	}

	view := service.ActiveView(t.Context(), a)
	if !service.SessionBusy(a) || view.Task != nil || view.RuntimeProjectionOK || view.Runtime.ActiveOperation != "" {
		t.Fatalf("maintenance projection = %+v", view)
	}
	for name, action := range map[string]func() error{
		"send": func() error {
			_, err := service.StartTask(t.Context(), a, agentchat.ChatRequest{CommandID: "conflicting-turn", Message: "Conflict."})
			return err
		},
		"delete": func() error { return service.DeleteSession(project.ID, first.ID) },
		"compact": func() error {
			_, err := service.CompactContext(t.Context(), a, "conflicting-compaction")
			return err
		},
		"control": func() error {
			_, err := service.SubmitCommand(t.Context(), a, agentruntime.Command{})
			return err
		},
		"config": func() error {
			_, err := service.PatchConversationConfig(t.Context(), a, conversationconfig.Patch{}, 1)
			return err
		},
	} {
		if err := action(); !errors.Is(err, agentruntime.ErrOperationActive) {
			t.Fatalf("%s did not preserve maintenance occupancy: %v", name, err)
		}
	}
	switch exit {
	case "cancel":
		cancel()
	case "shutdown":
		closed := make(chan error, 1)
		go func() {
			defer func() {
				if value := recover(); value != nil {
					closed <- fmt.Errorf("shutdown panicked: %v", value)
				}
			}()
			application.Close()
			closed <- nil
		}()
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("shutdown did not cancel the model request")
		}
	case "complete":
		unblock()
	default:
		t.Fatalf("unknown compaction exit: %s", exit)
	}
	select {
	case err := <-compacted:
		if exit == "complete" && err != nil {
			t.Fatal(err)
		}
		if exit != "complete" && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("compaction did not finish")
	}
	if service.SessionBusy(a) {
		t.Fatal("completed compaction retained maintenance occupancy")
	}
	if count := conversation.MessageCountTotal(); count != 24 {
		t.Fatalf("compaction changed the original transcript: %d messages", count)
	}
	if exit != "shutdown" {
		if _, err := service.MessagesPage(t.Context(), a, -1, 100); err != nil {
			t.Fatal(err)
		}
	}
}
