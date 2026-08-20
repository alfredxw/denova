package interactive

import (
	"testing"
)

// memoryTestStory 建立一个三回合故事并注入六类记忆记录,覆盖:
// 有效/已推翻记录、伏笔开闭、一跳展开实体(蚀骨剑)与 epoch 重抽。
// 返回值中的三个 TurnEvent 供调用方引用回合 ID。
func memoryTestStory(t *testing.T) (*Store, StorySummary, TurnEvent, TurnEvent, TurnEvent) {
	t.Helper()
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "记忆检索", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "前往银月港",
		Narrative: "林舟在银月港见到了岚,得到蚀骨剑。",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "询问黑帆船",
		Narrative: "岚承认黑帆船会在午夜靠岸,并警告林舟不要提剑的来历。",
	})
	if err != nil {
		t.Fatal(err)
	}
	third, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "向岚坦白",
		Narrative: "林舟向岚坦白了自己持有蚀骨剑,师徒情谊出现裂痕。",
	})
	if err != nil {
		t.Fatal(err)
	}

	records := []NarrativeMemoryRecord{
		{ID: "mem_sword", Kind: MemoryKindObjectState, Subject: "蚀骨剑", Object: "林舟", Text: "蚀骨剑在林舟手中。", Evidence: "得到蚀骨剑", ValidFrom: first.ID},
		{ID: "mem_know_secret", Kind: MemoryKindKnowledge, Subject: "岚", Object: "剑的来历", Text: "岚知道蚀骨剑的来历但未告知林舟。", Evidence: "警告林舟不要提剑的来历", ValidFrom: second.ID},
		{ID: "mem_promise", Kind: MemoryKindPromise, Subject: "剑的来历", Text: "剑的来历尚未向读者揭示。", Evidence: "不要提剑的来历", ValidFrom: second.ID, Status: MemoryStatusOpen},
		{ID: "mem_rel_master", Kind: MemoryKindRelationship, Subject: "林舟", Object: "岚", Text: "林舟与岚是师徒。", Evidence: "师徒", ValidFrom: first.ID, ValidTo: third.ID},
		{ID: "mem_rel_break", Kind: MemoryKindRelationship, Subject: "林舟", Object: "岚", Text: "林舟与岚师徒决裂。", Evidence: "师徒情谊出现裂痕", ValidFrom: third.ID},
		{ID: "mem_beat", Kind: MemoryKindBeat, Subject: "岚", Text: "岚在第五回合承担守护者的戏剧功能。", Evidence: "守护", ValidFrom: third.ID},
	}
	if _, err := store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: first.ID,
		Records:      []NarrativeMemoryRecord{records[0], records[3]},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: second.ID,
		Records:      []NarrativeMemoryRecord{records[1], records[2]},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: third.ID,
		Records:      []NarrativeMemoryRecord{records[4], records[5]},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story, first, second, third
}

func TestProjectNarrativeMemoryValidityIntervals(t *testing.T) {
	store, story, _, second, third := memoryTestStory(t)
	meta, lines, err := store.readStoryLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, branch, err := resolveBranch(meta, "main")
	if err != nil {
		t.Fatal(err)
	}
	path, _ := eventPath(branch.Head, eventsByID(lines))
	memoryEvents, turnOrder := collectNarrativeMemory(path)

	// 最新视角:6 条记录纳入,1 条被推翻(师徒关系),5 条有效。
	projection := ProjectNarrativeMemory(memoryEvents, turnOrder, "")
	if projection.DedupedRecords != 6 {
		t.Fatalf("deduped records: %d", projection.DedupedRecords)
	}
	if projection.ValidRecords != 5 || projection.ExpiredRecords != 1 {
		t.Fatalf("valid=%d expired=%d", projection.ValidRecords, projection.ExpiredRecords)
	}

	// 第二回合视角(只看 third 之前):first(2 条)+ second(2 条)= 4 条,
	// 决裂尚未发生,师徒关系仍有效(ValidTo 指向未来回合不视为推翻)。
	early := ProjectNarrativeMemory(memoryEvents, turnOrder, third.ID)
	if early.ValidRecords != 4 || early.ExpiredRecords != 0 {
		t.Fatalf("early valid=%d expired=%d", early.ValidRecords, early.ExpiredRecords)
	}
	found := false
	for _, record := range early.Records {
		if record.ID == "mem_rel_break" {
			t.Fatalf("future record leaked into early view: %#v", record)
		}
		if record.ID == "mem_rel_master" && record.ValidTo == third.ID {
			// 记录本身在视角内存在;但 ValidTo 指向未来回合,不应视为已推翻。
			if memoryRecordExpired(record, turnPositionIndex(turnOrder), 2) {
				t.Fatal("master relation should still be valid before third turn")
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("master relation record missing from early view: %#v", early.Records)
	}

	// 第一回合视角:只有来自 first 的记录。
	firstView := ProjectNarrativeMemory(memoryEvents, turnOrder, second.ID)
	if len(firstView.Records) != 2 {
		t.Fatalf("first view records: %#v", firstView.Records)
	}
	if first, second := firstView.ValidRecords, firstView.DedupedRecords; first != 2 || second != 2 {
		t.Fatalf("first view valid=%d deduped=%d", first, second)
	}
}

func TestProjectNarrativeMemoryEpochInvalidation(t *testing.T) {
	store, story, _, _, third := memoryTestStory(t)
	// 对 third 重抽:该回合旧 epoch 的 2 条作废,新 epoch 只保留 1 条;
	// first/second 的记录不受影响(epoch 按 source_turn 分组)。
	if _, err := store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: third.ID,
		Records: []NarrativeMemoryRecord{
			{ID: "mem_rewrite", Kind: MemoryKindKnowledge, Subject: "林舟", Text: "林舟已向岚坦白。", Evidence: "坦白", ValidFrom: third.ID},
		},
	}); err != nil {
		t.Fatal(err)
	}
	meta, lines, err := store.readStoryLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, branch, err := resolveBranch(meta, "main")
	if err != nil {
		t.Fatal(err)
	}
	path, _ := eventPath(branch.Head, eventsByID(lines))
	memoryEvents, turnOrder := collectNarrativeMemory(path)
	projection := ProjectNarrativeMemory(memoryEvents, turnOrder, "")
	// first 2 + second 2 + third 新 1 = 5;被作废的是 third 旧 epoch 的 2 条。
	if projection.DedupedRecords != 5 || projection.StaleRecords != 2 {
		t.Fatalf("deduped=%d stale=%d (want 5/2)", projection.DedupedRecords, projection.StaleRecords)
	}
	rewrite, breakRel, beat := false, false, false
	for _, record := range projection.Records {
		switch record.ID {
		case "mem_rewrite":
			rewrite = true
		case "mem_rel_break":
			breakRel = true
		case "mem_beat":
			beat = true
		}
	}
	if !rewrite || breakRel || beat {
		t.Fatalf("rewrite=%v break=%v beat=%v records: %#v", rewrite, breakRel, beat, projection.Records)
	}
}

func TestSearchStoryMemoryKeywordAndExpansion(t *testing.T) {
	store, story, _, _, _ := memoryTestStory(t)

	result, err := store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{Keywords: []string{"蚀骨剑"}})
	if err != nil {
		t.Fatal(err)
	}
	// 直接命中:mem_sword(Subject 精确)+ mem_know_secret(Text 含"蚀骨剑")。
	// 一跳展开拉入:锚点 = 蚀骨剑/林舟/岚/剑的来历。
	// mem_rel_break(林舟)、mem_promise(剑的来历)、mem_beat(岚)被展开;
	// mem_rel_master 虽也是 Subject=林舟 但已过期(ValidTo 非空),展开不拉入。
	byID := map[string]MemorySearchHit{}
	for _, hit := range result.Hits {
		byID[hit.RecordID] = hit
	}
	direct := 0
	expanded := 0
	for _, hit := range result.Hits {
		if hit.ExpandedFrom == "" {
			direct++
		} else {
			expanded++
		}
	}
	if direct != 2 {
		t.Fatalf("direct hits: %d (%s)", direct, hitIDs(result.Hits))
	}
	if expanded != 3 {
		t.Fatalf("expanded hits: %d (%s)", expanded, hitIDs(result.Hits))
	}
	if _, ok := byID["mem_rel_master"]; ok {
		t.Fatalf("expired record must not be pulled in by expansion: %#v", byID["mem_rel_master"])
	}
	// mem_rel_master 的 Subject=林舟、Object=岚 都触及锚点,但它已被 third 回合
	// 推翻(ValidTo 非空)。展开不得带回被推翻的事实——"当时如何"要由直接关键词
	// 命中的路径覆盖,那条路径仍会保留过期记录并标注 valid_to。
	if result.Explain == nil {
		t.Fatal("explain missing")
	}
	if result.Explain.Pipeline.Anchors == 0 || result.Explain.Pipeline.ExpandedRecords != 3 {
		t.Fatalf("pipeline: %#v", result.Explain.Pipeline)
	}
	for _, detail := range result.Explain.HitDetails {
		if detail.RecordID == "mem_sword" {
			if len(detail.KeywordParts) != 1 || detail.KeywordParts[0].Field != "subject" || detail.KeywordParts[0].Score != memoryScoreSubjectExact {
				t.Fatalf("sword detail: %#v", detail)
			}
		}
	}
}

func hitIDs(hits []MemorySearchHit) string {
	ids := ""
	for i, hit := range hits {
		if i > 0 {
			ids += ","
		}
		ids += hit.RecordID + "(" + hit.ExpandedFrom + ")"
	}
	return ids
}

func TestSearchStoryMemoryPromiseBoostAndFilter(t *testing.T) {
	store, story, _, _, _ := memoryTestStory(t)

	result, err := store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{Keywords: []string{"来历"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) == 0 || result.Hits[0].RecordID != "mem_promise" {
		t.Fatalf("promise should rank first with boost: %s", hitIDs(result.Hits))
	}
	// Subject="剑的来历" 包含"来历"(60) + 伏笔加成(15) = 75。
	if result.Hits[0].Score != memoryScoreSubjectContains+memoryScorePromiseBoost {
		t.Fatalf("promise score: %d", result.Hits[0].Score)
	}
	// knowledge(岚知道剑的来历)Text 同样含"来历",无伏笔加成,排第二。
	if len(result.Hits) < 2 || result.Hits[1].RecordID != "mem_know_secret" {
		t.Fatalf("knowledge should rank second: %s", hitIDs(result.Hits))
	}

	// 未命中记录必须出现在 Filtered 并说明原因。
	filteredReasons := map[string]string{}
	for _, item := range result.Explain.Filtered {
		filteredReasons[item.RecordID] = item.Reason
	}
	if filteredReasons["mem_sword"] != "no_keyword_match" {
		t.Fatalf("filtered reasons: %#v", filteredReasons)
	}
}

func TestSearchStoryMemoryKindSubjectFilterAndBrowse(t *testing.T) {
	store, story, _, _, _ := memoryTestStory(t)

	// kind 过滤。
	result, err := store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{Kind: MemoryKindPromise})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].RecordID != "mem_promise" {
		t.Fatalf("kind filter: %s", hitIDs(result.Hits))
	}
	if result.Explain.Pipeline.Candidates != 1 {
		t.Fatalf("candidates: %d", result.Explain.Pipeline.Candidates)
	}

	// subject 过滤。
	result, err = store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{Subject: "岚"})
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range result.Hits {
		if hit.Subject != "岚" {
			t.Fatalf("subject filter leaked: %#v", hit)
		}
	}

	// 空关键词浏览:全部有效+已推翻记录,按视角排序。
	result, err = store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 6 {
		t.Fatalf("browse hits: %d (%s)", len(result.Hits), hitIDs(result.Hits))
	}

	// beforeTurnID 视角:第二回合视角看不到第三回合的记录。
	result, err = store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{BeforeTurnID: story2TurnID(t, store, story.ID, 2)})
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range result.Hits {
		if hit.ValidFrom == story2TurnID(t, store, story.ID, 3) {
			t.Fatalf("future record leaked: %#v", hit)
		}
	}
}

func TestSearchStoryMemoryAppendValidation(t *testing.T) {
	store, story, _, _, _ := memoryTestStory(t)

	// 缺少 evidence。
	_, err := store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: "turn_missing",
		Records:      []NarrativeMemoryRecord{{ID: "mem_bad", Kind: MemoryKindKnowledge, Subject: "岚", Text: "x", Evidence: "y"}},
	})
	if err == nil {
		t.Fatal("expected error for unknown source turn")
	}

	// 非法 kind。
	_, err = store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: story2TurnID(t, store, story.ID, 3),
		Records:      []NarrativeMemoryRecord{{ID: "mem_bad", Kind: "haha", Subject: "岚", Text: "x", Evidence: "y"}},
	})
	if err == nil {
		t.Fatal("expected error for invalid kind")
	}

	// 空 records 合法(抽取器认为本回合无值得记录的事实)。
	event, err := store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: story2TurnID(t, store, story.ID, 3),
		Records:      nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(event.Records) != 0 {
		t.Fatalf("empty records should be allowed: %#v", event.Records)
	}
}

// story2TurnID 返回 main 分支上第 n 个回合的 ID(1-based)。
func story2TurnID(t *testing.T, store *Store, storyID string, n int) string {
	t.Helper()
	ctx, err := store.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 || n > len(ctx.Snapshot.Turns) {
		t.Fatalf("turn index out of range: %d of %d", n, len(ctx.Snapshot.Turns))
	}
	return ctx.Snapshot.Turns[n-1].ID
}
