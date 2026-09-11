package compaction

// Result is the product response to an explicit Compaction command. Detailed
// model and cache measurements remain available through Agent diagnostics.
type Result struct {
	Triggered          bool   `json:"triggered"`
	SkippedReason      string `json:"skipped_reason,omitempty"`
	Revision           uint64 `json:"revision"`
	Summary            string `json:"summary"`
	TokensBefore       int    `json:"tokens_before"`
	TokensAfter        int    `json:"tokens_after"`
	SourceMessageCount int    `json:"source_message_count"`
}
