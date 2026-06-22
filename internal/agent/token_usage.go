package agent

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

type runTokenUsage struct {
	RunID                string              `json:"run_id,omitempty"`
	AgentKind            string              `json:"agent_kind,omitempty"`
	PromptTokens         int                 `json:"prompt_tokens,omitempty"`
	CachedPromptTokens   int                 `json:"cached_prompt_tokens,omitempty"`
	UncachedPromptTokens int                 `json:"uncached_prompt_tokens,omitempty"`
	CacheHitRate         float64             `json:"cache_hit_rate,omitempty"`
	CompletionTokens     int                 `json:"completion_tokens,omitempty"`
	ReasoningTokens      int                 `json:"reasoning_tokens,omitempty"`
	TotalTokens          int                 `json:"total_tokens,omitempty"`
	ModelCalls           int                 `json:"model_calls,omitempty"`
	GeneratedBytes       int                 `json:"generated_bytes,omitempty"`
	Calls                []runTokenUsageCall `json:"usage_calls,omitempty"`
}

type runTokenUsageCall struct {
	Index                int      `json:"index,omitempty"`
	CreatedAt            string   `json:"created_at,omitempty"`
	FinishReason         string   `json:"finish_reason,omitempty"`
	RequestedTools       []string `json:"requested_tools,omitempty"`
	AfterTools           []string `json:"after_tools,omitempty"`
	PromptTokens         int      `json:"prompt_tokens,omitempty"`
	CachedPromptTokens   int      `json:"cached_prompt_tokens,omitempty"`
	UncachedPromptTokens int      `json:"uncached_prompt_tokens,omitempty"`
	CacheHitRate         float64  `json:"cache_hit_rate,omitempty"`
	CompletionTokens     int      `json:"completion_tokens,omitempty"`
	ReasoningTokens      int      `json:"reasoning_tokens,omitempty"`
	TotalTokens          int      `json:"total_tokens,omitempty"`
}

type runTokenUsageCollector struct {
	runID      string
	agentKind  string
	stats      runTokenUsage
	afterTools []string
	emitted    bool
}

func newRunTokenUsageCollector(runID, agentKind string) *runTokenUsageCollector {
	return &runTokenUsageCollector{
		runID:     strings.TrimSpace(runID),
		agentKind: strings.TrimSpace(agentKind),
	}
}

func (c *runTokenUsageCollector) AddMessage(msg *schema.Message) {
	if c == nil || msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	usage := msg.ResponseMeta.Usage
	if usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 && usage.TotalTokens <= 0 {
		return
	}
	c.stats.ModelCalls++
	call := runTokenUsageCall{
		Index:                c.stats.ModelCalls,
		CreatedAt:            time.Now().UTC().Format(time.RFC3339Nano),
		FinishReason:         strings.TrimSpace(msg.ResponseMeta.FinishReason),
		RequestedTools:       toolNamesFromCalls(msg.ToolCalls),
		AfterTools:           append([]string(nil), c.afterTools...),
		PromptTokens:         usage.PromptTokens,
		CachedPromptTokens:   usage.PromptTokenDetails.CachedTokens,
		UncachedPromptTokens: uncachedPromptTokens(usage.PromptTokens, usage.PromptTokenDetails.CachedTokens),
		CompletionTokens:     usage.CompletionTokens,
		ReasoningTokens:      usage.CompletionTokensDetails.ReasoningTokens,
		TotalTokens:          usage.TotalTokens,
	}
	c.afterTools = nil
	if call.PromptTokens > 0 {
		call.CacheHitRate = roundRatio(float64(call.CachedPromptTokens) / float64(call.PromptTokens))
	}
	c.stats.Calls = append(c.stats.Calls, call)
	c.stats.PromptTokens += usage.PromptTokens
	c.stats.CachedPromptTokens += usage.PromptTokenDetails.CachedTokens
	c.stats.UncachedPromptTokens += call.UncachedPromptTokens
	c.stats.CompletionTokens += usage.CompletionTokens
	c.stats.ReasoningTokens += usage.CompletionTokensDetails.ReasoningTokens
	c.stats.TotalTokens += usage.TotalTokens
}

func (c *runTokenUsageCollector) NoteToolResult(name string) {
	if c == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	for _, existing := range c.afterTools {
		if existing == name {
			return
		}
	}
	c.afterTools = append(c.afterTools, name)
}

func (c *runTokenUsageCollector) EmitIfAny(emit func(Event), generatedBytes int) {
	if c == nil || c.emitted || c.stats.ModelCalls == 0 || emit == nil {
		return
	}
	c.emitted = true
	stats := c.stats
	stats.RunID = c.runID
	stats.AgentKind = c.agentKind
	stats.GeneratedBytes = generatedBytes
	if stats.PromptTokens > 0 {
		stats.CacheHitRate = roundRatio(float64(stats.CachedPromptTokens) / float64(stats.PromptTokens))
	}
	emit(Event{Type: "token_usage", Data: map[string]any{
		"created_at":             time.Now().UTC().Format(time.RFC3339Nano),
		"run_id":                 stats.RunID,
		"agent_kind":             stats.AgentKind,
		"prompt_tokens":          stats.PromptTokens,
		"cached_prompt_tokens":   stats.CachedPromptTokens,
		"uncached_prompt_tokens": stats.UncachedPromptTokens,
		"cache_hit_rate":         stats.CacheHitRate,
		"completion_tokens":      stats.CompletionTokens,
		"reasoning_tokens":       stats.ReasoningTokens,
		"total_tokens":           stats.TotalTokens,
		"model_calls":            stats.ModelCalls,
		"generated_bytes":        stats.GeneratedBytes,
		"usage_calls":            stats.Calls,
	}})
}

func toolNamesFromCalls(calls []schema.ToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, 0, len(calls))
	seen := make(map[string]bool, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func tokenUsageContent(stats runTokenUsage) string {
	if stats.PromptTokens <= 0 {
		return fmt.Sprintf("model_calls=%d total_tokens=%d", stats.ModelCalls, stats.TotalTokens)
	}
	return fmt.Sprintf(
		"cache_hit_rate=%.1f%% prompt_tokens=%d cached_prompt_tokens=%d uncached_prompt_tokens=%d total_tokens=%d model_calls=%d",
		stats.CacheHitRate*100,
		stats.PromptTokens,
		stats.CachedPromptTokens,
		stats.UncachedPromptTokens,
		stats.TotalTokens,
		stats.ModelCalls,
	)
}

func uncachedPromptTokens(promptTokens, cachedPromptTokens int) int {
	if promptTokens <= 0 {
		return 0
	}
	if cachedPromptTokens <= 0 {
		return promptTokens
	}
	if cachedPromptTokens >= promptTokens {
		return 0
	}
	return promptTokens - cachedPromptTokens
}

func roundRatio(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return math.Round(value*10000) / 10000
}
