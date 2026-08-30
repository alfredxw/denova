package agent

import (
	"context"
	"fmt"
	"strings"
)

// projectToolArtifactPaths resolves portable artifact references only on the
// detached provider request. Canonical loop state keeps owner-relative paths.
func projectToolArtifactPaths(ctx context.Context, storage ToolArtifactStorage, messages []*Message) ([]*Message, error) {
	result := cloneMessages(messages)
	if storage == nil {
		return result, nil
	}
	resolver, ok := storage.(ToolArtifactPathResolver)
	if !ok {
		return result, nil
	}
	for _, message := range result {
		if message == nil || message.ToolResult == nil {
			continue
		}
		paths := make(map[string]string)
		for index := range message.ToolResult.Artifacts {
			stored := strings.TrimSpace(message.ToolResult.Artifacts[index].ReadablePath)
			if stored == "" {
				continue
			}
			resolved, err := resolver.ResolveToolArtifactPath(ctx, stored)
			if err != nil {
				return nil, fmt.Errorf("resolve tool artifact %q: %w", message.ToolResult.Artifacts[index].ID, err)
			}
			paths[stored] = resolved
			message.ToolResult.Artifacts[index].ReadablePath = resolved
		}
		if len(paths) == 0 {
			continue
		}
		message.Content = replaceProjectedArtifactPaths(message.Content, paths)
		if hints := message.ToolResult.ContextHints; hints != nil {
			hints.Recovery.ArtifactPath = replaceProjectedArtifactPaths(hints.Recovery.ArtifactPath, paths)
		}
		if receipt := message.ToolResult.ProtectedReceipt; receipt != nil {
			receipt.Outcome = replaceProjectedArtifactPaths(receipt.Outcome, paths)
		}
	}
	return result, nil
}

func replaceProjectedArtifactPaths(value string, paths map[string]string) string {
	for stored, resolved := range paths {
		value = strings.ReplaceAll(value, stored, resolved)
	}
	return value
}
