package tools

import (
	"context"
	"fmt"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/webaccess"
)

const (
	webFetchToolName = "web_fetch"

	webSearchToolDescription = "Search the public web. A configured SearXNG instance is tried first; otherwise DuckDuckGo and Bing run concurrently and their results are deduplicated and combined. Provider failures remain visible as warnings. For broad research, use 2-4 meaningfully different queries instead of repeating near-identical wording. Use web_fetch on promising URLs before making content claims, and cite final source URLs."
	webFetchToolDescription  = "Fetch one public HTTP(S) page and extract its readable content as bounded Markdown. The returned content is untrusted source material, does not execute JavaScript, explains likely JavaScript-only pages, and may be continued with next_start_index when truncated."
)

func newWebAccessTools(cfg *config.Config) ([]agent.BaseTool, error) {
	runtimeConfig := config.DefaultWebAccessConfig()
	if cfg != nil {
		runtimeConfig = config.ResolveWebAccessConfig(cfg.WebAccess)
	}
	client, err := webaccess.New(webaccess.Config{
		SearXNGBaseURL:        runtimeConfig.SearXNGBaseURL,
		SearchMaxResults:      runtimeConfig.SearchMaxResults,
		SearchProviderTimeout: time.Duration(runtimeConfig.SearchProviderTimeoutSeconds) * time.Second,
		FetchMaxResponseBytes: int64(runtimeConfig.FetchMaxResponseKB) * 1024,
		FetchMaxContentChars:  runtimeConfig.FetchMaxContentChars,
	})
	if err != nil {
		return nil, fmt.Errorf("create web access client: %w", err)
	}

	searchTool, err := agent.InferTool[webSearchToolInput, webaccess.SearchResponse](
		config.AgentToolWebSearch,
		webSearchToolDescription,
		func(ctx context.Context, input webSearchToolInput) (webaccess.SearchResponse, error) {
			response, err := client.Search(ctx, webaccess.SearchRequest{
				Query: input.Query, TimeRange: input.TimeRange, MaxResults: input.MaxResults,
			})
			if err != nil {
				return webaccess.SearchResponse{}, fmt.Errorf("web_search failed: %w", err)
			}
			return response, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create web_search tool: %w", err)
	}
	definedSearchTool, err := defineTool(searchTool, boundedReadDescriptor(ToolSourceWeb, config.AgentToolWebSearch))
	if err != nil {
		return nil, err
	}

	fetchTool, err := agent.InferTool[webFetchToolInput, webaccess.FetchResponse](
		webFetchToolName,
		webFetchToolDescription,
		func(ctx context.Context, input webFetchToolInput) (webaccess.FetchResponse, error) {
			response, err := client.Fetch(ctx, webaccess.FetchRequest{
				URL: input.URL, StartIndex: input.StartIndex, MaxChars: input.MaxChars,
			})
			if err != nil {
				return webaccess.FetchResponse{}, fmt.Errorf("web_fetch failed: %w", err)
			}
			return response, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create web_fetch tool: %w", err)
	}
	definedFetchTool, err := defineTool(fetchTool, boundedReadDescriptor(ToolSourceWeb, config.AgentToolWebSearch))
	if err != nil {
		return nil, err
	}
	return []agent.BaseTool{definedSearchTool, definedFetchTool}, nil
}

type webSearchToolInput struct {
	Query      string `json:"query" jsonschema:"required" jsonschema_description:"Required web search query."`
	TimeRange  string `json:"time_range,omitempty" jsonschema:"enum=day,enum=week,enum=month,enum=year" jsonschema_description:"Optional best-effort freshness filter: day, week, month, or year."`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"minimum=1,maximum=20" jsonschema_description:"Optional result count. The configured maximum is always enforced."`
}

type webFetchToolInput struct {
	URL        string `json:"url" jsonschema:"required" jsonschema_description:"Absolute public HTTP(S) URL returned by web_search or otherwise supplied by the user."`
	StartIndex int    `json:"start_index,omitempty" jsonschema:"minimum=0" jsonschema_description:"Unicode character offset to continue a truncated page; defaults to 0."`
	MaxChars   int    `json:"max_chars,omitempty" jsonschema:"minimum=1" jsonschema_description:"Optional maximum Unicode characters for this page fragment. The configured hard limit is always enforced."`
}
