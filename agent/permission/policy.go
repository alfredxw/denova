package permission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"
)

type Mode string

const (
	ModeSafeDefault Mode = "safe_default"
	ModeReadOnly    Mode = "read_only"
	ModeCoding      Mode = "coding"
	ModeFullAccess  Mode = "full_access"
)

type Rule struct {
	Tool       string                  `json:"tool"`
	Source     agent.ToolSource        `json:"source"`
	Capability string                  `json:"capability,omitempty"`
	Scope      agent.ToolMutationScope `json:"scope"`
}

type RuleStore interface {
	Identity() agent.CapabilityIdentity
	Allowed(context.Context, Rule) (bool, error)
	Remember(context.Context, Rule) error
}

type memoryRules struct {
	mu      sync.RWMutex
	allowed map[Rule]struct{}
}

// MemoryRules is a process-local RuleStore for temporary Agents and tests. Its
// identity describes the in-memory storage semantics; rule contents remain
// dynamic and intentionally do not participate in Definition behavior identity.
// Durable Sessions should provide a durable RuleStore instead.
func MemoryRules() RuleStore { return &memoryRules{allowed: make(map[Rule]struct{})} }

func (*memoryRules) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "permission.rules.memory", Version: 1}
}

func (store *memoryRules) Allowed(_ context.Context, rule Rule) (bool, error) {
	store.mu.RLock()
	_, ok := store.allowed[rule]
	store.mu.RUnlock()
	return ok, nil
}

func (store *memoryRules) Remember(_ context.Context, rule Rule) error {
	store.mu.Lock()
	store.allowed[rule] = struct{}{}
	store.mu.Unlock()
	return nil
}

type policy struct {
	mode  Mode
	rules RuleStore
}

func SafeDefault() agent.PermissionPolicy { return newPolicy(ModeSafeDefault, nil) }
func ReadOnly() agent.PermissionPolicy    { return newPolicy(ModeReadOnly, nil) }
func Coding() agent.PermissionPolicy      { return newPolicy(ModeCoding, nil) }
func FullAccess() agent.PermissionPolicy  { return newPolicy(ModeFullAccess, nil) }

// SafeDefaultWithRules and CodingWithRules make the one durable RuleStore
// authority explicit. The prior variadic constructor silently ignored a
// second Store, which made composition order change authorization behavior.
func SafeDefaultWithRules(store RuleStore) (agent.PermissionPolicy, error) {
	return newPolicyWithRules(ModeSafeDefault, store)
}

func CodingWithRules(store RuleStore) (agent.PermissionPolicy, error) {
	return newPolicyWithRules(ModeCoding, store)
}

func newPolicyWithRules(mode Mode, store RuleStore) (agent.PermissionPolicy, error) {
	if store == nil {
		return nil, errors.New("Permission RuleStore is nil")
	}
	identity := store.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return nil, errors.New("Permission RuleStore requires a stable identity")
	}
	return newPolicy(mode, store), nil
}

func newPolicy(mode Mode, store RuleStore) agent.PermissionPolicy {
	return &policy{mode: mode, rules: store}
}

func (policy *policy) Identity() agent.CapabilityIdentity {
	identity := struct {
		Mode  Mode
		Rules agent.CapabilityIdentity
	}{Mode: policy.mode}
	if policy.rules != nil {
		identity.Rules = policy.rules.Identity()
	}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	hash := hex.EncodeToString(digest[:])
	return agent.CapabilityIdentity{Kind: "permission." + string(policy.mode), Version: 1, ConfigHash: hash}
}

func (policy *policy) Evaluate(ctx context.Context, request agent.PermissionRequest) (agent.PermissionDecision, error) {
	if policy == nil {
		return agent.PermissionDecision{}, errors.New("Permission Policy is nil")
	}
	if criticalToolRequest(request) {
		return agent.PermissionDecision{Kind: agent.PermissionBlock, Reason: criticalReason()}, nil
	}
	rule := ruleFor(request)
	if policy.rules != nil {
		allowed, err := policy.rules.Allowed(ctx, rule)
		if err != nil {
			return agent.PermissionDecision{}, fmt.Errorf("read Permission rule: %w", err)
		}
		if allowed {
			return agent.PermissionDecision{Kind: agent.PermissionAllow}, nil
		}
	}
	switch policy.mode {
	case ModeReadOnly:
		if readOnly(request) {
			return agent.PermissionDecision{Kind: agent.PermissionAllow}, nil
		}
		return agent.PermissionDecision{Kind: agent.PermissionBlock, Reason: protectedReason(request.Tool)}, nil
	case ModeFullAccess:
		return agent.PermissionDecision{Kind: agent.PermissionAllow}, nil
	case ModeSafeDefault, ModeCoding:
		if readOnly(request) || internalIdempotentState(request) {
			return agent.PermissionDecision{Kind: agent.PermissionAllow}, nil
		}
		return agent.PermissionDecision{
			Kind: agent.PermissionAsk, Reason: protectedReason(request.Tool),
			Details: agent.PermissionDetails{
				Mode: string(policy.mode), Risk: "high", RuleID: "protected_resource",
				CanRemember: policy.rules != nil,
			},
		}, nil
	default:
		return agent.PermissionDecision{}, fmt.Errorf("unsupported Permission mode %q", policy.mode)
	}
}

func (policy *policy) Resolve(ctx context.Context, request agent.PermissionResolveRequest) (agent.PermissionResolvedDecision, error) {
	switch request.Resolution.Permission {
	case agent.PermissionAllowOnce:
		return agent.PermissionResolvedDecision{Allowed: true}, nil
	case agent.PermissionDeny:
		return agent.PermissionResolvedDecision{}, nil
	case agent.PermissionRemember:
		if policy.rules == nil {
			return agent.PermissionResolvedDecision{}, errors.New("Permission remember requires a RuleStore")
		}
		if err := policy.rules.Remember(ctx, ruleFor(request.Request)); err != nil {
			return agent.PermissionResolvedDecision{}, fmt.Errorf("persist Permission rule: %w", err)
		}
		return agent.PermissionResolvedDecision{Allowed: true, Remembered: true}, nil
	default:
		return agent.PermissionResolvedDecision{}, errors.New("invalid Permission resolution")
	}
}

func readOnly(request agent.PermissionRequest) bool {
	return request.Descriptor.MutationScope == agent.ToolMutationNone && request.Descriptor.Source != agent.ToolSourceShell
}

func internalIdempotentState(request agent.PermissionRequest) bool {
	return request.Descriptor.MutationScope == agent.ToolMutationSession && request.Descriptor.Recovery == agent.ToolRecoveryIdempotent
}

func ruleFor(request agent.PermissionRequest) Rule {
	return Rule{Tool: request.Tool, Source: request.Descriptor.Source, Capability: request.Descriptor.Capability, Scope: request.Descriptor.MutationScope}
}

func protectedReason(tool string) agent.LocalizedText {
	return agent.LocalizedText{
		Chinese: fmt.Sprintf("工具 %s 将访问或修改受保护资源。", tool),
		English: fmt.Sprintf("Tool %s will access or modify protected resources.", tool),
	}
}

func criticalReason() agent.LocalizedText {
	return agent.LocalizedText{Chinese: "该操作可能破坏系统或大范围删除数据，已被拒绝。", English: "This operation may damage the system or delete data broadly and was denied."}
}

func criticalToolRequest(request agent.PermissionRequest) bool {
	if request.Descriptor.Source != agent.ToolSourceShell {
		return false
	}
	text := strings.ToLower(string(request.Arguments))
	text = strings.Join(strings.Fields(text), " ")
	critical := []string{
		"rm -rf /", "rm -fr /", "mkfs.", "format c:",
		"remove-item c:\\ -recurse", "remove-item / -recurse",
		"dd if=/dev/zero of=/dev/", "> /dev/sda", "shutdown -h", "reboot -f",
	}
	for _, pattern := range critical {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

var _ agent.PermissionPolicy = (*policy)(nil)
