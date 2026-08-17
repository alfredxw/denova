package lifecycle

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type productCompactionProviderProbe struct {
	projection ProductCompactionProjection
}

func (probe productCompactionProviderProbe) PrepareAgentCompaction(
	context.Context,
	agent.CompactionCompactRequest,
) (ProductCompactionProjection, error) {
	return probe.projection, nil
}

type compactionDelegateProbe struct {
	request agent.CompactionCompactRequest
}

func (*compactionDelegateProbe) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "compaction.probe", Version: 1}
}

func (*compactionDelegateProbe) SummaryLimitBytes() int { return 64 << 10 }

func (*compactionDelegateProbe) Plan(context.Context, agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	return agent.CompactionPlan{}, nil
}

func (probe *compactionDelegateProbe) Compact(
	_ context.Context,
	request agent.CompactionCompactRequest,
) (agent.CompactionCheckpoint, error) {
	probe.request = request
	return agent.CompactionCheckpoint{Summary: "next checkpoint", TokenEstimate: 3}, nil
}

func TestConversationCompactionUsesCanonicalProductDeltaAndKeepsActiveCheckpoint(t *testing.T) {
	delegate := &compactionDelegateProbe{}
	contextData := &agent.HostData{Type: "game.cursor", Version: 1, Data: []byte(`{"turn":2}`)}
	manager := BindConversationCompaction(delegate, productCompactionProviderProbe{projection: ProductCompactionProjection{
		SourceMessages: []*agent.Message{agent.UserMessage("canonical action"), agent.AssistantMessage("canonical narrative", nil)},
		ContextData:    contextData,
	}})
	checkpointMarker := agent.SystemMessage("active checkpoint marker")
	checkpoint, err := manager.Compact(context.Background(), agent.CompactionCompactRequest{
		Messages: []*agent.Message{
			agent.UserMessage("old rendered game prompt"), agent.AssistantMessage("old answer", nil),
			agent.UserMessage("new rendered game prompt"), agent.AssistantMessage("new answer", nil),
		},
		SourceMessages: []*agent.Message{checkpointMarker, agent.UserMessage("repeated rendered prompt")},
		Plan:           agent.CompactionPlan{Action: agent.CompactionCreate, SourceFrom: 0, SourceTo: 4},
		Current: agent.CompactionState{
			ID: "checkpoint-1", ReplacementFrom: 0, ReplacementTo: 2, Summary: "previous",
		},
		Present: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(delegate.request.SourceMessages) != 3 ||
		delegate.request.SourceMessages[0].Content != "active checkpoint marker" ||
		delegate.request.SourceMessages[1].Content != "canonical action" ||
		delegate.request.SourceMessages[2].Content != "canonical narrative" {
		t.Fatalf("delegate source = %#v", delegate.request.SourceMessages)
	}
	if checkpoint.ContextData != contextData {
		t.Fatalf("checkpoint context data = %#v", checkpoint.ContextData)
	}
}
