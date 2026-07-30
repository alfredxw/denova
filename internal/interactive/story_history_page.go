package interactive

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"denova/internal/conversationjournal"
)

const (
	defaultStoryHistoryPageTurns = 100
	maxStoryHistoryPageTurns     = 200
	storyHistoryScanTransactions = 128
	storyHistoryCursorVersion    = 1
	storyRecentCacheRecordLimit  = maxStoryHistoryPageTurns * 2
)

type storyHistoryCursor struct {
	Version    int                        `json:"v"`
	Generation string                     `json:"generation"`
	BranchID   string                     `json:"branch_id"`
	Through    conversationjournal.Cursor `json:"through"`
	TargetID   string                     `json:"target_id"`
}

type locatedStoryRecord struct {
	record   StoryEventRecord
	cursor   conversationjournal.Cursor
	position int
}

type loadedStoryHistoryPage struct {
	page       StoryHistoryPage
	records    []StoryEventRecord
	meta       StoryMeta
	projection *storyBranchProjection
	pageHeadID string
	totalTurns int
	turnStart  int
	snapshot   Snapshot
}

// ReadHistoryPage reads an older bounded page on the resolved branch path.
// The opaque cursor is generation-scoped, so delete/recreate cannot silently
// splice two story incarnations together.
func (s *Store) ReadHistoryPage(storyID, branchID, beforeCursor string, limit int) (StoryHistoryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	loaded, err := s.readStoryHistoryPageLocked(storyID, branchID, beforeCursor, limit, true)
	if err != nil {
		return StoryHistoryPage{}, err
	}
	return loaded.page, nil
}

func (s *Store) readStoryRecentLocked(storyID, branchID string) (StoryMeta, []StoryEventRecord, error) {
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	if branchID == "" {
		branchID = handle.projection.Meta.CurrentBranch
	}
	head := handle.journal.Head()
	if cached, ok := handle.recent[branchID]; ok && cached.cursor == head.Cursor {
		meta, cloneErr := cloneStoryMeta(cached.meta)
		if cloneErr != nil {
			return StoryMeta{}, nil, cloneErr
		}
		records, cloneErr := cloneStoryEventRecords(cached.records)
		if cloneErr != nil {
			return StoryMeta{}, nil, cloneErr
		}
		s.rememberStoryReplayStats(storyID, StoryJournalReplayStats{})
		return meta, records, nil
	}
	loaded, err := s.readStoryHistoryPageLocked(storyID, branchID, "", maxStoryHistoryPageTurns, true)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	handle, err = s.openStoryJournalLocked(storyID)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	if err := cacheStoryRecentLoaded(handle, branchID, loaded.meta, loaded.records); err != nil {
		return StoryMeta{}, nil, err
	}
	return loaded.meta, loaded.records, nil
}

func cacheStoryRecentLoaded(handle *storyJournalHandle, branchID string, meta StoryMeta, records []StoryEventRecord) error {
	if handle == nil || handle.journal == nil {
		return nil
	}
	if len(records) > storyRecentCacheRecordLimit {
		records = records[len(records)-storyRecentCacheRecordLimit:]
	}
	cachedMeta, err := cloneStoryMeta(meta)
	if err != nil {
		return err
	}
	cachedRecords, err := cloneStoryEventRecords(records)
	if err != nil {
		return err
	}
	if handle.recent == nil {
		handle.recent = make(map[string]storyRecentCache)
	}
	handle.recent[branchID] = storyRecentCache{
		cursor: handle.journal.Head().Cursor, meta: cachedMeta, records: cachedRecords,
	}
	return nil
}

func cloneStoryEventRecords(records []StoryEventRecord) ([]StoryEventRecord, error) {
	data, err := json.Marshal(records)
	if err != nil {
		return nil, err
	}
	var cloned []StoryEventRecord
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func advanceStoryRecentCaches(handle *storyJournalHandle, cursor conversationjournal.Cursor, meta StoryMeta, events []StoryEventRecord) {
	if handle == nil || len(handle.recent) == 0 {
		return
	}
	for branchID, cached := range handle.recent {
		clonedMeta, err := cloneStoryMeta(meta)
		if err != nil {
			delete(handle.recent, branchID)
			continue
		}
		cached.meta = clonedMeta
		cached.cursor = cursor
		for _, event := range events {
			if event.Envelope.BranchID == branchID || event.Envelope.BranchID == "" {
				cached.records = append(cached.records, event)
			}
		}
		if len(cached.records) > storyRecentCacheRecordLimit {
			cached.records = append([]StoryEventRecord(nil), cached.records[len(cached.records)-storyRecentCacheRecordLimit:]...)
		}
		handle.recent[branchID] = cached
	}
}

func (s *Store) storyBranchProjectionLocked(storyID, branchID string) (*storyBranchProjection, error) {
	handle, err := s.openStoryJournalLocked(storyID)
	if err != nil {
		return nil, err
	}
	if branchID == "" {
		branchID = handle.projection.Meta.CurrentBranch
	}
	projection := handle.projection.Branches[branchID]
	if projection == nil {
		return nil, fmt.Errorf("分支不存在: %s", branchID)
	}
	return projection, nil
}

func (s *Store) readStoryHistoryPageLocked(storyID, branchID, beforeCursor string, limit int, repairTornTail bool) (loadedStoryHistoryPage, error) {
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return loadedStoryHistoryPage{}, err
	}
	defer release()

	handle, err := s.refreshStoryJournalLocked(storyID, repairTornTail)
	if err != nil {
		return loadedStoryHistoryPage{}, err
	}
	meta, err := cloneStoryMeta(handle.projection.Meta)
	if err != nil {
		return loadedStoryHistoryPage{}, err
	}
	meta = normalizeStoryMeta(meta)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return loadedStoryHistoryPage{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	projection := handle.projection.Branches[branchID]
	if projection == nil {
		projection = &storyBranchProjection{Head: branch.Head, State: initialStoryState()}
	}
	limit = normalizeStoryHistoryPageLimit(limit)

	through := projection.TailCursor
	targetID := strings.TrimSpace(branch.Head)
	if through == 0 {
		through = handle.journal.Head().Cursor
	}
	if strings.TrimSpace(beforeCursor) != "" {
		cursor, decodeErr := decodeStoryHistoryCursor(beforeCursor)
		if decodeErr != nil {
			return loadedStoryHistoryPage{}, decodeErr
		}
		if cursor.Generation != handle.projection.Generation || cursor.BranchID != branchID {
			return loadedStoryHistoryPage{}, fmt.Errorf("历史游标已失效，请重新加载 / History cursor is stale; reload the story")
		}
		through = cursor.Through
		targetID = cursor.TargetID
	}

	pathNewestFirst := make([]locatedStoryRecord, 0, limit*2)
	sideRecords := make([]locatedStoryRecord, 0, limit*2)
	turnsFound := 0
	nextTargetID := targetID
	var bytesRead int64
	physicalSeen := make(map[conversationjournal.Cursor]bool)
	transactionSeen := make(map[conversationjournal.Cursor]bool)
	firstScan := true
	for through > 0 && (firstScan || (nextTargetID != "" && turnsFound < limit)) {
		firstScan = false
		after := conversationjournal.Cursor(0)
		if through > storyHistoryScanTransactions {
			after = through - storyHistoryScanTransactions
		}
		records, readErr := handle.journal.ReadRange(context.Background(), conversationjournal.Range{
			After: after, Through: through, Limit: storyHistoryScanTransactions,
		})
		if readErr != nil {
			return loadedStoryHistoryPage{}, readErr
		}
		bytesRead += handle.journal.ReplayStats().LastRangeBytesRead
		for _, physical := range records {
			cursor := physical.Location.Cursor
			physicalSeen[cursor] = true
			if !physical.Legacy {
				transactionSeen[cursor] = true
				continue
			}
			_, _, legacyTransaction, decodeErr := decodeStoryProjectionPayload(physical.Payload)
			if decodeErr != nil {
				return loadedStoryHistoryPage{}, decodeErr
			}
			if legacyTransaction {
				transactionSeen[cursor] = true
			}
		}
		located, decodeErr := decodeLocatedStoryRecords(records)
		if decodeErr != nil {
			return loadedStoryHistoryPage{}, decodeErr
		}
		byID := make(map[string]locatedStoryRecord, len(located))
		for _, item := range located {
			if item.record.Envelope.ID != "" {
				byID[item.record.Envelope.ID] = item
			}
			if item.record.Envelope.Type == StoryEventTypeTurn || (item.record.Envelope.BranchID == branchID && isStoryHistorySideCandidate(item.record.Envelope.Type)) {
				sideRecords = append(sideRecords, item)
			}
		}
		for nextTargetID != "" && turnsFound < limit {
			item, found := byID[nextTargetID]
			if !found {
				break
			}
			pathNewestFirst = append(pathNewestFirst, item)
			nextTargetID = parentIDFromRaw(item.record.Raw)
			if item.record.Envelope.Type == StoryEventTypeTurn {
				turnsFound++
			}
		}
		through = after
		if after == 0 {
			break
		}
	}
	if nextTargetID != "" && through == 0 && turnsFound < limit {
		return loadedStoryHistoryPage{}, fmt.Errorf("故事分支父链不完整: missing=%s", nextTargetID)
	}

	records := storyHistoryProjectionRecords(pathNewestFirst, sideRecords, branchID)
	temporaryMeta := meta
	temporaryBranch := temporaryMeta.Branches[branchID]
	if len(pathNewestFirst) > 0 {
		temporaryBranch.Head = pathNewestFirst[0].record.Envelope.ID
	}
	temporaryMeta.Branches[branchID] = temporaryBranch
	snapshot, err := snapshotFromLines(storyID, branchID, temporaryMeta, records)
	if err != nil {
		return loadedStoryHistoryPage{}, err
	}
	turns := snapshot.Turns
	hasMore := nextTargetID != ""
	nextCursor := ""
	if hasMore && len(pathNewestFirst) > 0 {
		oldest := pathNewestFirst[len(pathNewestFirst)-1]
		nextThrough := conversationjournal.Cursor(0)
		if oldest.cursor > 1 {
			nextThrough = oldest.cursor - 1
		}
		nextCursor, err = encodeStoryHistoryCursor(storyHistoryCursor{
			Version: storyHistoryCursorVersion, Generation: handle.projection.Generation,
			BranchID: branchID, Through: nextThrough, TargetID: nextTargetID,
		})
		if err != nil {
			return loadedStoryHistoryPage{}, err
		}
	}
	totalTurns := projection.Depth
	if totalTurns < len(turns) {
		totalTurns = len(turns)
	}
	turnStart := totalTurns - len(turns)
	if beforeCursor != "" {
		turnStart = 0
	}
	s.rememberStoryReplayStats(storyID, StoryJournalReplayStats{
		BytesRead: bytesRead, RecordsRead: int64(len(physicalSeen)), TransactionsRead: int64(len(transactionSeen)), EventsRead: int64(len(records)),
	})
	return loadedStoryHistoryPage{
		page: StoryHistoryPage{
			StoryID: storyID, BranchID: branchID, Turns: turns,
			BeforeCursor: nextCursor, HasMore: hasMore,
		},
		records: records, meta: meta, projection: projection, pageHeadID: temporaryBranch.Head,
		totalTurns: totalTurns, turnStart: turnStart, snapshot: snapshot,
	}, nil
}

func decodeLocatedStoryRecords(records []conversationjournal.Record) ([]locatedStoryRecord, error) {
	result := make([]locatedStoryRecord, 0, len(records))
	for _, physical := range records {
		_, events, _, err := decodeStoryProjectionPayload(physical.Payload)
		if err != nil {
			return nil, fmt.Errorf("解析故事历史失败 (cursor %d): %w", physical.Location.Cursor, err)
		}
		for index, event := range events {
			result = append(result, locatedStoryRecord{record: event, cursor: physical.Location.Cursor, position: physical.Location.RecordIndex + index})
		}
	}
	return result, nil
}

func storyHistoryProjectionRecords(pathNewestFirst, candidates []locatedStoryRecord, branchID string) []StoryEventRecord {
	pathIDs := make(map[string]bool, len(pathNewestFirst))
	versionKeys := make(map[string]bool, len(pathNewestFirst))
	for _, item := range pathNewestFirst {
		pathIDs[item.record.Envelope.ID] = true
		if item.record.Envelope.Type == StoryEventTypeTurn {
			versionKeys[turnVersionKey(item.record.Envelope.BranchID, parentIDFromRaw(item.record.Raw))] = true
		}
	}
	selected := make([]locatedStoryRecord, 0, len(pathNewestFirst)+len(candidates))
	for index := len(pathNewestFirst) - 1; index >= 0; index-- {
		selected = append(selected, pathNewestFirst[index])
	}
	for _, item := range candidates {
		record := item.record
		if pathIDs[record.Envelope.ID] {
			continue
		}
		include := false
		switch record.Envelope.Type {
		case StoryEventTypeTurn:
			// A bounded recent graph may include neighboring branches. Only the
			// active ancestry contributes snapshot.Turns or model context.
			include = record.Envelope.BranchID != branchID || versionKeys[turnVersionKey(branchID, parentIDFromRaw(record.Raw))]
		case StoryEventTypeTurnNarrativeRevised, StoryEventTypeTurnDisplayAppended, StoryEventTypeTurnStateRevised:
			include = pathIDs[storyRevisionTurnID(record)]
		case StoryEventTypeHotChoices:
			include = pathIDs[parentIDFromRaw(record.Raw)]
		case StoryEventTypePlayerInput, StoryEventTypeModelContextBatch:
			parentID := parentIDFromRaw(record.Raw)
			include = parentID == "" || pathIDs[parentID]
		}
		if include {
			selected = append(selected, item)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].cursor != selected[j].cursor {
			return selected[i].cursor < selected[j].cursor
		}
		return selected[i].position < selected[j].position
	})
	result := make([]StoryEventRecord, 0, len(selected))
	seen := make(map[string]bool, len(selected))
	for _, item := range selected {
		key := fmt.Sprintf("%d:%d:%s", item.cursor, item.position, item.record.Envelope.ID)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item.record)
	}
	return result
}

func storyRevisionTurnID(record StoryEventRecord) string {
	switch record.Envelope.Type {
	case StoryEventTypeTurnNarrativeRevised:
		var event TurnNarrativeRevisedEvent
		_ = mapToStruct(record.Raw, &event)
		return event.TurnID
	case StoryEventTypeTurnDisplayAppended:
		var event TurnDisplayAppendedEvent
		_ = mapToStruct(record.Raw, &event)
		return event.TurnID
	case StoryEventTypeTurnStateRevised:
		var event TurnStateRevisedEvent
		_ = mapToStruct(record.Raw, &event)
		return event.TurnID
	default:
		return ""
	}
}

func isStoryHistorySideCandidate(eventType string) bool {
	switch eventType {
	case StoryEventTypeTurn, StoryEventTypePlayerInput, StoryEventTypeModelContextBatch, StoryEventTypeHotChoices,
		StoryEventTypeTurnNarrativeRevised, StoryEventTypeTurnDisplayAppended, StoryEventTypeTurnStateRevised:
		return true
	default:
		return false
	}
}

func normalizeStoryHistoryPageLimit(limit int) int {
	if limit <= 0 {
		return defaultStoryHistoryPageTurns
	}
	if limit > maxStoryHistoryPageTurns {
		return maxStoryHistoryPageTurns
	}
	return limit
}

func encodeStoryHistoryCursor(cursor storyHistoryCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeStoryHistoryCursor(value string) (storyHistoryCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return storyHistoryCursor{}, fmt.Errorf("历史游标无效 / Invalid history cursor")
	}
	var cursor storyHistoryCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Version != storyHistoryCursorVersion || cursor.Generation == "" || cursor.BranchID == "" || cursor.TargetID == "" {
		return storyHistoryCursor{}, fmt.Errorf("历史游标无效 / Invalid history cursor")
	}
	return cursor, nil
}

func cloneStoryMeta(meta StoryMeta) (StoryMeta, error) {
	data, err := json.Marshal(meta)
	if err != nil {
		return StoryMeta{}, err
	}
	var cloned StoryMeta
	if err := json.Unmarshal(data, &cloned); err != nil {
		return StoryMeta{}, err
	}
	return cloned, nil
}
