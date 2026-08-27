package context

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestMessageEstimateUsesCanonicalAgentEstimator(t *testing.T) {
	message := agent.UserMessageWithAttachments("inspect", []agent.Attachment{{
		Name: "map.png", MediaType: "image/png", Size: 3_000, Path: "/inputs/map.png", SHA256: "digest",
	}})
	if got, want := EstimateMessageTokens(message), agent.EstimateMessageTokens(message); got != want {
		t.Fatalf("context estimate = %d, canonical estimate = %d", got, want)
	}
}
