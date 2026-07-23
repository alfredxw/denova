package agent

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	adk "github.com/alfredxw/denova/adk"

	agentcontext "denova/internal/agent/context"
)

type contextBuildLog struct {
	ledger *ContextLedger
	parts  []ContextAnalysisPart
}

func newContextBuildLog(policies ...ContextLedgerPolicy) *contextBuildLog {
	policy := DefaultLoopPolicy().ContextLedger
	if len(policies) > 0 {
		policy = policies[0]
	}
	return &contextBuildLog{ledger: NewContextLedger(policy)}
}

func (l *contextBuildLog) add(source, title, content, note string) {
	if l == nil {
		return
	}
	l.ledger.Add(source, title, content, note)
	l.parts = append(l.parts, NewContextAnalysisPart(ContextAnalysisPartInput{
		Source:  source,
		Title:   title,
		Content: content,
		Note:    note,
	}))
}

func (l *contextBuildLog) addFragment(fragment agentcontext.Fragment) {
	if l == nil {
		return
	}
	l.ledger.AddPart(
		fragment.Source, fragment.Title, fragment.Purpose, fragment.Content,
		fragment.Note, fragment.Included, fragment.Truncated, fragment.Limit,
	)
	l.parts = append(l.parts, NewContextAnalysisPart(ContextAnalysisPartInput{
		ID:      fragment.ID,
		Source:  fragment.Source,
		Title:   fragment.Title,
		Content: fragment.Content,
		Note:    fragment.Note,
	}))
}

func contextBuildLogFromAssembly(policy ContextLedgerPolicy, originalMessage string, result agentcontext.Result) *contextBuildLog {
	log := newContextBuildLog(policy)
	log.add("用户输入", "本轮原始请求", originalMessage, "display history; not charged to injection budget")
	for _, fragment := range result.Fragments {
		log.addFragment(fragment)
	}
	return log
}

func (l *contextBuildLog) String() string {
	if l == nil || l.ledger == nil {
		return "count=0"
	}
	return l.ledger.Summary()
}

func (l *contextBuildLog) Audit() []ContextLedgerPart {
	if l == nil || l.ledger == nil {
		return nil
	}
	return l.ledger.Parts()
}

func (l *contextBuildLog) auditForMessages(messages []*adk.Message) []ContextLedgerPart {
	if l == nil || l.ledger == nil {
		return nil
	}
	ledger := NewContextLedger(l.ledger.policy)
	for _, part := range l.parts {
		content := strings.TrimSpace(part.Content)
		included := false
		if content != "" {
			for _, message := range messages {
				if message != nil && strings.Contains(strings.TrimSpace(message.Content), content) {
					included = true
					break
				}
			}
		}
		note := part.Note
		if content != "" && !included {
			if note != "" {
				note += "; "
			}
			note += "not_present_after_final_compaction"
		}
		ledger.AddPart(part.Source, part.Title, "", part.Content, note, included, content != "" && !included, 0)
	}
	return ledger.Parts()
}

func (l *contextBuildLog) FullParts() []ContextAnalysisPart {
	if l == nil || len(l.parts) == 0 {
		return nil
	}
	result := make([]ContextAnalysisPart, len(l.parts))
	copy(result, l.parts)
	return result
}

func contextLedgerPartsForConversation(log *contextBuildLog, conversation Conversation, messages []*adk.Message) []ContextLedgerPart {
	parts := log.auditForMessages(messages)
	if reporter, ok := conversation.(FinalContextLedgerReporter); ok {
		return append(parts, reporter.ContextLedgerPartsForMessages(messages)...)
	}
	if reporter, ok := conversation.(ContextLedgerReporter); ok {
		parts = append(parts, reporter.ContextLedgerParts()...)
	}
	return parts
}

func trimmedNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func messageListSummary(messages []*adk.Message) string {
	if len(messages) == 0 {
		return "count=0"
	}
	roleCounts := make(map[string]int)
	totalBytes := 0
	totalChars := 0
	for _, msg := range messages {
		if msg == nil {
			roleCounts["<nil>"]++
			continue
		}
		role := fmt.Sprint(msg.Role)
		roleCounts[role]++
		totalBytes += len(msg.Content)
		totalChars += utf8.RuneCountInString(msg.Content)
	}

	parts := make([]string, 0, len(messages))
	for i, msg := range messages {
		parts = append(parts, messageSummary(i, len(messages), msg))
	}

	return fmt.Sprintf("count=%d roles=%s total_bytes=%d total_chars=%d parts=[%s]", len(messages), roleCountSummary(roleCounts), totalBytes, totalChars, strings.Join(parts, "; "))
}

func messageSummary(index, total int, msg *adk.Message) string {
	if msg == nil {
		return fmt.Sprintf("%d:<nil>", index)
	}
	source := "会话历史"
	if index == total-1 {
		source = "本轮增强后用户输入"
	}
	return fmt.Sprintf("%d:source=%s role=%s(%s)", index, source, msg.Role, promptPartSummary(msg.Content))
}

func roleCountSummary(counts map[string]int) string {
	if len(counts) == 0 {
		return "{}"
	}
	roles := make([]string, 0, len(counts))
	for role := range counts {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	parts := make([]string, 0, len(roles))
	for _, role := range roles {
		parts = append(parts, fmt.Sprintf("%s:%d", role, counts[role]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func stringListSummary(values []string) string {
	if len(values) == 0 {
		return "count=0"
	}
	totalBytes := 0
	for _, value := range values {
		totalBytes += len(value)
	}
	display := values
	if len(display) > 6 {
		display = append(append([]string(nil), values[:3]...), append([]string{fmt.Sprintf("... omitted=%d ...", len(values)-6)}, values[len(values)-3:]...)...)
	}
	return fmt.Sprintf("count=%d total_bytes=%d items=%q", len(values), totalBytes, display)
}

func selectionListSummary(selections []TextSelectionRef) string {
	if len(selections) == 0 {
		return "count=0"
	}
	totalBytes := 0
	parts := make([]string, 0, minInt(len(selections), 6)+1)
	for i, sel := range selections {
		totalBytes += len(sel.Content)
		if i < 3 || i >= len(selections)-3 {
			parts = append(parts, fmt.Sprintf("%s:%d-%d(%s)", sel.FileName, sel.StartLine, sel.EndLine, promptPartSummary(sel.Content)))
		} else if i == 3 {
			parts = append(parts, fmt.Sprintf("... omitted=%d ...", len(selections)-6))
		}
	}
	return fmt.Sprintf("count=%d total_content_bytes=%d items=[%s]", len(selections), totalBytes, strings.Join(parts, "; "))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
