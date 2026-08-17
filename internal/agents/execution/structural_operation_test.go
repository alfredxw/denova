package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentstructural "denova/internal/agents/context/structural"

	agent "github.com/alfredxw/denova/agent"
)

func TestStructuralOperationRejectsInvalidPublicRequestBeforeOpeningSession(t *testing.T) {
	t.Parallel()

	backend := new(publicBackend)
	for _, spec := range []agentstructural.Spec{
		{Action: agentstructural.Compact},
		{Action: agentstructural.Compact, CommandID: strings.Repeat("x", 4<<10+1)},
		{Action: agentstructural.Action("future-action"), CommandID: "unsupported-structural-action"},
	} {
		if _, err := backend.executeStructural(context.Background(), spec); !errors.Is(err, agent.ErrInvalidInput) {
			t.Fatalf("executeStructural(%#v) error = %v, want ErrInvalidInput", spec, err)
		}
	}
}
