package continuallearning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
	agentstate "github.com/alfredxw/denova/agent/state"
)

const updateHarnessStateToolName = "update_harness_state"

type updateHarnessStateInput struct {
	BaseRevision string                     `json:"base_revision" jsonschema_description:"Exact revision returned by read(harness://state/current)."`
	Summary      string                     `json:"summary,omitempty" jsonschema_description:"Short English history summary. Omit to use a stable generic summary."`
	Changes      []updateHarnessStateChange `json:"changes" jsonschema_description:"Complete atomic file replacements or deletions. Independent input errors and candidate diagnostics are returned together."`
}

type updateHarnessStateChange struct {
	Path    string  `json:"path" jsonschema_description:"Harness State relative path such as tools/research_company.js."`
	Content *string `json:"content,omitempty" jsonschema_description:"Complete UTF-8 replacement content. Required unless delete is true."`
	Delete  bool    `json:"delete,omitempty" jsonschema_description:"Delete this path. Must not be combined with content."`
}

// StateUpdateTool constructs the single root-only mutation endpoint shared by
// user Agents and the Harness Optimizer.
func (service *Service) StateUpdateTool() (agent.ToolDefinition, error) {
	if _, err := service.requireEnabled(); err != nil {
		return agent.ToolDefinition{}, err
	}
	tool, err := agent.InferTool(
		updateHarnessStateToolName,
		"Atomically replace or delete User Harness State files against the current revision. Read harness://state/current first. The complete candidate is validated before any file changes. Use only when the user explicitly requests persistent User State, or during an authorized Harness Optimizer run.",
		func(ctx context.Context, input updateHarnessStateInput) (agent.ToolResult, error) {
			return service.runStateUpdate(ctx, input)
		},
	)
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	maxResultBytes := config.DefaultAgentToolResultLimitKB * 1024
	if service.host != nil {
		if configured := service.host.Runtime().Config.AgentToolResultLimitKB; configured > 0 {
			maxResultBytes = configured * 1024
		}
	}
	digest := sha256.Sum256([]byte(service.manager.Root()))
	definition := agent.ToolDefinition{
		Tool: tool,
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceWrite, Capability: config.AgentToolHarnessState,
			Execution: agent.ToolExecutionConfigExclusive, MutationScope: agent.ToolMutationConfig,
			PostCheck: agent.ToolPostCheckConfigRevision, Recovery: agent.ToolRecoveryReconcilable,
			ResultProjection: agent.ToolResultBoundedModelContext, ResultRetention: agent.ToolResultProtected,
			Steering: agent.SteeringFinishCurrent, MaxResultBytes: maxResultBytes,
			Presentation: agent.UniformToolPresentation(agent.ToolPresentationFile),
		},
		ImplementationIdentity: agent.CapabilityIdentity{
			Kind: "denova.harness_state.update", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
		},
	}
	if err := definition.Validate(context.Background()); err != nil {
		return agent.ToolDefinition{}, err
	}
	return definition, nil
}

func (service *Service) runStateUpdate(
	ctx context.Context,
	input updateHarnessStateInput,
) (agent.ToolResult, error) {
	diagnostics := validateStateUpdateInput(input)
	if len(diagnostics) != 0 {
		return stateUpdateFailure("invalid_changes", "Harness State changes are invalid.", diagnostics, ""), nil
	}
	changes := make([]StateChange, len(input.Changes))
	for index, change := range input.Changes {
		changes[index] = StateChange{Path: change.Path, Delete: change.Delete}
		if change.Content != nil {
			changes[index].Content = *change.Content
		}
	}
	result, err := service.UpdateState(ctx, StateUpdateRequest{
		BaseRevision: strings.TrimSpace(input.BaseRevision), Summary: input.Summary, Changes: changes,
	})
	if err != nil {
		var validation *StateValidationError
		switch {
		case errors.As(err, &validation):
			return stateUpdateFailure("state_validation_failed", "Harness State validation failed.", validation.Diagnostics, ""), nil
		case errors.Is(err, agentstate.ErrConflict):
			current, currentErr := service.manager.Store().Current(ctx)
			if currentErr != nil {
				return agent.ToolResult{}, errors.Join(err, currentErr)
			}
			return stateUpdateFailure("state_revision_conflict", "Harness State changed; read the current revision and form a new complete ChangeSet.", nil, current.Revision), nil
		default:
			return agent.ToolResult{}, err
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode Harness State update result: %w", err)
	}
	toolResult := agent.TextToolResult(string(encoded))
	toolResult.Details = append(json.RawMessage(nil), encoded...)
	toolResult.Metadata.Target = "harness://state/current"
	return toolResult, nil
}

func validateStateUpdateInput(input updateHarnessStateInput) []StateDiagnostic {
	var diagnostics []StateDiagnostic
	if strings.TrimSpace(input.BaseRevision) == "" {
		diagnostics = append(diagnostics, StateDiagnostic{Code: "base_revision_missing", Message: "base_revision is required"})
	}
	if len(input.Changes) == 0 {
		diagnostics = append(diagnostics, StateDiagnostic{Code: "changes_missing", Message: "at least one State change is required"})
	}
	stateChanges := make([]agentstate.Change, len(input.Changes))
	for index, change := range input.Changes {
		stateChanges[index] = agentstate.Change{Path: change.Path, Delete: change.Delete}
		switch {
		case change.Delete && change.Content != nil:
			diagnostics = append(diagnostics, StateDiagnostic{
				Code: "change_content_with_delete", Path: change.Path,
				Message: fmt.Sprintf("State change %d cannot combine content with delete", index),
			})
		case !change.Delete && change.Content == nil:
			diagnostics = append(diagnostics, StateDiagnostic{
				Code: "change_content_missing", Path: change.Path,
				Message: fmt.Sprintf("State change %d requires complete content or delete=true", index),
			})
		}
	}
	diagnostics = append(diagnostics, agentstate.ValidateChanges(stateChanges)...)
	return diagnostics
}

func stateUpdateFailure(code, message string, diagnostics []StateDiagnostic, currentRevision string) agent.ToolResult {
	details := struct {
		Code            string            `json:"code"`
		Message         string            `json:"message"`
		CurrentRevision string            `json:"current_revision,omitempty"`
		Diagnostics     []StateDiagnostic `json:"diagnostics,omitempty"`
	}{Code: code, Message: message, CurrentRevision: currentRevision, Diagnostics: diagnostics}
	encoded, err := json.Marshal(details)
	if err != nil {
		return agent.ToolErrorResult(message, message)
	}
	result := agent.ToolErrorResult(string(encoded), string(encoded))
	result.Details = encoded
	return result
}
