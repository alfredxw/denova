package context

import (
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alfredxw/denova/agent"
)

const (
	// Content-byte defaults are intentionally above 128 KiB. Applications may
	// lower or raise them, but an unset budget must not silently squeeze ordinary
	// context into a small implicit limit.
	DefaultMaxFragmentBytes      = 256 * 1024
	DefaultMaxTotalBytes         = 4 * 1024 * 1024
	DefaultMaxFragments          = 256
	DefaultMaxMetadataFieldBytes = 4 * 1024
)

// Budget is the hard context boundary enforced by Assembler. MaxFragmentBytes
// limits each content body, while MaxTotalBytes rejects a set of fragments whose
// complete rendered form is too large; it never chooses fragments to truncate by
// arrival order. The existing transcript is not charged to this injection safety
// ceiling. MaxMetadataFieldBytes is applied independently to every
// caller-controlled metadata string.
type Budget struct {
	MaxFragmentBytes      int `json:"max_fragment_bytes"`
	MaxTotalBytes         int `json:"max_total_bytes"`
	MaxFragments          int `json:"max_fragments"`
	MaxMetadataFieldBytes int `json:"max_metadata_field_bytes"`
}

func DefaultBudget() Budget {
	return Budget{
		MaxFragmentBytes:      DefaultMaxFragmentBytes,
		MaxTotalBytes:         DefaultMaxTotalBytes,
		MaxFragments:          DefaultMaxFragments,
		MaxMetadataFieldBytes: DefaultMaxMetadataFieldBytes,
	}
}

func (b Budget) normalized() Budget {
	defaults := DefaultBudget()
	if b.MaxFragmentBytes <= 0 {
		b.MaxFragmentBytes = defaults.MaxFragmentBytes
	}
	if b.MaxTotalBytes <= 0 {
		b.MaxTotalBytes = defaults.MaxTotalBytes
	}
	if b.MaxFragments <= 0 {
		b.MaxFragments = defaults.MaxFragments
	}
	if b.MaxMetadataFieldBytes <= 0 {
		b.MaxMetadataFieldBytes = defaults.MaxMetadataFieldBytes
	}
	return b
}

// Fragment is one independently auditable context injection. Model-visible
// fragments must carry Source, Purpose, Placement, Limit, and Hash after
// assembly. Raw transcript, thinking, display events, and raw tool output are
// deliberately not represented by this type unless a caller explicitly
// projects them into a bounded fragment.
type Fragment struct {
	ID        string    `json:"id,omitempty"`
	Source    string    `json:"source"`
	Title     string    `json:"title,omitempty"`
	Purpose   string    `json:"purpose"`
	Content   string    `json:"content"`
	Placement Placement `json:"placement"`
	Limit     int       `json:"limit"`
	Hash      string    `json:"hash"`
	Included  bool      `json:"included"`
	Truncated bool      `json:"truncated,omitempty"`
	Note      string    `json:"note,omitempty"`
}

type AssembleRequest struct {
	Messages     []*agent.Message
	Fragments    []Fragment
	PreviewChars int
}

// Assembler is the single model-context placement and budget Module.
type Assembler struct {
	budget   Budget
	renderer Renderer
}

// Option configures context presentation without weakening the assembler's
// provenance and hard-budget guarantees.
type Option func(*Assembler)

// WithRenderer replaces the neutral prompt renderer. A nil renderer is
// ignored and the provider-neutral DefaultRenderer remains active.
func WithRenderer(renderer Renderer) Option {
	return func(assembler *Assembler) {
		if renderer != nil {
			assembler.renderer = renderer
		}
	}
}

func NewAssembler(budget Budget, options ...Option) *Assembler {
	assembler := &Assembler{budget: budget.normalized(), renderer: DefaultRenderer{}}
	for _, option := range options {
		if option != nil {
			option(assembler)
		}
	}
	return assembler
}

func (a *Assembler) Assemble(ctx stdcontext.Context, req AssembleRequest) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	budget := DefaultBudget()
	renderer := Renderer(DefaultRenderer{})
	if a != nil {
		budget = a.budget.normalized()
		if a.renderer != nil {
			renderer = a.renderer
		}
	}
	previewChars := req.PreviewChars
	if previewChars <= 0 {
		previewChars = DefaultPreviewChars
	}
	if len(req.Fragments) > budget.MaxFragments {
		return Result{}, fmt.Errorf("context fragment count %d exceeds limit %d", len(req.Fragments), budget.MaxFragments)
	}
	requestedFragments := append([]Fragment(nil), req.Fragments...)
	messages := cloneMessages(req.Messages)
	fragments := make([]Fragment, 0, len(requestedFragments))
	leading := make([]Fragment, 0, len(requestedFragments))
	finalUser := make([]Fragment, 0, len(requestedFragments))
	ledger := make([]LedgerPart, 0, len(requestedFragments))
	analysis := make([]AnalysisPart, 0, len(requestedFragments))
	injectedBytes := 0
	hasFinalUserMessage := len(messages) > 0 && messages[len(messages)-1] != nil && messages[len(messages)-1].Role == agent.User

	for _, fragment := range requestedFragments {
		resolved, err := resolveFragment(fragment, budget, hasFinalUserMessage)
		if err != nil {
			return Result{}, err
		}
		var renderedBytes int
		if resolved.Included && resolved.Placement != PlacementAuditOnly && resolved.Content != "" {
			renderedBytes = renderedFragmentBytes(renderer, resolved, finalUser)
			if renderedBytes > budget.MaxTotalBytes-injectedBytes {
				return Result{}, fmt.Errorf(
					"context injected bytes exceed limit: source=%s required=%d remaining=%d limit=%d",
					resolved.Source, renderedBytes, max(0, budget.MaxTotalBytes-injectedBytes), budget.MaxTotalBytes,
				)
			}
		}
		resolved = boundFragmentMetadata(resolved, budget.MaxMetadataFieldBytes)
		resolved.Hash = fragmentContentHash(resolved.Content)
		fragments = append(fragments, resolved)
		ledger = append(ledger, ledgerPartForFragment(resolved, previewChars, budget.MaxMetadataFieldBytes))
		if resolved.Included {
			analysis = append(analysis, analysisPartForFragment(len(analysis)+1, resolved))
		}
		if !resolved.Included || resolved.Placement == PlacementAuditOnly || resolved.Content == "" {
			continue
		}
		injectedBytes += renderedBytes
		switch resolved.Placement {
		case PlacementLeadingMessage:
			leading = append(leading, resolved)
		case PlacementFinalUserPrefix:
			finalUser = append(finalUser, resolved)
		}
	}

	if len(leading) > 0 {
		leadingMessages := make([]*agent.Message, 0, len(leading))
		for _, fragment := range leading {
			message := agent.UserMessage(renderer.RenderLeading(fragment))
			message.Extra = map[string]any{MessageExtraPlacement: string(PlacementLeadingMessage)}
			leadingMessages = append(leadingMessages, message)
		}
		messages = append(leadingMessages, messages...)
	}
	if len(finalUser) > 0 && len(messages) > 0 {
		last := *messages[len(messages)-1]
		last.Content = renderer.RenderFinalUser(last.Content, finalUser)
		messages[len(messages)-1] = &last
	}

	return Result{
		Messages:      messages,
		Ledger:        ledger,
		AnalysisParts: analysis,
		Fragments:     fragments,
		InjectedBytes: injectedBytes,
	}, nil
}

func renderedFragmentBytes(renderer Renderer, fragment Fragment, priorFinalUser []Fragment) int {
	switch fragment.Placement {
	case PlacementLeadingMessage:
		return len(renderer.RenderLeading(fragment))
	case PlacementFinalUserPrefix:
		before := len(renderer.RenderFinalUser("", priorFinalUser))
		afterFragments := make([]Fragment, 0, len(priorFinalUser)+1)
		afterFragments = append(afterFragments, priorFinalUser...)
		afterFragments = append(afterFragments, fragment)
		after := len(renderer.RenderFinalUser("", afterFragments))
		if after <= before {
			return 0
		}
		return after - before
	default:
		return 0
	}
}

func resolveFragment(fragment Fragment, budget Budget, hasFinalMessage bool) (Fragment, error) {
	fragment = boundFragmentMetadata(fragment, budget.MaxMetadataFieldBytes)
	fragment.Content = strings.TrimSpace(strings.ToValidUTF8(fragment.Content, "\uFFFD"))
	if fragment.Placement == "" {
		fragment.Placement = PlacementAuditOnly
	}
	if !validPlacement(fragment.Placement) {
		placement, _ := truncateMetadataField(string(fragment.Placement), budget.MaxMetadataFieldBytes)
		return Fragment{}, fmt.Errorf("invalid context fragment placement %q", placement)
	}
	if fragment.Included && fragment.Placement != PlacementAuditOnly {
		if fragment.Source == "" {
			return Fragment{}, fmt.Errorf("context fragment source is required")
		}
		if fragment.Purpose == "" {
			return Fragment{}, fmt.Errorf("context fragment purpose is required: source=%s", fragment.Source)
		}
	}
	fragment.Limit = effectiveFragmentLimit(fragment.Limit, budget.MaxFragmentBytes)
	if content, truncated := truncateUTF8Bytes(fragment.Content, fragment.Limit); truncated {
		fragment.Content = content
		fragment.Truncated = true
		fragment.Note = appendFragmentNote(fragment.Note, "fragment_limit_applied")
	}
	if fragment.Included && fragment.Placement == PlacementFinalUserPrefix && !hasFinalMessage {
		fragment.Included = false
		fragment.Note = appendFragmentNote(fragment.Note, "no_final_user_message")
	}
	fragment = boundFragmentMetadata(fragment, budget.MaxMetadataFieldBytes)
	fragment.Hash = fragmentContentHash(fragment.Content)
	return fragment, nil
}

func validPlacement(placement Placement) bool {
	switch placement {
	case PlacementLeadingMessage, PlacementFinalUserPrefix, PlacementAuditOnly:
		return true
	}
	return false
}

func effectiveFragmentLimit(declared, hardMax int) int {
	if declared <= 0 || declared > hardMax {
		return hardMax
	}
	return declared
}

func truncateUTF8Bytes(value string, maxBytes int) (string, bool) {
	value = strings.ToValidUTF8(value, "\uFFFD")
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return strings.TrimSpace(value[:end]), true
}

func truncateMetadataField(value string, maxBytes int) (string, bool) {
	return truncateUTF8Bytes(strings.TrimSpace(value), maxBytes)
}

func boundFragmentMetadata(fragment Fragment, maxBytes int) Fragment {
	var truncated bool
	fragment.ID, truncated = truncateMetadataField(fragment.ID, maxBytes)
	metadataTruncated := truncated
	fragment.Source, truncated = truncateMetadataField(fragment.Source, maxBytes)
	metadataTruncated = metadataTruncated || truncated
	fragment.Title, truncated = truncateMetadataField(fragment.Title, maxBytes)
	metadataTruncated = metadataTruncated || truncated
	fragment.Purpose, truncated = truncateMetadataField(fragment.Purpose, maxBytes)
	metadataTruncated = metadataTruncated || truncated
	fragment.Note, truncated = truncateMetadataField(fragment.Note, maxBytes)
	fragment.Truncated = fragment.Truncated || metadataTruncated || truncated
	return fragment
}

func fragmentContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func appendFragmentNote(note, value string) string {
	if note == "" {
		return value
	}
	return note + "; " + value
}

func sourceFromFragment(fragment Fragment) Source {
	return Source{
		Source:    fragment.Source,
		Title:     fragment.Title,
		Purpose:   fragment.Purpose,
		Content:   fragment.Content,
		Placement: fragment.Placement,
		Limit:     fragment.Limit,
		Included:  fragment.Included,
		Truncated: fragment.Truncated,
		Note:      fragment.Note,
	}
}

func ledgerPartForFragment(fragment Fragment, previewChars, maxMetadataFieldBytes int) LedgerPart {
	preview, _ := truncateMetadataField(Preview(fragment.Content, previewChars), maxMetadataFieldBytes)
	return LedgerPart{
		Source:    fragment.Source,
		Title:     fragment.Title,
		Purpose:   fragment.Purpose,
		Placement: fragment.Placement,
		Bytes:     len(fragment.Content),
		Chars:     utf8.RuneCountInString(fragment.Content),
		Preview:   preview,
		Hash:      fragment.Hash,
		Note:      fragment.Note,
		Included:  fragment.Included,
		Truncated: fragment.Truncated,
		Limit:     fragment.Limit,
	}
}

func analysisPartForFragment(index int, fragment Fragment) AnalysisPart {
	return AnalysisPart{
		ID:      fmt.Sprintf("source_%d", index),
		Source:  fragment.Source,
		Title:   fragment.Title,
		Role:    string(agent.User),
		Content: fragment.Content,
		Note:    fragment.Note,
		Bytes:   len(fragment.Content),
		Chars:   utf8.RuneCountInString(fragment.Content),
	}
}
