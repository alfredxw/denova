package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"denova/config"
	"denova/internal/agents/toolapproval"

	agent "github.com/alfredxw/denova/agent"
)

type PermissionConfig struct {
	Mode      config.AgentApprovalMode
	ProjectID string
	Workspace string
	Rules     []config.AgentApprovalRule

	PersistRule func(context.Context, config.AgentApprovalRule) error
	GOOS        string
	clock       func() time.Time
}

type denovaPermissionPolicy struct{ config PermissionConfig }

// NewPermissionPolicy adapts Denova's mature shell/network classifier to the
// public durable PermissionPolicy. Rules are snapshotted for Definition
// identity; remembered authorization is persisted before Agent publishes the
// resolved interaction.
func NewPermissionPolicy(configValue PermissionConfig) (agent.PermissionPolicy, error) {
	configValue.Mode = config.NormalizeAgentApprovalMode(configValue.Mode)
	configValue.ProjectID = strings.TrimSpace(configValue.ProjectID)
	configValue.Workspace = strings.TrimSpace(configValue.Workspace)
	configValue.Rules = config.NormalizeAgentApprovalRules(configValue.Rules)
	if err := config.ValidateAgentApprovalRules(configValue.Rules); err != nil {
		return nil, fmt.Errorf("validate Denova Agent approval rules: %w", err)
	}
	if configValue.GOOS == "" {
		configValue.GOOS = runtime.GOOS
	}
	if configValue.clock == nil {
		configValue.clock = time.Now
	}
	return &denovaPermissionPolicy{config: configValue}, nil
}

func (policy *denovaPermissionPolicy) Identity() agent.CapabilityIdentity {
	payload := struct {
		Mode      config.AgentApprovalMode
		ProjectID string
		Workspace string
		Rules     []config.AgentApprovalRule
		GOOS      string
	}{
		policy.config.Mode, policy.config.ProjectID, policy.config.Workspace,
		config.NormalizeAgentApprovalRules(policy.config.Rules), policy.config.GOOS,
	}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{
		Kind: "denova.permission", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
	}
}

func (policy *denovaPermissionPolicy) Evaluate(_ context.Context, request agent.PermissionRequest) (agent.PermissionDecision, error) {
	decision := policy.evaluate(request)
	if err := decision.Validate(); err != nil {
		return agent.PermissionDecision{}, err
	}
	kind := agent.PermissionAsk
	switch decision.Action {
	case toolapproval.ActionAllow:
		kind = agent.PermissionAllow
	case toolapproval.ActionPrompt:
		kind = agent.PermissionAsk
	case toolapproval.ActionDeny:
		kind = agent.PermissionBlock
	default:
		return agent.PermissionDecision{}, fmt.Errorf("unsupported Denova approval action %q", decision.Action)
	}
	return agent.PermissionDecision{Kind: kind, Reason: localizedApprovalReason(decision.Reason)}, nil
}

func (policy *denovaPermissionPolicy) Resolve(ctx context.Context, request agent.PermissionResolveRequest) (agent.PermissionResolvedDecision, error) {
	switch request.Resolution.Permission {
	case agent.PermissionAllowOnce:
		return agent.PermissionResolvedDecision{Allowed: true}, nil
	case agent.PermissionDeny:
		return agent.PermissionResolvedDecision{}, nil
	case agent.PermissionRemember:
		decision := policy.evaluate(request.Request)
		if err := decision.Validate(); err != nil {
			return agent.PermissionResolvedDecision{}, err
		}
		if decision.Action == toolapproval.ActionAllow {
			return agent.PermissionResolvedDecision{Allowed: true, Remembered: true}, nil
		}
		if decision.Action != toolapproval.ActionPrompt || decision.Remember == nil {
			return agent.PermissionResolvedDecision{}, errors.New("Denova approval does not permit a remembered workspace rule")
		}
		if policy.config.PersistRule == nil {
			return agent.PermissionResolvedDecision{}, errors.New("Denova Permission remember requires a rule store")
		}
		rule, err := toolapproval.NewWorkspaceRule(
			policy.config.ProjectID,
			policy.config.Workspace,
			request.Request.Tool,
			*decision.Remember,
			toolapproval.ArgumentsHash(string(request.Request.Arguments)),
			decision.Command,
			decision.Cwd,
			decision.RuleID,
			policy.config.clock(),
		)
		if err != nil {
			return agent.PermissionResolvedDecision{}, err
		}
		if err := policy.config.PersistRule(ctx, rule); err != nil {
			return agent.PermissionResolvedDecision{}, fmt.Errorf("persist Denova Agent approval rule: %w", err)
		}
		return agent.PermissionResolvedDecision{Allowed: true, Remembered: true}, nil
	default:
		return agent.PermissionResolvedDecision{}, errors.New("invalid Denova Permission resolution")
	}
}

func (policy *denovaPermissionPolicy) evaluate(request agent.PermissionRequest) toolapproval.Decision {
	return toolapproval.Evaluate(toolapproval.Request{
		Mode: policy.config.Mode, ProjectID: policy.config.ProjectID, Workspace: policy.config.Workspace,
		ToolName: request.Tool, Arguments: string(request.Arguments), Descriptor: request.Descriptor,
		GOOS: policy.config.GOOS, Rules: config.NormalizeAgentApprovalRules(policy.config.Rules),
	})
}

func localizedApprovalReason(reason string) agent.LocalizedText {
	reason = strings.TrimSpace(reason)
	parts := strings.SplitN(reason, " / ", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		return agent.LocalizedText{Chinese: strings.TrimSpace(parts[0]), English: strings.TrimSpace(parts[1])}
	}
	return agent.LocalizedText{Chinese: reason, English: reason}
}

var _ agent.PermissionPolicy = (*denovaPermissionPolicy)(nil)
