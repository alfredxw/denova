package context

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alfredxw/denova/agent"
)

type Placement string

const (
	PlacementLeadingMessage  Placement = "leading_message"
	PlacementFinalUserPrefix Placement = "final_user_prefix"
	PlacementAuditOnly       Placement = "audit_only"

	DefaultPreviewChars = 100
)

// Renderer owns the human-facing placement text around bounded fragments.
// The assembler deliberately does not own language, prompt wording, or product
// policy. Renderers must be deterministic and include fragment Content exactly
// once so the assembler can account for every model-visible byte.
type Renderer interface {
	RenderLeading(Fragment) string
	RenderFinalUser(userRequest string, fragments []Fragment) string
}

// DefaultRenderer provides a neutral, dependency-free format suitable for
// reusable agents. Applications can replace it to localize prompt wording
// without changing provenance, limits, hashing, or placement behavior.
type DefaultRenderer struct{}

func (DefaultRenderer) RenderLeading(fragment Fragment) string {
	title := strings.TrimSpace(fragment.Title)
	if title == "" {
		title = "Context"
	}
	return "# " + title + "\n\n" + strings.TrimSpace(fragment.Content)
}

func (DefaultRenderer) RenderFinalUser(userRequest string, fragments []Fragment) string {
	if len(fragments) == 0 {
		return userRequest
	}
	var builder strings.Builder
	for index, fragment := range fragments {
		if index > 0 {
			builder.WriteString("\n\n---\n\n")
		}
		title := strings.TrimSpace(fragment.Title)
		if title == "" {
			title = "Context"
		}
		builder.WriteString("# ")
		builder.WriteString(title)
		builder.WriteString("\n\n")
		builder.WriteString(strings.TrimSpace(fragment.Content))
	}
	builder.WriteString("\n\n---\n\n# User request\n\n")
	builder.WriteString(strings.TrimSpace(userRequest))
	return builder.String()
}

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
	Messages      []*agent.Message
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

func cloneMessages(messages []*agent.Message) []*agent.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]*agent.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, agent.CloneMessage(msg))
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
