package interactive

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultMemoryImportance    = 3
	defaultStoryMemoryInterval = 6
	maxMemoryTextBytes         = DirectorContextMaxBytes
	maxStoryMemorySchemaBytes  = DirectorContextMaxBytes
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

// MarkInteractiveMemoryRunReady records that Memory Recorder has completed a
// real run, even when it produced no patches. The marker prevents a no-op run
// from being scheduled again on every following turn.
func (s *Store) MarkInteractiveMemoryRunReady(storyID, branchID, turnID string) error {
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
	turnID = strings.TrimSpace(turnID)
	return s.markTurnMemoryReadyLocked(storyID, meta, lines, branchID, turnID, interactiveMemoryRunMarkerPrefix+turnID)
}

// ClaimInteractiveMemoryRun atomically claims one turn for memory recording.
// Automatic maintenance is idempotent; explicit user rebuilds may force a new
// revision of the derived Story Memory index.
func (s *Store) ClaimInteractiveMemoryRun(storyID, branchID, turnID string, force bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return false, err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return false, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return false, fmt.Errorf("记忆运行缺少所属回合")
	}
	updated := false
	for _, record := range lines {
		if record.Envelope.ID != turnID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		status, _ := record.Raw["memory_status"].(string)
		status = strings.TrimSpace(status)
		if !force && (status == "running" || status == "ready") {
			return false, nil
		}
		record.Raw["memory_status"] = "running"
		delete(record.Raw, "memory_error")
		updated = true
		break
	}
	if !updated {
		return false, fmt.Errorf("记忆运行所属回合不存在: %s", turnID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return false, err
	}
	if err := s.touchIndexLocked(storyID, now, 0); err != nil {
		return false, err
	}
	return true, nil
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
