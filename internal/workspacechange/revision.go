package workspacechange

import (
	"crypto/sha256"
	"fmt"
)

// Revision returns the stable content revision used for optimistic concurrency.
func Revision(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", sum[:])
}

func stateRevision(content []byte, exists bool) string {
	if !exists {
		return "missing"
	}
	return Revision(content)
}
