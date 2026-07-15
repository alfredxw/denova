package interactive

import (
	"strings"
	"testing"

	"denova/internal/book"
)

func TestParseDirectorLoreContextReferencesSeparatesActiveCandidateAndOffstage(t *testing.T) {
	content := strings.Replace(defaultDirectorLoreContextDocument(), "## 当前\n", "## 当前\n\n- [[沈凝]]：当前见证者\n", 1)
	content = strings.Replace(content, "## 候场\n", "## 候场\n\n- [[戒律长老]]：规则破坏时入场\n", 1)
	content = strings.Replace(content, "## 暂离场\n", "## 暂离场\n\n- [[罗衡]]：暂时离开\n", 1)
	refs := ParseDirectorLoreContextReferences(content)
	if strings.Join(refs.Active, ",") != "沈凝" || strings.Join(refs.Candidates, ",") != "戒律长老" || strings.Join(refs.Offstage, ",") != "罗衡" {
		t.Fatalf("unexpected lore context refs: %#v", refs)
	}
	visible := ExtractDirectorLoreContextActiveSection(content)
	if !strings.Contains(visible, "沈凝") || strings.Contains(visible, "戒律长老") || strings.Contains(visible, "罗衡") {
		t.Fatalf("visible lore context should contain active refs only:\n%s", visible)
	}
}

func TestUpdateDirectorPlanIgnoresUnavailableNameReferences(t *testing.T) {
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
	disabled := false
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{ID: "disabled", Enabled: &disabled, Type: "character", Name: "未启用的人", Content: "不会加载"}); err != nil {
		t.Fatal(err)
	}
	plan.Docs.LoreContext = strings.Replace(plan.Docs.LoreContext, "## 当前\n", "## 当前\n\n- [[不存在的人]]\n- [[未启用的人]]\n", 1)
	plan.Docs.LoreContext = strings.Replace(plan.Docs.LoreContext, "## 候场\n", "## 候场\n\n- [[不存在的人]]\n", 1)
	updated, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{BranchID: "main", Docs: plan.Docs, BaseRevision: plan.Metadata.Revision})
	if err != nil {
		t.Fatalf("unavailable lore references should be ignored: %v", err)
	}
	if !strings.Contains(updated.Docs.LoreContext, "[[不存在的人]]") || !strings.Contains(updated.Docs.LoreContext, "[[未启用的人]]") {
		t.Fatalf("ignored references should remain in the Director workset for later correction:\n%s", updated.Docs.LoreContext)
	}
}

func TestDirectorLoreContextRejectsUnknownLifecycleHeading(t *testing.T) {
	content := defaultDirectorLoreContextDocument() + "\n\n## 当前角色\n\n- [[沈凝]]"
	if err := validateDirectorLoreContextDoc(content); err == nil || !strings.Contains(err.Error(), "请改为 当前、候场或暂离场") {
		t.Fatalf("unknown H2 should produce an actionable validation error: %v", err)
	}
}

func TestDirectorLoreContextAllowsOnDemandRulesButRejectsResidentDuplicates(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "规则加载边界"})
	if err != nil {
		t.Fatal(err)
	}
	lore := book.NewLoreStore(workspace)
	if _, err := lore.Create(book.LoreItemInput{ID: "check", Type: "rule", Name: "按需检定规则", LoadMode: book.LoreLoadModeAuto, Content: "只在场景需要时加载。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lore.Create(book.LoreItemInput{ID: "resident", Type: "world", Name: "常驻世界底线", LoadMode: book.LoreLoadModeResident, Content: "始终完整注入。"}); err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	plan.Docs.LoreContext = strings.Replace(plan.Docs.LoreContext, "## 当前\n", "## 当前\n\n- [[按需检定规则]]\n", 1)
	if _, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{BranchID: "main", Docs: plan.Docs, BaseRevision: plan.Metadata.Revision}); err != nil {
		t.Fatalf("on-demand rule should be available through the active workset: %v", err)
	}

	updated, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	updated.Docs.LoreContext = strings.Replace(updated.Docs.LoreContext, "## 当前\n", "## 当前\n\n- [[常驻世界底线]]\n", 1)
	if _, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{BranchID: "main", Docs: updated.Docs, BaseRevision: updated.Metadata.Revision}); err == nil || !strings.Contains(err.Error(), "常驻资料") {
		t.Fatalf("resident Lore should not be duplicated in the workset: %v", err)
	}
}
