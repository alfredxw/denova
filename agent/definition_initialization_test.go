package agent_test

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/cleanup"
	"github.com/alfredxw/denova/agent/compaction"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/builtin"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
	"github.com/alfredxw/denova/agent/tools"
)

type definitionInitializationModel struct{}

func (definitionInitializationModel) Generate(
	context.Context,
	[]*agent.Message,
	...agent.ModelOption,
) (*agent.Message, error) {
	return agent.AssistantMessage("ok", nil), nil
}

func (definitionInitializationModel) Stream(
	context.Context,
	[]*agent.Message,
	...agent.ModelOption,
) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{agent.AssistantMessage("ok", nil)}), nil
}

func TestNewInitializesAndValidatesCompleteStaticDefinition(t *testing.T) {
	owner, err := agent.New(context.Background(), agent.Definition{
		Model: definitionInitializationModel{},
		Tools: tools.Combine(
			tools.Todo(),
			tools.Ask(),
		),
		Cleanup: cleanup.Standard(cleanup.StandardConfig{ContextWindowTokens: 128_000}),
		Compaction: compaction.Standard(compaction.StandardConfig{
			Summarizer: compaction.SummarizerFunc{
				Capability: agent.CapabilityIdentity{Kind: "test.summary", Version: 1},
				Func: func(context.Context, compaction.SummaryRequest) (compaction.Summary, error) {
					return compaction.Summary{Content: "summary"}, nil
				},
			},
			HardLimitBytes: 4 << 20, SummaryLimitBytes: 256 << 10,
		}),
	}, agent.WithSessionStore(sessionfile.New(t.TempDir())))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
}

func TestNewResolvesDeclarativeBuiltinModelAndIdentity(t *testing.T) {
	model := builtin.Model(providers.ModelConfig{
		Provider: providers.ProviderOpenAI,
		Model:    "gpt-5",
	})
	owner, err := agent.New(context.Background(), agent.Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	if identity := model.(agent.DefinitionModel).ModelIdentity(); identity.Kind == "" || identity.Version == 0 {
		t.Fatalf("declarative model identity = %#v", identity)
	}
}

func TestNewReportsEveryInvalidDeclarativeCapability(t *testing.T) {
	owner, err := agent.New(context.Background(), agent.Definition{
		Model: builtin.Model(providers.ModelConfig{}),
		Tools: tools.Combine(
			tools.Todo(nil),
			tools.Skills(nil),
		),
		Cleanup: cleanup.Standard(cleanup.StandardConfig{
			ContextWindowTokens: 128_000,
			CacheState:          cleanup.CacheState("invalid"),
		}),
		Compaction: compaction.Standard(compaction.StandardConfig{}),
	})
	if owner != nil || err == nil {
		t.Fatalf("owner=%#v error=%v", owner, err)
	}
	for _, expected := range []string{
		"Model", "Tools", "Toolset[0]", "TodoStore", "Toolset[1]", "SkillSource",
		"Cleanup", "CacheState", "Compaction", "Summarizer",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("agent.New error %q does not include %q", err, expected)
		}
	}
}
