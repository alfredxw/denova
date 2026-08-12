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
	"sync"
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

	LoadRules   func(context.Context) ([]config.AgentApprovalRule, error)
	PersistRule func(context.Context, config.AgentApprovalRule) error
	GOOS        string
	clock       func() time.Time
}

type denovaPermissionPolicy struct {
	config PermissionConfig
	rules  *permissionRuleState
}

type permissionRuleState struct {
	mu    sync.RWMutex
	rules []config.AgentApprovalRule
}

// NewPermissionPolicy adapts Denova's mature shell/network classifier to the
// public durable PermissionPolicy. Persisted rules are dynamic policy data,
// not Definition behavior identity: a remembered rule must become visible to
// the current Run without invalidating same-cycle cold recovery.
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
	return &denovaPermissionPolicy{
		config: configValue,
		rules:  &permissionRuleState{rules: clonePermissionRules(configValue.Rules)},
	}, nil
}

// BindPermissionRuleStore attaches the process-owned durable settings query
// and transaction without changing the policy's semantic identity. Dynamic
// rule contents are deliberately excluded from Definition identity.
func BindPermissionRuleStore(
	policy agent.PermissionPolicy,
	load func(context.Context) ([]config.AgentApprovalRule, error),
	persist func(context.Context, config.AgentApprovalRule) error,
) agent.PermissionPolicy {
	denova, ok := policy.(*denovaPermissionPolicy)
	if !ok || denova == nil {
		return policy
	}
	cloned := *denova
	cloned.config = denova.config
	cloned.config.LoadRules = load
	cloned.config.PersistRule = persist
	return &cloned
}

func (policy *denovaPermissionPolicy) Identity() agent.CapabilityIdentity {
	payload := struct {
		Mode      config.AgentApprovalMode
		ProjectID string
		Workspace string
		GOOS      string
		Matcher   int
	}{
		policy.config.Mode, policy.config.ProjectID, policy.config.Workspace,
		policy.config.GOOS, config.AgentApprovalRuleMatcherVersion,
	}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{
		Kind: "denova.permission", Version: 2, ConfigHash: hex.EncodeToString(digest[:]),
	}
}

func (policy *denovaPermissionPolicy) Evaluate(ctx context.Context, request agent.PermissionRequest) (agent.PermissionDecision, error) {
	rules, err := policy.rulesForEvaluation(ctx)
	if err != nil {
		return agent.PermissionDecision{}, err
	}
	decision := policy.evaluate(request, rules)
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
	details := agent.PermissionDetails{
		Mode: string(policy.config.Mode), Command: decision.Command, Cwd: decision.Cwd,
		Risk: string(decision.Risk), RuleID: decision.RuleID,
	}
	if decision.Command == "" {
		details.Details = strings.TrimSpace(strings.ToValidUTF8(string(request.Arguments), "\uFFFD"))
	}
	if decision.Remember != nil {
		details.CanRemember = true
		details.RuleMatcherVersion = decision.Remember.MatcherVersion
		details.RuleCommandKey = decision.Remember.CommandKey
		details.RuleCommandPattern = decision.Remember.CommandPattern
	}
	return agent.PermissionDecision{
		Kind: kind, Reason: localizedApprovalReason(decision.Reason), Details: details,
	}, nil
}

func (policy *denovaPermissionPolicy) Resolve(ctx context.Context, request agent.PermissionResolveRequest) (agent.PermissionResolvedDecision, error) {
	switch request.Resolution.Permission {
	case agent.PermissionAllowOnce:
		return agent.PermissionResolvedDecision{Allowed: true}, nil
	case agent.PermissionDeny:
		return agent.PermissionResolvedDecision{}, nil
	case agent.PermissionRemember:
		rules, err := policy.rulesForEvaluation(ctx)
		if err != nil {
			return agent.PermissionResolvedDecision{}, err
		}
		decision := policy.evaluate(request.Request, rules)
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
		if err := policy.rules.canRemember(rule); err != nil {
			return agent.PermissionResolvedDecision{}, err
		}
		if err := policy.config.PersistRule(ctx, rule); err != nil {
			return agent.PermissionResolvedDecision{}, fmt.Errorf("persist Denova Agent approval rule: %w", err)
		}
		if err := policy.rules.remember(rule); err != nil {
			return agent.PermissionResolvedDecision{}, err
		}
		return agent.PermissionResolvedDecision{Allowed: true, Remembered: true}, nil
	default:
		return agent.PermissionResolvedDecision{}, errors.New("invalid Denova Permission resolution")
	}
}

func (policy *denovaPermissionPolicy) rulesForEvaluation(ctx context.Context) ([]config.AgentApprovalRule, error) {
	if policy.config.LoadRules == nil {
		return policy.rules.snapshot(), nil
	}
	rules, err := policy.config.LoadRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("load Denova Agent approval rules: %w", err)
	}
	rules = config.NormalizeAgentApprovalRules(rules)
	if err := config.ValidateAgentApprovalRules(rules); err != nil {
		return nil, fmt.Errorf("validate loaded Denova Agent approval rules: %w", err)
	}
	policy.rules.replace(rules)
	return clonePermissionRules(rules), nil
}

func (policy *denovaPermissionPolicy) evaluate(request agent.PermissionRequest, rules []config.AgentApprovalRule) toolapproval.Decision {
	return toolapproval.Evaluate(toolapproval.Request{
		Mode: policy.config.Mode, ProjectID: policy.config.ProjectID, Workspace: policy.config.Workspace,
		ToolName: request.Tool, Arguments: string(request.Arguments), Descriptor: request.Descriptor,
		GOOS: policy.config.GOOS, Rules: rules,
	})
}

func (state *permissionRuleState) snapshot() []config.AgentApprovalRule {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return clonePermissionRules(state.rules)
}

func (state *permissionRuleState) replace(rules []config.AgentApprovalRule) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.rules = clonePermissionRules(rules)
}

func (state *permissionRuleState) remember(rule config.AgentApprovalRule) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := checkPermissionRuleConflict(state.rules, rule); err != nil {
		return err
	}
	for _, current := range state.rules {
		if current.ID == rule.ID {
			return nil
		}
	}
	state.rules = config.NormalizeAgentApprovalRules(append(state.rules, rule))
	return nil
}

func (state *permissionRuleState) canRemember(rule config.AgentApprovalRule) error {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return checkPermissionRuleConflict(state.rules, rule)
}

func checkPermissionRuleConflict(rules []config.AgentApprovalRule, rule config.AgentApprovalRule) error {
	for _, current := range rules {
		if current.ID == rule.ID && !samePermissionRuleBoundary(current, rule) {
			return fmt.Errorf("Agent approval rule id %q is already bound to another command", rule.ID)
		}
	}
	return nil
}

func samePermissionRuleBoundary(left, right config.AgentApprovalRule) bool {
	return left.Scope == right.Scope && left.ProjectID == right.ProjectID &&
		left.Workspace == right.Workspace && left.ToolName == right.ToolName &&
		left.MatcherVersion == right.MatcherVersion && left.CommandKey == right.CommandKey &&
		left.CommandPattern == right.CommandPattern
}

func clonePermissionRules(rules []config.AgentApprovalRule) []config.AgentApprovalRule {
	return append([]config.AgentApprovalRule(nil), rules...)
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
