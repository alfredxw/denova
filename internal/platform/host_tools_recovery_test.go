package platform

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"denova/config"
	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// Pause after receiving the tool result, before the model can finish the task.
type pausedPluginModel struct {
	resultReady chan struct{}
	calls       atomic.Int32
}

func (m *pausedPluginModel) Generate(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	if m.calls.Add(1) == 1 {
		return (pluginCallingModel{}).Generate(ctx, messages, options...)
	}
	select {
	case m.resultReady <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *pausedPluginModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := m.Generate(ctx, messages, options...)
	return agent.StreamReaderFromArray([]*agent.Message{message}), err
}

func TestPausedPluginTaskRejectsChangedSharedSettingsAfterReopen(t *testing.T) {
	m, projectID := testManager(t)
	release := testInstall(t, m, testCandidate(t, m, projectID, "tool", "test.shared-tools", Plugin))
	cfg := pluginTestConfig(t, m, projectID)
	tools, err := m.HostAgentTools(cfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	store := agentsession.Memory()
	model := &pausedPluginModel{resultReady: make(chan struct{}, 1)}
	definition := agent.Definition{Name: "shared-plugin-test", Model: model, ModelIdentity: agent.CapabilityIdentity{Kind: "test.paused_plugin_model", Version: 1}, Tools: tools}
	owner, err := agent.New(t.Context(), definition, agent.WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	session, err := owner.Session(ctx, agent.NamedSession("paused-plugin-task"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(ctx, agent.Text("Use the plugin and keep its result."))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.resultReady:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := session.SuspendAndClose(ctx, agent.SuspendRequest{RunID: run.ID(), IdempotencyKey: "pause"}); err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(ctx); err != nil || result.Status != agent.ResultSuspended {
		t.Fatalf("pause result=%#v error=%v", result, err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := writeBytes(m.settingsPath(release.Ref), []byte("enabled = true\n")); err != nil {
		t.Fatal(err)
	}
	definition.Tools, err = m.HostAgentTools(cfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	owner, err = agent.New(t.Context(), definition, agent.WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err = owner.Session(ctx, agent.NamedSession("paused-plugin-task"))
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := session.ResumeRun(ctx, agent.ResumeRequest{RunID: run.ID(), IdempotencyKey: "resume"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := resumed.Wait(ctx)
	if err == nil || result.Status != agent.ResultFailed || !strings.HasPrefix(result.Reason, agent.ErrDefinitionMismatch.Error()) {
		t.Fatalf("changed configuration resumed: result=%#v error=%v", result, err)
	}
	if model.calls.Load() != 2 {
		t.Fatalf("configuration mismatch replayed model work: %d calls", model.calls.Load())
	}
}
