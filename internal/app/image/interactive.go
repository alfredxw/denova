package imageapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	apptask "denova/internal/app/task"
	imageasset "denova/internal/image/asset"
	imagepreset "denova/internal/image/preset"
	"denova/internal/interactive"
)

const (
	interactiveImageToolName     = "generate_interactive_image"
	interactiveImageSkill        = "interactive-image"
	interactiveImageSourceAuto   = "auto"
	interactiveImageSourceManual = "manual"
)

type InteractiveGenerateResult struct {
	Enabled       bool                          `json:"enabled"`
	Skipped       bool                          `json:"skipped,omitempty"`
	SkippedReason string                        `json:"skipped_reason,omitempty"`
	Image         *imageasset.InteractiveResult `json:"image,omitempty"`
}

func (service *Service) GenerateInteractiveImage(ctx context.Context, storyID string, req interactive.InteractiveImageGenerateRequest) (InteractiveGenerateResult, error) {
	req.CommandID = strings.TrimSpace(req.CommandID)
	if err := validateAgentCommandID(req.CommandID); err != nil {
		return InteractiveGenerateResult{}, err
	}
	runtime, err := service.AcquireRuntime(ctx, "")
	if err != nil {
		return InteractiveGenerateResult{}, err
	}
	defer runtime.Release()
	store := runtime.Interactive
	if store == nil {
		return InteractiveGenerateResult{}, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return InteractiveGenerateResult{}, err
	}
	turn, turnIndex, err := interactiveImageTargetTurn(storyCtx.Snapshot.Turns, req.TurnID)
	if err != nil {
		return InteractiveGenerateResult{}, err
	}
	source := normalizeInteractiveImageSource(req.Source)
	eventID := interactiveImageEventID(req.CommandID)
	if image, settled, projectionErr := interactiveImageCommandProjection(turn.DisplayEvents, eventID); settled {
		if projectionErr != nil {
			return InteractiveGenerateResult{}, projectionErr
		}
		return InteractiveGenerateResult{Enabled: true, Image: image}, nil
	}
	should, reason := shouldGenerateInteractiveImage(storyCtx.Meta.ImageSettings, storyCtx.Snapshot.Turns, turnIndex, source, req.Force)
	if !should {
		return InteractiveGenerateResult{Enabled: storyCtx.Meta.ImageSettings.Mode != interactive.StoryImageModeManual, Skipped: true, SkippedReason: reason}, nil
	}
	if existing := interactiveImageDisplayEvent(turn.DisplayEvents); existing != nil && !req.Force {
		return InteractiveGenerateResult{Enabled: true, Skipped: true, SkippedReason: "already_exists"}, nil
	}

	preset := loadImagePreset(runtime.Config.DataDir(), storyCtx.Meta.ImageSettings.PresetID)
	sourceContext := interactiveImageSourceContext(storyCtx.Meta, storyCtx.Snapshot.Turns, turnIndex)
	systemPrompt := interactiveImageSystemPrompt(preset)
	toolPrompt := preset.PromptForTargets(imagepreset.TargetToolRequest)
	result, err := service.generateWithAgentUsingHooks(runtime, AgentGenerateRequest{
		CommandID:     req.CommandID,
		Purpose:       "interactive_image",
		SourceContext: sourceContext,
		SystemPrompt:  systemPrompt,
		ToolPrompt:    toolPrompt,
		SkillName:     interactiveImageSkill,
		StoryID:       storyID,
		BranchID:      storyCtx.Snapshot.BranchID,
		TurnID:        turn.ID,
		AltText:       interactiveImageAltText(storyCtx.Meta.Title, turnIndex),
	}, imageAgentRunHooks{
		OnAccepted: func() error {
			return store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
				ID: eventID, Role: "tool_call", Content: interactiveImageToolName, Name: interactiveImageToolName,
				Status: "running", Args: interactiveImageEventArgs(req.CommandID, source, req.Force),
				ToolPresentation: interactiveImageToolPresentation(),
			})
		},
		OnInteractiveImage: func(image *imageasset.InteractiveResult) error {
			return appendInteractiveImageSuccess(store, storyID, storyCtx.Snapshot.BranchID, turn.ID, eventID, req.CommandID, source, req.Force, image)
		},
	})
	if err != nil {
		if errors.Is(err, apptask.ErrCommandConflict) {
			return InteractiveGenerateResult{}, err
		}
		resultErr := err
		if runtime.Context().Err() != nil {
			resultErr = runtime.Context().Err()
		}
		_ = store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
			ID:               eventID,
			Role:             "tool_call",
			Content:          interactiveImageToolName,
			Name:             interactiveImageToolName,
			Status:           "error",
			Args:             interactiveImageEventArgs(req.CommandID, source, req.Force),
			Result:           interactiveImageErrorResult(resultErr),
			ToolPresentation: interactiveImageToolPresentation(),
		})
		return InteractiveGenerateResult{}, fmt.Errorf("%w: %v", ErrExecution, resultErr)
	}
	if result.InteractiveImage == nil {
		err := fmt.Errorf("image Agent returned no interactive image")
		_ = store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
			ID:               eventID,
			Role:             "tool_call",
			Content:          interactiveImageToolName,
			Name:             interactiveImageToolName,
			Status:           "error",
			Args:             interactiveImageEventArgs(req.CommandID, source, req.Force),
			Result:           interactiveImageErrorResult(err),
			ToolPresentation: interactiveImageToolPresentation(),
		})
		return InteractiveGenerateResult{}, fmt.Errorf("%w: %v", ErrExecution, err)
	}
	if err := runtime.Context().Err(); err != nil {
		return InteractiveGenerateResult{}, fmt.Errorf("%w: %v", ErrExecution, err)
	}
	if err := appendInteractiveImageSuccess(store, storyID, storyCtx.Snapshot.BranchID, turn.ID, eventID, req.CommandID, source, req.Force, result.InteractiveImage); err != nil {
		return InteractiveGenerateResult{}, fmt.Errorf("%w: %v", ErrExecution, err)
	}
	return InteractiveGenerateResult{Enabled: true, Image: result.InteractiveImage}, nil
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
			return interactive.TurnEvent{}, -1, fmt.Errorf("interactive image target turn is required")
		}
		return turns[len(turns)-1], len(turns) - 1, nil
	}
	for i, turn := range turns {
		if turn.ID == turnID {
			return turn, i, nil
		}
	}
	return interactive.TurnEvent{}, -1, fmt.Errorf("interactive image target turn does not exist: %s", turnID)
}

func shouldGenerateInteractiveImage(settings interactive.StoryImageSettings, turns []interactive.TurnEvent, turnIndex int, source string, force bool) (bool, string) {
	if force {
		return true, ""
	}
	settings = normalizeStoryImageSettings(settings)
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

func normalizeStoryImageSettings(settings interactive.StoryImageSettings) interactive.StoryImageSettings {
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

func interactiveImageDisplayEventByID(events []interactive.DisplayEvent, eventID string) *interactive.DisplayEvent {
	eventID = strings.TrimSpace(eventID)
	for i := len(events) - 1; i >= 0; i-- {
		if strings.TrimSpace(events[i].ID) == eventID && strings.TrimSpace(events[i].Role) == "tool_call" {
			return &events[i]
		}
	}
	return nil
}

func interactiveImageCommandProjection(events []interactive.DisplayEvent, eventID string) (*imageasset.InteractiveResult, bool, error) {
	event := interactiveImageDisplayEventByID(events, eventID)
	if event == nil {
		return nil, false, nil
	}
	switch strings.TrimSpace(event.Status) {
	case "success":
		var image imageasset.InteractiveResult
		if err := json.Unmarshal([]byte(event.Result), &image); err != nil {
			return nil, true, fmt.Errorf("decode interactive image display result: %w", err)
		}
		if strings.TrimSpace(image.ImagePath) == "" {
			return nil, true, fmt.Errorf("interactive image display result has no image path")
		}
		return &image, true, nil
	case "error":
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal([]byte(event.Result), &payload)
		return nil, true, errors.New(firstNonEmpty(strings.TrimSpace(payload.Error), "interactive image generation failed"))
	default:
		return nil, false, nil
	}
}

func appendInteractiveImageSuccess(
	store *interactive.Store,
	storyID, branchID, turnID, eventID, commandID, source string,
	force bool,
	image *imageasset.InteractiveResult,
) error {
	if image == nil {
		return fmt.Errorf("image Agent returned no interactive image")
	}
	data, err := json.Marshal(image)
	if err != nil {
		return err
	}
	return store.AppendTurnDisplayEvent(storyID, branchID, turnID, interactive.DisplayEvent{
		ID: eventID, Role: "tool_call", Content: interactiveImageToolName, Name: interactiveImageToolName,
		Status: "success", Args: interactiveImageEventArgs(commandID, source, force), Result: string(data),
		ToolPresentation: interactiveImageToolPresentation(),
	})
}

func interactiveImageToolPresentation() *agent.ToolPresentation {
	presentation := agent.UniformToolPresentation(agent.ToolPresentationInteractiveMedia)
	return &presentation
}

func interactiveImageEventID(commandID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(commandID)))
	return fmt.Sprintf("interactive-image-command-%x", digest[:16])
}

func interactiveImageEventArgs(commandID, source string, force bool) string {
	data, _ := json.Marshal(map[string]any{
		"command_id": strings.TrimSpace(commandID),
		"source":     source,
		"force":      force,
	})
	return string(data)
}

func interactiveImageErrorResult(err error) string {
	message := "interactive image generation failed"
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
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-image] load image preset failed id=%s err=%v; fallback=%s", presetID, err, imagepreset.DefaultID))
		return imagepreset.DefaultPreset()
	}
	return preset
}

func interactiveImageSystemPrompt(preset imagepreset.Preset) string {
	var sb strings.Builder
	sb.WriteString("This request is for an interactive-story image. Generate exactly one image from events already established in the interactive turns. Do not reveal future plot or rewrite the narrative prose.")
	if systemPrompt := strings.TrimSpace(preset.PromptForTargets(imagepreset.TargetAgentSystem)); systemPrompt != "" {
		sb.WriteString("\n\n## Image Preset\n\n")
		if strings.TrimSpace(preset.ID) != "" {
			fmt.Fprintf(&sb, "- ID: %s\n", limitInteractiveImageRunes(preset.ID, 120))
		}
		if strings.TrimSpace(preset.Name) != "" {
			fmt.Fprintf(&sb, "- Name: %s\n", limitInteractiveImageRunes(preset.Name, 120))
		}
		sb.WriteString("\n")
		sb.WriteString(limitInteractiveImageRunes(systemPrompt, imagepreset.MaxPromptChars))
	}
	return sb.String()
}

func interactiveImageSourceContext(meta interactive.StoryMeta, turns []interactive.TurnEvent, turnIndex int) string {
	var sb strings.Builder
	writeContextLine(&sb, "Story title", meta.Title)
	writeContextLine(&sb, "Story origin", meta.Origin)
	writeContextLine(&sb, "Narrative style", meta.StoryTellerID)
	start := turnIndex - 2
	if start < 0 {
		start = 0
	}
	if start < turnIndex {
		sb.WriteString("\n## Preceding Turns\n\n")
		for i := start; i < turnIndex; i++ {
			fmt.Fprintf(&sb, "### Turn %d\nUser: %s\nNarrative: %s\n\n", i+1, limitInteractiveImageRunes(turns[i].User, 600), limitInteractiveImageRunes(turns[i].Narrative, 1200))
		}
	}
	if turnIndex >= 0 && turnIndex < len(turns) {
		turn := turns[turnIndex]
		sb.WriteString("\n## Current Turn\n\n")
		fmt.Fprintf(&sb, "User: %s\n\nNarrative: %s\n", limitInteractiveImageRunes(turn.User, 800), limitInteractiveImageRunes(turn.Narrative, 2400))
	}
	return strings.TrimSpace(sb.String())
}

func writeContextLine(sb *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(sb, "- %s: %s\n", label, limitInteractiveImageRunes(value, 600))
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
