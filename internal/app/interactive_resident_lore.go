package app

import (
	"fmt"
	"strings"

	"denova/internal/book"
)

// residentLoreReader is the narrow storage boundary required to assemble one
// revision-consistent stable context shared by interactive helper agents.
type residentLoreReader interface {
	List() ([]book.LoreItem, error)
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
		return residentLoreSnapshot{}, fmt.Errorf("读取装配前资料库 revision 失败: %w", err)
	}
	items, err := reader.List()
	if err != nil {
		return residentLoreSnapshot{}, fmt.Errorf("读取资料库条目失败: %w", err)
	}
	content, err := reader.ResidentContextMarkdown()
	if err != nil {
		return residentLoreSnapshot{}, fmt.Errorf("读取完整常驻资料失败: %w", err)
	}
	endRevision, err := reader.Revision()
	if err != nil {
		return residentLoreSnapshot{}, fmt.Errorf("读取装配后资料库 revision 失败: %w", err)
	}
	startRevision = strings.TrimSpace(startRevision)
	endRevision = strings.TrimSpace(endRevision)
	if startRevision != endRevision {
		return residentLoreSnapshot{}, fmt.Errorf("资料库在常驻上下文装配期间发生变化: before=%s after=%s", startRevision, endRevision)
	}
	snapshot := residentLoreSnapshot{Content: content, Revision: endRevision}
	for _, item := range items {
		body := strings.TrimSpace(item.Content)
		if item.LoadMode != book.LoreLoadModeResident || body == "" {
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
	if snapshot.BodyBytes > book.ResidentLoreSafetyMaxBytes {
		return fmt.Errorf("%s的常驻资料正文异常过大（%d KB）；请检查是否误将大型文件设为常驻资料", purpose, (snapshot.BodyBytes+1023)/1024)
	}
	if maxContextBytes <= 0 {
		return fmt.Errorf("%s的常驻资料稳定上下文缺少大小上限", purpose)
	}
	if len([]byte(snapshot.Content)) > maxContextBytes {
		return fmt.Errorf("%s的常驻资料稳定上下文超过上限: %d > %d bytes", purpose, len([]byte(snapshot.Content)), maxContextBytes)
	}
	return nil
}
