package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/agents/conversationjournal"
	"denova/internal/localfs"
)

const storyIndexSchemaVersion = 1

func (s *Store) storyDir() string {
	return filepath.Join(s.root, "interactive", "story")
}

func (s *Store) indexPath() string {
	return filepath.Join(s.storyDir(), "index.json")
}

func (s *Store) storyPath(storyID string) string {
	return filepath.Join(s.storyDir(), "story-"+storyID+".jsonl")
}

func (s *Store) actorStateSchemaPath(storyID string) string {
	return filepath.Join(s.root, "interactive", "story-schema", "story-"+storyID+"-actor-state.json")
}

func (s *Store) usageDir() string {
	return filepath.Join(s.root, "interactive", "usage")
}

func (s *Store) usagePath(storyID string) string {
	return filepath.Join(s.usageDir(), "usage-"+storyID+".jsonl")
}

func (s *Store) readIndexLocked() (Index, error) {
	data, err := os.ReadFile(s.indexPath())
	if os.IsNotExist(err) {
		return Index{Version: storyIndexSchemaVersion, Stories: []StorySummary{}}, nil
	}
	if err != nil {
		return Index{}, err
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, fmt.Errorf("解析互动故事索引失败: %w", err)
	}
	if index.Version > storyIndexSchemaVersion {
		return Index{}, fmt.Errorf("unsupported interactive story index version: %d", index.Version)
	}
	if index.Version < storyIndexSchemaVersion {
		previousVersion := index.Version
		migrated, migrateErr := s.rebuildStoryIndexLocked(index)
		if migrateErr != nil {
			return Index{}, migrateErr
		}
		if writeErr := s.writeIndexLocked(migrated); writeErr != nil {
			return Index{}, writeErr
		}
		slog.InfoContext(context.Background(), fmt.Sprintf(
			"[interactive-story] migrated story index schema from=%d to=%d stories=%d",
			previousVersion, storyIndexSchemaVersion, len(migrated.Stories),
		))
		return migrated, nil
	}
	if index.Stories == nil {
		index.Stories = []StorySummary{}
	}
	for i := range index.Stories {
		index.Stories[i] = normalizeStorySummary(index.Stories[i])
	}
	return index, nil
}

func (s *Store) writeIndexLocked(index Index) error {
	index.Version = storyIndexSchemaVersion
	if index.Stories == nil {
		index.Stories = []StorySummary{}
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicBytes(s.indexPath(), append(data, '\n'), 0o644)
}

func (s *Store) rebuildStoryIndexLocked(index Index) (Index, error) {
	stories := make([]StorySummary, 0, len(index.Stories))
	for _, indexed := range index.Stories {
		handle, err := s.refreshStoryJournalLocked(indexed.ID, true)
		if err != nil {
			return Index{}, fmt.Errorf("rebuild story index failed story_id=%s: %w", indexed.ID, err)
		}
		stories = append(stories, storySummaryFromProjection(handle.projection))
	}
	index.Version = storyIndexSchemaVersion
	index.Stories = stories
	return index, nil
}

// publishStorySummaryLocked replaces the rebuildable catalog row from the
// canonical journal projection. Callers never increment counts independently.
func (s *Store) publishStorySummaryLocked(storyID string) (StorySummary, error) {
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return StorySummary{}, err
	}
	summary := storySummaryFromProjection(handle.projection)
	index, err := s.readIndexLocked()
	if err != nil {
		return StorySummary{}, err
	}
	for i := range index.Stories {
		if index.Stories[i].ID == storyID {
			index.Stories[i] = summary
			if err := s.writeIndexLocked(index); err != nil {
				return StorySummary{}, err
			}
			return summary, nil
		}
	}
	index.Stories = append(index.Stories, summary)
	if strings.TrimSpace(index.CurrentStoryID) == "" {
		index.CurrentStoryID = storyID
	}
	if err := s.writeIndexLocked(index); err != nil {
		return StorySummary{}, err
	}
	return summary, nil
}

func (s *Store) syncStorySummaryLocked(storyID string) error {
	_, err := s.publishStorySummaryLocked(storyID)
	return err
}

func storySummaryFromProjection(projection *storyJournalProjection) StorySummary {
	if projection == nil {
		return StorySummary{}
	}
	turnCount := 0
	if branch := projection.Branches[projection.Meta.CurrentBranch]; branch != nil {
		turnCount = max(0, branch.Depth)
	}
	meta := projection.Meta
	return normalizeStorySummary(StorySummary{
		ID: meta.StoryID, Title: meta.Title, Origin: meta.Origin,
		StoryTellerID: meta.StoryTellerID, StoryDirectorID: normalizedStoryDirectorID(meta.StoryDirectorID),
		ModuleRefs:       cloneStoryDirectorModuleRefs(meta.ModuleRefs),
		PlanningMode:     meta.PlanningMode,
		ReplyTargetChars: meta.ReplyTargetChars, ChoiceCount: meta.ChoiceCount,
		Opening: meta.Opening, ImageSettings: meta.ImageSettings,
		StateSchemaPolicy: cloneStoryStateSchemaPolicy(meta.StateSchemaPolicy),
		CreatedAt:         meta.CreatedAt, UpdatedAt: meta.UpdatedAt,
		Branches: len(meta.Branches), Events: projection.EventCount, TurnCount: turnCount,
	})
}

func (s *Store) readStoryLocked(storyID string) (StoryMeta, []StoryEventRecord, error) {
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	defer release()
	meta, lines, err := s.readStoryJournalWithRepairLocked(storyID, true)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	if err := s.freezeLegacyActorStateSchemaLocked(storyID, &meta, lines); err != nil {
		return StoryMeta{}, nil, err
	}
	// A legacy sidecar may carry a revision that was not available while the
	// JSONL metadata was normalized. Keep the fixed status aligned with the
	// actual frozen schema without changing the schema itself.
	normalizeFixedStoryStateSchemaInitialization(&meta)
	return meta, lines, nil
}

// readStoryJournalLocked is the read-only half of story loading. Receipt
// reconciliation uses it so opening an unfinished Agent operation cannot
// trigger legacy migrations or any other canonical write.
func (s *Store) readStoryJournalLocked(storyID string) (StoryMeta, []StoryEventRecord, error) {
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	defer release()
	return s.readStoryJournalWithRepairLocked(storyID, false)
}

func (s *Store) readStoryJournalWithRepairLocked(storyID string, repairTornTail bool) (StoryMeta, []StoryEventRecord, error) {
	return s.readStoryConversationJournalLocked(storyID, repairTornTail)
}

func trimJSONLRecord(record []byte) []byte {
	if len(record) > 0 && record[len(record)-1] == '\n' {
		record = record[:len(record)-1]
	}
	if len(record) > 0 && record[len(record)-1] == '\r' {
		record = record[:len(record)-1]
	}
	return record
}

func (s *Store) rememberStoryReplayStats(storyID string, stats StoryJournalReplayStats) {
	if s.lastStoryReplayByStory == nil {
		s.lastStoryReplayByStory = make(map[string]StoryJournalReplayStats)
	}
	s.lastStoryReplayByStory[strings.TrimSpace(storyID)] = stats
}

func (s *Store) freezeLegacyActorStateSchemaLocked(storyID string, meta *StoryMeta, events []StoryEventRecord) error {
	return s.freezeLegacyActorStateSchemaFromStateLocked(storyID, meta, stateFromPath(events))
}

func (s *Store) freezeLegacyActorStateSchemaFromStateLocked(storyID string, meta *StoryMeta, state map[string]any) error {
	if meta == nil || meta.ActorStateSchema != nil {
		return nil
	}
	if data, err := os.ReadFile(s.actorStateSchemaPath(storyID)); err == nil {
		var snapshot ActorStateSchemaSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return fmt.Errorf("解析旧故事冻结状态 schema 失败: %w", err)
		}
		before, _ := json.Marshal(snapshot)
		enrichLegacyActorStateSchema(&snapshot, state)
		meta.ActorStateSchema = normalizeActorStateSchemaSnapshot(&snapshot)
		after, _ := json.Marshal(meta.ActorStateSchema)
		if string(before) == string(after) {
			return nil
		}
		return s.writeActorStateSchemaSnapshot(storyID, meta.ActorStateSchema)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取旧故事冻结状态 schema 失败: %w", err)
	}
	if strings.TrimSpace(s.novaDir) == "" {
		return nil
	}
	director := s.storyDirectorForMeta(*meta)
	if err := validateActorStateSystem(director.ActorState); err != nil {
		return fmt.Errorf("旧故事状态 schema 需要人工处理，未执行迁移: %w", err)
	}
	backupDir := filepath.Join(s.novaDir, "backups", "state-system-v6", time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("创建旧故事状态迁移备份目录失败: %w", err)
	}
	data, err := os.ReadFile(s.storyPath(storyID))
	if err != nil {
		return fmt.Errorf("读取旧故事状态迁移备份失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, filepath.Base(s.storyPath(storyID))), data, 0o644); err != nil {
		return fmt.Errorf("写入旧故事状态迁移备份失败: %w", err)
	}
	meta.ActorStateSchema = FreezeActorStateSchemaWithRules(director.ActorState, director.TRPGSystem, true)
	enrichLegacyActorStateSchema(meta.ActorStateSchema, state)
	return s.writeActorStateSchemaSnapshot(storyID, meta.ActorStateSchema)
}

func (s *Store) writeActorStateSchemaSnapshot(storyID string, snapshot *ActorStateSchemaSnapshot) error {
	schemaPath := s.actorStateSchemaPath(storyID)
	if err := os.MkdirAll(filepath.Dir(schemaPath), 0o755); err != nil {
		return fmt.Errorf("创建旧故事冻结状态 schema 目录失败: %w", err)
	}
	schemaData, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化旧故事冻结状态 schema 失败: %w", err)
	}
	tmp := schemaPath + ".tmp"
	if err := os.WriteFile(tmp, append(schemaData, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入旧故事冻结状态 schema 失败: %w", err)
	}
	if err := os.Rename(tmp, schemaPath); err != nil {
		return fmt.Errorf("提交旧故事冻结状态 schema 失败: %w", err)
	}
	return nil
}

func (s *Store) rewriteStoryLocked(storyID string, meta StoryMeta, events []StoryEventRecord, newEvents ...any) error {
	meta = normalizeStoryMeta(meta)
	if err := validateStoryMeta(meta); err != nil {
		return err
	}
	lines := make([]any, 0, len(events)+len(newEvents)+1)
	lines = append(lines, meta)
	for _, event := range events {
		record, err := mapToStoryEventRecord(event.Raw)
		if err != nil {
			return err
		}
		lines = append(lines, record.Raw)
	}
	for _, event := range newEvents {
		record, err := storyEventRecordForWrite(event)
		if err != nil {
			return err
		}
		lines = append(lines, record.Raw)
	}
	writer := writeJSONL
	if s.rewriteJSONL != nil {
		writer = s.rewriteJSONL
	}
	// Close before replacing the inode. A handle bound to the previous
	// incarnation must never refresh against the replacement file.
	if err := s.evictStoryJournalLocked(storyID); err != nil {
		return fmt.Errorf("关闭待维护故事 journal 失败: %w", err)
	}
	if err := writer(s.storyPath(storyID), lines); err != nil {
		return err
	}
	if err := os.Remove(conversationjournal.SidecarPath(s.storyPath(storyID))); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除失效故事索引失败: %w", err)
	}
	return nil
}

func writeJSONL(path string, lines []any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return err
	}
	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)
	for _, line := range lines {
		if err := enc.Encode(line); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return localfs.SyncDirectory(filepath.Dir(path))
}

func writeAtomicBytes(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return localfs.SyncDirectory(dir)
}

func mapToStruct(raw map[string]any, out any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
