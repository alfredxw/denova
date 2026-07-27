package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

func appendClearRecordLine(sess *Session, line []byte) error {
	var marker clearRecord
	if err := json.Unmarshal(line, &marker); err != nil {
		return err
	}
	if marker.CreatedAt.IsZero() {
		marker.CreatedAt = sess.UpdatedAt
	}
	sess.clearAfterIndex = sess.messageBaseIndex + len(sess.messages)
	sess.records = append(sess.records, historyRecord{kind: historyTypeClear, createdAt: marker.CreatedAt})
	advanceContextRevision(sess, marker.ContextRevision)
	advanceUpdatedAt(sess, marker.CreatedAt)
	return nil
}

func appendInterruptionRecordLine(sess *Session, line []byte, lineNumber int) error {
	var marker interruptionRecord
	if err := json.Unmarshal(line, &marker); err != nil {
		return err
	}
	interruption := marker.Interruption
	if strings.TrimSpace(interruption.ID) == "" {
		interruption.ID = legacyJournalRecordID("interrupt", lineNumber)
	}
	if strings.TrimSpace(interruption.Status) == "" {
		interruption.Status = InterruptionPending
	}
	sess.records = append(sess.records, historyRecord{kind: historyTypeInterrupt, interruption: &interruption, createdAt: interruption.CreatedAt})
	advanceUpdatedAt(sess, interruption.CreatedAt)
	return nil
}

func appendAskRecordLine(sess *Session, line []byte) error {
	var marker askRecord
	if err := json.Unmarshal(line, &marker); err != nil {
		return err
	}
	interaction, err := normalizeAskInteraction(marker.AskInteraction)
	if err != nil {
		return err
	}
	copy := cloneAskInteraction(interaction)
	sess.records = append(sess.records, historyRecord{kind: historyTypeAsk, ask: &copy, createdAt: interaction.CreatedAt})
	advanceUpdatedAt(sess, interaction.CreatedAt)
	return nil
}

func appendContextBoundaryRecordLine(sess *Session, line []byte) error {
	var record contextBoundaryRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return err
	}
	if strings.TrimSpace(record.BoundaryID) == "" || len(record.BoundaryID) > maxContextLabelBytes {
		return fmt.Errorf("context boundary id is invalid")
	}
	if err := validateContextBoundarySnapshot(&record.Boundary); err != nil {
		return fmt.Errorf("context boundary %q: %w", record.BoundaryID, err)
	}
	advanceUpdatedAt(sess, record.CreatedAt)
	return nil
}

func appendCompactionRecordLine(sess *Session, line []byte, lineNumber int) error {
	var record ContextCompaction
	if err := json.Unmarshal(line, &record); err != nil {
		return err
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = legacyJournalRecordID("compaction", lineNumber)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = sess.UpdatedAt
	}
	record.Type = historyTypeCompaction
	sess.records = append(sess.records, historyRecord{kind: historyTypeCompaction, compaction: &record, createdAt: record.CreatedAt})
	advanceContextRevision(sess, record.ContextRevision)
	advanceUpdatedAt(sess, record.CreatedAt)
	return nil
}

func appendCompactionRemovalRecordLine(sess *Session, line []byte, lineNumber int) error {
	var record ContextCompactionRemoval
	if err := json.Unmarshal(line, &record); err != nil {
		return err
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = legacyJournalRecordID("compaction-removal", lineNumber)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = sess.UpdatedAt
	}
	record.Type = historyTypeCompactionRemoved
	sess.records = append(sess.records, historyRecord{kind: historyTypeCompactionRemoved, compactionRemoval: &record, createdAt: record.CreatedAt})
	advanceContextRevision(sess, record.ContextRevision)
	advanceUpdatedAt(sess, record.CreatedAt)
	return nil
}

func appendDisplayRecordLine(sess *Session, line []byte, lineNumber int) error {
	var marker displayRecord
	if err := json.Unmarshal(line, &marker); err != nil {
		return err
	}
	recordID := strings.TrimSpace(marker.RecordID)
	if recordID == "" {
		recordID = legacyJournalRecordID("display", lineNumber)
	}
	if findJournalRecordIndex(sess.records, recordID) >= 0 {
		return fmt.Errorf("重复的 display record_id: %s", recordID)
	}
	event := marker.DisplayEvent
	if strings.TrimSpace(event.Role) == "" {
		return fmt.Errorf("display record role 不能为空")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = sess.UpdatedAt
	}
	sess.records = append(sess.records, historyRecord{
		journalID: recordID, kind: historyTypeDisplay, display: &event, createdAt: event.CreatedAt,
		displayArgsPersistedBytes: len(event.Args), displayContentPersistedBytes: len(event.Content),
	})
	advanceUpdatedAt(sess, event.CreatedAt)
	return nil
}

func applySessionPatchLine(sess *Session, line []byte) error {
	var patch sessionPatchRecord
	if err := json.Unmarshal(line, &patch); err != nil {
		return err
	}
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return fmt.Errorf("session patch title 不能为空")
		}
		sess.title = title
	}
	advanceUpdatedAt(sess, patch.UpdatedAt)
	return nil
}

func applyDisplayPatchLine(sess *Session, line []byte) error {
	var patch displayPatchRecord
	if err := json.Unmarshal(line, &patch); err != nil {
		return err
	}
	index := findJournalRecordIndex(sess.records, strings.TrimSpace(patch.TargetRecordID))
	if index < 0 || sess.records[index].display == nil {
		if sess.partialMaterialization {
			advanceUpdatedAt(sess, patch.CreatedAt)
			return nil
		}
		return fmt.Errorf("display patch target 不存在: %s", patch.TargetRecordID)
	}
	record := &sess.records[index]
	event := record.display
	if patch.DisplayPhase != nil {
		event.DisplayPhase = *patch.DisplayPhase
	}
	if patch.Status != nil {
		event.Status = *patch.Status
	}
	if patch.Result != nil {
		event.Result = *patch.Result
	}
	if patch.ArgsAppend != "" {
		persistedBytes := min(max(record.displayArgsPersistedBytes, 0), len(event.Args))
		pending := event.Args[persistedBytes:]
		event.Args = event.Args[:persistedBytes] + patch.ArgsAppend + pending
		record.displayArgsPersistedBytes = persistedBytes + len(patch.ArgsAppend)
	}
	if patch.ContentAppend != "" {
		persistedBytes := min(max(record.displayContentPersistedBytes, 0), len(event.Content))
		pending := event.Content[persistedBytes:]
		event.Content = event.Content[:persistedBytes] + patch.ContentAppend + pending
		record.displayContentPersistedBytes = persistedBytes + len(patch.ContentAppend)
	}
	if patch.Illustration != nil {
		event.Illustration = cloneChapterIllustration(patch.Illustration)
	}
	advanceUpdatedAt(sess, patch.CreatedAt)
	return nil
}

func applyInterruptionPatchLine(sess *Session, line []byte) error {
	var patch interruptionPatchRecord
	if err := json.Unmarshal(line, &patch); err != nil {
		return err
	}
	for i := range sess.records {
		record := &sess.records[i]
		if record.kind != historyTypeInterrupt || record.interruption == nil || record.interruption.ID != patch.TargetID {
			continue
		}
		record.interruption.Status = patch.Status
		record.interruption.ResolvedAt = patch.ResolvedAt
		advanceUpdatedAt(sess, patch.UpdatedAt)
		return nil
	}
	if sess.partialMaterialization {
		advanceUpdatedAt(sess, patch.UpdatedAt)
		return nil
	}
	return fmt.Errorf("interruption patch target 不存在: %s", patch.TargetID)
}

func applyAskPatchLine(sess *Session, line []byte) error {
	var patch askPatchRecord
	if err := json.Unmarshal(line, &patch); err != nil {
		return err
	}
	for index := range sess.records {
		record := &sess.records[index]
		if record.kind != historyTypeAsk || record.ask == nil || record.ask.ID != patch.TargetID {
			continue
		}
		resolvedAt := patch.ResolvedAt
		record.ask.Status = patch.Status
		record.ask.Answers = cloneAskAnswerResults(patch.Answers)
		record.ask.CancelReason = patch.CancelReason
		record.ask.ResolvedAt = &resolvedAt
		advanceUpdatedAt(sess, patch.UpdatedAt)
		return nil
	}
	if sess.partialMaterialization {
		advanceUpdatedAt(sess, patch.UpdatedAt)
		return nil
	}
	return fmt.Errorf("ask patch target does not exist: %s", patch.TargetID)
}

func appendMessageRecordLine(sess *Session, line []byte, kind string) error {
	var record messageRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return err
	}
	if record.Message.Role == "" && record.Message.Content == "" && len(record.Message.ToolCalls) == 0 {
		return fmt.Errorf("message record 缺少 role、content 和 tool_calls")
	}
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = nextLegacyMessageCreatedAt(sess)
	}
	msg := record.Message
	metadata := sanitizeMessageMetadata(record.MessageMetadata)
	sess.messages = append(sess.messages, &msg)
	sess.messageCount++
	sess.records = append(sess.records, historyRecord{kind: kind, message: &msg, messageMetadata: metadata, createdAt: createdAt})
	advanceContextRevision(sess, metadata.ContextRevision)
	advanceUpdatedAt(sess, createdAt)
	return nil
}

func appendLegacyMessageLine(sess *Session, line []byte) error {
	var msg agent.Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return err
	}
	if msg.Role == "" && msg.Content == "" && len(msg.ToolCalls) == 0 {
		return fmt.Errorf("旧格式消息缺少 role、content 和 tool_calls")
	}
	createdAt := nextLegacyMessageCreatedAt(sess)
	sess.messages = append(sess.messages, &msg)
	sess.messageCount++
	sess.records = append(sess.records, historyRecord{kind: historyTypeMessage, message: &msg, createdAt: createdAt})
	advanceContextRevision(sess, 0)
	advanceUpdatedAt(sess, createdAt)
	return nil
}

func findJournalRecordIndex(records []historyRecord, journalID string) int {
	if journalID == "" {
		return -1
	}
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].journalID == journalID {
			return i
		}
	}
	return -1
}

func legacyJournalRecordID(kind string, lineNumber int) string {
	return fmt.Sprintf("legacy-%s-line-%d", kind, lineNumber)
}

func advanceUpdatedAt(sess *Session, candidate time.Time) {
	if candidate.After(sess.UpdatedAt) {
		sess.UpdatedAt = candidate
	}
}

func advanceContextRevision(sess *Session, persisted uint64) {
	if persisted > sess.contextRevision {
		sess.contextRevision = persisted
		return
	}
	sess.contextRevision++
}

func nextLegacyMessageCreatedAt(sess *Session) time.Time {
	base := sess.UpdatedAt
	if base.IsZero() {
		base = sess.CreatedAt
	}
	if base.IsZero() {
		base = time.Now().UTC()
	}
	return base.Add(time.Duration(len(sess.records)+1) * time.Millisecond)
}
