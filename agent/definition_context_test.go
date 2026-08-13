package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

type boundedTestContextSource struct {
	identity  CapabilityIdentity
	fragments []ContextFragment
}

func (source boundedTestContextSource) Identity() CapabilityIdentity { return source.identity }

func (source boundedTestContextSource) Materialize(context.Context, ContextRequest) ([]ContextFragment, error) {
	return append([]ContextFragment(nil), source.fragments...), nil
}

type contextBoundaryModel struct{ calls atomic.Int32 }

func (model *contextBoundaryModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	model.calls.Add(1)
	return AssistantMessage("unexpected", nil), nil
}

func (model *contextBoundaryModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	model.calls.Add(1)
	return StreamReaderFromArray([]*Message{AssistantMessage("unexpected", nil)}), nil
}

func TestAssembleCycleMessagesPreservesVerbatimHostRendering(t *testing.T) {
	transcript := []*Message{UserMessage("earlier"), AssistantMessage("answer", nil)}
	messages, modelUser, err := assembleCycleMessages(transcript, "raw request", []ContextFragment{
		{
			Source: "denova.stable", Purpose: "preserve localized stable context", Resource: "CREATOR.md",
			Revision: "1", Stability: ContextStablePrefix, Placement: ContextLeadingMessage, Rendering: ContextRenderVerbatim,
			Content: "# 创作者指令\n\n完整内容", HardLimit: 64 << 10,
		},
		{
			Source: "denova.turn", Purpose: "preserve the exact localized turn assembly", Resource: "turn",
			Revision: "command-1", Placement: ContextFinalUserMessage, Rendering: ContextRenderVerbatim,
			Content: "# 本轮上下文\n\n状态\n\n---\n\n# 本轮用户请求（最高优先级）\n\nraw request", HardLimit: 64 << 10,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || messages[0].Content != "# 创作者指令\n\n完整内容" ||
		messages[3].Content != "# 本轮上下文\n\n状态\n\n---\n\n# 本轮用户请求（最高优先级）\n\nraw request" {
		t.Fatalf("messages = %#v", messages)
	}
	if modelUser == nil || modelUser.Content != messages[3].Content {
		t.Fatalf("model user = %#v", modelUser)
	}
}

func TestFinalUserRenderingDoesNotReplaceDurableRawTranscript(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("answer", nil)}}
	owner, err := New(context.Background(), Definition{
		Key: "raw-final-user", Model: model,
		ModelIdentity: CapabilityIdentity{Kind: "model.raw-final-user", Version: 1},
		Context:       &mutableFinalContext{content: "localized model-only turn"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("raw-final-user"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("raw player action"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
	checkpoint, err := session.harness.EngineCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := decodeEngineTranscript(checkpoint.State)
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript.Messages) != 2 || transcript.Messages[0].Content != "raw player action" ||
		transcript.Messages[1].Content != "answer" || transcript.ActiveModelUser != nil {
		t.Fatalf("durable transcript = %#v", transcript)
	}
	calls := model.calls()
	if len(calls) != 1 || len(calls[0]) == 0 ||
		!strings.Contains(calls[0][len(calls[0])-1].Content, "localized model-only turn") {
		t.Fatalf("model-visible messages = %#v", calls)
	}
}

func TestCompactionCheckpointUsesTargetAgentSummaryLimit(t *testing.T) {
	messages := []*Message{UserMessage("old"), AssistantMessage("answer", nil)}
	state := CompactionState{
		ID: "checkpoint", Revision: 1, Summary: strings.Repeat("x", 65<<10),
		ReplacementFrom: 0, ReplacementTo: 2,
	}
	if _, err := effectiveCompactionMessages(messages, state, true, 64<<10); !errors.Is(err, ErrContextLimit) {
		t.Fatalf("oversized checkpoint error = %v, want ErrContextLimit", err)
	}
	state.Summary = strings.Repeat("x", 64<<10)
	effective, err := effectiveCompactionMessages(messages, state, true, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective) != 1 || !strings.Contains(effective[0].Content, state.Summary) {
		t.Fatalf("valid checkpoint was not injected: %#v", effective)
	}
}

func TestContextFinalUserMessageIsUnambiguous(t *testing.T) {
	base := ContextFragment{
		Source: "host", Purpose: "test", Resource: "turn", Stability: ContextTurn, Placement: ContextFinalUserMessage,
		Rendering: ContextRenderVerbatim, Content: "request", HardLimit: 64 << 10,
	}
	if err := validateContextFragments([]ContextFragment{base, base}); err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("duplicate final user message error = %v", err)
	}
	prefix := base
	prefix.Placement = ContextFinalUserPrefix
	if err := validateContextFragments([]ContextFragment{base, prefix}); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("mixed final user context error = %v", err)
	}
}

func TestAttributedContextOmitsEmptyRevision(t *testing.T) {
	rendered := renderContextFragment(ContextFragment{
		Source: "Denova User State", Purpose: "apply live user instructions", Resource: "prompts/ide.md",
		Stability: ContextStablePrefix, Placement: ContextLeadingMessage, Content: "Prefer verified edits.", HardLimit: 64 << 10,
	})
	if strings.Contains(rendered, "Revision:") {
		t.Fatalf("unversioned live context exposed a revision field:\n%s", rendered)
	}
	for _, required := range []string{"Source: Denova User State", "Resource: prompts/ide.md", "Prefer verified edits."} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("attributed context missing %q:\n%s", required, rendered)
		}
	}
}

func TestContextSourceIdentityAndDeclaredBoundsFailClosedBeforeModel(t *testing.T) {
	for _, test := range []struct {
		name       string
		identity   CapabilityIdentity
		fragments  []ContextFragment
		wantError  string
		persistent bool
	}{
		{
			name: "durable source requires identity", persistent: true,
			fragments: []ContextFragment{{
				Source: "host", Purpose: "test", Resource: "state", Stability: ContextStablePrefix, Placement: ContextLeadingMessage,
				Content: "bounded", HardLimit: 64 << 10,
			}},
			wantError: "Context capability identity is incomplete",
		},
		{
			name:     "fragment cannot raise its declared bound after rendering",
			identity: CapabilityIdentity{Kind: "context.test.bounded", Version: 1},
			fragments: []ContextFragment{{
				Source: "host", Purpose: "test", Resource: "state", Stability: ContextStablePrefix, Placement: ContextLeadingMessage,
				Content: "sixbytes", HardLimit: 5,
			}},
			wantError: "exceeds its 5-byte hard limit",
		},
		{
			name:     "fragment provenance is mandatory",
			identity: CapabilityIdentity{Kind: "context.test.provenance", Version: 1},
			fragments: []ContextFragment{{
				Purpose: "test", Resource: "state", Stability: ContextStablePrefix, Placement: ContextLeadingMessage,
				Content: "bounded", HardLimit: 64 << 10,
			}},
			wantError: "requires source, purpose, resource, and HardLimit",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := &contextBoundaryModel{}
			options := []Option(nil)
			if test.persistent {
				options = append(options, WithSessionStore(&persistentMemoryStore{Store: agentsession.Memory()}))
			}
			owner, err := New(context.Background(), Definition{
				Model: model, ModelIdentity: CapabilityIdentity{Kind: "model.test.context-boundary", Version: 1},
				Context: boundedTestContextSource{identity: test.identity, fragments: test.fragments},
			}, options...)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.Close(context.Background()) })
			run, err := owner.Run(context.Background(), Input{Text: "inspect", IdempotencyKey: "context-boundary-" + test.name})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := run.Wait(context.Background()); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Run error = %v, want %q", err, test.wantError)
			}
			if calls := model.calls.Load(); calls != 0 {
				t.Fatalf("invalid Context reached model %d time(s)", calls)
			}
		})
	}
}
