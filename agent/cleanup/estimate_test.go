package cleanup

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestEstimateMessagesUsesCanonicalAgentEstimator(t *testing.T) {
	messages := []*agent.Message{agent.UserMessageWithAttachments("inspect", []agent.Attachment{{
		Name: "map.png", MediaType: "image/png", Size: 3_000, Path: "/inputs/map.png", SHA256: "digest",
	}})}
	if got, want := EstimateMessages(messages), agent.EstimateMessagesTokens(messages); got != want {
		t.Fatalf("Cleanup estimate = %d, canonical estimate = %d", got, want)
	}
}
