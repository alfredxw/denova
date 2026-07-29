package agents

import (
	"log"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

// ToolResultContextPolicy controls whether stable tool receipts can cross user
// turns. Rich result bodies remain available inside the source run only.
type ToolResultContextPolicy struct {
	AgentKind      string
	Enabled        bool
	MaxResultBytes int
}

func resolveToolResultContextPolicy(cfg *config.Config, agentKind string) ToolResultContextPolicy {
	settings := config.ResolveAgentContext(cfg, agentKind)
	return ToolResultContextPolicy{
		AgentKind: strings.TrimSpace(agentKind), Enabled: settings.ToolResultRetentionEnabled,
		MaxResultBytes: configToolResultMaxBytes(cfg),
	}
}

func ResolveToolResultContextPolicyForConversation(cfg *config.Config, agentKind string) ToolResultContextPolicy {
	return resolveToolResultContextPolicy(cfg, agentKind)
}

func (p ToolResultContextPolicy) normalized() ToolResultContextPolicy {
	p.MaxResultBytes = normalizeToolResultLimitBytes(p.MaxResultBytes)
	return p
}

type toolResultContextConversation interface {
	AppendContextMessages(messages ...*agent.Message) error
	ToolResultContextPolicy() ToolResultContextPolicy
}

type pendingToolContextCall struct {
	call  agent.ToolCall
	valid bool
}

type toolResultContextRecorder struct {
	conversation toolResultContextConversation
	policy       ToolResultContextPolicy
	pending      map[string]pendingToolContextCall
}

func newToolResultContextRecorder(conversation Conversation) *toolResultContextRecorder {
	contextConversation, ok := conversation.(toolResultContextConversation)
	if !ok || contextConversation == nil {
		return &toolResultContextRecorder{}
	}
	policy := contextConversation.ToolResultContextPolicy().normalized()
	if !policy.Enabled {
		return &toolResultContextRecorder{}
	}
	return &toolResultContextRecorder{conversation: contextConversation, policy: policy}
}

// RecordAssistantToolCalls stages calls in memory. A call and its stable result
// receipt are committed together only after execution reaches a terminal state.
func (r *toolResultContextRecorder) RecordAssistantToolCalls(msg *agent.Message, meta agentEventMetadata) {
	if r == nil || r.conversation == nil || meta.SubAgent || msg == nil || len(msg.ToolCalls) == 0 {
		return
	}
	if r.pending == nil {
		r.pending = make(map[string]pendingToolContextCall, len(msg.ToolCalls))
	}
	for _, call := range msg.ToolCalls {
		callID := strings.TrimSpace(call.ID)
		if callID == "" || normalizeToolName(call.Function.Name) == "" {
			continue
		}
		if existing, found := r.pending[callID]; found {
			existing.valid = false
			r.pending[callID] = existing
			continue
		}
		r.pending[callID] = pendingToolContextCall{
			call: call, valid: validateToolArgumentsJSON(call.Function.Arguments) == nil,
		}
	}
}

func (r *toolResultContextRecorder) RecordToolResult(message *agent.Message, meta agentEventMetadata) {
	if r == nil || r.conversation == nil || meta.SubAgent || message == nil {
		return
	}
	callID := strings.TrimSpace(message.ToolCallID)
	pending, ok := r.pending[callID]
	delete(r.pending, callID)
	if !ok || !pending.valid {
		return
	}
	retention := toolMessageRetention(message)
	if retention == "" {
		retention = legacyToolContextRetention(pending.call.Function.Name, r.policy)
	}
	if isUnknownToolEffectResult(message.Content) {
		retention = agent.ToolContextReceipt
	}
	if retention != agent.ToolContextReceipt {
		return
	}

	arguments := retainedArgumentsFromResult(message)
	if arguments == "" {
		arguments = projectRetainedToolArguments(pending.call.Function.Arguments, min(r.policy.MaxResultBytes, retainedContextProjectionBytes))
	}
	if arguments == "" || validateToolArgumentsJSON(arguments) != nil {
		return
	}
	pending.call.Function.Arguments = arguments
	result := stableRetainedToolMessage(message, pending.call.Function.Name, r.policy)
	if result == nil || strings.TrimSpace(result.Content) == "" {
		return
	}
	if err := r.conversation.AppendContextMessages(agent.AssistantMessage("", []agent.ToolCall{pending.call}), result); err != nil {
		log.Printf("[agent-run] persist tool receipt pair failed tool=%s call_id=%s err=%v", pending.call.Function.Name, callID, err)
	}
}

func toolMessageRetention(message *agent.Message) agent.ToolContextRetention {
	if message == nil || message.ToolResult == nil {
		return ""
	}
	return message.ToolResult.ContextRetention
}

func retainedArgumentsFromResult(message *agent.Message) string {
	if message == nil || message.ToolResult == nil {
		return ""
	}
	return strings.TrimSpace(message.ToolResult.RetainedArguments)
}

func stableRetainedToolMessage(message *agent.Message, toolName string, policy ToolResultContextPolicy) *agent.Message {
	if message == nil || message.Role != agent.ToolRole {
		return nil
	}
	next := message.Clone()
	next.ToolName = normalizeToolName(toolName)
	content := ""
	if isUnknownToolEffectResult(next.Content) {
		content = next.Content
	} else if next.ToolResult != nil {
		content = strings.TrimSpace(next.ToolResult.RetainedContent)
	}
	if content == "" {
		content = toolResultContextContent(next.ToolName, next.Content, policy)
	}
	next.Content = content
	if next.ToolResult != nil {
		next.ToolResult.ContextRetention = agent.ToolContextReceipt
		next.ToolResult.RetainedContent = ""
		next.ToolResult.RetainedArguments = ""
	}
	return next
}

func applyToolResultContextPolicy(messages []*agent.Message, policy ToolResultContextPolicy) []*agent.Message {
	if len(messages) == 0 {
		return messages
	}
	return filterToolContextMessages(completeUnknownToolResults(messages), policy.normalized())
}

func filterToolContextMessages(messages []*agent.Message, policy ToolResultContextPolicy) []*agent.Message {
	type callProjection struct {
		call      agent.ToolCall
		valid     bool
		results   int
		retain    bool
		arguments string
		content   string
	}
	calls := make(map[string]callProjection)
	for _, message := range messages {
		if message == nil || message.Role != agent.Assistant {
			continue
		}
		for _, call := range message.ToolCalls {
			callID := strings.TrimSpace(call.ID)
			if callID == "" || normalizeToolName(call.Function.Name) == "" {
				continue
			}
			if existing, found := calls[callID]; found {
				existing.valid = false
				calls[callID] = existing
				continue
			}
			calls[callID] = callProjection{call: call, valid: validateToolArgumentsJSON(call.Function.Arguments) == nil}
		}
	}
	for _, message := range messages {
		if message == nil || message.Role != agent.ToolRole {
			continue
		}
		callID := strings.TrimSpace(message.ToolCallID)
		projection, found := calls[callID]
		if !found {
			continue
		}
		projection.results++
		retention := toolMessageRetention(message)
		if retention == "" {
			retention = legacyToolContextRetention(projection.call.Function.Name, policy)
		}
		projection.retain = isUnknownToolEffectResult(message.Content) || (policy.Enabled && retention == agent.ToolContextReceipt)
		projection.arguments = retainedArgumentsFromResult(message)
		if projection.arguments == "" {
			projection.arguments = projectRetainedToolArguments(projection.call.Function.Arguments, min(policy.MaxResultBytes, retainedContextProjectionBytes))
		}
		if stable := stableRetainedToolMessage(message, projection.call.Function.Name, policy); stable != nil {
			projection.content = stable.Content
		}
		calls[callID] = projection
	}

	filtered := make([]*agent.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		switch message.Role {
		case agent.Assistant:
			if len(message.ToolCalls) == 0 {
				filtered = append(filtered, message)
				continue
			}
			next := message.Clone()
			next.ToolCalls = nil
			for _, call := range message.ToolCalls {
				projection, found := calls[strings.TrimSpace(call.ID)]
				if !found || !projection.valid || projection.results != 1 || !projection.retain || projection.arguments == "" {
					continue
				}
				call.Function.Arguments = projection.arguments
				next.ToolCalls = append(next.ToolCalls, call)
			}
			if len(next.ToolCalls) > 0 || strings.TrimSpace(next.Content) != "" {
				filtered = append(filtered, next)
			}
		case agent.ToolRole:
			projection, found := calls[strings.TrimSpace(message.ToolCallID)]
			if !found || !projection.valid || projection.results != 1 || !projection.retain || strings.TrimSpace(projection.content) == "" {
				continue
			}
			next := message.Clone()
			next.ToolName = normalizeToolName(projection.call.Function.Name)
			next.Content = projection.content
			if next.ToolResult != nil {
				next.ToolResult.RetainedContent = ""
				next.ToolResult.RetainedArguments = ""
			}
			filtered = append(filtered, next)
		default:
			filtered = append(filtered, message)
		}
	}
	return filtered
}

// legacyToolContextRetention is read-only migration behavior for histories
// written before ToolDescriptor.ContextRetention existed. New calls never use
// tool-name whitelists.
func legacyToolContextRetention(toolName string, policy ToolResultContextPolicy) agent.ToolContextRetention {
	name := normalizeToolName(toolName)
	if strings.TrimSpace(policy.AgentKind) == config.AgentKindInteractiveStory {
		if name == "read_lore_items" {
			return agent.ToolContextReceipt
		}
		return agent.ToolContextTransient
	}
	switch name {
	case "list_lore_items", "search_story_history":
		return agent.ToolContextTransient
	default:
		return agent.ToolContextReceipt
	}
}

func toolResultContextContent(toolName, content string, policy ToolResultContextPolicy) string {
	if isRetainedToolReceipt(content) {
		return content
	}
	manifest := unknownToolManifest(toolName)
	if normalizeToolName(toolName) == "read_lore_items" {
		manifest.Source = agent.ToolSourceLore
	}
	result := agent.TextToolResult(content)
	result.Metadata.OriginalModelBytes = len(content)
	return projectRetainedToolReceipt(manifest, result, min(policy.normalized().MaxResultBytes, retainedContextProjectionBytes))
}

func ApplyToolResultContextPolicyForConversation(messages []*agent.Message, policy ToolResultContextPolicy) []*agent.Message {
	return applyToolResultContextPolicy(messages, policy)
}
