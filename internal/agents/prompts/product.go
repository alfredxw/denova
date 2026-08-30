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
	imageprompting "denova/internal/image/prompting"
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
		"Author one complete final prompt for the selected image model. Use natural language only when the selected model's prompt guide does not require another syntax.",
		"If the caller requires a Skill, load the complete Skill with the skill tool before calling generate_image.",
	}
	if guide := imageprompting.SelectedGuide(cfg); guide != "" {
		parts = append(parts, guide)
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
	version, versionErr := ComposeBuiltinSystemInstruction(promptCfg, config.AgentKindVersionSummary, "version_summary", workspaceForPrompt(promptCfg, state), "builtin_base", "Version Summary Generation Rules", "define the version summary task and output constraint", versionSummarySystemInstruction)
	toolAgent, toolErr := ComposeBuiltinSystemInstruction(promptCfg, config.AgentKindToolAgent, "tool_agent", workspaceForPrompt(promptCfg, state), "builtin_base", "Chapter Split Regex Task", "define the structured chapter-regex inference task", ChapterSplitRegexSystemInstruction())
	image, imageErr := ComposeImageInstruction(promptCfg, state, "")
	return config.AgentPromptSettings{
		General:          config.AgentPromptOverride{SystemPrompt: systemPromptPreview(general, generalErr)},
		IDE:              config.AgentPromptOverride{SystemPrompt: systemPromptPreview(ide, ideErr)},
		InteractiveStory: config.AgentPromptOverride{SystemPrompt: systemPromptPreview(interactiveStory, interactiveErr)},
		VersionSummary:   config.AgentPromptOverride{SystemPrompt: systemPromptPreview(version, versionErr)},
		ToolAgent:        config.AgentPromptOverride{SystemPrompt: systemPromptPreview(toolAgent, toolErr)},
		Image:            config.AgentPromptOverride{SystemPrompt: systemPromptPreview(image, imageErr)},
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
	return config.AgentPromptBlockSettings{
		General:          builtinPromptBlocks(promptCfg, config.AgentKindGeneral, generalAgentFlowInstruction(promptCfg)),
		IDE:              builtinPromptBlocks(promptCfg, config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace)),
		InteractiveStory: builtinPromptBlocks(promptCfg, config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace)),
		VersionSummary:   builtinPromptBlocks(promptCfg, config.AgentKindVersionSummary, versionSummarySystemInstruction),
		ToolAgent:        builtinPromptBlocks(promptCfg, config.AgentKindToolAgent, ChapterSplitRegexSystemInstruction()),
		Image:            builtinPromptBlocks(promptCfg, config.AgentKindImage, ""),
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
	return config.AgentPromptSourceSettings{
		General: builtinPromptSourceList(promptCfg, config.AgentKindGeneral, generalAgentFlowInstruction(promptCfg)),
		IDE: builtinPromptSourceList(promptCfg, config.AgentKindIDE, ideFlowInstruction(promptCfg, ideWorkspace),
			readonlyPromptSource("teller", "Default IDE Storyteller Rules", ideTeller.ID, ideTeller.Prompt),
		),
		InteractiveStory: builtinPromptSourceList(promptCfg, config.AgentKindInteractiveStory, interactiveStoryFlowInstruction(promptCfg, interactiveWorkspace)),
		VersionSummary:   builtinPromptSourceList(promptCfg, config.AgentKindVersionSummary, versionSummarySystemInstruction),
		ToolAgent:        builtinPromptSourceList(promptCfg, config.AgentKindToolAgent, ChapterSplitRegexSystemInstruction()),
		Image:            builtinPromptSourceList(promptCfg, config.AgentKindImage, ""),
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
	case config.AgentKindVersionSummary:
		return ""
	case config.AgentKindToolAgent:
		return filterPromptLines(flow, "Output JSON only", "Do not return Markdown")
	default:
		return strings.TrimSpace(flow)
	}
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
