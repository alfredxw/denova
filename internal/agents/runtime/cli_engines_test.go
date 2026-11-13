package agentruntime

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	externaljournal "denova/internal/agents/runtime/external/journal"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func TestCLIEnginesValidateLiveModelsAndJournalAdmission(t *testing.T) {
	for _, selection := range []config.RuntimeSelection{
		{Kind: config.RuntimeGrok, Grok: &config.GrokRuntimeSettings{Model: "runtime-model", Effort: "high"}},
		{Kind: config.RuntimeAntigravity, Antigravity: &config.AntigravityRuntimeSettings{Model: "runtime-model", Effort: "medium"}},
	} {
		t.Run(string(selection.Kind), func(t *testing.T) {
			engines := NewEngines()
			defer engines.Close()
			connection := &testConnection{state: external.ConnectionState{Status: "ready"}}
			engines.entries[selection.Kind].factory = func(context.Context) (external.Connection, error) { return connection, nil }
			adapter, release, err := engines.Acquire(context.Background(), selection, config.Config{})
			if err != nil || adapter != connection {
				t.Fatalf("acquire = %v", err)
			}
			release()
			if _, err := externaljournal.NewRecord(externaljournal.OperationAccepted, "operation", 1, externaljournal.Accepted{CommandID: "command", Fingerprint: "fingerprint", Runtime: selection, InputMessageID: "input"}); err != nil {
				t.Fatal(err)
			}
			if selection.Grok != nil {
				selection.Grok.Model = "missing"
			} else {
				selection.Antigravity.Model = "missing"
			}
			if _, _, err := engines.Acquire(context.Background(), selection, config.Config{}); !errors.Is(err, ErrEngineModelUnavailable) {
				t.Fatalf("missing CLI model executed: %v", err)
			}
			store := ProductState{engine: selection.Kind}
			bound := &Session{options: agentrun.Options{AgentKind: config.AgentKindGeneral}, selection: selection, control: &ExternalController{store: store}}
			if _, err := bound.UpdateGoal(context.Background(), agentschema.GoalMutation{}); !errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported) {
				t.Fatalf("unsupported Goal mutation admitted: %v", err)
			}
			if text, err := store.GoalContext(context.Background()); err != nil || text != "" {
				t.Fatalf("inactive Goal leaked into CLI context: %q, %v", text, err)
			}
		})
	}
}
