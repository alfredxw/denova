package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/agent"
	agentcontext "denova/internal/agent/context"
	"denova/internal/book"
	"denova/internal/interactive"
)

// interactiveContextSource is a transient description of one domain fragment.
// Only bounded metadata from these values is persisted in the run ledger.
type interactiveContextSource struct {
	Source    string
	Title     string
	Purpose   string
	Content   string
	Note      string
	Limit     int
	Truncated bool

	// MetadataOnly identifies useful story metadata that was not placed in the
	// final model-visible message list.
	MetadataOnly bool
	// ExactMessage prevents a compaction summary that merely paraphrases an old
	// turn from making the original message look retained.
	ExactMessage bool
}

func (c *interactiveConversation) stableLeadingMessageSnapshot() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stableLeadingMessage
}

// preserveInteractiveStableLeadingMessage keeps complete resident Lore outside
// the compactable history tail, mirroring the stable-prefix behavior used by
// writing-mode sessions.
func preserveInteractiveStableLeadingMessage(messages []*schema.Message, content string) []*schema.Message {
	content = strings.TrimSpace(content)
	if content == "" {
		return messages
	}
	result := make([]*schema.Message, 0, len(messages)+1)
	result = append(result, schema.UserMessage(content))
	for _, msg := range messages {
		if msg != nil && msg.Role == schema.User && strings.TrimSpace(msg.Content) == content {
			continue
		}
		result = append(result, msg)
	}
	return result
}

func interactiveCompactionResultForMessages(result agent.ContextCompactionResult, messages []*schema.Message, tools []*schema.ToolInfo) agent.ContextCompactionResult {
	previousTokens := result.TokensAfter
	result.TokensAfter = agent.EstimateContextTokens(messages, tools)
	result.ProjectedTokensAfter += result.TokensAfter - previousTokens
	if result.ProjectedTokensAfter < result.TokensAfter {
		result.ProjectedTokensAfter = result.TokensAfter
	}
	result.MessageCountAfter = len(messages)
	return result
}

func interactiveStoryContextSources(title, origin string, teller interactive.Teller, storyMemory, directorPlanVisible, residentLore, loreRevision, loreRuntime, ruleSummary, actorStateRuntime, strategyPrompt string, turnMemory interactiveTurnMemory, userAction string) []interactiveContextSource {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title, Note: "metadata_only", MetadataOnly: true},
		{Source: "互动故事", Title: "开端", Content: origin, Note: "metadata_only", MetadataOnly: true},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "turn_context")...)
	if strings.TrimSpace(storyMemory) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "故事记忆", Title: "当前分支可见故事记忆", Content: storyMemory,
			Note: "bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(directorPlanVisible) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "DirectorPlan", Title: "后台导演规划可读区", Content: directorPlanVisible,
			Note: "bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(residentLore) != "" {
		parts = append(parts, interactiveContextSource{
			Source:  "ResidentLore",
			Title:   "已启用常驻 Lore 正文",
			Purpose: "stable leading model context",
			Content: residentLore,
			Note:    fmt.Sprintf("complete=true; source=enabled resident lore; body_max_bytes=%d; revision=%s", book.ResidentLoreSafetyMaxBytes, strings.TrimSpace(loreRevision)),
			Limit:   interactiveResidentLoreMessageMaxBytes,
		})
	}
	if strings.TrimSpace(loreRuntime) != "" {
		parts = append(parts, interactiveContextSource{
			Source:  "LoreContext",
			Title:   "当前分支活动资料工作集",
			Purpose: "turn-scoped active lore context",
			Content: loreRuntime,
			Note:    "complete=true; source=lore-context.md active references",
			Limit:   interactiveResolvedLoreContextMaxBytes,
		})
	}
	if strings.TrimSpace(ruleSummary) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "StoryDirector", Title: "故事导演规则清单", Content: ruleSummary,
			Note: "bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(actorStateRuntime) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "ActorState", Title: "当前 Actor 状态与词条", Purpose: "turn-scoped state snapshot",
			Content: actorStateRuntime, Note: "source=Snapshot.State.actors; bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(strategyPrompt) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "StoryDirector.strategy.prompt_markdown", Title: "故事导演 Markdown 策略提示", Content: strategyPrompt,
			Note: "bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(turnMemory.PreviousSummary) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "历史回合", Title: fmt.Sprintf("较早 %d 回合压缩摘要", turnMemory.PreviousCount),
			Content: turnMemory.PreviousSummary, Note: "compressed", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	for i, turn := range turnMemory.Turns {
		parts = append(parts,
			interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("第 %d 回合用户行动", i+1), Content: turn.User, ExactMessage: true},
			interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("第 %d 回合剧情", i+1), Content: turn.Narrative, ExactMessage: true},
		)
	}
	parts = append(parts, interactiveContextSource{Source: "本轮行动", Title: "当前用户行动", Content: userAction})
	return parts
}

func interactiveContextLedgerParts(parts []interactiveContextSource, messages []*schema.Message, policy agent.ToolResultContextPolicy) []agent.ContextLedgerPart {
	ledger := agent.NewContextLedger(agent.DefaultLoopPolicy().ContextLedger)
	for _, part := range parts {
		matchedMessage, visible := interactiveContextSourceMessage(part, messages)
		included := !part.MetadataOnly && visible
		truncated := part.Truncated
		note := part.Note
		auditContent := part.Content
		limit := part.Limit
		if included && part.Source == "ResidentLore" {
			// Resident Lore is injected as a standalone message with a title and
			// provenance note. Audit that exact model-visible value so bytes and
			// hash can be reconciled with the final request after compaction.
			auditContent = matchedMessage
			bodyBytes := len([]byte(strings.TrimSpace(part.Content)))
			wrapperBytes := len([]byte(strings.TrimSpace(matchedMessage))) - bodyBytes
			if wrapperBytes < 0 {
				wrapperBytes = 0
			}
			note = joinInteractiveContextNote(note, fmt.Sprintf("wrapper_bytes=%d; message_max_bytes=%d; exact_final_message=true", wrapperBytes, part.Limit))
		}
		if !part.MetadataOnly && strings.TrimSpace(part.Content) != "" && !included {
			truncated = true
			note = joinInteractiveContextNote(note, "not_present_after_final_compaction")
		}
		ledger.AddPart(part.Source, part.Title, part.Purpose, auditContent, note, included, truncated, limit)
	}
	addFinalInteractiveMessageContextParts(ledger, messages, policy)
	return ledger.Parts()
}

func cloneInteractiveContextSources(parts []interactiveContextSource) []interactiveContextSource {
	if len(parts) == 0 {
		return nil
	}
	result := make([]interactiveContextSource, len(parts))
	copy(result, parts)
	return result
}

func interactiveContextSourceMessage(part interactiveContextSource, messages []*schema.Message) (string, bool) {
	content := strings.TrimSpace(part.Content)
	if content == "" {
		return "", false
	}
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		visible := strings.TrimSpace(msg.Content)
		if part.ExactMessage {
			if visible == content {
				return visible, true
			}
			continue
		}
		if strings.Contains(visible, content) {
			return visible, true
		}
	}
	return "", false
}

func joinInteractiveContextNote(existing, extra string) string {
	existing = strings.TrimSpace(existing)
	extra = strings.TrimSpace(extra)
	if existing == "" {
		return extra
	}
	if extra == "" || strings.Contains(existing, extra) {
		return existing
	}
	return existing + "; " + extra
}

func addFinalInteractiveMessageContextParts(ledger *agent.ContextLedger, messages []*schema.Message, policy agent.ToolResultContextPolicy) {
	policy = normalizeInteractiveToolResultContextPolicy(policy)
	for index, msg := range messages {
		if msg == nil {
			continue
		}
		if agent.IsContextCompactionSummaryMessage(msg) {
			ledger.AddPart(
				"ContextCompaction", fmt.Sprintf("模型可见压缩摘要 %d", index+1), "model-visible compacted history",
				msg.Content, "source=committed context compaction; final_message=true", true, false, interactiveStoryRuntimeContextBytes,
			)
		}
		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			for _, call := range msg.ToolCalls {
				data, _ := json.Marshal(call)
				toolName := strings.TrimSpace(call.Function.Name)
				toolID := strings.TrimSpace(call.ID)
				note := fmt.Sprintf("tool_name=%s; tool_call_id=%s; args_preview_chars=%d; hard_limit=%d; limit_unit=chars; limit_scope=arguments; final_message=true", toolName, toolID, policy.PreviewChars, policy.PreviewChars)
				ledger.AddPartWithLimitUnit(
					"历史工具上下文", interactiveToolContextTitle("工具调用", toolName, toolID), "paired cross-turn tool call",
					string(data), note, true, interactiveToolContextTruncated(call.Function.Arguments), policy.PreviewChars, "chars",
				)
			}
		}
		if msg.Role == schema.Tool {
			toolName := strings.TrimSpace(msg.ToolName)
			toolID := strings.TrimSpace(msg.ToolCallID)
			note := fmt.Sprintf("tool_name=%s; tool_call_id=%s; semantic_filtered=true; total_budget_bytes=%d; final_message=true", toolName, toolID, policy.BudgetBytes)
			ledger.AddPart(
				"历史工具上下文", interactiveToolContextTitle("工具结果", toolName, toolID), "paired cross-turn tool result",
				msg.Content, note, true, interactiveToolContextTruncated(msg.Content), policy.BudgetBytes,
			)
		}
	}
}

func normalizeInteractiveToolResultContextPolicy(policy agent.ToolResultContextPolicy) agent.ToolResultContextPolicy {
	if policy.KeepRecent <= 0 {
		policy.KeepRecent = config.DefaultToolResultKeepRecent
	}
	if policy.BudgetBytes <= 0 {
		policy.BudgetBytes = config.DefaultToolResultContextBudgetKB * 1024
	}
	if policy.PreviewChars <= 0 {
		policy.PreviewChars = config.DefaultToolResultPreviewChars
	}
	return policy
}

func interactiveToolContextTitle(kind, toolName, toolID string) string {
	identity := toolName
	if identity == "" {
		identity = "unknown_tool"
	}
	if toolID != "" {
		identity += " (" + toolID + ")"
	}
	return kind + " " + identity
}

func interactiveToolContextTruncated(content string) bool {
	return strings.Contains(content, "truncated for retained context") ||
		strings.Contains(content, "preview truncated for context") ||
		strings.Contains(content, "retained tool result placeholder")
}

func interactiveTellerSlotSources(teller interactive.Teller, targets ...string) []interactiveContextSource {
	allowed := make(map[string]bool, len(targets))
	for _, target := range targets {
		allowed[target] = true
	}
	parts := []interactiveContextSource{}
	for _, slot := range teller.Slots {
		if !slot.Enabled || !allowed[slot.Target] || strings.TrimSpace(slot.Content) == "" {
			continue
		}
		parts = append(parts, interactiveContextSource{
			Source: "导演注入规则", Title: fmt.Sprintf("%s（%s）", slot.Name, slot.Target),
			Content: slot.Content, Note: "teller=" + teller.ID,
		})
	}
	return parts
}

func interactiveTellerSlotSummary(teller interactive.Teller, targets ...string) string {
	sources := interactiveTellerSlotSources(teller, targets...)
	if len(sources) == 0 {
		return "count=0"
	}
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Title)
	}
	return fmt.Sprintf("count=%d names=%q", len(names), names)
}

func interactiveContextSourceListSummary(parts []interactiveContextSource) string {
	sources := make([]agentcontext.Source, 0, len(parts))
	for _, part := range parts {
		sources = append(sources, agentcontext.Source{
			Source: part.Source, Title: part.Title, Purpose: part.Purpose, Content: part.Content,
			Placement: agentcontext.PlacementAuditOnly, Limit: part.Limit, Included: !part.MetadataOnly,
			Truncated: part.Truncated, Note: part.Note,
		})
	}
	return agentcontext.SourceSummary(sources, agentcontext.DefaultPreviewChars)
}
