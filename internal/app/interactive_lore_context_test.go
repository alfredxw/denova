package app

import (
	"strings"
	"testing"

	"denova/config"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestInteractiveStoryLoadsAllResidentLoreAndActiveOnDemandLore(t *testing.T) {
	workspace := t.TempDir()
	lore := book.NewLoreStore(workspace)
	for _, input := range []book.LoreItemInput{
		{ID: "rule", Type: "world", Name: "公开比试规则", LoadMode: book.LoreLoadModeResident, Content: "公开比试禁止场外偷袭。"},
		{ID: "active", Type: "character", Name: "沈凝", Content: "沈凝不会无证据帮助任何人。"},
		{ID: "candidate", Type: "character", Name: "戒律长老", Keywords: []string{"演武场"}, Content: "戒律长老掌握隐藏裁决权。"},
	} {
		if _, err := lore.Create(input); err != nil {
			t.Fatal(err)
		}
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "资料工作集", Origin: "主角报名公开比试"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	plan.Docs.LoreContext = strings.Replace(plan.Docs.LoreContext, "## 当前\n", "## 当前\n\n- [[沈凝]]：当前见证者\n", 1)
	plan.Docs.LoreContext = strings.Replace(plan.Docs.LoreContext, "## 候场\n", "## 候场\n\n- [[戒律长老]]：规则破坏时入场\n", 1)
	if _, err := store.UpdateDirectorPlan(story.ID, interactive.UpdateDirectorPlanRequest{BranchID: "main", Docs: plan.Docs, BaseRevision: plan.Metadata.Revision}); err != nil {
		t.Fatal(err)
	}
	conversation := newInteractiveConversation(store, "", workspace, story.ID, "main", "", story.ReplyTargetChars, &config.Config{})
	messages, err := conversation.PrepareMessages("", "我走进演武场")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(messages[0].Content, "公开比试禁止场外偷袭") {
		t.Fatalf("resident lore should be a stable leading message:\n%s", messages[0].Content)
	}
	instruction := messages[len(messages)-1].Content
	if !strings.Contains(instruction, "沈凝不会无证据帮助任何人") {
		t.Fatalf("active on-demand lore missing from instruction:\n%s", instruction)
	}
	if strings.Contains(instruction, "戒律长老掌握隐藏裁决权") {
		t.Fatalf("keyword matches must not auto-inject on-demand lore:\n%s", instruction)
	}
}

func TestDirectorReceivesCommittedTemporaryLoreRecallForPromotion(t *testing.T) {
	workspace := t.TempDir()
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{ID: "luo", Type: "character", Name: "洛青衣", Content: "洛青衣完整设定"}); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "临时召回"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	turn := interactive.TurnEvent{ModelContextMessages: []interactive.ModelContextMessage{
		{
			Role: "assistant",
			ToolCalls: []interactive.ModelContextToolCall{{
				ID: "call-read-lore",
				Function: interactive.ModelContextFunctionCall{
					Name:      "read_lore_items",
					Arguments: `{"names":["洛青衣"]}`,
				},
			}},
		},
		{
			Role:       "tool",
			ToolCallID: "call-read-lore",
			ToolName:   "read_lore_items",
			Content:    "# 资料库条目\n\n## 洛青衣（character / important / auto）\nID：luo\n\n```markdown\n洛青衣完整设定\n```",
		},
	}}
	context, err := buildInteractiveDirectorLoreContext(workspace, plan, turn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "[[洛青衣]]") || !strings.Contains(context, "临时读取") || !strings.Contains(context, "首次资料审阅") {
		t.Fatalf("director lore context should expose temporary recall and revision state:\n%s", context)
	}
}

func TestDirectorRecognizesOnlySuccessfulFullLoreToolResultsAsTemporaryRecalls(t *testing.T) {
	items := []book.LoreItem{
		{ID: "full", Name: "完整命中"},
		{ID: "failed", Name: "失败调用"},
		{ID: "index", Name: "目录命中"},
	}
	messages := []interactive.ModelContextMessage{
		{Role: "assistant", ToolCalls: []interactive.ModelContextToolCall{
			{ID: "call-full", Function: interactive.ModelContextFunctionCall{Name: "list_lore_items", Arguments: `{"keywords":["完整"],"detail":"full"}`}},
			{ID: "call-failed", Function: interactive.ModelContextFunctionCall{Name: "read_lore_items", Arguments: `{"ids":["failed"]}`}},
			{ID: "call-index", Function: interactive.ModelContextFunctionCall{Name: "list_lore_items", Arguments: `{"keywords":["目录"]}`}},
		}},
		{Role: "tool", ToolCallID: "call-full", Content: "# 资料库条目\n\n## 完整命中（character / important / auto）\nID：full\n\n```markdown\n正文中可能出现并非回执的字段\nID：failed\n```"},
		{Role: "tool", ToolCallID: "call-failed", Content: "资料正文累计超过本任务上下文上限"},
		{Role: "tool", ToolCallID: "call-index", Content: "# 资料库索引\n\n- [index] 目录命中"},
	}

	got := formatTemporaryLoreRecalls(items, messages)
	if !strings.Contains(got, "[[完整命中]]") {
		t.Fatalf("detail=full result should be recognized as a temporary recall:\n%s", got)
	}
	if strings.Contains(got, "失败调用") || strings.Contains(got, "目录命中") {
		t.Fatalf("failed and index-only calls must not become read receipts:\n%s", got)
	}
}

func TestDirectorLoreRosterIsInjectedOnEveryRun(t *testing.T) {
	workspace := t.TempDir()
	lore := book.NewLoreStore(workspace)
	for _, input := range []book.LoreItemInput{
		{ID: "resident", Type: "rule", Name: "常驻规则", Importance: "major", LoadMode: book.LoreLoadModeResident, Content: "常驻正文"},
		{ID: "hero", Type: "character", Name: "沈凝", Importance: "major", LoadMode: book.LoreLoadModeAuto, Content: "沈凝正文"},
		{ID: "faction", Type: "faction", Name: "戒律堂", Importance: "important", LoadMode: book.LoreLoadModeAuto, Content: "戒律堂正文"},
	} {
		if _, err := lore.Create(input); err != nil {
			t.Fatal(err)
		}
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "目录注入"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}

	opening, err := buildInteractiveDirectorLoreContext(workspace, plan, interactive.TurnEvent{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"资料名称目录", "[character/major] 沈凝", "[faction/important] 戒律堂"} {
		if !strings.Contains(opening, want) {
			t.Fatalf("opening roster missing %q:\n%s", want, opening)
		}
	}
	if strings.Contains(opening, "[rule/major] 常驻规则") || strings.Contains(opening, "常驻正文") || strings.Contains(opening, "沈凝正文") {
		t.Fatalf("dynamic director Lore should exclude resident context and all catalog bodies:\n%s", opening)
	}
	stable, err := buildInteractiveDirectorStableContext(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stable.Content, "常驻正文") || strings.Contains(stable.Content, "沈凝正文") || !strings.Contains(stable.Title, "complete=true") {
		t.Fatalf("resident Lore should use its own complete stable source: %#v", stable)
	}

	currentRevision, err := lore.Revision()
	if err != nil {
		t.Fatal(err)
	}
	plan.Metadata.LoreRevision = currentRevision
	regular, err := buildInteractiveDirectorLoreContext(workspace, plan, interactive.TurnEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(regular, "资料名称目录") || !strings.Contains(regular, "[character/major] 沈凝") {
		t.Fatalf("ordinary patches should retain the bounded discovery roster:\n%s", regular)
	}

}
