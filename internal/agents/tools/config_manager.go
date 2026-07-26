package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/configresources"
)

const maxConfigReadIDs = 32

type configReadInput struct {
	Operation string   `json:"operation" jsonschema:"enum=describe,enum=list,enum=get" jsonschema_description:"describe returns resource contracts; list returns a bounded catalog; get reads exact IDs."`
	Resource  string   `json:"resource,omitempty" jsonschema_description:"Resource kind. Omit only when operation=describe to inspect every available resource."`
	IDs       []string `json:"ids,omitempty" jsonschema:"maxItems=32" jsonschema_description:"Exact resource IDs for operation=get."`
	Scope     string   `json:"scope,omitempty" jsonschema_description:"Optional resource scope, such as user or workspace."`
	Query     string   `json:"query,omitempty" jsonschema_description:"Optional adapter-specific bounded catalog filter."`
}

type configApplyInput struct {
	Operation string         `json:"operation" jsonschema:"enum=create,enum=update,enum=delete" jsonschema_description:"One independently committed mutation."`
	Resource  string         `json:"resource" jsonschema_description:"Resource kind returned by config_read describe."`
	ID        string         `json:"id,omitempty" jsonschema_description:"Exact resource ID; required for update and delete."`
	Scope     string         `json:"scope,omitempty" jsonschema_description:"Required for scoped resources such as skill and agent_profile."`
	Revision  string         `json:"revision,omitempty" jsonschema_description:"Required for update and delete, and for agent_profile SubAgent create; copy the latest exact-scope revision from config_read."`
	Value     map[string]any `json:"value,omitempty" jsonschema_description:"Complete create/update value documented by the resource Skill reference; agent_profile delete requires value.kind."`
}

func newConfigManagerTools(cfg *config.Config, _ config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	registry, err := newConfigResourceRegistry(cfg)
	if err != nil {
		return nil, err
	}
	readTool, err := agent.InferTool(
		"config_read",
		"Inspect, list, or read Denova configuration resources through one typed registry. Call operation=describe before using an unfamiliar resource; use list before get, and keep returned revisions for later mutations.",
		func(ctx context.Context, input configReadInput) (string, error) {
			if len(input.IDs) > maxConfigReadIDs {
				return "", fmt.Errorf("config_read accepts at most %d ids", maxConfigReadIDs)
			}
			value, err := registry.Read(ctx, configresources.ReadRequest{
				Operation: input.Operation, Resource: input.Resource, IDs: input.IDs, Scope: input.Scope, Query: input.Query,
			})
			if err != nil {
				return "", err
			}
			return marshalConfigResourceResult(value)
		},
	)
	if err != nil {
		return nil, err
	}
	applyTool, err := agent.InferTool(
		"config_apply",
		"Create, update, or delete exactly one Denova configuration resource. Updates, deletes, and agent_profile SubAgent creates require the latest revision from config_read; agent_profile deletes require value.kind. Resource-specific value shapes live in the config-manager Skill references.",
		func(ctx context.Context, input configApplyInput) (agent.ToolResult, error) {
			value, err := registry.Apply(ctx, configresources.Mutation{
				Operation: input.Operation, Resource: input.Resource, ID: input.ID, Scope: input.Scope, Revision: input.Revision, Value: input.Value,
			})
			if err != nil {
				return agent.ToolResult{}, err
			}
			return configApplyResult(value)
		},
	)
	if err != nil {
		return nil, err
	}
	readDefinition, err := defineTool(readTool, configReadDescriptor())
	if err != nil {
		return nil, err
	}
	applyDefinition, err := defineTool(applyTool, configApplyDescriptor())
	if err != nil {
		return nil, err
	}
	return []agent.ToolDefinition{readDefinition, applyDefinition}, nil
}

func newConfigResourceRegistry(cfg *config.Config) (*configresources.Registry, error) {
	novaDir := strings.TrimSpace(cfg.DataDir())
	workspace := strings.TrimSpace(cfg.Workspace)
	workspaces := append([]string(nil), cfg.AutomationWorkspaces...)
	return configresources.New(
		newStyleReferenceResource(novaDir),
		newNarrativeStyleResource(novaDir),
		newStoryDirectorResource(novaDir),
		newEventPackageResource(novaDir),
		newRuleSystemResource(novaDir),
		newStateSystemResource(novaDir),
		newImagePresetResource(novaDir),
		newAutomationResource(novaDir, workspace, workspaces),
		newSkillConfigResource(cfg),
		newAgentProfileResource(cfg),
	)
}

func configReadDescriptor() agent.ToolDescriptor {
	return agent.ToolDescriptor{
		Source: agent.ToolSourceRead, Capability: config.AgentToolConfigRead, Execution: agent.ToolExecutionParallelRead,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone, Recovery: agent.ToolRecoveryReadOnly,
		ResultProjection: agent.ToolResultBoundedModelContext, Steering: agent.SteeringFinishCurrent, MaxResultBytes: defaultToolResultMaxBytes,
	}
}

func configApplyDescriptor() agent.ToolDescriptor {
	return agent.ToolDescriptor{
		Source: agent.ToolSourceWrite, Capability: config.AgentToolConfigApply, Execution: agent.ToolExecutionConfigExclusive,
		MutationScope: agent.ToolMutationConfig, PostCheck: agent.ToolPostCheckConfigRevision, Recovery: agent.ToolRecoveryReconcilable,
		ResultProjection: agent.ToolResultBoundedModelContext, Steering: agent.SteeringFinishCurrent, MaxResultBytes: defaultToolResultMaxBytes,
	}
}

func marshalConfigResourceResult(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode config resource result: %w", err)
	}
	return string(data), nil
}

type configApplyReceiptDetails struct {
	Schema    string `json:"schema"`
	Status    string `json:"status"`
	Resource  string `json:"resource"`
	Operation string `json:"operation"`
	ID        string `json:"id"`
	Revision  string `json:"revision,omitempty"`
}

func configApplyResult(value any) (agent.ToolResult, error) {
	content, err := marshalConfigResourceResult(value)
	if err != nil {
		return agent.ToolResult{}, err
	}
	receipt, ok := value.(configMutationReceipt)
	if !ok {
		return agent.ToolResult{}, fmt.Errorf("config resource Adapter returned %T, want configMutationReceipt", value)
	}
	details, err := json.Marshal(configApplyReceiptDetails{
		Schema: "config.mutation_receipt.v1", Status: "applied",
		Resource: receipt.Resource, Operation: receipt.Operation,
		ID: receipt.ID, Revision: receipt.Revision,
	})
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode config mutation receipt: %w", err)
	}
	result := agent.TextToolResult(content)
	result.Details = details
	result.Metadata.Target = receipt.ID
	return result, nil
}

func firstConfigNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
