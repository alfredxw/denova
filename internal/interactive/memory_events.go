package interactive

import (
	"fmt"
	"strings"
	"time"
)

// AppendNarrativeMemory 把一次抽取产出的记忆记录追加为故事事件。
// 形态对齐 AppendContextCompaction:空字段补默认值、epoch 沿分支路径递增、
// 追加后推进分支头并触达索引。SourceTurnID 必填且必须指向当前分支路径上
// 已存在的回合,保证每条记忆可回溯到事件日志中的事实来源。
func (s *Store) AppendNarrativeMemory(storyID, branchID string, event NarrativeMemoryEvent) (NarrativeMemoryEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return NarrativeMemoryEvent{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return NarrativeMemoryEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	if strings.TrimSpace(event.SourceTurnID) == "" {
		return NarrativeMemoryEvent{}, fmt.Errorf("记忆事件缺少 source_turn_id")
	}
	if err := validateNarrativeMemorySource(lines, branch.Head, event.SourceTurnID); err != nil {
		return NarrativeMemoryEvent{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if event.ID == "" {
		event.ID = newID("nm")
	}
	event.V = schemaVersion
	event.Type = StoryEventTypeNarrativeMemory
	event.ParentID = branch.Head
	event.BranchID = branchID
	if event.Ts == "" {
		event.Ts = now
	}
	if event.Epoch <= 0 {
		event.Epoch = nextNarrativeMemoryEpoch(lines, branch.Head)
	}
	event.Records = normalizeNarrativeMemoryRecords(event.Records, event)
	// 实体对齐是写入路径的不变量,不是抽取器旁边的一个可选步骤:任何注入
	// 途径(模型抽取、手动注入、测试夹具)写进来的记录都要对齐到已有写法,
	// 否则检索侧的字面实体匹配会被写法漂移切断。
	// 名册取本事件之前的记录 —— 本事件自己的记录不能当自己的权威来源。
	if aligned := alignRecordsToRoster(event.Records, entityRosterFromLines(lines, branch.Head, "", DefaultMemoryRosterLimit)); len(aligned) > 0 {
		if event.Trace == nil {
			event.Trace = &NarrativeMemoryTrace{}
		}
		event.Trace.AlignedEntities = append(event.Trace.AlignedEntities, aligned...)
	}
	if err := validateNarrativeMemoryRecords(event.Records); err != nil {
		return NarrativeMemoryEvent{}, err
	}
	branch.Head = event.ID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return NarrativeMemoryEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return NarrativeMemoryEvent{}, err
	}
	return event, nil
}

// validateNarrativeMemorySource 要求被抽取的回合真实存在于当前分支路径上,
// 防止记忆记录指向其它分支或已废弃的历史。
func validateNarrativeMemorySource(lines []StoryEventRecord, head, sourceTurnID string) error {
	events := eventsByID(lines)
	path, _ := eventPath(head, events)
	for _, record := range path {
		if record.Envelope.Type == StoryEventTypeTurn && record.Envelope.ID == sourceTurnID {
			return nil
		}
	}
	return fmt.Errorf("记忆来源回合不在当前分支路径上: %s", sourceTurnID)
}

func nextNarrativeMemoryEpoch(lines []StoryEventRecord, head string) int {
	events := eventsByID(lines)
	path, _ := eventPath(head, events)
	epoch := 0
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeNarrativeMemory {
			continue
		}
		var memory NarrativeMemoryEvent
		if err := mapToStruct(record.Raw, &memory); err != nil {
			continue
		}
		if memory.Epoch > epoch {
			epoch = memory.Epoch
		}
	}
	return epoch + 1
}

func normalizeNarrativeMemoryRecords(records []NarrativeMemoryRecord, event NarrativeMemoryEvent) []NarrativeMemoryRecord {
	result := make([]NarrativeMemoryRecord, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		record.ID = strings.TrimSpace(record.ID)
		record.Kind = strings.TrimSpace(record.Kind)
		record.Subject = strings.TrimSpace(record.Subject)
		record.Object = strings.TrimSpace(record.Object)
		record.Text = strings.TrimSpace(record.Text)
		record.Evidence = strings.TrimSpace(record.Evidence)
		record.ValidFrom = strings.TrimSpace(record.ValidFrom)
		record.ValidTo = strings.TrimSpace(record.ValidTo)
		record.Status = strings.TrimSpace(record.Status)
		if record.ValidFrom == "" {
			record.ValidFrom = event.SourceTurnID
		}
		if record.ID == "" {
			record.ID = newID("mem")
		}
		if seen[record.ID] {
			continue
		}
		seen[record.ID] = true
		result = append(result, record)
	}
	return result
}

func validateNarrativeMemoryRecords(records []NarrativeMemoryRecord) error {
	for _, record := range records {
		if !isValidMemoryKind(record.Kind) {
			return fmt.Errorf("记忆记录 kind 无效: %q", record.Kind)
		}
		if record.Subject == "" {
			return fmt.Errorf("记忆记录缺少 subject: %s", record.ID)
		}
		if record.Text == "" {
			return fmt.Errorf("记忆记录缺少 text: %s", record.ID)
		}
		if record.Evidence == "" {
			return fmt.Errorf("记忆记录缺少 evidence: %s", record.ID)
		}
		if record.ValidFrom == "" {
			return fmt.Errorf("记忆记录缺少 valid_from: %s", record.ID)
		}
		if record.Status != "" && record.Status != MemoryStatusOpen && record.Status != MemoryStatusPaid {
			return fmt.Errorf("记忆记录 status 无效: %q", record.Status)
		}
	}
	return nil
}

func isValidMemoryKind(kind string) bool {
	switch kind {
	case MemoryKindKnowledge, MemoryKindReveal, MemoryKindPromise, MemoryKindObjectState, MemoryKindRelationship, MemoryKindBeat:
		return true
	}
	return false
}

// MemoryProjection 是沿一条分支路径投影出的叙事记忆切片。
// 计数字段构成观测瀑布:Events → DedupedRecords → ValidRecords,
// 与检索调试端的流水线展示一一对应。
type MemoryProjection struct {
	StoryID   string `json:"story_id"`
	BranchID  string `json:"branch_id"`
	// Events 是视角范围内的 memory 事件数(投影前)。
	Events int `json:"events"`
	// DedupedRecords 是 epoch 去重后纳入的记录数。
	DedupedRecords int `json:"deduped_records"`
	// StaleRecords 是被更大 epoch 作废的记录数。
	StaleRecords int `json:"stale_records"`
	// ValidRecords 是视角下仍然有效的记录数(候选池)。
	ValidRecords int `json:"valid_records"`
	// ExpiredRecords 是视角下已被推翻的记录数(保留供"当时如何"查询)。
	ExpiredRecords int `json:"expired_records"`
	// Records 按事件顺序排列:先按视角过滤,再标注是否有效。
	Records []NarrativeMemoryRecord `json:"records"`
}

// ProjectNarrativeMemory 把 memory 事件投影为视角化的记录切片。
//
// 三个口径:
//   - turnOrder 是分支路径上的回合顺序,决定 beforeTurnID 的位置语义;
//   - 有效期:valid_to 为空 = 开区间仍有效;非空且其回合位置早于视角 =
//     已被推翻(闭区间起点不含视角);晚于或等于视角 = 仍有效。
//   - beforeTurnID 为空 = 最新视角(路径末尾)。
//
// 已推翻的记录保留在 Records 中(带 ValidTo),供"何时被推翻"类查询,
// 但不计入 ValidRecords。
func ProjectNarrativeMemory(events []NarrativeMemoryEvent, turnOrder []string, beforeTurnID string) MemoryProjection {
	projection := MemoryProjection{Records: []NarrativeMemoryRecord{}}
	position := turnPositionIndex(turnOrder)
	beforePos := len(turnOrder)
	if id := strings.TrimSpace(beforeTurnID); id != "" {
		if pos, ok := position[id]; ok {
			beforePos = pos
		}
	}

	// pass 1: 视角范围内的每个 source_turn_id 只保留最大 epoch。
	latestEpochByTurn := map[string]int{}
	for _, event := range events {
		pos, ok := position[event.SourceTurnID]
		if !ok || pos >= beforePos {
			continue
		}
		projection.Events++
		if event.Epoch > latestEpochByTurn[event.SourceTurnID] {
			latestEpochByTurn[event.SourceTurnID] = event.Epoch
		}
	}
	// pass 2: 逐事件纳入或作废。
	taken := map[string]bool{}
	for _, event := range events {
		pos, ok := position[event.SourceTurnID]
		if !ok || pos >= beforePos {
			continue
		}
		if event.Epoch < latestEpochByTurn[event.SourceTurnID] || taken[event.SourceTurnID] {
			projection.StaleRecords += len(event.Records)
			continue
		}
		taken[event.SourceTurnID] = true
		projection.DedupedRecords += len(event.Records)
		for _, record := range event.Records {
			from, ok := position[record.ValidFrom]
			if !ok || from >= beforePos {
				continue
			}
			projection.Records = append(projection.Records, record)
		}
	}
	for _, record := range projection.Records {
		if memoryRecordExpired(record, position, beforePos) {
			projection.ExpiredRecords++
		} else {
			projection.ValidRecords++
		}
	}
	return projection
}

// memoryRecordExpired 判断记录在视角下是否已被推翻。
// valid_to 指向的回合不在 turnOrder 中时保守视为仍有效:检索宁可多带
// 一条标注,也不静默丢弃可能相关的事实。
func memoryRecordExpired(record NarrativeMemoryRecord, position map[string]int, beforePos int) bool {
	if record.ValidTo == "" {
		return false
	}
	pos, ok := position[record.ValidTo]
	if !ok {
		return false
	}
	return pos < beforePos
}

func turnPositionIndex(turnOrder []string) map[string]int {
	position := make(map[string]int, len(turnOrder))
	for i, turnID := range turnOrder {
		if _, exists := position[turnID]; !exists {
			position[turnID] = i
		}
	}
	return position
}
