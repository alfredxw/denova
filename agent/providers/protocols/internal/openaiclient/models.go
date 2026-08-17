package openaiclient

import (
	"context"
	"sort"
	"strings"

	sdk "github.com/openai/openai-go/v3"

	"github.com/alfredxw/denova/agent/providers"
)

// ListModels reads one OpenAI-compatible /models response and tolerates blank
// or duplicate entries so one malformed item does not discard useful results.
func ListModels(ctx context.Context, config providers.ModelConfig) ([]providers.ModelInfo, error) {
	client := sdk.NewClient(Options(config)...)
	page, err := client.Models.List(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]providers.ModelInfo, 0, len(page.Data))
	seen := make(map[string]struct{}, len(page.Data))
	for _, candidate := range page.Data {
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, providers.ModelInfo{
			ID:      id,
			OwnedBy: strings.TrimSpace(candidate.OwnedBy),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}
