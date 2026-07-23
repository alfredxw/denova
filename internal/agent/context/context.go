package context

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type Placement string

const (
	PlacementLeadingMessage  Placement = "leading_message"
	PlacementFinalUserPrefix Placement = "final_user_prefix"
	PlacementAuditOnly       Placement = "audit_only"

	DefaultPreviewChars = 100

	finalUserSourceNote     = "状态快照可能过期，以工具读取为准。"
	contextSourceSeparator  = "\n\n---\n\n"
	finalUserRequestWrapper = "\n\n---\n\n# 本轮用户请求（最高优先级）\n\n"
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

type Result struct {
	Messages      []*schema.Message
	Ledger        []LedgerPart
	AnalysisParts []AnalysisPart
	Fragments     []Fragment
	// InjectedBytes is the exact rendered string overhead introduced into
	// Messages by included fragments; the pre-existing transcript is excluded.
	InjectedBytes int
}

type LedgerPart struct {
	Source    string    `json:"source"`
	Title     string    `json:"title"`
	Purpose   string    `json:"purpose,omitempty"`
	Placement Placement `json:"placement"`
	Bytes     int       `json:"bytes"`
	Chars     int       `json:"chars"`
	Preview   string    `json:"preview"`
	Hash      string    `json:"hash"`
	Note      string    `json:"note,omitempty"`
	Included  bool      `json:"included"`
	Truncated bool      `json:"truncated,omitempty"`
	Limit     int       `json:"limit,omitempty"`
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
			fmt.Sprintf("placement=%q", part.Placement),
			"bytes=" + intString(part.Bytes),
			"chars=" + intString(part.Chars),
			"preview=" + strconv.Quote(part.Preview),
			"hash=" + part.Hash,
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
	return PrependFinalUserSources(agentMessage, []Source{{Title: title, Content: content, Included: true}})
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
	var sb strings.Builder
	for i, source := range included {
		if i > 0 {
			sb.WriteString(contextSourceSeparator)
		}
		sb.WriteString(finalUserSourceBlock(source))
	}
	sb.WriteString(finalUserRequestWrapper)
	sb.WriteString(strings.TrimSpace(agentMessage))
	return sb.String()
}

func finalUserSourceBlock(source Source) string {
	title := strings.TrimSpace(source.Title)
	if title == "" {
		title = "本轮动态上下文"
	}
	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString(finalUserSourceNote)
	sb.WriteString("\n\n")
	sb.WriteString(strings.TrimSpace(source.Content))
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
	source = normalizeSource(source)
	fragment := boundFragmentMetadata(Fragment{
		Source: source.Source, Title: source.Title, Purpose: source.Purpose,
		Content: source.Content, Placement: source.Placement, Limit: source.Limit,
		Included: source.Included, Truncated: source.Truncated, Note: source.Note,
		Hash: fragmentContentHash(source.Content),
	}, DefaultMaxMetadataFieldBytes)
	return ledgerPartForFragment(fragment, previewChars, DefaultMaxMetadataFieldBytes)
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
