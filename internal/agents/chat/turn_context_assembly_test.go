package chat

import (
	"context"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	agentreview "denova/internal/agents/review"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/book/lore"
)

func fixedTurnRuntimeEnvironment(service *book.Service) turnRuntimeEnvironment {
	workspace := ""
	if service != nil {
		workspace = service.Workspace()
	}
	return turnRuntimeEnvironment{
		CapturedAt: time.Date(2026, 7, 24, 7, 30, 20, 0, time.UTC),
		Workspace:  workspace,
	}
}

type pureTurnTestConversation struct {
	budget agentcontext.Budget
}

func (pureTurnTestConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage(input.UserMessage)}, Fragments: input.Fragments,
	})
	return agentcontext.ModelContextResult{Messages: assembled.Messages, Context: assembled}, err
}
func (pureTurnTestConversation) AppendAssistant(string) error                 { return nil }
func (pureTurnTestConversation) MarkInterrupted(string, string, string) error { return nil }
func (pureTurnTestConversation) PendingInterruption() *session.Interruption   { return nil }
func (pureTurnTestConversation) ResolveInterruption(string) error             { return nil }
func (c pureTurnTestConversation) ModelContextBudget() agentcontext.Budget    { return c.budget }

func assembleTurnForTest(t *testing.T, req ChatRequest, pending *session.Interruption, service *book.Service, budget agentcontext.Budget) (turnInputProjection, agentcontext.ModelContextResult) {
	t.Helper()
	turn, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation:        pureTurnTestConversation{budget: budget},
		Request:             req,
		PendingInterruption: pending,
		BookService:         service,
		Environment:         fixedTurnRuntimeEnvironment(service),
	})
	if err != nil {
		t.Fatal(err)
	}
	return turnInputProjection{
		OriginalMessage:    turn.OriginalMessage,
		Fragments:          turn.ModelContext.Context.Fragments,
		ResumeInterruption: turn.ResumeInterruption,
	}, turn.ModelContext
}

func finalAssembledUserMessage(t *testing.T, assembled agentcontext.ModelContextResult) string {
	t.Helper()
	for index := len(assembled.Messages) - 1; index >= 0; index-- {
		if assembled.Messages[index] != nil && assembled.Messages[index].Role == agent.User {
			return assembled.Messages[index].Content
		}
	}
	t.Fatal("assembled context has no user message")
	return ""
}

func TestTurnInputProjectionProjectsEverySourceAsAuditableFragment(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "references", "voice.md"), []byte("reference body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lore.NewStore(workspace).Create(lore.ItemInput{ID: "hero", Type: "character", Name: "Hero", Content: "lore body"}); err != nil {
		t.Fatal(err)
	}
	composition, assembled := assembleTurnForTest(t, ChatRequest{
		Message:        "Revise the scene",
		PlanMode:       true,
		WritingSkill:   "novel-standard",
		References:     []string{"references/voice.md"},
		LoreReferences: []string{"hero"},
		Selections:     []TextSelectionRef{{FileName: "chapter.md", StartLine: 2, EndLine: 4, Content: "selected body"}},
		ResolvedReviewFeedback: agentreview.Contexts{{
			ReviewThreadID: "review-1",
			Comments:       []agentreview.Comment{{ID: "comment-1", Body: "review body"}},
		}},
	}, nil, book.NewService(workspace), agentcontext.DefaultBudget())

	wantSources := []string{
		"runtime.environment",
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

func TestTurnInputProjectionProvidesCurrentRuntimeEnvironment(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	environment := turnRuntimeEnvironment{
		CapturedAt: time.Date(2026, 7, 24, 15, 30, 20, 0, location),
		Workspace:  "/Users/creator/novel",
	}
	turn, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation: pureTurnTestConversation{budget: agentcontext.DefaultBudget()},
		Request:      ChatRequest{Message: "现在几点？"},
		Environment:  environment,
	})
	if err != nil {
		t.Fatal(err)
	}
	assembled := turn.ModelContext

	wantContent := "- 上下文快照时间 / Captured at: 2026-07-24T15:30:20+08:00\n" +
		"- 时区 / Time zone: Asia/Shanghai (UTC+08:00)\n" +
		"- 当前工作区 / Workspace: /Users/creator/novel\n" +
		"- 说明 / Note: 这是现实运行环境的本轮快照，不是作品或互动故事中的世界时间；作品时间线仍以作品状态为准。 / This is a turn-scoped real-world runtime snapshot, not in-story time; story chronology remains governed by workspace state."
	if len(assembled.Context.Fragments) == 0 {
		t.Fatal("assembled context has no runtime environment fragment")
	}
	fragment := assembled.Context.Fragments[0]
	if fragment.ID != "runtime_environment" || fragment.Source != "runtime.environment" ||
		fragment.Title != "当前运行环境 / Current runtime environment" ||
		fragment.Placement != agentcontext.PlacementFinalUserPrefix || !fragment.Included ||
		fragment.Purpose == "" || fragment.Hash == "" || fragment.Content != wantContent {
		t.Fatalf("runtime environment fragment = %#v", fragment)
	}
	final := finalAssembledUserMessage(t, assembled)
	environmentIndex := strings.Index(final, "# 当前运行环境 / Current runtime environment")
	requestIndex := strings.Index(final, "# 本轮用户请求（最高优先级）")
	if environmentIndex < 0 || requestIndex < 0 || environmentIndex >= requestIndex || !strings.HasSuffix(strings.TrimSpace(final), "现在几点？") {
		t.Fatalf("runtime environment must precede the authoritative user request:\n%s", final)
	}
}

func TestPrepareAgentContextUsesDurableCycleStartAndOmitsClockFromStructuralPreparation(t *testing.T) {
	startedAt := time.Date(2026, 8, 13, 9, 45, 30, 0, time.FixedZone("UTC+08:00", 8*60*60))
	prepared, err := PrepareAgentContext(
		context.Background(),
		pureTurnTestConversation{budget: agentcontext.DefaultBudget()},
		ChatRequest{Message: "继续写作"},
		nil,
		"/workspace/book",
		startedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	final := finalAssembledUserMessage(t, prepared.ModelContext)
	if !strings.Contains(final, "Captured at: 2026-08-13T09:45:30+08:00") {
		t.Fatalf("real turn did not use durable cycle time:\n%s", final)
	}

	structural, err := PrepareAgentContext(
		context.Background(),
		pureTurnTestConversation{budget: agentcontext.DefaultBudget()},
		ChatRequest{Message: "compact"},
		nil,
		"/workspace/book",
		time.Time{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if final := finalAssembledUserMessage(t, structural.ModelContext); strings.Contains(final, "Captured at:") {
		t.Fatalf("structural preparation injected a process wall clock:\n%s", final)
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
	maxTotalBytes := 4096
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
		MaxFragmentBytes:      &maxFragmentBytes,
		MaxTotalInjectedBytes: &maxTotalBytes,
	}}}
	conversation := agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
		sess, cfg, config.AgentKindIDE,
		"Stable workspace", strings.Repeat("stable-", 80),
		"Dynamic workspace", strings.Repeat("dynamic-", 80),
	)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: "assembly-command", OperationID: "assembly-operation", Cycle: 1})

	result, err := conversation.AssembleModelContext(context.Background(), "continue", agentcontext.ModelContextInput{
		UserMessage: "continue",
		Budget:      agentcontext.ContextBudgetForAgent(cfg, config.AgentKindIDE),
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
	if _, pending, err := conversation.PendingAgentCycleCommit(agentrun.DomainCommitInput); err != nil || pending {
		t.Fatalf("pure assembly staged an input domain intent pending=%t err=%v", pending, err)
	}
	if err := agentcontext.CommitModelInput(context.Background(), conversation, "continue", result); err != nil {
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
	identity := agentrun.CycleIdentity{CommandID: "materialized-command", OperationID: "materialized-operation", Cycle: 1}
	references := []agentcontext.UserReference{{Kind: "file", Label: "chapters/01.md"}}
	intent, err := session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, agent.UserMessage("继续写"), session.MessageMetadata{AgentKind: config.AgentKindIDE, UserReferences: references})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CommitDomainMessage(intent); err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	conversation.BindAgentCycleIdentity(identity)
	assembled, err := conversation.AssembleModelContext(context.Background(), "继续写", agentcontext.ModelContextInput{
		UserMessage:    "继续写",
		UserReferences: references,
		Budget:         conversation.ModelContextBudget(),
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
	if err := agentcontext.CommitModelInput(context.Background(), conversation, "different projection text must not change canonical input", assembled); err != nil {
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
	conversation := agentconversation.NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: "snapshot-command", OperationID: "snapshot-operation", Cycle: 1})
	assembled, err := conversation.AssembleModelContext(context.Background(), "stale input", agentcontext.ModelContextInput{
		UserMessage: "stale input", Budget: conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessage(agent.UserMessage("concurrent structural context")); err != nil {
		t.Fatal(err)
	}
	if err := agentcontext.CommitModelInput(context.Background(), conversation, "stale input", assembled); !errors.Is(err, session.ErrContextRevisionConflict) {
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
	conversation := agentconversation.NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	assembled, err := conversation.AssembleModelContext(context.Background(), "analysis-only", agentcontext.ModelContextInput{
		UserMessage: "analysis-only", Budget: conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := agentcontext.CommitModelInput(context.Background(), conversation, "analysis-only", assembled); !errors.Is(err, agentconversation.ErrMissingAgentCycleIdentity) {
		t.Fatalf("identity-free commit error = %v, want fail-closed identity error", err)
	}
	if got := sess.MessageCountTotal(); got != 0 {
		t.Fatalf("identity-free commit mutated session history: %d", got)
	}
}
