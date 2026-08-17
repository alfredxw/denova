package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const sessionTranscriptSourceKind = "denova.session.transcript"

// CanonicalTranscript projects the complete product Session history into the
// public Agent transcript before admission or inspection. Product UI history
// remains the canonical source; Agent owns the derived raw generation used by
// Cleanup, Compaction, replay, and exact provider projection.
func (c *SessionConversation) CanonicalTranscript(ctx context.Context) (agent.TranscriptSyncRequest, error) {
	if c == nil || c.session == nil {
		return agent.TranscriptSyncRequest{}, fmt.Errorf("session canonical transcript is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return agent.TranscriptSyncRequest{}, err
		}
	}
	if err := c.session.RefreshCanonical(ctx); err != nil {
		return agent.TranscriptSyncRequest{}, err
	}
	messages, cursor := c.session.GetEffectiveMessagesWithCursor()
	identityDigest := sha256.Sum256([]byte(strings.TrimSpace(c.agentKind) + "\x00" + c.session.ID))
	return agent.TranscriptSyncRequest{
		Source: agent.CapabilityIdentity{
			Kind: sessionTranscriptSourceKind, Version: 1,
			ConfigHash: hex.EncodeToString(identityDigest[:]),
		},
		SourceRevision: cursor.Revision,
		Messages:       messages,
	}, nil
}
