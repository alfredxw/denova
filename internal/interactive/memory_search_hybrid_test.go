package interactive

import (
	"context"
	"errors"
	"testing"
)

// fakeMemoryEmbedder 按查询文本返回预置向量,让融合排序可以在没有网络的
// 情况下被逐名次断言。
type fakeMemoryEmbedder struct {
	model   string
	byText  map[string][]float32
	err     error
	callLog []string
}

func (f *fakeMemoryEmbedder) EmbeddingModelID() string { return f.model }

func (f *fakeMemoryEmbedder) EmbedMemoryTexts(_ context.Context, texts []string) ([][]float32, error) {
	f.callLog = append(f.callLog, texts...)
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vector, ok := f.byText[text]
		if !ok {
			vector = []float32{0, 1}
		}
		out = append(out, vector)
	}
	return out, nil
}

func indexOfHit(result MemorySearchResult, recordID string) int {
	for i, hit := range result.Hits {
		if hit.RecordID == recordID {
			return i
		}
	}
	return -1
}

// TestSearchStoryMemoryHybridRecallsWithoutLiteralMatch 覆盖向量召回的核心
// 价值:一条没有任何字面命中的记录,靠语义相似度进入结果。
func TestSearchStoryMemoryHybridRecallsWithoutLiteralMatch(t *testing.T) {
	store, story, _, _, _ := memoryTestStory(t)
	if err := store.AppendMemoryVectors(story.ID, "embed-v1", map[string][]float32{
		"mem_beat": {1, 0},
		"mem_sword": {0, 1},
	}); err != nil {
		t.Fatal(err)
	}
	embedder := &fakeMemoryEmbedder{model: "embed-v1", byText: map[string][]float32{"黑帆船": {1, 0}}}
	req := MemorySearchRequest{Keywords: []string{"黑帆船"}}

	// 纯关键词路径:记忆记录里没有任何一条提到"黑帆船",应当颗粒无收。
	keywordOnly, err := store.SearchStoryMemory(story.ID, "main", req)
	if err != nil {
		t.Fatal(err)
	}
	if len(keywordOnly.Hits) != 0 {
		t.Fatalf("keyword-only search should miss, got %v", hitIDs(keywordOnly.Hits))
	}
	if keywordOnly.VectorEnabled {
		t.Fatal("keyword-only search must not report vector recall")
	}

	hybrid, err := store.SearchStoryMemoryHybrid(context.Background(), embedder, story.ID, "main", req)
	if err != nil {
		t.Fatal(err)
	}
	if !hybrid.VectorEnabled {
		t.Fatal("hybrid search should report vector recall")
	}
	if indexOfHit(hybrid, "mem_beat") < 0 {
		t.Fatalf("vector-only recall missing, got %v", hitIDs(hybrid.Hits))
	}
	// 相似度低于地板值的记录不该被向量支路拉进来。
	if indexOfHit(hybrid, "mem_sword") >= 0 {
		t.Fatalf("below-floor record must not be recalled, got %v", hitIDs(hybrid.Hits))
	}
	if hybrid.Explain.Pipeline.VectorCandidates != 1 {
		t.Fatalf("expected 1 vector candidate, got %d", hybrid.Explain.Pipeline.VectorCandidates)
	}
}

// TestSearchStoryMemoryHybridFusionPrefersBothPaths 断言 RRF 的核心口径:
// 两路召回都认可的记录,胜过任一路的独占冠军。
func TestSearchStoryMemoryHybridFusionPrefersBothPaths(t *testing.T) {
	store, story, _, _, _ := memoryTestStory(t)
	if err := store.AppendMemoryVectors(story.ID, "embed-v1", map[string][]float32{
		"mem_rel_break": {1, 0},
	}); err != nil {
		t.Fatal(err)
	}
	embedder := &fakeMemoryEmbedder{model: "embed-v1", byText: map[string][]float32{"岚": {1, 0}}}
	req := MemorySearchRequest{Keywords: []string{"岚"}}

	keywordOnly, err := store.SearchStoryMemory(story.ID, "main", req)
	if err != nil {
		t.Fatal(err)
	}
	// 关键词路径下 mem_rel_break 只是 object 命中(70 分),排在 subject 精确
	// 命中(110 分)之后。
	if first := keywordOnly.Hits[0].RecordID; first == "mem_rel_break" {
		t.Fatalf("keyword-only search should not already rank mem_rel_break first: %v", hitIDs(keywordOnly.Hits))
	}

	hybrid, err := store.SearchStoryMemoryHybrid(context.Background(), embedder, story.ID, "main", req)
	if err != nil {
		t.Fatal(err)
	}
	if got := hybrid.Hits[0].RecordID; got != "mem_rel_break" {
		t.Fatalf("fused ranking should promote the dual-path hit, got %q (%v)", got, hitIDs(hybrid.Hits))
	}

	var detail MemoryHitDetail
	for _, item := range hybrid.Explain.HitDetails {
		if item.RecordID == "mem_rel_break" {
			detail = item
		}
	}
	if detail.KeywordRank == 0 || detail.VectorRank != 1 {
		t.Fatalf("expected both ranks recorded, got keyword=%d vector=%d", detail.KeywordRank, detail.VectorRank)
	}
	if detail.FusedScore <= 0 || detail.VectorScore <= 0 {
		t.Fatalf("expected positive fused/vector scores, got %v/%v", detail.FusedScore, detail.VectorScore)
	}
	if hybrid.Explain.Pipeline.FusedRanked == 0 {
		t.Fatal("fused_ranked counter should be populated")
	}
}

// TestSearchStoryMemoryHybridDegradesGracefully 覆盖所有降级路径:向量始终是
// 可选增强,任何一环失效都必须退回关键词结果而不是让检索失败。
func TestSearchStoryMemoryHybridDegradesGracefully(t *testing.T) {
	store, story, _, _, _ := memoryTestStory(t)
	if err := store.AppendMemoryVectors(story.ID, "embed-v1", map[string][]float32{
		"mem_rel_break": {1, 0},
	}); err != nil {
		t.Fatal(err)
	}
	req := MemorySearchRequest{Keywords: []string{"岚"}}
	baseline, err := store.SearchStoryMemory(story.ID, "main", req)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		embedder MemoryEmbedder
		req      MemorySearchRequest
	}{
		{name: "no embedder", embedder: nil, req: req},
		{
			name:     "embedding call fails",
			embedder: &fakeMemoryEmbedder{model: "embed-v1", err: errors.New("boom")},
			req:      req,
		},
		{
			// 换 embedding 模型后旧向量整批失效:语义空间不同,混用会得到
			// 无意义的距离。
			name:     "model changed",
			embedder: &fakeMemoryEmbedder{model: "embed-v2", byText: map[string][]float32{"岚": {1, 0}}},
			req:      req,
		},
		{
			// 浏览模式没有关键词,向量支路无查询可嵌。
			name:     "browse mode without keywords",
			embedder: &fakeMemoryEmbedder{model: "embed-v1"},
			req:      MemorySearchRequest{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := store.SearchStoryMemoryHybrid(context.Background(), tc.embedder, story.ID, "main", tc.req)
			if err != nil {
				t.Fatalf("hybrid search must not fail: %v", err)
			}
			if result.VectorEnabled {
				t.Fatal("vector recall should be reported as disabled")
			}
			if result.Explain.Pipeline.VectorCandidates != 0 {
				t.Fatalf("expected no vector candidates, got %d", result.Explain.Pipeline.VectorCandidates)
			}
			if tc.req.Keywords == nil {
				return
			}
			if got, want := hitIDs(result.Hits), hitIDs(baseline.Hits); len(got) != len(want) {
				t.Fatalf("degraded result should match keyword baseline: got %v want %v", got, want)
			} else {
				for i := range got {
					if got[i] != want[i] {
						t.Fatalf("degraded result order differs: got %v want %v", got, want)
					}
				}
			}
		})
	}
}

// TestSearchStoryMemoryHybridEmbedsQueryOnce 确认查询向量只取一次,且组装
// 措辞与记录侧一致(用同一分隔符),否则向量距离没有可比性。
func TestSearchStoryMemoryHybridEmbedsQueryOnce(t *testing.T) {
	store, story, _, _, _ := memoryTestStory(t)
	embedder := &fakeMemoryEmbedder{model: "embed-v1", byText: map[string][]float32{}}
	if _, err := store.SearchStoryMemoryHybrid(context.Background(), embedder, story.ID, "main", MemorySearchRequest{
		Keywords: []string{"岚", "蚀骨剑"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(embedder.callLog) != 1 {
		t.Fatalf("expected exactly one embedding call, got %d", len(embedder.callLog))
	}
	if want := "岚 | 蚀骨剑"; embedder.callLog[0] != want {
		t.Fatalf("query text %q should match record-side assembly %q", embedder.callLog[0], want)
	}
}
