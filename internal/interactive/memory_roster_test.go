package interactive

import (
	"testing"
)

func TestBuildMemoryEntityRosterPicksDominantSpelling(t *testing.T) {
	records := []NarrativeMemoryRecord{
		{Kind: MemoryKindRelationship, Subject: "林舟", Object: "岚"},
		{Kind: MemoryKindBeat, Subject: "林舟"},
		// 写法漂移:归一化后与"林舟"同键,但出现次数少,不该当选权威写法。
		{Kind: MemoryKindKnowledge, Subject: "林 舟", Object: "剑的来历"},
		{Kind: MemoryKindObjectState, Subject: "蚀骨剑", Object: "林舟"},
	}
	roster := buildMemoryEntityRoster(records, DefaultMemoryRosterLimit)

	byName := map[string]MemoryEntity{}
	for _, entity := range roster {
		byName[entity.Name] = entity
	}
	linzhou, ok := byName["林舟"]
	if !ok {
		t.Fatalf("dominant spelling missing from roster: %#v", roster)
	}
	// 四次提及全部归到同一实体,含那条写法漂移的。
	if linzhou.Mentions != 4 {
		t.Fatalf("mentions should merge across spellings, got %d", linzhou.Mentions)
	}
	if _, drifted := byName["林 舟"]; drifted {
		t.Fatalf("drifted spelling must not get its own entry: %#v", roster)
	}
	// 名册按活跃度排序,常驻实体排在一次性提及的前面。
	if roster[0].Name != "林舟" {
		t.Fatalf("most-mentioned entity should lead the roster, got %q", roster[0].Name)
	}
	if len(linzhou.Kinds) == 0 {
		t.Fatal("entity should carry the kinds it appeared in")
	}
}

func TestBuildMemoryEntityRosterIsBounded(t *testing.T) {
	records := make([]NarrativeMemoryRecord, 0, 20)
	for i := range 20 {
		records = append(records, NarrativeMemoryRecord{
			Kind:    MemoryKindBeat,
			Subject: string(rune('A' + i)),
		})
	}
	if roster := buildMemoryEntityRoster(records, 5); len(roster) != 5 {
		t.Fatalf("roster should honour the limit, got %d", len(roster))
	}
	// limit<=0 回落默认值而不是变成无界。
	if roster := buildMemoryEntityRoster(records, 0); len(roster) != 20 {
		t.Fatalf("expected all 20 entities under the default limit, got %d", len(roster))
	}
}

func TestBuildMemoryEntityRosterSkipsEmptyEntities(t *testing.T) {
	roster := buildMemoryEntityRoster([]NarrativeMemoryRecord{
		{Kind: MemoryKindBeat, Subject: "岚", Object: ""},
		{Kind: MemoryKindBeat, Subject: "   ", Object: "  "},
		// 全是标点的实体归一化后为空,不能进名册。
		{Kind: MemoryKindBeat, Subject: "——", Object: "……"},
	}, DefaultMemoryRosterLimit)
	if len(roster) != 1 || roster[0].Name != "岚" {
		t.Fatalf("only the real entity should survive: %#v", roster)
	}
}

func TestStoryEntityRosterRespectsBeforeTurnID(t *testing.T) {
	store, story, first, _, _ := memoryTestStory(t)

	full, err := store.StoryEntityRoster(story.ID, "main", "", DefaultMemoryRosterLimit)
	if err != nil {
		t.Fatal(err)
	}
	early, err := store.StoryEntityRoster(story.ID, "main", first.ID, DefaultMemoryRosterLimit)
	if err != nil {
		t.Fatal(err)
	}
	// 回到第一回合之前,后续回合才出现的实体不该被看见 —— 重放某个较早时点的
	// 抽取不能获得未来知识。
	if len(early) >= len(full) {
		t.Fatalf("early roster (%d) should be smaller than full roster (%d)", len(early), len(full))
	}
}

func TestMemoryEntityRosterIndex(t *testing.T) {
	index := MemoryEntityRosterIndex([]MemoryEntity{{Name: "蚀骨剑"}, {Name: "林舟"}})
	if got := index[NormalizeEntityName("蚀骨剑")]; got != "蚀骨剑" {
		t.Fatalf("index lookup failed, got %q", got)
	}
	// 索引键是归一化形式,漂移写法也能查到权威写法。
	if got := index[NormalizeEntityName("林 舟")]; got != "林舟" {
		t.Fatalf("drifted spelling should resolve to canonical, got %q", got)
	}
	if index := MemoryEntityRosterIndex(nil); index != nil {
		t.Fatal("empty roster should yield a nil index")
	}
}

func TestNormalizeEntityNameCollapsesSpacingDrift(t *testing.T) {
	// 实体键比检索键更激进:检索键把内部空格当分隔符保留,实体键要把这类
	// 排版漂移收敛掉,否则"林 舟"永远对齐不到"林舟"。
	if NormalizeEntityName("林 舟") != NormalizeEntityName("林舟") {
		t.Fatal("spacing drift should collapse to one entity key")
	}
	if normalizeMemorySearchText("林 舟") == normalizeMemorySearchText("林舟") {
		t.Fatal("retrieval key is expected to keep them apart; entity key exists precisely to bridge that")
	}
	// 大小写、全半角与标点仍沿用检索侧的口径。
	if NormalizeEntityName("ＬＩＮ") != NormalizeEntityName("lin") {
		t.Fatal("fullwidth and case folding should follow the retrieval key")
	}
	if NormalizeEntityName("蚀骨剑!") != NormalizeEntityName("蚀骨剑") {
		t.Fatal("punctuation should be stripped")
	}
	// 不同实体不能被误合并。
	if NormalizeEntityName("林舟") == NormalizeEntityName("岚") {
		t.Fatal("distinct entities must not collide")
	}
}
