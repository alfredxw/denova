package session

import (
	"encoding/json"
	"fmt"

	"denova/internal/agents/conversationjournal"
	externaljournal "denova/internal/agents/runtime/external/journal"
	"denova/internal/agents/sessionjournal"
)

const historyPageMaxBytes = 8 << 20

// historyPageProjection keeps only the requested display window while scanning
// its sparse interval. Assistant digests preserve canonical/display deduplication
// even when an unusually large turn spans more than one page.
type historyPageProjection struct {
	rows                *Session
	index               *sessionJournalProjection
	positions           []int
	start, end, maxRows int
}

func (p *historyPageProjection) apply(record conversationjournal.Record) error {
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(record.Payload, &header); err != nil {
		return err
	}
	before := p.index.HistoryCount
	patch := false
	switch header.Type {
	case historyTypeDisplay, historyTypeDisplayPatch, historyTypeMessage, historyTypeClear, "":
		if err := p.index.Apply(record); err != nil {
			return err
		}
		patch = header.Type == historyTypeDisplayPatch
	case externaljournal.RecordType:
		var event externaljournal.Record
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			return err
		}
		switch event.Kind {
		case externaljournal.ToolStarted:
			p.index.HistoryCount++
		case externaljournal.OperationClosed:
			usage, err := ExternalUsageDisplay(event)
			if err != nil {
				return err
			}
			if usage == nil {
				return nil
			}
			p.index.HistoryCount++
		case externaljournal.ToolFinished:
			patch = true
		case externaljournal.OperationAccepted, externaljournal.ContextCheckpoint, externaljournal.GuidanceDelivered:
			return nil
		}
	case "session", historyTypeContextMessage, historyTypeContextBatch, sessionjournal.RecordType,
		historyTypeInterrupt, historyTypeInterruptionPatch, historyTypeSessionPatch, historyTypeRuntimePatch, platformRecordType:
		return nil
	default:
		if isRetiredSessionJournalRecordType(header.Type) {
			return nil
		}
		return fmt.Errorf("unknown history page record type %q", header.Type)
	}
	if !patch && (p.index.HistoryCount == before || before >= p.end) {
		return nil
	}
	if err := appendConversationRecord(p.rows, record); err != nil {
		return err
	}
	// Only History() consumes this projection; the model message array would
	// otherwise keep bodies alive after their display rows leave the window.
	p.rows.messages = nil
	if !patch {
		p.positions = append(p.positions, before)
		last := p.rows.records[len(p.rows.records)-1]
		if before <= p.start && (last.kind == historyTypeClear || last.message != nil && last.message.Role == "user") {
			p.drop(len(p.rows.records) - 1)
		}
	}
	bytes := 0
	for i := len(p.rows.records) - 1; i >= 0; i-- {
		row := p.rows.records[i]
		if row.display != nil {
			bytes += len(row.display.Content) + len(row.display.Args) + len(row.display.Result)
		}
		if row.message != nil {
			bytes += len(row.message.Content) + len(row.messageMetadata.DisplayContent)
		}
		// A single item remains whole. Tool production already enforces its own
		// result budget; pagination must not silently truncate creator content.
		if i < len(p.rows.records)-1 && (bytes > historyPageMaxBytes || len(p.rows.records)-i > p.maxRows) {
			p.drop(i + 1)
			break
		}
	}
	return nil
}

func (p *historyPageProjection) drop(count int) {
	if count == 0 {
		return
	}
	p.rows.records = append([]historyRecord(nil), p.rows.records[count:]...)
	p.positions = append([]int(nil), p.positions[count:]...)
}
