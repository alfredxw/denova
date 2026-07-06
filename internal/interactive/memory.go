package interactive

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultMemoryImportance    = 3
	defaultStoryMemoryInterval = 6
	maxMemoryTextBytes         = DirectorContextMinBytes
	maxStoryMemorySchemaBytes  = DirectorContextMinBytes
	maxMemoryListItems         = 24
	maxMemoryRecalls           = 20
)

type interactiveMemoryBook struct {
	V          int                       `json:"v"`
	StoryID    string                    `json:"story_id"`
	Settings   StoryMemorySettings       `json:"settings"`
	Structures []StoryMemoryStructure    `json:"structures"`
	Records    []StoryMemoryRecord       `json:"records"`
	Entries    []InteractiveMemoryEntry  `json:"entries,omitempty"`
	Recalls    []InteractiveMemoryRecall `json:"recalls,omitempty"`
}

type storyMemoryStructureSource struct {
	ID       string
	Name     string
	Disabled bool
}

func (s *Store) memoryDir() string {
	return filepath.Join(s.root, "interactive", "memory")
}

func (s *Store) memoryPath(storyID string) string {
	return filepath.Join(s.memoryDir(), "story-"+storyID+".json")
}

func (s *Store) storyMemoryStructuresForStoryLocked(meta StoryMeta, book interactiveMemoryBook) ([]StoryMemoryStructure, storyMemoryStructureSource) {
	source := storyMemoryStructureSource{ID: "legacy", Name: "Story Memory"}
	if strings.TrimSpace(s.novaDir) == "" {
		return book.Structures, source
	}
	directorID := NormalizeStoryDirectorID(meta.StoryDirectorID)
	if directorID == "" {
		directorID = DefaultStoryDirectorID
	}
	director, err := NewStoryDirectorLibrary(s.novaDir).Get(directorID)
	if err != nil {
		log.Printf("[interactive-memory] fallback to story-local memory structures story_id=%s director_id=%s error=%v", meta.StoryID, directorID, err)
		return book.Structures, source
	}
	refs := NormalizeStoryDirectorModuleRefs(director.ModuleRefs)
	source = storyMemoryStructureSource{
		ID:       firstNonEmptyString(refs.MemoryStructureID, DefaultStoryMemoryStructureModuleID),
		Disabled: refs.MemoryStructureDisabled,
	}
	if module, err := NewStoryMemoryStructureLibrary(s.novaDir).Get(source.ID); err == nil {
		source.Name = module.Name
	} else {
		source.Name = source.ID
	}
	structures := normalizeStoryMemoryStructuresForModule(director.ResolvedSnapshot.StoryMemoryStructures)
	if len(structures) == 0 {
		if module, err := NewStoryMemoryStructureLibrary(s.novaDir).Get(source.ID); err == nil {
			structures = module.Structures
			source.Name = module.Name
		}
	}
	if len(structures) == 0 {
		log.Printf("[interactive-memory] fallback to story-local memory structures story_id=%s director_id=%s memory_structure_id=%s", meta.StoryID, directorID, source.ID)
		return book.Structures, source
	}
	return structures, source
}

func runtimeStoryMemoryStructures(structures []StoryMemoryStructure, source storyMemoryStructureSource) []StoryMemoryStructure {
	if source.Disabled {
		return []StoryMemoryStructure{}
	}
	return structures
}

func (s *Store) InteractiveMemory(storyID, branchID string, includeArchived bool) (InteractiveMemoryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryState{}, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return InteractiveMemoryState{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryState{}, err
	}
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), includeArchived)
	entries := storyMemoryRecordsToInteractiveEntries(records, structures)
	status, statusErr := latestMemorySyncStatus(lines, branchID, branch.Head)
	return InteractiveMemoryState{
		StoryID:      storyID,
		BranchID:     branchID,
		Entries:      entries,
		RecentRecall: latestMemoryRecall(book.Recalls, branchID),
		SyncStatus:   status,
		SyncError:    statusErr,
	}, nil
}

func (s *Store) StoryMemory(storyID, branchID string, includeArchived bool) (StoryMemoryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StoryMemoryState{}, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return StoryMemoryState{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemoryState{}, err
	}
	structures, structureSource := s.storyMemoryStructuresForStoryLocked(meta, book)
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), includeArchived)
	status, statusErr := latestMemorySyncStatus(lines, branchID, branch.Head)
	_, nextAuto := storyMemoryAutoDecisionLocked(book, lines, branchID, branch.Head)
	return StoryMemoryState{
		StoryID:                 storyID,
		BranchID:                branchID,
		Settings:                book.Settings,
		Structures:              structures,
		MemoryStructureID:       structureSource.ID,
		MemoryStructureName:     structureSource.Name,
		MemoryStructureDisabled: structureSource.Disabled,
		Records:                 records,
		RecentRecall:            latestMemoryRecall(book.Recalls, branchID),
		SyncStatus:              status,
		SyncError:               statusErr,
		NextAutoInTurns:         nextAuto,
	}, nil
}

func (s *Store) UpdateStoryMemorySettings(storyID string, req StoryMemorySettingsUpdateRequest) (StoryMemorySettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return StoryMemorySettings{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemorySettings{}, err
	}
	if req.Enabled != nil {
		book.Settings.Enabled = *req.Enabled
	}
	if req.AutoIntervalTurns != nil {
		book.Settings.AutoIntervalTurns = normalizeStoryMemoryInterval(*req.AutoIntervalTurns)
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemorySettings{}, err
	}
	return book.Settings, nil
}

func (s *Store) SaveStoryMemoryStructure(storyID string, req StoryMemoryStructureRequest) (StoryMemoryStructure, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return StoryMemoryStructure{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemoryStructure{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	structure := normalizeStoryMemoryStructure(req, now)
	if structure.ID == "" {
		structure.ID = newID("sm")
	}
	if err := validateStoryMemoryStructure(structure); err != nil {
		return StoryMemoryStructure{}, err
	}
	for i := range book.Structures {
		if book.Structures[i].ID != structure.ID {
			continue
		}
		if book.Structures[i].ReadOnly {
			return StoryMemoryStructure{}, fmt.Errorf("故事记忆结构为只读派生表，不能编辑: %s", structure.ID)
		}
		structure.BuiltIn = book.Structures[i].BuiltIn
		structure.ReadOnly = book.Structures[i].ReadOnly
		structure.Derived = book.Structures[i].Derived
		structure.CreatedAt = firstMemoryText(book.Structures[i].CreatedAt, now)
		structure.UpdatedAt = now
		book.Structures[i] = structure
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return StoryMemoryStructure{}, err
		}
		return structure, nil
	}
	structure.CreatedAt = now
	structure.UpdatedAt = now
	book.Structures = append(book.Structures, structure)
	sortStoryMemoryStructures(book.Structures)
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemoryStructure{}, err
	}
	return structure, nil
}

func (s *Store) DeleteStoryMemoryStructure(storyID, structureID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return err
	}
	structureID = strings.TrimSpace(structureID)
	if structureID == "" {
		return fmt.Errorf("故事记忆结构 ID 不能为空")
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return err
	}
	next := book.Structures[:0]
	removed := false
	for _, structure := range book.Structures {
		if structure.ID == structureID {
			if structure.ReadOnly {
				return fmt.Errorf("故事记忆结构为只读派生表，不能删除: %s", structureID)
			}
			removed = true
			continue
		}
		next = append(next, structure)
	}
	if !removed {
		return fmt.Errorf("故事记忆结构不存在: %s", structureID)
	}
	book.Structures = next
	for i := range book.Records {
		if book.Records[i].StructureID == structureID {
			book.Records[i].Archived = true
			book.Records[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	return s.writeMemoryBookLocked(storyID, book)
}

func (s *Store) SaveStoryMemoryRecord(storyID string, req StoryMemoryRecordRequest) (StoryMemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	branchID, branch, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	record, err := saveStoryMemoryRecordWithStructuresLocked(&book, structures, branchID, branch.Head, req, true, eventPathSet(branch.Head, lines))
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemoryRecord{}, err
	}
	return record, nil
}

func (s *Store) SetStoryMemoryRecordArchived(storyID, recordID, branchID string, archived bool) (StoryMemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	record, err := setStoryMemoryRecordArchivedLocked(&book, branchID, branch.Head, recordID, archived, eventPathSet(branch.Head, lines))
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemoryRecord{}, err
	}
	return record, nil
}

func (s *Store) ApplyStoryMemoryPatches(storyID, branchID, turnID string, patches []StoryMemoryPatch) ([]StoryMemoryRecord, error) {
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
	anchorTurnID := strings.TrimSpace(turnID)
	if anchorTurnID == "" {
		anchorTurnID = branch.Head
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return nil, err
	}
	structures, structureSource := s.storyMemoryStructuresForStoryLocked(meta, book)
	runtimeStructures := runtimeStoryMemoryStructures(structures, structureSource)
	if len(runtimeStructures) == 0 {
		log.Printf("[interactive-memory] skip story memory patches because memory structure is disabled or empty story_id=%s branch_id=%s memory_structure_id=%s disabled=%t", storyID, branchID, structureSource.ID, structureSource.Disabled)
		return []StoryMemoryRecord{}, nil
	}
	pathSet := eventPathSet(branch.Head, lines)
	records := make([]StoryMemoryRecord, 0, len(patches))
	for _, patch := range patches {
		normalizedPatch, ok := normalizeStoryMemoryPatchForAgent(book, runtimeStructures, patch)
		if !ok {
			log.Printf("[interactive-memory] skip story memory patch with missing keyed key story_id=%s branch_id=%s structure_id=%s", storyID, branchID, patch.StructureID)
			continue
		}
		record, err := applyStoryMemoryPatchWithStructuresLocked(&book, runtimeStructures, branchID, anchorTurnID, normalizedPatch, pathSet)
		if err != nil {
			return nil, err
		}
		if record.ID != "" {
			records = append(records, record)
		}
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return nil, err
	}
	return records, nil
}

func normalizeStoryMemoryPatchForAgent(book interactiveMemoryBook, structures []StoryMemoryStructure, patch StoryMemoryPatch) (StoryMemoryPatch, bool) {
	op := strings.TrimSpace(patch.Op)
	if op == "" {
		op = "upsert"
	}
	if op == "archive" || op == "restore" {
		return patch, true
	}
	structureID := sanitizeMemoryID(patch.StructureID)
	structure := storyMemoryStructureByID(structures, structureID)
	if structure.ID == "" || !storyMemoryStructureEnabled(structure) {
		return patch, false
	}
	if len(patch.Values) > 0 {
		nextValues := make(map[string]string, len(patch.Values))
		enabledFieldIDs := make(map[string]bool, len(structure.Fields))
		for _, field := range structure.Fields {
			if storyMemoryFieldEnabled(field) {
				enabledFieldIDs[field.ID] = true
			}
		}
		for key, value := range patch.Values {
			if enabledFieldIDs[key] {
				nextValues[key] = value
			}
		}
		patch.Values = nextValues
	}
	if structure.Mode != "keyed" {
		return patch, true
	}
	if strings.TrimSpace(patch.Key) != "" {
		return patch, true
	}
	if structure.KeyFieldID != "" {
		if key := strings.TrimSpace(patch.Values[structure.KeyFieldID]); key != "" {
			patch.Key = key
			return patch, true
		}
	}
	for _, record := range book.Records {
		if record.ID == sanitizeMemoryID(patch.RecordID) && record.StructureID == structure.ID {
			patch.Key = record.Key
			return patch, strings.TrimSpace(patch.Key) != ""
		}
	}
	return patch, false
}

func (s *Store) ShouldGenerateStoryMemory(storyID, branchID string) (bool, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return false, 0, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return false, 0, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return false, 0, err
	}
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	if len(runtimeStoryMemoryStructures(structures, source)) == 0 {
		return false, 0, nil
	}
	should, next := storyMemoryAutoDecisionLocked(book, lines, branchID, branch.Head)
	return should, next, nil
}

func (s *Store) StoryMemoryContextSummary(storyID, branchID string, limit int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return "", err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return "", err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return "", err
	}
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	structures = runtimeStoryMemoryStructures(structures, source)
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), false)
	return formatStoryMemoryContextSummary(structures, records, limit), nil
}

func (s *Store) StoryMemoryCompactionContext(storyID, branchID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return "", err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return "", err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return "", err
	}
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	structures = runtimeStoryMemoryStructures(structures, source)
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), false)
	return formatStoryMemoryCompactionContext(structures, records), nil
}

func (s *Store) StoryMemorySchemaContext(storyID string, limit int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return "", err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return "", err
	}
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	structures = runtimeStoryMemoryStructures(structures, source)
	return formatStoryMemorySchemaContext(structures, limit), nil
}

func (s *Store) CreateInteractiveMemory(storyID string, req InteractiveMemoryCreateRequest) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	branchID, branch, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	record, err := saveStoryMemoryRecordWithStructuresLocked(&book, structures, branchID, branch.Head, interactiveMemoryCreateToStoryRecord(req), true, eventPathSet(branch.Head, lines))
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(structures, record.StructureID)), nil
}

func (s *Store) UpdateInteractiveMemory(storyID, memoryID string, req InteractiveMemoryUpdateRequest) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	branchID, branch, err := resolveBranch(meta, "")
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return InteractiveMemoryEntry{}, fmt.Errorf("记忆 ID 不能为空")
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pathSet := eventPathSet(branch.Head, lines)
	for i := range book.Records {
		if book.Records[i].ID != memoryID {
			continue
		}
		next := book.Records[i]
		applyInteractiveMemoryUpdateToRecord(&next, req)
		record, err := saveStoryMemoryRecordWithStructuresLocked(&book, structures, branchID, branch.Head, StoryMemoryRecordRequest{
			ID:          next.ID,
			BranchID:    branchID,
			StructureID: next.StructureID,
			TurnID:      next.TurnID,
			Key:         next.Key,
			Values:      next.Values,
			Manual:      next.Manual,
		}, next.Manual, pathSet)
		if err != nil {
			return InteractiveMemoryEntry{}, err
		}
		record.UpdatedAt = now
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return InteractiveMemoryEntry{}, err
		}
		return storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(structures, record.StructureID)), nil
	}
	return InteractiveMemoryEntry{}, fmt.Errorf("记忆不存在: %s", memoryID)
}

func (s *Store) SetInteractiveMemoryArchived(storyID, memoryID string, archived bool) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return InteractiveMemoryEntry{}, fmt.Errorf("记忆 ID 不能为空")
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range book.Records {
		if book.Records[i].ID != memoryID {
			continue
		}
		book.Records[i].Archived = archived
		book.Records[i].UpdatedAt = now
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return InteractiveMemoryEntry{}, err
		}
		return storyMemoryRecordToInteractiveEntry(book.Records[i], storyMemoryStructureByID(structures, book.Records[i].StructureID)), nil
	}
	return InteractiveMemoryEntry{}, fmt.Errorf("记忆不存在: %s", memoryID)
}

func (s *Store) AppendInteractiveMemory(storyID, branchID, turnID string, req InteractiveMemoryCreateRequest) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return InteractiveMemoryEntry{}, fmt.Errorf("记忆缺少所属回合")
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	runtimeStructures := runtimeStoryMemoryStructures(structures, source)
	if len(runtimeStructures) == 0 {
		return InteractiveMemoryEntry{}, fmt.Errorf("故事记忆结构已禁用或为空")
	}
	recordReq := interactiveMemoryCreateToStoryRecord(req)
	recordReq.BranchID = branchID
	recordReq.TurnID = turnID
	record, err := saveStoryMemoryRecordWithStructuresLocked(&book, runtimeStructures, branchID, branch.Head, recordReq, false, eventPathSet(branch.Head, lines))
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.markTurnMemoryReadyLocked(storyID, meta, lines, branchID, turnID, record.ID); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(runtimeStructures, record.StructureID)), nil
}

func (s *Store) MarkInteractiveMemoryReady(storyID, branchID, turnID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return err
	}
	return s.markTurnMemoryReadyLocked(storyID, meta, lines, branchID, strings.TrimSpace(turnID), "")
}

func (s *Store) MarkInteractiveMemoryFailed(storyID string, req MarkStateFailedRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		return fmt.Errorf("记忆失败标记缺少所属回合")
	}
	errText := strings.TrimSpace(req.Error)
	if errText == "" {
		errText = "记忆生成失败"
	}
	updated := false
	for _, record := range lines {
		raw := record.Raw
		if record.Envelope.ID != parentID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		raw["memory_status"] = "failed"
		raw["memory_error"] = errText
		if current, ok := raw["state_status"].(string); ok && current == "pending" {
			raw["state_status"] = "failed"
			raw["state_error"] = errText
		}
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("记忆失败标记所属回合不存在: %s", parentID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

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

func (s *Store) RecordInteractiveMemoryRecall(storyID, branchID, turnID, query string, memoryIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return err
	}
	recall := InteractiveMemoryRecall{
		BranchID:  branchID,
		TurnID:    strings.TrimSpace(turnID),
		Query:     trimMemoryText(query),
		MemoryIDs: sanitizeStringList(memoryIDs),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	book.Recalls = append(book.Recalls, recall)
	if len(book.Recalls) > maxMemoryRecalls {
		book.Recalls = book.Recalls[len(book.Recalls)-maxMemoryRecalls:]
	}
	return s.writeMemoryBookLocked(storyID, book)
}

func (s *Store) readMemoryBookLocked(storyID string) (interactiveMemoryBook, error) {
	path := s.memoryPath(storyID)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return normalizeMemoryBook(interactiveMemoryBook{V: 2, StoryID: storyID, Settings: StoryMemorySettings{Enabled: true, AutoIntervalTurns: defaultStoryMemoryInterval}}), nil
	}
	if err != nil {
		return interactiveMemoryBook{}, err
	}
	var book interactiveMemoryBook
	if err := json.Unmarshal(data, &book); err != nil {
		return interactiveMemoryBook{}, fmt.Errorf("解析互动记忆失败: %w", err)
	}
	book.StoryID = storyID
	return normalizeMemoryBook(book), nil
}

func (s *Store) writeMemoryBookLocked(storyID string, book interactiveMemoryBook) error {
	if err := os.MkdirAll(s.memoryDir(), 0o755); err != nil {
		return err
	}
	book = normalizeMemoryBook(book)
	book.V = 2
	book.StoryID = storyID
	book.Entries = nil
	data, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		return err
	}
	path := s.memoryPath(storyID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) markTurnMemoryReadyLocked(storyID string, meta StoryMeta, lines []StoryEventRecord, branchID, turnID, memoryID string) error {
	if turnID == "" {
		return fmt.Errorf("记忆完成标记缺少所属回合")
	}
	updated := false
	for _, record := range lines {
		raw := record.Raw
		if record.Envelope.ID != turnID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		raw["memory_status"] = "ready"
		if memoryID != "" {
			raw["memory_entry_id"] = memoryID
		}
		delete(raw, "memory_error")
		if current, ok := raw["state_status"].(string); ok && current == "pending" {
			raw["state_status"] = "ready"
			delete(raw, "state_error")
		}
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("记忆所属回合不存在: %s", turnID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func newInteractiveMemoryEntry(branchID, turnID string, manual bool, req InteractiveMemoryCreateRequest) (InteractiveMemoryEntry, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	entry := InteractiveMemoryEntry{
		ID:         newID("mem"),
		BranchID:   strings.TrimSpace(branchID),
		TurnID:     strings.TrimSpace(turnID),
		Title:      trimMemoryText(req.Title),
		Summary:    trimMemoryText(req.Summary),
		Content:    trimMemoryText(req.Content),
		People:     sanitizeStringList(req.People),
		Places:     sanitizeStringList(req.Places),
		Tags:       sanitizeStringList(req.Tags),
		Importance: normalizeMemoryImportance(req.Importance),
		Manual:     manual,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if entry.Title == "" && entry.Summary != "" {
		entry.Title = memoryPreview(entry.Summary, 24)
	}
	if err := validateMemoryEntry(entry); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return entry, nil
}

func applyMemoryUpdate(entry *InteractiveMemoryEntry, req InteractiveMemoryUpdateRequest) {
	if req.Title != nil {
		entry.Title = trimMemoryText(*req.Title)
	}
	if req.Summary != nil {
		entry.Summary = trimMemoryText(*req.Summary)
	}
	if req.Content != nil {
		entry.Content = trimMemoryText(*req.Content)
	}
	if req.People != nil {
		entry.People = sanitizeStringList(req.People)
	}
	if req.Places != nil {
		entry.Places = sanitizeStringList(req.Places)
	}
	if req.Tags != nil {
		entry.Tags = sanitizeStringList(req.Tags)
	}
	if req.Importance != nil {
		entry.Importance = normalizeMemoryImportance(*req.Importance)
	}
}

func validateMemoryEntry(entry InteractiveMemoryEntry) error {
	if strings.TrimSpace(entry.BranchID) == "" {
		return fmt.Errorf("记忆缺少分支")
	}
	if strings.TrimSpace(entry.Title) == "" {
		return fmt.Errorf("记忆标题不能为空")
	}
	if strings.TrimSpace(entry.Summary) == "" && strings.TrimSpace(entry.Content) == "" {
		return fmt.Errorf("记忆摘要或正文至少需要一项")
	}
	return nil
}

func filterMemoryEntries(entries []InteractiveMemoryEntry, branchID string, includeArchived bool) []InteractiveMemoryEntry {
	out := make([]InteractiveMemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.BranchID != branchID {
			continue
		}
		if entry.Archived && !includeArchived {
			continue
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt == out[j].UpdatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

func normalizeMemoryBook(book interactiveMemoryBook) interactiveMemoryBook {
	if book.V <= 0 {
		book.V = 1
	}
	book.Settings = normalizeStoryMemorySettings(book.Settings, book.V)
	if len(book.Structures) == 0 {
		book.Structures = defaultStoryMemoryStructures()
	} else {
		book.Structures = refreshBuiltInStoryMemoryStructures(book.Structures)
	}
	if len(book.Records) == 0 && len(book.Entries) > 0 {
		book.Records = migrateInteractiveEntriesToStoryMemoryRecords(book.Entries)
	}
	for i := range book.Structures {
		book.Structures[i] = normalizeStoryMemoryStructureFromStored(book.Structures[i])
	}
	sortStoryMemoryStructures(book.Structures)
	if book.Records == nil {
		book.Records = []StoryMemoryRecord{}
	}
	return book
}

func normalizeStoryMemorySettings(settings StoryMemorySettings, version int) StoryMemorySettings {
	if settings.AutoIntervalTurns <= 0 {
		settings.AutoIntervalTurns = defaultStoryMemoryInterval
	}
	if version < 2 && !settings.Enabled {
		settings.Enabled = true
	}
	return settings
}

func normalizeStoryMemoryInterval(value int) int {
	if value <= 0 {
		return defaultStoryMemoryInterval
	}
	if value > 50 {
		return 50
	}
	return value
}

func defaultStoryMemoryStructures() []StoryMemoryStructure {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	structures := []StoryMemoryStructure{
		defaultStoryMemoryStructure("current_state", "当前状态", "记录当前剧情线的全局时间、地点和场景状态。此表有且仅有一行。", "每轮整理必须更新为当前回合结束后的状态；时间和天数必须自洽，不得按现实消息轮次盲目累加。", "singleton", "", true, 10, []StoryMemoryField{
			defaultStoryMemoryField("story_start_date", "故事开局日期", "当前故事线的剧内开局日期。", "格式尽量使用 YYYY-MM-DD；初始化后除非用户明确重置开局时间，否则不要改动。", true, 10),
			defaultStoryMemoryField("location", "当前详细地点", "主角当前所在的具体场景名称。", "填写具体场景名，不要只写宽泛区域。", true, 20),
			defaultStoryMemoryField("previous_time", "上轮场景时间", "上一轮交互结束时的剧内时间。", "格式尽量与当前时间一致；首轮没有上一轮时填本轮开始前的合理时间。", true, 30),
			defaultStoryMemoryField("elapsed_time", "经过的时间", "当前时间相对上轮场景时间经过了多久。", "用自然语言描述时间跨度，例如“几分钟”“半个时辰”“两天”。", true, 40),
			defaultStoryMemoryField("time", "当前时间", "当前回合结束后的剧内时间。", "必须填写明确日期和时间；若正文未给出，需根据世界观和场景推进推定。", true, 50),
			defaultStoryMemoryField("current_day", "当前天数", "从故事开局日期开始计算的剧内天数。", "开局日期当天为第 1 天；只有剧内日期跨日时才变化。", true, 60),
			defaultStoryMemoryField("event", "当前事件", "当前场景正在承接的核心事件。", "一句话写清本轮结束后仍影响下一轮的当前事件，不写下一步选项。", false, 70),
		}, now),
		defaultStoryMemoryStructure("protagonist", "主角信息", "记录主角的核心身份、能力、资源、关系和经历。此表有且仅有一行。", "只记录主角长期需要承接的信息；技能、物品和人物关系用纯文本子列表维护，避免拆成过多默认表。", "singleton", "", true, 20, []StoryMemoryField{
			defaultStoryMemoryField("name", "人物名称", "主角的名字或稳定称呼。", "使用剧情中最稳定的称呼。", true, 10),
			defaultStoryMemoryField("gender_age", "性别/年龄", "主角的性别和年龄或年龄阶段。", "没有明确年龄时根据设定给出合理阶段或估计年龄。", false, 20),
			defaultStoryMemoryField("appearance", "外貌特征", "主角相对稳定的外貌特征。", "只写客观可观察特征；不要把临时姿态、表情或单轮状态写成外貌。", false, 30),
			defaultStoryMemoryField("identity", "职业/身份", "主角在社会、组织或世界规则中的主要身份。", "填写当前最主要身份，可包含门派、职位、阶层、公开身份或隐藏身份。", false, 40),
			defaultStoryMemoryField("current_condition", "当前近况", "主角当前身体、情绪或压力状态。", "写一口话的具体近况；正常时填“一切如常”。", false, 50),
			defaultStoryMemoryField("location", "所在地点", "主角当前所在地点。", "应与当前状态表的当前详细地点保持一致或更具体。", false, 60),
			defaultStoryMemoryField("abilities", "基础属性/特有能力", "主角能力、属性、境界或特殊能力。", "用分号分隔多项；只记录已经设定或剧情证实的能力，不临场编造。", false, 70),
			defaultStoryMemoryField("skills", "技能列表", "主角掌握的技能。", "按“技能名称｜类型｜等级/阶段｜效果”维护多项；无技能时留空。", false, 80),
			defaultStoryMemoryField("items", "重要物品/资源", "主角持有的重要物品、装备、资源或线索。", "按“名称｜数量/规模｜用途/意义｜状态”维护多项；只记录会影响后续剧情的内容。", false, 90),
			defaultStoryMemoryField("relationships", "与其他人物关系", "主角与重要角色之间的关系和最近互动。", "每行一个人物，格式建议“人物：关系及最近关键互动”；只写已发生或已证实内容。", false, 100),
			defaultStoryMemoryField("experience", "关键经历", "主角背景故事和剧情推进后的关键经历。", "随剧情增量更新，不超过 400 字；超过时压缩，只保留影响后续剧情的事实。", false, 110),
		}, now),
		defaultStoryMemoryStructure("important_character", "重要角色", "记录会影响后续剧情的关键角色。", "每个关键角色一行；只记录会影响后续剧情承接的人物，不记录临时路人。", "keyed", "name", true, 30, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名", "角色姓名或稳定称呼。", "使用角色最稳定的正式姓名或常用称呼。", true, 10),
			defaultStoryMemoryField("gender_age", "性别/年龄", "角色性别和年龄或年龄阶段。", "没有明确年龄时根据设定给出合理阶段或估计年龄。", false, 20),
			defaultStoryMemoryField("brief", "一句话介绍", "角色身份背景的一句话概括。", "不超过 30 字；只写身份背景，不写好坏强弱等主观评价。", false, 30),
			defaultStoryMemoryField("appearance", "外貌特征", "角色相对稳定的外貌特征。", "只写客观可观察特征；临时衣着、姿态和表情放到当前状态。", false, 40),
			defaultStoryMemoryField("identity", "身份", "角色职业、阵营、社会身份或剧情身份。", "只写已设定或已揭示身份；疑似身份写明待确认。", false, 50),
			defaultStoryMemoryField("location", "所在地点", "角色当前或最后确认的地点。", "不知道时填“未知”；离场后写最后确认地点或去向。", false, 60),
			defaultStoryMemoryField("current_status", "当前状态", "角色当前行为、处境、伤势、情绪基调或可互动状态。", "只写当前可承接状态，不写无依据内心独白。", false, 70),
			defaultStoryMemoryField("relationship_to_protagonist", "与主角关系", "该角色与主角的关系和最近关键互动。", "避免只写“朋友/敌人”等标签，要补一句具体依据。", false, 80),
			defaultStoryMemoryField("relationships", "与其他重要角色关系", "该角色与其他重要角色的关系网络。", "每行一个人物，格式建议“人物：关系及最近互动”；只写已接触或已设定关系。", false, 90),
			defaultStoryMemoryField("known_about_protagonist", "对主角已知信息", "该角色已经知道的主角相关情报。", "上限 5 项，保留最影响后续互动的情报。", false, 100),
			defaultStoryMemoryField("unknown_about_protagonist", "对主角未知/误解", "该角色仍想探明或误解的主角相关情报。", "上限 5 项；没有明确误解或未知点时留空。", false, 110),
			defaultStoryMemoryField("important_items", "持有关键物品", "角色持有的重要物品、资源或线索。", "多项用分号分隔；只记录关键物品。", false, 120),
			defaultStoryMemoryField("experience", "关键经历", "角色背景与登场后的关键经历。", "随剧情增量更新，不超过 350 字；超过时压缩，只保留影响后续剧情的事实。", false, 130),
			defaultStoryMemoryField("left_scene", "是否离场", "该角色是否已经离开当前可互动场景。", "只能填写“是”或“否”。", false, 140),
		}, now),
		defaultStoryMemoryStructure("world_context", "世界上下文", "记录地点、势力、组织、阵营、关键场景和世界规则节点。", "本表记录外部结构如何影响剧情，不重复记录角色完整档案；普通地点或一次性背景无需记录。", "keyed", "name", true, 40, []StoryMemoryField{
			defaultStoryMemoryField("name", "节点名称", "地点、势力、组织、规则或关键场景名称。", "使用稳定可复用名称。", true, 10),
			defaultStoryMemoryField("type", "节点类型", "节点类别。", "可填地点、势力、组织、规则、场景、家族、阵营等。", true, 20),
			defaultStoryMemoryField("scope", "所属范围", "上级区域、所属世界、阵营范围或适用范围。", "没有明确范围时填“未知”或留空。", false, 30),
			defaultStoryMemoryField("description", "描述", "该节点的性质、环境、规则或结构说明。", "写对后续剧情有用的事实，不写百科式长篇设定。", false, 40),
			defaultStoryMemoryField("related_characters", "相关角色", "与该节点有关的重要角色。", "多名角色用分号分隔。", false, 50),
			defaultStoryMemoryField("plot_relation", "与主角/主线关系", "该节点如何影响主角、主线或关键关系。", "写推动、阻碍、保护、监视、误导、交易、压迫等具体作用。", false, 60),
			defaultStoryMemoryField("stance", "当前立场", "节点对主角或当前事件的立场。", "没有明确立场时填“未知”。", false, 70),
			defaultStoryMemoryField("status", "当前状态", "节点当前状态。", "记录开放、封锁、覆灭、隐藏、紧张、待调查等状态。", false, 80),
		}, now),
		defaultStoryMemoryStructure("open_threads", "进行中事项", "记录任务、备忘录、承诺、伏笔、计划、未解决误会和待办。", "只维护仍需后续承接的事项；结束、失效或不再参与判断时归档。", "keyed", "title", true, 50, []StoryMemoryField{
			defaultStoryMemoryField("title", "标题", "事项的稳定短标题。", "用可复用短标题，不要每轮改名。", true, 10),
			defaultStoryMemoryField("type", "事项类型", "任务、备忘、承诺、伏笔、计划、误会、纪念日、调查等。", "选择最贴近的一类；不要新增无意义分类。", true, 20),
			defaultStoryMemoryField("related", "相关对象", "相关角色、地点、势力或物品。", "多项用分号分隔。", false, 30),
			defaultStoryMemoryField("detail", "详细内容", "事项来由、关键细节和当前卡点。", "写清楚为什么需要后续承接，避免一句话空泛概括。", false, 40),
			defaultStoryMemoryField("progress", "当前进度/状态", "已完成事项、当前阻碍或当前状态。", "简要描述已发生变化；没有进展时沿用旧值。", false, 50),
			defaultStoryMemoryField("deadline", "时限", "完成、兑现或爆发的时间限制。", "没有明确时限时填“暂无明确时限”。", false, 60),
			defaultStoryMemoryField("stakes", "风险/收益", "事项成功、失败或拖延的后果。", "没有明确风险收益时填“暂无明确风险收益”。", false, 70),
			defaultStoryMemoryField("result", "后续结果", "事项完结或状态变更后的结果。", "未完结时留空；完结时写具体结果，不写“已解决”等空泛收束。", false, 80),
		}, now),
		defaultStoryMemoryStructure("rule_state_summary", "规则与数值状态", "记录规则引擎和数值系统需要长期承接的资源、属性、状态和最近检定。此表有且仅有一行。", "只记录已经由剧情、工具或规则结算确认的状态；数值变化必须能追溯到最近事件或规则检定，不自行臆造。", "singleton", "", true, 60, []StoryMemoryField{
			defaultStoryMemoryField("resources", "资源数值", "生命、体力、灵力、金钱、声望等资源状态。", "按“资源：当前值/上限｜变化原因”维护；未知上限时写当前状态和来源。", false, 10),
			defaultStoryMemoryField("attributes", "属性/境界", "力量、敏捷、修为、经营等级等稳定属性。", "按“属性：数值或阶段｜来源”维护；只写已确认属性。", false, 20),
			defaultStoryMemoryField("conditions", "持续状态", "伤势、中毒、疲劳、增益、诅咒、通缉等会跨回合影响行动的状态。", "每项写清持续条件、影响和解除方式；过期状态应移除或标记已结束。", false, 30),
			defaultStoryMemoryField("relationship_scores", "关系数值", "好感、信任、敌意、债务等可数值化关系。", "按“角色/势力：指标=值｜变化原因”维护；没有数值系统时可留空。", false, 40),
			defaultStoryMemoryField("flags", "规则标记", "会影响后续检定或分支的布尔/枚举标记。", "按“标记：值｜来源”维护，例如“已暴露身份：否”。", false, 50),
			defaultStoryMemoryField("last_rule_checks", "最近规则检定", "最近关键规则检定及结果。", "记录 3-5 个影响后续叙事的检定，格式建议“回合/检定：成功等级｜代价｜影响”。", false, 60),
		}, now),
		defaultStoryMemoryStructure("relationship_state", "关系状态", "记录普通互动、恋爱、误会、敌意、债务和同盟等可推进关系。", "每次整理只更新已经被剧情证实的关系变化；不要替代重要角色表的完整人物档案。", "keyed", "name", true, 70, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名/对象", "角色、势力或关系对象名称。", "使用重要角色、势力或稳定称呼。", true, 10),
			defaultStoryMemoryField("relationship_type", "关系类型", "同盟、师徒、竞争、恋爱、误会、债务、敌对等。", "选择最影响后续互动的一类或两类。", true, 20),
			defaultStoryMemoryField("affection", "好感/亲近", "对象对主角的亲近、好感或抗拒。", "可写数值或阶段；必须带最近依据。", false, 30),
			defaultStoryMemoryField("trust", "信任/戒备", "对象对主角的信任、戒备或怀疑。", "写清触发原因和当前风险。", false, 40),
			defaultStoryMemoryField("tension", "张力/冲突", "暧昧、敌意、竞争、亏欠或压力。", "记录会推动下一次互动的张力，不写泛泛情绪。", false, 50),
			defaultStoryMemoryField("misunderstanding", "误会/秘密", "仍未消解的误会、隐瞒、秘密或错认。", "没有时留空；有时写清谁误会了什么。", false, 60),
			defaultStoryMemoryField("last_interaction", "最近关键互动", "最近一次改变关系状态的互动。", "一句话记录事件和结果。", false, 70),
			defaultStoryMemoryField("next_hook", "后续关系钩子", "下一次关系推进可承接的入口。", "只记录已经被当前剧情合理铺垫的入口。", false, 80),
		}, now),
		defaultStoryMemoryStructure("foreshadowing_resolved", "伏笔与回收", "记录已埋下、推进中、已回收或已失效的伏笔。", "伏笔必须有来源事件、可见线索和回收条件；回收后写具体回收结果，避免只写“已回收”。", "keyed", "title", true, 80, []StoryMemoryField{
			defaultStoryMemoryField("title", "伏笔标题", "伏笔的稳定短标题。", "用可复用短标题，不要每轮改名。", true, 10),
			defaultStoryMemoryField("status", "状态", "seeded、developing、ready、paid_off、void 等状态。", "根据剧情事实选择；不确定时保持上一状态。", true, 20),
			defaultStoryMemoryField("seeded_at", "埋设来源", "伏笔首次出现的事件、回合或场景。", "写清来源，方便后续回查。", false, 30),
			defaultStoryMemoryField("clues", "已露线索", "用户或角色已经看见的线索。", "列出 1-5 条关键线索；不要加入未揭示真相。", false, 40),
			defaultStoryMemoryField("payoff_condition", "回收条件", "触发回收需要满足的条件。", "写成可判断条件，例如“主角交出残卷且长老在场”。", false, 50),
			defaultStoryMemoryField("payoff_result", "回收结果", "伏笔回收、反转或失效后的具体结果。", "未回收时留空；已回收时写对主线、关系或世界状态的影响。", false, 60),
			defaultStoryMemoryField("related_events", "关联事件", "相关事件、角色、物品或长期弧线。", "多项用分号分隔。", false, 70),
		}, now),
		defaultStoryMemoryStructure("long_term_arc_progress", "长期弧线进度", "记录逆袭、复仇、种田、经营、修炼、恋爱等长期情节的目标、阶段、压力和回报。", "只维护仍在推进或需要后续承接的长期弧线；阶段变化必须由剧情或规则结果支撑。", "keyed", "arc_name", true, 90, []StoryMemoryField{
			defaultStoryMemoryField("arc_name", "弧线名称", "长期情节的稳定名称。", "例如“青云宗逆袭线”“北境种田线”。", true, 10),
			defaultStoryMemoryField("arc_type", "弧线类型", "逆袭、复仇、种田、经营、修炼、恋爱、学院比拼等。", "选择主要类型，可用逗号补充副类型。", true, 20),
			defaultStoryMemoryField("goal", "长期目标", "此弧线当前阶段的长期目标。", "写成可推进目标，不写抽象愿望。", false, 30),
			defaultStoryMemoryField("current_phase", "当前阶段", "铺垫、升级、挫败、反转、收获、收束等阶段。", "用一个短语描述当前节奏阶段。", false, 40),
			defaultStoryMemoryField("pressure", "当前压力/危机", "推进此弧线的外部压力、倒计时或危机。", "写清压力来源和失败后果。", false, 50),
			defaultStoryMemoryField("milestones", "已达成节点", "此弧线已完成的重要节点。", "按时间顺序压缩记录，只保留影响后续的节点。", false, 60),
			defaultStoryMemoryField("setbacks", "失败/代价", "此弧线中已经付出的代价、失败或错失机会。", "失败后果要保留，不要下一轮自然消失。", false, 70),
			defaultStoryMemoryField("next_pressure", "下一压力点", "后续可触发的压力、比拼、危机或收益窗口。", "只记录已铺垫且合理的下一压力点。", false, 80),
			defaultStoryMemoryField("terminal_risk", "终局风险", "可能导致主线失败、死亡或弧线中断的风险。", "没有明确风险时留空；有风险时写触发条件。", false, 90),
		}, now),
		defaultStoryMemoryStructure("plot_summary", "剧情纪要", "轮次日志，每轮或每批整理追加一条新记录。", "纪要以第三人称客观记录正文明确发生的事实，不生成下一步行动选项，不加入推测、情绪化语言或主观判断。", "append", "", true, 100, []StoryMemoryField{
			defaultStoryMemoryField("code_index", "编码索引", "本条纪要的唯一顺序索引。", "格式建议 AM0001 起递增；无法确认时根据已有纪要顺序推定。", true, 10),
			defaultStoryMemoryField("time_span", "时间跨度", "本轮事件发生的精确时间范围。", "格式尽量与当前状态表一致。", true, 20),
			defaultStoryMemoryField("place", "地点", "本轮事件发生的地点。", "按从大到小的层级描述地点。", true, 30),
			defaultStoryMemoryField("summary", "概览", "30 字以内的一句话概括。", "不超过 30 字，客观概括本轮事实。", false, 40),
			defaultStoryMemoryField("event", "详细纪要", "以第三人称客观记录本轮事件。", "必须基于正文明确发生的事实；记录关键因果、对话、移动、物品交互和状态变化；不少于 300 字；结尾禁止总结或升华。", true, 50),
			defaultStoryMemoryField("key_dialogue", "重要对话", "造成事实重点或后续影响的重要原文对话。", "摘录 2-4 句并标明说话者；没有关键对话时留空。", false, 60),
			defaultStoryMemoryField("current_day", "当前天数", "本轮结束时对应的剧内天数。", "必须与当前状态表的当前天数一致。", true, 70),
		}, now),
		defaultStoryMemoryStructure("romance_profile", "恋爱关系档案", "记录恋爱对象或潜在恋爱对象的关系阶段和情感变化。", "默认关闭；用户启用后才参与自动整理。只记录已发生或已表现出的关系变化，不替代重要角色表。", "keyed", "name", false, 110, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名", "恋爱对象或潜在恋爱对象姓名。", "必须对应重要角色表中的角色。", true, 10),
			defaultStoryMemoryField("relationship_stage", "关系阶段", "该角色与主角的关系阶段。", "用具体短语描述当前阶段，并写出依据。", false, 20),
			defaultStoryMemoryField("affection", "好感/亲近度", "该角色对主角的亲近、好感或抗拒状态。", "用自然语言描述，不强制数值。", false, 30),
			defaultStoryMemoryField("trust", "信任度", "该角色对主角的信任状态。", "用自然语言描述信任依据和风险。", false, 40),
			defaultStoryMemoryField("attitude", "当前态度", "该角色当前面对主角的态度。", "只基于正文表现和已知设定，不主观脑补。", false, 50),
			defaultStoryMemoryField("key_experience", "关键经历", "影响关系发展的关键经历。", "不超过 300 字；只保留影响后续互动的节点。", false, 60),
		}, now),
		defaultStoryMemoryStructure("romance_diary", "恋爱日记", "记录特定角色视角下值得长期保留的情感节点。", "默认关闭；只记录明显改变关系、误会、期待、后悔、动摇或无法说出口想法的节点，不记录普通互动流水账。", "append", "", false, 120, []StoryMemoryField{
			defaultStoryMemoryField("writer", "写作角色", "日记视角角色。", "必须是已建档的重要角色或恋爱档案角色。", true, 10),
			defaultStoryMemoryField("related", "关联角色", "该情感节点关联的角色。", "通常为主角，也可包含关键第三人。", false, 20),
			defaultStoryMemoryField("content", "日记内容", "该角色视角下的情感节点。", "100-200 字；聚焦内心变化、误解、期待、动摇或确认。", true, 30),
			defaultStoryMemoryField("time", "发生时间", "该节点发生的剧内时间。", "尽量与当前状态表时间一致。", false, 40),
			defaultStoryMemoryField("event_type", "事件类型", "情感节点类型。", "可填初次相遇、日常互动、感情升温、冲突矛盾、和解修复、亲密接触等。", false, 50),
			defaultStoryMemoryField("impact", "影响判断", "该节点对后续关系的影响。", "写具体影响，不写空泛总结。", false, 60),
		}, now),
		defaultStoryMemoryStructure("mature_relationship_profile", "成人向关系档案", "记录用户主动启用后的成人向关系扩展信息。", "默认关闭；作为可配置扩展结构存在，不照搬外部模板的私有字段。启用后只记录用户作品设定中明确允许且后续需要承接的内容。", "keyed", "name", false, 130, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名", "角色姓名。", "必须对应重要角色表中的角色。", true, 10),
			defaultStoryMemoryField("boundary", "边界与偏好", "角色在成人向互动中的边界、偏好或禁忌。", "只记录已设定或已明确表达的内容。", false, 20),
			defaultStoryMemoryField("relationship_context", "关系语境", "成人向内容与主角关系、权力结构或剧情状态的关联。", "必须服务后续剧情承接，不写一次性场景细节。", false, 30),
			defaultStoryMemoryField("continuity_notes", "连续性备注", "需要长期保持一致的成人向连续性信息。", "压缩记录，避免露骨流水账。", false, 40),
		}, now),
	}
	for i := range structures {
		if isDerivedStoryMemoryStructureID(structures[i].ID) {
			structures[i].ReadOnly = true
			structures[i].Derived = true
		}
	}
	return structures
}

func isDerivedStoryMemoryStructureID(id string) bool {
	switch sanitizeMemoryID(id) {
	case "current_state", "rule_state_summary":
		return true
	default:
		return false
	}
}

func defaultStoryMemoryStructure(id, name, description, generationInstruction, mode, keyFieldID string, enabled bool, order int, fields []StoryMemoryField, now string) StoryMemoryStructure {
	return StoryMemoryStructure{ID: id, Name: name, Description: description, GenerationInstruction: generationInstruction, Mode: mode, KeyFieldID: keyFieldID, Fields: fields, Enabled: boolPtr(enabled), Order: order, BuiltIn: true, CreatedAt: now, UpdatedAt: now}
}

func defaultStoryMemoryField(id, name, description, generationInstruction string, required bool, order int) StoryMemoryField {
	return StoryMemoryField{ID: id, Name: name, Description: description, GenerationInstruction: generationInstruction, Required: required, Order: order}
}

func boolPtr(value bool) *bool {
	return &value
}

func refreshBuiltInStoryMemoryStructures(structures []StoryMemoryStructure) []StoryMemoryStructure {
	defaults := defaultStoryMemoryStructures()
	storedBuiltInByID := make(map[string]StoryMemoryStructure, len(structures))
	custom := make([]StoryMemoryStructure, 0, len(structures))
	for _, structure := range structures {
		if structure.BuiltIn {
			storedBuiltInByID[structure.ID] = structure
			continue
		}
		custom = append(custom, structure)
	}
	out := make([]StoryMemoryStructure, 0, len(defaults)+len(custom))
	for _, preset := range defaults {
		next := preset
		if stored, ok := storedBuiltInByID[preset.ID]; ok {
			next.CreatedAt = firstMemoryText(stored.CreatedAt, preset.CreatedAt)
			next.UpdatedAt = firstMemoryText(stored.UpdatedAt, preset.UpdatedAt)
			if stored.Enabled != nil {
				next.Enabled = stored.Enabled
			}
			next.Fields = mergeBuiltInStoryMemoryFields(preset.Fields, stored.Fields)
		}
		out = append(out, next)
	}
	out = append(out, custom...)
	return out
}

func mergeBuiltInStoryMemoryFields(defaults, stored []StoryMemoryField) []StoryMemoryField {
	storedByID := make(map[string]StoryMemoryField, len(stored))
	for _, field := range stored {
		storedByID[field.ID] = field
	}
	out := make([]StoryMemoryField, 0, len(defaults))
	for _, field := range defaults {
		if storedField, ok := storedByID[field.ID]; ok && storedField.Enabled != nil {
			field.Enabled = storedField.Enabled
		}
		out = append(out, field)
	}
	return out
}

func normalizeStoryMemoryStructure(req StoryMemoryStructureRequest, now string) StoryMemoryStructure {
	structure := StoryMemoryStructure{
		ID:                    sanitizeMemoryID(req.ID),
		Name:                  trimMemoryText(req.Name),
		Description:           trimMemoryText(req.Description),
		GenerationInstruction: trimMemoryText(req.GenerationInstruction),
		Mode:                  strings.TrimSpace(req.Mode),
		KeyFieldID:            sanitizeMemoryID(req.KeyFieldID),
		Enabled:               req.Enabled,
		Order:                 req.Order,
		ReadOnly:              req.ReadOnly,
		Derived:               req.Derived,
		Fields:                normalizeStoryMemoryFields(req.Fields),
		UpdatedAt:             now,
	}
	if structure.Mode == "" {
		structure.Mode = "append"
	}
	if isDerivedStoryMemoryStructureID(structure.ID) {
		structure.ReadOnly = true
		structure.Derived = true
	}
	return structure
}

func normalizeStoryMemoryStructureFromStored(structure StoryMemoryStructure) StoryMemoryStructure {
	structure.ID = sanitizeMemoryID(structure.ID)
	structure.Name = trimMemoryText(structure.Name)
	structure.Description = trimMemoryText(structure.Description)
	structure.GenerationInstruction = trimMemoryText(structure.GenerationInstruction)
	structure.Mode = strings.TrimSpace(structure.Mode)
	if structure.Mode == "" {
		structure.Mode = "append"
	}
	structure.KeyFieldID = sanitizeMemoryID(structure.KeyFieldID)
	if isDerivedStoryMemoryStructureID(structure.ID) {
		structure.ReadOnly = true
		structure.Derived = true
	}
	structure.Fields = normalizeStoryMemoryFields(structure.Fields)
	return structure
}

func normalizeStoryMemoryFields(fields []StoryMemoryField) []StoryMemoryField {
	out := make([]StoryMemoryField, 0, len(fields))
	for i, field := range fields {
		field.ID = sanitizeMemoryID(field.ID)
		if field.ID == "" {
			field.ID = fmt.Sprintf("field_%d", i+1)
		}
		field.Name = trimMemoryText(field.Name)
		if field.Name == "" {
			field.Name = field.ID
		}
		field.Description = trimMemoryText(field.Description)
		field.GenerationInstruction = trimMemoryText(field.GenerationInstruction)
		if field.Order == 0 {
			field.Order = (i + 1) * 10
		}
		out = append(out, field)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order == out[j].Order {
			return out[i].ID < out[j].ID
		}
		return out[i].Order < out[j].Order
	})
	return out
}

func validateStoryMemoryStructure(structure StoryMemoryStructure) error {
	if strings.TrimSpace(structure.ID) == "" {
		return fmt.Errorf("故事记忆结构 ID 不能为空")
	}
	if strings.TrimSpace(structure.Name) == "" {
		return fmt.Errorf("故事记忆结构名称不能为空")
	}
	switch structure.Mode {
	case "singleton", "keyed", "append":
	default:
		return fmt.Errorf("故事记忆结构模式无效: %s", structure.Mode)
	}
	if len(structure.Fields) == 0 {
		return fmt.Errorf("故事记忆结构至少需要一个字段")
	}
	if structure.Mode == "keyed" && structure.KeyFieldID == "" {
		return fmt.Errorf("keyed 结构必须配置 key_field_id")
	}
	return nil
}

func sortStoryMemoryStructures(structures []StoryMemoryStructure) {
	sort.SliceStable(structures, func(i, j int) bool {
		if structures[i].Order == structures[j].Order {
			return structures[i].ID < structures[j].ID
		}
		return structures[i].Order < structures[j].Order
	})
}

func storyMemoryStructureEnabled(structure StoryMemoryStructure) bool {
	return structure.Enabled == nil || *structure.Enabled
}

func enabledStoryMemoryStructures(structures []StoryMemoryStructure) []StoryMemoryStructure {
	out := make([]StoryMemoryStructure, 0, len(structures))
	for _, structure := range structures {
		if storyMemoryStructureEnabled(structure) {
			out = append(out, structure)
		}
	}
	return out
}

func storyMemoryFieldEnabled(field StoryMemoryField) bool {
	return field.Enabled == nil || *field.Enabled
}

func migrateInteractiveEntriesToStoryMemoryRecords(entries []InteractiveMemoryEntry) []StoryMemoryRecord {
	records := make([]StoryMemoryRecord, 0, len(entries))
	for _, entry := range entries {
		values := map[string]string{
			"event": firstMemoryText(entry.Summary, entry.Content, entry.Title),
		}
		if strings.TrimSpace(entry.Content) != "" {
			values["detail"] = trimMemoryText(entry.Content)
		}
		if len(entry.Places) > 0 {
			values["place"] = strings.Join(entry.Places, "，")
		}
		record := StoryMemoryRecord{
			ID:           firstMemoryText(entry.ID, newID("mem")),
			StructureID:  "plot_summary",
			BranchID:     entry.BranchID,
			TurnID:       entry.TurnID,
			AnchorTurnID: entry.TurnID,
			Key:          entry.Title,
			Values:       values,
			Archived:     entry.Archived,
			Manual:       entry.Manual,
			Source:       "legacy",
			CreatedAt:    entry.CreatedAt,
			UpdatedAt:    entry.UpdatedAt,
		}
		if record.CreatedAt == "" {
			record.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if record.UpdatedAt == "" {
			record.UpdatedAt = record.CreatedAt
		}
		records = append(records, record)
	}
	return records
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

func storyMemoryStructureByID(structures []StoryMemoryStructure, id string) StoryMemoryStructure {
	for _, structure := range structures {
		if structure.ID == id {
			return structure
		}
	}
	return StoryMemoryStructure{ID: id, Name: id, Mode: "append"}
}

func interactiveMemoryCreateToStoryRecord(req InteractiveMemoryCreateRequest) StoryMemoryRecordRequest {
	values := map[string]string{
		"event": firstMemoryText(req.Summary, req.Content, req.Title),
	}
	if strings.TrimSpace(req.Content) != "" {
		values["detail"] = trimMemoryText(req.Content)
	}
	if len(req.Places) > 0 {
		values["place"] = strings.Join(sanitizeStringList(req.Places), "，")
	}
	return StoryMemoryRecordRequest{
		BranchID:    req.BranchID,
		StructureID: "plot_summary",
		TurnID:      req.TurnID,
		Key:         trimMemoryText(req.Title),
		Values:      values,
		Manual:      true,
	}
}

func saveStoryMemoryRecordLocked(book *interactiveMemoryBook, branchID, anchorTurnID string, req StoryMemoryRecordRequest, manual bool, pathSet map[string]bool) (StoryMemoryRecord, error) {
	req.StructureID = sanitizeMemoryID(req.StructureID)
	structure := storyMemoryStructureByID(book.Structures, req.StructureID)
	if structure.ID == "" {
		return StoryMemoryRecord{}, fmt.Errorf("故事记忆结构不存在: %s", req.StructureID)
	}
	if manual && structure.ReadOnly {
		return StoryMemoryRecord{}, fmt.Errorf("故事记忆结构为只读派生表，不能手动编辑: %s", structure.ID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := StoryMemoryRecord{
		ID:           sanitizeMemoryID(req.ID),
		StructureID:  req.StructureID,
		BranchID:     branchID,
		TurnID:       strings.TrimSpace(req.TurnID),
		AnchorTurnID: firstMemoryText(req.TurnID, anchorTurnID),
		Key:          trimMemoryText(req.Key),
		Values:       sanitizeStoryMemoryValues(req.Values),
		Manual:       manual || req.Manual,
		Source:       "manual",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if record.Key == "" && structure.KeyFieldID != "" {
		record.Key = record.Values[structure.KeyFieldID]
	}
	if record.ID != "" {
		for i := range book.Records {
			if book.Records[i].ID != record.ID {
				continue
			}
			if book.Records[i].BranchID != branchID && recordVisibleOnBranch(book.Records[i], branchID, pathSet) {
				copy := book.Records[i]
				copy.ID = newID("mem")
				copy.BranchID = branchID
				copy.TurnID = ""
				copy.AnchorTurnID = ""
				copy.InheritedFrom = book.Records[i].ID
				copy.Values = record.Values
				copy.Key = record.Key
				copy.Manual = record.Manual
				copy.Source = record.Source
				copy.CreatedAt = now
				copy.UpdatedAt = now
				book.Records = append(book.Records, copy)
				return copy, validateStoryMemoryRecord(copy, structure)
			}
			record.CreatedAt = firstMemoryText(book.Records[i].CreatedAt, now)
			record.UpdatedAt = now
			record.Archived = book.Records[i].Archived
			book.Records[i] = record
			return record, validateStoryMemoryRecord(record, structure)
		}
	}
	record.ID = newID("mem")
	if structure.Mode != "append" {
		if existing, ok := findStoryMemoryUpsertRecord(book.Records, structure, branchID, record.Key, pathSet); ok {
			record.ID = existing.ID
			req.ID = existing.ID
			return saveStoryMemoryRecordLocked(book, branchID, anchorTurnID, req, manual, pathSet)
		}
	}
	if err := validateStoryMemoryRecord(record, structure); err != nil {
		return StoryMemoryRecord{}, err
	}
	book.Records = append(book.Records, record)
	return record, nil
}

func saveStoryMemoryRecordWithStructuresLocked(book *interactiveMemoryBook, structures []StoryMemoryStructure, branchID, anchorTurnID string, req StoryMemoryRecordRequest, manual bool, pathSet map[string]bool) (StoryMemoryRecord, error) {
	originalStructures := book.Structures
	book.Structures = structures
	defer func() {
		book.Structures = originalStructures
	}()
	return saveStoryMemoryRecordLocked(book, branchID, anchorTurnID, req, manual, pathSet)
}

func setStoryMemoryRecordArchivedLocked(book *interactiveMemoryBook, branchID, anchorTurnID, recordID string, archived bool, pathSet map[string]bool) (StoryMemoryRecord, error) {
	recordID = sanitizeMemoryID(recordID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range book.Records {
		if book.Records[i].ID != recordID {
			continue
		}
		if book.Records[i].BranchID != branchID && recordVisibleOnBranch(book.Records[i], branchID, pathSet) {
			copy := book.Records[i]
			copy.ID = newID("mem")
			copy.BranchID = branchID
			copy.TurnID = ""
			copy.AnchorTurnID = ""
			copy.Archived = archived
			copy.InheritedFrom = book.Records[i].ID
			copy.CreatedAt = now
			copy.UpdatedAt = now
			book.Records = append(book.Records, copy)
			return copy, nil
		}
		book.Records[i].Archived = archived
		book.Records[i].UpdatedAt = now
		return book.Records[i], nil
	}
	return StoryMemoryRecord{}, fmt.Errorf("故事记忆不存在: %s", recordID)
}

func applyStoryMemoryPatchLocked(book *interactiveMemoryBook, branchID, anchorTurnID string, patch StoryMemoryPatch, pathSet map[string]bool) (StoryMemoryRecord, error) {
	op := strings.TrimSpace(patch.Op)
	if op == "" {
		op = "upsert"
	}
	switch op {
	case "archive":
		archived := true
		if patch.Archived != nil {
			archived = *patch.Archived
		}
		return setStoryMemoryRecordArchivedLocked(book, branchID, anchorTurnID, patch.RecordID, archived, pathSet)
	case "restore":
		return setStoryMemoryRecordArchivedLocked(book, branchID, anchorTurnID, patch.RecordID, false, pathSet)
	case "upsert", "append", "set":
		record, err := saveStoryMemoryRecordLocked(book, branchID, anchorTurnID, StoryMemoryRecordRequest{
			ID:          patch.RecordID,
			StructureID: patch.StructureID,
			Key:         patch.Key,
			Values:      patch.Values,
		}, false, pathSet)
		if record.ID != "" {
			for i := range book.Records {
				if book.Records[i].ID == record.ID {
					book.Records[i].Source = "agent"
					record.Source = "agent"
					break
				}
			}
		}
		return record, err
	default:
		return StoryMemoryRecord{}, fmt.Errorf("不支持的故事记忆操作: %s", op)
	}
}

func applyStoryMemoryPatchWithStructuresLocked(book *interactiveMemoryBook, structures []StoryMemoryStructure, branchID, anchorTurnID string, patch StoryMemoryPatch, pathSet map[string]bool) (StoryMemoryRecord, error) {
	originalStructures := book.Structures
	book.Structures = structures
	defer func() {
		book.Structures = originalStructures
	}()
	return applyStoryMemoryPatchLocked(book, branchID, anchorTurnID, patch, pathSet)
}

func findStoryMemoryUpsertRecord(records []StoryMemoryRecord, structure StoryMemoryStructure, branchID, key string, pathSet map[string]bool) (StoryMemoryRecord, bool) {
	visible := visibleStoryMemoryRecords(records, branchID, pathSet, false)
	for _, record := range visible {
		if record.StructureID != structure.ID {
			continue
		}
		if structure.Mode == "singleton" {
			return record, true
		}
		if structure.Mode == "keyed" && strings.TrimSpace(record.Key) == strings.TrimSpace(key) {
			return record, true
		}
	}
	return StoryMemoryRecord{}, false
}

func validateStoryMemoryRecord(record StoryMemoryRecord, structure StoryMemoryStructure) error {
	if record.StructureID == "" {
		return fmt.Errorf("故事记忆缺少结构")
	}
	if record.BranchID == "" {
		return fmt.Errorf("故事记忆缺少分支")
	}
	if len(record.Values) == 0 {
		return fmt.Errorf("故事记忆内容不能为空")
	}
	if structure.Mode == "keyed" && strings.TrimSpace(record.Key) == "" {
		return fmt.Errorf("keyed 故事记忆缺少 key")
	}
	return nil
}

func applyInteractiveMemoryUpdateToRecord(record *StoryMemoryRecord, req InteractiveMemoryUpdateRequest) {
	if record.Values == nil {
		record.Values = map[string]string{}
	}
	if req.Title != nil {
		record.Key = trimMemoryText(*req.Title)
	}
	if req.Summary != nil {
		record.Values["event"] = trimMemoryText(*req.Summary)
	}
	if req.Content != nil && strings.TrimSpace(*req.Content) != "" {
		record.Values["detail"] = trimMemoryText(*req.Content)
	}
	if req.Places != nil {
		record.Values["place"] = strings.Join(sanitizeStringList(req.Places), "，")
	}
}

func storyMemoryAutoDecisionLocked(book interactiveMemoryBook, lines []StoryEventRecord, branchID, headID string) (bool, int) {
	if !book.Settings.Enabled {
		return false, 0
	}
	interval := normalizeStoryMemoryInterval(book.Settings.AutoIntervalTurns)
	turns := turnPath(lines, headID)
	lastIndex := -1
	pathSet := eventPathSet(headID, lines)
	for _, record := range visibleStoryMemoryRecords(book.Records, branchID, pathSet, false) {
		if record.Source != "agent" {
			continue
		}
		anchor := firstMemoryText(record.AnchorTurnID, record.TurnID)
		for i, turn := range turns {
			if turn.ID == anchor && i > lastIndex {
				lastIndex = i
			}
		}
	}
	delta := len(turns) - lastIndex - 1
	if delta >= interval {
		return true, interval
	}
	return false, interval - delta
}

func formatStoryMemoryContextSummary(structures []StoryMemoryStructure, records []StoryMemoryRecord, limit int) string {
	if limit <= 0 || limit > maxMemoryTextBytes {
		limit = maxMemoryTextBytes
	}
	return formatStoryMemoryContext(structures, records, limit, maxMemoryListItems, true)
}

func formatStoryMemoryCompactionContext(structures []StoryMemoryStructure, records []StoryMemoryRecord) string {
	return formatStoryMemoryContext(structures, records, 0, 0, false)
}

func formatStoryMemoryContext(structures []StoryMemoryStructure, records []StoryMemoryRecord, limit, itemLimit int, bounded bool) string {
	if len(enabledStoryMemoryStructures(structures)) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("来源: interactive/memory/story-{story_id}.json 的当前分支可见故事记忆\n")
	if bounded {
		sb.WriteString("边界: 已按调用方上下文预算裁剪\n")
	} else {
		sb.WriteString("边界: 不截断，用于上下文压缩 Agent\n")
	}
	count := 0
	for _, structure := range structures {
		if !storyMemoryStructureEnabled(structure) {
			continue
		}
		items := make([]StoryMemoryRecord, 0)
		for _, record := range records {
			if record.StructureID == structure.ID {
				items = append(items, record)
			}
		}
		if len(items) == 0 {
			continue
		}
		sb.WriteString("\n## ")
		sb.WriteString(structure.Name)
		sb.WriteString("\n")
		for _, record := range items {
			if bounded && (count >= itemLimit || sb.Len() >= limit) {
				sb.WriteString("\n(后续故事记忆已截断)\n")
				return limitMemoryText(sb.String(), limit)
			}
			if record.Key != "" {
				sb.WriteString("- ")
				sb.WriteString(record.Key)
				sb.WriteString(": ")
			} else {
				sb.WriteString("- ")
			}
			parts := make([]string, 0, len(structure.Fields))
			for _, field := range structure.Fields {
				if !storyMemoryFieldEnabled(field) {
					continue
				}
				if value := strings.TrimSpace(record.Values[field.ID]); value != "" {
					parts = append(parts, field.Name+"="+value)
				}
			}
			if len(parts) == 0 {
				for key, value := range record.Values {
					parts = append(parts, key+"="+value)
				}
			}
			sb.WriteString(strings.Join(parts, "；"))
			sb.WriteString("\n")
			count++
		}
	}
	if bounded {
		return limitMemoryText(sb.String(), limit)
	}
	return strings.TrimSpace(sb.String())
}

func formatStoryMemorySchemaContext(structures []StoryMemoryStructure, limit int) string {
	if limit <= 0 || limit > maxStoryMemorySchemaBytes {
		limit = maxStoryMemorySchemaBytes
	}
	structures = storyMemorySchemaContextOrder(structures)
	if len(structures) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("来源: interactive/memory/story-{story_id}.json 的故事记忆结构定义\n")
	sb.WriteString("边界: 已按调用方上下文预算裁剪\n")
	sb.WriteString("规则: story_memory_patches 只能使用下列 structure_id 和字段 ID；每条 patch 的 values 必须包含目标结构列出的所有字段，且字段值不能为空；keyed 结构必须提供 key，且 key 应等于 key_field_id 对应字段值；生成时必须遵守 structure 和 field 的 generation_instruction。\n")
	for _, structure := range structures {
		if !storyMemoryStructureEnabled(structure) {
			continue
		}
		if sb.Len() >= limit {
			sb.WriteString("\n(后续故事记忆结构已截断)\n")
			return limitMemoryText(sb.String(), limit)
		}
		sb.WriteString("\n## ")
		sb.WriteString(structure.ID)
		if strings.TrimSpace(structure.Name) != "" {
			sb.WriteString("（")
			sb.WriteString(structure.Name)
			sb.WriteString("）")
		}
		sb.WriteString("\n")
		sb.WriteString("- mode: ")
		sb.WriteString(firstMemoryText(structure.Mode, "append"))
		sb.WriteString("\n")
		if strings.TrimSpace(structure.KeyFieldID) != "" {
			sb.WriteString("- key_field_id: ")
			sb.WriteString(structure.KeyFieldID)
			sb.WriteString("\n")
		}
		if strings.TrimSpace(structure.Description) != "" {
			sb.WriteString("- description: ")
			sb.WriteString(structure.Description)
			sb.WriteString("\n")
		}
		if strings.TrimSpace(structure.GenerationInstruction) != "" {
			sb.WriteString("- generation_instruction: ")
			sb.WriteString(structure.GenerationInstruction)
			sb.WriteString("\n")
		}
		sb.WriteString("- fields:\n")
		for _, field := range structure.Fields {
			if !storyMemoryFieldEnabled(field) {
				continue
			}
			if sb.Len() >= limit {
				sb.WriteString("(后续字段已截断)\n")
				return limitMemoryText(sb.String(), limit)
			}
			sb.WriteString("  - ")
			sb.WriteString(field.ID)
			if strings.TrimSpace(field.Name) != "" {
				sb.WriteString("（")
				sb.WriteString(field.Name)
				sb.WriteString("）")
			}
			if field.Required {
				sb.WriteString(" required")
			}
			if strings.TrimSpace(field.Description) != "" {
				sb.WriteString(": ")
				sb.WriteString(field.Description)
			}
			if strings.TrimSpace(field.GenerationInstruction) != "" {
				sb.WriteString("\n    generation_instruction: ")
				sb.WriteString(field.GenerationInstruction)
			}
			sb.WriteString("\n")
		}
	}
	return limitMemoryText(sb.String(), limit)
}

func storyMemorySchemaContextOrder(structures []StoryMemoryStructure) []StoryMemoryStructure {
	out := make([]StoryMemoryStructure, 0, len(structures))
	for _, structure := range structures {
		if !structure.BuiltIn {
			out = append(out, structure)
		}
	}
	for _, structure := range structures {
		if structure.BuiltIn {
			out = append(out, structure)
		}
	}
	return out
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

func sanitizeStoryMemoryValues(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		key = sanitizeMemoryID(key)
		value = trimMemoryText(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	return out
}

func sanitizeMemoryID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func firstMemoryText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func latestMemoryRecall(recalls []InteractiveMemoryRecall, branchID string) *InteractiveMemoryRecall {
	for i := len(recalls) - 1; i >= 0; i-- {
		if recalls[i].BranchID != branchID {
			continue
		}
		recall := recalls[i]
		return &recall
	}
	return nil
}

func latestMemorySyncStatus(lines []StoryEventRecord, branchID, headID string) (string, string) {
	if headID == "" {
		return "", ""
	}
	for _, record := range lines {
		if record.Envelope.ID != headID || record.Envelope.BranchID != branchID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return "failed", err.Error()
		}
		return turn.MemoryStatus, turn.MemoryError
	}
	return "", ""
}

func trimMemoryText(value string) string {
	return strings.TrimSpace(value)
}

func limitMemoryText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if suffix := memoryTruncationSuffix(value); suffix != "" && len(suffix)+1 < limit {
		prefix := limitMemoryTextRaw(value, limit-len(suffix)-1)
		return strings.TrimSpace(prefix) + "\n" + suffix
	}
	return limitMemoryTextRaw(value, limit)
}

func limitMemoryTextRaw(value string, limit int) string {
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	if limit <= 0 {
		return ""
	}
	return strings.TrimSpace(value[:limit])
}

func memoryTruncationSuffix(value string) string {
	for _, suffix := range []string{
		"(后续故事记忆已截断)",
		"(后续故事记忆结构已截断)",
		"(后续字段已截断)",
	} {
		if strings.Contains(value, suffix) {
			return suffix
		}
	}
	return ""
}

func normalizeMemoryImportance(value int) int {
	if value <= 0 {
		return defaultMemoryImportance
	}
	if value > 5 {
		return 5
	}
	return value
}

func sanitizeStringList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		out = append(out, value)
		seen[value] = true
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func memoryPreview(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if limit <= 0 || len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}
