// Package browser provides the optional provider-neutral browser Toolset.
package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type Action struct {
	Kind      string          `json:"kind"`
	Target    string          `json:"target,omitempty"`
	Value     string          `json:"value,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ActionResult struct {
	URL      string                 `json:"url,omitempty"`
	Title    string                 `json:"title,omitempty"`
	Content  string                 `json:"content,omitempty"`
	Artifact *agent.ToolArtifactRef `json:"artifact,omitempty"`
}

type Controller interface {
	Identity() agent.CapabilityIdentity
	Execute(context.Context, Action) (ActionResult, error)
}

type input struct {
	Actions []Action `json:"actions" jsonschema:"minItems=1,maxItems=32"`
}

type itemResult struct {
	Index  int           `json:"index"`
	Action Action        `json:"action"`
	Result *ActionResult `json:"result,omitempty"`
	Error  string        `json:"error,omitempty"`
}

func New(controller Controller) (agent.Toolset, error) {
	if controller == nil {
		return nil, errors.New("browser requires a Controller")
	}
	identity := controller.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return nil, errors.New("browser Controller requires a stable Identity")
	}
	tool, err := agent.InferTool("browser", "Execute an ordered browser action sequence through the configured controller. Each action reports its own outcome and later actions continue when safe.\n\n通过已配置的控制器按顺序执行浏览器操作；每个操作逐项返回结果，在安全时继续后续操作。", func(ctx context.Context, request input) (agent.ToolResult, error) {
		if len(request.Actions) == 0 || len(request.Actions) > 32 {
			return agent.ToolResult{}, errors.New("browser requires 1..32 actions")
		}
		results := make([]itemResult, len(request.Actions))
		artifacts := make([]agent.ToolArtifactRef, 0)
		for index, action := range request.Actions {
			results[index] = itemResult{Index: index, Action: action}
			action.Kind = strings.TrimSpace(action.Kind)
			if action.Kind == "" || len(action.Kind) > 256 || len(action.Target) > 64<<10 || len(action.Value) > 1<<20 ||
				len(action.Arguments) > 1<<20 || len(action.Arguments) > 0 && !json.Valid(action.Arguments) {
				results[index].Error = "invalid browser action"
				continue
			}
			result, actionErr := controller.Execute(ctx, action)
			if actionErr != nil {
				results[index].Error = actionErr.Error()
				continue
			}
			results[index].Result = &result
			if result.Artifact != nil {
				artifacts = append(artifacts, *result.Artifact)
			}
		}
		encoded, err := json.Marshal(struct {
			Results []itemResult `json:"results"`
		}{results})
		if err != nil {
			return agent.ToolResult{}, fmt.Errorf("encode browser result: %w", err)
		}
		result := agent.TextToolResult(string(encoded))
		result.Artifacts = artifacts
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return agent.StaticToolsIdentified(agent.CapabilityIdentity{
		Kind: "tools.plugin.browser", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
	}, agent.ToolDefinition{Tool: tool, Descriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceWeb, Capability: "browser", Execution: agent.ToolExecutionSessionExclusive,
		MutationScope: agent.ToolMutationExternal, PostCheck: agent.ToolPostCheckExternalReceipt,
		Recovery: agent.ToolRecoveryNonIdempotent, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: agent.ToolResultProtected, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 8 << 20,
	}}), nil
}
