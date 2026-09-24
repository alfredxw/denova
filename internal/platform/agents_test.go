package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"denova/internal/agents/canonicalstore"
	agent "github.com/alfredxw/denova/agent"
)

type platformTestModel struct{ calls atomic.Int32 }

func (m *platformTestModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	m.calls.Add(1)
	return agent.AssistantMessage("Test response.", nil), nil
}
func (m *platformTestModel) Stream(ctx context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := m.Generate(ctx, input, options...)
	return agent.StreamReaderFromArray([]*agent.Message{message}), err
}

func TestAgentCommandsSurviveRestartAndIndexRebuild(t *testing.T) {
	m, projectID := testManager(t)
	store, err := canonicalstore.New(m.root, m.registry)
	if err != nil {
		t.Fatal(err)
	}
	model := &platformTestModel{}
	resolver := func(context.Context, string) (agent.BaseChatModel, agent.CapabilityIdentity, error) {
		return model, agent.CapabilityIdentity{Kind: "platform.test", Version: 1}, nil
	}
	m.ConfigureAgents(store, resolver)
	release := testInstall(t, m, testCandidate(t, m, projectID, "agent", "test.npc", Game))
	instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "NPC", ProjectID: projectID, Models: map[string]string{"local:writer": "test"}})
	if err != nil {
		t.Fatal(err)
	}
	options := OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}
	opened, err := m.OpenInstance(context.Background(), instance.ID, options)
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
	command := map[string]any{"commandId": "turn-1", "input": map[string]string{"text": "Return a test response."}}
	status, data = testRequest(t, opened.Connection, "POST", "/agents/sessions/"+session.Ref.SessionID+"/runs", "", command)
	if status != 202 {
		t.Fatalf("start: %d %s", status, data)
	}
	var result RunResult
	_ = json.Unmarshal(data, &result)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, data = testRequest(t, opened.Connection, "GET", "/agents/runs/"+result.Run.RunID, "", nil)
		_ = json.Unmarshal(data, &result)
		if result.Status != "accepted" && result.Status != "running" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if result.Status != "completed" || result.Text != "Test response." || result.Completion == nil {
		t.Fatalf("result: %s", data)
	}
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, layout, err := m.registry.Resolve(projectID, true)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := filepath.Glob(filepath.Join(layout.SessionsDir(), "*.idx.json"))
	if err != nil || len(indexes) == 0 {
		t.Fatalf("expected rebuildable session indexes: %v %v", indexes, err)
	}
	for _, index := range indexes {
		if err := os.Remove(index); err != nil {
			t.Fatal(err)
		}
	}
	store, err = canonicalstore.New(m.root, m.registry)
	if err != nil {
		t.Fatal(err)
	}
	m.ConfigureAgents(store, resolver)
	opened, err = m.OpenInstance(context.Background(), instance.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	status, data = testRequest(t, opened.Connection, "POST", "/agents/sessions/"+session.Ref.SessionID+"/runs", "", command)
	if status != 202 {
		t.Fatalf("find after restart: %d %s", status, data)
	}
	var restored RunResult
	_ = json.Unmarshal(data, &restored)
	if restored.Status != "completed" || restored.Completion == nil || restored.Completion.RecordID != result.Completion.RecordID || model.calls.Load() != 1 {
		t.Fatalf("command was not restored: %s; calls %d", data, model.calls.Load())
	}
	command["input"] = map[string]string{"text": "Different input"}
	status, _ = testRequest(t, opened.Connection, "POST", "/agents/sessions/"+session.Ref.SessionID+"/runs", "", command)
	if status != 409 {
		t.Fatalf("command id reused with other input: %d", status)
	}
}
