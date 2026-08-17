package prompts

import (
	"strings"

	"denova/config"
	agentcontext "denova/internal/agents/context"
)

func ChapterSplitRegexSystemInstruction() string {
	return strings.Join([]string{
		"Identify chapter and volume heading lines for Denova novel import.",
		"Output only a JSON object with schema {\"split_regex\":\"...\",\"reason\":\"...\"}.",
		"split_regex must be a Go regexp that matches chapter and volume heading lines independently. Do not use multiline matching.",
		"When a heading contains both a numbering prefix and a prose title, prefer capture group 1 for the complete chapter title. A pattern without capture groups is also valid.",
		"Keep the regexp conservative: match only chapter or volume headings, not ordinary prose sentences.",
		"Do not return Markdown, explanatory text, or code fences.",
	}, "\n")
}

func ContextCompactionSystemInstruction() string {
	base := staticPromptAsset(contextCompactionWorkflowAsset)
	return base + "\n\nUse the following single stable Markdown checkpoint schema for every source kind. Sections that do not apply may be empty, but do not rename them or create an alternative format:\n" +
		agentcontext.CompactionCheckpointSchema()
}

// ComposeContextCompactionInstruction admits the internal checkpoint protocol
// against the source Agent's context budgets. It deliberately excludes
// user-configurable Agent prompts because compaction is a side fork of the
// source request, not an independently configurable Agent identity.
func ComposeContextCompactionInstruction(cfg *config.Config, sourceAgentKind string) (SystemPromptComposition, error) {
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	return composeSystemPrompt(cfg, sourceAgentKind, "context_compaction", workspace, []SystemPromptFragment{{
		ID:       "context_checkpoint_protocol",
		Source:   "internal runtime",
		Title:    "Context checkpoint maintenance protocol",
		Purpose:  "compile a bounded incremental checkpoint for the source Agent request",
		Content:  ContextCompactionSystemInstruction(),
		Prefix:   "# Internal Context Checkpoint Protocol\n\n",
		Required: true,
		Overflow: SystemPromptOverflowReject,
	}})
}
