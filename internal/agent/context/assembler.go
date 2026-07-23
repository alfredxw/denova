package context

import (
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	adk "github.com/alfredxw/denova/adk"
)

const (
	// Content-byte defaults are intentionally above 128 KiB. Individual product
	// settings may lower or raise them, but an unset budget must not silently
	// squeeze normal creative-workspace context into a small generic limit.
	DefaultMaxFragmentBytes      = 256 * 1024
	DefaultMaxTotalBytes         = 1024 * 1024
	DefaultMaxFragments          = 256
	DefaultMaxMetadataFieldBytes = 4 * 1024
)

// Budget is the hard context boundary enforced by Assembler. MaxFragmentBytes
// limits each content body, while MaxTotalBytes counts every newly rendered
// model-visible byte, including titles, wrappers, and separators. The existing
// transcript is not charged to this injection budget. MaxMetadataFieldBytes is
// applied independently to every caller-controlled metadata string.
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

// ContextDescriptor is the stable provenance and budget contract owned by one
// ContextProjector. A projector may return multiple content fragments, but it
// cannot change these fields per call without registering a different
// descriptor.
type ContextDescriptor struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Purpose   string    `json:"purpose"`
	Placement Placement `json:"placement"`
	Limit     int       `json:"limit"`
}

// ContextProjector is the extension Seam for writing, interactive, automation,
// and future context sources. It returns domain content only; Assembler owns
// provenance validation, budgets, hashing, and final message placement.
type ContextProjector interface {
	Descriptor() ContextDescriptor
	Project(stdcontext.Context) ([]Fragment, error)
}

type AssembleRequest struct {
	Messages     []*adk.Message
	Fragments    []Fragment
	Projectors   []ContextProjector
	PreviewChars int
}

// Assembler is the single model-context placement and budget Module.
type Assembler struct {
	budget Budget
}

func NewAssembler(budget Budget) *Assembler {
	return &Assembler{budget: budget.normalized()}
}

func (a *Assembler) Assemble(ctx stdcontext.Context, req AssembleRequest) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	budget := DefaultBudget()
	if a != nil {
		budget = a.budget.normalized()
	}
	previewChars := req.PreviewChars
	if previewChars <= 0 {
		previewChars = DefaultPreviewChars
	}
	if len(req.Fragments) > budget.MaxFragments {
		return Result{}, fmt.Errorf("context fragment count %d exceeds limit %d", len(req.Fragments), budget.MaxFragments)
	}
	projected, err := projectFragments(ctx, req.Projectors, budget.MaxFragments, len(req.Fragments), budget.MaxMetadataFieldBytes)
	if err != nil {
		return Result{}, err
	}
	requestedFragments := make([]Fragment, 0, len(req.Fragments)+len(projected))
	requestedFragments = append(requestedFragments, req.Fragments...)
	requestedFragments = append(requestedFragments, projected...)
	messages := cloneMessages(req.Messages)
	fragments := make([]Fragment, 0, len(requestedFragments))
	leading := make([]Source, 0, len(requestedFragments))
	finalUser := make([]Source, 0, len(requestedFragments))
	ledger := make([]LedgerPart, 0, len(requestedFragments))
	analysis := make([]AnalysisPart, 0, len(requestedFragments))
	injectedBytes := 0
	hasFinalUserMessage := len(messages) > 0 && messages[len(messages)-1] != nil && messages[len(messages)-1].Role == adk.User

	for _, fragment := range requestedFragments {
		resolved, err := resolveFragment(fragment, budget, hasFinalUserMessage)
		if err != nil {
			return Result{}, err
		}
		var renderedBytes int
		if resolved.Included && resolved.Placement != PlacementAuditOnly && resolved.Content != "" {
			resolved, renderedBytes = fitFragmentToRenderedBudget(
				resolved,
				budget.MaxTotalBytes-injectedBytes,
				len(finalUser) > 0,
			)
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
		source := sourceFromFragment(resolved)
		switch resolved.Placement {
		case PlacementLeadingMessage:
			leading = append(leading, source)
		case PlacementFinalUserPrefix:
			finalUser = append(finalUser, source)
		}
	}

	if len(leading) > 0 {
		leadingMessages := make([]*adk.Message, 0, len(leading))
		for _, source := range leading {
			leadingMessages = append(leadingMessages, adk.UserMessage(StandaloneMessage(source.Title, source.Content, "")))
		}
		messages = append(leadingMessages, messages...)
	}
	if len(finalUser) > 0 && len(messages) > 0 {
		last := *messages[len(messages)-1]
		last.Content = PrependFinalUserSources(last.Content, finalUser)
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

func fitFragmentToRenderedBudget(fragment Fragment, remaining int, hasPriorFinalUser bool) (Fragment, int) {
	if remaining <= 0 {
		fragment.Content = ""
		fragment.Included = false
		fragment.Truncated = true
		fragment.Note = appendFragmentNote(fragment.Note, "total_budget_exhausted")
		fragment.Note = appendFragmentNote(fragment.Note, "empty_after_budget")
		return fragment, 0
	}

	renderedBytes := renderedFragmentBytes(fragment, hasPriorFinalUser)
	if renderedBytes <= remaining {
		return fragment, renderedBytes
	}
	availableContentBytes := len(fragment.Content) - (renderedBytes - remaining)
	if availableContentBytes <= 0 {
		fragment.Content = ""
		fragment.Included = false
		fragment.Truncated = true
		fragment.Note = appendFragmentNote(fragment.Note, "total_budget_exhausted")
		fragment.Note = appendFragmentNote(fragment.Note, "empty_after_budget")
		return fragment, 0
	}
	fragment.Content, _ = truncateUTF8Bytes(fragment.Content, availableContentBytes)
	fragment.Truncated = true
	fragment.Note = appendFragmentNote(fragment.Note, "total_budget_applied")
	if fragment.Content == "" {
		fragment.Included = false
		fragment.Note = appendFragmentNote(fragment.Note, "empty_after_budget")
		return fragment, 0
	}
	renderedBytes = renderedFragmentBytes(fragment, hasPriorFinalUser)
	if renderedBytes > remaining {
		fragment.Content = ""
		fragment.Included = false
		fragment.Note = appendFragmentNote(fragment.Note, "empty_after_budget")
		return fragment, 0
	}
	return fragment, renderedBytes
}

func renderedFragmentBytes(fragment Fragment, hasPriorFinalUser bool) int {
	source := sourceFromFragment(fragment)
	switch fragment.Placement {
	case PlacementLeadingMessage:
		return len(StandaloneMessage(source.Title, source.Content, ""))
	case PlacementFinalUserPrefix:
		bytes := len(finalUserSourceBlock(source))
		if hasPriorFinalUser {
			return bytes + len(contextSourceSeparator)
		}
		return bytes + len(finalUserRequestWrapper)
	default:
		return 0
	}
}

func projectFragments(ctx stdcontext.Context, projectors []ContextProjector, maxFragments, existingFragments, maxMetadataFieldBytes int) ([]Fragment, error) {
	var result []Fragment
	for _, projector := range projectors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if projector == nil {
			continue
		}
		descriptor := normalizeDescriptor(projector.Descriptor(), maxMetadataFieldBytes)
		fragments, err := projector.Project(ctx)
		if err != nil {
			return nil, fmt.Errorf("project context %q: %w", descriptor.ID, err)
		}
		if len(fragments) > maxFragments-existingFragments-len(result) {
			return nil, fmt.Errorf("context fragment count %d exceeds limit %d", existingFragments+len(result)+len(fragments), maxFragments)
		}
		for index, fragment := range fragments {
			fragment, err = applyDescriptor(fragment, descriptor, index, len(fragments), maxMetadataFieldBytes)
			if err != nil {
				return nil, err
			}
			result = append(result, fragment)
		}
	}
	return result, nil
}

func normalizeDescriptor(descriptor ContextDescriptor, maxMetadataFieldBytes int) ContextDescriptor {
	descriptor.ID, _ = truncateMetadataField(descriptor.ID, maxMetadataFieldBytes)
	descriptor.Source, _ = truncateMetadataField(descriptor.Source, maxMetadataFieldBytes)
	descriptor.Purpose, _ = truncateMetadataField(descriptor.Purpose, maxMetadataFieldBytes)
	if descriptor.Placement == "" {
		descriptor.Placement = PlacementAuditOnly
	}
	return descriptor
}

func applyDescriptor(fragment Fragment, descriptor ContextDescriptor, index, count, maxMetadataFieldBytes int) (Fragment, error) {
	fragment = boundFragmentMetadata(fragment, maxMetadataFieldBytes)
	if fragment.ID == "" {
		fragment.ID = descriptor.ID
		if count > 1 {
			fragment.ID = fmt.Sprintf("%s:%d", descriptor.ID, index+1)
		}
	}
	if fragment.Source != "" && strings.TrimSpace(fragment.Source) != descriptor.Source {
		return Fragment{}, fmt.Errorf("context projector %q changed descriptor source", descriptor.ID)
	}
	if fragment.Purpose != "" && strings.TrimSpace(fragment.Purpose) != descriptor.Purpose {
		return Fragment{}, fmt.Errorf("context projector %q changed descriptor purpose", descriptor.ID)
	}
	if fragment.Placement != "" && fragment.Placement != descriptor.Placement {
		return Fragment{}, fmt.Errorf("context projector %q changed descriptor placement", descriptor.ID)
	}
	fragment.Source = descriptor.Source
	fragment.Purpose = descriptor.Purpose
	fragment.Placement = descriptor.Placement
	if fragment.Limit <= 0 || (descriptor.Limit > 0 && fragment.Limit > descriptor.Limit) {
		fragment.Limit = descriptor.Limit
	}
	return fragment, nil
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
		Role:    string(adk.User),
		Content: fragment.Content,
		Note:    fragment.Note,
		Bytes:   len(fragment.Content),
		Chars:   utf8.RuneCountInString(fragment.Content),
	}
}
