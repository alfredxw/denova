package tools

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const maxRewindReportBytes = 64 * 1024

type checkpointInput struct {
	Purpose string `json:"purpose" jsonschema:"required" jsonschema_description:"Concise goal for the exploratory work that will be replaced by the rewind report."`
}

type rewindInput struct {
	CheckpointID string `json:"checkpoint_id,omitempty" jsonschema_description:"Checkpoint ID returned by checkpoint; omit to use the latest active checkpoint."`
	Report       string `json:"report,omitempty" jsonschema_description:"Bounded conclusions worth retaining after exploratory transcript is removed. Mutation receipts are retained automatically."`
}

// Checkpoint creates a run-local exploration boundary and stages it for
// durable publication with the successful assistant output.
func Checkpoint() (agent.Tool, error) {
	return agent.InferTool(
		"checkpoint",
		"Create one context checkpoint before noisy exploration. Rewind it before yielding or creating another checkpoint. This does not snapshot or roll back files; it only marks the model transcript.",
		func(ctx context.Context, input checkpointInput) (agent.ContextCheckpointResult, error) {
			controller, ok := agent.ContextWindowControllerFromContext(ctx)
			if !ok {
				return agent.ContextCheckpointResult{}, fmt.Errorf("checkpoint is unavailable outside a managed top-level Agent run")
			}
			input.Purpose = strings.TrimSpace(input.Purpose)
			if input.Purpose == "" {
				return agent.ContextCheckpointResult{}, fmt.Errorf("checkpoint purpose is required")
			}
			return controller.Checkpoint(ctx, agent.ContextCheckpointRequest{Purpose: input.Purpose})
		},
	)
}

// Rewind drops exploratory model transcript after a checkpoint while keeping a
// bounded report and automatically captured mutation receipts.
func Rewind() (agent.Tool, error) {
	return agent.InferTool(
		"rewind",
		"Close the active checkpoint and replace its exploratory model transcript with a concise report. Workspace or external side effects are never rolled back; their receipts are retained automatically.",
		func(ctx context.Context, input rewindInput) (agent.ContextRewindResult, error) {
			controller, ok := agent.ContextWindowControllerFromContext(ctx)
			if !ok {
				return agent.ContextRewindResult{}, fmt.Errorf("rewind is unavailable outside a managed top-level Agent run")
			}
			if len(input.Report) > maxRewindReportBytes {
				return agent.ContextRewindResult{}, fmt.Errorf("rewind report exceeds %d bytes", maxRewindReportBytes)
			}
			if strings.TrimSpace(input.Report) == "" {
				return agent.ContextRewindResult{}, fmt.Errorf("rewind report is required")
			}
			return controller.Rewind(ctx, agent.ContextRewindRequest{
				CheckpointID: strings.TrimSpace(input.CheckpointID), Report: strings.TrimSpace(input.Report),
			})
		},
	)
}
