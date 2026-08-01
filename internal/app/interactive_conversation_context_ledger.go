package app

import (
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"encoding/json"
	"fmt"
	"strings"

	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/book/lore"
	"denova/internal/interactive/teller"
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
func preserveInteractiveStableLeadingMessage(messages []*agents.Message, content string) []*agents.Message {
	return agentcompaction.PreserveLeadingMessage(messages, content)
}

func interactiveCompactionResultForMessages(result agentcompaction.Result, messages []*agents.Message, tools []*agents.ToolInfo) agentcompaction.Result {
	result = agentcompaction.RecalculateProjection(result, agentcontext.EstimateTokens(messages, tools))
	result.MessageCountAfter = len(messages)
	return result
}

func interactiveStoryContextSources(title, origin string, teller teller.Definition, historyCheckpoint, directorPlanVisible, residentLore, loreRevision, loreRuntime, ruleSummary, actorStateRuntime, stateSchemaInitialization, strategyPrompt string, turnHistory interactiveTurnHistory, userAction string) []interactiveContextSource {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title, Note: "metadata_only", MetadataOnly: true},
		{Source: "互动故事", Title: "开端", Content: origin, Note: "metadata_only", MetadataOnly: true},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "turn_context")...)
	if strings.TrimSpace(historyCheckpoint) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "HistoryCheckpoint", Title: "当前分支历史上下文 checkpoint", Content: historyCheckpoint,
			Purpose: "rebuildable context projection", Note: "source=committed turns; bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(directorPlanVisible) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "DirectorPlan", Title: "正文 Agent 简报", Content: directorPlanVisible,
			Note: "source=agent-brief.md; bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(residentLore) != "" {
		parts = append(parts, interactiveContextSource{
			Source:  "ResidentLore",
			Title:   "已启用常驻 Lore 正文",
			Purpose: "stable leading model context",
			Content: residentLore,
			Note:    fmt.Sprintf("complete=true; source=enabled resident lore; body_max_bytes=%d; revision=%s", lore.ResidentLoreSafetyMaxBytes, strings.TrimSpace(loreRevision)),
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
			Source: "ActorState", Title: "当前 Actor 状态手册", Purpose: "turn-scoped state write guide",
			Content: actorStateRuntime, Note: "source=effective Actor schema + Snapshot.State.actors; missing initial Actors projected in memory; bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(stateSchemaInitialization) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "StoryMeta.state_schema_policy", Title: "开局状态结构契约", Purpose: "opening-only schema initialization protocol",
			Content: stateSchemaInitialization, Note: "source=story policy + initialization status; bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(strategyPrompt) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "StoryDirector.strategy.prompt_markdown", Title: "故事导演 Markdown 策略提示", Content: strategyPrompt,
			Note: "bounded", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	if strings.TrimSpace(turnHistory.PreviousSummary) != "" {
		parts = append(parts, interactiveContextSource{
			Source: "历史回合", Title: fmt.Sprintf("较早 %d 回合历史检查点", turnHistory.PreviousCount),
			Content: turnHistory.PreviousSummary, Note: "compressed", Limit: interactiveStoryRuntimeContextBytes,
		})
	}
	for i, turn := range turnHistory.Turns {
		parts = append(parts,
			interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("第 %d 回合用户行动", i+1), Content: turn.User, ExactMessage: true},
			interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("第 %d 回合剧情", i+1), Content: turn.Narrative, ExactMessage: true},
		)
	}
	parts = append(parts, interactiveContextSource{Source: "本轮行动", Title: "当前用户行动", Content: userAction})
	return parts
}

func interactiveContextLedgerParts(parts []interactiveContextSource, messages []*agents.Message, policy toolresult.ContextPolicy) []agentcontext.AuditPart {
	ledger := agentcontext.NewAuditLedger(agentrun.DefaultLoopPolicy().ContextLedger)
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

// resolveInteractiveContextSources makes the semantic domain ledger a
// projection of the final assembled messages. A source that did not survive
// the hard assembly budget is retained as bounded metadata only; its original
// unbounded body is never reported as model-visible content.
func resolveInteractiveContextSources(parts []interactiveContextSource, messages []*agents.Message) []interactiveContextSource {
	resolved := cloneInteractiveContextSources(parts)
	for i := range resolved {
		part := &resolved[i]
		if part.MetadataOnly || strings.TrimSpace(part.Content) == "" {
			continue
		}
		if _, visible := interactiveContextSourceMessage(*part, messages); visible {
			continue
		}
		part.Content = ""
		part.Truncated = true
		part.Note = joinInteractiveContextNote(part.Note, "not_present_after_context_assembly")
	}
	return resolved
}

func cloneInteractiveContextSources(parts []interactiveContextSource) []interactiveContextSource {
	if len(parts) == 0 {
		return nil
	}
	result := make([]interactiveContextSource, len(parts))
	copy(result, parts)
	return result
}

func interactiveContextSourceMessage(part interactiveContextSource, messages []*agents.Message) (string, bool) {
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

func addFinalInteractiveMessageContextParts(ledger *agentcontext.AuditLedger, messages []*agents.Message, policy toolresult.ContextPolicy) {
	resultLimit := policy.MaxResultBytes
	for index, msg := range messages {
		if msg == nil {
			continue
		}
		if agentcontext.IsCompactionSummaryMessage(msg) {
			ledger.AddPart(
				"ContextCompaction", fmt.Sprintf("模型可见历史检查点 %d", index+1), "model-visible history checkpoint",
				msg.Content, "source=committed context compaction; final_message=true", true, false, interactiveStoryRuntimeContextBytes,
			)
		}
		if msg.Role == agents.RoleAssistant && len(msg.ToolCalls) > 0 {
			for _, call := range msg.ToolCalls {
				data, _ := json.Marshal(call)
				toolName := strings.TrimSpace(call.Function.Name)
				toolID := strings.TrimSpace(call.ID)
				note := fmt.Sprintf("tool_name=%s; tool_call_id=%s; source=model_tool_call; preserved_exactly=true; bounded_by=model_completion; final_message=true", toolName, toolID)
				ledger.AddPart(
					"历史工具上下文", interactiveToolContextTitle("工具调用", toolName, toolID), "paired cross-turn tool call",
					string(data), note, true, false, 0,
				)
			}
		}
		if msg.Role == agents.RoleTool {
			toolName := strings.TrimSpace(msg.ToolName)
			toolID := strings.TrimSpace(msg.ToolCallID)
			note := fmt.Sprintf("tool_name=%s; tool_call_id=%s; context_policy_applied=true; single_result_limit_bytes=%d; final_message=true", toolName, toolID, resultLimit)
			ledger.AddPart(
				"历史工具上下文", interactiveToolContextTitle("工具结果", toolName, toolID), "paired model-visible tool result",
				msg.Content, note, true, interactiveToolContextTruncated(msg.Content), resultLimit,
			)
		}
	}
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
	return strings.Contains(content, "[tool result truncated]") ||
		strings.Contains(content, `"schema":"tool_result.retained.v1"`)
}

func interactiveTellerSlotSources(teller teller.Definition, targets ...string) []interactiveContextSource {
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

func interactiveTellerSlotSummary(teller teller.Definition, targets ...string) string {
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

func interactiveContextSourceListSummary(parts []interactiveContextSource, fragments []agentcontext.Fragment) string {
	sources := make([]agentcontext.Source, 0, len(fragments)+len(parts))
	for _, fragment := range fragments {
		sources = append(sources, agentcontext.Source{
			Source: fragment.Source, Title: fragment.Title, Purpose: fragment.Purpose,
			Content: fragment.Content, Placement: fragment.Placement, Limit: fragment.Limit,
			Included: fragment.Included, Truncated: fragment.Truncated, Note: fragment.Note,
		})
	}
	for _, part := range parts {
		sources = append(sources, agentcontext.Source{
			Source: part.Source, Title: part.Title, Purpose: part.Purpose, Content: part.Content,
			Placement: agentcontext.PlacementAuditOnly, Limit: part.Limit, Included: !part.MetadataOnly,
			Truncated: part.Truncated, Note: part.Note,
		})
	}
	return agentcontext.SourceSummary(sources, agentcontext.DefaultPreviewChars)
}
