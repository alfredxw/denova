package agent

import (
	"crypto/sha256"
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
	Hash      string `json:"hash,omitempty"`
	Preview   string `json:"preview"`
	Note      string `json:"note,omitempty"`
	Included  bool   `json:"included"`
	Truncated bool   `json:"truncated,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	LimitUnit string `json:"limit_unit,omitempty"`
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
	l.AddPartWithLimitUnit(source, title, purpose, content, note, included, truncated, limit, "bytes")
}

// AddPartWithLimitUnit records a part whose hard limit is expressed in an
// explicit unit. Byte limits remain the default for existing callers.
func (l *ContextLedger) AddPartWithLimitUnit(source, title, purpose, content, note string, included, truncated bool, limit int, limitUnit string) {
	if l == nil || !l.policy.Enabled {
		return
	}
	source = strings.TrimSpace(source)
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if source == "" && title == "" && content == "" {
		return
	}
	hash := ""
	if content != "" {
		sum := sha256.Sum256([]byte(content))
		hash = fmt.Sprintf("sha256:%x", sum[:8])
	}
	limitUnit = strings.TrimSpace(limitUnit)
	if limit <= 0 {
		limitUnit = ""
	} else if limitUnit == "" {
		limitUnit = "bytes"
	}
	l.parts = append(l.parts, ContextLedgerPart{
		Source:    source,
		Title:     title,
		Purpose:   strings.TrimSpace(purpose),
		Bytes:     len(content),
		Chars:     utf8.RuneCountInString(content),
		Hash:      hash,
		Preview:   safeLogPreview(content, l.policy.PreviewChars),
		Note:      strings.TrimSpace(note),
		Included:  included,
		Truncated: truncated,
		Limit:     limit,
		LimitUnit: limitUnit,
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
			"hash=" + strconv.Quote(part.Hash),
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
			if part.LimitUnit != "" {
				fields = append(fields, "limit_unit="+strconv.Quote(part.LimitUnit))
			}
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
