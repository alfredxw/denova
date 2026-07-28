package tools

import (
	"context"
	"fmt"
	"strings"

	"denova/internal/automation"
	"denova/internal/configresources"
)

func newAutomationResource(novaDir, workspace string, workspaces []string) configresources.Adapter {
	store := configManagerAutomationStore(novaDir, workspace, workspaces)
	return configResourceAdapter{
		descriptor: configresources.Descriptor{
			Name: "automation", Description: "User and workspace automation definitions without runtime history fields.",
			Scopes: []string{automation.ScopeUser, automation.ScopeWorkspace}, Operations: configCRUDOperations(),
			RevisionField: "revision", Reference: "references/automation.md",
		},
		list: func(_ context.Context, request configresources.ReadRequest) (any, error) {
			scope, _, err := automationConfigTarget(request.Scope, workspace)
			if err != nil {
				return nil, err
			}
			tasks, err := store.ListInScope(scope)
			if err != nil {
				return nil, err
			}
			return configresources.NewCatalog(automationDefinitions(tasks)), nil
		},
		get: func(_ context.Context, request configresources.ReadRequest) (any, error) {
			scope, _, err := automationConfigTarget(request.Scope, workspace)
			if err != nil {
				return nil, err
			}
			ids := normalizeConfigIDs(request.IDs)
			if len(ids) != 1 {
				return nil, fmt.Errorf("automation exact read requires one id")
			}
			task, err := store.GetInScope(scope, ids[0])
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					return nil, configresources.Missing(err)
				}
				return nil, err
			}
			return automationDefinitionFromTask(task), nil
		},
		apply: func(_ context.Context, mutation configresources.Mutation) (any, error) {
			switch mutation.Operation {
			case configresources.ApplyCreate:
				var value automationTaskWriteInput
				if err := decodeConfigValue(mutation.Value, &value); err != nil {
					return nil, err
				}
				if value.Target == nil || strings.TrimSpace(value.Target.Kind) == "" {
					return nil, fmt.Errorf("automation create requires value.target.kind")
				}
				scope, target, err := automationConfigTarget(mutation.Scope, workspace)
				if err != nil {
					return nil, err
				}
				if strings.TrimSpace(value.Target.Kind) != target.Kind {
					return nil, fmt.Errorf("automation target kind %q does not match %s scope", value.Target.Kind, scope)
				}
				input := value.newTask()
				// Scope and workspace identity are host-owned. A model-supplied
				// target.workspace is display input only and never selects a path.
				input.Scope = scope
				input.Target = target
				task, err := store.Create(input)
				return automationMutationReceipt(mutation, task, err)
			case configresources.ApplyUpdate:
				var value automationTaskWriteInput
				if err := decodeConfigValue(mutation.Value, &value); err != nil {
					return nil, err
				}
				if value.Target != nil {
					return nil, fmt.Errorf("automation target is immutable; select it with scope")
				}
				scope, _, err := automationConfigTarget(mutation.Scope, workspace)
				if err != nil {
					return nil, err
				}
				current, err := store.GetInScope(scope, mutation.ID)
				if err != nil {
					return nil, err
				}
				value.applyDefinition(&current)
				task, err := store.UpdateInScopeIfRevision(scope, mutation.ID, current, mutation.Revision)
				return automationMutationReceipt(mutation, task, err)
			case configresources.ApplyDelete:
				scope, _, err := automationConfigTarget(mutation.Scope, workspace)
				if err != nil {
					return nil, err
				}
				if err := store.DeleteInScopeIfRevision(scope, mutation.ID, mutation.Revision); err != nil {
					return nil, err
				}
				return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: mutation.ID, Revision: mutation.Revision}, nil
			default:
				return nil, fmt.Errorf("unsupported automation operation %q", mutation.Operation)
			}
		},
	}
}

func automationMutationReceipt(mutation configresources.Mutation, task automation.Task, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return configMutationReceipt{
		Resource: mutation.Resource, Operation: mutation.Operation, ID: task.CatalogID,
		Revision: task.Revision, Value: automationDefinitionFromTask(task),
	}, nil
}

type automationDefinitionTarget struct {
	Kind string `json:"kind"`
}

// automationDefinition is the Config Manager projection of a task definition.
// Scheduler state, run history, durable command identities, and host effects
// never enter model context through config_read or mutation receipts.
type automationDefinition struct {
	ID                  string                         `json:"id"`
	CatalogID           string                         `json:"catalog_id,omitempty"`
	Revision            string                         `json:"revision"`
	Scope               string                         `json:"scope"`
	Target              automationDefinitionTarget     `json:"target"`
	Enabled             bool                           `json:"enabled"`
	Name                string                         `json:"name"`
	Template            string                         `json:"template"`
	Prompt              string                         `json:"prompt"`
	ModelProfileID      string                         `json:"model_profile_id,omitempty"`
	Schedule            automation.Schedule            `json:"schedule"`
	Triggers            []automation.TriggerDefinition `json:"triggers"`
	DefaultActionPolicy string                         `json:"default_action_policy"`
	WriteMode           string                         `json:"write_mode"`
	WriteScope          string                         `json:"write_scope"`
	OutputPolicy        string                         `json:"output_policy"`
	OutputPath          string                         `json:"output_path"`
}

func automationDefinitions(tasks []automation.Task) []automationDefinition {
	result := make([]automationDefinition, len(tasks))
	for index := range tasks {
		result[index] = automationDefinitionFromTask(tasks[index])
	}
	return result
}

func automationDefinitionFromTask(task automation.Task) automationDefinition {
	return automationDefinition{
		ID: task.ID, CatalogID: task.CatalogID, Revision: task.Revision, Scope: task.Scope,
		Target: automationDefinitionTarget{Kind: task.Target.Kind}, Enabled: task.Enabled,
		Name: task.Name, Template: task.Template, Prompt: task.Prompt, ModelProfileID: task.ModelProfileID,
		Schedule: task.Schedule, Triggers: append([]automation.TriggerDefinition(nil), task.Triggers...),
		DefaultActionPolicy: task.DefaultActionPolicy, WriteMode: task.WriteMode, WriteScope: task.WriteScope,
		OutputPolicy: task.OutputPolicy, OutputPath: task.OutputPath,
	}
}

func automationConfigTarget(scope, workspace string) (string, automation.ExecutionTarget, error) {
	scope = strings.TrimSpace(scope)
	switch scope {
	case automation.ScopeUser:
		return scope, automation.ExecutionTarget{Kind: automation.TargetKindUser}, nil
	case automation.ScopeWorkspace:
		workspace = strings.TrimSpace(workspace)
		if workspace == "" {
			return "", automation.ExecutionTarget{}, fmt.Errorf("workspace is required for workspace-scoped automation")
		}
		return scope, automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, Workspace: workspace}, nil
	default:
		return "", automation.ExecutionTarget{}, fmt.Errorf("automation scope must be %q or %q", automation.ScopeUser, automation.ScopeWorkspace)
	}
}
