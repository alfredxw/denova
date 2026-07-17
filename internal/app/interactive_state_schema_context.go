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

func stateSchemaAdaptationWorkspaceContext(state *book.State) (stateSchemaAdaptationWorkspaceSources, error) {
	if state == nil || strings.TrimSpace(state.Workspace()) == "" {
		return stateSchemaAdaptationWorkspaceSources{}, nil
	}
	creativeBrief := trimStateSchemaPromptText(state.IdeasContext(), 2000)
	store := book.NewLoreStore(state.Workspace())
	resident, err := assembleResidentLore(store)
	if err != nil {
		return stateSchemaAdaptationWorkspaceSources{}, fmt.Errorf("装配状态结构审查的完整常驻资料失败 workspace=%s: %w", state.Workspace(), err)
	}
	if err := validateResidentLoreSnapshot(resident, "状态结构审查", maxInteractiveStateSchemaResidentLoreContextBytes); err != nil {
		return stateSchemaAdaptationWorkspaceSources{}, err
	}
	return stateSchemaAdaptationWorkspaceSources{
		CreativeBrief:     creativeBrief,
		ResidentLore:      resident.Content,
		ResidentLoreBytes: resident.BodyBytes,
		ResidentLoreIDs:   resident.IDs,
		LoreRevision:      resident.Revision,
	}, nil
}

func stateSchemaLoreRevision(state *book.State) (string, error) {
	if state == nil || strings.TrimSpace(state.Workspace()) == "" {
		return "", nil
	}
	return book.NewLoreStore(state.Workspace()).Revision()
}
