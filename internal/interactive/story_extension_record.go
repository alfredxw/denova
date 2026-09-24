package interactive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"denova/internal/agents/conversationjournal"
)

const StoryEventTypeExtensionRecord = "extension_record"

// ExtensionRecord is opaque application data attached to a Story or one exact
// turn revision. It never enters model context. JSON is capped at 1 MiB; files
// remain separate assets. Owner and key are assigned/validated by the host.
type ExtensionRecord struct {
	Owner          string          `json:"owner"`
	Key            string          `json:"key"`
	TurnID         string          `json:"turn_id,omitempty"`
	SourceRevision string          `json:"source_revision,omitempty"`
	SchemaVersion  int             `json:"schema_version"`
	Value          json.RawMessage `json:"value"`
}

type extensionRecordEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	ExtensionRecord
}

func extensionRecordKey(branchID string, record ExtensionRecord) string {
	raw, _ := json.Marshal([]string{record.Owner, branchID, record.TurnID, record.SourceRevision, record.Key})
	return string(raw)
}

func validateExtensionRecord(record ExtensionRecord) error {
	if strings.TrimSpace(record.Owner) == "" || len(record.Owner) > 256 || strings.TrimSpace(record.Key) == "" || len(record.Key) > 256 {
		return fmt.Errorf("extension record owner and key must contain 1..256 bytes")
	}
	if (record.TurnID == "") != (record.SourceRevision == "") || len(record.TurnID) > 256 || len(record.SourceRevision) > 256 {
		return fmt.Errorf("extension turn record requires an exact source revision")
	}
	if record.SchemaVersion < 1 || len(record.Value) > 1<<20 || !json.Valid(record.Value) {
		return fmt.Errorf("extension record requires a positive schema version and at most 1 MiB of valid JSON")
	}
	return nil
}

// ExtensionRecord reads an indexed canonical record; missing records have a
// zero revision. Story-level records use an empty branch, internally main.
func (s *Store) ExtensionRecord(ctx context.Context, storyID, branchID string, address ExtensionRecord) (ExtensionRecord, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return ExtensionRecord{}, 0, err
	}
	if branchID == "" {
		branchID = "main"
	}
	return s.extensionRecordLocked(ctx, handle, branchID, address)
}

func (s *Store) extensionRecordLocked(ctx context.Context, handle *storyJournalHandle, branchID string, address ExtensionRecord) (ExtensionRecord, uint64, error) {
	cursor := handle.projection.ExtensionRecords[extensionRecordKey(branchID, address)]
	if cursor == 0 {
		return ExtensionRecord{}, 0, nil
	}
	records, err := handle.journal.ReadRange(ctx, conversationjournal.Range{After: cursor - 1, Through: cursor})
	if err != nil {
		return ExtensionRecord{}, 0, err
	}
	for _, record := range records {
		var event extensionRecordEvent
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			return ExtensionRecord{}, 0, err
		}
		if event.Type == StoryEventTypeExtensionRecord && extensionRecordKey(event.BranchID, event.ExtensionRecord) == extensionRecordKey(branchID, address) {
			return event.ExtensionRecord, uint64(cursor), nil
		}
	}
	return ExtensionRecord{}, 0, fmt.Errorf("extension record locator is invalid")
}

// SetExtensionRecord uses cross-store admission and CAS. Derived turn data is
// accepted only while its original branch still contains that exact revision;
// asynchronous work never overwrites another revision or a different branch.
func (s *Store) SetExtensionRecord(ctx context.Context, storyID, branchID string, expected uint64, record ExtensionRecord) (uint64, error) {
	if err := validateExtensionRecord(record); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return 0, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return 0, err
	}
	if branchID == "" {
		branchID = "main"
	}
	branch, ok := handle.projection.Meta.Branches[branchID]
	if !ok {
		return 0, fmt.Errorf("extension record branch is unavailable")
	}
	key := extensionRecordKey(branchID, record)
	if uint64(handle.projection.ExtensionRecords[key]) != expected {
		return 0, fmt.Errorf("%w: extension record changed", conversationjournal.ErrConflict)
	}
	if record.TurnID != "" {
		cursor := ""
		found := false
		for {
			page, err := s.readStoryHistoryPageLocked(storyID, branchID, cursor, maxStoryHistoryPageTurns, true)
			if err != nil {
				return 0, err
			}
			for _, turn := range page.page.Turns {
				if turn.ID != record.TurnID {
					continue
				}
				if TurnNarrativeRevision(turn) != record.SourceRevision {
					return 0, fmt.Errorf("%w: source turn revision changed", conversationjournal.ErrConflict)
				}
				found = true
				break
			}
			if found || !page.page.HasMore {
				break
			}
			cursor = page.page.BeforeCursor
		}
		if !found {
			return 0, fmt.Errorf("%w: source turn is no longer on this branch", conversationjournal.ErrConflict)
		}
	}
	meta := handle.projection.Meta
	event := extensionRecordEvent{V: schemaVersion, Type: StoryEventTypeExtensionRecord, ID: newID("ext"), ParentID: branch.Head, BranchID: branchID, Ts: time.Now().UTC().Format(time.RFC3339Nano), ExtensionRecord: record}
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return 0, err
	}
	return uint64(handle.projection.ExtensionRecords[key]), nil
}

// ExtensionRecordAddresses lists metadata only; values are read by canonical
// locators. This supports managed application discovery without a second index.
func (s *Store) ExtensionRecordAddresses(storyID, owner string) ([]ExtensionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return nil, err
	}
	result := []ExtensionRecord{}
	for key := range handle.projection.ExtensionRecords {
		var parts []string
		if json.Unmarshal([]byte(key), &parts) != nil || len(parts) != 5 {
			return nil, fmt.Errorf("invalid extension record address")
		}
		if parts[0] == owner && parts[1] == "main" && parts[2] == "" {
			result = append(result, ExtensionRecord{Owner: owner, Key: parts[4]})
		}
	}
	return result, nil
}

// TurnNarrativeRevision identifies an immutable prose revision, including edits
// that later restore the same text. It is not a content hash.
func TurnNarrativeRevision(turn TurnEvent) string {
	if turn.NarrativeRevision != "" {
		return turn.NarrativeRevision
	}
	return turn.ID
}

// ExportJournal copies the one canonical journal while a cross-store lease
// prevents a partial concurrent append. Rebuildable indexes are omitted.
func (s *Store) ExportJournal(ctx context.Context, storyID string, writer io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer release()
	if _, err := s.refreshStoryJournalLocked(storyID, true); err != nil {
		return err
	}
	file, err := os.Open(s.storyPath(storyID))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}
