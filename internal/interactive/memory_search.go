package interactive

import (
	"context"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	DefaultMemorySearchLimit = 8
	MaxMemorySearchLimit     = 12
	memoryEvidenceRunes      = 120
)

// MemorySearchRequest 是对叙事记忆投影的一次有界查询。
type MemorySearchRequest struct {
	Keywords     []string `json:"keywords,omitempty"`
	Kind         string   `json:"kind,omitempty"`
	Subject      string   `json:"subject,omitempty"`
	BeforeTurnID string   `json:"before_turn_id,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

// MemorySearchHit 是一条命中记录,带得分与溯源。字段与 StoryHistoryHit
// 同构,便于前端复用展示形态。
type MemorySearchHit struct {
	RecordID    string `json:"record_id"`
	Kind        string `json:"kind"`
	Subject     string `json:"subject"`
	Object      string `json:"object,omitempty"`
	Text        string `json:"text"`
	Evidence    string `json:"evidence"`
	ValidFrom   string `json:"valid_from"`
	ValidTo     string `json:"valid_to,omitempty"`
	Status      string `json:"status,omitempty"`
	Score       int    `json:"score,omitempty"`
	// ExpandedFrom 非空 = 一跳展开拉入,值为锚点记录 ID;空 = 直接命中。
	ExpandedFrom string `json:"expanded_from,omitempty"`
}

// MemoryHitDetail 解释一条命中的得分构成。
type MemoryHitDetail struct {
	RecordID string `json:"record_id"`
	// KeywordParts 列出每个命中来源(字段与关键词)及其分值贡献。
	KeywordParts []MemoryKeywordPart `json:"keyword_parts,omitempty"`
	// PromiseBoost 是伏笔类命中加成。
	PromiseBoost int `json:"promise_boost,omitempty"`
	// ExpandedFrom 同 MemorySearchHit.ExpandedFrom。
	ExpandedFrom string `json:"expanded_from,omitempty"`
	// VectorScore 是与查询向量的余弦相似度;向量未启用时为 0。
	VectorScore float64 `json:"vector_score,omitempty"`
	// KeywordRank / VectorRank 是该记录在两路召回里的名次(1 起;0 = 未进该路)。
	KeywordRank int `json:"keyword_rank,omitempty"`
	VectorRank  int `json:"vector_rank,omitempty"`
	// FusedScore 是 RRF 融合分,向量启用时决定最终排序。
	FusedScore float64 `json:"fused_score,omitempty"`
}

type MemoryKeywordPart struct {
	Field    string `json:"field"`    // subject / object / text
	Keyword  string `json:"keyword"`
	Score    int    `json:"score"`
}

// MemoryFilteredItem 解释一条候选记录为何未进入最终结果。
type MemoryFilteredItem struct {
	RecordID string `json:"record_id"`
	Subject  string `json:"subject"`
	Text     string `json:"text"`
	Reason   string `json:"reason"` // expired / kind_mismatch / subject_mismatch / no_keyword_match / low_score / budget_cut
	// Score 是被过滤时的得分(0 = 未进入打分)。
	Score int `json:"score,omitempty"`
}

// MemorySearchPipelineCounters 是检索流水线的计数瀑布,与观测面板逐行对应。
// 向量与 RRF 行在未启用 embedding 时恒为 0。
type MemorySearchPipelineCounters struct {
	ProjectedEvents  int `json:"projected_events"`
	DedupedRecords   int `json:"deduped_records"`
	StaleRecords     int `json:"stale_records"`
	ValidRecords     int `json:"valid_records"`
	ExpiredRecords   int `json:"expired_records"`
	Candidates       int `json:"candidates"`        // 通过 kind/subject 前置过滤的记录数
	KeywordMatched   int `json:"keyword_matched"`   // 关键词打分 > 0 的记录数
	VectorCandidates int `json:"vector_candidates"` // 进入向量召回名次的记录数
	FusedRanked      int `json:"fused_ranked"`      // 融合后参与排序的记录数
	Anchors          int `json:"anchors"`           // 一跳展开锚点数
	ExpandedRecords  int `json:"expanded_records"`  // 一跳展开拉入的记录数
	FinalAfterBudget int `json:"final_after_budget"`
}

// MemorySearchExplain 是检索调试 API 的解释载荷。
type MemorySearchExplain struct {
	Pipeline  MemorySearchPipelineCounters `json:"pipeline"`
	HitDetails []MemoryHitDetail           `json:"hit_details,omitempty"`
	Filtered  []MemoryFilteredItem        `json:"filtered,omitempty"`
}

// MemorySearchResult 同一检索代码路径的两种序列化:
// 工具输出只含 Hits 与 Truncated;调试 API 额外含 Explain。
type MemorySearchResult struct {
	StoryID      string             `json:"story_id"`
	BranchID     string             `json:"branch_id"`
	Keywords     []string           `json:"keywords,omitempty"`
	Kind         string             `json:"kind,omitempty"`
	Subject      string             `json:"subject,omitempty"`
	Match        string             `json:"match"`
	BeforeTurnID string             `json:"before_turn_id,omitempty"`
	Limit        int                  `json:"limit"`
	Truncated    bool                 `json:"truncated"`
	// VectorEnabled 表明本次检索是否用上了向量召回。false 时结果完全来自
	// 关键词路径(未配置 embedding 模型、无缓存向量,或向量调用失败降级)。
	VectorEnabled bool                 `json:"vector_enabled"`
	Hits          []MemorySearchHit    `json:"hits"`
	Explain       *MemorySearchExplain `json:"explain,omitempty"`
}

// SearchStoryMemory 在当前分支的叙事记忆投影上执行查询条件化检索。
// 纯关键词路径,不含向量召回;需要混合检索时用 SearchStoryMemoryHybrid。
func (s *Store) SearchStoryMemory(storyID, branchID string, req MemorySearchRequest) (MemorySearchResult, error) {
	return s.searchStoryMemory(storyID, branchID, req, nil, "")
}

// SearchStoryMemoryHybrid 在关键词召回之外并联一路向量召回,用 RRF 融合两份
// 名次。embedder 为 nil、无关键词、query 向量取用失败或该故事尚无缓存向量时,
// 整条向量支路静默跳过 —— 向量始终是可选增强,不能让检索因它不可用而失败。
//
// query 向量在进入存储锁之前取,持锁期间不做任何网络调用。
func (s *Store) SearchStoryMemoryHybrid(ctx context.Context, embedder MemoryEmbedder, storyID, branchID string, req MemorySearchRequest) (MemorySearchResult, error) {
	var queryVector []float32
	model := ""
	if embedder != nil {
		if keywords := normalizeMemorySearchKeywords(req.Keywords); len(keywords) > 0 {
			model = embedder.EmbeddingModelID()
			// 查询侧与记录侧必须用同一种措辞组装,否则向量距离没有意义。
			query := strings.Join(keywords, " | ")
			vectors, err := embedder.EmbedMemoryTexts(ctx, []string{query})
			if err != nil || len(vectors) == 0 {
				// 降级而非报错:关键词路径足以独立给出结果。
				model = ""
			} else {
				queryVector = vectors[0]
			}
		}
	}
	return s.searchStoryMemory(storyID, branchID, req, queryVector, model)
}

// searchStoryMemory 是两个入口共用的实现。空关键词时返回最近视角的有效记录
// (兼任记忆浏览器);命中后按锚点实体一跳展开同实体记录(默认开启),把
// "现状 + 认知边界 + 悬置伏笔"整体带出。
func (s *Store) searchStoryMemory(storyID, branchID string, req MemorySearchRequest, queryVector []float32, embeddingModel string) (MemorySearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return MemorySearchResult{}, err
	}
	branchID, branch, err := resolveBranch(meta, strings.TrimSpace(branchID))
	if err != nil {
		return MemorySearchResult{}, err
	}
	path, _ := eventPath(branch.Head, eventsByID(lines))

	memoryEvents, turnOrder := collectNarrativeMemory(path)
	keywords := normalizeMemorySearchKeywords(req.Keywords)
	kind := normalizeMemoryKind(req.Kind)
	subject := normalizeMemorySearchText(req.Subject)
	beforeTurnID := strings.TrimSpace(req.BeforeTurnID)
	limit := normalizeMemorySearchLimit(req.Limit)
	match := "any"
	if len(keywords) == 0 {
		match = "all"
	}

	projection := ProjectNarrativeMemory(memoryEvents, turnOrder, beforeTurnID)
	explain := MemorySearchExplain{Pipeline: MemorySearchPipelineCounters{
		ProjectedEvents: projection.Events,
		DedupedRecords:  projection.DedupedRecords,
		StaleRecords:    projection.StaleRecords,
		ValidRecords:    projection.ValidRecords,
		ExpiredRecords:  projection.ExpiredRecords,
	}}

	candidates := make([]NarrativeMemoryRecord, 0, len(projection.Records))
	for _, record := range projection.Records {
		reason := memoryPrefilterReason(record, kind, subject)
		if reason != "" {
			explain.Filtered = append(explain.Filtered, MemoryFilteredItem{
				RecordID: record.ID,
				Subject:  record.Subject,
				Text:     boundedMemoryText(record.Text, 80),
				Reason:   reason,
			})
			continue
		}
		candidates = append(candidates, record)
	}
	explain.Pipeline.Candidates = len(candidates)

	direct, filteredByScore, keywordMatched := scoreMemoryRecords(candidates, keywords)
	explain.Pipeline.KeywordMatched = keywordMatched

	// 向量召回与关键词召回并联,两路各出一份名次,稍后用 RRF 融合。
	vectorScores := s.vectorRecallLocked(storyID, candidates, queryVector, embeddingModel, limit)
	vectorEnabled := len(vectorScores) > 0
	explain.Pipeline.VectorCandidates = len(vectorScores)

	// 合并直接命中与一跳展开。
	byID := map[string]bool{}
	merged := make([]scoredMemoryHit, 0, len(direct))
	detailIndex := map[string]int{}
	for _, hit := range direct {
		merged = append(merged, hit)
		byID[hit.hit.RecordID] = true
		detail := MemoryHitDetail{RecordID: hit.hit.RecordID}
		detail.KeywordParts = hit.parts
		detail.PromiseBoost = hit.promiseBoost
		explain.HitDetails = append(explain.HitDetails, detail)
		detailIndex[hit.hit.RecordID] = len(explain.HitDetails) - 1
	}

	// 向量独有命中:语义相关但没有任何字面命中的记录,由向量支路补进候选。
	// 它们同样可以当一跳展开的锚点,所以必须在展开之前并入。
	if vectorEnabled {
		for _, record := range candidates {
			if byID[record.ID] {
				continue
			}
			if _, ok := vectorScores[record.ID]; !ok {
				continue
			}
			merged = append(merged, scoredMemoryHit{hit: memorySearchHitFrom(record, memoryVectorOnlyScore)})
			byID[record.ID] = true
			explain.HitDetails = append(explain.HitDetails, MemoryHitDetail{RecordID: record.ID})
			detailIndex[record.ID] = len(explain.HitDetails) - 1
		}
	}

	expanded := 0
	if len(keywords) > 0 {
		anchors := memoryExpansionAnchors(merged)
		explain.Pipeline.Anchors = len(anchors)
		for _, record := range candidates {
			if byID[record.ID] {
				continue
			}
			anchor := memoryExpansionAnchorFor(record, anchors)
			if anchor == "" {
				continue
			}
			expanded++
			hit := scoredMemoryHit{
				hit: MemorySearchHit{
					RecordID:    record.ID,
					Kind:        record.Kind,
					Subject:     record.Subject,
					Object:      record.Object,
					Text:        record.Text,
					Evidence:    boundedMemoryText(record.Evidence, memoryEvidenceRunes),
					ValidFrom:   record.ValidFrom,
					ValidTo:     record.ValidTo,
					Status:      record.Status,
					Score:       memoryExpansionScore,
					ExpandedFrom: anchor,
				},
			}
			merged = append(merged, hit)
			byID[record.ID] = true
			explain.HitDetails = append(explain.HitDetails, MemoryHitDetail{
				RecordID:     record.ID,
				ExpandedFrom: anchor,
			})
		}
	}
	explain.Pipeline.ExpandedRecords = expanded
	explain.Filtered = append(explain.Filtered, filteredByScore...)

	fusedRanked := fuseMemoryRanks(merged, vectorScores, explain.HitDetails, detailIndex)
	explain.Pipeline.FusedRanked = fusedRanked

	sort.SliceStable(merged, func(i, j int) bool {
		// 向量启用时由 RRF 融合分主导;两路都缺席的记录(一跳展开项)融合分为 0,
		// 自然落到直接命中之后,预算截断时优先保住命中。
		if vectorEnabled && merged[i].fused != merged[j].fused {
			return merged[i].fused > merged[j].fused
		}
		if merged[i].hit.Score != merged[j].hit.Score {
			return merged[i].hit.Score > merged[j].hit.Score
		}
		if merged[i].hit.ValidFrom != merged[j].hit.ValidFrom {
			return merged[i].hit.ValidFrom > merged[j].hit.ValidFrom
		}
		return merged[i].hit.RecordID < merged[j].hit.RecordID
	})

	truncated := len(merged) > limit
	if truncated {
		for _, item := range merged[limit:] {
			explain.Filtered = append(explain.Filtered, MemoryFilteredItem{
				RecordID: item.hit.RecordID,
				Subject:  item.hit.Subject,
				Text:     boundedMemoryText(item.hit.Text, 80),
				Reason:   "budget_cut",
				Score:    item.hit.Score,
			})
		}
		merged = merged[:limit]
	}
	explain.Pipeline.FinalAfterBudget = len(merged)

	hits := make([]MemorySearchHit, 0, len(merged))
	for _, item := range merged {
		hits = append(hits, item.hit)
	}
	sort.SliceStable(explain.Filtered, func(i, j int) bool {
		return explain.Filtered[i].RecordID < explain.Filtered[j].RecordID
	})
	return MemorySearchResult{
		StoryID:      storyID,
		BranchID:     branchID,
		Keywords:     keywords,
		Kind:         kind,
		Subject:      subject,
		Match:        match,
		BeforeTurnID: beforeTurnID,
		Limit:         limit,
		Truncated:     truncated,
		VectorEnabled: vectorEnabled,
		Hits:          hits,
		Explain:       &explain,
	}, nil
}

const (
	memoryExpansionScore = 40 // 一跳展开拉入的固定保底分:低于多数直接命中,高于无关键词浏览

	// memoryVectorOnlyScore 给"语义相关但无任何字面命中"的向量独有召回。
	// 介于一跳展开(40)与最弱的字面命中 text contains(50)之间。
	memoryVectorOnlyScore = 45

	memoryScoreSubjectExact    = 110
	memoryScoreSubjectContains = 60
	memoryScoreObjectContains  = 70
	memoryScoreTextContains    = 50
	memoryScorePromiseBoost    = 15

	// memoryRRFK 是 RRF 的平滑常数。60 是该算法的通行取值:它把头部名次的
	// 差距压平,让两路召回都认可的记录胜过任一路的独占冠军。
	memoryRRFK = 60.0

	// memoryVectorFloor 是进入向量名次所需的最低余弦相似度。没有下限时
	// RRF 的平缓衰减会让完全无关的记录也拿到分;这是经验阈值,可调。
	memoryVectorFloor = 0.25
)

type scoredMemoryHit struct {
	hit          MemorySearchHit
	parts        []MemoryKeywordPart
	promiseBoost int
	// fused 是 RRF 融合分,向量启用时决定排序;两路都未召回则为 0。
	fused float64
}

// memorySearchHitFrom 把一条记录转成命中,证据按统一上限裁剪。
func memorySearchHitFrom(record NarrativeMemoryRecord, score int) MemorySearchHit {
	return MemorySearchHit{
		RecordID:  record.ID,
		Kind:      record.Kind,
		Subject:   record.Subject,
		Object:    record.Object,
		Text:      record.Text,
		Evidence:  boundedMemoryText(record.Evidence, memoryEvidenceRunes),
		ValidFrom: record.ValidFrom,
		ValidTo:   record.ValidTo,
		Status:    record.Status,
		Score:     score,
	}
}

// vectorRecallLocked 对候选记录做向量召回,返回 record ID → 余弦相似度。
// 只使用侧车里已缓存的向量:检索路径不做补算,缺向量的记录靠关键词路径覆盖。
// 返回空 map 表示向量支路未生效(未配置、无缓存或全部低于地板值)。
func (s *Store) vectorRecallLocked(storyID string, candidates []NarrativeMemoryRecord, queryVector []float32, model string, limit int) map[string]float64 {
	if len(queryVector) == 0 || strings.TrimSpace(model) == "" || len(candidates) == 0 {
		return nil
	}
	cached := s.readMemoryVectorsLocked(storyID, model)
	if len(cached) == 0 {
		return nil
	}
	type scored struct {
		recordID string
		score    float64
	}
	ranked := make([]scored, 0, len(candidates))
	for _, record := range candidates {
		vector, ok := cached[record.ID]
		if !ok {
			continue
		}
		similarity := cosineSimilarity(queryVector, vector)
		if similarity < memoryVectorFloor {
			continue
		}
		ranked = append(ranked, scored{recordID: record.ID, score: similarity})
	}
	if len(ranked) == 0 {
		return nil
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].recordID < ranked[j].recordID
	})
	// 只保留前 limit 名。RRF 的名次衰减很平缓,尾部候选若不截断会稀释融合分。
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make(map[string]float64, len(ranked))
	for _, item := range ranked {
		out[item.recordID] = item.score
	}
	return out
}

// fuseMemoryRanks 用 RRF(Reciprocal Rank Fusion)融合关键词名次与向量名次,
// 就地写入每条命中的 fused 分,并把名次明细回填到 Explain。
// 返回至少进入一路召回的记录数。
//
// RRF 只看名次不看原始分值,因此不需要在"关键词分 110"与"余弦 0.83"之间
// 编造可比的量纲——这正是选它而非加权求和的原因。
func fuseMemoryRanks(merged []scoredMemoryHit, vectorScores map[string]float64, details []MemoryHitDetail, detailIndex map[string]int) int {
	if len(merged) == 0 {
		return 0
	}
	keywordRank := memoryKeywordRanks(merged)
	vectorRank := memoryVectorRanks(vectorScores)

	fused := 0
	for i := range merged {
		recordID := merged[i].hit.RecordID
		kwRank, hasKeyword := keywordRank[recordID]
		vecRank, hasVector := vectorRank[recordID]
		if !hasKeyword && !hasVector {
			continue
		}
		fused++
		score := 0.0
		if hasKeyword {
			score += 1 / (memoryRRFK + float64(kwRank))
		}
		if hasVector {
			score += 1 / (memoryRRFK + float64(vecRank))
		}
		merged[i].fused = score

		index, ok := detailIndex[recordID]
		if !ok || index >= len(details) {
			continue
		}
		details[index].KeywordRank = kwRank
		details[index].VectorRank = vecRank
		details[index].VectorScore = vectorScores[recordID]
		details[index].FusedScore = score
	}
	return fused
}

// memoryKeywordRanks 按关键词得分给直接命中排名次(1 起)。
// 一跳展开与向量独有召回不属于关键词召回,不参与该路名次。
func memoryKeywordRanks(merged []scoredMemoryHit) map[string]int {
	ordered := make([]scoredMemoryHit, 0, len(merged))
	for _, hit := range merged {
		if hit.hit.ExpandedFrom != "" || len(hit.parts) == 0 {
			continue
		}
		ordered = append(ordered, hit)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].hit.Score != ordered[j].hit.Score {
			return ordered[i].hit.Score > ordered[j].hit.Score
		}
		return ordered[i].hit.RecordID < ordered[j].hit.RecordID
	})
	ranks := make(map[string]int, len(ordered))
	for i, hit := range ordered {
		ranks[hit.hit.RecordID] = i + 1
	}
	return ranks
}

// memoryVectorRanks 按余弦相似度给向量召回排名次(1 起)。
func memoryVectorRanks(scores map[string]float64) map[string]int {
	if len(scores) == 0 {
		return nil
	}
	ordered := make([]string, 0, len(scores))
	for recordID := range scores {
		ordered = append(ordered, recordID)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if scores[ordered[i]] != scores[ordered[j]] {
			return scores[ordered[i]] > scores[ordered[j]]
		}
		return ordered[i] < ordered[j]
	})
	ranks := make(map[string]int, len(ordered))
	for i, recordID := range ordered {
		ranks[recordID] = i + 1
	}
	return ranks
}

// collectNarrativeMemory 从路径上的事件记录中抽取 memory 事件与回合顺序。
func collectNarrativeMemory(path []StoryEventRecord) ([]NarrativeMemoryEvent, []string) {
	memoryEvents := make([]NarrativeMemoryEvent, 0)
	turnOrder := make([]string, 0)
	for _, record := range path {
		switch record.Envelope.Type {
		case StoryEventTypeTurn:
			turnOrder = append(turnOrder, record.Envelope.ID)
		case StoryEventTypeNarrativeMemory:
			var event NarrativeMemoryEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				continue
			}
			memoryEvents = append(memoryEvents, event)
		}
	}
	return memoryEvents, turnOrder
}

// memoryPrefilterReason 返回记录被前置过滤的原因;空 = 通过。
func memoryPrefilterReason(record NarrativeMemoryRecord, kind, subject string) string {
	if kind != "" && record.Kind != kind {
		return "kind_mismatch"
	}
	if subject != "" && normalizeMemorySearchText(record.Subject) != subject {
		return "subject_mismatch"
	}
	return ""
}

// scoreMemoryRecords 对候选记录做关键词打分。空关键词 = 全部命中
// (记忆浏览模式),得分按 ValidFrom 新旧区分。
func scoreMemoryRecords(candidates []NarrativeMemoryRecord, keywords []string) (hits []scoredMemoryHit, filtered []MemoryFilteredItem, matched int) {
	hits = make([]scoredMemoryHit, 0, len(candidates))
	for _, record := range candidates {
		score, parts, boost := memoryKeywordScore(record, keywords)
		if len(keywords) > 0 && score <= 0 {
			filtered = append(filtered, MemoryFilteredItem{
				RecordID: record.ID,
				Subject:  record.Subject,
				Text:     boundedMemoryText(record.Text, 80),
				Reason:   "no_keyword_match",
			})
			continue
		}
		matched++
		hits = append(hits, scoredMemoryHit{
			hit: MemorySearchHit{
				RecordID:  record.ID,
				Kind:      record.Kind,
				Subject:   record.Subject,
				Object:    record.Object,
				Text:      record.Text,
				Evidence:  boundedMemoryText(record.Evidence, memoryEvidenceRunes),
				ValidFrom: record.ValidFrom,
				ValidTo:   record.ValidTo,
				Status:    record.Status,
				Score:     score,
			},
			parts:        parts,
			promiseBoost: boost,
		})
	}
	return hits, filtered, matched
}

// memoryKeywordScore 计算单条记录的关键词得分与命中分解。
// 每个关键词取其最佳命中来源,多关键词得分累加;伏笔类命中加成。
func memoryKeywordScore(record NarrativeMemoryRecord, keywords []string) (int, []MemoryKeywordPart, int) {
	if len(keywords) == 0 {
		return 1, nil, 0
	}
	subject := normalizeMemorySearchText(record.Subject)
	object := normalizeMemorySearchText(record.Object)
	text := normalizeMemorySearchText(record.Text)
	score := 0
	parts := make([]MemoryKeywordPart, 0, len(keywords))
	boost := 0
	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}
		part := MemoryKeywordPart{Keyword: keyword}
		switch {
		case subject == keyword:
			part.Field, part.Score = "subject", memoryScoreSubjectExact
		case strings.Contains(subject, keyword):
			part.Field, part.Score = "subject", memoryScoreSubjectContains
		case object != "" && strings.Contains(object, keyword):
			part.Field, part.Score = "object", memoryScoreObjectContains
		case strings.Contains(text, keyword):
			part.Field, part.Score = "text", memoryScoreTextContains
		default:
			continue
		}
		score += part.Score
		parts = append(parts, part)
	}
	if score > 0 && record.Kind == MemoryKindPromise {
		boost = memoryScorePromiseBoost
		score += boost
	}
	return score, parts, boost
}

// memoryExpansionAnchors 收集直接命中记录的实体作为一跳展开锚点。
func memoryExpansionAnchors(hits []scoredMemoryHit) map[string]bool {
	anchors := map[string]bool{}
	for _, hit := range hits {
		if subject := normalizeMemorySearchText(hit.hit.Subject); subject != "" {
			anchors[subject] = true
		}
		if object := normalizeMemorySearchText(hit.hit.Object); object != "" {
			anchors[object] = true
		}
	}
	return anchors
}

// memoryExpansionAnchorFor 返回记录可通过哪个锚点实体被一跳展开拉入;空 = 不可达。
func memoryExpansionAnchorFor(record NarrativeMemoryRecord, anchors map[string]bool) string {
	if len(anchors) == 0 {
		return ""
	}
	// 已过期记录不再通过展开拉入:当前状态查询不该带回被推翻的事实,
	// "当时如何"由直接关键词命中路径覆盖。
	if record.ValidTo != "" {
		return ""
	}
	if anchors[normalizeMemorySearchText(record.Subject)] {
		return record.Subject
	}
	if anchors[normalizeMemorySearchText(record.Object)] {
		return record.Object
	}
	return ""
}

func normalizeMemorySearchKeywords(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeMemorySearchText(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) == 8 {
			break
		}
	}
	return result
}

func normalizeMemoryKind(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !isValidMemoryKind(value) {
		return ""
	}
	return value
}

func normalizeMemorySearchLimit(value int) int {
	if value <= 0 {
		return DefaultMemorySearchLimit
	}
	if value > MaxMemorySearchLimit {
		return MaxMemorySearchLimit
	}
	return value
}
func normalizeMemorySearchText(value string) string {
	value = cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
	return strings.Join(strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	}), " ")
}

func boundedMemoryText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes-1])) + "…"
}
