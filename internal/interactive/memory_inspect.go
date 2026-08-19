package interactive

import (
	"strings"
)

// MemoryLibraryEntry 是记忆库浏览视图中的一条记录:投影记录 + 所属事件的
// 溯源信息,供观测面板按 kind 分组展示。
type MemoryLibraryEntry struct {
	NarrativeMemoryRecord `json:",inline"`
	EventID               string `json:"event_id"`
	Epoch                 int    `json:"epoch"`
	SourceTurnID          string `json:"source_turn_id"`
	Ts                    string `json:"ts"`
}

// MemoryLibraryView 是一个故事的叙事记忆库浏览视图。
type MemoryLibraryView struct {
	StoryID   string                `json:"story_id"`
	BranchID  string                `json:"branch_id"`
	Kind      string                `json:"kind,omitempty"`
	BeforeTurnID string             `json:"before_turn_id,omitempty"`
	Entries   []MemoryLibraryEntry  `json:"entries"`
	Stats     MemoryLibraryStats    `json:"stats"`
}

// MemoryLibraryStats 是从事件流推导的记忆库健康指标,零额外存储。
type MemoryLibraryStats struct {
	TotalTurns      int            `json:"total_turns"`
	TurnsWithMemory int            `json:"turns_with_memory"`
	Coverage        float64        `json:"coverage"`
	Events          int            `json:"events"`
	Records         int            `json:"records"`
	KindCounts      map[string]int `json:"kind_counts"`
	OpenPromises    int            `json:"open_promises"`
	PaidPromises    int            `json:"paid_promises"`
	ExpiredRecords  int            `json:"expired_records"`
	// LastPublish 是最近一次抽取事件的 Trace(可能为 nil,如仅手工注入)。
	LastPublish *NarrativeMemoryTrace `json:"last_publish,omitempty"`
}

// BrowseStoryMemory 返回故事的记忆库浏览视图与统计。
// kind 非空时只返回该类型;beforeTurnID 语义与 SearchStoryMemory 一致。
func (s *Store) BrowseStoryMemory(storyID, branchID, kind, beforeTurnID string) (MemoryLibraryView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return MemoryLibraryView{}, err
	}
	branchID, branch, err := resolveBranch(meta, strings.TrimSpace(branchID))
	if err != nil {
		return MemoryLibraryView{}, err
	}
	path, _ := eventPath(branch.Head, eventsByID(lines))
	memoryEvents, turnOrder := collectNarrativeMemory(path)

	kind = normalizeMemoryKind(kind)
	projection := ProjectNarrativeMemory(memoryEvents, turnOrder, strings.TrimSpace(beforeTurnID))

	// 记录 → 所属事件溯源:投影只保留记录,这里重建 ID→事件的映射。
	entryByRecord := map[string]MemoryLibraryEntry{}
	for _, event := range memoryEvents {
		for _, record := range event.Records {
			if _, exists := entryByRecord[record.ID]; !exists {
				entryByRecord[record.ID] = MemoryLibraryEntry{
					NarrativeMemoryRecord: record,
					EventID:               event.ID,
					Epoch:                 event.Epoch,
					SourceTurnID:          event.SourceTurnID,
					Ts:                    event.Ts,
				}
			}
		}
	}
	view := MemoryLibraryView{
		StoryID:      storyID,
		BranchID:     branchID,
		Kind:         kind,
		BeforeTurnID: strings.TrimSpace(beforeTurnID),
		Entries:      []MemoryLibraryEntry{},
	}
	for _, record := range projection.Records {
		if kind != "" && record.Kind != kind {
			continue
		}
		if entry, ok := entryByRecord[record.ID]; ok {
			view.Entries = append(view.Entries, entry)
		} else {
			view.Entries = append(view.Entries, MemoryLibraryEntry{
				NarrativeMemoryRecord: record,
				SourceTurnID:          record.ValidFrom,
			})
		}
	}

	view.Stats = memoryLibraryStats(memoryEvents, turnOrder, projection)
	return view, nil
}

// memoryLibraryStats 从事件流推导覆盖率与分布指标。
func memoryLibraryStats(memoryEvents []NarrativeMemoryEvent, turnOrder []string, projection MemoryProjection) MemoryLibraryStats {
	stats := MemoryLibraryStats{KindCounts: map[string]int{}}
	stats.TotalTurns = len(turnOrder)
	turnsWithMemory := map[string]bool{}
	for _, event := range memoryEvents {
		stats.Events++
		turnsWithMemory[event.SourceTurnID] = true
		// LastPublish 取最后一个带 Trace 的事件(路径按时间序)。
		if event.Trace != nil {
			stats.LastPublish = event.Trace
		}
	}
	stats.TurnsWithMemory = len(turnsWithMemory)
	if stats.TotalTurns > 0 {
		stats.Coverage = float64(stats.TurnsWithMemory) / float64(stats.TotalTurns)
	}
	for _, record := range projection.Records {
		stats.Records++
		stats.KindCounts[record.Kind]++
		if record.Kind == MemoryKindPromise {
			if record.Status == MemoryStatusPaid {
				stats.PaidPromises++
			} else {
				stats.OpenPromises++
			}
		}
		if record.ValidTo != "" {
			stats.ExpiredRecords++
		}
	}
	return stats
}
