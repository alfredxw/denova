package chat

import (
	"context"
	agentconversation "denova/internal/agents/conversation"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

func TestContextLedgerPartsForConversationIncludesDomainFragments(t *testing.T) {
	log := newContextBuildLog(agentrun.DefaultLoopPolicy().ContextLedger)
	log.add("用户输入", "本轮原始请求", "推门", "")
	conversation := &contextLedgerReportingConversation{parts: []agentcontext.AuditPart{{
		Source: "LoreContext", Title: "常驻资料", Bytes: 128, Chars: 64, Included: true,
	}}}

	parts := contextLedgerPartsForConversation(log, conversation, []*agent.Message{agent.UserMessage("推门")})
	if len(parts) != 2 || parts[0].Source != "用户输入" || parts[1].Source != "LoreContext" {
		t.Fatalf("domain context fragments were not merged into the durable ledger: %#v", parts)
	}
}

func TestContextLedgerPartsForConversationUsesPostCompactionMessages(t *testing.T) {
	log := newContextBuildLog(agentrun.DefaultLoopPolicy().ContextLedger)
	log.add("文件引用", "removed.md", "压缩前引用正文", "")
	conversation := &finalContextLedgerReportingConversation{}
	finalMessages := []*agent.Message{
		agentcontext.NewCompactionSummaryMessage(2, "有界摘要"),
		agent.UserMessage("最终用户消息"),
	}

	parts := contextLedgerPartsForConversation(log, conversation, finalMessages)
	if len(parts) != 2 || parts[0].Source != "文件引用" || parts[0].Included || !parts[0].Truncated || !strings.Contains(parts[0].Note, "not_present_after_final_compaction") || parts[1].Source != "final_messages" || conversation.messageCount != len(finalMessages) || conversation.lastContent != "最终用户消息" {
		t.Fatalf("ledger reporter did not receive the post-compaction message list: parts=%#v conversation=%#v", parts, conversation)
	}
}

func TestSingleInstructionConversationReportsStablePrefixToContextLedger(t *testing.T) {
	conversation := agentconversation.NewInstructionConversation(agentconversation.InstructionOptions{
		StableContextTitle: "常驻资料（complete=true; revision=rev-1）",
		StableContext:      "世界规则正文", StableContextMaxBytes: 1024,
	})
	parts := conversation.ContextLedgerParts()
	if len(parts) != 1 || parts[0].Source != "ResidentLore" || parts[0].Limit != 1024 || parts[0].Hash == "" || !strings.Contains(parts[0].Note, "complete=true") || !strings.Contains(parts[0].Note, "message_max_bytes=1024") {
		t.Fatalf("stable resident prefix missing from durable ledger: %#v", parts)
	}
}

type contextLedgerReportingConversation struct {
	parts    []agentcontext.AuditPart
	metadata agentrun.TraceMetadata
}

func (c *contextLedgerReportingConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	return agentcontext.AssembleSingleUserModelContext(ctx, input)
}

type finalContextLedgerReportingConversation struct {
	contextLedgerReportingConversation
	messageCount int
	lastContent  string
}

func (c *finalContextLedgerReportingConversation) ContextLedgerPartsForMessages(messages []*agent.Message) []agentcontext.AuditPart {
	c.messageCount = len(messages)
	if len(messages) > 0 && messages[len(messages)-1] != nil {
		c.lastContent = messages[len(messages)-1].Content
	}
	return []agentcontext.AuditPart{{Source: "final_messages", Included: true}}
}

func (c *contextLedgerReportingConversation) AppendAssistant(string) error { return nil }
func (c *contextLedgerReportingConversation) MarkInterrupted(string, string, string) error {
	return nil
}
func (c *contextLedgerReportingConversation) PendingInterruption() *session.Interruption { return nil }
func (c *contextLedgerReportingConversation) ResolveInterruption(string) error           { return nil }
func (c *contextLedgerReportingConversation) ContextLedgerParts() []agentcontext.AuditPart {
	return append([]agentcontext.AuditPart(nil), c.parts...)
}
func (c *contextLedgerReportingConversation) RunTraceMetadata() agentrun.TraceMetadata {
	return c.metadata
}
