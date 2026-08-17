package anthropicmessages

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/alfredxw/denova/agent/providers"
)

// ListModels implements Anthropic's optional paginated model discovery API.
// Invalid or duplicate entries do not discard valid suggestions from the same
// response, and callers remain free to use model IDs that are not advertised.
func (*Adapter) ListModels(ctx context.Context, config providers.ModelConfig) ([]providers.ModelInfo, error) {
	if config.Protocol != providers.ProtocolAnthropicMessages {
		return nil, fmt.Errorf("anthropic messages model listing: protocol must be %q", providers.ProtocolAnthropicMessages)
	}

	client := anthropic.NewClient(anthropicClientOptions(config)...)
	pager := client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{
		Limit: param.NewOpt(int64(1000)),
	})
	models := make([]providers.ModelInfo, 0)
	seen := make(map[string]struct{})
	for pager.Next() {
		candidate := pager.Current()
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, providers.ModelInfo{
			ID:          id,
			DisplayName: strings.TrimSpace(candidate.DisplayName),
		})
	}
	if err := pager.Err(); err != nil {
		return nil, adaptAPIError(err)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

var _ providers.ModelListingAdapter = (*Adapter)(nil)
