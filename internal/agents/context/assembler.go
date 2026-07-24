package context

import agentcontext "github.com/alfredxw/denova/agent/context"

// Assembler is the reusable bounded assembler configured with Denova's prompt
// renderer. The core policy remains in the standalone agent module; only the
// product's localized wrapper text lives here.
type Assembler = agentcontext.Assembler

func NewAssembler(budget Budget) *Assembler {
	return agentcontext.NewAssembler(budget, agentcontext.WithRenderer(denovaRenderer{}))
}

type denovaRenderer struct{}

func (denovaRenderer) RenderLeading(fragment Fragment) string {
	return StandaloneMessage(fragment.Title, fragment.Content, "")
}

func (denovaRenderer) RenderFinalUser(userRequest string, fragments []Fragment) string {
	sources := make([]Source, 0, len(fragments))
	for _, fragment := range fragments {
		sources = append(sources, Source{
			Source: fragment.Source, Title: fragment.Title, Purpose: fragment.Purpose,
			Content: fragment.Content, Placement: fragment.Placement, Limit: fragment.Limit,
			Included: fragment.Included, Truncated: fragment.Truncated, Note: fragment.Note,
		})
	}
	return PrependFinalUserSources(userRequest, sources)
}
