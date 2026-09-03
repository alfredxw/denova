package compaction

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
)

func TestAgentManagerSummaryLimitUsesTightestTargetContextLimit(t *testing.T) {
	enabled := true
	fragmentBytes := 96 << 10
	totalBytes := 80 << 10
	providerBytes := 128 << 10
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
		CompactionEnabled: &enabled,
		MaxFragmentBytes:  &fragmentBytes, MaxTotalInjectedBytes: &totalBytes,
		MaxProviderInputBytes: &providerBytes,
	}}}
	manager, err := NewAgentManager(cfg, config.AgentKindIDE, nil, agent.CapabilityIdentity{Kind: "test.model", Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.SummaryLimitBytes(); got != totalBytes {
		t.Fatalf("summary limit = %d, want tightest target limit %d", got, totalBytes)
	}
}

func TestAgentManagerForModelSeparatesPolicyKindFromConcreteModelWindow(t *testing.T) {
	cfg := &config.Config{OpenAIContextWindowTokens: 100_000}
	identity := agent.CapabilityIdentity{Kind: "test.model.child-window", Version: 1}
	small, err := NewAgentManagerForModel(cfg, config.AgentKindIDE, 12_000, nil, identity)
	if err != nil {
		t.Fatal(err)
	}
	large, err := NewAgentManagerForModel(cfg, config.AgentKindIDE, 24_000, nil, identity)
	if err != nil {
		t.Fatal(err)
	}
	if small.Identity() == large.Identity() {
		t.Fatal("concrete model context window did not change Compaction behavior identity")
	}
	messages := []*agent.Message{agent.UserMessage(strings.Repeat("history ", 200)), agent.AssistantMessage("answer", nil), agent.UserMessage("continue")}
	plan, err := small.Plan(context.Background(), agent.CompactionPlanRequest{
		Messages: messages, ModelRequest: messages, ModelSnapshot: (&agent.ModelCall{Messages: messages}).Snapshot(), Force: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Validation.ContextWindowTokens != 12_000 || plan.Metrics.ContextWindowTokens != 12_000 {
		t.Fatalf("child model window was not applied to plan: %#v", plan)
	}
}

func TestCompactionSummarizerIdentityIncludesCheckpointGuidance(t *testing.T) {
	guidance := "Preserve verification evidence."
	base := denovaSummarizer{agentKind: config.AgentKindIDE, contextWindowTokens: 100_000}
	configured := base
	configured.cfg = &config.Config{AgentContexts: config.AgentContextSettings{
		IDE: config.AgentContextOverride{CheckpointGuidance: &guidance},
	}}
	if base.Identity() == configured.Identity() {
		t.Fatal("checkpoint guidance did not change the compaction summarizer identity")
	}
}

func TestAgentManagerAdvancesBeforeCacheSafeForkCapacityIsExhausted(t *testing.T) {
	cfg := &config.Config{OpenAIContextWindowTokens: 100_000}
	manager, err := NewAgentManager(
		cfg,
		config.AgentKindIDE,
		&compactionForkCaptureModel{response: agent.AssistantMessage("unused", nil)},
		agent.CapabilityIdentity{Kind: "test.model.capacity-preflight", Version: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	source := []*agent.Message{
		agent.UserMessage(strings.Repeat("old request ", 6_000)),
		agent.AssistantMessage(strings.Repeat("old answer ", 6_000), nil),
		agent.UserMessage("current request"),
	}
	primary := append([]*agent.Message{agent.SystemMessage("stable system")}, source...)
	call := &agent.ModelCall{
		Messages: primary,
		Options:  []agent.ModelOption{agent.WithTools(nil), agent.WithMaxTokens(70_000)},
	}
	plan, err := manager.Plan(context.Background(), agent.CompactionPlanRequest{
		Messages: source, ModelRequest: primary, ModelSnapshot: call.Snapshot(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != agent.CompactionCreate || plan.SourceTo <= plan.SourceFrom {
		t.Fatalf("capacity preflight plan = %#v", plan)
	}
}

func TestAgentManagerCompactionForkPreservesFinalModelRequestIdentity(t *testing.T) {
	response := agent.AssistantMessage("## Goal\nPreserve the exact task.", nil)
	model := &compactionForkCaptureModel{response: response}
	cfg := &config.Config{OpenAIContextWindowTokens: 100_000}
	manager, err := NewAgentManager(
		cfg,
		config.AgentKindIDE,
		model,
		agent.CapabilityIdentity{Kind: "test.model.cache-fork", Version: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	source := []*agent.Message{
		agent.UserMessage("old request"),
		agent.AssistantMessage("old answer", nil),
	}
	primary := []*agent.Message{
		agent.SystemMessage("stable system"),
		source[0].Clone(),
		source[1].Clone(),
		agent.UserMessage("current request"),
	}
	tools := []*agent.ToolInfo{{Name: "read", Desc: "read files"}}
	call := &agent.ModelCall{
		Model: model, Messages: primary,
		Options: []agent.ModelOption{
			agent.WithTools(tools),
			agent.WithMaxTokens(2048),
			agent.WithToolChoice(agent.ToolChoiceAllowed, "read"),
		},
	}
	checkpoint, err := manager.Compact(context.Background(), agent.CompactionCompactRequest{
		Messages: source, ModelRequest: primary, ModelSnapshot: call.Snapshot(),
		Plan: agent.CompactionPlan{Action: agent.CompactionCreate, SourceFrom: 0, SourceTo: len(source)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Summary != response.Content || model.requests != 1 {
		t.Fatalf("checkpoint=%#v model requests=%d", checkpoint, model.requests)
	}
	if len(model.inputs[0]) != len(primary)+1 || !reflect.DeepEqual(model.inputs[0][:len(primary)], primary) {
		t.Fatalf("compaction fork changed provider prefix: %#v", model.inputs[0])
	}
	resolved := model.options[0]
	if len(resolved.Tools) != 1 || resolved.Tools[0].Name != "read" || resolved.MaxTokens == nil || *resolved.MaxTokens != 4000 ||
		resolved.ToolChoice == nil || *resolved.ToolChoice != agent.ToolChoiceAllowed ||
		!reflect.DeepEqual(resolved.AllowedToolNames, []string{"read"}) {
		t.Fatalf("compaction fork changed model options: %#v", resolved)
	}
}

func TestAgentManagerCompactionDoesNotSummarizeModelHiddenToolHistory(t *testing.T) {
	disabled := false
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("summary without hidden tool body", nil)}
	cfg := &config.Config{
		OpenAIContextWindowTokens: 100_000,
		AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
			ToolResultContextEnabled: &disabled,
		}},
	}
	manager, err := NewAgentManager(
		cfg, config.AgentKindIDE, model,
		agent.CapabilityIdentity{Kind: "test.model.hidden-tool-compaction", Version: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	toolCall := agent.ToolCall{
		ID: "read-secret", Type: "function",
		Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"secret.md"}`},
	}
	raw := []*agent.Message{
		agent.UserMessage("old request"),
		agent.AssistantMessage("", []agent.ToolCall{toolCall}),
		{Role: agent.ToolRole, ToolCallID: "read-secret", ToolName: "read", Content: "MODEL_HIDDEN_SECRET_BODY"},
		agent.AssistantMessage("old answer", nil),
	}
	visible := []*agent.Message{raw[0].Clone(), raw[3].Clone(), agent.UserMessage("current request")}
	checkpoint, err := manager.Compact(context.Background(), agent.CompactionCompactRequest{
		Messages: raw, SourceMessages: raw, ModelRequest: visible,
		ModelSnapshot: (&agent.ModelCall{Model: model, Messages: visible}).Snapshot(),
		Plan:          agent.CompactionPlan{Action: agent.CompactionCreate, SourceFrom: 0, SourceTo: len(raw)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Summary != "summary without hidden tool body" || model.requests != 1 {
		t.Fatalf("checkpoint=%#v model requests=%d", checkpoint, model.requests)
	}
	for _, message := range model.inputs[0] {
		if message != nil && strings.Contains(message.Content, "MODEL_HIDDEN_SECRET_BODY") {
			t.Fatalf("model-hidden tool body was resurrected in Compaction: %#v", model.inputs[0])
		}
	}
}

func TestAgentManagerIdentityIncludesToolContextVisibilityPolicy(t *testing.T) {
	enabled, disabled := true, false
	identity := agent.CapabilityIdentity{Kind: "test.model.tool-context-identity", Version: 1}
	visible, err := NewAgentManager(&config.Config{
		OpenAIContextWindowTokens: 100_000,
		AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
			ToolResultContextEnabled: &enabled,
		}},
	}, config.AgentKindIDE, nil, identity)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := NewAgentManager(&config.Config{
		OpenAIContextWindowTokens: 100_000,
		AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
			ToolResultContextEnabled: &disabled,
		}},
	}, config.AgentKindIDE, nil, identity)
	if err != nil {
		t.Fatal(err)
	}
	if visible.Identity() == hidden.Identity() {
		t.Fatal("tool-result visibility policy did not change Compaction behavior identity")
	}
}
