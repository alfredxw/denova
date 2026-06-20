package agent

import (
	"fmt"

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
	return NewSessionConversation(sess, WithSessionRecentTurns(config.ResolveAgentContext(cfg, agentKind).RecentTurns))
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

func (c *SessionConversation) PrepareMessages(originalMessage, agentMessage string) ([]*schema.Message, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("会话不存在")
	}
	if err := c.session.Append(schema.UserMessage(originalMessage)); err != nil {
		return nil, err
	}
	history := append([]*schema.Message(nil), c.session.GetEffectiveMessages()...)
	if len(history) > 0 {
		history[len(history)-1] = schema.UserMessage(agentMessage)
	}
	return limitMessagesByRecentTurns(history, c.recentTurns), nil
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
