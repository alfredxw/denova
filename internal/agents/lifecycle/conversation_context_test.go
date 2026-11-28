package lifecycle

import (
	"context"
	"strings"
	"testing"

	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	"github.com/alfredxw/denova/agent"
	sdkcontext "github.com/alfredxw/denova/agent/context"
	agentcompaction "github.com/alfredxw/denova/agent/context/compaction"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

type contextTestConversation struct {
	cycle      agentrun.CycleIdentity
	kind       string
	assemblies int
}

func (conversation *contextTestConversation) AssembleModelContext(
	context.Context,
	string,
	agentcontext.ModelContextInput,
) (agentcontext.ModelContextResult, error) {
	conversation.assemblies++
	assembled, err := agentcontext.NewAssembler(agentcontext.Budget{}).Assemble(context.Background(), agentcontext.AssembleRequest{
		Messages: []*agentschema.Message{agentschema.UserMessage("精确的 Denova 用户消息 / exact Denova user message")},
		Fragments: []agentcontext.Fragment{
			{
				ID: "stable", Source: "workspace.stable", Title: "稳定状态", Purpose: "cache prefix",
				Content: "stable body", Placement: agentcontext.PlacementLeadingMessage,
				Limit: 1024, Included: true,
			},
			{
				ID: "turn", Source: "workspace.turn", Purpose: "current state", Content: "turn body",
				Placement: agentcontext.PlacementAuditOnly, Limit: 1024, Included: true,
			},
		},
	})
	return agentcontext.ModelContextResult{Messages: assembled.Messages, Context: assembled}, err
}

type boundaryCommitterProbe struct {
	preparedContext   agentchat.AgentContextPreparation
	outputPreparation agentchat.AgentContextPreparation
	outputRequest     agentcanonical.OutputCommitRequest
}

func (probe *boundaryCommitterProbe) MaterializeInput(_ context.Context, request agentcanonical.InputCommitRequest) (agentcanonical.CommitReceipt, error) {
	return agentcanonical.CommitReceipt{Revision: "input:1"}, nil
}

func (probe *boundaryCommitterProbe) ApplyPreparedContext(_ context.Context, prepared agentchat.AgentContextPreparation) error {
	probe.preparedContext = prepared
	return nil
}

func (probe *boundaryCommitterProbe) CommitOutput(_ context.Context, prepared agentchat.AgentContextPreparation, request agentcanonical.OutputCommitRequest) (agentcanonical.OutputCommitReceipt, error) {
	probe.outputPreparation = prepared
	probe.outputRequest = request
	return agentcanonical.OutputCommitReceipt{Revision: "output:1"}, nil
}

func (*contextTestConversation) AppendAssistant(string) error                 { return nil }
func (*contextTestConversation) MarkInterrupted(string, string, string) error { return nil }
func (*contextTestConversation) PendingInterruption() *session.Interruption   { return nil }
func (*contextTestConversation) ResolveInterruption(string) error             { return nil }
func (conversation *contextTestConversation) BindAgentCycleIdentity(identity agentrun.CycleIdentity) {
	conversation.cycle = identity
}
func (conversation *contextTestConversation) BindAgentKind(kind string) { conversation.kind = kind }

func TestConversationContextSourcePreservesExactDenovaRenderingAndCycleIdentity(t *testing.T) {
	conversation := &contextTestConversation{}
	preparedCalled := false
	source, err := NewConversationContextSource(ConversationContextConfig{
		Conversation: conversation,
		Request:      agentchat.ChatRequest{Message: "raw request"},
		Options:      agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book"},
		Identity:     agentschema.CapabilityIdentity{Kind: "context.denova-test", Version: 1},
		OnPrepared: func(agentchat.AgentContextPreparation) {
			preparedCalled = true
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fragments, err := source.Materialize(context.Background(), sdkcontext.ContextRequest{
		Run:   agentschema.RunView{ID: "run-1", CommandID: "command-1", Cycle: 2},
		Input: agent.Input{Text: "raw request"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if conversation.cycle.CommandID != "command-1" || conversation.cycle.OperationID != "run-1" || conversation.cycle.Cycle != 2 {
		t.Fatalf("cycle identity=%#v", conversation.cycle)
	}
	if conversation.kind != agentrun.AgentKindIDE || !preparedCalled {
		t.Fatalf("agent kind=%q prepared=%v", conversation.kind, preparedCalled)
	}
	if len(fragments) != 3 {
		t.Fatalf("fragments=%#v", fragments)
	}
	leading := fragments[0]
	if leading.Placement != agentschema.ContextLeadingMessage || leading.Rendering != agentschema.ContextRenderVerbatim ||
		leading.Role != agentschema.User || !strings.Contains(leading.Content, "stable body") || leading.HardLimit < minimumDenovaContextHardLimit {
		t.Fatalf("leading fragment=%#v", leading)
	}
	if fragments[1].Placement != agentschema.ContextAuditOnly || fragments[1].Content != "turn body" {
		t.Fatalf("audit fragment=%#v", fragments[1])
	}
	final := fragments[2]
	if final.Placement != agentschema.ContextFinalUserMessage || final.Rendering != agentschema.ContextRenderVerbatim ||
		final.Content != "精确的 Denova 用户消息 / exact Denova user message" || final.HardLimit < minimumDenovaContextHardLimit {
		t.Fatalf("final fragment=%#v", final)
	}
}

func TestConversationContextSourceRejectsInexactCycleIdentity(t *testing.T) {
	source, err := NewConversationContextSource(ConversationContextConfig{
		Conversation: &contextTestConversation{},
		Request:      agentchat.ChatRequest{Message: "raw request"},
		Identity:     agentschema.CapabilityIdentity{Kind: "context.denova-test", Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.Materialize(context.Background(), sdkcontext.ContextRequest{Run: agentschema.RunView{ID: "run-1", Cycle: 1}})
	if err == nil || !strings.Contains(err.Error(), "exact Agent cycle identity") {
		t.Fatalf("error=%v", err)
	}
}

func TestConversationBoundarySharesExactPreparationAcrossCanonicalAndContext(t *testing.T) {
	conversation := &contextTestConversation{}
	committer := &boundaryCommitterProbe{}
	boundary, err := NewConversationBoundary(ConversationBoundaryConfig{
		Conversation:      conversation,
		Request:           agentchat.ChatRequest{Message: "raw request"},
		Options:           agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book"},
		ContextIdentity:   agentschema.CapabilityIdentity{Kind: "context.boundary-test", Version: 1},
		CanonicalIdentity: agentschema.CapabilityIdentity{Kind: "canonical.boundary-test", Version: 1},
		Committer:         committer,
	})
	if err != nil {
		t.Fatal(err)
	}
	identity := agentcanonical.CommitIdentity{CommandID: "command-1", RunID: "run-1", Cycle: 1, Stage: agentcanonical.CommitInput}
	if _, err := boundary.CanonicalAdapter().MaterializeInput(context.Background(), agentcanonical.InputCommitRequest{
		Identity: identity, Hash: "input-hash", Input: agent.Text("raw request"),
	}); err != nil {
		t.Fatal(err)
	}
	fragments, err := boundary.ContextSource().Materialize(context.Background(), sdkcontext.ContextRequest{
		Run: agentschema.RunView{ID: "run-1", CommandID: "command-1", Cycle: 1}, Input: agent.Text("raw request"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if conversation.assemblies != 1 || len(fragments) == 0 {
		t.Fatalf("assemblies=%d fragments=%#v", conversation.assemblies, fragments)
	}
	outputIdentity := identity
	outputIdentity.Stage = agentcanonical.CommitOutput
	if _, err := boundary.CanonicalAdapter().CommitOutput(context.Background(), agentcanonical.OutputCommitRequest{
		Identity: outputIdentity, Hash: "output-hash", Message: *agentschema.AssistantMessage("answer", nil),
	}); err != nil {
		t.Fatal(err)
	}
	if conversation.assemblies != 1 || len(committer.outputPreparation.ModelContext.Messages) == 0 ||
		committer.outputPreparation.ModelContext.Messages[0].Content != committer.preparedContext.ModelContext.Messages[0].Content {
		t.Fatalf("canonical stages did not share one preparation")
	}
}

func TestConversationBoundaryCommitsProductProjectedOutputWithRawAgentHash(t *testing.T) {
	conversation := &contextTestConversation{}
	committer := &boundaryCommitterProbe{}
	boundary, err := NewConversationBoundary(ConversationBoundaryConfig{
		Conversation:      conversation,
		Request:           agentchat.ChatRequest{Message: "continue"},
		Options:           agentrun.Options{AgentKind: agentrun.AgentKindInteractiveStory},
		ContextIdentity:   agentschema.CapabilityIdentity{Kind: "context.output-projection-test", Version: 1},
		CanonicalIdentity: agentschema.CapabilityIdentity{Kind: "canonical.output-projection-test", Version: 1},
		Committer:         committer,
		ProjectOutput: func(output agentcanonical.OutputCommitRequest) (*agentschema.Message, *agentcanonical.OutputProjection) {
			if output.Message.Content != "" {
				t.Fatalf("raw Agent output = %#v", output.Message)
			}
			projected := output.Message.Clone()
			projected.Content = "durable story narrative"
			return projected, &agentcanonical.OutputProjection{Content: projected.Content}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := boundary.CanonicalAdapter().CommitOutput(context.Background(), agentcanonical.OutputCommitRequest{
		Identity: agentcanonical.CommitIdentity{CommandID: "command-1", RunID: "run-1", Cycle: 1, Stage: agentcanonical.CommitOutput},
		Hash:     "raw-agent-output-hash",
		Message:  *agentschema.AssistantMessage("", nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if committer.outputRequest.Message.Content != "durable story narrative" || committer.outputRequest.Hash != "raw-agent-output-hash" {
		t.Fatalf("committed output = %#v", committer.outputRequest)
	}
	if receipt.Transcript == nil || receipt.Transcript.Content != "durable story narrative" {
		t.Fatalf("output receipt = %#v", receipt)
	}
}

func TestConversationBoundaryRematerializesSameCycleAfterAgentCompaction(t *testing.T) {
	conversation := &contextTestConversation{}
	committer := &boundaryCommitterProbe{}
	boundary, err := NewConversationBoundary(ConversationBoundaryConfig{
		Conversation:      conversation,
		Request:           agentchat.ChatRequest{Message: "raw request"},
		Options:           agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book"},
		ContextIdentity:   agentschema.CapabilityIdentity{Kind: "context.boundary-compaction-test", Version: 1},
		CanonicalIdentity: agentschema.CapabilityIdentity{Kind: "canonical.boundary-compaction-test", Version: 1},
		Committer:         committer,
	})
	if err != nil {
		t.Fatal(err)
	}
	run := agentschema.RunView{ID: "run-compaction", CommandID: "command-compaction", Cycle: 1}
	request := sdkcontext.ContextRequest{Run: run, Input: agent.Text("raw request")}
	if _, err := boundary.ContextSource().Materialize(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Compaction = &agentcompaction.CompactionState{
		ID: "checkpoint-1", Revision: 1, Summary: "summary", SourceMessageCount: 1,
	}
	if _, err := boundary.ContextSource().Materialize(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if conversation.assemblies != 2 {
		t.Fatalf("same-cycle compaction assemblies=%d, want 2", conversation.assemblies)
	}
	identity := agentcanonical.CommitIdentity{CommandID: run.CommandID, RunID: run.ID, Cycle: run.Cycle, Stage: agentcanonical.CommitOutput}
	if _, err := boundary.CanonicalAdapter().CommitOutput(context.Background(), agentcanonical.OutputCommitRequest{
		Identity: identity, Hash: "output-hash", Message: *agentschema.AssistantMessage("answer", nil),
	}); err != nil {
		t.Fatal(err)
	}
	if conversation.assemblies != 2 {
		t.Fatalf("output commit rebuilt newest compaction context: assemblies=%d", conversation.assemblies)
	}
}
