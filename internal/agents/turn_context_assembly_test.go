package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
	"denova/internal/book"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

var modelContextTestCycle atomic.Uint64

type pureTurnTestConversation struct{}

func (pureTurnTestConversation) AssembleModelContext(ctx context.Context, _ string, input ModelContextInput) (ModelContextResult, error) {
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage(input.UserMessage)}, Fragments: input.Fragments,
	})
	return ModelContextResult{Messages: assembled.Messages, Context: assembled}, err
}
func (pureTurnTestConversation) AppendAssistant(string) error                 { return nil }
func (pureTurnTestConversation) MarkInterrupted(string, string, string) error { return nil }
func (pureTurnTestConversation) PendingInterruption() *session.Interruption   { return nil }
func (pureTurnTestConversation) ResolveInterruption(string) error             { return nil }

func assembleTurnForTest(t *testing.T, req ChatRequest, pending *session.Interruption, service *book.Service, budget agentcontext.Budget) (turnInputProjection, ModelContextResult) {
	t.Helper()
	projection := projectTurnInput(req, pending, service, budget)
	assembled, err := AssembleModelContext(context.Background(), pureTurnTestConversation{}, projection.OriginalMessage, ModelContextInput{
		UserMessage: req.Message, Fragments: projection.Fragments, Budget: budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection, assembled
}

func finalAssembledUserMessage(t *testing.T, assembled ModelContextResult) string {
	t.Helper()
	for index := len(assembled.Messages) - 1; index >= 0; index-- {
		if assembled.Messages[index] != nil && assembled.Messages[index].Role == agent.User {
			return assembled.Messages[index].Content
		}
	}
	t.Fatal("assembled context has no user message")
	return ""
}

func assembleAndCommitModelContextForTest(conversation Conversation, originalMessage, userMessage string) ([]*agent.Message, error) {
	if sessionConversation, ok := conversation.(*SessionConversation); ok {
		identity := sessionConversation.agentCycleIdentitySnapshot()
		_, alreadyCommitted := sessionConversation.LastAgentCycleCommitReceipt(HarnessDomainCommitInput)
		if !validHarnessCycleIdentity(identity) || alreadyCommitted {
			cycle := modelContextTestCycle.Add(1)
			sessionConversation.BindAgentCycleIdentity(HarnessCycleIdentity{
				CommandID:   runstate.CommandID(fmt.Sprintf("test-command-%d", cycle)),
				OperationID: runstate.OperationID(fmt.Sprintf("test-operation-%d", cycle)),
				Cycle:       1,
			})
		}
	}
	result, err := AssembleModelContext(context.Background(), conversation, originalMessage, ModelContextInput{
		UserMessage: userMessage,
		Budget:      modelContextBudgetForConversation(conversation),
	})
	if err == nil {
		err = CommitModelInput(context.Background(), conversation, originalMessage, result)
	}
	return result.Messages, err
}

func TestTurnInputProjectionProjectsEverySourceAsAuditableFragment(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "references", "voice.md"), []byte("reference body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{ID: "hero", Type: "character", Name: "Hero", Content: "lore body"}); err != nil {
		t.Fatal(err)
	}
	composition, assembled := assembleTurnForTest(t, ChatRequest{
		Message:        "Revise the scene",
		PlanMode:       true,
		WritingSkill:   "novel-standard",
		References:     []string{"references/voice.md"},
		LoreReferences: []string{"hero"},
		Selections:     []TextSelectionRef{{FileName: "chapter.md", StartLine: 2, EndLine: 4, Content: "selected body"}},
		ResolvedReviewFeedback: ReviewFeedbackContexts{{
			ReviewThreadID: "review-1",
			Comments:       []ReviewFeedbackComment{{ID: "comment-1", Body: "review body"}},
		}},
	}, nil, book.NewService(workspace), agentcontext.DefaultBudget())

	wantSources := []string{
		"turn.rule.plan_mode",
		"turn.skill.selection",
		"workspace.file.reference",
		"workspace.lore.reference",
		"editor.selection",
		"workspace.review.feedback",
		"turn.rule.context_boundary",
	}
	if len(composition.Fragments) != len(wantSources) {
		t.Fatalf("fragments = %#v, want one fragment per explicit source", composition.Fragments)
	}
	for i, source := range wantSources {
		fragment := composition.Fragments[i]
		if fragment.Source != source || fragment.ID == "" || fragment.Purpose == "" || fragment.Placement != agentcontext.PlacementFinalUserPrefix || !fragment.Included {
			t.Fatalf("fragment[%d] = %#v, want complete provenance for %q", i, fragment, source)
		}
	}
	modelMessage := finalAssembledUserMessage(t, assembled)
	for _, content := range []string{"reference body", "lore body", "selected body", "review body", "novel-standard", "Revise the scene"} {
		if !strings.Contains(modelMessage, content) {
			t.Fatalf("assembled model input missing %q: %s", content, modelMessage)
		}
	}
}

func TestSessionConversationAssemblesTurnAndRuntimeFragmentsUnderOneBudget(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	maxFragmentBytes := 256
	maxTotalBytes := 700
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
		MaxFragmentBytes:      &maxFragmentBytes,
		MaxTotalInjectedBytes: &maxTotalBytes,
	}}}
	conversation := NewSessionConversationForAgentWithRuntimeContexts(
		sess, cfg, config.AgentKindIDE,
		"Stable workspace", strings.Repeat("stable-", 80),
		"Dynamic workspace", strings.Repeat("dynamic-", 80),
	)
	conversation.BindAgentCycleIdentity(HarnessCycleIdentity{CommandID: "assembly-command", OperationID: "assembly-operation", Cycle: 1})

	result, err := conversation.AssembleModelContext(context.Background(), "continue", ModelContextInput{
		UserMessage: "continue",
		Budget:      contextBudgetForAgent(cfg, config.AgentKindIDE),
		Fragments: []agentcontext.Fragment{
			{ID: "reference", Source: "workspace.file", Title: "@chapter.md", Purpose: "honor an explicit file reference", Content: strings.Repeat("reference-", 80), Placement: agentcontext.PlacementFinalUserPrefix, Included: true},
			{ID: "selection", Source: "editor.selection", Title: "chapter.md:L1-L2", Purpose: "edit the selected text", Content: strings.Repeat("selection-", 80), Placement: agentcontext.PlacementFinalUserPrefix, Included: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Context.InjectedBytes > maxTotalBytes {
		t.Fatalf("injected bytes = %d, budget = %d", result.Context.InjectedBytes, maxTotalBytes)
	}
	if len(result.Context.Fragments) != 4 {
		t.Fatalf("fragments = %#v, want turn and runtime fragments in one result", result.Context.Fragments)
	}
	wantSources := []string{"workspace.file", "editor.selection", "workspace.runtime.stable", "workspace.runtime.dynamic"}
	for i, want := range wantSources {
		fragment := result.Context.Fragments[i]
		if fragment.Source != want || fragment.Purpose == "" || fragment.Hash == "" || fragment.Limit != maxFragmentBytes {
			t.Fatalf("fragment[%d] = %#v, want complete bounded provenance for %q", i, fragment, want)
		}
	}
	if len(result.Messages) == 0 || result.Messages[len(result.Messages)-1].Role != agent.User || !strings.HasSuffix(strings.TrimSpace(result.Messages[len(result.Messages)-1].Content), "continue") {
		t.Fatalf("final model message does not retain the raw request at highest priority: %#v", result.Messages)
	}
	if visible := sess.History(); len(visible) != 0 {
		t.Fatalf("pure assembly changed display history: %#v", visible)
	}
	if _, pending, err := conversation.PendingAgentCycleCommit(HarnessDomainCommitInput); err != nil || pending {
		t.Fatalf("pure assembly staged an input domain intent pending=%t err=%v", pending, err)
	}
	if err := conversation.CommitModelInput(context.Background(), "continue", result); err != nil {
		t.Fatal(err)
	}
	visible := sess.History()
	if len(visible) != 1 || visible[0].Content != "continue" {
		t.Fatalf("input commit must persist only the raw user message: %#v", visible)
	}
}

func TestSessionConversationReusesMaterializedInputExactlyOnceInRealModelAssembly(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("materialized-model-input")
	if err != nil {
		t.Fatal(err)
	}
	identity := HarnessCycleIdentity{CommandID: "materialized-command", OperationID: "materialized-operation", Cycle: 1}
	references := []session.UserMessageReference{{Kind: "file", Label: "chapters/01.md"}}
	intent, err := session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, agent.UserMessage("继续写"), session.MessageMetadata{AgentKind: config.AgentKindIDE, UserReferences: references})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CommitDomainMessage(intent); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	conversation.BindAgentCycleIdentity(identity)
	conversation.SetUserMessageReferences(references)
	assembled, err := conversation.AssembleModelContext(context.Background(), "继续写", ModelContextInput{
		UserMessage: "继续写",
		Budget:      conversation.ModelContextBudget(),
		Fragments: []agentcontext.Fragment{{
			ID: "selection", Source: "editor.selection", Title: "选区", Purpose: "edit selection",
			Content: "第一段", Placement: agentcontext.PlacementFinalUserPrefix, Included: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	userMessages := 0
	for _, message := range assembled.Messages {
		if message != nil && message.Role == agent.User && strings.Contains(message.Content, "继续写") {
			userMessages++
			if message.Content == "继续写" {
				t.Fatal("materialized raw input was duplicated beside the enhanced final user message")
			}
		}
	}
	if userMessages != 1 {
		t.Fatalf("model-visible accepted input count = %d, messages=%#v", userMessages, assembled.Messages)
	}
	if err := conversation.CommitModelInput(context.Background(), "different projection text must not change canonical input", assembled); err != nil {
		t.Fatal(err)
	}
	if history := sess.History(); len(history) != 1 || history[0].Content != "继续写" {
		t.Fatalf("CommitModelInput duplicated or rewrote canonical input: %#v", history)
	}
}

func TestSessionConversationRejectsCommitAfterContextSnapshotChanges(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("snapshot-cas")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	conversation.BindAgentCycleIdentity(HarnessCycleIdentity{CommandID: "snapshot-command", OperationID: "snapshot-operation", Cycle: 1})
	assembled, err := conversation.AssembleModelContext(context.Background(), "stale input", ModelContextInput{
		UserMessage: "stale input", Budget: conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessage(agent.UserMessage("concurrent structural context")); err != nil {
		t.Fatal(err)
	}
	if err := conversation.CommitModelInput(context.Background(), "stale input", assembled); !errors.Is(err, session.ErrContextRevisionConflict) {
		t.Fatalf("stale assembly commit error = %v, want context revision conflict", err)
	}
	if got := sess.MessageCountTotal(); got != 1 {
		t.Fatalf("stale user input was published after snapshot changed: messages=%d", got)
	}
}

func TestSessionConversationCommitFailsClosedWithoutDurableCycleIdentity(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("missing-identity")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	assembled, err := conversation.AssembleModelContext(context.Background(), "analysis-only", ModelContextInput{
		UserMessage: "analysis-only", Budget: conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := conversation.CommitModelInput(context.Background(), "analysis-only", assembled); !errors.Is(err, ErrMissingAgentCycleIdentity) {
		t.Fatalf("identity-free commit error = %v, want fail-closed identity error", err)
	}
	if got := sess.MessageCountTotal(); got != 0 {
		t.Fatalf("identity-free commit mutated session history: %d", got)
	}
}
