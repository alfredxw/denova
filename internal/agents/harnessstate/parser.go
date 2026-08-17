package harnessstate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
	agentstate "github.com/alfredxw/denova/agent/state"
	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type contextFrontmatter struct {
	ID        string   `yaml:"id"`
	Purpose   string   `yaml:"purpose"`
	Agents    []string `yaml:"agents"`
	Placement string   `yaml:"placement"`
	Enabled   *bool    `yaml:"enabled"`
}

type subAgentFrontmatter struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Enabled      *bool    `yaml:"enabled"`
	Parents      []string `yaml:"parents"`
	ModelProfile string   `yaml:"model_profile"`
	Tools        []string `yaml:"tools"`
}

type toolDescriptionFile struct {
	Tools map[string]struct {
		Description string `toml:"description"`
	} `toml:"tools"`
}

func parseAll(ctx context.Context, snapshot agentstate.Snapshot, cfg *config.Config) (Harness, []agentstate.Diagnostic) {
	harness := Harness{
		prompts: make(map[string]string), toolDescriptions: make(map[string]string),
	}
	var diagnostics []agentstate.Diagnostic
	knownKinds := make(map[string]bool)
	for _, kind := range sortedAgentKinds() {
		knownKinds[kind] = true
	}
	knownTools := knownToolNames()
	scriptTargets := make(map[string]map[string]bool)
	engine, engineErr := scriptEngine()
	if engineErr != nil {
		diagnostics = appendDiagnostic(diagnostics, "script_engine_invalid", "tools", engineErr.Error())
	}
	for _, file := range snapshot.Files() {
		if path.Dir(file.Path) != "tools" || path.Ext(file.Path) != ".js" {
			continue
		}
		if !utf8.Valid(file.Content) {
			continue
		}
		tool, fileDiagnostics := parseScriptTool(ctx, file.Path, file.Content, engine)
		diagnostics = append(diagnostics, fileDiagnostics...)
		if tool == nil {
			continue
		}
		if knownTools[tool.name] {
			diagnostics = appendDiagnostic(diagnostics, "script_tool_name_conflict", file.Path, fmt.Sprintf("Script Tool %q conflicts with a registered tool", tool.name))
			continue
		}
		if _, exists := scriptTargets[tool.name]; exists {
			diagnostics = appendDiagnostic(diagnostics, "script_tool_name_conflict", file.Path, fmt.Sprintf("Script Tool %q is defined more than once", tool.name))
			continue
		}
		if len(fileDiagnostics) != 0 {
			continue
		}
		if err := validateScriptToolDefinition(ctx, cfg, engine, *tool); err != nil {
			diagnostics = appendDiagnostic(diagnostics, "script_schema_invalid", file.Path, err.Error())
			continue
		}
		targets := make(map[string]bool, len(tool.agents))
		for _, kind := range tool.agents {
			targets[kind] = true
		}
		scriptTargets[tool.name] = targets
		harness.scriptTools = append(harness.scriptTools, *tool)
	}

	for _, file := range snapshot.Files() {
		if !utf8.Valid(file.Content) {
			diagnostics = appendDiagnostic(diagnostics, "invalid_utf8", file.Path, "State files must contain valid UTF-8")
			continue
		}
		switch {
		case file.Path == toolsFilePath:
			parsed, fileDiagnostics := parseToolDescriptions(file.Path, file.Content, knownTools)
			diagnostics = append(diagnostics, fileDiagnostics...)
			for name, description := range parsed {
				harness.toolDescriptions[name] = description
			}
		case path.Dir(file.Path) == "prompts" && path.Ext(file.Path) == ".md":
			kind := strings.TrimSuffix(path.Base(file.Path), ".md")
			if !knownKinds[kind] {
				diagnostics = appendDiagnostic(diagnostics, "unknown_agent_kind", file.Path, fmt.Sprintf("prompt targets unknown Agent kind %q", kind))
				continue
			}
			content := strings.TrimSpace(string(file.Content))
			if content == "" {
				diagnostics = appendDiagnostic(diagnostics, "empty_prompt", file.Path, "prompt content must not be empty")
				continue
			}
			harness.prompts[kind] = content
		case path.Dir(file.Path) == "context" && path.Ext(file.Path) == ".md":
			fragment, fileDiagnostics := parseContext(file.Path, file.Content, knownKinds)
			diagnostics = append(diagnostics, fileDiagnostics...)
			if len(fileDiagnostics) == 0 && fragment != nil {
				harness.contexts = append(harness.contexts, *fragment)
			}
		case path.Dir(file.Path) == "subagents" && path.Ext(file.Path) == ".md":
			subAgent, fileDiagnostics := parseSubAgent(file.Path, file.Content, cfg, knownKinds, scriptTargets)
			diagnostics = append(diagnostics, fileDiagnostics...)
			if len(fileDiagnostics) == 0 && subAgent != nil {
				harness.subAgents = append(harness.subAgents, *subAgent)
			}
		case path.Dir(file.Path) == "tools" && path.Ext(file.Path) == ".js":
			// Parsed in the first pass so subagent references are order independent.
		default:
			diagnostics = appendDiagnostic(diagnostics, "unsupported_path", file.Path, "State files must be prompts/*.md, context/*.md, subagents/*.md, tools/*.js, or tools.toml")
		}
	}

	sort.Slice(harness.contexts, func(i, j int) bool { return harness.contexts[i].Resource < harness.contexts[j].Resource })
	sort.Slice(harness.subAgents, func(i, j int) bool { return harness.subAgents[i].ID < harness.subAgents[j].ID })
	sort.Slice(harness.scriptTools, func(i, j int) bool { return harness.scriptTools[i].name < harness.scriptTools[j].name })
	diagnostics = append(diagnostics, validateBudgets(harness, cfg)...)
	return harness, diagnostics
}

func parseContext(filePath string, content []byte, knownKinds map[string]bool) (*ContextFragment, []agentstate.Diagnostic) {
	var metadata contextFrontmatter
	body, err := decodeMarkdown(filePath, content, &metadata)
	if err != nil {
		return nil, []agentstate.Diagnostic{{Code: "invalid_frontmatter", Path: filePath, Message: err.Error()}}
	}
	metadata.ID = strings.TrimSpace(metadata.ID)
	metadata.Purpose = strings.TrimSpace(metadata.Purpose)
	metadata.Placement = strings.TrimSpace(metadata.Placement)
	var diagnostics []agentstate.Diagnostic
	wantID := strings.TrimSuffix(path.Base(filePath), ".md")
	if metadata.ID == "" || metadata.ID != wantID || config.NormalizeSubAgentID(metadata.ID) != metadata.ID {
		diagnostics = appendDiagnostic(diagnostics, "invalid_context_id", filePath, fmt.Sprintf("context id must be the normalized filename %q", wantID))
	}
	if metadata.Purpose == "" {
		diagnostics = appendDiagnostic(diagnostics, "missing_context_purpose", filePath, "context purpose is required")
	}
	if len(metadata.Agents) == 0 {
		diagnostics = appendDiagnostic(diagnostics, "missing_context_agents", filePath, "context must target at least one Agent kind")
	}
	metadata.Agents = normalizedUnique(metadata.Agents)
	for _, kind := range metadata.Agents {
		if !knownKinds[kind] {
			diagnostics = appendDiagnostic(diagnostics, "unknown_agent_kind", filePath, fmt.Sprintf("context targets unknown Agent kind %q", kind))
		}
	}
	if metadata.Placement == "" {
		metadata.Placement = string(agent.ContextLeadingMessage)
	}
	if metadata.Placement != string(agent.ContextLeadingMessage) {
		diagnostics = appendDiagnostic(diagnostics, "unsupported_context_placement", filePath, "V1 Harness State context placement must be leading_message")
	}
	if strings.TrimSpace(body) == "" {
		diagnostics = appendDiagnostic(diagnostics, "empty_context", filePath, "context content must not be empty")
	}
	if metadata.Enabled != nil && !*metadata.Enabled {
		return nil, diagnostics
	}
	return &ContextFragment{
		ID: metadata.ID, Purpose: metadata.Purpose, Agents: metadata.Agents,
		Placement: agent.ContextPlacement(metadata.Placement), Content: strings.TrimSpace(body), Resource: filePath,
	}, diagnostics
}

func parseSubAgent(
	filePath string,
	content []byte,
	cfg *config.Config,
	knownKinds map[string]bool,
	scriptTargets map[string]map[string]bool,
) (*config.SubAgentConfig, []agentstate.Diagnostic) {
	var metadata subAgentFrontmatter
	body, err := decodeMarkdown(filePath, content, &metadata)
	if err != nil {
		return nil, []agentstate.Diagnostic{{Code: "invalid_frontmatter", Path: filePath, Message: err.Error()}}
	}
	metadata.ID = strings.TrimSpace(metadata.ID)
	metadata.Name = strings.TrimSpace(metadata.Name)
	metadata.Description = strings.TrimSpace(metadata.Description)
	metadata.ModelProfile = strings.TrimSpace(metadata.ModelProfile)
	var diagnostics []agentstate.Diagnostic
	wantID := strings.TrimSuffix(path.Base(filePath), ".md")
	if metadata.ID == "" || metadata.ID != wantID || config.NormalizeSubAgentID(metadata.ID) != metadata.ID {
		diagnostics = appendDiagnostic(diagnostics, "invalid_subagent_id", filePath, fmt.Sprintf("subagent id must be the normalized filename %q", wantID))
	}
	if metadata.Description == "" {
		diagnostics = appendDiagnostic(diagnostics, "missing_subagent_description", filePath, "subagent description is required")
	}
	if strings.TrimSpace(body) == "" {
		diagnostics = appendDiagnostic(diagnostics, "empty_subagent_prompt", filePath, "subagent system prompt must not be empty")
	}
	metadata.Parents = normalizedUnique(metadata.Parents)
	if len(metadata.Parents) == 0 {
		diagnostics = appendDiagnostic(diagnostics, "missing_subagent_parents", filePath, "subagent must target at least one parent Agent kind")
	}
	for _, parent := range metadata.Parents {
		if !knownKinds[parent] || !config.IsSubAgentParentKind(parent) {
			diagnostics = appendDiagnostic(diagnostics, "invalid_subagent_parent", filePath, fmt.Sprintf("Agent kind %q cannot own subagents", parent))
		}
	}
	if metadata.ModelProfile != "" && !config.ModelProfileExists(cfg, metadata.ModelProfile) {
		diagnostics = appendDiagnostic(diagnostics, "unknown_model_profile", filePath, fmt.Sprintf("model profile %q does not exist", metadata.ModelProfile))
	}

	toolOverride := make(config.AgentToolOverride, len(config.AgentToolCapabilities()))
	for _, capability := range config.AgentToolCapabilities() {
		toolOverride[capability.Source] = false
	}
	for _, capability := range normalizedUnique(metadata.Tools) {
		targets, scriptTool := scriptTargets[capability]
		if !scriptTool && !capabilityExists(capability) {
			diagnostics = appendDiagnostic(diagnostics, "unknown_tool_capability", filePath, fmt.Sprintf("tool capability %q does not exist", capability))
			continue
		}
		for _, parent := range metadata.Parents {
			if scriptTool && !targets[parent] {
				diagnostics = appendDiagnostic(diagnostics, "script_tool_exceeds_parent", filePath, fmt.Sprintf("Script Tool %q does not target parent Agent %q", capability, parent))
			} else if !scriptTool && !agentKindAllowsCapability(parent, capability) {
				diagnostics = appendDiagnostic(diagnostics, "tool_capability_exceeds_parent", filePath, fmt.Sprintf("tool capability %q is not available to parent Agent %q", capability, parent))
			}
		}
		toolOverride[capability] = true
	}
	if metadata.Enabled != nil && !*metadata.Enabled {
		return nil, diagnostics
	}
	name := metadata.Name
	if name == "" {
		name = metadata.ID
	}
	return &config.SubAgentConfig{
		ID: metadata.ID, Name: name, Description: metadata.Description,
		SystemPrompt: strings.TrimSpace(body), Enabled: metadata.Enabled, Parents: metadata.Parents,
		Model: config.AgentModelOverride{ProfileID: metadata.ModelProfile}, Tools: toolOverride,
	}, diagnostics
}

func parseToolDescriptions(filePath string, content []byte, knownTools map[string]bool) (map[string]string, []agentstate.Diagnostic) {
	var document toolDescriptionFile
	decoder := toml.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, []agentstate.Diagnostic{{Code: "invalid_tools_toml", Path: filePath, Message: err.Error()}}
	}
	result := make(map[string]string, len(document.Tools))
	var diagnostics []agentstate.Diagnostic
	for name, override := range document.Tools {
		name = strings.TrimSpace(name)
		description := strings.TrimSpace(override.Description)
		if !knownTools[name] {
			diagnostics = appendDiagnostic(diagnostics, "unknown_tool", filePath, fmt.Sprintf("tool %q is not registered", name))
			continue
		}
		if description == "" {
			diagnostics = appendDiagnostic(diagnostics, "empty_tool_description", filePath, fmt.Sprintf("tool %q description must not be empty", name))
			continue
		}
		result[name] = description
	}
	return result, diagnostics
}

func decodeMarkdown(filePath string, content []byte, target any) (string, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", fmt.Errorf("%s must start with YAML frontmatter", filePath)
	}
	rest := text[len("---\n"):]
	separator := strings.Index(rest, "\n---\n")
	if separator < 0 {
		return "", fmt.Errorf("%s has unterminated YAML frontmatter", filePath)
	}
	decoder := yaml.NewDecoder(strings.NewReader(rest[:separator]))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return "", fmt.Errorf("decode %s frontmatter: %w", filePath, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("decode %s frontmatter: multiple YAML documents are not allowed", filePath)
		}
		return "", fmt.Errorf("decode %s frontmatter: %w", filePath, err)
	}
	return rest[separator+len("\n---\n"):], nil
}

func validateBudgets(harness Harness, cfg *config.Config) []agentstate.Diagnostic {
	var diagnostics []agentstate.Diagnostic
	for _, kind := range sortedAgentKinds() {
		limits := config.ResolveAgentContext(cfg, kind)
		count, total := 0, 0
		if prompt := harness.prompts[kind]; prompt != "" {
			count++
			total += len(prompt)
			if len(prompt) > limits.MaxFragmentBytes {
				diagnostics = appendDiagnostic(diagnostics, "fragment_too_large", "prompts/"+kind+".md", fmt.Sprintf("prompt exceeds the %d-byte fragment limit", limits.MaxFragmentBytes))
			}
		}
		for _, fragment := range harness.contexts {
			if !contains(fragment.Agents, kind) {
				continue
			}
			count++
			total += len(fragment.Content)
			if len(fragment.Content) > limits.MaxFragmentBytes {
				diagnostics = appendDiagnostic(diagnostics, "fragment_too_large", fragment.Resource, fmt.Sprintf("context exceeds the %d-byte fragment limit for Agent %q", limits.MaxFragmentBytes, kind))
			}
			if len(fragment.Purpose) > limits.MaxMetadataFieldBytes {
				diagnostics = appendDiagnostic(diagnostics, "metadata_too_large", fragment.Resource, fmt.Sprintf("context purpose exceeds the %d-byte metadata limit for Agent %q", limits.MaxMetadataFieldBytes, kind))
			}
		}
		if count > limits.MaxFragments {
			diagnostics = appendDiagnostic(diagnostics, "too_many_fragments", "context", fmt.Sprintf("Agent %q has %d State fragments; limit is %d", kind, count, limits.MaxFragments))
		}
		if total > limits.MaxTotalInjectedBytes {
			diagnostics = appendDiagnostic(diagnostics, "injected_state_too_large", "context", fmt.Sprintf("Agent %q State injects %d bytes; limit is %d", kind, total, limits.MaxTotalInjectedBytes))
		}
	}
	for _, subAgent := range harness.subAgents {
		filePath := "subagents/" + subAgent.ID + ".md"
		for _, parent := range subAgent.Parents {
			limits := config.ResolveAgentContext(cfg, parent)
			if len(subAgent.SystemPrompt) > limits.MaxFragmentBytes {
				diagnostics = appendDiagnostic(diagnostics, "fragment_too_large", filePath, fmt.Sprintf("subagent prompt exceeds the %d-byte fragment limit for parent Agent %q", limits.MaxFragmentBytes, parent))
			}
			if len(subAgent.Name) > limits.MaxMetadataFieldBytes || len(subAgent.Description) > limits.MaxMetadataFieldBytes {
				diagnostics = appendDiagnostic(diagnostics, "metadata_too_large", filePath, fmt.Sprintf("subagent name or description exceeds the %d-byte metadata limit for parent Agent %q", limits.MaxMetadataFieldBytes, parent))
			}
		}
	}
	metadataLimit := config.MaxAgentContextMetadataFieldBytes
	for _, kind := range sortedAgentKinds() {
		if limit := config.ResolveAgentContext(cfg, kind).MaxMetadataFieldBytes; limit < metadataLimit {
			metadataLimit = limit
		}
	}
	for name, description := range harness.toolDescriptions {
		if len(description) > metadataLimit {
			diagnostics = appendDiagnostic(diagnostics, "metadata_too_large", toolsFilePath, fmt.Sprintf("tool %q description exceeds the %d-byte metadata limit", name, metadataLimit))
		}
	}
	return diagnostics
}

func knownToolNames() map[string]bool {
	result := make(map[string]bool)
	for _, goos := range []string{"linux", "windows"} {
		for _, entry := range config.AgentToolCapabilityCatalogForGOOS(goos) {
			for _, name := range entry.ToolNames {
				result[name] = true
			}
		}
	}
	return result
}

func capabilityExists(name string) bool {
	for _, capability := range config.AgentToolCapabilities() {
		if capability.Source == name {
			return true
		}
	}
	return false
}

func agentKindAllowsCapability(kind, capability string) bool {
	definition, ok := config.LookupAgentKind(kind)
	return ok && contains(definition.ToolCapabilities, capability)
}

func normalizedUnique(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func appendDiagnostic(diagnostics []agentstate.Diagnostic, code, filePath, message string) []agentstate.Diagnostic {
	return append(diagnostics, agentstate.Diagnostic{Code: code, Path: filePath, Message: message})
}
