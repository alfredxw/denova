package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type PermissionDecisionKind string

const (
	PermissionAllow PermissionDecisionKind = "allow"
	PermissionAsk   PermissionDecisionKind = "ask"
	PermissionBlock PermissionDecisionKind = "deny"
)

type PermissionRequest struct {
	Session    SessionView
	Run        RunView
	CallID     string
	Tool       string
	Arguments  json.RawMessage
	Descriptor ToolDescriptor
}

type PermissionDecision struct {
	Kind    PermissionDecisionKind
	Reason  LocalizedText
	Details PermissionDetails
}

// PermissionDetails is policy-owned audit and presentation metadata. Agent
// supplies the exact tool, call, canonical arguments, and argument hash so a
// host never needs to reconstruct authorization evidence from product state.
type PermissionDetails struct {
	Mode               string `json:"mode,omitempty"`
	Command            string `json:"command,omitempty"`
	Details            string `json:"details,omitempty"`
	Cwd                string `json:"cwd,omitempty"`
	Risk               string `json:"risk,omitempty"`
	RuleID             string `json:"rule_id,omitempty"`
	CanRemember        bool   `json:"can_remember,omitempty"`
	RuleMatcherVersion int    `json:"rule_matcher_version,omitempty"`
	RuleMatchKey       string `json:"rule_match_key,omitempty"`
	RuleDisplayPattern string `json:"rule_display_pattern,omitempty"`
}

type PermissionResolveRequest struct {
	Request    PermissionRequest
	Resolution InteractionResolution
}

type PermissionResolvedDecision struct {
	Allowed    bool
	Remembered bool
}

type PermissionPolicy interface {
	Identity() CapabilityIdentity
	Evaluate(context.Context, PermissionRequest) (PermissionDecision, error)
	// Resolve must persist a remembered rule before returning success. Agent
	// commits InteractionResolved only after this method completes.
	Resolve(context.Context, PermissionResolveRequest) (PermissionResolvedDecision, error)
}

type safeDefaultPermissionPolicy struct{}

func (safeDefaultPermissionPolicy) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "permission.safe_default", Version: 1}
}

func (safeDefaultPermissionPolicy) Evaluate(_ context.Context, request PermissionRequest) (PermissionDecision, error) {
	if request.Descriptor.MutationScope == ToolMutationNone && request.Descriptor.Source != ToolSourceShell ||
		request.Descriptor.MutationScope == ToolMutationSession && request.Descriptor.Recovery == ToolRecoveryIdempotent {
		return PermissionDecision{Kind: PermissionAllow}, nil
	}
	return PermissionDecision{
		Kind: PermissionAsk,
		Reason: LocalizedText{
			Chinese: fmt.Sprintf("工具 %s 将访问或修改受保护资源。", request.Tool),
			English: fmt.Sprintf("Tool %s will access or modify protected resources.", request.Tool),
		},
	}, nil
}

func (safeDefaultPermissionPolicy) Resolve(_ context.Context, request PermissionResolveRequest) (PermissionResolvedDecision, error) {
	switch request.Resolution.Permission {
	case PermissionAllowOnce:
		return PermissionResolvedDecision{Allowed: true}, nil
	case PermissionDeny:
		return PermissionResolvedDecision{}, nil
	case PermissionRemember:
		return PermissionResolvedDecision{}, errors.New("safe default Permission Policy has no durable rule store")
	default:
		return PermissionResolvedDecision{}, errors.New("permission resolution is invalid")
	}
}

func effectivePermissionPolicy(policy PermissionPolicy) PermissionPolicy {
	if policy == nil {
		return safeDefaultPermissionPolicy{}
	}
	return policy
}

const permissionDetailsMaxBytes = 16 << 10

func permissionPresentation(request PermissionRequest, decision PermissionDecision) PermissionPresentation {
	details := decision.Details
	if strings.TrimSpace(details.Mode) == "" {
		details.Mode = "custom"
	}
	if strings.TrimSpace(details.Risk) == "" {
		details.Risk = "high"
	}
	if strings.TrimSpace(details.RuleID) == "" {
		details.RuleID = "permission_required"
	}
	if strings.TrimSpace(details.Command) == "" && strings.TrimSpace(details.Details) == "" {
		details.Details = truncatePermissionDetails(string(request.Arguments))
	}
	digest := sha256.Sum256(request.Arguments)
	return PermissionPresentation{
		Tool: request.Tool, CallID: request.CallID,
		Arguments: append(json.RawMessage(nil), request.Arguments...), Reason: decision.Reason,
		Mode: details.Mode, Command: details.Command, Details: details.Details, Cwd: details.Cwd,
		Risk: details.Risk, RuleID: details.RuleID, ArgsHash: fmt.Sprintf("%x", digest[:]),
		CanRemember: details.CanRemember, RuleMatcherVersion: details.RuleMatcherVersion,
		RuleMatchKey: details.RuleMatchKey, RuleDisplayPattern: details.RuleDisplayPattern,
		Options: permissionOptions(details.CanRemember),
	}
}

func truncatePermissionDetails(value string) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, "\uFFFD"))
	if len(value) <= permissionDetailsMaxBytes {
		return value
	}
	const marker = "\n… [details truncated]"
	value = value[:permissionDetailsMaxBytes-len(marker)]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + marker
}

func permissionOptions(canRemember bool) []PermissionOption {
	options := []PermissionOption{{
		Value: string(PermissionAllowOnce),
		Label: LocalizedText{Chinese: "仅允许这一次", English: "Allow once"},
		Description: LocalizedText{
			Chinese: "仅执行当前调用，不保存权限规则。",
			English: "Execute only this call without saving a permission rule.",
		},
	}}
	if canRemember {
		options = append(options, PermissionOption{
			Value: string(PermissionRemember),
			Label: LocalizedText{Chinese: "在当前工作区始终允许", English: "Always allow here"},
			Description: LocalizedText{
				Chinese: "保存当前展示的命令匹配规则。",
				English: "Save the displayed command matching rule for this workspace.",
			},
		})
	}
	return append(options, PermissionOption{
		Value: string(PermissionDeny),
		Label: LocalizedText{Chinese: "拒绝", English: "Deny"},
		Description: LocalizedText{
			Chinese: "阻止这次调用，让 Agent 选择其他方案。",
			English: "Block this call and let the Agent choose another approach.",
		},
	})
}

type permissionMiddleware struct {
	*BaseMiddleware
	policy  PermissionPolicy
	session SessionView
	run     RunView
}

func (middleware *permissionMiddleware) WrapToolCall(
	_ context.Context,
	endpoint ToolCallEndpoint,
	tool *ToolContext,
) (ToolCallEndpoint, error) {
	if middleware == nil || middleware.policy == nil || endpoint == nil || tool == nil {
		return nil, errors.New("Permission middleware is incomplete")
	}
	return func(ctx context.Context, arguments string, options ...ToolOption) (ToolResult, error) {
		request := PermissionRequest{
			Session: middleware.session, Run: middleware.run,
			CallID: tool.ExecutionID, Tool: tool.Name, Arguments: json.RawMessage(arguments),
			Descriptor: tool.Definition.Descriptor,
		}
		decision, err := middleware.policy.Evaluate(ctx, request)
		if err != nil {
			return ToolResult{}, fmt.Errorf("evaluate permission for tool %q: %w", tool.Name, err)
		}
		switch decision.Kind {
		case PermissionAllow:
			return endpoint(ctx, arguments, options...)
		case PermissionBlock:
			return ToolResult{}, fmt.Errorf("%w: tool %q", ErrPermissionDenied, tool.Name)
		case PermissionAsk:
			if err := validateLocalizedText(decision.Reason); err != nil {
				return ToolResult{}, fmt.Errorf("permission reason for tool %q: %w", tool.Name, err)
			}
			interactionID := "permission-" + strings.TrimSpace(tool.ExecutionID)
			if strings.TrimSpace(tool.ExecutionID) == "" {
				return ToolResult{}, errors.New("Permission requires a durable tool execution ID")
			}
			presentation := permissionPresentation(request, decision)
			resolution, err := RequestInteraction(ctx, InteractionRequest{
				ID: interactionID, Kind: InteractionPermission,
				Permission: &presentation,
			})
			if err != nil {
				return ToolResult{}, err
			}
			if resolution.Cancelled || resolution.Permission == PermissionDeny {
				return ToolResult{}, fmt.Errorf("%w: tool %q", ErrPermissionDenied, tool.Name)
			}
			if resolution.Permission != PermissionAllowOnce && resolution.Permission != PermissionRemember {
				return ToolResult{}, errors.New("Permission Interaction returned an invalid resolution")
			}
			return endpoint(ctx, arguments, options...)
		default:
			return ToolResult{}, fmt.Errorf("Permission Policy returned unsupported decision %q", decision.Kind)
		}
	}, nil
}
