package context

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const DefaultAuditPreviewChars = 100

// AuditPolicy controls the bounded, in-memory audit projection of model-visible
// context. Full source content is never retained by AuditLedger.
type AuditPolicy struct {
	Enabled      bool
	PreviewChars int
}

// AuditPart describes one intentional context source without retaining its
// complete content. Run traces persist a further content-free projection.
type AuditPart struct {
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

// AuditLedger records every bounded context fragment intentionally exposed to
// the model during one turn.
type AuditLedger struct {
	policy AuditPolicy
	parts  []AuditPart
}

// NewAuditLedger creates a ledger using a normalized preview policy.
func NewAuditLedger(policy AuditPolicy) *AuditLedger {
	if policy.PreviewChars <= 0 {
		policy.PreviewChars = DefaultAuditPreviewChars
	}
	return &AuditLedger{policy: policy}
}

// Policy returns the normalized policy used by the ledger.
func (ledger *AuditLedger) Policy() AuditPolicy {
	if ledger == nil {
		return AuditPolicy{}
	}
	return ledger.policy
}

// Add records a context part with a free-form note.
func (ledger *AuditLedger) Add(source, title, content, note string) {
	ledger.AddPart(source, title, "", content, note, true, false, 0)
}

// AddPart records one context part whose optional hard limit is expressed in
// bytes.
func (ledger *AuditLedger) AddPart(source, title, purpose, content, note string, included, truncated bool, limit int) {
	ledger.AddPartWithLimitUnit(source, title, purpose, content, note, included, truncated, limit, "bytes")
}

// AddPartWithLimitUnit records one context part with an explicit limit unit.
func (ledger *AuditLedger) AddPartWithLimitUnit(source, title, purpose, content, note string, included, truncated bool, limit int, limitUnit string) {
	if ledger == nil || !ledger.policy.Enabled {
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
	ledger.parts = append(ledger.parts, AuditPart{
		Source: source, Title: title, Purpose: strings.TrimSpace(purpose),
		Bytes: len(content), Chars: utf8.RuneCountInString(content), Hash: hash,
		Preview: auditPreview(content, ledger.policy.PreviewChars), Note: strings.TrimSpace(note),
		Included: included, Truncated: truncated, Limit: limit, LimitUnit: limitUnit,
	})
}

// Parts returns an isolated copy of the audit projection.
func (ledger *AuditLedger) Parts() []AuditPart {
	if ledger == nil || len(ledger.parts) == 0 {
		return nil
	}
	return append([]AuditPart(nil), ledger.parts...)
}

// Summary returns a compact diagnostic representation without full content.
func (ledger *AuditLedger) Summary() string {
	if ledger == nil || len(ledger.parts) == 0 {
		return "count=0"
	}
	parts := make([]string, 0, len(ledger.parts))
	for index, part := range ledger.parts {
		fields := []string{
			fmt.Sprintf("%d:source=%q", index, part.Source),
			fmt.Sprintf("title=%q", part.Title),
			"bytes=" + strconv.Itoa(part.Bytes),
			"chars=" + strconv.Itoa(part.Chars),
			"included=" + strconv.FormatBool(part.Included),
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
			fields = append(fields, "limit="+strconv.Itoa(part.Limit))
			if part.LimitUnit != "" {
				fields = append(fields, "limit_unit="+strconv.Quote(part.LimitUnit))
			}
		}
		parts = append(parts, strings.Join(fields, ","))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(ledger.parts), strings.Join(parts, "; "))
}

func auditPreview(content string, limit int) string {
	content = strings.NewReplacer("\n", "\\n", "\r", "\\r").Replace(content)
	if limit <= 0 || len(content) <= limit {
		return content
	}
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	return content[:limit] + "..."
}
