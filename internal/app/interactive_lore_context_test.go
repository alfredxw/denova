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
	plan.Docs.LoreContext = strings.Replace(plan.Docs.LoreContext, "## 当前角色\n", "## 当前角色\n\n- [[沈凝]]：当前见证者\n", 1)
	plan.Docs.LoreContext = strings.Replace(plan.Docs.LoreContext, "## 候场角色\n", "## 候场角色\n\n- [[戒律长老]]：规则破坏时入场\n", 1)
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
	turn := interactive.TurnEvent{ModelContextMessages: []interactive.ModelContextMessage{{
		Role: "assistant",
		ToolCalls: []interactive.ModelContextToolCall{{Function: interactive.ModelContextFunctionCall{
			Name:      "read_lore_items",
			Arguments: `{"names":["洛青衣"]}`,
		}}},
	}}}
	context, err := buildInteractiveDirectorLoreContext(workspace, plan, turn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "[[洛青衣]]") || !strings.Contains(context, "临时读取") || !strings.Contains(context, "首次资料审阅") {
		t.Fatalf("director lore context should expose temporary recall and revision state:\n%s", context)
	}
}
