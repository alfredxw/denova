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
	session     *session.Session
	recentTurns int
	cfg         *config.Config
	agentKind   string
}

func NewSessionConversation(sess *session.Session, options ...SessionConversationOption) *SessionConversation {
	c := &SessionConversation{session: sess, recentTurns: 30}
	for _, option := range options {
		if option != nil {
			option(c)
		}
	}
	return c
}

func NewSessionConversationForAgent(sess *session.Session, cfg *config.Config, agentKind string) *SessionConversation {
	contextSettings := config.ResolveAgentContext(cfg, agentKind)
	return NewSessionConversation(
		sess,
		WithSessionRecentTurns(contextSettings.RecentTurns),
		WithSessionContextConfig(cfg, agentKind),
	)
}

type SessionConversationOption func(*SessionConversation)

func WithSessionRecentTurns(recentTurns int) SessionConversationOption {
	return func(c *SessionConversation) {
		if recentTurns <= 0 {
			c.recentTurns = 30
			return
		}
		if recentTurns > 30 {
			recentTurns = 30
		}
		c.recentTurns = recentTurns
	}
}

func WithSessionContextConfig(cfg *config.Config, agentKind string) SessionConversationOption {
	return func(c *SessionConversation) {
		c.cfg = cfg
		c.agentKind = agentKind
	}
}

func (c *SessionConversation) PrepareMessages(originalMessage, agentMessage string) ([]*schema.Message, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("会话不存在")
	}
	if err := c.session.Append(schema.UserMessage(originalMessage)); err != nil {
		return nil, err
	}
	return c.modelMessages(agentMessage), nil
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
	}
	shouldCompact, skipped := policy.shouldCompact(tokensBefore, input.Force)
	if !shouldCompact {
		result.SkippedReason = skipped
		return input.Messages, result, nil
	}
	source := compactionSourceMessages(compactionSourceBaseMessages(input), input.KeepLatestUser)
	if len(source) == 0 {
		result.SkippedReason = "empty_source"
		return input.Messages, result, nil
	}
	sourceStart, sourceEnd := c.compactionSourceRange(input.KeepLatestUser)
	if !input.Force {
		if removal, ok := c.session.LatestContextCompactionRemoval(c.agentKind); ok && removal.SourceStartIndex == sourceStart && removal.SourceEndIndex >= sourceEnd {
			result.SkippedReason = "removed_same_source"
			return input.Messages, result, nil
		}
	}
	sourceTokens := EstimateContextTokens(source, nil)
	emitContextCompactionEvent(input.Emit, phase, "started", result)
	summary, err := summarizeContextForCompaction(ctx, c.cfg, c.agentKind, source, input.ReferenceContext, sourceTokens, policy)
	if err != nil {
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	epoch := c.nextCompactionEpoch()
	newMessages := compactMessagesForModel(input.Messages, summary, epoch, policy.RetainedRecentTurns)
	result.Triggered = true
	result.Epoch = epoch
	result.Summary = summary
	result.TokensAfter = EstimateContextTokens(newMessages, input.Tools)
	result.TargetRatio = contextCompactionRatio(estimateStringTokens(summary), sourceTokens)
	result.SourceMessageCount = len(source)
	result.MessageCountAfter = len(newMessages)
	record := contextCompactionRecordFromResult(result, c.agentKind, sourceStart, sourceEnd, policy.RetainedRecentTurns, summary)
	record, err = c.session.AppendContextCompaction(record)
	if err != nil {
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	if record.Epoch != epoch {
		result.Epoch = record.Epoch
		newMessages = compactMessagesForModel(input.Messages, summary, record.Epoch, policy.RetainedRecentTurns)
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
		tailStart := compaction.SourceEndIndex - effectiveStart
		if tailStart < 0 {
			tailStart = 0
		}
		if tailStart > len(history) {
			tailStart = len(history)
		}
		tail := limitMessagesByRecentTurns(history[tailStart:], policy.RetainedRecentTurns)
		history = make([]*schema.Message, 0, 1+len(tail))
		history = append(history, NewContextCompactionSummaryMessage(compaction.Epoch, compaction.Summary))
		history = append(history, tail...)
	} else if !policy.Enabled || policy.ContextWindowTokens <= 0 {
		history = limitMessagesByRecentTurns(history, c.recentTurns)
	}
	if len(history) > 0 {
		history[len(history)-1] = schema.UserMessage(agentMessage)
	}
	return history
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
	if policy.RetainedRecentTurns <= 0 {
		policy.RetainedRecentTurns = c.recentTurns
	}
	if policy.RetainedRecentTurns > 30 {
		policy.RetainedRecentTurns = 30
	}
	return policy
}

func (c *SessionConversation) nextCompactionEpoch() int {
	return c.session.NextContextCompactionEpoch(c.agentKind)
}

func (c *SessionConversation) compactionSourceRange(keepLatestUser bool) (int, int) {
	total := c.session.MessageCountTotal()
	effectiveCount := c.session.MessageCountSinceClear()
	sourceStart := total - effectiveCount
	sourceEnd := total
	if !keepLatestUser && sourceEnd > sourceStart {
		sourceEnd--
	}
	return sourceStart, sourceEnd
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

func limitMessagesByRecentTurns(messages []*schema.Message, recentTurns int) []*schema.Message {
	if recentTurns <= 0 {
		recentTurns = 30
	}
	if recentTurns > 30 {
		recentTurns = 30
	}
	userCount := 0
	start := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] == nil || messages[i].Role != schema.User {
			continue
		}
		userCount++
		if userCount == recentTurns {
			start = i
			break
		}
	}
	if userCount < recentTurns {
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
