package codex

import (
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

// InputEstimator uses the same resolved model identity as Run. Unrecognized CLI
// aliases use the conservative shared policy rather than compressed file size.
func (c *Client) InputEstimator(input external.Input) agent.InputEstimator {
	model := ""
	if input.Selection.Codex != nil {
		model = input.Selection.Codex.Model
	}
	if input.Selection.ModelProfileID() != "" {
		model = c.apiModel
	}
	return (providers.ModelConfig{Model: model}).InputEstimator()
}
