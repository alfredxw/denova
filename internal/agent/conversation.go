package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/session"
)

// Conversation 抽象 Agent 对话的上下文读取与结果写入。
// 写作模式写入普通 session，互动模式可写入 interactive/story。
type Conversation interface {
	PrepareMessages(originalMessage, agentMessage string) ([]*schema.Message, error)
	AppendAssistant(content string) error
	MarkInterrupted(userMessage, assistantContent, reason string) error
	PendingInterruption() *session.Interruption
	ResolveInterruption(id string) error
}

// ContextSourceReporter 可由 Conversation 提供本轮已拼装的业务上下文来源。
// ChatService 会在 PrepareMessages 后追加打印，便于排查非通用注入内容。
type ContextSourceReporter interface {
	ContextSourceSummary() string
}

type SessionConversation struct {
	session             *session.Session
	cfg                 *config.Config
	agentKind           string
	runtimeContextTitle string
	runtimeContext      string
}

func NewSessionConversation(sess *session.Session, options ...SessionConversationOption) *SessionConversation {
	c := &SessionConversation{session: sess}
	for _, option := range options {
		if option != nil {
			option(c)
		}
	}
	return c
}

func NewSessionConversationForAgent(sess *session.Session, cfg *config.Config, agentKind string) *SessionConversation {
	return NewSessionConversation(
		sess,
		WithSessionContextConfig(cfg, agentKind),
	)
}

func NewSessionConversationForAgentWithRuntimeContext(sess *session.Session, cfg *config.Config, agentKind, title, content string) *SessionConversation {
	return NewSessionConversation(
		sess,
		WithSessionContextConfig(cfg, agentKind),
		WithSessionRuntimeContext(title, content),
	)
}

type SessionConversationOption func(*SessionConversation)

func WithSessionContextConfig(cfg *config.Config, agentKind string) SessionConversationOption {
	return func(c *SessionConversation) {
		c.cfg = cfg
		c.agentKind = agentKind
	}
}

func WithSessionRuntimeContext(title, content string) SessionConversationOption {
	return func(c *SessionConversation) {
		c.runtimeContextTitle = title
		c.runtimeContext = content
	}
}

func (c *SessionConversation) PrepareMessages(originalMessage, agentMessage string) ([]*schema.Message, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("会话不存在")
	}
	if err := c.session.Append(schema.UserMessage(originalMessage)); err != nil {
		return nil, err
	}
	return c.modelMessages(appendRuntimeContextToAgentMessage(agentMessage, c.runtimeContextTitle, c.runtimeContext)), nil
}

func (c *SessionConversation) ContextSourceSummary() string {
	if c == nil || strings.TrimSpace(c.runtimeContext) == "" {
		return ""
	}
	title := strings.TrimSpace(c.runtimeContextTitle)
	if title == "" {
		title = "本轮动态上下文"
	}
	contextLog := newContextBuildLog()
	contextLog.add("本轮动态上下文", title, c.runtimeContext, "appended_to_final_user_message")
	return contextLog.String()
}

func (c *SessionConversation) CompactContextIfNeeded(ctx context.Context, input ContextCompactionInput) ([]*schema.Message, ContextCompactionResult, error) {
	policy := c.compactionPolicy()
	phase := strings.TrimSpace(input.Phase)
	if phase == "" {
		phase = contextCompactionPhasePreRun
	}
	tokensBefore := EstimateContextTokens(input.Messages, input.Tools)
	result := ContextCompactionResult{
		Phase:               phase,
		TokensBefore:        tokensBefore,
		ContextWindowTokens: policy.ContextWindowTokens,
		Threshold:           policy.Threshold,
		MessageCountBefore:  len(input.Messages),
		RetainedTurns:       policy.RetainedTurns,
	}
	shouldCompact, skipped := policy.shouldCompact(tokensBefore, input.Force)
	if !shouldCompact {
		result.SkippedReason = skipped
		return input.Messages, result, nil
	}
	source, existingMemory, sourceStart, sourceEnd := c.compactionIncrementalSource(input.KeepLatestUser)
	if strings.TrimSpace(input.ExistingMemory) != "" {
		existingMemory = input.ExistingMemory
	}
	if len(source) == 0 && strings.TrimSpace(existingMemory) == "" && strings.TrimSpace(input.ReferenceContext) == "" {
		result.SkippedReason = "empty_source"
		return input.Messages, result, nil
	}
	if !input.Force {
		if removal, ok := c.session.LatestContextCompactionRemoval(c.agentKind); ok && removal.SourceStartIndex == sourceStart && removal.SourceEndIndex >= sourceEnd {
			result.SkippedReason = "removed_same_source"
			return input.Messages, result, nil
		}
	}
	sourceTokens := EstimateContextTokens(source, nil)
	emitContextCompactionEvent(input.Emit, phase, "started", result)
	summary, inputChars, err := summarizeContextForCompaction(ctx, c.cfg, c.agentKind, existingMemory, source, input.ReferenceContext, sourceTokens, policy, func(attempt int, delta string) {
		emitContextCompactionDeltaEvent(input.Emit, phase, result, attempt, delta)
	})
	if err != nil {
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	epoch := c.nextCompactionEpoch()
	newMessages := compactMessagesForModel(input.Messages, summary, epoch, policy.RetainedTurns)
	result.Triggered = true
	result.Epoch = epoch
	result.Summary = summary
	result.TokensAfter = EstimateContextTokens(newMessages, input.Tools)
	result.TargetRatio = contextCompactionRatio(countRunes(summary), inputChars)
	result.SourceMessageCount = len(source)
	result.MessageCountAfter = len(newMessages)
	record := contextCompactionRecordFromResult(result, c.agentKind, sourceStart, sourceEnd, policy.RetainedTurns, summary)
	record, err = c.session.AppendContextCompaction(record)
	if err != nil {
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	if record.Epoch != epoch {
		result.Epoch = record.Epoch
		newMessages = compactMessagesForModel(input.Messages, summary, record.Epoch, policy.RetainedTurns)
		result.TokensAfter = EstimateContextTokens(newMessages, input.Tools)
		result.MessageCountAfter = len(newMessages)
	}
	emitContextCompactionEvent(input.Emit, phase, "completed", result)
	return newMessages, result, nil
}

func (c *SessionConversation) modelMessages(agentMessage string) []*schema.Message {
	history := append([]*schema.Message(nil), c.session.GetEffectiveMessages()...)
	policy := c.compactionPolicy()
	if compaction, ok := c.session.LatestContextCompaction(c.agentKind); ok && strings.TrimSpace(compaction.Summary) != "" {
		total := c.session.MessageCountTotal()
		effectiveStart := total - len(history)
		retainedTurns := compaction.RetainedTurns
		if retainedTurns <= 0 {
			retainedTurns = policy.RetainedTurns
		}
		tail := compactedMessagesAfterSource(history, effectiveStart, compaction.SourceEndIndex, retainedTurns)
		history = make([]*schema.Message, 0, 1+len(tail))
		history = append(history, NewContextCompactionSummaryMessage(compaction.Epoch, compaction.Summary))
		history = append(history, tail...)
	}
	if len(history) > 0 {
		history[len(history)-1] = schema.UserMessage(agentMessage)
	}
	return history
}

func appendRuntimeContextToAgentMessage(agentMessage, title, content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return agentMessage
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "本轮动态上下文"
	}
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(agentMessage))
	sb.WriteString("\n\n---\n\n# ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString("以下内容来自当前 workspace 的有界状态快照，随作品进展变化，只用于本轮判断；用户本轮请求仍以上方正文为准。需要更完整或最新内容时，按来源路径使用工具读取确认。\n\n")
	sb.WriteString(content)
	return sb.String()
}

func (c *SessionConversation) compactionPolicy() contextCompactionPolicy {
	if c == nil {
		return contextCompactionPolicy{}
	}
	agentKind := c.agentKind
	if strings.TrimSpace(agentKind) == "" {
		agentKind = config.AgentKindIDE
	}
	policy := resolveContextCompactionPolicy(c.cfg, agentKind)
	return policy
}

func (c *SessionConversation) nextCompactionEpoch() int {
	return c.session.NextContextCompactionEpoch(c.agentKind)
}

func (c *SessionConversation) compactionIncrementalSource(keepLatestUser bool) ([]*schema.Message, string, int, int) {
	if c == nil || c.session == nil {
		return nil, "", 0, 0
	}
	messages := c.session.GetMessages()
	total := len(messages)
	sourceStart := total - c.session.MessageCountSinceClear()
	if sourceStart < 0 {
		sourceStart = 0
	}
	existingMemory := ""
	if compaction, ok := c.session.LatestContextCompaction(c.agentKind); ok {
		existingMemory = compaction.Summary
		if compaction.SourceEndIndex > sourceStart {
			sourceStart = compaction.SourceEndIndex
		}
	}
	if sourceStart > total {
		sourceStart = total
	}
	sourceEnd := total
	if !keepLatestUser && sourceEnd > sourceStart {
		sourceEnd--
	}
	if sourceEnd < sourceStart {
		sourceEnd = sourceStart
	}
	source := compactionSourceMessages(messages[sourceStart:sourceEnd], true)
	return source, existingMemory, sourceStart, sourceEnd
}

func compactionSourceMessages(messages []*schema.Message, keepLatestUser bool) []*schema.Message {
	source := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if isContextCompactionMessage(msg) {
			continue
		}
		source = append(source, sanitizeCompactionSourceMessage(msg))
	}
	if !keepLatestUser && len(source) > 0 && source[len(source)-1].Role == schema.User {
		source = source[:len(source)-1]
	}
	return source
}

func sanitizeCompactionSourceMessage(msg *schema.Message) *schema.Message {
	if msg == nil {
		return nil
	}
	copied := *msg
	copied.ReasoningContent = ""
	return &copied
}

func retainTailByUserTurns(messages []*schema.Message, retainedTurns int) []*schema.Message {
	if retainedTurns <= 0 {
		retainedTurns = config.DefaultContextCompactionRetainedTurns
	}
	if retainedTurns > config.MaxContextCompactionRetainedTurns {
		retainedTurns = config.MaxContextCompactionRetainedTurns
	}
	userCount := 0
	start := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] == nil || messages[i].Role != schema.User {
			continue
		}
		userCount++
		if userCount == retainedTurns {
			start = i
			break
		}
	}
	if userCount < retainedTurns {
		return messages
	}
	return append([]*schema.Message(nil), messages[start:]...)
}

func (c *SessionConversation) AppendAssistant(content string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.Append(schema.AssistantMessage(content, nil))
}

func (c *SessionConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.AppendDisplayEvent(event)
}

func (c *SessionConversation) UpdateDisplayToolStatus(id, name, status string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.UpdateDisplayToolStatus(id, name, status)
}

func (c *SessionConversation) AppendDisplayToolArgs(id, name, delta string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.AppendDisplayToolArgs(id, name, delta)
}

func (c *SessionConversation) UpdateDisplayToolResult(id, name, status, result string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.UpdateDisplayToolResult(id, name, status, result)
}

func (c *SessionConversation) MarkInterrupted(userMessage, assistantContent, reason string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.MarkInterrupted(userMessage, assistantContent, reason)
}

func (c *SessionConversation) PendingInterruption() *session.Interruption {
	if c == nil || c.session == nil {
		return nil
	}
	return c.session.PendingInterruption()
}

func (c *SessionConversation) ResolveInterruption(id string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.ResolveInterruption(id)
}
