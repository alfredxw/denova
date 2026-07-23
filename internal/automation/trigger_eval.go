package automation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	// MaxTriggerContextChars bounds the complete model-facing observation. It
	// is intentionally above 128 KiB so normal chapter batches retain useful
	// evidence while still preventing unbounded workspace injection.
	MaxTriggerContextChars = 256 * 1024
	// The following limits name each source field independently. They keep
	// metadata compact and reserve the context budget for evidence snippets.
	MaxTriggerContextSourceChars   = 120
	MaxTriggerContextSummaryChars  = 1000
	MaxTriggerEvidenceItems        = 128
	MaxTriggerEvidenceSourceChars  = 80
	MaxTriggerEvidenceTitleChars   = 160
	MaxTriggerEvidenceRefChars     = 240
	MaxTriggerEvidenceSnippetChars = 1200

	// Model output is never fed back unchecked. These bounds reject malformed
	// or noisy decisions rather than silently truncating their semantics.
	MaxSemanticEvaluationResponseChars = 64 * 1024
	MaxSemanticEvaluationReasonChars   = 8 * 1024
	MaxSemanticEvaluationTitleChars    = 512
	MaxSemanticEvaluationEvidenceRefs  = 128
	MaxSemanticEvaluationRefChars      = 512
)

type TriggerContext struct {
	// Source identifies the producer of this bounded observation, not an
	// arbitrary log stream.
	Source string `json:"source"`
	// Summary states why this observation is being evaluated this turn.
	Summary string `json:"summary"`
	// Evidence contains only model-relevant, bounded excerpts and references.
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
	ctx.Source = trimRunes(strings.TrimSpace(ctx.Source), MaxTriggerContextSourceChars)
	ctx.Summary = trimRunes(strings.TrimSpace(ctx.Summary), MaxTriggerContextSummaryChars)
	total := len([]rune(ctx.Source)) + len([]rune(ctx.Summary))
	evidenceCapacity := len(ctx.Evidence)
	if evidenceCapacity > MaxTriggerEvidenceItems {
		evidenceCapacity = MaxTriggerEvidenceItems
	}
	evidence := make([]TriggerEvidence, 0, evidenceCapacity)
	for _, item := range ctx.Evidence {
		if len(evidence) >= MaxTriggerEvidenceItems {
			break
		}
		next := TriggerEvidence{
			Source:  trimRunes(strings.TrimSpace(item.Source), MaxTriggerEvidenceSourceChars),
			Title:   trimRunes(strings.TrimSpace(item.Title), MaxTriggerEvidenceTitleChars),
			Ref:     trimRunes(strings.TrimSpace(item.Ref), MaxTriggerEvidenceRefChars),
			Snippet: trimRunes(strings.TrimSpace(item.Snippet), MaxTriggerEvidenceSnippetChars),
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
	if len(raw) > MaxSemanticEvaluationResponseChars*utf8.UTFMax || utf8.RuneCountInString(raw) > MaxSemanticEvaluationResponseChars {
		return SemanticEvaluation{}, fmt.Errorf("semantic evaluation exceeds %d characters", MaxSemanticEvaluationResponseChars)
	}
	var eval SemanticEvaluation
	if err := json.Unmarshal([]byte(raw), &eval); err != nil {
		return SemanticEvaluation{}, fmt.Errorf("parse semantic evaluation failed: %w", err)
	}
	return ValidateSemanticEvaluation(eval)
}

// ValidateSemanticEvaluation canonicalizes and bounds every model-controlled
// decision field before the decision may enter durable trigger state.
func ValidateSemanticEvaluation(eval SemanticEvaluation) (SemanticEvaluation, error) {
	eval.Reason = strings.TrimSpace(eval.Reason)
	eval.Title = strings.TrimSpace(eval.Title)
	if utf8.RuneCountInString(eval.Reason) > MaxSemanticEvaluationReasonChars {
		return SemanticEvaluation{}, fmt.Errorf("semantic evaluation reason exceeds %d characters", MaxSemanticEvaluationReasonChars)
	}
	if utf8.RuneCountInString(eval.Title) > MaxSemanticEvaluationTitleChars {
		return SemanticEvaluation{}, fmt.Errorf("semantic evaluation title exceeds %d characters", MaxSemanticEvaluationTitleChars)
	}
	if math.IsNaN(eval.Confidence) || math.IsInf(eval.Confidence, 0) || eval.Confidence < 0 || eval.Confidence > 1 {
		return SemanticEvaluation{}, fmt.Errorf("semantic evaluation confidence must be between 0 and 1")
	}
	refs := eval.EvidenceRefs[:0]
	seen := make(map[string]bool, len(eval.EvidenceRefs))
	for _, ref := range eval.EvidenceRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		if utf8.RuneCountInString(ref) > MaxSemanticEvaluationRefChars {
			return SemanticEvaluation{}, fmt.Errorf("semantic evaluation evidence ref exceeds %d characters", MaxSemanticEvaluationRefChars)
		}
		if len(refs) >= MaxSemanticEvaluationEvidenceRefs {
			return SemanticEvaluation{}, fmt.Errorf("semantic evaluation exceeds %d evidence refs", MaxSemanticEvaluationEvidenceRefs)
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	eval.EvidenceRefs = refs
	return eval, nil
}

// ValidateSemanticEvaluationEvidence ensures model-selected references came
// from the exact bounded context used for this decision. It returns a canonical
// de-duplicated evaluation suitable for durable persistence.
func ValidateSemanticEvaluationEvidence(eval SemanticEvaluation, ctx TriggerContext) (SemanticEvaluation, error) {
	var err error
	eval, err = ValidateSemanticEvaluation(eval)
	if err != nil {
		return SemanticEvaluation{}, err
	}
	allowed := make(map[string]bool, len(ctx.Evidence)*2)
	for _, evidence := range ctx.Evidence {
		if ref := strings.TrimSpace(evidence.Ref); ref != "" {
			allowed[ref] = true
		}
		if title := strings.TrimSpace(evidence.Title); title != "" {
			allowed[title] = true
		}
	}
	for _, ref := range eval.EvidenceRefs {
		if !allowed[ref] {
			return SemanticEvaluation{}, fmt.Errorf("semantic evaluation evidence ref %q is outside bounded context", ref)
		}
	}
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
	count := 0
	for index := range value {
		if count == limit {
			return value[:index]
		}
		count++
	}
	return value
}
