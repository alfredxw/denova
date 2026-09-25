package agent

import (
	"context"
	"fmt"
)

// Tool approval revalidates executable contracts without reloading model
// context, which may include files changed by earlier calls in the same turn.
func materializeDefinitionTools(ctx context.Context, request PrepareRequest, prepared *preparedDefinition) error {
	var tools []ToolDefinition
	var err error
	if prepared.definition.Tools != nil {
		tools, err = prepared.definition.Tools.PrepareTools(ctx, ToolRequest{
			Session: request.Session, Run: request.Run, Input: request.Input,
		})
		if err != nil {
			return fmt.Errorf("prepare agent Toolset: %w", err)
		}
	}
	registry, err := NewRegistry(ctx, tools...)
	if err != nil {
		return fmt.Errorf("prepare agent Toolset: %w", err)
	}
	prepared.tools = registry.Definitions()
	prepared.toolSnapshots = registry.Snapshots()
	return nil
}
