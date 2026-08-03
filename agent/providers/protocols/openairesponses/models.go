package openairesponses

import (
	"context"
	"fmt"

	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/internal/openaiclient"
)

// ListModels implements the optional OpenAI-compatible model discovery seam.
func (*Adapter) ListModels(ctx context.Context, config providers.ModelConfig) ([]providers.ModelInfo, error) {
	if config.Protocol != providers.ProtocolOpenAIResponses {
		return nil, fmt.Errorf("openai responses model listing: protocol must be %q", providers.ProtocolOpenAIResponses)
	}
	models, err := openaiclient.ListModels(ctx, config)
	if err != nil {
		return nil, adaptAPIError(err)
	}
	return models, nil
}

var _ providers.ModelListingAdapter = (*Adapter)(nil)
