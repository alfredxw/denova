package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"nova/internal/imagepreset"
	"nova/internal/interactive"
	"nova/internal/interactiveimage"
)

const (
	interactiveImageToolName     = "generate_interactive_image"
	interactiveImageSkill        = "interactive-image"
	interactiveImageSourceAuto   = "auto"
	interactiveImageSourceManual = "manual"
)

type InteractiveImageGenerateResult struct {
	Enabled       bool                     `json:"enabled"`
	Skipped       bool                     `json:"skipped,omitempty"`
	SkippedReason string                   `json:"skipped_reason,omitempty"`
	Image         *interactiveimage.Result `json:"image,omitempty"`
}

func (a *App) GenerateInteractiveImage(ctx context.Context, storyID string, req interactive.InteractiveImageGenerateRequest) (InteractiveImageGenerateResult, error) {
	return a.interactiveService().GenerateInteractiveImage(ctx, storyID, req)
}

func (s *InteractiveAppService) GenerateInteractiveImage(ctx context.Context, storyID string, req interactive.InteractiveImageGenerateRequest) (InteractiveImageGenerateResult, error) {
	a := s.app
	a.mu.RLock()
	store := a.interactive
	workspace := a.workspace
	novaDir := ""
	if a.cfg != nil {
		novaDir = a.cfg.NovaDir
	}
	a.mu.RUnlock()
	if store == nil || strings.TrimSpace(workspace) == "" {
		return InteractiveImageGenerateResult{}, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return InteractiveImageGenerateResult{}, err
	}
	turn, turnIndex, err := interactiveImageTargetTurn(storyCtx.Snapshot.Turns, req.TurnID)
	if err != nil {
		return InteractiveImageGenerateResult{}, err
	}
	source := normalizeInteractiveImageSource(req.Source)
	should, reason := shouldGenerateInteractiveImage(storyCtx.Meta.ImageSettings, storyCtx.Snapshot.Turns, turnIndex, source, req.Force)
	if !should {
		return InteractiveImageGenerateResult{Enabled: storyCtx.Meta.ImageSettings.Mode != interactive.StoryImageModeManual, Skipped: true, SkippedReason: reason}, nil
	}
	if existing := interactiveImageDisplayEvent(turn.DisplayEvents); existing != nil && !req.Force {
		return InteractiveImageGenerateResult{Enabled: true, Skipped: true, SkippedReason: "already_exists"}, nil
	}
	eventID := interactiveImageEventID(turn.ID)
	if err := store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
		ID:      eventID,
		Role:    "tool_call",
		Content: interactiveImageToolName,
		Name:    interactiveImageToolName,
		Status:  "running",
		Args:    interactiveImageEventArgs(source, req.Force),
	}); err != nil {
		return InteractiveImageGenerateResult{}, err
	}

	preset := loadImagePreset(novaDir, storyCtx.Meta.ImageSettings.PresetID)
	sourceContext := interactiveImageSourceContext(storyCtx.Meta, storyCtx.Snapshot.Turns, turnIndex, store)
	systemPrompt := interactiveImageSystemPrompt(preset)
	toolPrompt := preset.PromptForTargets(imagepreset.TargetToolRequest)
	result, err := a.GenerateImageWithAgent(ctx, ImageAgentGenerateRequest{
		Purpose:       "interactive_image",
		SourceContext: sourceContext,
		SystemPrompt:  systemPrompt,
		ToolPrompt:    toolPrompt,
		SkillName:     interactiveImageSkill,
		StoryID:       storyID,
		BranchID:      storyCtx.Snapshot.BranchID,
		TurnID:        turn.ID,
		AltText:       interactiveImageAltText(storyCtx.Meta.Title, turnIndex),
	})
	if err != nil {
		_ = store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
			ID:      eventID,
			Role:    "tool_call",
			Content: interactiveImageToolName,
			Name:    interactiveImageToolName,
			Status:  "error",
			Args:    interactiveImageEventArgs(source, req.Force),
			Result:  interactiveImageErrorResult(err),
		})
		return InteractiveImageGenerateResult{}, err
	}
	if result.InteractiveImage == nil {
		err := fmt.Errorf("图像 Agent 未返回互动图像")
		_ = store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
			ID:      eventID,
			Role:    "tool_call",
			Content: interactiveImageToolName,
			Name:    interactiveImageToolName,
			Status:  "error",
			Args:    interactiveImageEventArgs(source, req.Force),
			Result:  interactiveImageErrorResult(err),
		})
		return InteractiveImageGenerateResult{}, err
	}
	data, err := json.Marshal(result.InteractiveImage)
	if err != nil {
		return InteractiveImageGenerateResult{}, err
	}
	if err := store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
		ID:      eventID,
		Role:    "tool_call",
		Content: interactiveImageToolName,
		Name:    interactiveImageToolName,
		Status:  "success",
		Args:    interactiveImageEventArgs(source, req.Force),
		Result:  string(data),
	}); err != nil {
		return InteractiveImageGenerateResult{}, err
	}
	return InteractiveImageGenerateResult{Enabled: true, Image: result.InteractiveImage}, nil
}

func normalizeInteractiveImageSource(source string) string {
	switch strings.TrimSpace(source) {
	case interactiveImageSourceAuto:
		return interactiveImageSourceAuto
	default:
		return interactiveImageSourceManual
	}
}

func interactiveImageTargetTurn(turns []interactive.TurnEvent, turnID string) (interactive.TurnEvent, int, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		if len(turns) == 0 {
			return interactive.TurnEvent{}, -1, fmt.Errorf("互动图像缺少目标回合")
		}
		return turns[len(turns)-1], len(turns) - 1, nil
	}
	for i, turn := range turns {
		if turn.ID == turnID {
			return turn, i, nil
		}
	}
	return interactive.TurnEvent{}, -1, fmt.Errorf("互动图像目标回合不存在: %s", turnID)
}

func shouldGenerateInteractiveImage(settings interactive.StoryImageSettings, turns []interactive.TurnEvent, turnIndex int, source string, force bool) (bool, string) {
	if force {
		return true, ""
	}
	settings = normalizeStoryImageSettingsForApp(settings)
	if source != interactiveImageSourceAuto {
		return true, ""
	}
	switch settings.Mode {
	case interactive.StoryImageModeManual:
		return false, "manual_mode"
	case interactive.StoryImageModeInterval:
		if turnIndex < 0 || turnIndex >= len(turns) {
			return false, "turn_not_found"
		}
		if (turnIndex+1)%settings.IntervalTurns == 0 {
			return true, ""
		}
		return false, "interval"
	default:
		return false, "disabled"
	}
}

func normalizeStoryImageSettingsForApp(settings interactive.StoryImageSettings) interactive.StoryImageSettings {
	if settings.Mode == "every_turn" {
		settings.Mode = interactive.StoryImageModeInterval
		settings.IntervalTurns = 1
	}
	if settings.Mode == "" {
		settings.Mode = interactive.StoryImageModeManual
	}
	if settings.IntervalTurns <= 0 {
		settings.IntervalTurns = 3
	}
	if imagepreset.NormalizeID(settings.PresetID) == "" {
		settings.PresetID = imagepreset.DefaultID
	}
	return settings
}

func interactiveImageDisplayEvent(events []interactive.DisplayEvent) *interactive.DisplayEvent {
	for i := len(events) - 1; i >= 0; i-- {
		if strings.TrimSpace(events[i].Name) == interactiveImageToolName || strings.TrimSpace(events[i].Content) == interactiveImageToolName {
			return &events[i]
		}
	}
	return nil
}

func interactiveImageEventID(turnID string) string {
	return fmt.Sprintf("interactive-image-%s-%d", strings.TrimSpace(turnID), time.Now().UTC().UnixNano())
}

func interactiveImageEventArgs(source string, force bool) string {
	data, _ := json.Marshal(map[string]any{
		"source": source,
		"force":  force,
	})
	return string(data)
}

func interactiveImageErrorResult(err error) string {
	message := "互动图像生成失败"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	data, _ := json.Marshal(map[string]string{
		"schema": "interactive_image_error.v1",
		"error":  message,
	})
	return string(data)
}

func loadImagePreset(novaDir, id string) imagepreset.Preset {
	presetID := imagepreset.NormalizeID(id)
	if presetID == "" {
		presetID = imagepreset.DefaultID
	}
	if strings.TrimSpace(novaDir) == "" {
		return imagepreset.DefaultPreset()
	}
	preset, err := imagepreset.NewLibrary(novaDir).Get(presetID)
	if err != nil {
		log.Printf("[interactive-image] load image preset failed id=%s err=%v; fallback=%s", presetID, err, imagepreset.DefaultID)
		return imagepreset.DefaultPreset()
	}
	return preset
}

func interactiveImageSystemPrompt(preset imagepreset.Preset) string {
	var sb strings.Builder
	sb.WriteString("当前调用点是互动图像。你必须基于已经发生的互动回合生成一张图像，不能透露未来剧情，不能改写叙事正文。")
	if systemPrompt := strings.TrimSpace(preset.PromptForTargets(imagepreset.TargetAgentSystem)); systemPrompt != "" {
		sb.WriteString("\n\n## 图像方案预设\n\n")
		if strings.TrimSpace(preset.ID) != "" {
			fmt.Fprintf(&sb, "- ID：%s\n", limitInteractiveImageRunes(preset.ID, 120))
		}
		if strings.TrimSpace(preset.Name) != "" {
			fmt.Fprintf(&sb, "- 名称：%s\n", limitInteractiveImageRunes(preset.Name, 120))
		}
		sb.WriteString("\n")
		sb.WriteString(limitInteractiveImageRunes(systemPrompt, imagepreset.MaxPromptChars))
	}
	return sb.String()
}

func interactiveImageSourceContext(meta interactive.StoryMeta, turns []interactive.TurnEvent, turnIndex int, store *interactive.Store) string {
	var sb strings.Builder
	writeContextLine(&sb, "故事标题", meta.Title)
	writeContextLine(&sb, "故事来源", meta.Origin)
	writeContextLine(&sb, "叙事方案", meta.StoryTellerID)
	start := turnIndex - 2
	if start < 0 {
		start = 0
	}
	if start < turnIndex {
		sb.WriteString("\n## 前置回合\n\n")
		for i := start; i < turnIndex; i++ {
			fmt.Fprintf(&sb, "### 回合 %d\n用户：%s\n叙事：%s\n\n", i+1, limitInteractiveImageRunes(turns[i].User, 600), limitInteractiveImageRunes(turns[i].Narrative, 1200))
		}
	}
	if turnIndex >= 0 && turnIndex < len(turns) {
		turn := turns[turnIndex]
		sb.WriteString("\n## 当前回合\n\n")
		fmt.Fprintf(&sb, "用户：%s\n\n叙事：%s\n", limitInteractiveImageRunes(turn.User, 800), limitInteractiveImageRunes(turn.Narrative, 2400))
		if store != nil {
			if memory, err := store.StoryMemoryContextSummary(meta.StoryID, turn.BranchID, 4*1024); err == nil && strings.TrimSpace(memory) != "" {
				sb.WriteString("\n## 故事记忆摘要\n\n")
				sb.WriteString(limitInteractiveImageRunes(memory, 2000))
				sb.WriteString("\n")
			} else if err != nil {
				log.Printf("[interactive-image] load story memory failed story_id=%s branch_id=%s err=%v", meta.StoryID, turn.BranchID, err)
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

func writeContextLine(sb *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(sb, "- %s：%s\n", label, limitInteractiveImageRunes(value, 600))
}

func interactiveImageAltText(title string, turnIndex int) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "互动图像"
	}
	if turnIndex >= 0 {
		return fmt.Sprintf("%s 第 %d 轮互动图像", title, turnIndex+1)
	}
	return title + " 互动图像"
}

func limitInteractiveImageRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
