// Package websearch provides the optional provider-neutral web_search Toolset.
package websearch

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

type Query struct {
	Text    string   `json:"text"`
	Domains []string `json:"domains,omitempty"`
	Limit   int      `json:"limit,omitempty"`
}

type Result struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Snippet   string `json:"snippet,omitempty"`
	Published string `json:"published,omitempty"`
}

type Provider interface {
	Identity() agent.CapabilityIdentity
	Search(context.Context, Query) ([]Result, error)
}

type input struct {
	Queries []Query `json:"queries" jsonschema:"minItems=1,maxItems=16"`
}

type queryResult struct {
	Index   int      `json:"index"`
	Query   Query    `json:"query"`
	Results []Result `json:"results,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func New(provider Provider) (agent.Toolset, error) {
	if provider == nil {
		return nil, errors.New("web_search requires a Provider")
	}
	identity := provider.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return nil, errors.New("web_search Provider requires a stable Identity")
	}
	tool, err := agent.InferTool("web_search", "Search the web through the configured provider. Batch queries return independent outcomes.", func(ctx context.Context, request input) (agent.ToolResult, error) {
		if len(request.Queries) == 0 || len(request.Queries) > 16 {
			return agent.ToolResult{}, errors.New("web_search requires 1..16 queries")
		}
		results := make([]queryResult, len(request.Queries))
		for index, query := range request.Queries {
			results[index] = queryResult{Index: index, Query: query}
			query.Text = strings.TrimSpace(query.Text)
			if query.Text == "" || len(query.Text) > 64<<10 || query.Limit < 0 || query.Limit > 100 {
				results[index].Error = "invalid web search query"
				continue
			}
			items, searchErr := provider.Search(ctx, query)
			if searchErr != nil {
				results[index].Error = searchErr.Error()
				continue
			}
			results[index].Results = items
		}
		return jsonResult(struct {
			Results []queryResult `json:"results"`
		}{results})
	})
	if err != nil {
		return nil, err
	}
	return agent.StaticToolsIdentified(pluginIdentity("tools.plugin.websearch", identity), agent.ToolDefinition{
		Tool: tool, Descriptor: webReadDescriptor("web_search"),
	})
}

func jsonResult(value any) (agent.ToolResult, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode web_search result: %w", err)
	}
	return agent.TextToolResult(string(encoded)), nil
}

func pluginIdentity(kind string, provider agent.CapabilityIdentity) agent.CapabilityIdentity {
	encoded, _ := json.Marshal(provider)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{Kind: kind, Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}

func webReadDescriptor(capability string) agent.ToolDescriptor {
	return agent.ToolDescriptor{
		Source: agent.ToolSourceWeb, Capability: capability, Execution: agent.ToolExecutionParallelRead,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryReadOnly, ResultRecoveryKind: agent.ToolResultRecoveryRefetch,
		ResultProjection: agent.ToolResultBoundedModelContext, ResultRetention: agent.ToolResultEagerCandidate,
		Steering: agent.SteeringFinishCurrent, MaxResultBytes: 2 << 20,
	}
}
