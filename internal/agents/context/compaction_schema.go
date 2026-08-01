package context

import "strings"

var compactionCheckpointHeadings = []string{
	"## Goal",
	"## Constraints",
	"## Current state",
	"## Decisions and rationale",
	"## Confirmed facts and sources",
	"## Tool outcomes and readable artifacts",
	"## Failures and rejected approaches",
	"## Unresolved issues",
	"## Next actions",
	"## Critical context that must not be lost",
}

// CompactionCheckpointSchema is the canonical Markdown shape shared by the
// compaction prompt and every runtime fork request.
func CompactionCheckpointSchema() string {
	return strings.Join(compactionCheckpointHeadings, "\n") + "\n"
}
