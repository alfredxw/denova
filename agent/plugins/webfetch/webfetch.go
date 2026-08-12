// Package webfetch provides the optional provider-neutral web_fetch Toolset.
package webfetch

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

type Request struct {
	URL      string `json:"url"`
	MaxBytes int    `json:"max_bytes,omitempty"`
}

type Response struct {
	URL         string                 `json:"url"`
	Status      int                    `json:"status"`
	ContentType string                 `json:"content_type,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Content     string                 `json:"content,omitempty"`
	Truncated   bool                   `json:"truncated,omitempty"`
	Artifact    *agent.ToolArtifactRef `json:"artifact,omitempty"`
}

type Provider interface {
	Identity() agent.CapabilityIdentity
	Fetch(context.Context, Request) (Response, error)
}

type input struct {
	Requests []Request `json:"requests" jsonschema:"minItems=1,maxItems=16"`
}

type itemResult struct {
	Index    int       `json:"index"`
	Request  Request   `json:"request"`
	Response *Response `json:"response,omitempty"`
	Error    string    `json:"error,omitempty"`
}

func New(provider Provider) (agent.Toolset, error) {
	if provider == nil {
		return nil, errors.New("web_fetch requires a Provider")
	}
	identity := provider.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return nil, errors.New("web_fetch Provider requires a stable Identity")
	}
	tool, err := agent.InferTool("web_fetch", "Fetch and extract web resources through the configured network policy. Batch requests return independent outcomes.\n\n通过已配置的网络策略抓取并提取网页资源；批量请求会逐项返回结果。", func(ctx context.Context, request input) (agent.ToolResult, error) {
		if len(request.Requests) == 0 || len(request.Requests) > 16 {
			return agent.ToolResult{}, errors.New("web_fetch requires 1..16 requests")
		}
		results := make([]itemResult, len(request.Requests))
		artifacts := make([]agent.ToolArtifactRef, 0)
		for index, item := range request.Requests {
			results[index] = itemResult{Index: index, Request: item}
			item.URL = strings.TrimSpace(item.URL)
			if item.URL == "" || len(item.URL) > 64<<10 || item.MaxBytes < 0 || item.MaxBytes > 32<<20 {
				results[index].Error = "invalid web fetch request"
				continue
			}
			response, fetchErr := provider.Fetch(ctx, item)
			if fetchErr != nil {
				results[index].Error = fetchErr.Error()
				continue
			}
			results[index].Response = &response
			if response.Artifact != nil {
				artifacts = append(artifacts, *response.Artifact)
			}
		}
		encoded, err := json.Marshal(struct {
			Results []itemResult `json:"results"`
		}{results})
		if err != nil {
			return agent.ToolResult{}, fmt.Errorf("encode web_fetch result: %w", err)
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
		Kind: "tools.plugin.webfetch", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
	}, agent.ToolDefinition{Tool: tool, Descriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceWeb, Capability: "web_fetch", Execution: agent.ToolExecutionParallelRead,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryReadOnly, ResultRecoveryKind: agent.ToolResultRecoveryRefetch,
		ResultProjection: agent.ToolResultBoundedModelContext, ResultRetention: agent.ToolResultEagerCandidate,
		Steering: agent.SteeringFinishCurrent, MaxResultBytes: 8 << 20,
	}})
}
