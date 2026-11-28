package modeltask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"

	"denova/config"
	"denova/internal/agents/modelio"
	agentrun "denova/internal/agents/run"

	agentmodel "github.com/alfredxw/denova/agent/model"
	"github.com/alfredxw/denova/agent/model/providers"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

// InlineEditMode identifies the surface owning an inline prose edit request.
type InlineEditMode string

const (
	InlineEditModeWriting  InlineEditMode = "writing"
	InlineEditModeLore     InlineEditMode = "lore"
	InlineEditModeGameTurn InlineEditMode = "game_turn"
)

// Explicit rune budgets protect latency and prompt stability for localized edits.
const (
	InlineEditMaxInstructionChars    = 2000
	InlineEditMaxSelectionChars      = 8000
	InlineEditMaxBeforeChars         = 2000
	InlineEditMaxAfterChars          = 1500
	InlineEditMaxPreviousResultChars = 12000
	InlineEditMaxStyleChars          = 4000
	InlineEditMaxLoreItems           = 6
	InlineEditMaxLoreChars           = 3000
)

var (
	// ErrInlineEditModelNotConfigured reports that no direct language model API
	// configuration is available for stateless inline editing (for example when
	// only an external CLI runtime is configured).
	ErrInlineEditModelNotConfigured = errors.New("inline edit requires a configured direct language model")
	// ErrInlineEditInvalidMode reports an unsupported inline edit mode.
	ErrInlineEditInvalidMode = errors.New("unsupported inline edit mode")
	// ErrInlineEditInstructionRequired reports an empty editing instruction.
	ErrInlineEditInstructionRequired = errors.New("inline edit instruction is required")
	// ErrInlineEditSelectionTooLong reports a selection exceeding the inline budget.
	ErrInlineEditSelectionTooLong = errors.New("inline edit selection exceeds maximum length")
)

// InlineEditLoreContext carries a compact read-only Lore reference matched near
// the active edit range.
type InlineEditLoreContext struct {
	Name        string
	Type        string
	Description string
}

// InlineEditRequest holds the bounded inputs for one stateless inline edit call.
type InlineEditRequest struct {
	Mode           InlineEditMode
	Instruction    string
	Selection      string
	Before         string
	After          string
	PreviousResult string
	StyleGuidance  string
	LoreItems      []InlineEditLoreContext
}

// ValidateInlineEditMode reports whether mode is one of the finite supported modes.
func ValidateInlineEditMode(mode InlineEditMode) bool {
	switch mode {
	case InlineEditModeWriting, InlineEditModeLore, InlineEditModeGameTurn:
		return true
	default:
		return false
	}
}

// ResolveInlineEditModel resolves and validates the direct chat model used for
// inline prose edits so callers can reject unconfigured models before opening
// an SSE response stream.
func ResolveInlineEditModel(cfg *config.Config, agentKind string) (providers.ModelConfig, error) {
	if cfg == nil {
		return providers.ModelConfig{}, ErrInlineEditModelNotConfigured
	}
	modelCfg, err := modelio.ConfigForAgent(cfg, agentKind)
	if err != nil {
		return providers.ModelConfig{}, fmt.Errorf("resolve inline edit model configuration: %w", err)
	}
	if strings.TrimSpace(modelCfg.Model) == "" {
		return providers.ModelConfig{}, ErrInlineEditModelNotConfigured
	}
	if strings.TrimSpace(modelCfg.APIKey) == "" && !isLoopbackBaseURL(modelCfg.BaseURL) {
		return providers.ModelConfig{}, ErrInlineEditModelNotConfigured
	}
	return modelCfg, nil
}

// ValidateInlineEditRequest checks hard input boundaries before streaming.
func ValidateInlineEditRequest(req InlineEditRequest) error {
	if !ValidateInlineEditMode(req.Mode) {
		return ErrInlineEditInvalidMode
	}
	if strings.TrimSpace(req.Instruction) == "" {
		return ErrInlineEditInstructionRequired
	}
	if len([]rune(req.Instruction)) > InlineEditMaxInstructionChars {
		return fmt.Errorf("%w: instruction exceeds %d characters", ErrInlineEditSelectionTooLong, InlineEditMaxInstructionChars)
	}
	if len([]rune(req.Selection)) > InlineEditMaxSelectionChars {
		return fmt.Errorf("%w: selection exceeds %d characters", ErrInlineEditSelectionTooLong, InlineEditMaxSelectionChars)
	}
	if len([]rune(req.PreviousResult)) > InlineEditMaxPreviousResultChars {
		return fmt.Errorf("%w: previous draft exceeds %d characters", ErrInlineEditSelectionTooLong, InlineEditMaxPreviousResultChars)
	}
	return nil
}

// StreamInlineEdit streams one tool-free localized prose edit or insertion.
// The final return value normalizes accidental outer code fences while leaving
// callers free to display streamed deltas incrementally.
func StreamInlineEdit(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
	req InlineEditRequest,
	onDelta func(string) error,
) (string, error) {
	if err := ValidateInlineEditRequest(req); err != nil {
		return "", err
	}
	modelCfg, err := ResolveInlineEditModel(cfg, agentKind)
	if err != nil {
		return "", err
	}

	var runErr error
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, agentKind, "inline_edit", "stream", map[string]any{
		"mode":              string(req.Mode),
		"selection_chars":   len([]rune(req.Selection)),
		"instruction_chars": len([]rune(req.Instruction)),
		"lore_count":        len(req.LoreItems),
		"has_style":         strings.TrimSpace(req.StyleGuidance) != "",
		"is_refinement":     strings.TrimSpace(req.PreviousResult) != "",
	})
	defer func() { finishTrace(runErr) }()

	cm, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("create inline edit model: %w", err)
	}

	// Keep inline edit free of multi-Agent delegation and file-tool runtime
	// contracts so the single-turn prose rewrite stays compact and fast.
	systemPrompt := buildInlineEditSystemPrompt(req)
	userPrompt := buildInlineEditUserPrompt(req)
	messages := []*agentschema.Message{
		agentschema.SystemMessage(systemPrompt),
		agentschema.UserMessage(userPrompt),
	}
	if err := modelio.ValidateConfiguredInput(cfg, agentKind, messages, nil); err != nil {
		runErr = err
		return "", err
	}

	slog.InfoContext(ctx, fmt.Sprintf(
		"[inline-edit] stream begin agent=%s mode=%s model=%q selection_chars=%d lore_count=%d refinement=%t",
		agentKind, req.Mode, modelCfg.Model, len([]rune(req.Selection)), len(req.LoreItems), strings.TrimSpace(req.PreviousResult) != "",
	))

	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, agentKind, "inline_edit", "stream", modelCfg, messages, nil, true)
	reader, err := cm.Stream(llmTraceCtx, messages, agentmodel.WithoutTools())
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, agentKind, "inline_edit", "stream", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("start inline edit stream: %w", err)
	}
	defer reader.Close()

	var raw strings.Builder
	var lastUsage *agentschema.TokenUsage
	for {
		chunk, recvErr := reader.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			agentrun.FinishLLMCallTrace(span, callID, agentKind, "inline_edit", "stream", modelCfg.Model, 0, nil, recvErr, nil)
			runErr = recvErr
			return "", fmt.Errorf("read inline edit stream: %w", recvErr)
		}
		if chunk == nil {
			continue
		}
		if chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil {
			lastUsage = chunk.ResponseMeta.Usage
		}
		if chunk.Content == "" {
			continue
		}
		raw.WriteString(chunk.Content)
		if onDelta != nil {
			if err := onDelta(chunk.Content); err != nil {
				agentrun.FinishLLMCallTrace(span, callID, agentKind, "inline_edit", "stream", modelCfg.Model, 0, nil, err, nil)
				runErr = err
				return "", err
			}
		}
	}

	result := normalizeInlineEditOutput(req.Selection, raw.String())
	finalMsg := agentschema.AssistantMessage(result, nil)
	if lastUsage != nil {
		finalMsg.ResponseMeta = &agentschema.ResponseMeta{Usage: lastUsage}
	}
	agentrun.FinishLLMCallTrace(span, callID, agentKind, "inline_edit", "stream", modelCfg.Model, 0, finalMsg, nil, nil)
	slog.InfoContext(ctx, fmt.Sprintf(
		"[inline-edit] stream done agent=%s mode=%s output_chars=%d",
		agentKind, req.Mode, len([]rune(result)),
	))
	return result, nil
}

func buildInlineEditSystemPrompt(req InlineEditRequest) string {
	var b strings.Builder
	b.WriteString("You are Denova's inline prose editor. Perform one surgical localized text edit without calling tools.\n")
	b.WriteString("Core rules:\n")
	b.WriteString("- Output ONLY the replacement or inserted prose. Never add explanations, greetings, labels, XML tags, or commentary.\n")
	b.WriteString("- Do not wrap your output in Markdown code fences unless the target selection itself uses code fences.\n")
	b.WriteString("- Match the language, point of view, tense, punctuation habits, and Markdown formatting of the surrounding text.\n")
	b.WriteString("- Keep names, facts, and relationships faithful to any provided lore references and surrounding context.\n")
	b.WriteString("- Do not repeat text that already appears immediately before or after the edit range.\n")

	switch req.Mode {
	case InlineEditModeWriting:
		b.WriteString("- Surface: novel manuscript editor. Preserve literary flow and paragraph rhythm.\n")
	case InlineEditModeLore:
		b.WriteString("- Surface: structured lore and worldbuilding entry. Keep setting details clear, concrete, and concise, preserving any [[reference]] links when relevant.\n")
	case InlineEditModeGameTurn:
		b.WriteString("- Surface: interactive story turn prose revision. Preserve immersive narrative voice and continuity; do not emit state-ledger markers, metadata blocks, or numbered choice lists unless already part of the target selection.\n")
	}

	if req.Selection == "" {
		b.WriteString("- Operation: insert new prose directly at the cursor position between <before_context> and <after_context>.\n")
	} else {
		b.WriteString("- Operation: rewrite <target_selection> so it fits seamlessly between <before_context> and <after_context>.\n")
	}
	if strings.TrimSpace(req.PreviousResult) != "" {
		b.WriteString("- Refinement: revise <previous_draft> according to the new <instruction> while keeping it aligned with the surrounding context.\n")
	}

	if style := clipHeadRunes(strings.TrimSpace(req.StyleGuidance), InlineEditMaxStyleChars); style != "" {
		b.WriteString("\n<narrative_style>\n")
		b.WriteString(style)
		b.WriteString("\n</narrative_style>\n")
	}
	return strings.TrimSpace(b.String())
}

func buildInlineEditUserPrompt(req InlineEditRequest) string {
	var b strings.Builder

	if loreBlock := formatInlineEditLore(req.LoreItems); loreBlock != "" {
		b.WriteString("<lore_context>\n")
		b.WriteString(loreBlock)
		b.WriteString("\n</lore_context>\n\n")
	}

	if before := clipTailRunes(req.Before, InlineEditMaxBeforeChars); strings.TrimSpace(before) != "" {
		b.WriteString("<before_context>\n")
		b.WriteString(before)
		b.WriteString("\n</before_context>\n\n")
	}

	if req.Selection != "" {
		b.WriteString("<target_selection>\n")
		b.WriteString(req.Selection)
		b.WriteString("\n</target_selection>\n\n")
	}

	if after := clipHeadRunes(req.After, InlineEditMaxAfterChars); strings.TrimSpace(after) != "" {
		b.WriteString("<after_context>\n")
		b.WriteString(after)
		b.WriteString("\n</after_context>\n\n")
	}

	if prev := strings.TrimSpace(req.PreviousResult); prev != "" {
		b.WriteString("<previous_draft>\n")
		b.WriteString(prev)
		b.WriteString("\n</previous_draft>\n\n")
	}

	b.WriteString("<instruction>\n")
	b.WriteString(strings.TrimSpace(req.Instruction))
	b.WriteString("\n</instruction>")
	return b.String()
}

func formatInlineEditLore(items []InlineEditLoreContext) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	remaining := InlineEditMaxLoreChars
	count := 0
	for _, item := range items {
		if count >= InlineEditMaxLoreItems || remaining <= 0 {
			break
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		var line strings.Builder
		line.WriteString("- ")
		line.WriteString(name)
		if itemType := strings.TrimSpace(item.Type); itemType != "" {
			line.WriteString(" (")
			line.WriteString(itemType)
			line.WriteString(")")
		}
		if desc := strings.TrimSpace(item.Description); desc != "" {
			line.WriteString(": ")
			line.WriteString(clipHeadRunes(desc, 360))
		}
		entry := line.String()
		entryRunes := len([]rune(entry))
		if entryRunes > remaining {
			if count == 0 {
				entry = clipHeadRunes(entry, remaining)
			} else {
				break
			}
		}
		if count > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(entry)
		remaining -= len([]rune(entry)) + 1
		count++
	}
	return b.String()
}

// normalizeInlineEditOutput removes accidental single outer Markdown fences or
// echoed XML wrapper tags if a model adds them despite instructions.
func normalizeInlineEditOutput(selection, output string) string {
	normalized := strings.ReplaceAll(output, "\r\n", "\n")
	trimmed := strings.TrimSpace(normalized)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(selection, "```") && strings.HasPrefix(trimmed, "```") && strings.HasSuffix(trimmed, "```") {
		if firstNewline := strings.IndexByte(trimmed, '\n'); firstNewline >= 0 {
			body := trimmed[firstNewline+1:]
			if strings.HasSuffix(body, "```") {
				candidate := strings.TrimSuffix(body, "```")
				if !strings.Contains(candidate, "```") {
					trimmed = strings.Trim(candidate, "\n")
				}
			}
		}
	}
	for _, tag := range []string{"target_selection", "previous_draft", "replacement", "output"} {
		openTag := "<" + tag + ">"
		closeTag := "</" + tag + ">"
		if strings.HasPrefix(trimmed, openTag) && strings.HasSuffix(trimmed, closeTag) {
			trimmed = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, openTag), closeTag))
		}
	}
	return trimmed
}

func clipHeadRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func clipTailRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[len(runes)-maxRunes:])
}

func isLoopbackBaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0"
}
