package app

import (
	"fmt"
	"strings"

	"denova/internal/book"
)

// stateSchemaAdaptationWorkspaceSources separates the stable resident Lore
// prefix from the bounded per-turn JSON assembled in interactive_state_schema.
type stateSchemaAdaptationWorkspaceSources struct {
	CreativeBrief     string
	ResidentLore      string
	ResidentLoreBytes int
	ResidentLoreIDs   []string
	LoreRevision      string
}

type stateSchemaLoreReader interface {
	List() ([]book.LoreItem, error)
	ResidentContextMarkdown() (string, error)
	Revision() (string, error)
}

type stateSchemaResidentLoreSnapshot struct {
	Content   string
	BodyBytes int
	IDs       []string
	Revision  string
}

func stateSchemaAdaptationWorkspaceContext(state *book.State) (stateSchemaAdaptationWorkspaceSources, error) {
	if state == nil || strings.TrimSpace(state.Workspace()) == "" {
		return stateSchemaAdaptationWorkspaceSources{}, nil
	}
	creativeBrief := trimStateSchemaPromptText(state.IdeasContext(), 2000)
	store := book.NewLoreStore(state.Workspace())
	resident, err := assembleStateSchemaResidentLore(store)
	if err != nil {
		return stateSchemaAdaptationWorkspaceSources{}, fmt.Errorf("装配状态结构审查的完整常驻资料失败 workspace=%s: %w", state.Workspace(), err)
	}
	if resident.BodyBytes > book.ResidentLoreSafetyMaxBytes {
		return stateSchemaAdaptationWorkspaceSources{}, fmt.Errorf("状态结构审查的常驻资料正文异常过大（%d KB）；请检查是否误将大型文件设为常驻资料", (resident.BodyBytes+1023)/1024)
	}
	if len([]byte(resident.Content)) > maxInteractiveStateSchemaResidentLoreContextBytes {
		return stateSchemaAdaptationWorkspaceSources{}, fmt.Errorf("状态结构审查的常驻资料稳定上下文超过上限: %d > %d bytes", len([]byte(resident.Content)), maxInteractiveStateSchemaResidentLoreContextBytes)
	}
	return stateSchemaAdaptationWorkspaceSources{
		CreativeBrief:     creativeBrief,
		ResidentLore:      resident.Content,
		ResidentLoreBytes: resident.BodyBytes,
		ResidentLoreIDs:   resident.IDs,
		LoreRevision:      resident.Revision,
	}, nil
}

// assembleStateSchemaResidentLore uses a before/after revision fence so the
// stable text, its IDs, and its audit revision always describe one snapshot.
func assembleStateSchemaResidentLore(reader stateSchemaLoreReader) (stateSchemaResidentLoreSnapshot, error) {
	startRevision, err := reader.Revision()
	if err != nil {
		return stateSchemaResidentLoreSnapshot{}, fmt.Errorf("读取装配前资料库 revision 失败: %w", err)
	}
	items, err := reader.List()
	if err != nil {
		return stateSchemaResidentLoreSnapshot{}, fmt.Errorf("读取资料库条目失败: %w", err)
	}
	content, err := reader.ResidentContextMarkdown()
	if err != nil {
		return stateSchemaResidentLoreSnapshot{}, fmt.Errorf("读取完整常驻资料失败: %w", err)
	}
	endRevision, err := reader.Revision()
	if err != nil {
		return stateSchemaResidentLoreSnapshot{}, fmt.Errorf("读取装配后资料库 revision 失败: %w", err)
	}
	startRevision = strings.TrimSpace(startRevision)
	endRevision = strings.TrimSpace(endRevision)
	if startRevision != endRevision {
		return stateSchemaResidentLoreSnapshot{}, fmt.Errorf("资料库在常驻上下文装配期间发生变化: before=%s after=%s", startRevision, endRevision)
	}
	snapshot := stateSchemaResidentLoreSnapshot{Content: content, Revision: endRevision}
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

func stateSchemaLoreRevision(state *book.State) (string, error) {
	if state == nil || strings.TrimSpace(state.Workspace()) == "" {
		return "", nil
	}
	return book.NewLoreStore(state.Workspace()).Revision()
}
