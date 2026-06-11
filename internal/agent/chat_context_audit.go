package agent

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"nova/internal/prompts"
)

// appendPlanModeInstruction 在用户消息前追加规划模式指令，允许读取文件但禁止写操作，只输出结构化计划。
func appendPlanModeInstruction(message string) string {
	return prompts.PlanMode(message)
}

// appendContextBoundaryInstruction 在用户消息前追加上下文边界说明，
// 强调当前请求才是"这次要做什么"，工作区/已确认小说状态是"背景是什么"，
// 历史对话只能用于辅助理解，不能直接成为本轮执行依据。
func appendContextBoundaryInstruction(message string) string {
	return prompts.ContextBoundary(message)
}

type contextBuildLog struct {
	parts []contextLogPart
}

type contextLogPart struct {
	Source  string
	Title   string
	Content string
	Note    string
}

type contextAuditPart struct {
	Source  string `json:"source"`
	Title   string `json:"title"`
	Bytes   int    `json:"bytes"`
	Chars   int    `json:"chars"`
	Preview string `json:"preview"`
	Note    string `json:"note,omitempty"`
}

func newContextBuildLog() *contextBuildLog {
	return &contextBuildLog{parts: []contextLogPart{}}
}

func (l *contextBuildLog) add(source, title, content, note string) {
	if l == nil {
		return
	}
	source = strings.TrimSpace(source)
	title = strings.TrimSpace(title)
	if source == "" && title == "" && strings.TrimSpace(content) == "" {
		return
	}
	l.parts = append(l.parts, contextLogPart{
		Source:  source,
		Title:   title,
		Content: content,
		Note:    strings.TrimSpace(note),
	})
}

func (l *contextBuildLog) addStyleRules(rules []StyleRule) {
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if scene == "" || len(rule.Styles) == 0 {
			continue
		}
		styles := trimmedNonEmpty(rule.Styles)
		if len(styles) == 0 {
			continue
		}
		l.add("注入规则", "场景化默认风格规则："+scene, strings.Join(styles, "、"), "Agent 将按场景自行判断是否 read_file")
	}
}

func (l *contextBuildLog) addSelections(selections []TextSelectionRef) {
	for _, sel := range selections {
		title := strings.TrimSpace(sel.FileName)
		if title == "" {
			title = "未命名选区"
		}
		if sel.StartLine > 0 || sel.EndLine > 0 {
			title = fmt.Sprintf("%s:L%d-L%d", title, sel.StartLine, sel.EndLine)
		}
		l.add("编辑器选区", title, sel.Content, "")
	}
}

func (l *contextBuildLog) String() string {
	if l == nil || len(l.parts) == 0 {
		return "count=0"
	}
	parts := make([]string, 0, len(l.parts))
	for i, part := range l.parts {
		content := strings.TrimSpace(part.Content)
		fields := []string{
			fmt.Sprintf("%d:source=%q", i, part.Source),
			fmt.Sprintf("title=%q", part.Title),
			"bytes=" + intString(len(content)),
			"chars=" + intString(utf8.RuneCountInString(content)),
			"preview=" + strconv.Quote(safeLogPreview(content, 100)),
		}
		if part.Note != "" {
			fields = append(fields, "note="+strconv.Quote(part.Note))
		}
		parts = append(parts, strings.Join(fields, ","))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(l.parts), strings.Join(parts, "; "))
}

func (l *contextBuildLog) Audit() []contextAuditPart {
	if l == nil || len(l.parts) == 0 {
		return nil
	}
	parts := make([]contextAuditPart, 0, len(l.parts))
	for _, part := range l.parts {
		content := strings.TrimSpace(part.Content)
		parts = append(parts, contextAuditPart{
			Source:  part.Source,
			Title:   part.Title,
			Bytes:   len(content),
			Chars:   utf8.RuneCountInString(content),
			Preview: safeLogPreview(content, 100),
			Note:    part.Note,
		})
	}
	return parts
}

func addContextLog(logs []*contextBuildLog, source, title, content, note string) {
	for _, l := range logs {
		if l != nil {
			l.add(source, title, content, note)
		}
	}
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

func messageListSummary(messages []*schema.Message) string {
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

func messageSummary(index, total int, msg *schema.Message) string {
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

// EventError 创建标准错误事件。
func EventError(err error) Event {
	return Event{Type: "error", Data: map[string]string{"message": fmt.Sprint(err)}}
}
