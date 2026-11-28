package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	"denova/internal/agents/modeltask"
	interactiveapp "denova/internal/app/interactive"
	appsettings "denova/internal/app/settings"
	booklore "denova/internal/book/lore"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	projectdomain "denova/internal/project"
	"denova/internal/style"
)

// InlineEditInput carries the API payload for one project-scoped inline prose edit.
type InlineEditInput struct {
	Mode           modeltask.InlineEditMode
	Instruction    string
	Selection      string
	Before         string
	After          string
	PreviousResult string
	LoreIDs        []string
	IncludeLore    *bool
	IncludeStyle   *bool
	StoryID        string
	Locale         string
}

// InlineEditMeta summarizes the resolved model and context injected into one inline edit.
type InlineEditMeta struct {
	Model     string `json:"model"`
	StyleName string `json:"style_name,omitempty"`
	LoreCount int    `json:"lore_count"`
}

// PreparedInlineEdit holds an immutable snapshot of project config and bounded
// context so SSE streaming can outlive the HTTP handler's project lease safely.
type PreparedInlineEdit struct {
	Config    config.Config
	AgentKind string
	Request   modeltask.InlineEditRequest
	Meta      InlineEditMeta
}

// PrepareProjectInlineEdit validates the request, resolves the project's
// effective language model, optional narrative style, and matched Lore items
// synchronously while the Project operation lease is active.
func (a *App) PrepareProjectInlineEdit(
	ctx context.Context,
	layout projectdomain.Layout,
	input InlineEditInput,
) (PreparedInlineEdit, error) {
	req := modeltask.InlineEditRequest{
		Mode:           input.Mode,
		Instruction:    input.Instruction,
		Selection:      input.Selection,
		Before:         input.Before,
		After:          input.After,
		PreviousResult: input.PreviousResult,
	}
	if err := modeltask.ValidateInlineEditRequest(req); err != nil {
		return PreparedInlineEdit{}, err
	}

	a.mu.RLock()
	baseCfg := a.cfg
	var activeInteractive *interactive.Store
	if a.cfg != nil && a.cfg.ProjectID == layout.ProjectID {
		activeInteractive = a.interactive
	}
	a.mu.RUnlock()

	if baseCfg == nil {
		return PreparedInlineEdit{}, modeltask.ErrInlineEditModelNotConfigured
	}
	effectiveCfg := *baseCfg
	effectiveCfg.ProjectID = layout.ProjectID
	effectiveCfg.Workspace = layout.ContentRoot
	effectiveCfg.ProjectStoreDir = layout.StoreRoot

	dataDir := effectiveCfg.DataDir()
	projectConfigPath := layout.ConfigPath()
	if layered, err := config.LoadLayeredWithStartupConfigAt(dataDir, layout.ContentRoot, projectConfigPath); err == nil {
		appsettings.ApplyLayered(&effectiveCfg, layered)
		if strings.TrimSpace(layered.Effective.IDEStoryTellerID) != "" {
			effectiveCfg.IDEStoryTellerID = layered.Effective.IDEStoryTellerID
		}
	} else {
		slog.WarnContext(ctx, "[app/inline-edit] load layered settings failed, using runtime defaults",
			"project_id", layout.ProjectID,
			"error", err,
		)
	}
	appsettings.ApplyLocale(&effectiveCfg, input.Locale)

	agentKind := config.AgentKindIDE
	if input.Mode == modeltask.InlineEditModeGameTurn {
		agentKind = config.AgentKindInteractiveStory
	}
	modelCfg, err := modeltask.ResolveInlineEditModel(&effectiveCfg, agentKind)
	if err != nil {
		return PreparedInlineEdit{}, err
	}

	includeStyle := input.Mode != modeltask.InlineEditModeLore
	if input.IncludeStyle != nil {
		includeStyle = *input.IncludeStyle
	}
	includeLore := true
	if input.IncludeLore != nil {
		includeLore = *input.IncludeLore
	}

	var styleName string
	if includeStyle && dataDir != "" {
		styleName, req.StyleGuidance = resolveInlineEditStyleGuidance(ctx, dataDir, layout, effectiveCfg, input, activeInteractive)
	}

	if includeLore && len(input.LoreIDs) > 0 && layout.ContentRoot != "" {
		req.LoreItems = resolveInlineEditLoreItems(ctx, layout.ContentRoot, input.LoreIDs)
	}

	return PreparedInlineEdit{
		Config:    effectiveCfg,
		AgentKind: agentKind,
		Request:   req,
		Meta: InlineEditMeta{
			Model:     modelCfg.Model,
			StyleName: styleName,
			LoreCount: len(req.LoreItems),
		},
	}, nil
}

// StreamPreparedInlineEdit executes the prepared tool-free streaming model call.
func (a *App) StreamPreparedInlineEdit(
	ctx context.Context,
	prepared PreparedInlineEdit,
	onDelta func(string) error,
) (string, error) {
	cfg := prepared.Config
	return modeltask.StreamInlineEdit(ctx, &cfg, prepared.AgentKind, prepared.Request, onDelta)
}

func resolveInlineEditStyleGuidance(
	ctx context.Context,
	dataDir string,
	layout projectdomain.Layout,
	cfg config.Config,
	input InlineEditInput,
	activeInteractive *interactive.Store,
) (string, string) {
	var def teller.Definition
	switch input.Mode {
	case modeltask.InlineEditModeGameTurn:
		tellerID := resolveGameStoryTellerID(ctx, layout, input.StoryID, activeInteractive)
		def = interactiveapp.LoadGameTeller(dataDir, tellerID)
	case modeltask.InlineEditModeWriting, modeltask.InlineEditModeLore:
		tellerID := strings.TrimSpace(cfg.IDEStoryTellerID)
		if tellerID == "" {
			tellerID = style.DefaultID
		}
		def = interactiveapp.LoadWritingTeller(dataDir, tellerID)
	}
	if def.ID == "" {
		return "", ""
	}
	guidance := buildTellerStyleGuidance(dataDir, def)
	if guidance == "" {
		return "", ""
	}
	name := strings.TrimSpace(def.Name)
	if name == "" {
		name = def.ID
	}
	return name, guidance
}

func resolveGameStoryTellerID(
	ctx context.Context,
	layout projectdomain.Layout,
	storyID string,
	activeInteractive *interactive.Store,
) string {
	storyID = strings.TrimSpace(storyID)
	if storyID == "" || layout.ContentRoot == "" {
		return ""
	}
	store := activeInteractive
	if store == nil {
		store = interactive.NewStore(layout.ContentRoot)
		defer func() {
			if err := store.Close(); err != nil {
				slog.WarnContext(ctx, "[app/inline-edit] close temporary interactive store failed",
					"project_id", layout.ProjectID,
					"error", err,
				)
			}
		}()
	}
	index, err := store.Index()
	if err != nil {
		slog.WarnContext(ctx, "[app/inline-edit] read interactive story index failed",
			"project_id", layout.ProjectID,
			"story_id", storyID,
			"error", err,
		)
		return ""
	}
	for _, story := range index.Stories {
		if story.ID == storyID {
			return strings.TrimSpace(story.StoryTellerID)
		}
	}
	return ""
}

func buildTellerStyleGuidance(dataDir string, def teller.Definition) string {
	var parts []string
	if name := strings.TrimSpace(def.Name); name != "" {
		parts = append(parts, fmt.Sprintf("Preset: %s", name))
	}
	if desc := strings.TrimSpace(def.Description); desc != "" {
		parts = append(parts, fmt.Sprintf("Summary: %s", desc))
	}
	if prompt := strings.TrimSpace(def.PromptForTargets("system", "turn_context")); prompt != "" {
		parts = append(parts, prompt)
	}

	globalPaths := append([]string(nil), def.StyleRefs...)
	var legacyContents []string
	for _, rule := range def.StyleRules {
		scene := strings.ToLower(strings.TrimSpace(rule.Scene))
		if scene == "全局" || scene == "global" {
			globalPaths = append(globalPaths, rule.StyleRefs...)
			legacyContents = append(legacyContents, rule.StyleContents...)
		}
	}
	if len(globalPaths) > 0 && dataDir != "" {
		lib := style.NewLibrary(dataDir)
		seen := map[string]bool{}
		for _, refPath := range globalPaths {
			normalized := style.NormalizeStoragePath(refPath)
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			doc, err := lib.Read(normalized)
			if err != nil {
				continue
			}
			if content := strings.TrimSpace(doc.Content); content != "" {
				parts = append(parts, content)
			}
		}
	}
	for _, legacy := range legacyContents {
		if trimmed := strings.TrimSpace(legacy); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func resolveInlineEditLoreItems(ctx context.Context, workspace string, loreIDs []string) []modeltask.InlineEditLoreContext {
	capped := make([]string, 0, min(len(loreIDs), modeltask.InlineEditMaxLoreItems))
	seen := map[string]bool{}
	for _, raw := range loreIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		capped = append(capped, id)
		if len(capped) >= modeltask.InlineEditMaxLoreItems {
			break
		}
	}
	if len(capped) == 0 {
		return nil
	}
	readResult, err := booklore.NewStore(workspace).ReadMany(capped)
	if err != nil {
		slog.WarnContext(ctx, "[app/inline-edit] read matched lore items failed", "error", err)
		return nil
	}
	result := make([]modeltask.InlineEditLoreContext, 0, len(readResult.Items))
	for _, item := range readResult.Items {
		if !item.Enabled {
			continue
		}
		description := strings.TrimSpace(item.BriefDescription)
		if description == "" {
			description = strings.TrimSpace(item.Content)
		}
		result = append(result, modeltask.InlineEditLoreContext{
			Name:        item.Name,
			Type:        item.Type,
			Description: description,
		})
	}
	return result
}
