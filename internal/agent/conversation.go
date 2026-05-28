package agent

import (
	"fmt"

	"github.com/cloudwego/eino/schema"

	"nova/internal/session"
)

// Conversation 抽象 Agent 对话的上下文读取与结果写入。
// IDE 模式写入普通 session，互动模式可写入 interactive/story。
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
	session *session.Session
}

func NewSessionConversation(sess *session.Session) *SessionConversation {
	return &SessionConversation{session: sess}
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
	return history, nil
}

func (c *SessionConversation) AppendAssistant(content string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.Append(schema.AssistantMessage(content, nil))
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
