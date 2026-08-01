package conversation

import (
	"context"
	"fmt"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

const maxInstructionStableContextTitleBytes = 512

type InstructionOptions struct {
	Instruction           string
	StableContextTitle    string
	StableContext         string
	StableContextMaxBytes int
	ContextBudget         agentcontext.Budget
}

// InstructionConversation is a storage-free one-request conversation with an
// optional bounded stable prefix. Director execution and context diagnostics
// share it so their provider-visible layouts cannot drift.
type InstructionConversation struct {
	instruction           string
	stableContextTitle    string
	stableContext         string
	stableContextMaxBytes int
	contextBudget         agentcontext.Budget

	mu          sync.Mutex
	lastContext agentcontext.Result
	output      string
}

func NewInstructionConversation(options InstructionOptions) *InstructionConversation {
	return &InstructionConversation{
		instruction: options.Instruction, stableContextTitle: options.StableContextTitle,
		stableContext: options.StableContext, stableContextMaxBytes: options.StableContextMaxBytes,
		contextBudget: options.ContextBudget,
	}
}

func (c *InstructionConversation) ModelContextBudget() agentcontext.Budget {
	if c == nil {
		return agentcontext.DefaultBudget()
	}
	return c.contextBudget
}

func (c *InstructionConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	message := strings.TrimSpace(input.UserMessage)
	if message == "" {
		message = c.instruction
	}
	stable := strings.TrimSpace(c.stableContext)
	fragments := append([]agentcontext.Fragment(nil), input.Fragments...)
	if stable != "" {
		if c.stableContextMaxBytes <= 0 {
			return agentcontext.ModelContextResult{}, fmt.Errorf("稳定模型上下文缺少大小上限")
		}
		if len([]byte(stable)) > c.stableContextMaxBytes {
			return agentcontext.ModelContextResult{}, fmt.Errorf("稳定模型上下文超过上限: %d > %d bytes", len([]byte(stable)), c.stableContextMaxBytes)
		}
		title := strings.TrimSpace(c.stableContextTitle)
		if title == "" {
			title = "稳定模型上下文"
		}
		if len([]byte(title)) > maxInstructionStableContextTitleBytes {
			return agentcontext.ModelContextResult{}, fmt.Errorf("稳定模型上下文标题超过上限: %d > %d bytes", len([]byte(title)), maxInstructionStableContextTitleBytes)
		}
		stableMessage := agentcontext.StandaloneMessage(title, stable, "")
		if len([]byte(stableMessage)) > c.stableContextMaxBytes {
			return agentcontext.ModelContextResult{}, fmt.Errorf("稳定模型上下文最终消息超过上限: %d > %d bytes", len([]byte(stableMessage)), c.stableContextMaxBytes)
		}
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_director_resident_lore", Source: "interactive.director.resident_lore", Title: title,
			Purpose: "provide complete enabled resident lore as the director's stable model prefix",
			Content: stable, Placement: agentcontext.PlacementLeadingMessage, Limit: c.stableContextMaxBytes, Included: true,
			Note: "source=enabled resident lore; complete=true",
		})
	}
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage(message)}, Fragments: fragments,
	})
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	for _, fragment := range assembled.Fragments {
		if fragment.Source == "interactive.director.resident_lore" && (!fragment.Included || fragment.Truncated) {
			return agentcontext.ModelContextResult{}, fmt.Errorf("稳定模型上下文超过 Agent 上下文注入预算；请提高单片段和单轮总注入上限")
		}
	}
	return agentcontext.ModelContextResult{
		Messages: assembled.Messages, Context: assembled,
		CommitState: instructionContextCommitState{context: assembled},
	}, nil
}

type instructionContextCommitState struct{ context agentcontext.Result }

func (c *InstructionConversation) CommitModelInput(ctx context.Context, _ string, assembled agentcontext.ModelContextResult) error {
	if c == nil {
		return fmt.Errorf("instruction conversation is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state, ok := assembled.CommitState.(instructionContextCommitState)
	if !ok {
		return fmt.Errorf("instruction model context is missing commit state")
	}
	c.mu.Lock()
	c.lastContext = state.context
	c.mu.Unlock()
	return nil
}

func (c *InstructionConversation) ContextSourceSummary() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	fragments := append([]agentcontext.Fragment(nil), c.lastContext.Fragments...)
	c.mu.Unlock()
	if len(fragments) > 0 {
		sources := make([]agentcontext.Source, 0, len(fragments))
		for _, fragment := range fragments {
			sources = append(sources, agentcontext.Source{
				Source: fragment.Source, Title: fragment.Title, Purpose: fragment.Purpose,
				Content: fragment.Content, Placement: fragment.Placement, Limit: fragment.Limit,
				Included: fragment.Included, Truncated: fragment.Truncated, Note: fragment.Note,
			})
		}
		return agentcontext.SourceSummary(sources, agentcontext.DefaultPreviewChars)
	}
	if strings.TrimSpace(c.stableContext) == "" {
		return ""
	}
	return fmt.Sprintf("stable_context title=%q max_bytes=%d content=%s", strings.TrimSpace(c.stableContextTitle), c.stableContextMaxBytes, prompts.PartSummary(c.stableContext))
}

func (c *InstructionConversation) ContextLedgerParts() []agentcontext.AuditPart {
	if c == nil || strings.TrimSpace(c.stableContext) == "" {
		return nil
	}
	return c.ContextLedgerPartsForMessages([]*agent.Message{agent.UserMessage(c.stableContextModelMessage())})
}

func (c *InstructionConversation) ContextLedgerPartsForMessages(messages []*agent.Message) []agentcontext.AuditPart {
	if c == nil || strings.TrimSpace(c.stableContext) == "" {
		return nil
	}
	stableMessage := c.stableContextModelMessage()
	included := false
	for _, message := range messages {
		if message != nil && message.Role == agent.User && strings.TrimSpace(message.Content) == stableMessage {
			included = true
			break
		}
	}
	ledger := agentcontext.NewAuditLedger(agentrun.DefaultLoopPolicy().ContextLedger)
	title := strings.TrimSpace(c.stableContextTitle)
	if title == "" {
		title = "稳定模型上下文"
	}
	bodyBytes := len([]byte(strings.TrimSpace(c.stableContext)))
	messageBytes := len([]byte(stableMessage))
	note := fmt.Sprintf("complete=true; source=enabled resident lore; body_bytes=%d; message_bytes=%d; message_max_bytes=%d; final_message=true", bodyBytes, messageBytes, c.stableContextMaxBytes)
	if !included {
		note += "; not_present_after_final_compaction"
	}
	ledger.AddPart("ResidentLore", title, "stable model prefix", stableMessage, note, included, !included, c.stableContextMaxBytes)
	return ledger.Parts()
}

func (c *InstructionConversation) stableContextModelMessage() string {
	stable := strings.TrimSpace(c.stableContext)
	if stable == "" {
		return ""
	}
	title := strings.TrimSpace(c.stableContextTitle)
	if title == "" {
		title = "稳定模型上下文"
	}
	return agentcontext.StandaloneMessage(title, stable, "")
}

func (c *InstructionConversation) AppendAssistant(content string) error {
	c.mu.Lock()
	c.output = content
	c.mu.Unlock()
	return nil
}

func (c *InstructionConversation) MarkInterrupted(_, assistantContent, _ string) error {
	return c.AppendAssistant(assistantContent)
}

func (*InstructionConversation) PendingInterruption() *session.Interruption { return nil }
func (*InstructionConversation) ResolveInterruption(string) error           { return nil }

func (c *InstructionConversation) Output() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(c.output)
}
