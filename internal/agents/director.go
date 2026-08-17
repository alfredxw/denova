package agents

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentinteractive "denova/internal/agents/interactive"
	agentrun "denova/internal/agents/run"
	"fmt"
	"strings"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/book"
	"denova/internal/interactive"

	agent "github.com/alfredxw/denova/agent"
)

const interactiveDirectorToolResultMaxBytes = interactive.DirectorContextMaxBytes

func GenerateInteractiveDirectorWithTools(ctx context.Context, executionRuntime *agentexecution.Runtime, cfg *config.Config, state *book.State, toolContext agentinteractive.InteractiveStoryToolContext, instruction string) (string, error) {
	if executionRuntime == nil {
		return "", fmt.Errorf("互动导演运行时不可用")
	}
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	if state == nil {
		return "", fmt.Errorf("互动导演故事状态不存在")
	}
	toolContext.CommandID = strings.TrimSpace(toolContext.CommandID)
	if err := agent.ValidateIdempotencyKey(toolContext.CommandID); err != nil {
		return "", fmt.Errorf("互动导演 command_id 无效: %w", err)
	}
	cycle, err := BuildInteractiveDirectorCycle(ctx, cfg, state, toolContext, instruction)
	if err != nil {
		return "", err
	}
	conversation, ok := cycle.Conversation.(*agentinteractive.DirectorConversation)
	if !ok {
		return "", fmt.Errorf("互动导演会话类型无效")
	}
	var runErr error
	outcome := executionRuntime.Run(ctx, agentexecution.StartRequest{Cycle: cycle, Emit: func(event agentrun.Event) {
		if event.Type != "error" {
			return
		}
		if data, ok := event.Data.(map[string]string); ok {
			runErr = fmt.Errorf("%s", data["message"])
		}
	}})
	if outcome.Status == agentrun.OutcomeFailed && outcome.Error != nil {
		runErr = outcome.Error
	}
	if runErr != nil {
		return "", runErr
	}
	output := conversation.Output()
	if output == "" {
		output = strings.TrimSpace(outcome.Content)
	}
	if output == "" && !agentinteractive.IsDirectorPlanTask(toolContext.MaintenanceTask) {
		return "", fmt.Errorf("互动导演 Agent 返回为空")
	}
	return output, nil
}

// BuildInteractiveDirectorCycle is the single product adapter for executable
// Director runs and read-only public Session inspection. Inspection may leave
// CommandID empty; GenerateInteractiveDirectorWithTools validates executable
// command identity before it reaches this composition seam.
func BuildInteractiveDirectorCycle(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	toolContext agentinteractive.InteractiveStoryToolContext,
	instruction string,
) (agentexecution.Cycle, error) {
	if cfg == nil {
		return agentexecution.Cycle{}, fmt.Errorf("配置不存在")
	}
	if state == nil {
		return agentexecution.Cycle{}, fmt.Errorf("互动导演故事状态不存在")
	}
	definition, systemPrompt, err := BuildInteractiveDirectorDefinitionWithComposition(ctx, cfg, state, toolContext)
	if err != nil {
		return agentexecution.Cycle{}, fmt.Errorf("构建互动导演 Agent 失败: %w", err)
	}
	runOptions := agentrun.Options{
		AgentKind:       config.AgentKindInteractiveDirector,
		StoryID:         toolContext.StoryID,
		BranchID:        toolContext.BranchID,
		TurnID:          toolContext.TurnID,
		MaintenanceTask: toolContext.MaintenanceTask,
		Workspace:       cfg.Workspace,
		SystemPromptLog: systemPrompt,
	}
	conversation := agentinteractive.NewDirectorConversation(agentinteractive.DirectorConversationOptions{
		Instruction: agentconversation.InstructionOptions{
			Instruction: instruction, StableContextTitle: toolContext.StableContextTitle,
			StableContext: toolContext.StableContext, StableContextMaxBytes: toolContext.StableContextMaxBytes,
			ContextBudget: agentcontext.ContextBudgetForAgent(cfg, config.AgentKindInteractiveDirector),
		},
		Display:         toolContext.DisplayConversation,
		CanonicalOutput: toolContext.CanonicalOutput,
	})
	bookService := book.NewService(state.Workspace())
	runOptions.ToolResultMaxBytes = interactiveDirectorToolResultMaxBytes
	return agentexecution.Cycle{
		Definition: definition, Conversation: conversation, BookService: bookService,
		Request: agentchat.ChatRequest{CommandID: toolContext.CommandID, Message: instruction}, Options: runOptions,
	}, nil
}
