package interactive

import (
	"sort"
	"strings"
)

func (s *Store) VisibleInteractiveMemories(storyID, branchID string, limit int) ([]InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return nil, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return nil, err
	}
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), false)
	entries := storyMemoryRecordsToInteractiveEntries(records, structures)
	if limit <= 0 || limit > maxMemoryListItems {
		limit = maxMemoryListItems
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (s *Store) ReadVisibleInteractiveMemories(storyID, branchID string, ids []string, limit int) ([]InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return nil, err
	}
	wanted := sanitizeStringList(ids)
	if len(wanted) == 0 {
		return []InteractiveMemoryEntry{}, nil
	}
	if limit <= 0 || limit > len(wanted) {
		limit = len(wanted)
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return nil, err
	}
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	visible := storyMemoryRecordsToInteractiveEntries(visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), false), structures)
	byID := make(map[string]InteractiveMemoryEntry, len(visible))
	for _, entry := range visible {
		byID[entry.ID] = entry
	}
	capacity := len(wanted)
	if capacity > limit {
		capacity = limit
	}
	out := make([]InteractiveMemoryEntry, 0, capacity)
	seen := map[string]bool{}
	for _, id := range wanted {
		if seen[id] {
			continue
		}
		entry, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, entry)
		seen[id] = true
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func visibleStoryMemoryRecords(records []StoryMemoryRecord, branchID string, pathSet map[string]bool, includeArchived bool) []StoryMemoryRecord {
	candidates := make([]StoryMemoryRecord, 0, len(records))
	for _, record := range records {
		if !recordVisibleOnBranch(record, branchID, pathSet) {
			continue
		}
		candidates = append(candidates, record)
	}
	overridden := map[string]bool{}
	for _, record := range candidates {
		if record.InheritedFrom != "" {
			overridden[record.InheritedFrom] = true
		}
	}
	out := make([]StoryMemoryRecord, 0, len(candidates))
	for _, record := range candidates {
		if overridden[record.ID] {
			continue
		}
		if record.Archived && !includeArchived {
			continue
		}
		out = append(out, record)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt == out[j].UpdatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

func recordVisibleOnBranch(record StoryMemoryRecord, branchID string, pathSet map[string]bool) bool {
	if record.BranchID == branchID {
		return true
	}
	anchor := firstMemoryText(record.AnchorTurnID, record.TurnID)
	return anchor != "" && pathSet[anchor]
}

func storyMemoryRecordsToInteractiveEntries(records []StoryMemoryRecord, structures []StoryMemoryStructure) []InteractiveMemoryEntry {
	entries := make([]InteractiveMemoryEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(structures, record.StructureID)))
	}
	return entries
}

func storyMemoryRecordToInteractiveEntry(record StoryMemoryRecord, structure StoryMemoryStructure) InteractiveMemoryEntry {
	title := record.Key
	if title == "" {
		title = firstMemoryText(record.Values["title"], record.Values["name"], structure.Name)
	}
	summary := firstMemoryText(record.Values["summary"], record.Values["event"], record.Values["description"], record.Values["brief"])
	contentParts := make([]string, 0, len(record.Values))
	used := map[string]bool{}
	for _, field := range structure.Fields {
		if value := strings.TrimSpace(record.Values[field.ID]); value != "" {
			contentParts = append(contentParts, field.Name+"："+value)
			used[field.ID] = true
		}
	}
	for key, value := range record.Values {
		if used[key] || strings.TrimSpace(value) == "" {
			continue
		}
		contentParts = append(contentParts, key+"："+value)
	}
	content := strings.Join(contentParts, "\n")
	return InteractiveMemoryEntry{
		ID:         record.ID,
		BranchID:   record.BranchID,
		TurnID:     record.TurnID,
		Title:      trimMemoryText(title),
		Summary:    trimMemoryText(summary),
		Content:    trimMemoryText(content),
		People:     valueListFromRecord(record, []string{"name", "people"}),
		Places:     valueListFromRecord(record, []string{"location", "place"}),
		Tags:       []string{structure.Name},
		Importance: defaultMemoryImportance,
		Archived:   record.Archived,
		Manual:     record.Manual,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}
}

func valueListFromRecord(record StoryMemoryRecord, keys []string) []string {
	var values []string
	for _, key := range keys {
		if value := strings.TrimSpace(record.Values[key]); value != "" {
			values = append(values, value)
		}
	}
	return sanitizeStringList(values)
}

func eventPathSet(headID string, lines []StoryEventRecord) map[string]bool {
	_, pathSet := eventPath(headID, eventsByID(lines))
	return pathSet
}

func turnPath(lines []StoryEventRecord, headID string) []TurnEvent {
	path, _ := eventPath(headID, eventsByID(lines))
	turns := make([]TurnEvent, 0, len(path))
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err == nil {
			turns = append(turns, turn)
		}
	}
	return turns
}
