package automation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const MaxTriggerContextChars = 8000

type TriggerContext struct {
	Source   string            `json:"source"`
	Summary  string            `json:"summary"`
	Evidence []TriggerEvidence `json:"evidence"`
}

type SemanticEvaluation struct {
	Matched      bool     `json:"matched"`
	Confidence   float64  `json:"confidence"`
	Reason       string   `json:"reason"`
	Title        string   `json:"title"`
	EvidenceRefs []string `json:"evidence_refs"`
}

func BoundedTriggerContext(ctx TriggerContext) TriggerContext {
	ctx.Source = trimRunes(strings.TrimSpace(ctx.Source), 120)
	ctx.Summary = trimRunes(strings.TrimSpace(ctx.Summary), 1000)
	total := len([]rune(ctx.Source)) + len([]rune(ctx.Summary))
	evidence := make([]TriggerEvidence, 0, len(ctx.Evidence))
	for _, item := range ctx.Evidence {
		next := TriggerEvidence{
			Source:  trimRunes(strings.TrimSpace(item.Source), 80),
			Title:   trimRunes(strings.TrimSpace(item.Title), 160),
			Ref:     trimRunes(strings.TrimSpace(item.Ref), 240),
			Snippet: trimRunes(strings.TrimSpace(item.Snippet), 1200),
		}
		itemSize := len([]rune(next.Source)) + len([]rune(next.Title)) + len([]rune(next.Ref)) + len([]rune(next.Snippet))
		if total+itemSize > MaxTriggerContextChars {
			break
		}
		total += itemSize
		evidence = append(evidence, next)
	}
	ctx.Evidence = evidence
	return ctx
}

func ParseSemanticEvaluation(raw string) (SemanticEvaluation, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SemanticEvaluation{}, fmt.Errorf("semantic evaluation is empty")
	}
	var eval SemanticEvaluation
	if err := json.Unmarshal([]byte(raw), &eval); err != nil {
		return SemanticEvaluation{}, fmt.Errorf("parse semantic evaluation failed: %w", err)
	}
	eval.Reason = strings.TrimSpace(eval.Reason)
	eval.Title = strings.TrimSpace(eval.Title)
	refs := eval.EvidenceRefs[:0]
	for _, ref := range eval.EvidenceRefs {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	eval.EvidenceRefs = refs
	return eval, nil
}

func EvidenceFingerprint(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func trimRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
