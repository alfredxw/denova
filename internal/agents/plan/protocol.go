// Package plan implements the model-to-display protocol used by planning
// agents. It owns both streamed tag parsing and tool-call projection so every
// caller produces the same plan events.
package plan

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	questionsOpenTag  = "<plan_questions>"
	questionsCloseTag = "</plan_questions>"
	proposalOpenTag   = "<proposed_plan>"
	proposalCloseTag  = "</proposed_plan>"
)

type blockKind string

const (
	blockQuestions blockKind = "plan_question"
	blockProposal  blockKind = "proposed_plan"
)

// ToolEventID is the stable display identity shared by protocol tool calls.
const ToolEventID = "plan_protocol_tool"

// Event is the bounded plan-card event emitted by Parser. The chat transport
// adapts it to its general event envelope at the package boundary.
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

// Parser incrementally removes plan protocol blocks from assistant text and
// emits them as structured display events.
type Parser struct {
	emit func(Event)
	meta Metadata

	buffer      strings.Builder
	block       blockKind
	blockID     string
	blockSeq    int
	blockBuffer strings.Builder

	successfulBlocks int
}

// NewParser creates a streaming plan protocol parser.
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
			if flush && p.block != "" {
				p.emitPlanBlock("error", "")
				visible.WriteString(openTag(p.block))
				visible.WriteString(p.blockBuffer.String())
				p.block = ""
				p.blockID = ""
				p.blockBuffer.Reset()
			}
			return visible.String()
		}

		if p.block != "" {
			closeTag := closeTag(p.block)
			if idx := strings.Index(buffer, closeTag); idx >= 0 {
				p.blockBuffer.WriteString(buffer[:idx])
				p.buffer.Reset()
				p.buffer.WriteString(buffer[idx+len(closeTag):])
				p.emitPlanBlock("success", normalizeBlockDisplay(p.blockBuffer.String()))
				p.block = ""
				p.blockID = ""
				p.blockBuffer.Reset()
				continue
			}
			if flush {
				p.emitPlanBlock("error", "")
				visible.WriteString(openTag(p.block))
				visible.WriteString(p.blockBuffer.String())
				visible.WriteString(buffer)
				p.buffer.Reset()
				p.block = ""
				p.blockID = ""
				p.blockBuffer.Reset()
				return visible.String()
			}
			retain := longestTagPrefixSuffix(buffer, []string{closeTag})
			if len(buffer) > retain {
				p.blockBuffer.WriteString(buffer[:len(buffer)-retain])
				p.buffer.Reset()
				p.buffer.WriteString(buffer[len(buffer)-retain:])
			}
			return visible.String()
		}

		kind, idx, openTag := nextOpenTag(buffer)
		if idx >= 0 {
			visible.WriteString(buffer[:idx])
			p.buffer.Reset()
			p.buffer.WriteString(buffer[idx+len(openTag):])
			p.block = kind
			p.blockID = p.nextBlockID(kind)
			p.emitPlanBlock("running", "")
			continue
		}
		if flush {
			visible.WriteString(buffer)
			p.buffer.Reset()
			return visible.String()
		}
		retain := longestTagPrefixSuffix(buffer, []string{questionsOpenTag, proposalOpenTag})
		if len(buffer) > retain {
			visible.WriteString(buffer[:len(buffer)-retain])
			p.buffer.Reset()
			p.buffer.WriteString(buffer[len(buffer)-retain:])
		}
		return visible.String()
	}
}

func (p *Parser) nextBlockID(kind blockKind) string {
	p.blockSeq++
	return fmt.Sprintf("%s-%d", kind, p.blockSeq)
}

func (p *Parser) emitPlanBlock(status string, content string) {
	if p == nil || p.block == "" {
		return
	}
	if status == "success" {
		p.successfulBlocks++
	}
	data := map[string]interface{}{
		"id":     p.blockID,
		"status": status,
	}
	if content != "" {
		data["content"] = content
	}
	p.emit(Event{Type: string(p.block), Data: p.meta.appendTo(data)})
}

func nextOpenTag(content string) (blockKind, int, string) {
	bestKind := blockKind("")
	bestIdx := -1
	bestTag := ""
	for _, candidate := range []struct {
		kind blockKind
		tag  string
	}{
		{kind: blockQuestions, tag: questionsOpenTag},
		{kind: blockProposal, tag: proposalOpenTag},
	} {
		idx := strings.Index(content, candidate.tag)
		if idx < 0 {
			continue
		}
		if bestIdx < 0 || idx < bestIdx {
			bestKind = candidate.kind
			bestIdx = idx
			bestTag = candidate.tag
		}
	}
	return bestKind, bestIdx, bestTag
}

func openTag(kind blockKind) string {
	switch kind {
	case blockQuestions:
		return questionsOpenTag
	case blockProposal:
		return proposalOpenTag
	default:
		return ""
	}
}

func closeTag(kind blockKind) string {
	switch kind {
	case blockQuestions:
		return questionsCloseTag
	case blockProposal:
		return proposalCloseTag
	default:
		return ""
	}
}

func longestTagPrefixSuffix(content string, tags []string) int {
	max := 0
	for _, tag := range tags {
		limit := len(tag) - 1
		if len(content) < limit {
			limit = len(content)
		}
		for n := limit; n > max; n-- {
			if strings.HasSuffix(content, tag[:n]) {
				max = n
				break
			}
		}
	}
	return max
}

func normalizeBlockDisplay(content string) string {
	return strings.TrimSpace(content)
}

func blockKindForToolName(name string) (blockKind, bool) {
	switch strings.TrimSpace(name) {
	case "plan_questions", "plan_question":
		return blockQuestions, true
	case "proposed_plan":
		return blockProposal, true
	default:
		return "", false
	}
}

// IsToolName reports whether a tool call belongs to the plan display protocol.
func IsToolName(name string) bool {
	_, ok := blockKindForToolName(name)
	return ok
}

// EmitToolRunning publishes the running state for a plan protocol tool.
func EmitToolRunning(name string, meta Metadata, emit func(Event)) bool {
	if emit == nil {
		return false
	}
	kind, ok := blockKindForToolName(name)
	if !ok {
		return false
	}
	emit(Event{Type: string(kind), Data: meta.appendTo(map[string]interface{}{
		"id":     ToolEventID,
		"status": "running",
	})})
	return true
}

// EmitToolCall projects a plan protocol tool call into a completed plan card.
func EmitToolCall(name, args string, meta Metadata, emit func(Event)) (bool, bool) {
	if emit == nil {
		return false, false
	}
	kind, ok := blockKindForToolName(name)
	if !ok {
		return false, false
	}
	content := normalizeBlockDisplay(toolContent(kind, args))
	if content == "" {
		EmitToolRunning(name, meta, emit)
		return true, false
	}
	data := meta.appendTo(map[string]interface{}{
		"id":      ToolEventID,
		"status":  "success",
		"content": content,
	})
	emit(Event{Type: string(kind), Data: data})
	return true, true
}

func toolContent(kind blockKind, args string) string {
	args = strings.TrimSpace(args)
	if kind == blockProposal {
		return extractProposedPlanToolContent(args)
	}
	return args
}

func extractProposedPlanToolContent(args string) string {
	if args == "" {
		return ""
	}
	var payload map[string]interface{}
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
