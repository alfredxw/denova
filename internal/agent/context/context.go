package context

import (
	stdcontext "context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

type Placement string

const (
	PlacementLeadingMessage  Placement = "leading_message"
	PlacementFinalUserPrefix Placement = "final_user_prefix"
	PlacementAuditOnly       Placement = "audit_only"

	DefaultPreviewChars = 100
)

// Source is one bounded context fragment intentionally made visible to the model
// or recorded in the audit trail.
type Source struct {
	Source    string
	Title     string
	Purpose   string
	Content   string
	Placement Placement
	Limit     int
	Included  bool
	Truncated bool
	Note      string
}

type Request struct {
	Messages     []*schema.Message
	Sources      []Source
	Adapter      ModeAdapter
	PreviewChars int
}

// ModeAdapter lets IDE, interactive, automation, and future modes provide
// domain-specific context sources without owning final message placement.
type ModeAdapter interface {
	Sources(stdcontext.Context) ([]Source, error)
}

type ModeAdapterFunc func(stdcontext.Context) ([]Source, error)

func (fn ModeAdapterFunc) Sources(ctx stdcontext.Context) ([]Source, error) {
	return fn(ctx)
}

type Result struct {
	Messages      []*schema.Message
	Ledger        []LedgerPart
	AnalysisParts []AnalysisPart
}

type LedgerPart struct {
	Source    string `json:"source"`
	Title     string `json:"title"`
	Purpose   string `json:"purpose,omitempty"`
	Bytes     int    `json:"bytes"`
	Chars     int    `json:"chars"`
	Preview   string `json:"preview"`
	Note      string `json:"note,omitempty"`
	Included  bool   `json:"included"`
	Truncated bool   `json:"truncated,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type AnalysisPart struct {
	ID      string `json:"id,omitempty"`
	Source  string `json:"source"`
	Title   string `json:"title"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content"`
	Note    string `json:"note,omitempty"`
	Bytes   int    `json:"bytes"`
	Chars   int    `json:"chars"`
}

func Build(ctx stdcontext.Context, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	sources := append([]Source(nil), req.Sources...)
	if req.Adapter != nil {
		adapterSources, err := req.Adapter.Sources(ctx)
		if err != nil {
			return Result{}, err
		}
		sources = append(sources, adapterSources...)
	}
	previewChars := req.PreviewChars
	if previewChars <= 0 {
		previewChars = DefaultPreviewChars
	}
	messages := cloneMessages(req.Messages)
	var ledger []LedgerPart
	var analysis []AnalysisPart
	var leadingSources []Source
	var finalUserSources []Source
	for _, source := range sources {
		source = normalizeSource(source)
		if source.Content == "" && source.Source == "" && source.Title == "" {
			continue
		}
		ledger = append(ledger, ledgerPart(source, previewChars))
		if source.Included {
			analysis = append(analysis, analysisPart(len(analysis)+1, source))
		}
		switch source.Placement {
		case PlacementLeadingMessage:
			if source.Included && strings.TrimSpace(source.Content) != "" {
				leadingSources = append(leadingSources, source)
			}
		case PlacementFinalUserPrefix:
			if source.Included && strings.TrimSpace(source.Content) != "" && len(messages) > 0 {
				finalUserSources = append(finalUserSources, source)
			}
		case PlacementAuditOnly:
		default:
		}
	}
	if len(leadingSources) > 0 {
		leadingMessages := make([]*schema.Message, 0, len(leadingSources))
		for _, source := range leadingSources {
			leadingMessages = append(leadingMessages, schema.UserMessage(StandaloneMessage(source.Title, source.Content, "")))
		}
		messages = append(leadingMessages, messages...)
	}
	if len(finalUserSources) > 0 && len(messages) > 0 {
		last := *messages[len(messages)-1]
		last.Content = PrependFinalUserSources(last.Content, finalUserSources)
		messages[len(messages)-1] = &last
	}
	return Result{Messages: messages, Ledger: ledger, AnalysisParts: analysis}, nil
}

func SourceSummary(sources []Source, previewChars int) string {
	if previewChars <= 0 {
		previewChars = DefaultPreviewChars
	}
	if len(sources) == 0 {
		return "count=0"
	}
	items := make([]string, 0, len(sources))
	for i, source := range sources {
		source = normalizeSource(source)
		if strings.TrimSpace(source.Content) == "" {
			continue
		}
		part := ledgerPart(source, previewChars)
		fields := []string{
			fmt.Sprintf("%d:source=%q", i, part.Source),
			fmt.Sprintf("title=%q", part.Title),
			"bytes=" + intString(part.Bytes),
			"chars=" + intString(part.Chars),
			"preview=" + strconv.Quote(part.Preview),
		}
		if part.Purpose != "" {
			fields = append(fields, "purpose="+strconv.Quote(part.Purpose))
		}
		if part.Note != "" {
			fields = append(fields, "note="+strconv.Quote(part.Note))
		}
		if part.Truncated {
			fields = append(fields, "truncated=true")
		}
		if part.Limit > 0 {
			fields = append(fields, "limit="+intString(part.Limit))
		}
		items = append(items, strings.Join(fields, ","))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(items), strings.Join(items, "; "))
}

func StandaloneMessage(title, content, note string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "稳定上下文"
	}
	note = strings.TrimSpace(note)
	if note == "" {
		note = "以下内容来自当前 workspace 的低变更率有界状态快照，放在模型输入前部以提升前缀缓存稳定性。需要更完整或最新内容时，按来源路径使用工具读取确认。"
	}
	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString(note)
	sb.WriteString("\n\n")
	sb.WriteString(content)
	return sb.String()
}

func PrependFinalUserSource(agentMessage, title, content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return agentMessage
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "本轮动态上下文"
	}
	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString("以下内容来自当前 workspace 的有界状态快照，随作品进展变化，只用于本轮判断。需要更完整或最新内容时，按来源路径使用工具读取确认。\n\n")
	sb.WriteString(content)
	sb.WriteString("\n\n---\n\n# 本轮用户请求（最高优先级）\n\n")
	sb.WriteString(strings.TrimSpace(agentMessage))
	return sb.String()
}

func PrependFinalUserSources(agentMessage string, sources []Source) string {
	included := make([]Source, 0, len(sources))
	for _, source := range sources {
		source = normalizeSource(source)
		if source.Included && strings.TrimSpace(source.Content) != "" {
			included = append(included, source)
		}
	}
	if len(included) == 0 {
		return agentMessage
	}
	if len(included) == 1 {
		source := included[0]
		return PrependFinalUserSource(agentMessage, source.Title, source.Content)
	}
	var sb strings.Builder
	for i, source := range included {
		title := strings.TrimSpace(source.Title)
		if title == "" {
			title = "本轮动态上下文"
		}
		if i > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString("# ")
		sb.WriteString(title)
		sb.WriteString("\n\n")
		sb.WriteString("以下内容来自当前 workspace 的有界状态快照，随作品进展变化，只用于本轮判断。需要更完整或最新内容时，按来源路径使用工具读取确认。\n\n")
		sb.WriteString(strings.TrimSpace(source.Content))
	}
	sb.WriteString("\n\n---\n\n# 本轮用户请求（最高优先级）\n\n")
	sb.WriteString(strings.TrimSpace(agentMessage))
	return sb.String()
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			out = append(out, nil)
			continue
		}
		copied := *msg
		out = append(out, &copied)
	}
	return out
}

func normalizeSource(source Source) Source {
	source.Source = strings.TrimSpace(source.Source)
	source.Title = strings.TrimSpace(source.Title)
	source.Purpose = strings.TrimSpace(source.Purpose)
	source.Content = strings.TrimSpace(source.Content)
	source.Note = strings.TrimSpace(source.Note)
	if source.Placement == "" {
		source.Placement = PlacementAuditOnly
	}
	if !source.Included && source.Placement != PlacementAuditOnly {
		source.Included = true
	}
	return source
}

func ledgerPart(source Source, previewChars int) LedgerPart {
	return LedgerPart{
		Source:    source.Source,
		Title:     source.Title,
		Purpose:   source.Purpose,
		Bytes:     len(source.Content),
		Chars:     utf8.RuneCountInString(source.Content),
		Preview:   Preview(source.Content, previewChars),
		Note:      source.Note,
		Included:  source.Included,
		Truncated: source.Truncated,
		Limit:     source.Limit,
	}
}

func analysisPart(index int, source Source) AnalysisPart {
	return AnalysisPart{
		ID:      fmt.Sprintf("source_%d", index),
		Source:  source.Source,
		Title:   source.Title,
		Role:    string(schema.User),
		Content: source.Content,
		Note:    source.Note,
		Bytes:   len(source.Content),
		Chars:   utf8.RuneCountInString(source.Content),
	}
}

func Preview(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func intString(v int) string {
	return strconv.Itoa(v)
}
