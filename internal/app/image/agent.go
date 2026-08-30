package imageapp

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"
	agenttools "denova/internal/agents/tools"
	apptask "denova/internal/app/task"
	imageasset "denova/internal/image/asset"
)

type AgentGenerateRequest struct {
	CommandID     string
	CustomAgentID string
	Purpose       string
	SourceContext string
	SystemPrompt  string
	ToolPrompt    string
	SkillName     string
	StoryID       string
	BranchID      string
	TurnID        string
	AltText       string
	LoreItemID    string
	ImagePresetID string
}

type AgentGenerateResult struct {
	AssistantText    string
	InteractiveImage *imageasset.InteractiveResult
	BookCover        *imageasset.CoverResult
}

type imageAgentRunHooks struct {
	OnAccepted         func() error
	OnInteractiveImage func(*imageasset.InteractiveResult) error
}

func (service *Service) GenerateWithAgent(ctx context.Context, req AgentGenerateRequest) (AgentGenerateResult, error) {
	if err := validateAgentCommandID(req.CommandID); err != nil {
		return AgentGenerateResult{}, err
	}
	runtime, err := service.AcquireRuntime(ctx, "")
	if err != nil {
		return AgentGenerateResult{}, err
	}
	defer runtime.Release()
	return service.generateWithAgent(runtime, req)
}

// GenerateProjectWithAgent runs one Image Agent request against an explicit
// Project without depending on the foreground writing workspace.
func (service *Service) GenerateProjectWithAgent(ctx context.Context, projectID string, req AgentGenerateRequest) (AgentGenerateResult, error) {
	if err := validateAgentCommandID(req.CommandID); err != nil {
		return AgentGenerateResult{}, err
	}
	runtime, err := service.AcquireProjectRuntime(ctx, projectID)
	if err != nil {
		return AgentGenerateResult{}, err
	}
	defer runtime.Release()
	return service.generateWithAgent(runtime, req)
}

// generateWithAgent executes inside a caller-owned workspace operation. The
// interactive-image flow deliberately reuses this core so its story reads,
// display events, Agent run, asset writes, and journal append remain one
// admitted composite even after the workspace scope starts draining.
func (service *Service) generateWithAgent(runtime *Runtime, req AgentGenerateRequest) (AgentGenerateResult, error) {
	return service.generateWithAgentUsingHooks(runtime, req, imageAgentRunHooks{})
}

func (service *Service) generateWithAgentUsingHooks(runtime *Runtime, req AgentGenerateRequest, hooks imageAgentRunHooks) (AgentGenerateResult, error) {
	req.CommandID = strings.TrimSpace(req.CommandID)
	if err := validateAgentCommandID(req.CommandID); err != nil {
		return AgentGenerateResult{}, err
	}
	if err := runtime.requireAgentAdapters(); err != nil {
		return AgentGenerateResult{}, err
	}
	cycle, conversation, err := service.prepareAgentCycle(runtime, req)
	if err != nil {
		return AgentGenerateResult{}, err
	}
	var result AgentGenerateResult
	var runErr error
	var hookErr error
	runCtx, cancelRun := context.WithCancel(runtime.Context())
	defer cancelRun()
	accepted, err := runtime.ExecutionRuntime.Start(runCtx, agentexecution.StartRequest{
		Cycle: cycle,
		Emit: func(ev agentrun.Event) {
			switch ev.Type {
			case "tool_result":
				if image := eventInteractiveImage(ev.Data); image != nil {
					result.InteractiveImage = image
					if hooks.OnInteractiveImage != nil && hookErr == nil {
						hookErr = hooks.OnInteractiveImage(image)
					}
				}
				if cover := eventBookCover(ev.Data); cover != nil {
					result.BookCover = cover
				}
			case "error":
				if runErr == nil {
					runErr = errors.New(eventErrorMessage(ev.Data))
				}
			}
		},
	})
	if err != nil {
		if errors.Is(err, agentrun.ErrInvalidCommand) {
			return result, fmt.Errorf("%w: command_id=%q", apptask.ErrCommandConflict, req.CommandID)
		}
		return result, err
	}
	if hooks.OnAccepted != nil {
		if err := hooks.OnAccepted(); err != nil {
			cancelRun()
			_ = accepted.Wait(runCtx)
			return result, err
		}
	}
	outcome := accepted.Wait(runCtx)
	result.AssistantText = strings.TrimSpace(conversation.assistant)
	if result.AssistantText == "" {
		result.AssistantText = strings.TrimSpace(outcome.Content)
	}
	if hookErr != nil {
		return result, hookErr
	}
	if runErr != nil {
		return result, runErr
	}
	if outcome.Status != agentrun.OutcomeCompleted {
		if outcome.Error != nil {
			return result, outcome.Error
		}
		return result, fmt.Errorf("image Agent did not complete: status=%s reason=%s", outcome.Status, strings.TrimSpace(outcome.Reason))
	}
	if err := runtime.Context().Err(); err != nil {
		return result, err
	}
	if strings.TrimSpace(req.Purpose) == "interactive_image" && result.InteractiveImage == nil {
		return result, fmt.Errorf("image Agent did not generate an interactive image")
	}
	if strings.TrimSpace(req.Purpose) == "book_cover" && result.BookCover == nil {
		return result, fmt.Errorf("image Agent did not generate a book cover")
	}
	if result.InteractiveImage != nil {
		slog.InfoContext(context.Background(), fmt.Sprintf("[image-agent] generated interactive image workspace=%s story_id=%s branch_id=%s turn_id=%s path=%s", runtime.Workspace, result.InteractiveImage.StoryID, result.InteractiveImage.BranchID, result.InteractiveImage.TurnID, result.InteractiveImage.ImagePath))
	} else {
		slog.InfoContext(context.Background(), fmt.Sprintf("[image-agent] completed image request workspace=%s purpose=%s", runtime.Workspace, strings.TrimSpace(req.Purpose)))
	}
	return result, nil
}

// prepareImageAgentSession establishes the canonical journal before the
// durable harness can accept or recover an input bound to it.
func prepareImageAgentSession(store *session.Store, customAgentID string) (*session.Session, error) {
	sess, err := session.AgentInstanceSession(store, config.AgentKindImage, customAgentID)
	if err != nil {
		return nil, fmt.Errorf("initialize Image Agent session: %w", err)
	}
	return sess, nil
}

func validateAgentCommandID(commandID string) error {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return apptask.ErrCommandIDRequired
	}
	return agentrun.ValidateCommandID(commandID)
}

type imageAgentConversation struct {
	journal       *agentconversation.SessionConversation
	message       string
	sourceContext string
	sourceSummary string
	assistant     string
	contextBudget agentcontext.Budget
	skillConfig   config.Config
}

type imageAgentContextCommitState struct {
	conversation *imageAgentConversation
	messageHash  string
}

func (c *imageAgentConversation) ModelContextBudget() agentcontext.Budget {
	if c == nil {
		return agentcontext.DefaultBudget()
	}
	return c.contextBudget
}

func (c *imageAgentConversation) ResolveExplicitSkills(ctx context.Context, message string) ([]novaskills.Invocation, error) {
	if c == nil {
		return nil, nil
	}
	return novaskills.ResolveConfiguredInvocations(ctx, &c.skillConfig, config.AgentKindImage, message)
}

func (c *imageAgentConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	if strings.TrimSpace(c.message) == "" {
		return agentcontext.ModelContextResult{}, fmt.Errorf("image Agent input is empty")
	}
	fragments := append([]agentcontext.Fragment(nil), input.Fragments...)
	if c.sourceContext != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "image_request_source_context", Source: "image.request.source_context", Title: "Image Generation Source Context",
			Purpose: "ground the generated image in the bounded source material selected for this request",
			Content: c.sourceContext, Placement: agentcontext.PlacementFinalUserPrefix, Included: true,
			Note: "source=image generation request; turn-scoped",
		})
	}
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{
		Messages: []*agents.Message{agents.UserMessage(c.message)}, Fragments: fragments,
	})
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	return agentcontext.ModelContextResult{
		Messages: assembled.Messages,
		Context:  assembled,
		CommitState: imageAgentContextCommitState{
			conversation: c,
			messageHash:  imageAgentSemanticHash(c.message),
		},
	}, nil
}

func (c *imageAgentConversation) AppendAssistant(content string) error {
	c.assistant = strings.TrimSpace(content)
	if c.journal == nil {
		return fmt.Errorf("Image Agent journal is unavailable")
	}
	return c.journal.AppendAssistant(content)
}

func (c *imageAgentConversation) AppendAssistantWithMetadata(content, thinking string, metadata session.MessageMetadata) error {
	c.assistant = strings.TrimSpace(content)
	if c.journal == nil {
		return fmt.Errorf("Image Agent journal is unavailable")
	}
	return c.journal.AppendAssistantWithMetadata(content, thinking, metadata)
}

func (c *imageAgentConversation) BindAgentCycleIdentity(identity agentrun.CycleIdentity) {
	if c != nil && c.journal != nil {
		c.journal.BindAgentCycleIdentity(identity)
	}
}

func (c *imageAgentConversation) BindAgentKind(agentKind string) {
	if c != nil && c.journal != nil {
		c.journal.BindAgentKind(agentKind)
	}
}

func (c *imageAgentConversation) PendingAgentCycleCommit(stage agentrun.DomainCommitStage) (agentrun.DomainCommitIntent, bool, error) {
	if c == nil || c.journal == nil {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	return c.journal.PendingAgentCycleCommit(stage)
}

func (c *imageAgentConversation) CommitAgentCycleStage(ctx context.Context, stage agentrun.DomainCommitStage, outcome agentrun.Outcome) error {
	if c == nil || c.journal == nil {
		return nil
	}
	return c.journal.CommitAgentCycleStage(ctx, stage, outcome)
}

func (c *imageAgentConversation) LastAgentCycleCommitReceipt(stage agentrun.DomainCommitStage) (agentrun.DomainCommitReceipt, bool) {
	if c == nil || c.journal == nil {
		return agentrun.DomainCommitReceipt{}, false
	}
	return c.journal.LastAgentCycleCommitReceipt(stage)
}

func (c *imageAgentConversation) MarkInterrupted(_, _, _ string) error       { return nil }
func (c *imageAgentConversation) PendingInterruption() *session.Interruption { return nil }
func (c *imageAgentConversation) ResolveInterruption(string) error           { return nil }
func (c *imageAgentConversation) ContextSourceSummary() string               { return c.sourceSummary }

func imageAgentMessage(req AgentGenerateRequest) string {
	var sb strings.Builder
	if skill := strings.TrimSpace(req.SkillName); skill != "" {
		sb.WriteString("/")
		sb.WriteString(skill)
		sb.WriteString("\n\n")
	}
	sb.WriteString("# Image Generation Request\n\n")
	writeImageAgentField(&sb, "purpose", req.Purpose)
	writeImageAgentField(&sb, "story_id", req.StoryID)
	writeImageAgentField(&sb, "branch_id", req.BranchID)
	writeImageAgentField(&sb, "turn_id", req.TurnID)
	writeImageAgentField(&sb, "alt_text", req.AltText)
	writeImageAgentField(&sb, "lore_item_id", req.LoreItemID)
	writeImageAgentField(&sb, "source_context_sha256", imageAgentSemanticHash(req.SourceContext))
	writeImageAgentField(&sb, "system_prompt_sha256", imageAgentSemanticHash(req.SystemPrompt))
	writeImageAgentField(&sb, "tool_prompt_sha256", imageAgentSemanticHash(req.ToolPrompt))
	sb.WriteString("\nLoad the required Skill, then call generate_image to complete the request.")
	return strings.TrimSpace(sb.String())
}

func eventBookCover(data interface{}) *imageasset.CoverResult {
	payload, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	toolName, _ := payload["name"].(string)
	content, _ := payload["content"].(string)
	cover, err := agenttools.ParseBookCoverResult(toolName, content)
	if err != nil {
		slog.WarnContext(context.Background(), "[image-agent] decode book cover ToolResult failed",
			"tool", strings.TrimSpace(toolName), "error", err)
		return nil
	}
	return cover
}

func imageAgentSemanticHash(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return fmt.Sprintf("%x", digest[:])
}

func writeImageAgentField(sb *strings.Builder, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	sb.WriteString("- ")
	sb.WriteString(key)
	sb.WriteString(": ")
	sb.WriteString(value)
	sb.WriteString("\n")
}

func imageAgentSourceSummary(req AgentGenerateRequest) string {
	var parts []string
	if strings.TrimSpace(req.Purpose) != "" {
		parts = append(parts, "purpose="+strings.TrimSpace(req.Purpose))
	}
	if strings.TrimSpace(req.SkillName) != "" {
		parts = append(parts, "skill="+strings.TrimSpace(req.SkillName))
	}
	if strings.TrimSpace(req.SourceContext) != "" {
		parts = append(parts, fmt.Sprintf("source_context_chars=%d", len([]rune(req.SourceContext))))
	}
	return strings.Join(parts, " ")
}

func eventInteractiveImage(data interface{}) *imageasset.InteractiveResult {
	payload, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	switch value := payload["interactive_image"].(type) {
	case *imageasset.InteractiveResult:
		return value
	case imageasset.InteractiveResult:
		return &value
	}
	// Protected, non-idempotent ToolResults always retain their bounded content,
	// but the optional product display projection may be absent after a runtime
	// boundary or replay. Decode the authoritative bounded result as a fallback
	// so a generated asset cannot be mistaken for a failed generation.
	toolName, _ := payload["name"].(string)
	content, _ := payload["content"].(string)
	image, err := agenttools.ParseInteractiveImageResult(toolName, content)
	if err != nil {
		slog.WarnContext(context.Background(), "[image-agent] decode interactive image ToolResult failed",
			"tool", strings.TrimSpace(toolName), "error", err)
		return nil
	}
	return image
}

func eventErrorMessage(data interface{}) string {
	payload, ok := data.(map[string]string)
	if ok {
		return firstNonEmpty(payload["message"], payload["error"], "image Agent execution failed")
	}
	if generic, ok := data.(map[string]interface{}); ok {
		message, _ := generic["message"].(string)
		errorText, _ := generic["error"].(string)
		return firstNonEmpty(message, errorText, "image Agent execution failed")
	}
	return "image Agent execution failed"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
