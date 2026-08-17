package prompts

import (
	_ "embed"
	"strings"
)

// Static prompt assets stay as Markdown so their review history, cache-facing
// byte changes, and model-visible structure remain obvious. Dynamic prompt
// builders continue to live in typed Go code.

//go:embed assets/general-agent.md
var generalAgentWorkflowAsset string

//go:embed assets/harness-optimizer.md
var harnessOptimizerWorkflowAsset string

//go:embed assets/interactive-director.md
var interactiveDirectorWorkflowAsset string

//go:embed assets/context-compaction.md
var contextCompactionWorkflowAsset string

func staticPromptAsset(content string) string {
	return strings.TrimSpace(content)
}
