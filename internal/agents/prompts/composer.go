package prompts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"denova/config"
)

// SystemPromptOverflowPolicy declares how one model-visible system source
// behaves when it exceeds the configured per-fragment or aggregate budget.
// Reject is used for contracts and executable instructions; Truncate is only
// appropriate for explicitly optional reference material.
type SystemPromptOverflowPolicy string

const (
	SystemPromptOverflowReject   SystemPromptOverflowPolicy = "reject"
	SystemPromptOverflowTruncate SystemPromptOverflowPolicy = "truncate"
	systemPromptTruncationMarker                            = "\n\n[System source truncated by configured context budget / 系统来源已按配置的上下文预算截断]"
)

// SystemPromptFragment is one independently attributable source in the system
// instruction. Prefix and Suffix are model-visible framing owned by Denova and
// are charged to the aggregate budget together with Content.
type SystemPromptFragment struct {
	ID       string
	Source   string
	Title    string
	Purpose  string
	Content  string
	Prefix   string
	Suffix   string
	Required bool
	Overflow SystemPromptOverflowPolicy
}

// SystemPromptManifestEntry is the immutable admission receipt for one source.
// Hashes are over source content (before and after admission), while the
// composition exposes a separate hash over the exact rendered instruction.
type SystemPromptManifestEntry struct {
	ID            string                     `json:"id"`
	Source        string                     `json:"source"`
	Title         string                     `json:"title,omitempty"`
	Purpose       string                     `json:"purpose"`
	Required      bool                       `json:"required"`
	Overflow      SystemPromptOverflowPolicy `json:"overflow"`
	OriginalBytes int                        `json:"original_bytes"`
	IncludedBytes int                        `json:"included_bytes"`
	OriginalSHA   string                     `json:"original_sha"`
	IncludedSHA   string                     `json:"included_sha"`
	Included      bool                       `json:"included"`
	Truncated     bool                       `json:"truncated,omitempty"`
	Rejected      bool                       `json:"rejected,omitempty"`
	Reason        string                     `json:"reason,omitempty"`
}

// SystemPromptComposition is the single-use artifact shared by Agent
// construction, runtime logging, settings previews, and context analysis. Its
// instruction and manifest are produced by the same admission pass.
type SystemPromptComposition struct {
	mode            string
	agentKind       string
	workspace       string
	instruction     string
	instructionHash string
	injectedBytes   int
	manifest        []SystemPromptManifestEntry
	fragments       []SystemPromptFragment
	assemblyErr     error
}

func (c SystemPromptComposition) Instruction() string { return c.instruction }

func (c SystemPromptComposition) InstructionHash() string { return c.instructionHash }

func (c SystemPromptComposition) InjectedBytes() int { return c.injectedBytes }

func (c SystemPromptComposition) Err() error { return c.assemblyErr }

func (c SystemPromptComposition) Manifest() []SystemPromptManifestEntry {
	return append([]SystemPromptManifestEntry(nil), c.manifest...)
}

// Fragments returns the admitted model-visible fragments. Optional sources
// rejected by admission remain represented only in Manifest.
func (c SystemPromptComposition) Fragments() []SystemPromptFragment {
	return append([]SystemPromptFragment(nil), c.fragments...)
}

// AgentKind identifies the runtime policy whose limits and protected contract
// were used to admit this composition.
func (c SystemPromptComposition) AgentKind() string { return c.agentKind }

// Workspace identifies the workspace bound into this composition.
func (c SystemPromptComposition) Workspace() string { return c.workspace }

// ValidateForAgent verifies that the immutable composition is complete,
// internally consistent, and admitted for the runtime that will consume it.
func (c SystemPromptComposition) ValidateForAgent(agentKind string) error {
	if c.assemblyErr != nil {
		return c.assemblyErr
	}
	if strings.TrimSpace(c.instruction) == "" {
		return fmt.Errorf("system prompt composition is required: agent=%s", strings.TrimSpace(agentKind))
	}
	if kind := strings.TrimSpace(agentKind); c.agentKind != "" && c.agentKind != kind {
		return fmt.Errorf("system prompt agent kind mismatch: composition=%s runtime=%s", c.agentKind, kind)
	}
	if c.instructionHash != systemPromptSHA(c.instruction) {
		return fmt.Errorf("system prompt composition hash mismatch: agent=%s", strings.TrimSpace(agentKind))
	}
	return nil
}

func (c SystemPromptComposition) isZero() bool {
	return strings.TrimSpace(c.mode) == "" && strings.TrimSpace(c.instruction) == "" && c.assemblyErr == nil
}

type admittedSystemPromptFragment struct {
	fragment SystemPromptFragment
	content  string
	entry    SystemPromptManifestEntry
}

func composeSystemPrompt(cfg *config.Config, agentKind, mode, workspace string, fragments []SystemPromptFragment) (SystemPromptComposition, error) {
	budget := config.ResolveAgentContext(cfg, agentKind)
	composition := SystemPromptComposition{
		mode: strings.TrimSpace(mode), agentKind: strings.TrimSpace(agentKind), workspace: strings.TrimSpace(workspace),
	}
	seen := make(map[string]struct{}, len(fragments))
	admitted := make([]admittedSystemPromptFragment, 0, len(fragments))
	activeFragments := 0
	for _, raw := range fragments {
		fragment := normalizeSystemPromptFragment(raw)
		if err := validateSystemPromptFragment(fragment, budget.MaxMetadataFieldBytes, seen); err != nil {
			return composition, fmt.Errorf("assemble system prompt agent=%s: %w", agentKind, err)
		}
		seen[fragment.ID] = struct{}{}
		if fragment.Required || fragment.Content != "" {
			activeFragments++
			if activeFragments > budget.MaxFragments {
				return composition, fmt.Errorf("system prompt fragment count %d exceeds configured limit %d: agent=%s", activeFragments, budget.MaxFragments, agentKind)
			}
		}
		original := fragment.Content
		entry := SystemPromptManifestEntry{
			ID: fragment.ID, Source: fragment.Source, Title: fragment.Title, Purpose: fragment.Purpose,
			Required: fragment.Required, Overflow: fragment.Overflow,
			OriginalBytes: len(original), OriginalSHA: systemPromptSHA(original),
		}
		content := original
		if content == "" {
			if fragment.Required {
				return composition, fmt.Errorf("required system prompt fragment %q is empty", fragment.ID)
			}
			entry.Rejected = true
			entry.Reason = "empty_optional_source"
			entry.IncludedSHA = systemPromptSHA("")
			admitted = append(admitted, admittedSystemPromptFragment{fragment: fragment, entry: entry})
			continue
		}
		if len(content) > budget.MaxFragmentBytes {
			if fragment.Overflow != SystemPromptOverflowTruncate {
				return composition, fmt.Errorf("system prompt fragment %q exceeds configured per-source limit: bytes=%d limit=%d", fragment.ID, len(content), budget.MaxFragmentBytes)
			}
			var marked bool
			content, marked = truncateSystemPromptWithMarker(content, budget.MaxFragmentBytes)
			entry.Truncated = true
			entry.Reason = "fragment_budget"
			if !marked {
				entry.Rejected = true
				entry.Included = false
				entry.Reason = appendSystemPromptReason(entry.Reason, "truncation_marker_does_not_fit")
			}
		}
		entry.Included = content != ""
		entry.IncludedBytes = len(content)
		entry.IncludedSHA = systemPromptSHA(content)
		admitted = append(admitted, admittedSystemPromptFragment{fragment: fragment, content: content, entry: entry})
	}

	for renderedSystemPromptBytes(admitted) > budget.MaxTotalInjectedBytes {
		over := renderedSystemPromptBytes(admitted) - budget.MaxTotalInjectedBytes
		changed := false
		for i := len(admitted) - 1; i >= 0 && over > 0; i-- {
			part := &admitted[i]
			if !part.entry.Included || part.fragment.Overflow != SystemPromptOverflowTruncate || part.fragment.Required {
				continue
			}
			currentRendered := len(part.fragment.Prefix) + len(part.content) + len(part.fragment.Suffix)
			targetContentBytes := len(part.content) - over
			if targetContentBytes <= len(systemPromptTruncationMarker) || currentRendered <= over {
				part.content = ""
				part.entry.Included = false
				part.entry.Rejected = true
				part.entry.Truncated = true
				part.entry.Reason = appendSystemPromptReason(part.entry.Reason, "total_budget_exhausted")
			} else {
				var marked bool
				part.content, marked = truncateSystemPromptWithMarker(part.content, targetContentBytes)
				if !marked {
					part.content = ""
					part.entry.Included = false
					part.entry.Rejected = true
					part.entry.Reason = appendSystemPromptReason(part.entry.Reason, "truncation_marker_does_not_fit")
				}
				part.entry.Truncated = true
				part.entry.Reason = appendSystemPromptReason(part.entry.Reason, "total_budget")
			}
			part.entry.IncludedBytes = len(part.content)
			part.entry.IncludedSHA = systemPromptSHA(part.content)
			changed = true
			over = renderedSystemPromptBytes(admitted) - budget.MaxTotalInjectedBytes
		}
		if !changed {
			return composition, fmt.Errorf("system prompt exceeds configured total budget: bytes=%d limit=%d agent=%s", renderedSystemPromptBytes(admitted), budget.MaxTotalInjectedBytes, agentKind)
		}
	}

	var instruction strings.Builder
	composition.manifest = make([]SystemPromptManifestEntry, 0, len(admitted))
	composition.fragments = make([]SystemPromptFragment, 0, len(admitted))
	for _, part := range admitted {
		composition.manifest = append(composition.manifest, part.entry)
		if !part.entry.Included {
			continue
		}
		resolved := part.fragment
		resolved.Content = part.content
		composition.fragments = append(composition.fragments, resolved)
		instruction.WriteString(resolved.Prefix)
		instruction.WriteString(part.content)
		instruction.WriteString(resolved.Suffix)
	}
	composition.instruction = instruction.String()
	composition.injectedBytes = len(composition.instruction)
	composition.instructionHash = systemPromptSHA(composition.instruction)
	return composition, nil
}

func normalizeSystemPromptFragment(fragment SystemPromptFragment) SystemPromptFragment {
	fragment.ID = strings.TrimSpace(fragment.ID)
	fragment.Source = strings.TrimSpace(fragment.Source)
	fragment.Title = strings.TrimSpace(fragment.Title)
	fragment.Purpose = strings.TrimSpace(fragment.Purpose)
	fragment.Content = strings.TrimSpace(strings.ToValidUTF8(fragment.Content, "\uFFFD"))
	if fragment.Overflow == "" {
		fragment.Overflow = SystemPromptOverflowReject
	}
	return fragment
}

func validateSystemPromptFragment(fragment SystemPromptFragment, metadataLimit int, seen map[string]struct{}) error {
	if fragment.ID == "" {
		return fmt.Errorf("system prompt fragment id is required")
	}
	if fragment.Source == "" {
		return fmt.Errorf("system prompt fragment %q source is required", fragment.ID)
	}
	if fragment.Purpose == "" {
		return fmt.Errorf("system prompt fragment %q purpose is required", fragment.ID)
	}
	if _, exists := seen[fragment.ID]; exists {
		return fmt.Errorf("duplicate system prompt fragment id %q", fragment.ID)
	}
	for name, value := range map[string]string{"id": fragment.ID, "source": fragment.Source, "title": fragment.Title, "purpose": fragment.Purpose} {
		if len(value) > metadataLimit {
			return fmt.Errorf("system prompt fragment %q metadata %s exceeds configured limit: bytes=%d limit=%d", fragment.ID, name, len(value), metadataLimit)
		}
	}
	switch fragment.Overflow {
	case SystemPromptOverflowReject, SystemPromptOverflowTruncate:
	default:
		return fmt.Errorf("system prompt fragment %q has invalid overflow policy %q", fragment.ID, fragment.Overflow)
	}
	if fragment.Required && fragment.Overflow == SystemPromptOverflowTruncate {
		return fmt.Errorf("required system prompt fragment %q cannot use truncation policy", fragment.ID)
	}
	return nil
}

func renderedSystemPromptBytes(parts []admittedSystemPromptFragment) int {
	total := 0
	for _, part := range parts {
		if part.entry.Included {
			total += len(part.fragment.Prefix) + len(part.content) + len(part.fragment.Suffix)
		}
	}
	return total
}

func truncateSystemPromptUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || value == "" {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return strings.TrimSpace(value[:end])
}

func truncateSystemPromptWithMarker(value string, maxBytes int) (string, bool) {
	value = strings.TrimSuffix(value, systemPromptTruncationMarker)
	if maxBytes <= len(systemPromptTruncationMarker) {
		return "", false
	}
	content := truncateSystemPromptUTF8(value, maxBytes-len(systemPromptTruncationMarker))
	if content == "" {
		return "", false
	}
	return content + systemPromptTruncationMarker, true
}

func systemPromptSHA(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func appendSystemPromptReason(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}

func failedSystemPromptComposition(mode, agentKind, workspace string, err error) SystemPromptComposition {
	return SystemPromptComposition{mode: mode, agentKind: agentKind, workspace: workspace, assemblyErr: err}
}
