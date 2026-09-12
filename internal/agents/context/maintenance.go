// Package context owns model-context assembly and read-only projections.
package context

// CompactionCheckpoint is a display view. Agent owns coverage and recovery.
type CompactionCheckpoint struct {
	Revision           uint64 `json:"revision"`
	Summary            string `json:"summary"`
	TokensBefore       int    `json:"tokens_before"`
	TokensAfter        int    `json:"tokens_after"`
	SourceMessageCount int    `json:"source_message_count"`
}
