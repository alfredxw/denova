package interactiveapp

import (
	"fmt"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

// Native observations share the Story journal with product tools. They never
// apply a narrative, state update or a second product transaction.
func (turn *ExternalTurn) observeNative(event agentrun.Event) error {
	tool, ok := event.Data.(external.NativeTool)
	if !ok {
		return fmt.Errorf("invalid native Game tool observation")
	}
	c := turn.config.Conversation
	operationID := string(turn.identity.OperationID)
	id := tool.ExecutionID(operationID)
	if tool.Status != "running" {
		invocation := agentschema.AssistantMessage(turn.segment, []agentschema.ToolCall{{ID: id, Type: "function", Function: agentschema.FunctionCall{Name: tool.Name, Arguments: string(tool.Arguments)}}})
		observation := agentschema.ToolMessage(agentschema.ToolResult{Status: agentschema.ToolResultStatus(tool.Status), ModelContent: tool.Result, DisplayContent: tool.Result}, id)
		observation.ToolName = tool.Name
		if err := c.AppendContextMessages(invocation, observation); err != nil {
			return err
		}
		turn.observations = append(turn.observations, externalGameMessages([]*agentschema.Message{invocation, observation})...)
		turn.segment = ""
	}
	if err := c.AppendDisplayEvent(session.DisplayEvent{ID: id, Role: "tool_call", Name: tool.Name, Args: string(tool.Arguments), Status: tool.Status, Result: tool.Result, RuntimeManaged: true, RunID: operationID, AgentKind: config.AgentKindInteractiveStory}); err != nil {
		return err
	}
	turn.send(tool.DisplayEvent(operationID))
	return nil
}
