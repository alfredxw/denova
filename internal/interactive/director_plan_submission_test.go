package interactive

import (
	"os"
	"strings"
	"testing"
	"time"

	"denova/internal/book"
)

type directorPatchTestRun struct {
	store        *Store
	story        StorySummary
	turn         TurnEvent
	token        DirectorPlanRunToken
	plan         DirectorPlan
	loreRevision string
	draft        *DirectorPlanUpdateDraft
}

func startDirectorPatchTestRun(t *testing.T, workspace, title string) directorPatchTestRun {
	t.Helper()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "前进", Narrative: "抵达门前。"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.DirectorPlanRunToken(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, turn.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	loreRevision, err := book.NewLoreStore(workspace).Revision()
	if err != nil {
		t.Fatal(err)
	}
	return directorPatchTestRun{
		store:        store,
		story:        story,
		turn:         turn,
		token:        token,
		plan:         plan,
		loreRevision: loreRevision,
		draft:        NewDirectorPlanUpdateDraft(plan.Docs, token),
	}
}

func directorSectionUpdate(document, baseHash, heading, content string) DirectorPlanDocumentUpdate {
	return DirectorPlanDocumentUpdate{
		Document: document,
		BaseHash: baseHash,
		Edits: []DirectorMarkdownEdit{{
			Op:      DirectorMarkdownEditReplaceSection,
			Heading: heading,
			Content: content,
		}},
	}
}

func TestStageDirectorPlanRunUpdatePublishesBriefPatchOnlyAfterFinalize(t *testing.T) {
	run := startDirectorPatchTestRun(t, t.TempDir(), "简报增量提交")
	update := directorSectionUpdate(
		DirectorDocumentAgentBrief,
		run.token.Hashes[DirectorPlanDocAgentBrief],
		"当前目标与可见钩子",
		"公开比试已经开始；本轮应承接围观者质疑，并给玩家留下回应或观察的空间。",
	)
	receipt, err := run.store.StageDirectorPlanRunUpdate(run.story.ID, "main", run.token, run.turn.ID, run.draft, DirectorPlanUpdateSubmission{
		Decision:           PlanDecision{Mode: PlanDecisionPatch, Reason: "公开局势发生实质变化"},
		Updates:            []DirectorPlanDocumentUpdate{update},
		Finalize:           true,
		SourceLoreRevision: run.loreRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Finalized || len(receipt.Accepted) != 1 || receipt.Accepted[0].Document != DirectorDocumentAgentBrief {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	staged, err := run.store.DirectorPlan(run.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if staged.Docs != run.plan.Docs || staged.Metadata.LastRun == nil || staged.Metadata.LastRun.Status != DirectorPlanStatusRunning {
		t.Fatalf("finalized draft must stay in memory before publication: %#v", staged)
	}
	finalDocs, ok := run.draft.FinalDocs()
	if !ok {
		t.Fatal("finalized draft did not expose final docs")
	}
	oldModTime := time.Unix(1_700_000_000, 0)
	for _, kind := range []string{DirectorPlanDocPlan, DirectorPlanDocAgentBrief, DirectorPlanDocLoreContext} {
		if err := os.Chtimes(run.plan.Metadata.Docs[kind].Path, oldModTime, oldModTime); err != nil {
			t.Fatal(err)
		}
	}
	completed, err := run.store.CompleteDirectorPlanRunWithDocs(run.story.ID, "main", run.token, run.turn.ID, `{"mode":"patch","reason":"公开局势发生实质变化"}`, finalDocs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(completed.Docs.AgentBrief, "公开比试已经开始") {
		t.Fatalf("brief patch was not published:\n%s", completed.Docs.AgentBrief)
	}
	if completed.Docs.Plan != run.plan.Docs.Plan || completed.Docs.LoreContext != run.plan.Docs.LoreContext {
		t.Fatal("a routine brief patch changed director.md or lore-context.md")
	}
	for _, kind := range []string{DirectorPlanDocPlan, DirectorPlanDocLoreContext} {
		info, statErr := os.Stat(run.plan.Metadata.Docs[kind].Path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if !info.ModTime().Equal(oldModTime) {
			t.Fatalf("routine brief patch rewrote unchanged %s", kind)
		}
	}
	briefInfo, err := os.Stat(run.plan.Metadata.Docs[DirectorPlanDocAgentBrief].Path)
	if err != nil {
		t.Fatal(err)
	}
	if briefInfo.ModTime().Equal(oldModTime) {
		t.Fatal("routine brief patch did not replace agent-brief.md")
	}
}

func TestStageDirectorPlanRunUpdateAcceptsFilesIndependentlyAndRetriesRejectedOnly(t *testing.T) {
	run := startDirectorPatchTestRun(t, t.TempDir(), "逐文件接受")
	briefUpdate := directorSectionUpdate(
		DirectorDocumentAgentBrief,
		run.token.Hashes[DirectorPlanDocAgentBrief],
		"状态连续性",
		"主角已经抵达门前，围观者开始关注其下一步选择。",
	)
	invalidLoreUpdate := DirectorPlanDocumentUpdate{
		Document: DirectorDocumentLoreContext,
		BaseHash: run.token.Hashes[DirectorPlanDocLoreContext],
		Edits: []DirectorMarkdownEdit{{
			Op:      DirectorMarkdownEditReplaceDocument,
			Content: "缺少固定标题",
		}},
	}
	first, err := run.store.StageDirectorPlanRunUpdate(run.story.ID, "main", run.token, run.turn.ID, run.draft, DirectorPlanUpdateSubmission{
		Decision:           PlanDecision{Mode: PlanDecisionPatch, Reason: "场景状态与候场集合变化"},
		Updates:            []DirectorPlanDocumentUpdate{briefUpdate, invalidLoreUpdate},
		Finalize:           true,
		SourceLoreRevision: run.loreRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Finalized || len(first.Accepted) != 1 || first.Accepted[0].Document != DirectorDocumentAgentBrief || len(first.Rejected) != 1 || first.Rejected[0].Document != DirectorDocumentLoreContext {
		t.Fatalf("documents were not accepted independently: %#v", first)
	}
	if len(first.RetryDocuments) != 1 || first.RetryDocuments[0] != DirectorDocumentLoreContext {
		t.Fatalf("retry should contain only failed lore file: %#v", first.RetryDocuments)
	}
	beforeRetry, err := run.store.DirectorPlan(run.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if beforeRetry.Docs != run.plan.Docs {
		t.Fatal("accepted brief leaked into the workspace before the whole draft finalized")
	}
	validLoreUpdate := directorSectionUpdate(
		DirectorDocumentLoreContext,
		run.token.Hashes[DirectorPlanDocLoreContext],
		"候场",
		"### 角色与势力\n\n暂无候场资料；只有出现明确触发条件后才加入。",
	)
	second, err := run.store.StageDirectorPlanRunUpdate(run.story.ID, "main", run.token, run.turn.ID, run.draft, DirectorPlanUpdateSubmission{
		Decision:           PlanDecision{Mode: PlanDecisionPatch, Reason: "场景状态与候场集合变化"},
		Updates:            []DirectorPlanDocumentUpdate{validLoreUpdate},
		Finalize:           true,
		SourceLoreRevision: run.loreRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Finalized || len(second.Accepted) != 1 || second.Accepted[0].Document != DirectorDocumentLoreContext {
		t.Fatalf("retry did not retain the previously accepted brief: %#v", second)
	}
	if len(second.AcceptedDocuments) != 2 || second.AcceptedDocuments[0] != DirectorDocumentAgentBrief || second.AcceptedDocuments[1] != DirectorDocumentLoreContext {
		t.Fatalf("accepted document set = %#v", second.AcceptedDocuments)
	}
	finalDocs, _ := run.draft.FinalDocs()
	completed, err := run.store.CompleteDirectorPlanRunWithDocs(run.story.ID, "main", run.token, run.turn.ID, `{"mode":"patch","reason":"场景状态与候场集合变化"}`, finalDocs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(completed.Docs.AgentBrief, "围观者开始关注") || !strings.Contains(completed.Docs.LoreContext, "暂无候场资料") {
		t.Fatal("the finalized independent patches were not published together")
	}
	if completed.Docs.Plan != run.plan.Docs.Plan {
		t.Fatal("independent brief/lore patches changed director.md")
	}
}

func TestStageDirectorPlanRunUpdateReplanRequiresPlanAndBriefButNotLore(t *testing.T) {
	run := startDirectorPatchTestRun(t, t.TempDir(), "重大偏差重规划")
	briefUpdate := directorSectionUpdate(
		DirectorDocumentAgentBrief,
		run.token.Hashes[DirectorPlanDocAgentBrief],
		"当前目标与可见钩子",
		"旧目标已经失效；玩家现在需要决定是否追踪突然出现的证据。",
	)
	first, err := run.store.StageDirectorPlanRunUpdate(run.story.ID, "main", run.token, run.turn.ID, run.draft, DirectorPlanUpdateSubmission{
		Decision:           PlanDecision{Mode: PlanDecisionReplan, Reason: "玩家选择使阶段前提失效"},
		Updates:            []DirectorPlanDocumentUpdate{briefUpdate},
		Finalize:           true,
		SourceLoreRevision: run.loreRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Finalized || len(first.RetryDocuments) != 1 || first.RetryDocuments[0] != DirectorDocumentPlan {
		t.Fatalf("replan should request only the missing private plan: %#v", first)
	}
	planUpdate := directorSectionUpdate(
		DirectorDocumentPlan,
		run.token.Hashes[DirectorPlanDocPlan],
		"阶段目标与隐藏钩子",
		"阶段目标改为追踪突然出现的证据；旧高潮失效，新的反转条件取决于玩家是否公开证据。",
	)
	second, err := run.store.StageDirectorPlanRunUpdate(run.story.ID, "main", run.token, run.turn.ID, run.draft, DirectorPlanUpdateSubmission{
		Decision:           PlanDecision{Mode: PlanDecisionReplan, Reason: "玩家选择使阶段前提失效"},
		Updates:            []DirectorPlanDocumentUpdate{planUpdate},
		Finalize:           true,
		SourceLoreRevision: run.loreRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Finalized || len(second.AcceptedDocuments) != 2 || containsString(second.AcceptedDocuments, DirectorDocumentLoreContext) {
		t.Fatalf("replan should finalize with plan+brief and unchanged lore: %#v", second)
	}
}

func TestStageDirectorPlanRunUpdateKeepFinalizesWithoutDocuments(t *testing.T) {
	run := startDirectorPatchTestRun(t, t.TempDir(), "保持现有规划")
	receipt, err := run.store.StageDirectorPlanRunUpdate(run.story.ID, "main", run.token, run.turn.ID, run.draft, DirectorPlanUpdateSubmission{
		Decision:           PlanDecision{Mode: PlanDecisionKeep, Reason: "计划仍有效"},
		Finalize:           true,
		SourceLoreRevision: run.loreRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Finalized || len(receipt.AcceptedDocuments) != 0 {
		t.Fatalf("valid keep rejected: %#v", receipt)
	}
	finalDocs, ok := run.draft.FinalDocs()
	if !ok || finalDocs != run.plan.Docs {
		t.Fatal("keep should finalize the unchanged baseline")
	}
}

func TestStageDirectorPlanRunUpdateRequiresBodyReviewForNewCast(t *testing.T) {
	workspace := t.TempDir()
	loreItem, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{ID: "shen", Type: "character", Name: "沈凝", Content: "沈凝完整人物设定。"})
	if err != nil {
		t.Fatal(err)
	}
	run := startDirectorPatchTestRun(t, workspace, "选角来源校验")
	update := directorSectionUpdate(
		DirectorDocumentLoreContext,
		run.token.Hashes[DirectorPlanDocLoreContext],
		"候场",
		"### 角色与势力\n\n- [[沈凝]]：可能在门后危机升级时入场。",
	)
	submission := DirectorPlanUpdateSubmission{
		Decision:           PlanDecision{Mode: PlanDecisionPatch},
		Updates:            []DirectorPlanDocumentUpdate{update},
		Finalize:           true,
		SourceLoreRevision: run.loreRevision,
	}
	receipt, err := run.store.StageDirectorPlanRunUpdate(run.story.ID, "main", run.token, run.turn.ID, run.draft, submission)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Rejected) != 1 || !strings.Contains(receipt.Rejected[0].Message, "尚未读取完整正文") {
		t.Fatalf("new cast should require a successful body read: %#v", receipt)
	}
	submission.ReviewedLoreIDs = []string{loreItem.ID}
	receipt, err = run.store.StageDirectorPlanRunUpdate(run.story.ID, "main", run.token, run.turn.ID, run.draft, submission)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Finalized || len(receipt.Accepted) != 1 {
		t.Fatalf("reviewed cast should be accepted: %#v", receipt)
	}
}

func TestDirectorLoreGroundingRequiresReviewWhenOffstageLoreJoinsCast(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	item, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{ID: "shen", Type: "character", Name: "沈凝", Content: "沈凝完整人物设定。"})
	if err != nil {
		t.Fatal(err)
	}
	previous := strings.Replace(defaultDirectorLoreContextDocument(), "## 暂离场\n", "## 暂离场\n\n- [[沈凝]]：过去角色\n", 1)
	next := strings.Replace(defaultDirectorLoreContextDocument(), "## 候场\n", "## 候场\n\n- [[沈凝]]：准备入场\n", 1)
	if err := store.validateDirectorLoreGrounding(previous, next, nil); err == nil || !strings.Contains(err.Error(), "尚未读取完整正文") {
		t.Fatalf("moving an unreviewed offstage entry into the cast should require a body read: %v", err)
	}
	if err := store.validateDirectorLoreGrounding(previous, next, []string{item.ID}); err != nil {
		t.Fatalf("reviewed offstage entry should be allowed to join the cast: %v", err)
	}
}

func TestStageDirectorPlanRunUpdateRejectsChangedLoreRevision(t *testing.T) {
	workspace := t.TempDir()
	run := startDirectorPatchTestRun(t, workspace, "资料并发变化")
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{ID: "new", Type: "character", Name: "新角色", Content: "新正文"}); err != nil {
		t.Fatal(err)
	}
	_, err := run.store.StageDirectorPlanRunUpdate(run.story.ID, "main", run.token, run.turn.ID, run.draft, DirectorPlanUpdateSubmission{
		Decision:           PlanDecision{Mode: PlanDecisionKeep},
		Finalize:           true,
		SourceLoreRevision: run.loreRevision,
	})
	if err == nil || !strings.Contains(err.Error(), "资料库在导演审阅期间已变化") {
		t.Fatalf("stale lore revision should be rejected: %v", err)
	}
}
