package prompts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"denova/config"
	"denova/internal/book"
)

const (
	ideWorkspaceStableContextTitle  = "Stable Workspace Sources"
	ideWorkspaceDynamicContextTitle = "Workspace Sources for This Turn"
	ideContextMaxOpenFiles          = 20
	ideContextMaxPathRunes          = 240
)

// IDEContextRef carries bounded, model-visible IDE focus state for one turn.
// It contains paths only and never editor contents.
type IDEContextRef struct {
	CurrentFile string   `json:"current_file,omitempty"`
	OpenFiles   []string `json:"open_files,omitempty"`
}

type IDEWorkspaceRuntimeContexts struct {
	StableTitle  string
	Stable       string
	DynamicTitle string
	Dynamic      string
}

func IDEWorkspaceRuntimeContextsForState(state *book.State) IDEWorkspaceRuntimeContexts {
	contexts := IDEWorkspaceRuntimeContexts{
		StableTitle:  ideWorkspaceStableContextTitle,
		DynamicTitle: ideWorkspaceDynamicContextTitle,
	}
	if state == nil {
		return contexts
	}
	workspaceContext := state.WorkspaceContext()
	contexts.Stable = strings.TrimSpace(workspaceContext.Stable)
	contexts.Dynamic = strings.TrimSpace(workspaceContext.Dynamic)
	if contexts.Stable == "" && contexts.Dynamic == "" {
		contexts.Stable = EmptyIDEStateHint()
	}
	return contexts
}

// IDEWorkspaceRuntimeContextsForContext combines the durable workspace state
// with the bounded, request-scoped IDE focus paths. Keeping the request DTO out
// of this package makes prompt composition independent from chat transport.
func IDEWorkspaceRuntimeContextsForContext(state *book.State, ide IDEContextRef) IDEWorkspaceRuntimeContexts {
	contexts := IDEWorkspaceRuntimeContextsForState(state)
	ideContext := IDEContextRuntimeContext(ide)
	if strings.TrimSpace(ideContext) == "" {
		return contexts
	}
	extra := fmt.Sprintf(
		"## Current IDE State (provided by the frontend request; paths only; at most %d open files)\n\n%s",
		ideContextMaxOpenFiles,
		ideContext,
	)
	contexts.Dynamic = strings.TrimSpace(strings.Join(nonEmptyStrings(contexts.Dynamic, extra), "\n\n"))
	return contexts
}

func IDEContextRuntimeContext(ide IDEContextRef) string {
	currentFile := boundedIDEContextPath(ide.CurrentFile)
	openFiles := boundedIDEContextPaths(ide.OpenFiles, ideContextMaxOpenFiles)
	if currentFile == "" && len(openFiles) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Source: frontend IDE request state. This contains only paths for open and focused files, never file contents.\n")
	if currentFile != "" {
		sb.WriteString("- Focused file: ")
		sb.WriteString(currentFile)
		sb.WriteString("\n")
	} else {
		sb.WriteString("- Focused file: none\n")
	}
	if len(openFiles) > 0 {
		sb.WriteString("- Open files: ")
		sb.WriteString(strings.Join(openFiles, ", "))
		if len(ide.OpenFiles) > len(openFiles) {
			sb.WriteString(fmt.Sprintf(" (%d more omitted)", len(ide.OpenFiles)-len(openFiles)))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("- Constraint: use a tool to read any needed file explicitly by path. Do not assume this context contains current file contents.")
	return strings.TrimSpace(sb.String())
}

func boundedIDEContextPaths(paths []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	result := make([]string, 0, min(len(paths), limit))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = boundedIDEContextPath(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, path)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func boundedIDEContextPath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimLeft(path, "/")
	if path == "" || strings.Contains(path, ":") {
		return ""
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return ""
		}
	}
	runes := []rune(path)
	if len(runes) <= ideContextMaxPathRunes {
		return path
	}
	return string(runes[:ideContextMaxPathRunes]) + "[truncated]"
}

func BuildInteractiveStoryInstruction(cfg *config.Config, state *book.State, teller InteractiveStorySystemInstructionInput) string {
	return BuildInteractiveStoryInstructionComposition(cfg, state, teller).Instruction()
}

func buildImageBuiltinInstruction(cfg *config.Config, state *book.State, systemPrompt string) (string, string) {
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	if state != nil {
		if workspace == "" {
			workspace = state.Workspace()
		}
	}
	parts := []string{
		"You are Denova's general Image Agent. Convert bounded context supplied by the caller into an image-generation request.",
		"Understand purpose, source_context, the caller's system prompt, and loaded Skills before calling generate_image.",
		"Generate only images and image metadata. Do not modify story prose, chapter prose, lore, configuration, or other workspace content.",
		"Image prompts should clearly describe the subject, scene, composition, lighting, visual style, mood, and any text, watermark, or logo to avoid.",
		"If the caller requires a Skill, load the complete Skill with the skill tool before calling generate_image.",
	}
	if trimmed := strings.TrimSpace(systemPrompt); trimmed != "" {
		parts = append(parts, "## Caller System Prompt\n\n"+trimmed)
	}
	return strings.Join(parts, "\n\n"), workspace
}

const versionSummarySystemInstruction = "You are Denova's version-summary generator. Infer the core creative change in this save from the file changes. Output exactly one Chinese version summary of 10 to 30 Han characters. Do not include numbering, quotation marks, a colon, a final period, or any explanation."

// BuiltinAgentPrompts returns the default system prompts shown in the Agents
// settings page. The result is read-only display data; persisted overrides
// still live under config.Settings.AgentPrompts.
func BuiltinAgentPrompts(cfg *config.Config, state *book.State, ideTeller IDEStoryTeller) config.AgentPromptSettings {
	promptCfg := &config.Config{}
	if cfg != nil {
		copy := *cfg
		copy.AgentPrompts = config.AgentPromptSettings{}
		promptCfg = &copy
	}
	ide, ideErr := ComposeInstruction(promptCfg, state, ideTeller)
	general, generalErr := ComposeGeneralInstruction(promptCfg)
	interactiveStory, interactiveErr := ComposeInteractiveStoryInstruction(promptCfg, state, InteractiveStorySystemInstructionInput{})
	configManager, configErr := ComposeConfigManagerInstruction(promptCfg, state)
	director, directorErr := ComposeBuiltinSystemInstruction(promptCfg, config.AgentKindInteractiveDirector, "interactive_director", workspaceForPrompt(promptCfg, state), "builtin_base", "Background Director System Rules", "define the interactive director planning workflow", BuildInteractiveDirectorSystemInstruction())
	version, versionErr := ComposeBuiltinSystemInstruction(promptCfg, config.AgentKindVersionSummary, "version_summary", workspaceForPrompt(promptCfg, state), "builtin_base", "Version Summary Generation Rules", "define the version summary task and output constraint", versionSummarySystemInstruction)
	toolAgent, toolErr := ComposeBuiltinSystemInstruction(promptCfg, config.AgentKindToolAgent, "tool_agent", workspaceForPrompt(promptCfg, state), "builtin_base", "Chapter Split Regex Task", "define the structured chapter-regex inference task", ChapterSplitRegexSystemInstruction())
	image, imageErr := ComposeImageInstruction(promptCfg, state, "")
	return config.AgentPromptSettings{
		General:             config.AgentPromptOverride{SystemPrompt: systemPromptPreview(general, generalErr)},
		IDE:                 config.AgentPromptOverride{SystemPrompt: systemPromptPreview(ide, ideErr)},
		InteractiveStory:    config.AgentPromptOverride{SystemPrompt: systemPromptPreview(interactiveStory, interactiveErr)},
		ConfigManager:       config.AgentPromptOverride{SystemPrompt: systemPromptPreview(configManager, configErr)},
		InteractiveDirector: config.AgentPromptOverride{SystemPrompt: systemPromptPreview(director, directorErr)},
		VersionSummary:      config.AgentPromptOverride{SystemPrompt: systemPromptPreview(version, versionErr)},
		ToolAgent:           config.AgentPromptOverride{SystemPrompt: systemPromptPreview(toolAgent, toolErr)},
		Image:               config.AgentPromptOverride{SystemPrompt: systemPromptPreview(image, imageErr)},
	}
}

func systemPromptPreview(composition SystemPromptComposition, err error) string {
	if err != nil {
		return "System prompt admission failed: " + err.Error()
	}
	return composition.Instruction()
}

func BuiltinAgentPromptBlocks(cfg *config.Config, state *book.State, ideTeller IDEStoryTeller) config.AgentPromptBlockSettings {
	promptCfg := &config.Config{}
	if cfg != nil {
		copy := *cfg
		copy.AgentPrompts = config.AgentPromptSettings{}
		promptCfg = &copy
	}
	ideWorkspace := workspaceForPrompt(promptCfg, state)
	interactiveWorkspace := workspaceForPrompt(promptCfg, state)
	configManagerFlow := configManagerFlowInstruction(promptCfg, state)
	return config.AgentPromptBlockSettings{
		General:             builtinPromptBlocks(promptCfg, config.AgentKindGeneral, generalAgentFlowInstruction(promptCfg)),
		IDE:                 builtinPromptBlocks(promptCfg, config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace)),
		InteractiveStory:    builtinPromptBlocks(promptCfg, config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace)),
		ConfigManager:       builtinPromptBlocks(promptCfg, config.AgentKindConfigManager, configManagerFlow),
		InteractiveDirector: builtinPromptBlocks(promptCfg, config.AgentKindInteractiveDirector, BuildInteractiveDirectorSystemInstruction()),
		VersionSummary:      builtinPromptBlocks(promptCfg, config.AgentKindVersionSummary, versionSummarySystemInstruction),
		ToolAgent:           builtinPromptBlocks(promptCfg, config.AgentKindToolAgent, ChapterSplitRegexSystemInstruction()),
		Image:               builtinPromptBlocks(promptCfg, config.AgentKindImage, ""),
	}
}

func BuiltinAgentPromptSources(cfg *config.Config, state *book.State, ideTeller IDEStoryTeller) config.AgentPromptSourceSettings {
	promptCfg := &config.Config{}
	if cfg != nil {
		copy := *cfg
		copy.AgentPrompts = config.AgentPromptSettings{}
		promptCfg = &copy
	}
	ideWorkspace := workspaceForPrompt(promptCfg, state)
	interactiveWorkspace := workspaceForPrompt(promptCfg, state)
	configManagerFlow := configManagerFlowInstruction(promptCfg, state)
	return config.AgentPromptSourceSettings{
		General: builtinPromptSourceList(promptCfg, config.AgentKindGeneral, generalAgentFlowInstruction(promptCfg)),
		IDE: builtinPromptSourceList(promptCfg, config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace),
			readonlyPromptSource("teller", "Default IDE Storyteller Rules", ideTeller.ID, ideTeller.Prompt),
		),
		InteractiveStory:    builtinPromptSourceList(promptCfg, config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace)),
		ConfigManager:       builtinPromptSourceList(promptCfg, config.AgentKindConfigManager, configManagerFlow),
		InteractiveDirector: builtinPromptSourceList(promptCfg, config.AgentKindInteractiveDirector, BuildInteractiveDirectorSystemInstruction()),
		VersionSummary:      builtinPromptSourceList(promptCfg, config.AgentKindVersionSummary, versionSummarySystemInstruction),
		ToolAgent:           builtinPromptSourceList(promptCfg, config.AgentKindToolAgent, ChapterSplitRegexSystemInstruction()),
		Image:               builtinPromptSourceList(promptCfg, config.AgentKindImage, ""),
	}
}

func builtinPromptBlocks(cfg *config.Config, agentKind, flow string) config.AgentPromptBlocks {
	return config.AgentPromptBlocks{
		RuntimeContract:      runtimeContractForAgent(agentKind),
		OutputProtocol:       outputProtocolForAgent(agentKind),
		EditableSystemPrompt: editablePromptFlowForAgent(agentKind, flow),
	}
}

func builtinPromptSourceList(cfg *config.Config, agentKind, flow string, extraSources ...config.AgentPromptSource) config.AgentPromptSourceList {
	sources := make([]config.AgentPromptSource, 0, len(extraSources)+4)
	sources = append(sources, config.AgentPromptSource{
		ID:      "runtime_contract",
		Title:   "Runtime Contract",
		Source:  "Denova runtime",
		Content: runtimeContractForAgent(agentKind),
	})
	if outputProtocol := strings.TrimSpace(outputProtocolForAgent(agentKind)); outputProtocol != "" {
		sources = append(sources, config.AgentPromptSource{
			ID:      "output_protocol",
			Title:   "Output Protocol",
			Source:  "Denova runtime",
			Content: outputProtocol,
		})
	}
	for _, source := range extraSources {
		if strings.TrimSpace(source.Content) != "" {
			sources = append(sources, source)
		}
	}
	sources = append(sources, config.AgentPromptSource{
		ID:       "flow",
		Title:    "Workflow Rules",
		Source:   "Denova built-in",
		Content:  editablePromptFlowForAgent(agentKind, flow),
		Editable: true,
		Field:    "flow_prompt",
	})
	sources = append(sources, config.AgentPromptSource{
		ID:       "custom",
		Title:    "User Customization",
		Source:   "user/workspace config",
		Editable: true,
		Field:    "system_prompt",
	})
	return config.AgentPromptSourceList{Sources: sources}
}

func readonlyPromptSource(id, title, source, content string) config.AgentPromptSource {
	return config.AgentPromptSource{
		ID:      id,
		Title:   title,
		Source:  source,
		Content: strings.TrimSpace(content),
	}
}

func styleRulePromptSources(rules []StyleRule) []promptSource {
	sources := make([]promptSource, 0, len(rules))
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if !rule.Global && scene == "" {
			continue
		}
		if len(rule.StyleReferences) == 0 && len(rule.StyleContents) == 0 {
			continue
		}
		content := StyleRulesInstruction([]StyleRule{rule})
		if strings.TrimSpace(content) == "" {
			continue
		}
		title := "Prose Style Reference: Global"
		if !rule.Global {
			title = "Prose Style Reference: " + scene
		}
		sources = append(sources, promptSource{
			source:  "System prompt",
			title:   title,
			content: content,
			note:    "Current narrative style",
		})
	}
	return sources
}

func ideFlowInstruction(cfg *config.Config, workspace string) string {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return BuildIDEWritingFlowInstruction(SystemInstructionInput{
		Workspace:             workspace,
		ChapterFilenameFormat: cfg.ChapterFilenameFormat,
		VolumeDirFormat:       cfg.VolumeDirFormat,
		ChapterGroupMin:       cfg.ChapterGroupMin,
		ChapterGroupMax:       cfg.ChapterGroupMax,
	})
}

func generalAgentFlowInstruction(cfg *config.Config) string {
	composition, err := ComposeGeneralInstruction(cfg)
	if err != nil {
		return ""
	}
	for _, fragment := range composition.fragments {
		if fragment.ID == "builtin_base" {
			return fragment.Content
		}
	}
	return ""
}

func interactiveStoryFlowInstruction(cfg *config.Config, workspace string) string {
	return BuildInteractiveStoryFlowInstruction(InteractiveStorySystemInstructionInput{
		Workspace: workspace,
	})
}

func editablePromptFlowForAgent(agentKind, flow string) string {
	switch agentKind {
	case config.AgentKindInteractiveDirector:
		return ""
	case config.AgentKindVersionSummary:
		return ""
	case config.AgentKindToolAgent:
		return filterPromptLines(flow, "Output JSON only", "Do not return Markdown")
	default:
		return strings.TrimSpace(flow)
	}
}

func BuildConfigManagerInstruction(cfg *config.Config, state *book.State, resourceSkills ...ConfigManagerResourceSkill) string {
	return BuildConfigManagerInstructionComposition(cfg, state, resourceSkills...).Instruction()
}

func configManagerFlowInstruction(cfg *config.Config, state *book.State) string {
	return configManagerFlowInstructionFor(workspaceForPrompt(cfg, state))
}

func configManagerFlowInstructionFor(workspace string) string {
	var sb strings.Builder
	sb.WriteString("You are Denova's unified Configuration Manager Agent. Through embedded module entry points, help users manage lore, presets for narrative style, story direction, state systems, and images, as well as automations, Skills, and Agent configuration.\n\n")
	if strings.TrimSpace(workspace) != "" {
		sb.WriteString("Current work workspace: ")
		sb.WriteString(strings.TrimSpace(workspace))
		sb.WriteString("\n\n")
	}
	sb.WriteString(strings.Join([]string{
		"## Working Method",
		"- Configuration resources have two entry points: config_read handles describe/list/get, and config_apply handles one create/update/delete operation.",
		"- For an unfamiliar resource, call config_read with operation=describe, then follow the corresponding reference in the config-manager Skill.",
		"- Narrow reads with list before get by exact ID. update/delete must include the revision returned by the most recent config_read.",
		"- Each config_apply call changes one independent resource. Continue with the new revision in the receipt; never overwrite concurrent changes with an old revision.",
		"- Agent-page configuration uses resource=agent_profile and requires explicit scope=user or scope=workspace.",
		"- Do not change ports, themes, remote access, editor appearance, or other settings outside the Agent page.",
		"- Do not directly edit backing files for lore, presets, automations, Skills, or Agent configuration with file tools.",
		"- Deletion, hiding, overwriting, and broad rewrites require an explicit user instruction. Ask first when the instruction is absent.",
		"",
		"## Module Boundaries",
		"- Lore stores stable, long-lived canon. In Game Mode, established events belong to Turn history, while current location, injuries, relationships, resources, and rule values belong to Actor State and do not enter lore by default.",
		"- Narrative styles own prose style, prompt slots, scene styles, and context policy. Story directors own orchestration and combine narrative styles, event packages, TRPG checks, state systems, and image presets through module_refs. Event packages contain event cards. Each TRPG check resource represents a DM check style, always uses d20, and may bind state fields through state_bindings. The state system is the source of truth for structured state, opening traits, current time and location, current event, computed fields, and rule-consumed fields. Templates are state-table schemas and may represent story context, protagonists, important characters, enemies, worlds, countdowns, specific characters, factions, bases, instances, or other state objects. Image presets own visual style, medium, composition, constraints, and avoid lists.",
		"- The opening page configures state-schema policy per new story. In dynamic mode, the foreground Game Agent completes the schema draft before the first atomic turn submission; fixed mode changes values only. The Story Director neither owns nor changes the state schema.",
		"- resource=skill writes SKILL.md and must explain use cases, context acquisition, and a concrete workflow. A built-in preset Skill can be changed only through a same-name workspace override, never by writing the built-in Skills directory.",
		"- Automation tasks must keep trigger conditions, notification or execution policy, and write permissions explicit.",
	}, "\n"))
	return sb.String()
}

type promptSource struct {
	source  string
	title   string
	content string
	note    string
}

// PartSummary returns a bounded, content-safe diagnostic summary for one
// prompt or model-output fragment.
func PartSummary(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join([]string{
		"present=" + boolString(s != ""),
		"bytes=" + intString(len(s)),
		"chars=" + intString(utf8.RuneCountInString(s)),
		"lines=" + intString(promptLineCount(s)),
		"sha=" + shortSHA256(s),
		"preview=" + strconv.Quote(logPreview(s, 80)),
	}, ",")
}

func filterPromptLines(content string, blockedPrefixes ...string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		blocked := false
		for _, prefix := range blockedPrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				blocked = true
				break
			}
		}
		if !blocked {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func intString(v int) string {
	return strconv.Itoa(v)
}

func promptLineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func shortSHA256(s string) string {
	if s == "" {
		return "-"
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func logPreview(content string, limit int) string {
	content = strings.ReplaceAll(content, "\n", "\\n")
	content = strings.ReplaceAll(content, "\r", "\\r")
	if limit <= 0 || len(content) <= limit {
		return content
	}
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	return content[:limit] + "..."
}
