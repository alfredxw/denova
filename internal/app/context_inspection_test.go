package app

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
	interactivestate "denova/internal/interactive/state"
)

type writingInspectionCleanupManager struct {
	identity    agent.CapabilityIdentity
	marker      string
	placeholder string
}

func (manager writingInspectionCleanupManager) Identity() agent.CapabilityIdentity {
	return manager.identity
}

func (manager writingInspectionCleanupManager) Plan(_ context.Context, request agent.CleanupPlanRequest) (agent.CleanupPlan, error) {
	for index, message := range request.ModelRequest {
		if message == nil || message.Role != agent.ToolRole || !strings.Contains(message.Content, manager.marker) {
			continue
		}
		return agent.CleanupPlan{
			Action: agent.CleanupProject, Reason: "verify exact Writing inspection", Renderer: "test.writing.inspection.cleanup.v1",
			Replacements: []agent.CleanupReplacement{{
				MessageIndex: index, ToolCallID: message.ToolCallID, Placeholder: manager.placeholder,
			}},
		}, nil
	}
	return agent.CleanupPlan{Action: agent.CleanupNone, Reason: "no matching tool result"}, nil
}

type writingInspectionCompactionManager struct {
	identity agent.CapabilityIdentity
	summary  string
}

func (manager writingInspectionCompactionManager) Identity() agent.CapabilityIdentity {
	return manager.identity
}

func (writingInspectionCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (writingInspectionCompactionManager) Plan(_ context.Context, request agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	if !request.Force || len(request.Messages) < 4 {
		return agent.CompactionPlan{Action: agent.CompactionNone}, nil
	}
	return agent.CompactionPlan{
		Action: agent.CompactionCreate, SourceFrom: 0, SourceTo: len(request.Messages) - 2,
		Validation: agent.CompactionValidationPolicy{HardLimitBytes: 8 << 20},
	}, nil
}

func (manager writingInspectionCompactionManager) Compact(context.Context, agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	summary := strings.TrimSpace(manager.summary)
	if summary == "" {
		summary = "EXACT_WRITING_PUBLIC_CHECKPOINT"
	}
	return agent.CompactionCheckpoint{Summary: summary, TokenEstimate: 8}, nil
}

type writingInspectionCaptureModel struct {
	mu    sync.Mutex
	input []*agent.Message
}

func (model *writingInspectionCaptureModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return model.capture(input), nil
}

func (model *writingInspectionCaptureModel) Stream(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{model.capture(input)}), nil
}

func (model *writingInspectionCaptureModel) capture(input []*agent.Message) *agent.Message {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.input = make([]*agent.Message, len(input))
	for index, message := range input {
		if message != nil {
			model.input[index] = message.Clone()
		}
	}
	return agent.AssistantMessage("Maintenance prepared.", nil)
}

func TestWritingAnalyzeContextUsesExactPublicInspectionAndPreservesRawToolHistory(t *testing.T) {
	disabled := false
	root := t.TempDir()
	if err := config.WriteSettingsFile(config.UserConfigPath(root), config.Settings{
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "inspection-model",
		AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
			ToolResultContextEnabled: &disabled,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	application, err := New(context.Background(), &config.Config{
		NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "inspection-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	application.mu.RLock()
	sess := application.session
	application.mu.RUnlock()
	if sess == nil {
		t.Fatal("Writing Session is unavailable")
	}
	for _, message := range []*agent.Message{
		agent.UserMessage("PRIOR_USER_CONTEXT"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-secret", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"secret.md"}`},
		}}),
		agent.ToolMessage(agent.ToolResult{Status: agent.ToolResultSuccess, ModelContent: "SECRET_TOOL_BODY"}, "call-secret", agent.WithToolName("read")),
		agent.AssistantMessage("PRIOR_ASSISTANT_CONTEXT", nil),
	} {
		if err := sess.Append(message); err != nil {
			t.Fatal(err)
		}
	}
	before := sess.GetMessages()
	analysis, err := application.AnalyzeContext(context.Background(), agentchat.ChatRequest{
		Message: "CURRENT_INSPECTION_REQUEST",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, sess.GetMessages()) {
		t.Fatal("read-only public inspection mutated canonical product history")
	}
	joined := contextAnalysisJoined(analysis.ContextMessages)
	for _, want := range []string{"PRIOR_USER_CONTEXT", "PRIOR_ASSISTANT_CONTEXT", "CURRENT_INSPECTION_REQUEST"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("exact Writing inspection is missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "SECRET_TOOL_BODY") || strings.Contains(joined, "secret.md") {
		t.Fatalf("model-only tool visibility policy was not applied by public Middleware:\n%s", joined)
	}
	if analysis.SystemPrompt == "" || analysis.TokenEstimate <= 0 || analysis.MessageCount != len(analysis.ContextMessages) {
		t.Fatalf("incomplete public inspection analysis: %#v", analysis)
	}
}

func TestWritingAnalyzeContextUsesActivePublicCleanupCompactionAndMiddleware(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "inspection-maintenance-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	application.mu.RLock()
	sess := application.session
	application.mu.RUnlock()
	if sess == nil {
		t.Fatal("Writing Session is unavailable")
	}
	const marker = "RICH_WRITING_TOOL_RESULT_FOR_PUBLIC_INSPECTION"
	const placeholder = "[Writing tool result cleaned by public Agent]"
	rich := strings.Repeat(marker+" ", 300)
	for _, message := range []*agent.Message{
		agent.UserMessage("Inspect the old source."),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-writing-inspection", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"chapter.md"}`},
		}}),
		agent.ToolMessage(agent.ToolResult{Status: agent.ToolResultSuccess, ModelContent: rich}, "call-writing-inspection", agent.WithToolName("read")),
		agent.AssistantMessage("The source was inspected.", nil),
	} {
		if err := sess.Append(message); err != nil {
			t.Fatal(err)
		}
	}

	request := agentchat.ChatRequest{CommandID: "writing-inspection-maintenance", Message: "Prepare the next section."}
	cycle, _, err := application.chat().prepareWritingCycle(context.Background(), request, "")
	if err != nil {
		t.Fatal(err)
	}
	if cycle.Definition.Cleanup == nil || cycle.Definition.Compaction == nil {
		t.Fatal("Writing Definition does not expose public Cleanup and Compaction")
	}
	model := &writingInspectionCaptureModel{}
	cycle.Definition.Model = model
	cycle.Definition.Cleanup = writingInspectionCleanupManager{
		identity: cycle.Definition.Cleanup.Identity(), marker: marker, placeholder: placeholder,
	}
	cycle.Definition.Compaction = writingInspectionCompactionManager{identity: cycle.Definition.Compaction.Identity()}
	operation, err := application.executionRuntime.Start(context.Background(), agentexecution.StartRequest{Cycle: cycle})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(context.Background()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("Writing maintenance seed outcome = %#v", outcome)
	}
	model.mu.Lock()
	seedInput := append([]*agent.Message(nil), model.input...)
	model.mu.Unlock()
	seedVisible := joinAgentMessageContent(seedInput)
	if !strings.Contains(seedVisible, placeholder) || strings.Contains(seedVisible, marker) {
		t.Fatalf("public Cleanup was not applied to the exact seed request: %s", seedVisible)
	}
	status, err := application.executionRuntime.RuntimeStatusProjection(context.Background(), cycle.Options)
	if err != nil || status.Cleanup == nil {
		t.Fatalf("public Cleanup status = %#v err=%v", status.Cleanup, err)
	}

	compacted, err := application.CompactContext(context.Background())
	if err != nil || !compacted.Triggered {
		t.Fatalf("Writing public Compaction = %#v err=%v", compacted, err)
	}
	status, err = application.executionRuntime.RuntimeStatusProjection(context.Background(), cycle.Options)
	if err != nil || status.Compaction == nil {
		t.Fatalf("public Compaction status = %#v err=%v", status.Compaction, err)
	}
	analysis, err := application.AnalyzeContext(context.Background(), agentchat.ChatRequest{Message: "Inspect the maintained context."})
	if err != nil {
		t.Fatal(err)
	}
	joined := contextAnalysisJoined(analysis.ContextMessages)
	if !analysis.CompactionActive || analysis.Compaction == nil ||
		analysis.Compaction.ID != status.Compaction.ID || !strings.Contains(analysis.SystemPrompt, "EXACT_WRITING_PUBLIC_CHECKPOINT") {
		t.Fatalf("AnalyzeContext did not project the active public checkpoint: analysis=%#v context=%s", analysis, joined)
	}
	visible := analysis.SystemPrompt + "\n" + joined
	if strings.Contains(visible, marker) || strings.Contains(visible, placeholder) {
		t.Fatalf("AnalyzeContext reconstructed pre-compaction product history: %s", joined)
	}
}

func TestInteractiveAnalyzeContextUsesActivePublicCleanupCompactionAndMiddleware(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "game-inspection-maintenance-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	fixedSchema := interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeFixedTemplate}
	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{
		Title: "Public Game inspection", StoryTellerID: "classic", StateSchemaPolicy: &fixedSchema,
	})
	if err != nil {
		t.Fatal(err)
	}

	const marker = "RICH_GAME_TOOL_RESULT_FOR_PUBLIC_INSPECTION"
	const placeholder = "[Game tool result cleaned by public Agent]"
	rich := strings.Repeat(marker+" ", 300)
	application.mu.RLock()
	store := application.interactive
	application.mu.RUnlock()
	if store == nil {
		t.Fatal("Game Store is unavailable")
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "读取旧线索", Narrative: "旧线索已经用于故事。",
		ModelContextMessages: []interactive.ModelContextMessage{
			{Role: "assistant", ToolCalls: []interactive.ModelContextToolCall{{
				ID: "call-game-inspection", Type: "function",
				Function: interactive.ModelContextFunctionCall{Name: "read", Arguments: `{"path":"lore/archive.md"}`},
			}}},
			{Role: "tool", ToolCallID: "call-game-inspection", ToolName: "read", Content: rich,
				ToolResult: &agent.ToolResultSummary{Status: agent.ToolResultSuccess, ResultRetention: agent.ToolResultDeferred}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	request := interactiveAgentCycleRequest{
		CommandID: "game-inspection-maintenance", StoryID: story.ID, BranchID: "main",
		Message: "继续追查旧线索", Locale: "zh-CN",
	}
	cycle, err := application.interactiveService().prepareInteractiveAgentCycle(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	cycle.request.CommandID = request.CommandID
	if cycle.definition.Cleanup == nil || cycle.definition.Compaction == nil {
		t.Fatal("Game Definition does not expose public Cleanup and Compaction")
	}
	updates := []interactivestate.Update{
		{Op: "replace", Path: "/story/当前详细地点", Value: "旧档案室"},
		{Op: "replace", Path: "/story/当前事件", Value: "主角继续追查旧线索"},
	}
	choices := []string{"检查门锁", "询问守卫", "追踪脚印", "查看档案", "暂时撤退"}
	receipt, err := cycle.conversation.SubmitTurnResult(context.Background(), interactive.TurnSubmissionInput{
		StateUpdates: &updates, Choices: &choices,
	})
	if err != nil || !receipt.Ready {
		t.Fatalf("stage Game turn result: receipt=%#v err=%v", receipt, err)
	}
	model := &writingInspectionCaptureModel{}
	cycle.definition.Model = model
	cycle.definition.Cleanup = writingInspectionCleanupManager{
		identity: cycle.definition.Cleanup.Identity(), marker: marker, placeholder: placeholder,
	}
	gameCompactionIdentity := cycle.definition.Compaction.Identity()
	cycle.definition.Compaction = writingInspectionCompactionManager{
		identity: gameCompactionIdentity, summary: "EXACT_GAME_PUBLIC_CHECKPOINT",
	}
	operation, err := cycle.executionRuntime.Start(context.Background(), agentexecution.StartRequest{Cycle: agentexecution.Cycle{
		Definition: cycle.definition, Conversation: cycle.conversation,
		BookService: cycle.bookService, Request: cycle.request, Options: cycle.options(""),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(context.Background()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("Game maintenance seed outcome = %#v", outcome)
	}
	model.mu.Lock()
	seedInput := append([]*agent.Message(nil), model.input...)
	model.mu.Unlock()
	seedVisible := joinAgentMessageContent(seedInput)
	if !strings.Contains(seedVisible, placeholder) || strings.Contains(seedVisible, marker) {
		t.Fatalf("public Game Cleanup was not applied to the exact seed request: %s", seedVisible)
	}
	compacted, err := application.CompactInteractiveContext(context.Background(), story.ID, "main")
	if err != nil || !compacted.Triggered {
		t.Fatalf("Game public Compaction = %#v err=%v", compacted, err)
	}
	analysis, err := application.AnalyzeInteractiveContext(
		story.ID, "main", "检查维护后的真实模型上下文", nil, "zh-CN",
	)
	if err != nil {
		t.Fatal(err)
	}
	joined := contextAnalysisJoined(analysis.ContextMessages)
	if !analysis.CompactionActive || analysis.Compaction == nil ||
		!strings.Contains(analysis.SystemPrompt, "EXACT_GAME_PUBLIC_CHECKPOINT") {
		t.Fatalf("Game analysis did not project the active public checkpoint: analysis=%#v context=%s", analysis, joined)
	}
	visible := analysis.SystemPrompt + "\n" + joined
	if strings.Contains(visible, marker) || strings.Contains(visible, placeholder) {
		t.Fatalf("Game analysis reconstructed pre-compaction Story history: %s", visible)
	}
	if !strings.Contains(joined, "检查维护后的真实模型上下文") {
		t.Fatalf("Game inspection omitted the prospective interactive input: %s", joined)
	}
}

func contextAnalysisJoined(parts []agentchat.ContextAnalysisPart) string {
	var joined strings.Builder
	var appendParts func([]agentchat.ContextAnalysisPart)
	appendParts = func(current []agentchat.ContextAnalysisPart) {
		for _, part := range current {
			joined.WriteString(part.Content)
			joined.WriteByte('\n')
			appendParts(part.Parts)
		}
	}
	appendParts(parts)
	return joined.String()
}
