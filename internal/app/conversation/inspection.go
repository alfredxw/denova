package conversationapp

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	agents "denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
)

// InspectedTurn is the product projection input returned by the same
// Definition and Conversation assembly used for an admitted conversation
// turn. Inspection itself is owned by the public Agent Session lifecycle.
type InspectedTurn struct {
	Inspection  agent.Inspection
	Composition prompts.SystemPromptComposition
}

// InspectPrepared assembles a read-only conversation cycle after the caller
// has resolved layered settings, Skills, review references, and writing
// policy. It deliberately does not install product commit callbacks: public
// Session.Inspect is side-effect free and only returns the exact prospective
// provider request.
func InspectPrepared(
	ctx context.Context,
	runtime Runtime,
	request agentchat.ChatRequest,
	options agentrun.Options,
) (InspectedTurn, error) {
	if runtime.ExecutionRuntime == nil {
		return InspectedTurn{}, fmt.Errorf("conversation Agent runtime is unavailable")
	}
	built, err := appagentruntime.BuildConversationAgent(
		ctx, &runtime.Config, runtime.State, runtime.IDETeller, runtime.AgentKind,
		agents.AgentHostCapabilities{Interactive: true},
	)
	if err != nil {
		return InspectedTurn{}, err
	}
	options.ReviewThreadID = strings.TrimSpace(request.ResolvedReviewFeedback.PrimaryReviewThreadID())
	options.IdleTimeout = appagentruntime.IdleTimeout(runtime.Config)
	options.ToolResultMaxBytes = appagentruntime.ToolResultMaxBytes(runtime.Config)
	options.SystemPromptLog = built.Composition
	inspection, err := runtime.ExecutionRuntime.Inspect(ctx, agentexecution.Cycle{
		Definition: built.Definition, Conversation: ProjectConversation(runtime, request),
		BookService: runtime.BookService, Request: request, Options: options,
	})
	if err != nil {
		return InspectedTurn{}, err
	}
	return InspectedTurn{Inspection: inspection, Composition: built.Composition}, nil
}
