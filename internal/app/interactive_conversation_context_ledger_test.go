package app

import (
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
)

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
	history, err := conversation.PrepareMessages("当前行动", "当前行动")
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
