package interactive

import (
	"testing"
)

// memoryChainStory 建一条实体关系链 甲—乙—丙—丁,外加两条非关系类记录。
// 链上每一环都要多走一跳才能抵达,用于把跳数、预算与路径逐跳钉住。
func memoryChainStory(t *testing.T) (*Store, StorySummary) {
	t.Helper()
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "关系链", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "开场",
		Narrative: "甲、乙、丙、丁先后登场。",
	})
	if err != nil {
		t.Fatal(err)
	}
	records := []NarrativeMemoryRecord{
		{ID: "chain_ab", Kind: MemoryKindRelationship, Subject: "甲", Object: "乙", Text: "甲与乙结盟。", Evidence: "结盟", ValidFrom: turn.ID},
		{ID: "chain_bc", Kind: MemoryKindRelationship, Subject: "乙", Object: "丙", Text: "乙与丙是旧识。", Evidence: "旧识", ValidFrom: turn.ID},
		{ID: "chain_cd", Kind: MemoryKindRelationship, Subject: "丙", Object: "丁", Text: "丙受丁差遣。", Evidence: "差遣", ValidFrom: turn.ID},
		// beat 不是关系类:能被拉入,但不能把前沿推进到新实体。
		// chain_beat_pair 的另一端 戊 只有在 beat 也导边时才会进入前沿。
		{ID: "chain_beat_b", Kind: MemoryKindBeat, Subject: "乙", Text: "乙承担引路人的戏剧功能。", Evidence: "引路", ValidFrom: turn.ID},
		{ID: "chain_beat_pair", Kind: MemoryKindBeat, Subject: "乙", Object: "戊", Text: "乙与戊共担一场戏。", Evidence: "同场", ValidFrom: turn.ID},
		{ID: "chain_ef", Kind: MemoryKindRelationship, Subject: "戊", Object: "己", Text: "戊与己是同乡。", Evidence: "同乡", ValidFrom: turn.ID},
		{ID: "chain_beat_d", Kind: MemoryKindBeat, Subject: "丁", Text: "丁承担幕后主使的戏剧功能。", Evidence: "主使", ValidFrom: turn.ID},
	}
	if _, err := store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: turn.ID,
		Records:      records,
	}); err != nil {
		t.Fatal(err)
	}
	return store, story
}

func expandedByID(result MemorySearchResult) map[string]MemorySearchHit {
	out := map[string]MemorySearchHit{}
	for _, hit := range result.Hits {
		if hit.ExpandedFrom != "" {
			out[hit.RecordID] = hit
		}
	}
	return out
}

// TestExpandMemoryGraphHopsReachFurther 逐跳钉住可达范围:每多一跳,链上正好
// 多带出一环。
func TestExpandMemoryGraphHopsReachFurther(t *testing.T) {
	store, story := memoryChainStory(t)

	tests := []struct {
		hops     int
		expanded []string
	}{
		// 一跳:锚点 {甲,乙} —— 三条触及乙的记录被带出。
		{hops: 1, expanded: []string{"chain_bc", "chain_beat_b", "chain_beat_pair"}},
		// 两跳:只有关系类的 chain_bc 把前沿推到丙,带出 chain_cd。
		// chain_beat_pair 虽在一跳内被带出,但 beat 不导边,戊 不进前沿。
		{hops: 2, expanded: []string{"chain_bc", "chain_beat_b", "chain_beat_pair", "chain_cd"}},
		// 三跳:chain_cd 推进到丁,带出 chain_beat_d。戊/己 始终不可达。
		{hops: 3, expanded: []string{"chain_bc", "chain_beat_b", "chain_beat_pair", "chain_cd", "chain_beat_d"}},
	}
	for _, tc := range tests {
		result, err := store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{
			Keywords:   []string{"甲"},
			ExpandHops: tc.hops,
		})
		if err != nil {
			t.Fatal(err)
		}
		expanded := expandedByID(result)
		if len(expanded) != len(tc.expanded) {
			t.Fatalf("hops=%d expanded %d records, want %d (%s)", tc.hops, len(expanded), len(tc.expanded), hitIDs(result.Hits))
		}
		for _, recordID := range tc.expanded {
			if _, ok := expanded[recordID]; !ok {
				t.Fatalf("hops=%d missing %s (%s)", tc.hops, recordID, hitIDs(result.Hits))
			}
		}
		if result.Explain.Pipeline.ExpandedHops != tc.hops {
			t.Fatalf("hops=%d pipeline reports %d", tc.hops, result.Explain.Pipeline.ExpandedHops)
		}
	}
}

// TestExpandMemoryGraphNonRelationalKindsDoNotBridge 是走边规则的核心:
// beat 类记录能被带出,但不能把前沿推进到它另一端的实体,否则任意"同场戏"
// 记录都会把无关角色连成一片。
func TestExpandMemoryGraphNonRelationalKindsDoNotBridge(t *testing.T) {
	store, story := memoryChainStory(t)
	// 从甲出发跑满跳数:chain_beat_pair(beat, 乙—戊)会在一跳内被带出,
	// 但它不导边,所以 戊 永不进入前沿,戊 那一侧的 chain_ef 始终不可达。
	result, err := store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{
		Keywords:   []string{"甲"},
		ExpandHops: MaxMemoryExpandHops,
		Limit:      MaxMemorySearchLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	expanded := expandedByID(result)
	if _, ok := expanded["chain_beat_pair"]; !ok {
		t.Fatalf("beat record should still be pulled in: %s", hitIDs(result.Hits))
	}
	if _, ok := expanded["chain_ef"]; ok {
		t.Fatalf("beat must not bridge to 戊's side of the graph: %s", hitIDs(result.Hits))
	}
	// 对照:同样距离下,关系类记录确实把前沿推进到了链的末端。
	if _, ok := expanded["chain_beat_d"]; !ok {
		t.Fatalf("relational chain should reach 丁: %s", hitIDs(result.Hits))
	}
}

// TestExpandMemoryGraphRecordsPathAndDecay 断言可解释性与衰减:远处的关联要能
// 看出来路,且保底分随跳数下降,好在预算截断时先被舍弃。
func TestExpandMemoryGraphRecordsPathAndDecay(t *testing.T) {
	store, story := memoryChainStory(t)
	result, err := store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{
		Keywords:   []string{"甲"},
		ExpandHops: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	expanded := expandedByID(result)

	near := expanded["chain_bc"]
	if near.ExpandedHop != 1 || near.ExpandedFrom != "乙" {
		t.Fatalf("first hop metadata wrong: %#v", near)
	}
	if len(near.ExpandedPath) != 1 || near.ExpandedPath[0] != "乙" {
		t.Fatalf("first hop path wrong: %#v", near.ExpandedPath)
	}

	far := expanded["chain_cd"]
	if far.ExpandedHop != 2 || far.ExpandedFrom != "丙" {
		t.Fatalf("second hop metadata wrong: %#v", far)
	}
	if len(far.ExpandedPath) != 2 || far.ExpandedPath[0] != "乙" || far.ExpandedPath[1] != "丙" {
		t.Fatalf("second hop path should retrace 乙→丙: %#v", far.ExpandedPath)
	}
	if far.Score >= near.Score {
		t.Fatalf("second hop score %d should decay below first hop %d", far.Score, near.Score)
	}
}

// TestExpandMemoryGraphPerHopBudget 确认每跳预算生效:高连通实体不能把整个
// 记忆库拉进结果。
func TestExpandMemoryGraphPerHopBudget(t *testing.T) {
	store, story := memoryChainStory(t)
	result, err := store.SearchStoryMemory(story.ID, "main", MemorySearchRequest{
		Keywords:   []string{"甲"},
		ExpandHops: 3,
		Limit:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// limit=1 同时是每跳预算与最终预算,结果里只该留下最强的那条直接命中。
	if len(result.Hits) != 1 {
		t.Fatalf("expected a single hit under limit=1, got %s", hitIDs(result.Hits))
	}
	if result.Hits[0].RecordID != "chain_ab" {
		t.Fatalf("direct hit should survive budget, got %q", result.Hits[0].RecordID)
	}
	if !result.Truncated {
		t.Fatal("result should report truncation")
	}
}

// TestExpandMemoryGraphHopsAreBounded 确认跳数被夹在合法区间,非法值不会让
// 展开无限扩散。
func TestExpandMemoryGraphHopsAreBounded(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{in: 0, want: DefaultMemoryExpandHops},
		{in: -3, want: DefaultMemoryExpandHops},
		{in: 2, want: 2},
		{in: 99, want: MaxMemoryExpandHops},
	}
	for _, tc := range tests {
		if got := normalizeMemoryExpandHops(tc.in); got != tc.want {
			t.Fatalf("normalizeMemoryExpandHops(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
