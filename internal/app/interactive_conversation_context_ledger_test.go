package app

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/agent"
	agentcontext "denova/internal/agent/context"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/prompts"
)

func TestInteractiveContextAnalysisMatchesRuntimeAssemblyWithoutSideEffects(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "纯分析", Origin: "雨夜启程"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{User: "走进旅店", Narrative: "壁炉旁安静下来。"}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Workspace: workspace}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "询问店主", 800, cfg)
	beforeStory, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}

	analysis, err := agent.BuildInteractiveStoryContextAnalysis(
		cfg,
		book.NewState(workspace),
		prompts.InteractiveStorySystemInstructionInput{ReplyTargetChars: 800},
		nil,
		agent.ChatRequest{Message: "询问店主"},
		beforeStory.Snapshot.ContextCompaction,
		conversation,
	)
	if err != nil {
		t.Fatal(err)
	}
	assembled, err := conversation.AssembleModelContext(context.Background(), "询问店主", agent.ModelContextInput{
		UserMessage: "询问店主",
		Budget:      conversation.ModelContextBudget(),
		Fragments: []agentcontext.Fragment{{
			ID:        "turn_rule_context_boundary",
			Source:    "turn.rule.context_boundary",
			Title:     "上下文边界",
			Purpose:   "keep the current user request authoritative over historical intent",
			Content:   prompts.ContextBoundary(""),
			Placement: agentcontext.PlacementFinalUserPrefix,
			Included:  true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.ContextMessages) != len(assembled.Messages) {
		t.Fatalf("analysis messages = %d, runtime assembly = %d", len(analysis.ContextMessages), len(assembled.Messages))
	}
	for i, message := range assembled.Messages {
		if analysis.ContextMessages[i].Role != string(message.Role) || analysis.ContextMessages[i].Content != message.Content {
			t.Fatalf("analysis message %d differs from runtime assembly: analysis=%#v runtime=%#v", i, analysis.ContextMessages[i], message)
		}
	}
	afterStory, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	conversation.mu.Lock()
	baseParentID := conversation.baseParentID
	lastSources := conversation.lastSources
	contextSources := append([]interactiveContextSource(nil), conversation.lastContextSources...)
	contextLedger := append([]agent.ContextLedgerPart(nil), conversation.lastContextLedgerParts...)
	stableLeadingMessage := conversation.stableLeadingMessage
	conversation.mu.Unlock()
	if !reflect.DeepEqual(beforeStory, afterStory) {
		t.Fatalf("context analysis mutated the story store: before=%#v after=%#v", beforeStory, afterStory)
	}
	if baseParentID != nil || lastSources != "" || len(contextSources) != 0 || len(contextLedger) != 0 || stableLeadingMessage != "" {
		t.Fatalf("pure analysis mutated conversation state: parent=%v sources=%q context_sources=%d ledger=%d stable=%q", baseParentID, lastSources, len(contextSources), len(contextLedger), stableLeadingMessage)
	}
}

func TestInteractiveConversationSharesOneBudgetAcrossTurnRuntimeAndResidentLore(t *testing.T) {
	workspace := t.TempDir()
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{
		ID: "resident-budget", Type: "world", Name: "预算世界", LoadMode: book.LoreLoadModeResident,
		Content: strings.Repeat("常驻设定。", 900),
	}); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "统一预算", Origin: "从门外开始"})
	if err != nil {
		t.Fatal(err)
	}
	maxFragmentBytes := 4 * 1024
	maxTotalBytes := 10 * 1024
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{InteractiveStory: config.AgentContextOverride{
		MaxFragmentBytes: &maxFragmentBytes, MaxTotalInjectedBytes: &maxTotalBytes,
	}}}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "推门", 800, cfg)
	result, err := conversation.AssembleModelContext(context.Background(), "推门", agent.ModelContextInput{
		UserMessage: "推门",
		Budget:      agent.ContextBudgetForAgent(cfg, config.AgentKindInteractiveStory),
		Fragments: []agentcontext.Fragment{{
			ID: "turn-reference", Source: "workspace.file.reference", Title: "@turn.md",
			Purpose: "provide an explicit turn reference", Content: strings.Repeat("本轮参考。", 900),
			Placement: agentcontext.PlacementFinalUserPrefix, Included: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Context.InjectedBytes > maxTotalBytes {
		t.Fatalf("interactive injected bytes = %d, budget = %d", result.Context.InjectedBytes, maxTotalBytes)
	}
	wantSources := map[string]bool{
		"workspace.file.reference":  false,
		"interactive.resident_lore": false,
		"interactive.turn_rules":    false,
		"interactive.runtime":       false,
	}
	for _, fragment := range result.Context.Fragments {
		if _, ok := wantSources[fragment.Source]; ok {
			wantSources[fragment.Source] = true
			if fragment.Purpose == "" || fragment.Hash == "" || fragment.Limit != maxFragmentBytes {
				t.Fatalf("interactive fragment lacks bounded provenance: %#v", fragment)
			}
			if fragment.Source == "interactive.turn_rules" && fragment.Included &&
				(!strings.Contains(fragment.Content, "必须显著影响本轮剧情裁定") || !strings.Contains(fragment.Content, "不要把规则文本作为正文输出")) {
				t.Fatalf("storyteller fragment lost its behavioral contract: %#v", fragment)
			}
		}
	}
	for source, seen := range wantSources {
		if !seen {
			t.Fatalf("interactive assembly did not include source %q: %#v", source, result.Context.Fragments)
		}
	}
	if len(result.Messages) == 0 || !strings.Contains(result.Messages[len(result.Messages)-1].Content, "推门") {
		t.Fatalf("interactive model messages lost the raw action: %#v", result.Messages)
	}
}

func TestResolvedInteractiveContextSourcesNeverAuditUnassembledBodiesAsVisible(t *testing.T) {
	parts := []interactiveContextSource{{
		Source: "DirectorPlan", Title: "正文 Agent 简报", Purpose: "turn runtime",
		Content: "只有未裁剪原文才包含的秘密尾段", Limit: 128,
	}}
	resolved := resolveInteractiveContextSources(parts, []*schema.Message{schema.UserMessage("只有未裁剪原文")})
	if len(resolved) != 1 || resolved[0].Content != "" || !resolved[0].Truncated || !strings.Contains(resolved[0].Note, "not_present_after_context_assembly") {
		t.Fatalf("unassembled domain source must become bounded omission metadata: %#v", resolved)
	}
	summary := interactiveContextSourceListSummary(resolved, nil)
	if strings.Contains(summary, "秘密尾段") {
		t.Fatalf("source summary leaked the unassembled source body: %s", summary)
	}
	ledger := interactiveContextLedgerParts(resolved, []*schema.Message{schema.UserMessage("只有未裁剪原文")}, agent.ToolResultContextPolicy{})
	if len(ledger) != 1 || ledger[0].Included || ledger[0].Bytes != 0 || !ledger[0].Truncated {
		t.Fatalf("ledger must describe the final omission, not the original body: %#v", ledger)
	}
}

func TestInteractiveContextLedgerUsesFinalCompactedMessages(t *testing.T) {
	workspace := t.TempDir()
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{
		ID: "resident-world", Type: "world", Name: "常驻世界", LoadMode: book.LoreLoadModeResident,
		Content: "常驻规则必须在压缩后继续完整可见。",
	}); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	actorSystem := interactive.DefaultActorStateModule().ActorState
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "只作元数据的标题", Origin: "只作元数据的开端", ActorState: &actorSystem,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		User: "应被压缩移除的旧行动", Narrative: "应被压缩移除的旧剧情",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		User: "保留的近期行动", Narrative: "保留的近期剧情",
		ModelContextMessages: []interactive.ModelContextMessage{
			{
				Role: "assistant",
				ToolCalls: []interactive.ModelContextToolCall{{
					ID: "call-lore", Type: "function",
					Function: interactive.ModelContextFunctionCall{Name: "read_lore_items", Arguments: `{"ids":["source-lore"]}`},
				}},
			},
			{
				Role: "tool", ToolName: "read_lore_items", ToolCallID: "call-lore",
				Content: "## 来源资料（world）\nID：source-lore\n\n不应跨轮复制的完整资料正文。",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "当前行动", 800, &config.Config{})
	history, err := assembleAndCommitInteractiveContextForTest(conversation, "当前行动", "当前行动")
	if err != nil {
		t.Fatal(err)
	}
	stable := conversation.stableLeadingMessageSnapshot()
	finalMessages := agent.BuildCompactedModelMessages(history, "旧行动和剧情已压缩为有界摘要。", 2, 2)
	finalMessages = preserveInteractiveStableLeadingMessage(finalMessages, stable)
	parts := conversation.ContextLedgerPartsForMessages(finalMessages)

	if len(finalMessages) == 0 || !strings.Contains(finalMessages[0].Content, "常驻规则必须在压缩后继续完整可见") {
		t.Fatalf("resident Lore must remain the stable leading message after compaction: %#v", finalMessages)
	}
	var metadataCount int
	var resident, sawActorState, compaction, removedOld, keptRecent bool
	var toolCalls, toolResults int
	for _, part := range parts {
		if part.Hash == "" && part.Bytes > 0 {
			t.Fatalf("non-empty ledger parts must have a content hash: %#v", part)
		}
		switch {
		case part.Source == "互动故事" && (part.Title == "故事标题" || part.Title == "开端"):
			metadataCount++
			if part.Included || part.Truncated || !strings.Contains(part.Note, "metadata_only") {
				t.Fatalf("story title/origin are audit metadata, not model input: %#v", part)
			}
		case part.Source == "ResidentLore":
			resident = part.Included && !part.Truncated && part.Limit > book.ResidentLoreSafetyMaxBytes && part.LimitUnit == "bytes" &&
				part.Bytes == len([]byte(strings.TrimSpace(stable))) && strings.Contains(part.Note, "exact_final_message=true")
		case part.Source == "ActorState":
			sawActorState = part.Included && part.Limit > 0
		case part.Source == "ContextCompaction":
			compaction = part.Included && strings.Contains(part.Preview, "压缩")
		case part.Source == "历史回合" && strings.HasPrefix(part.Title, "第 1 回合"):
			removedOld = !part.Included && part.Truncated && strings.Contains(part.Note, "not_present_after_final_compaction")
		case part.Source == "历史回合" && strings.HasPrefix(part.Title, "第 2 回合"):
			keptRecent = keptRecent || part.Included
		case part.Source == "历史工具上下文" && strings.HasPrefix(part.Title, "工具调用"):
			toolCalls++
			if part.Limit != 0 || part.LimitUnit != "" || part.Truncated || !strings.Contains(part.Note, "preserved_exactly=true") || !strings.Contains(part.Note, "bounded_by=model_completion") {
				t.Fatalf("tool-call ledger must describe exact model-produced arguments: %#v", part)
			}
		case part.Source == "历史工具上下文" && strings.HasPrefix(part.Title, "工具结果"):
			toolResults++
			if strings.Contains(part.Preview, "完整资料正文") || !strings.Contains(part.Note, "semantic_filtered=true") || !strings.Contains(part.Note, "single_result_limit_bytes=") || part.Limit <= 0 || part.LimitUnit != "bytes" || !part.Truncated {
				t.Fatalf("tool-result ledger must describe the semantic cross-turn receipt, not the original body: %#v", part)
			}
		}
	}
	if metadataCount != 2 || !resident || !sawActorState || !compaction || !removedOld || !keptRecent || toolCalls != 1 || toolResults != 1 {
		t.Fatalf("final context ledger mismatch metadata=%d resident=%t actor=%t compaction=%t removed=%t recent=%t calls=%d results=%d parts=%#v", metadataCount, resident, sawActorState, compaction, removedOld, keptRecent, toolCalls, toolResults, parts)
	}
}
