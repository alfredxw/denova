package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	planQuestionsOpenTag  = "<plan_questions>"
	planQuestionsCloseTag = "</plan_questions>"
	proposedPlanOpenTag   = "<proposed_plan>"
	proposedPlanCloseTag  = "</proposed_plan>"

	planBlockDisplayMaxBytes = 32 * 1024
	planBlockTruncatedHint   = "\n...\n[plan display truncated / Plan 展示已截断]"
)

type planBlockKind string

const (
	planBlockQuestions planBlockKind = "plan_question"
	planBlockProposal  planBlockKind = "proposed_plan"
)

const planProtocolToolEventID = "plan_protocol_tool"

type planProtocolParser struct {
	emit func(Event)
	meta agentEventMetadata

	buffer      strings.Builder
	block       planBlockKind
	blockID     string
	blockSeq    int
	blockBuffer strings.Builder

	successfulBlocks int
}

func newPlanProtocolParser(meta agentEventMetadata, emit func(Event)) *planProtocolParser {
	if emit == nil {
		emit = func(Event) {}
	}
	return &planProtocolParser{emit: emit, meta: meta}
}

func (p *planProtocolParser) Push(content string) string {
	if p == nil || content == "" {
		return content
	}
	p.buffer.WriteString(content)
	return p.drain(false)
}

func (p *planProtocolParser) Flush() string {
	if p == nil {
		return ""
	}
	return p.drain(true)
}

func (p *planProtocolParser) HasSuccessfulBlock() bool {
	return p != nil && p.successfulBlocks > 0
}

func (p *planProtocolParser) NoteSuccessfulBlock() {
	if p != nil {
		p.successfulBlocks++
	}
}

func (p *planProtocolParser) drain(flush bool) string {
	var visible strings.Builder
	for {
		buffer := p.buffer.String()
		if buffer == "" {
			if flush && p.block != "" {
				p.emitPlanBlock("error", "")
				visible.WriteString(openTagForPlanBlock(p.block))
				visible.WriteString(p.blockBuffer.String())
				p.block = ""
				p.blockID = ""
				p.blockBuffer.Reset()
			}
			return visible.String()
		}

		if p.block != "" {
			closeTag := closeTagForPlanBlock(p.block)
			if idx := strings.Index(buffer, closeTag); idx >= 0 {
				p.blockBuffer.WriteString(buffer[:idx])
				p.buffer.Reset()
				p.buffer.WriteString(buffer[idx+len(closeTag):])
				p.emitPlanBlock("success", truncatePlanBlockDisplay(p.blockBuffer.String()))
				p.block = ""
				p.blockID = ""
				p.blockBuffer.Reset()
				continue
			}
			if flush {
				p.emitPlanBlock("error", "")
				visible.WriteString(openTagForPlanBlock(p.block))
				visible.WriteString(p.blockBuffer.String())
				visible.WriteString(buffer)
				p.buffer.Reset()
				p.block = ""
				p.blockID = ""
				p.blockBuffer.Reset()
				return visible.String()
			}
			retain := longestPlanTagPrefixSuffix(buffer, []string{closeTag})
			if len(buffer) > retain {
				p.blockBuffer.WriteString(buffer[:len(buffer)-retain])
				p.buffer.Reset()
				p.buffer.WriteString(buffer[len(buffer)-retain:])
			}
			return visible.String()
		}

		kind, idx, openTag := nextPlanOpenTag(buffer)
		if idx >= 0 {
			visible.WriteString(buffer[:idx])
			p.buffer.Reset()
			p.buffer.WriteString(buffer[idx+len(openTag):])
			p.block = kind
			p.blockID = p.nextPlanBlockID(kind)
			p.emitPlanBlock("running", "")
			continue
		}
		if flush {
			visible.WriteString(buffer)
			p.buffer.Reset()
			return visible.String()
		}
		retain := longestPlanTagPrefixSuffix(buffer, []string{planQuestionsOpenTag, proposedPlanOpenTag})
		if len(buffer) > retain {
			visible.WriteString(buffer[:len(buffer)-retain])
			p.buffer.Reset()
			p.buffer.WriteString(buffer[len(buffer)-retain:])
		}
		return visible.String()
	}
}

func (p *planProtocolParser) nextPlanBlockID(kind planBlockKind) string {
	p.blockSeq++
	return fmt.Sprintf("%s-%d", kind, p.blockSeq)
}

func (p *planProtocolParser) emitPlanBlock(status string, content string) {
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

func nextPlanOpenTag(content string) (planBlockKind, int, string) {
	bestKind := planBlockKind("")
	bestIdx := -1
	bestTag := ""
	for _, candidate := range []struct {
		kind planBlockKind
		tag  string
	}{
		{kind: planBlockQuestions, tag: planQuestionsOpenTag},
		{kind: planBlockProposal, tag: proposedPlanOpenTag},
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

func openTagForPlanBlock(kind planBlockKind) string {
	switch kind {
	case planBlockQuestions:
		return planQuestionsOpenTag
	case planBlockProposal:
		return proposedPlanOpenTag
	default:
		return ""
	}
}

func closeTagForPlanBlock(kind planBlockKind) string {
	switch kind {
	case planBlockQuestions:
		return planQuestionsCloseTag
	case planBlockProposal:
		return proposedPlanCloseTag
	default:
		return ""
	}
}

func longestPlanTagPrefixSuffix(content string, tags []string) int {
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

func truncatePlanBlockDisplay(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= planBlockDisplayMaxBytes {
		return content
	}
	limit := planBlockDisplayMaxBytes - len(planBlockTruncatedHint)
	if limit < 0 {
		limit = planBlockDisplayMaxBytes
	}
	return truncateUTF8StringBytes(content, limit) + planBlockTruncatedHint
}

func truncateUTF8StringBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && (value[maxBytes]&0xC0) == 0x80 {
		maxBytes--
	}
	return value[:maxBytes]
}

func planBlockKindForToolName(name string) (planBlockKind, bool) {
	switch strings.TrimSpace(name) {
	case "plan_questions", "plan_question":
		return planBlockQuestions, true
	case "proposed_plan":
		return planBlockProposal, true
	default:
		return "", false
	}
}

func isPlanProtocolToolName(name string) bool {
	_, ok := planBlockKindForToolName(name)
	return ok
}

func emitPlanProtocolToolRunning(name string, meta agentEventMetadata, emit func(Event)) bool {
	if emit == nil {
		return false
	}
	kind, ok := planBlockKindForToolName(name)
	if !ok {
		return false
	}
	emit(Event{Type: string(kind), Data: meta.appendTo(map[string]interface{}{
		"id":     planProtocolToolEventID,
		"status": "running",
	})})
	return true
}

func emitPlanProtocolToolCall(name, args string, meta agentEventMetadata, emit func(Event)) (bool, bool) {
	if emit == nil {
		return false, false
	}
	kind, ok := planBlockKindForToolName(name)
	if !ok {
		return false, false
	}
	content := truncatePlanBlockDisplay(planProtocolToolContent(kind, args))
	if content == "" {
		emitPlanProtocolToolRunning(name, meta, emit)
		return true, false
	}
	data := meta.appendTo(map[string]interface{}{
		"id":      planProtocolToolEventID,
		"status":  "success",
		"content": content,
	})
	emit(Event{Type: string(kind), Data: data})
	return true, true
}

func planProtocolToolContent(kind planBlockKind, args string) string {
	args = strings.TrimSpace(args)
	if kind == planBlockProposal {
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
