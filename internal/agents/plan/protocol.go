// Package plan owns the model-to-display protocol for a reviewable final plan.
// Interactive clarification is handled by the ordinary durable Ask tool.
package plan

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	proposalOpenTag  = "<proposed_plan>"
	proposalCloseTag = "</proposed_plan>"
	proposalEvent    = "proposed_plan"
)

// ToolEventID is the stable display identity shared by protocol tool calls.
const ToolEventID = "plan_protocol_tool"

// Event is the bounded plan-card event emitted by Parser. The chat transport
// adapts it to its general event envelope at the package seam.
type Event struct {
	Type string
	Data map[string]any
}

// Metadata identifies the run source of a plan event.
type Metadata struct {
	AgentKind         string
	RunID             string
	AgentName         string
	RootAgentName     string
	RunPath           []string
	SubAgent          bool
	SubAgentSessionID string
	SubAgentType      string
}

func (m Metadata) appendTo(data map[string]any) map[string]any {
	if data == nil {
		data = map[string]any{}
	}
	if m.AgentName != "" {
		data["agent_name"] = m.AgentName
	}
	if m.AgentKind != "" {
		data["agent_kind"] = m.AgentKind
	}
	if m.RunID != "" {
		data["run_id"] = m.RunID
	}
	if m.RootAgentName != "" {
		data["root_agent_name"] = m.RootAgentName
	}
	if len(m.RunPath) > 0 {
		data["run_path"] = append([]string(nil), m.RunPath...)
	}
	if m.SubAgentSessionID != "" {
		data["subagent_session_id"] = m.SubAgentSessionID
	}
	if m.SubAgentType != "" {
		data["subagent_type"] = m.SubAgentType
	}
	data["subagent"] = m.SubAgent
	return data
}

// Parser incrementally removes proposed-plan blocks from assistant text and
// emits them as structured display events.
type Parser struct {
	emit func(Event)
	meta Metadata

	buffer      strings.Builder
	inProposal  bool
	blockID     string
	blockSeq    int
	blockBuffer strings.Builder

	successfulBlocks int
}

// NewParser creates a streaming proposed-plan parser.
func NewParser(meta Metadata, emit func(Event)) *Parser {
	if emit == nil {
		emit = func(Event) {}
	}
	return &Parser{emit: emit, meta: meta}
}

func (p *Parser) Push(content string) string {
	if p == nil || content == "" {
		return content
	}
	p.buffer.WriteString(content)
	return p.drain(false)
}

func (p *Parser) Flush() string {
	if p == nil {
		return ""
	}
	return p.drain(true)
}

func (p *Parser) HasSuccessfulBlock() bool {
	return p != nil && p.successfulBlocks > 0
}

func (p *Parser) NoteSuccessfulBlock() {
	if p != nil {
		p.successfulBlocks++
	}
}

func (p *Parser) drain(flush bool) string {
	var visible strings.Builder
	for {
		buffer := p.buffer.String()
		if buffer == "" {
			if flush && p.inProposal {
				p.emitPlanBlock("error", "")
				visible.WriteString(proposalOpenTag)
				visible.WriteString(p.blockBuffer.String())
				p.resetBlock()
			}
			return visible.String()
		}

		if p.inProposal {
			if idx := strings.Index(buffer, proposalCloseTag); idx >= 0 {
				p.blockBuffer.WriteString(buffer[:idx])
				p.buffer.Reset()
				p.buffer.WriteString(buffer[idx+len(proposalCloseTag):])
				p.emitPlanBlock("success", normalizeBlockDisplay(p.blockBuffer.String()))
				p.resetBlock()
				continue
			}
			if flush {
				p.emitPlanBlock("error", "")
				visible.WriteString(proposalOpenTag)
				visible.WriteString(p.blockBuffer.String())
				visible.WriteString(buffer)
				p.buffer.Reset()
				p.resetBlock()
				return visible.String()
			}
			retain := longestTagPrefixSuffix(buffer, proposalCloseTag)
			if len(buffer) > retain {
				p.blockBuffer.WriteString(buffer[:len(buffer)-retain])
				p.buffer.Reset()
				p.buffer.WriteString(buffer[len(buffer)-retain:])
			}
			return visible.String()
		}

		if idx := strings.Index(buffer, proposalOpenTag); idx >= 0 {
			visible.WriteString(buffer[:idx])
			p.buffer.Reset()
			p.buffer.WriteString(buffer[idx+len(proposalOpenTag):])
			p.inProposal = true
			p.blockSeq++
			p.blockID = fmt.Sprintf("%s-%d", proposalEvent, p.blockSeq)
			p.emitPlanBlock("running", "")
			continue
		}
		if flush {
			visible.WriteString(buffer)
			p.buffer.Reset()
			return visible.String()
		}
		retain := longestTagPrefixSuffix(buffer, proposalOpenTag)
		if len(buffer) > retain {
			visible.WriteString(buffer[:len(buffer)-retain])
			p.buffer.Reset()
			p.buffer.WriteString(buffer[len(buffer)-retain:])
		}
		return visible.String()
	}
}

func (p *Parser) resetBlock() {
	p.inProposal = false
	p.blockID = ""
	p.blockBuffer.Reset()
}

func (p *Parser) emitPlanBlock(status, content string) {
	if p == nil || !p.inProposal {
		return
	}
	if status == "success" {
		p.successfulBlocks++
	}
	data := map[string]any{"id": p.blockID, "status": status}
	if content != "" {
		data["content"] = content
	}
	p.emit(Event{Type: proposalEvent, Data: p.meta.appendTo(data)})
}

func longestTagPrefixSuffix(content, tag string) int {
	limit := len(tag) - 1
	if len(content) < limit {
		limit = len(content)
	}
	for length := limit; length > 0; length-- {
		if strings.HasSuffix(content, tag[:length]) {
			return length
		}
	}
	return 0
}

func normalizeBlockDisplay(content string) string {
	return strings.TrimSpace(content)
}

// IsToolName reports whether a tool call belongs to the final-plan display
// protocol. Plan questions deliberately use the ordinary Ask tool instead.
func IsToolName(name string) bool {
	return strings.TrimSpace(name) == proposalEvent
}

// EmitToolRunning publishes the running state for a proposed-plan tool call.
func EmitToolRunning(name string, meta Metadata, emit func(Event)) bool {
	if emit == nil || !IsToolName(name) {
		return false
	}
	emit(Event{Type: proposalEvent, Data: meta.appendTo(map[string]any{
		"id": ToolEventID, "status": "running",
	})})
	return true
}

// EmitToolCall projects a proposed-plan protocol tool call into a completed
// plan card. The tool form remains accepted for provider compatibility even
// though the canonical prompt asks for a tagged assistant block.
func EmitToolCall(name, args string, meta Metadata, emit func(Event)) (bool, bool) {
	if emit == nil || !IsToolName(name) {
		return false, false
	}
	content := normalizeBlockDisplay(extractProposedPlanToolContent(strings.TrimSpace(args)))
	if content == "" {
		EmitToolRunning(name, meta, emit)
		return true, false
	}
	emit(Event{Type: proposalEvent, Data: meta.appendTo(map[string]any{
		"id": ToolEventID, "status": "success", "content": content,
	})})
	return true, true
}

func extractProposedPlanToolContent(args string) string {
	if args == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return args
	}
	for _, key := range []string{"content", "plan", "markdown", "proposal", "summary"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return args
}
