package interactive

import (
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
// 向量与 RRF 行在阶段 2 前恒为 0。
type MemorySearchPipelineCounters struct {
	ProjectedEvents  int `json:"projected_events"`
	DedupedRecords   int `json:"deduped_records"`
	StaleRecords     int `json:"stale_records"`
	ValidRecords     int `json:"valid_records"`
	ExpiredRecords   int `json:"expired_records"`
	Candidates       int `json:"candidates"`       // 通过 kind/subject 前置过滤的记录数
	KeywordMatched   int `json:"keyword_matched"`  // 关键词打分 > 0 的记录数
	VectorCandidates int `json:"vector_candidates"` // 阶段 2;未启用恒 0
	FusedRanked      int `json:"fused_ranked"`      // 阶段 2 前 = KeywordMatched
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
	Limit        int                `json:"limit"`
	Truncated    bool               `json:"truncated"`
	Hits         []MemorySearchHit  `json:"hits"`
	Explain      *MemorySearchExplain `json:"explain,omitempty"`
}

// SearchStoryMemory 在当前分支的叙事记忆投影上执行查询条件化检索。
// 空关键词时返回最近视角的有效记录(兼任记忆浏览器);命中后按锚点实体
// 一跳展开同实体记录(默认开启),把"现状 + 认知边界 + 悬置伏笔"整体带出。
func (s *Store) SearchStoryMemory(storyID, branchID string, req MemorySearchRequest) (MemorySearchResult, error) {
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
	explain.Pipeline.FusedRanked = keywordMatched

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

	sort.SliceStable(merged, func(i, j int) bool {
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
		Limit:        limit,
		Truncated:    truncated,
		Hits:         hits,
		Explain:      &explain,
	}, nil
}

const (
	memoryExpansionScore = 40 // 一跳展开拉入的固定保底分:低于多数直接命中,高于无关键词浏览

	memoryScoreSubjectExact = 110
	memoryScoreSubjectContains = 60
	memoryScoreObjectContains = 70
	memoryScoreTextContains  = 50
	memoryScorePromiseBoost  = 15
)

type scoredMemoryHit struct {
	hit          MemorySearchHit
	parts        []MemoryKeywordPart
	promiseBoost int
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
