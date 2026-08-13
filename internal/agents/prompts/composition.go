package prompts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	"denova/internal/book"
)

// IDEStoryTeller describes the resolved writing-director rules used by one
// Agent run. Runtime workspace state is projected separately at turn time.
type IDEStoryTeller struct {
	ID                      string
	Name                    string
	Description             string
	Prompt                  string
	StyleRules              []StyleRule
	ImagePresetID           string
	ImagePresetName         string
	ImagePresetSystemPrompt string
}

// ConfigManagerResourceSkill is a bounded, already-resolved Skill body that
// config_manager should treat as run-scoped schema/workflow guidance.
type ConfigManagerResourceSkill struct {
	Name        string
	Description string
	Content     string
}

// ComposeGeneralInstruction assembles the project-scoped contract for the
// General Agent. The project directory is its working root; Denova's user data
// directory receives no implicit privilege or restriction when explicitly
// added as a project.
func ComposeGeneralInstruction(cfg *config.Config) (SystemPromptComposition, error) {
	workspace := ""
	if cfg != nil {
		workspace = strings.TrimSpace(cfg.Workspace)
	}
	return ComposeBuiltinSystemInstruction(
		cfg,
		config.AgentKindGeneral,
		"general",
		workspace,
		"builtin_base",
		"General Agent workflow",
		"define the project-scoped general-purpose agent workflow",
		strings.Join([]string{
			"You are Denova's general-purpose project Agent. Complete research, development, writing, organization, and automation tasks in the current Project.",
			"Understand the request and relevant project state, then use the available tools as needed. Follow applicable project instruction files, including instructions near the files you change.",
			"Prefer dedicated file and search tools when they fit; independent tool calls may run in parallel.",
			"Write code in the surrounding style, including its naming and comment density. Make the smallest complete change and verify it in proportion to its impact.",
			"Report the actual outcome: what changed, what was verified, any failure or skipped check, and any remaining limitation.",
		}, "\n\n"),
	)
}

func ComposeHarnessOptimizerInstruction(cfg *config.Config) (SystemPromptComposition, error) {
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	return ComposeBuiltinSystemInstruction(
		cfg, config.AgentKindHarnessOptimizer, "harness_optimizer", workspace,
		"harness_optimizer_builtin", "Harness Optimizer",
		"analyze trajectory evidence and make minimal validated changes in the live Harness State directory",
		strings.TrimSpace(`# Harness Optimizer

You improve Denova's user-level Harness State from durable trajectory evidence or direct user instruction.

## Working model

- The current workspace is the live Harness State directory. Use ordinary read, glob, grep, write, edit, and shell tools to inspect or modify it. Every file edit takes effect immediately; there is no draft or publish step.
- State supports prompts/<agent-kind>.md, context/<id>.md, tools.toml, and subagents/<id>.md. Skills remain in the existing Skills directory and are not copied into State.
- Keep changes small, reusable, and evidence-backed. Prefer no change when a signal is project-specific, temporary, contradictory, or weak.
- Do not operate on .git or private runtime paths. Validation, Git history, and restore are application responsibilities.
- Before finishing, inspect the complete live directory, explain the evidence and intended behavioral effect, and call out a no-op explicitly when appropriate.
`),
	)
}

// ComposeInstruction assembles and admits the exact IDE system instruction.
func ComposeInstruction(cfg *config.Config, state *book.State, teller IDEStoryTeller) (SystemPromptComposition, error) {
	workspace := workspaceForPrompt(cfg, state)
	builtIn := make([]SystemPromptFragment, 0, 3+len(teller.StyleRules))
	builtIn = append(builtIn, tellerSystemPromptFragment("ide_teller", "Default Writing Director Rules", teller.ID, teller.Name, teller.Description, teller.Prompt))
	builtIn = append(builtIn, styleRuleSystemPromptFragments(teller.StyleRules)...)
	builtIn = append(builtIn, SystemPromptFragment{
		ID: "builtin_base", Source: "Denova built-in", Title: "Writing workflow",
		Purpose: "define the built-in writing workflow and workspace rules",
		Content: ideFlowInstruction(cfg, workspace), Required: true, Overflow: SystemPromptOverflowReject,
	})
	if prompt := strings.TrimSpace(teller.ImagePresetSystemPrompt); prompt != "" {
		var content strings.Builder
		content.WriteString("## Image Preset System Rules (Image Generation Only)\n\n")
		if id := strings.TrimSpace(teller.ImagePresetID); id != "" {
			content.WriteString("- id: " + id + "\n")
		}
		if name := strings.TrimSpace(teller.ImagePresetName); name != "" {
			content.WriteString("- name: " + name + "\n")
		}
		content.WriteString("\nThe following rules apply only when constructing the image prompt for `generate_image`. Do not apply these visual constraints to ordinary prose writing, lore changes, or non-image tasks.\n\n")
		content.WriteString(prompt)
		builtIn = append(builtIn, SystemPromptFragment{
			ID: "image_preset", Source: "image preset configuration",
			Title: "Image preset system rules", Purpose: "constrain image prompts generated during writing tasks",
			Content: content.String(), Overflow: SystemPromptOverflowTruncate,
		})
	}
	return composeProtectedSystemInstruction(cfg, config.AgentKindIDE, "ide", workspace, builtIn)
}

// BuildInstructionComposition preserves the existing display API while
// retaining any admission error on the returned artifact.
func BuildInstructionComposition(cfg *config.Config, state *book.State, teller IDEStoryTeller) SystemPromptComposition {
	composition, err := ComposeInstruction(cfg, state, teller)
	if err != nil {
		return failedSystemPromptComposition("ide", config.AgentKindIDE, workspaceForPrompt(cfg, state), err)
	}
	return composition
}

// ComposeInteractiveStoryInstruction assembles and admits the exact game
// narrator instruction without silently pre-truncating style sources.
func ComposeInteractiveStoryInstruction(cfg *config.Config, state *book.State, teller InteractiveStorySystemInstructionInput) (SystemPromptComposition, error) {
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	if state != nil {
		if workspace == "" {
			workspace = state.Workspace()
		}
	}
	builtIn := make([]SystemPromptFragment, 0, 3+len(teller.StyleRules))
	builtIn = append(builtIn, tellerSystemPromptFragment(
		"interactive_teller", "Director System Rules", teller.StoryTellerID, teller.StoryTellerName,
		teller.StoryTellerDescription, teller.StoryTellerSystemPrompt,
	))
	builtIn = append(builtIn, styleRuleSystemPromptFragments(teller.StyleRules)...)
	baseInput := teller
	baseInput.StoryTellerSystemPrompt = ""
	baseInput.StyleRules = nil
	baseInput.Workspace = workspace
	if baseInput.ReplyTargetChars <= 0 && cfg != nil {
		baseInput.ReplyTargetChars = cfg.InteractiveReplyTargetChars
	}
	builtIn = append(builtIn, SystemPromptFragment{
		ID: "builtin_base", Source: "Denova built-in", Title: "Interactive story workflow",
		Purpose: "define the built-in game narration workflow and output behavior",
		Content: BuildInteractiveStoryFlowInstruction(baseInput), Required: true, Overflow: SystemPromptOverflowReject,
	})
	return composeProtectedSystemInstruction(cfg, config.AgentKindInteractiveStory, "interactive", workspace, builtIn)
}

// BuildInteractiveStoryInstructionComposition preserves the display API.
func BuildInteractiveStoryInstructionComposition(cfg *config.Config, state *book.State, teller InteractiveStorySystemInstructionInput) SystemPromptComposition {
	composition, err := ComposeInteractiveStoryInstruction(cfg, state, teller)
	if err != nil {
		workspace := ""
		if cfg != nil {
			workspace = cfg.Workspace
		}
		return failedSystemPromptComposition("interactive", config.AgentKindInteractiveStory, workspace, err)
	}
	return composition
}

// ComposeInteractiveDirectorInstruction assembles the exact background
// director instruction used by both execution and context analysis.
func ComposeInteractiveDirectorInstruction(cfg *config.Config, state *book.State) (SystemPromptComposition, error) {
	return ComposeBuiltinSystemInstruction(cfg, config.AgentKindInteractiveDirector, "interactive_director", workspaceForPrompt(cfg, state), "builtin_base", "Background Director system rules", "define the interactive director planning workflow", BuildInteractiveDirectorSystemInstruction())
}

// ComposeConfigManagerInstruction assembles the config manager prompt and
// gives every auto-loaded Skill its own independently bounded source receipt.
func ComposeConfigManagerInstruction(cfg *config.Config, state *book.State, resourceSkills ...ConfigManagerResourceSkill) (SystemPromptComposition, error) {
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	if state != nil {
		if workspace == "" {
			workspace = state.Workspace()
		}
	}
	builtIn := []SystemPromptFragment{{
		ID: "builtin_base", Source: "Denova built-in", Title: "Config Manager built-in rules",
		Purpose: "define resource configuration workflows and safety boundaries",
		Content: configManagerFlowInstructionFor(workspace), Required: true, Overflow: SystemPromptOverflowReject,
	}}
	skillOrdinal := 0
	skillOccurrences := make(map[string]int, len(resourceSkills))
	for _, skill := range resourceSkills {
		name := strings.TrimSpace(skill.Name)
		content := strings.TrimSpace(skill.Content)
		if name == "" || content == "" {
			continue
		}
		skillOrdinal++
		skillOccurrences[name]++
		var skillContent strings.Builder
		skillContent.WriteString("### /")
		skillContent.WriteString(name)
		skillContent.WriteString("\n\n")
		if description := strings.TrimSpace(skill.Description); description != "" {
			skillContent.WriteString("description: ")
			skillContent.WriteString(description)
			skillContent.WriteString("\n\n")
		}
		skillContent.WriteString(content)
		prefix := "\n\n## Config Manager Skill\n\nUse the active Skill below for this run. Read only the references needed for the requested resource.\n\n"
		if skillOrdinal > 1 {
			prefix = "\n"
		}
		builtIn = append(builtIn, SystemPromptFragment{
			ID: fmt.Sprintf("config_skill:%s:%03d", shortSystemPromptSHA(systemPromptSHA(name)), skillOccurrences[name]), Source: "configuration Skill",
			Title: "/" + name, Purpose: "provide run-scoped configuration schema and workflow guidance",
			Content: skillContent.String(), Prefix: prefix, Suffix: "\n", Overflow: SystemPromptOverflowTruncate,
		})
	}
	return composeProtectedSystemInstruction(cfg, config.AgentKindConfigManager, "config_manager", workspace, builtIn)
}

func BuildConfigManagerInstructionComposition(cfg *config.Config, state *book.State, resourceSkills ...ConfigManagerResourceSkill) SystemPromptComposition {
	composition, err := ComposeConfigManagerInstruction(cfg, state, resourceSkills...)
	if err != nil {
		workspace := ""
		if cfg != nil {
			workspace = cfg.Workspace
		}
		return failedSystemPromptComposition("config_manager", config.AgentKindConfigManager, workspace, err)
	}
	return composition
}

// ComposeImageInstruction assembles image runtime rules and the caller's image
// prompt as independently admitted sources. Project instructions are context.
func ComposeImageInstruction(cfg *config.Config, state *book.State, systemPrompt string) (SystemPromptComposition, error) {
	base, workspace := buildImageBuiltinInstruction(cfg, state, "")
	builtIn := []SystemPromptFragment{{
		ID: "builtin_base", Source: "Denova built-in", Title: "Image Agent base rules",
		Purpose: "define the built-in image generation workflow", Content: base,
		Required: true, Overflow: SystemPromptOverflowReject,
	}, {
		ID: "image_call_prompt", Source: "image generation caller", Title: "Call-site system prompt",
		Purpose: "constrain the current image generation request", Content: systemPrompt,
		Prefix: "\n\n## Call-site System Prompt\n\n", Overflow: SystemPromptOverflowTruncate,
	}}
	return composeProtectedSystemInstruction(cfg, config.AgentKindImage, "image", workspace, builtIn)
}

func BuildImageInstructionComposition(cfg *config.Config, state *book.State, systemPrompt string) SystemPromptComposition {
	composition, err := ComposeImageInstruction(cfg, state, systemPrompt)
	if err != nil {
		workspace := ""
		if cfg != nil {
			workspace = cfg.Workspace
		}
		return failedSystemPromptComposition("image", config.AgentKindImage, workspace, err)
	}
	return composition
}

func BuildImageInstruction(cfg *config.Config, state *book.State, systemPrompt string) string {
	return BuildImageInstructionComposition(cfg, state, systemPrompt).Instruction()
}

func tellerSystemPromptFragment(id, title, tellerID, name, description, content string) SystemPromptFragment {
	if strings.TrimSpace(tellerID) == "" && strings.TrimSpace(name) == "" && strings.TrimSpace(description) == "" && strings.TrimSpace(content) == "" {
		return SystemPromptFragment{
			ID: id, Source: "story teller configuration", Title: title,
			Purpose: "apply the selected story teller behavior and style", Overflow: SystemPromptOverflowTruncate,
		}
	}
	var visible strings.Builder
	visible.WriteString("# " + title + "\n\n")
	if value := strings.TrimSpace(tellerID); value != "" {
		visible.WriteString("Director ID: " + value + "\n")
	}
	if value := strings.TrimSpace(name); value != "" {
		visible.WriteString("Director name: " + value + "\n")
	}
	if value := strings.TrimSpace(description); value != "" {
		visible.WriteString("Director description: " + value + "\n")
	}
	visible.WriteString("\n")
	visible.WriteString(strings.TrimSpace(content))
	return SystemPromptFragment{
		ID: id, Source: "story teller configuration", Title: title,
		Purpose: "apply the selected story teller behavior and style", Content: visible.String(),
		Suffix:   "\n\n---\n\n",
		Overflow: SystemPromptOverflowTruncate,
	}
}

func styleRuleSystemPromptFragments(rules []StyleRule) []SystemPromptFragment {
	entries := make([]SystemPromptFragment, 0, len(rules))
	ordinal := 0
	occurrences := make(map[string]int, len(rules))
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if !rule.Global && scene == "" {
			continue
		}
		if len(rule.StyleReferences) == 0 && len(rule.StyleContents) == 0 {
			continue
		}
		ordinal++
		content := strings.TrimSpace(StyleRuleEntryInstruction(rule, ordinal))
		if content == "" {
			continue
		}
		title := "Prose style reference: global"
		identity := "global"
		if !rule.Global {
			title = "Prose style reference: " + scene
			identity = "scene:" + scene
		}
		occurrences[identity]++
		entries = append(entries, SystemPromptFragment{
			ID: fmt.Sprintf("style_rule:%s:%03d", shortSystemPromptSHA(systemPromptSHA(identity)), occurrences[identity]), Source: "current narrative style", Title: title,
			Purpose: "provide optional prose style references for the selected scene",
			Content: content, Suffix: "\n", Overflow: SystemPromptOverflowTruncate,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	fragments := make([]SystemPromptFragment, 0, len(entries)+2)
	fragments = append(fragments, SystemPromptFragment{
		ID: "style_protocol_header", Source: "Denova built-in", Title: "Prose style reference protocol",
		Purpose: "describe the provenance and shape of the following style entries",
		Content: StyleRulesProtocolHeader(), Required: true, Overflow: SystemPromptOverflowReject,
	})
	fragments = append(fragments, entries...)
	fragments = append(fragments, SystemPromptFragment{
		ID: "style_protocol_footer", Source: "Denova built-in", Title: "Prose style reference trigger rules",
		Purpose: "define when and how style references may influence generated prose",
		Content: StyleRulesProtocolFooter(), Suffix: "\n\n---\n\n", Required: true, Overflow: SystemPromptOverflowReject,
	})
	return fragments
}

func workspaceForPrompt(cfg *config.Config, state *book.State) string {
	if cfg != nil && strings.TrimSpace(cfg.Workspace) != "" {
		return strings.TrimSpace(cfg.Workspace)
	}
	if state != nil {
		return strings.TrimSpace(state.Workspace())
	}
	return ""
}

func (c SystemPromptComposition) LogAdmission(taskID, sessionID string) {
	if c.isZero() {
		return
	}
	if c.assemblyErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-prompt] system composition failed mode=%s workspace=%s task_id=%s session_id=%s err=%v", c.mode, c.workspace, strings.TrimSpace(taskID), strings.TrimSpace(sessionID), c.assemblyErr))
		return
	}
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[agent-prompt] system composition mode=%s workspace=%s task_id=%s session_id=%s bytes=%d sha=%s sources=%s",
		c.mode, c.workspace, strings.TrimSpace(taskID), strings.TrimSpace(sessionID), c.injectedBytes, c.instructionHash, systemPromptManifestSummary(c.manifest),
	))
}

func systemPromptManifestSummary(manifest []SystemPromptManifestEntry) string {
	parts := make([]string, 0, len(manifest))
	for _, entry := range manifest {
		parts = append(parts, fmt.Sprintf(
			"id=%q,source=%q,title=%q,bytes=%d/%d,sha=%s,included=%t,truncated=%t,rejected=%t,reason=%q",
			entry.ID, entry.Source, entry.Title, entry.IncludedBytes, entry.OriginalBytes, shortSystemPromptSHA(entry.IncludedSHA),
			entry.Included, entry.Truncated, entry.Rejected, entry.Reason,
		))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(parts), strings.Join(parts, "; "))
}

func shortSystemPromptSHA(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
