package platform

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"denova/internal/agents/canonicalstore"
	agent "github.com/alfredxw/denova/agent"
)

type cancellablePlatformModel struct {
	started chan struct{}
	calls   atomic.Int32
}

func (m *cancellablePlatformModel) Generate(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	m.calls.Add(1)
	close(m.started)
	<-ctx.Done()
	return nil, ctx.Err()
}
func (m *cancellablePlatformModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	_, err := m.Generate(ctx, messages, options...)
	return nil, err
}

func TestAgentCancellationStreamsAndPersistsWithoutReplaying(t *testing.T) {
	m, projectID := testManager(t)
	store, err := canonicalstore.New(m.root, m.registry)
	if err != nil {
		t.Fatal(err)
	}
	model := &cancellablePlatformModel{started: make(chan struct{})}
	m.ConfigureAgents(store, func(context.Context, string) (agent.BaseChatModel, agent.CapabilityIdentity, error) {
		return model, agent.CapabilityIdentity{Kind: "platform.cancel.test", Version: 1}, nil
	})
	release := testInstall(t, m, testCandidate(t, m, projectID, "agent", "test.cancel", Game))
	instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "NPC", ProjectID: projectID, Models: map[string]string{"local:writer": "test"}})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
	if err != nil {
		t.Fatal(err)
	}
	status, data := testRequest(t, opened.Connection, "POST", "/agents/sessions", "", EnsureAgentSession{ProjectID: projectID, Definition: "local:character", Key: "npc"})
	if status != 201 {
		t.Fatalf("ensure: %d %s", status, data)
	}
	var session AgentSession
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatal(err)
	}
	command := map[string]any{"commandId": "turn-cancel", "input": map[string]string{"text": "Wait for the player."}}
	status, data = testRequest(t, opened.Connection, "POST", "/agents/sessions/"+session.Ref.SessionID+"/runs", "", command)
	if status != 202 {
		t.Fatalf("run: %d %s", status, data)
	}
	var result RunResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("model never started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", opened.Connection.BaseURL+"/agents/runs/"+result.Run.RunID+"/events", nil)
	request.Header.Set("Authorization", "Bearer "+opened.Connection.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "data:") {
			if !strings.Contains(line, `"snapshot"`) {
				t.Fatalf("missing initial snapshot: %s", line)
			}
			break
		}
	}
	status, data = testRequest(t, opened.Connection, "POST", "/agents/runs/"+result.Run.RunID+"/cancel", "", nil)
	if status != 200 {
		t.Fatalf("cancel: %d %s", status, data)
	}
	remaining, err := io.ReadAll(reader)
	if err != nil || !strings.Contains(string(remaining), `"status":"aborted"`) {
		t.Fatalf("stream did not settle cancellation: %s %v", remaining, err)
	}
	status, data = testRequest(t, opened.Connection, "POST", "/agents/sessions/"+session.Ref.SessionID+"/runs", "", command)
	if status != 202 || !strings.Contains(string(data), `"status":"aborted"`) || model.calls.Load() != 1 {
		t.Fatalf("cancelled command replayed: %d %s, calls %d", status, data, model.calls.Load())
	}
}
