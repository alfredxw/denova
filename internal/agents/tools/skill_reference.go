package tools

import (
	"context"
	"errors"
	"strings"

	agenttools "github.com/alfredxw/denova/agent/tools"

	novaskills "denova/internal/agents/skills"
)

type skillReferenceReadInput struct {
	Path   string `json:"path" jsonschema_description:"Canonical skill://<name>/references/<file> URI advertised by a loaded Skill."`
	Offset int    `json:"offset,omitempty" jsonschema:"minimum=1" jsonschema_description:"One-based first line; defaults to 1."`
	Limit  int    `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum selected lines; defaults to the configured bounded reference window."`
}

func newSkillReferenceReadAdapter(backend *novaskills.Backend) (agenttools.ReadAdapter, error) {
	if backend == nil {
		return nil, errors.New("skill reference backend is nil")
	}
	return agenttools.NewReadAdapter(
		"skill_reference",
		func(_ context.Context, path string) (bool, error) {
			return strings.HasPrefix(strings.ToLower(strings.TrimSpace(path)), "skill://"), nil
		},
		func(ctx context.Context, input skillReferenceReadInput) (agenttools.ReadResult, error) {
			result, err := backend.ReadReference(ctx, input.Path, input.Offset, input.Limit)
			if err != nil {
				return agenttools.ReadResult{}, err
			}
			returned := referenceLineCount(result.Content)
			return agenttools.ReadResult{
				Path: result.URI, Kind: "skill_reference", Content: result.Content,
				Offset: result.Offset, Limit: returned, Total: result.Total,
				Truncated: result.Offset-1+returned < result.Total,
			}, nil
		},
	)
}

func referenceLineCount(content string) int {
	if content == "" {
		return 0
	}
	count := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		count++
	}
	return count
}
