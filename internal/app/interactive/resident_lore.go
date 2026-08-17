package interactiveapp

import (
	"denova/internal/book/lore"
	"fmt"
	"strings"
)

// residentLoreReader is the narrow storage boundary required to assemble one
// revision-consistent stable context shared by interactive helper agents.
type residentLoreReader interface {
	List() ([]lore.Item, error)
	ResidentContextMarkdown() (string, error)
	Revision() (string, error)
}

type residentLoreSnapshot struct {
	Content   string
	BodyBytes int
	IDs       []string
	Revision  string
}

// assembleResidentLore uses a before/after revision fence so the stable text,
// its IDs, and its audit revision always describe the same Lore snapshot.
func assembleResidentLore(reader residentLoreReader) (residentLoreSnapshot, error) {
	startRevision, err := reader.Revision()
	if err != nil {
		return residentLoreSnapshot{}, fmt.Errorf("read lore revision before assembly: %w", err)
	}
	items, err := reader.List()
	if err != nil {
		return residentLoreSnapshot{}, fmt.Errorf("read lore items: %w", err)
	}
	content, err := reader.ResidentContextMarkdown()
	if err != nil {
		return residentLoreSnapshot{}, fmt.Errorf("read complete resident lore: %w", err)
	}
	endRevision, err := reader.Revision()
	if err != nil {
		return residentLoreSnapshot{}, fmt.Errorf("read lore revision after assembly: %w", err)
	}
	startRevision = strings.TrimSpace(startRevision)
	endRevision = strings.TrimSpace(endRevision)
	if startRevision != endRevision {
		return residentLoreSnapshot{}, fmt.Errorf("lore changed during resident-context assembly: before=%s after=%s", startRevision, endRevision)
	}
	snapshot := residentLoreSnapshot{Content: content, Revision: endRevision}
	for _, item := range items {
		body := strings.TrimSpace(item.Content)
		if item.LoadMode != lore.LoadModeResident || body == "" {
			continue
		}
		snapshot.IDs = append(snapshot.IDs, strings.TrimSpace(item.ID))
		snapshot.BodyBytes += len([]byte(body))
	}
	return snapshot, nil
}

func validateResidentLoreSnapshot(snapshot residentLoreSnapshot, purpose string, maxContextBytes int) error {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		purpose = "Agent"
	}
	if snapshot.BodyBytes > lore.ResidentLoreSafetyMaxBytes {
		return fmt.Errorf("%s resident lore content is unexpectedly large (%d KB); check for oversized resident files", purpose, (snapshot.BodyBytes+1023)/1024)
	}
	if maxContextBytes <= 0 {
		return fmt.Errorf("%s resident-lore stable context has no size limit", purpose)
	}
	if len([]byte(snapshot.Content)) > maxContextBytes {
		return fmt.Errorf("%s resident-lore stable context exceeds its limit: %d > %d bytes", purpose, len([]byte(snapshot.Content)), maxContextBytes)
	}
	return nil
}
