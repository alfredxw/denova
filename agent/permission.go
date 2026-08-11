package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	Kind   PermissionDecisionKind
	Reason LocalizedText
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
			resolution, err := RequestInteraction(ctx, InteractionRequest{
				ID: interactionID, Kind: InteractionPermission,
				Permission: &PermissionPresentation{
					Tool: tool.Name, CallID: tool.ExecutionID,
					Arguments: append(json.RawMessage(nil), request.Arguments...), Reason: decision.Reason,
				},
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
