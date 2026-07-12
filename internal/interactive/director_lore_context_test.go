package interactive

import (
	"strings"
	"testing"

	"denova/internal/book"
)

func TestParseDirectorLoreContextReferencesSeparatesActiveCandidateAndOffstage(t *testing.T) {
	content := strings.Replace(defaultDirectorLoreContextDocument(), "## 当前角色\n", "## 当前角色\n\n- [[沈凝]]：当前见证者\n", 1)
	content = strings.Replace(content, "## 候场角色\n", "## 候场角色\n\n- [[戒律长老]]：规则破坏时入场\n", 1)
	content = strings.Replace(content, "## 暂离场角色\n", "## 暂离场角色\n\n- [[罗衡]]：暂时离开\n", 1)
	refs := ParseDirectorLoreContextReferences(content)
	if strings.Join(refs.Active, ",") != "沈凝" || strings.Join(refs.Candidates, ",") != "戒律长老" || strings.Join(refs.Offstage, ",") != "罗衡" {
		t.Fatalf("unexpected lore context refs: %#v", refs)
	}
	visible := ExtractDirectorLoreContextActiveSection(content)
	if !strings.Contains(visible, "沈凝") || strings.Contains(visible, "戒律长老") || strings.Contains(visible, "罗衡") {
		t.Fatalf("visible lore context should contain active refs only:\n%s", visible)
	}
}

func TestUpdateDirectorPlanValidatesNameReferences(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "引用校验"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	plan.Docs.LoreContext = strings.Replace(plan.Docs.LoreContext, "## 当前角色\n", "## 当前角色\n\n- [[不存在的人]]\n", 1)
	if _, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{BranchID: "main", Docs: plan.Docs, BaseRevision: plan.Metadata.Revision}); err == nil || !strings.Contains(err.Error(), "不存在或未启用") {
		t.Fatalf("expected missing lore reference validation, got %v", err)
	}
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{ID: "known", Type: "character", Name: "不存在的人", Content: "已补入资料库"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{BranchID: "main", Docs: plan.Docs, BaseRevision: plan.Metadata.Revision}); err != nil {
		t.Fatalf("valid unique name reference should save: %v", err)
	}
}
