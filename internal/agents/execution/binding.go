package execution

import (
	"fmt"
	"strings"

	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func commandContextRefs(req agentchat.ChatRequest) []runstate.ContextRef {
	caller := agentchat.CallerView(req)
	refs := make([]runstate.ContextRef, 0, len(caller.References)+len(caller.LoreReferences)+len(caller.StyleScenes)+len(caller.Selections)+len(caller.IDEContext.OpenFiles)+1)
	appendRef := func(source, resource, selector string) {
		resource = strings.TrimSpace(resource)
		if resource == "" {
			return
		}
		refs = append(refs, runstate.ContextRef{Source: source, Resource: resource, Selector: selector, ByteLimit: agentchat.ReferenceFileByteLimit})
	}
	for _, resource := range caller.References {
		appendRef("workspace_file", resource, "")
	}
	for _, resource := range caller.LoreReferences {
		appendRef("lore_item", resource, "")
	}
	for _, scene := range caller.StyleScenes {
		appendRef("style_scene", scene, "")
	}
	for _, selection := range caller.Selections {
		appendRef("editor_selection", selection.FileName, fmt.Sprintf("lines:%d-%d", selection.StartLine, selection.EndLine))
	}
	appendRef("ide_focus", caller.IDEContext.CurrentFile, "current")
	for _, resource := range caller.IDEContext.OpenFiles {
		appendRef("ide_focus", resource, "open")
	}
	return refs
}

func newOperationIdentity(prefix string) string {
	return agentrun.NewID(prefix)
}
