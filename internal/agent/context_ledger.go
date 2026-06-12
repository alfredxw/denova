package agent

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ContextLedger records every bounded context fragment that Nova intentionally
// makes visible to the model for a single turn.
type ContextLedger struct {
	policy ContextLedgerPolicy
	parts  []ContextLedgerPart
}

// ContextLedgerPart is the durable audit shape for one model-visible context source.
type ContextLedgerPart struct {
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

// NewContextLedger creates a context ledger for one Agent turn.
func NewContextLedger(policy ContextLedgerPolicy) *ContextLedger {
	if policy.PreviewChars <= 0 {
		policy.PreviewChars = defaultContextLedgerPreviewChars
	}
	return &ContextLedger{policy: policy}
}

// Add records a context part with a free-form note.
func (l *ContextLedger) Add(source, title, content, note string) {
	l.AddPart(source, title, "", content, note, true, false, 0)
}

// AddPart records one context part. Content is never retained in full.
func (l *ContextLedger) AddPart(source, title, purpose, content, note string, included, truncated bool, limit int) {
	if l == nil || !l.policy.Enabled {
		return
	}
	source = strings.TrimSpace(source)
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if source == "" && title == "" && content == "" {
		return
	}
	l.parts = append(l.parts, ContextLedgerPart{
		Source:    source,
		Title:     title,
		Purpose:   strings.TrimSpace(purpose),
		Bytes:     len(content),
		Chars:     utf8.RuneCountInString(content),
		Preview:   safeLogPreview(content, l.policy.PreviewChars),
		Note:      strings.TrimSpace(note),
		Included:  included,
		Truncated: truncated,
		Limit:     limit,
	})
}

// Parts returns a copy of all ledger parts.
func (l *ContextLedger) Parts() []ContextLedgerPart {
	if l == nil || len(l.parts) == 0 {
		return nil
	}
	result := make([]ContextLedgerPart, len(l.parts))
	copy(result, l.parts)
	return result
}

// Summary returns a compact log-friendly representation.
func (l *ContextLedger) Summary() string {
	if l == nil || len(l.parts) == 0 {
		return "count=0"
	}
	parts := make([]string, 0, len(l.parts))
	for i, part := range l.parts {
		fields := []string{
			fmt.Sprintf("%d:source=%q", i, part.Source),
			fmt.Sprintf("title=%q", part.Title),
			"bytes=" + intString(part.Bytes),
			"chars=" + intString(part.Chars),
			"preview=" + strconv.Quote(part.Preview),
			"included=" + ledgerBoolString(part.Included),
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
		parts = append(parts, strings.Join(fields, ","))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(l.parts), strings.Join(parts, "; "))
}

func ledgerBoolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
