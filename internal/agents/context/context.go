// Package context adapts the reusable agent context assembler to Denova's
// localized prompt wording. Product code depends on this package only for that
// presentation policy; budgets, provenance, hashing, projection, and placement
// are owned by github.com/alfredxw/denova/agent/context.
package context

import (
	"strings"

	agentcontext "github.com/alfredxw/denova/agent/context"
)

type Placement = agentcontext.Placement

const (
	PlacementLeadingMessage  = agentcontext.PlacementLeadingMessage
	PlacementFinalUserPrefix = agentcontext.PlacementFinalUserPrefix
	PlacementAuditOnly       = agentcontext.PlacementAuditOnly

	DefaultPreviewChars          = agentcontext.DefaultPreviewChars
	DefaultMaxFragmentBytes      = agentcontext.DefaultMaxFragmentBytes
	DefaultMaxTotalBytes         = agentcontext.DefaultMaxTotalBytes
	DefaultMaxFragments          = agentcontext.DefaultMaxFragments
	DefaultMaxMetadataFieldBytes = agentcontext.DefaultMaxMetadataFieldBytes

	finalUserSourceNote     = "State snapshots may be stale; tool reads are authoritative."
	truncationNotice        = "Content truncated; use a tool to read the complete source if needed."
	contextSourceSeparator  = "\n\n---\n\n"
	finalUserRequestWrapper = "\n\n---\n\n# Current User Request (Highest Priority)\n\n"
)

type Budget = agentcontext.Budget
type Fragment = agentcontext.Fragment
type AssembleRequest = agentcontext.AssembleRequest
type Result = agentcontext.Result
type Source = agentcontext.Source
type LedgerPart = agentcontext.LedgerPart
type AnalysisPart = agentcontext.AnalysisPart

func DefaultBudget() Budget {
	return agentcontext.DefaultBudget()
}

func SourceSummary(sources []Source, previewChars int) string {
	return agentcontext.SourceSummary(sources, previewChars)
}

func Preview(value string, maxRunes int) string {
	return agentcontext.Preview(value, maxRunes)
}

func StandaloneMessage(title, content, note string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Stable Context"
	}
	note = strings.TrimSpace(note)
	if note == "" {
		note = "The following content is a bounded, low-churn snapshot from the current workspace, placed near the beginning of model input for stable prefix caching. Use tools with the source path when more complete or current content is required."
	}
	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	builder.WriteString(note)
	builder.WriteString("\n\n")
	builder.WriteString(content)
	return builder.String()
}

func PrependFinalUserSource(agentMessage, title, content string) string {
	return PrependFinalUserSources(agentMessage, []Source{{Title: title, Content: content, Included: true}})
}

func PrependFinalUserSources(agentMessage string, sources []Source) string {
	included := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source.Included && strings.TrimSpace(source.Content) != "" {
			included = append(included, source)
		}
	}
	if len(included) == 0 {
		return agentMessage
	}
	var builder strings.Builder
	for index, source := range included {
		if index > 0 {
			builder.WriteString(contextSourceSeparator)
		}
		builder.WriteString(finalUserSourceBlock(source))
	}
	builder.WriteString(finalUserRequestWrapper)
	builder.WriteString(strings.TrimSpace(agentMessage))
	return builder.String()
}

func finalUserSourceBlock(source Source) string {
	title := strings.TrimSpace(source.Title)
	if title == "" {
		title = "Current Dynamic Context"
	}
	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	builder.WriteString(finalUserSourceNote)
	builder.WriteString("\n\n")
	builder.WriteString(renderSourceContent(source))
	return builder.String()
}

func renderSourceContent(source Source) string {
	content := strings.TrimSpace(source.Content)
	var builder strings.Builder
	if source.Source == "workspace.file.reference" {
		// The renderer owns framing so truncation can never remove the closing
		// fence. Raw Markdown remains the auditable content and hash input.
		builder.WriteString("```markdown\n")
		builder.WriteString(content)
		builder.WriteString("\n```")
	} else {
		builder.WriteString(content)
	}
	if source.Truncated {
		builder.WriteString("\n\n> ")
		builder.WriteString(truncationNotice)
	}
	return builder.String()
}
