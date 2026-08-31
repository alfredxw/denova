package harnessstate

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"

	"denova/config"

	agentscript "github.com/alfredxw/denova/agent/script"
	agentstate "github.com/alfredxw/denova/agent/state"
	publictools "github.com/alfredxw/denova/agent/tools"
	"github.com/invopop/jsonschema"
	"gopkg.in/yaml.v3"
)

var scriptToolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

var scriptSchemaKeywords = map[string]bool{
	"allOf": true, "anyOf": true, "oneOf": true, "not": true,
	"if": true, "then": true, "else": true,
	"dependentSchemas": true, "dependentRequired": true,
	"prefixItems": true, "items": true, "contains": true,
	"properties": true, "patternProperties": true, "additionalProperties": true, "propertyNames": true,
	"type": true, "enum": true, "const": true,
	"multipleOf": true, "maximum": true, "exclusiveMaximum": true, "minimum": true, "exclusiveMinimum": true,
	"maxLength": true, "minLength": true, "pattern": true,
	"maxItems": true, "minItems": true, "uniqueItems": true, "maxContains": true, "minContains": true,
	"maxProperties": true, "minProperties": true, "required": true,
	"format": true, "contentEncoding": true, "contentMediaType": true, "contentSchema": true,
	"title": true, "description": true, "default": true, "deprecated": true, "readOnly": true, "writeOnly": true, "examples": true,
}

type scriptToolFrontmatter struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Agents      []string       `yaml:"agents"`
	Enabled     *bool          `yaml:"enabled"`
	InputSchema map[string]any `yaml:"input_schema"`
}

func scriptEngine() (*agentscript.Engine, error) {
	return agentscript.NewEngine(agentscript.Config{})
}

func validateScriptToolDefinition(
	ctx context.Context,
	cfg *config.Config,
	engine *agentscript.Engine,
	tool ScriptTool,
) error {
	maxResultBytes := config.DefaultAgentToolResultLimitKB * 1024
	var timeout time.Duration
	if cfg != nil {
		if cfg.AgentToolResultLimitKB > 0 {
			maxResultBytes = cfg.AgentToolResultLimitKB * 1024
		}
		if cfg.AgentScriptTimeoutSeconds > 0 {
			timeout = time.Duration(cfg.AgentScriptTimeoutSeconds) * time.Second
		}
	}
	definition, err := publictools.SavedScriptTool(publictools.ScriptConfig{
		Engine: engine, MaxResultBytes: maxResultBytes, Timeout: timeout,
	}, publictools.SavedScriptToolSpec{
		Name: tool.name, Description: tool.description,
		InputSchema: tool.inputSchema, Program: tool.program,
	})
	if err != nil {
		return err
	}
	return definition.Validate(ctx)
}

func parseScriptTool(
	ctx context.Context,
	filePath string,
	content []byte,
	engine *agentscript.Engine,
) (*ScriptTool, []agentstate.Diagnostic) {
	metadata, body, paddedBody, err := decodeScriptTool(filePath, content)
	if err != nil {
		return nil, []agentstate.Diagnostic{{Code: "invalid_script_frontmatter", Path: filePath, Message: err.Error()}}
	}
	var diagnostics []agentstate.Diagnostic
	metadata.Name = strings.TrimSpace(metadata.Name)
	metadata.Description = strings.TrimSpace(metadata.Description)
	wantName := strings.TrimSuffix(path.Base(filePath), ".js")
	if metadata.Name != wantName || !scriptToolNamePattern.MatchString(metadata.Name) {
		diagnostics = appendDiagnostic(diagnostics, "script_tool_name_mismatch", filePath, fmt.Sprintf("Script Tool name must be the normalized filename %q", wantName))
	}
	if metadata.Description == "" {
		diagnostics = appendDiagnostic(diagnostics, "script_description_missing", filePath, "Script Tool description is required")
	} else if containsHan(metadata.Description) {
		diagnostics = appendDiagnostic(diagnostics, "script_description_not_english", filePath, "Script Tool description must be written in English")
	}
	metadata.Agents = normalizedUnique(metadata.Agents)
	if len(metadata.Agents) == 0 {
		diagnostics = appendDiagnostic(diagnostics, "script_agents_missing", filePath, "Script Tool must target at least one Agent")
	}
	for _, kind := range metadata.Agents {
		if !validScriptToolTarget(kind) {
			diagnostics = appendDiagnostic(diagnostics, "script_agent_invalid", filePath, fmt.Sprintf("Script Tool targets unsupported Agent or custom Agent ID %q", kind))
		}
	}
	if strings.TrimSpace(body) == "" {
		diagnostics = appendDiagnostic(diagnostics, "script_body_empty", filePath, "Script Tool body must not be empty")
	}

	schema, schemaDiagnostics := parseScriptSchema(filePath, metadata.InputSchema)
	diagnostics = append(diagnostics, schemaDiagnostics...)
	if engine == nil {
		diagnostics = appendDiagnostic(diagnostics, "script_engine_unavailable", filePath, "Script engine is unavailable")
		return nil, diagnostics
	}
	program, compileDiagnostics := engine.Compile(ctx, agentscript.Source{Name: filePath, Code: paddedBody})
	for _, diagnostic := range compileDiagnostics {
		diagnostics = append(diagnostics, agentstate.Diagnostic{
			Code: diagnostic.Kind, Path: filePath, Line: diagnostic.Line, Column: diagnostic.Column, Message: diagnostic.Message,
		})
	}
	enabled := metadata.Enabled == nil || *metadata.Enabled
	tool := &ScriptTool{
		name: metadata.Name, description: metadata.Description, agents: metadata.Agents,
		enabled: enabled, resource: filePath, inputSchema: schema, program: program,
	}
	return tool, diagnostics
}

// Script Tool audiences may name one supported fixed runtime or an exact
// custom Agent ID. Custom IDs are syntax-validated but not cross-referenced so
// archiving or deleting one Agent cannot invalidate the complete Harness.
func validScriptToolTarget(target string) bool {
	target = strings.TrimSpace(target)
	switch target {
	case config.AgentKindGeneral, config.AgentKindIDE, config.AgentKindInteractiveStory:
		return true
	}
	if config.IsReservedAgentID(target) {
		return false
	}
	return target != "" && config.NormalizeCustomAgentID(target) == target
}

func decodeScriptTool(filePath string, content []byte) (scriptToolFrontmatter, string, string, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return scriptToolFrontmatter{}, "", "", fmt.Errorf("%s must start with YAML frontmatter", filePath)
	}
	rest := text[len("---\n"):]
	separator := strings.Index(rest, "\n---\n")
	if separator < 0 {
		return scriptToolFrontmatter{}, "", "", fmt.Errorf("%s has unterminated YAML frontmatter", filePath)
	}
	frontmatter := rest[:separator]
	bodyOffset := len("---\n") + separator + len("\n---\n")
	var metadata scriptToolFrontmatter
	decoder := yaml.NewDecoder(strings.NewReader(frontmatter))
	decoder.KnownFields(true)
	if err := decoder.Decode(&metadata); err != nil {
		return scriptToolFrontmatter{}, "", "", fmt.Errorf("decode %s frontmatter: %w", filePath, err)
	}
	body := text[bodyOffset:]
	padded := strings.Repeat("\n", strings.Count(text[:bodyOffset], "\n")) + body
	return metadata, body, padded, nil
}

func parseScriptSchema(filePath string, raw map[string]any) (*jsonschema.Schema, []agentstate.Diagnostic) {
	if raw == nil {
		return nil, []agentstate.Diagnostic{{Code: "script_schema_missing", Path: filePath, Message: "Script Tool input_schema is required"}}
	}
	var diagnostics []agentstate.Diagnostic
	validateScriptSchemaMap(raw, "input_schema", filePath, &diagnostics)
	if value, _ := raw["type"].(string); value != "object" {
		diagnostics = appendDiagnostic(diagnostics, "script_schema_invalid", filePath, "Script Tool input_schema must have an object root")
	}
	if _, exists := raw["additionalProperties"]; !exists {
		raw["additionalProperties"] = false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, appendDiagnostic(diagnostics, "script_schema_invalid", filePath, fmt.Sprintf("encode Script Tool input_schema: %v", err))
	}
	schema := &jsonschema.Schema{}
	if err := json.Unmarshal(encoded, schema); err != nil {
		return nil, appendDiagnostic(diagnostics, "script_schema_invalid", filePath, fmt.Sprintf("decode Script Tool input_schema: %v", err))
	}
	return schema, diagnostics
}

func validateScriptSchemaMap(value map[string]any, fieldPath, filePath string, diagnostics *[]agentstate.Diagnostic) {
	for keyword, raw := range value {
		if strings.HasPrefix(keyword, "$") || !scriptSchemaKeywords[keyword] {
			*diagnostics = appendDiagnostic(*diagnostics, "script_schema_unknown_keyword", filePath, fmt.Sprintf("%s contains unsupported keyword %q", fieldPath, keyword))
			continue
		}
		if keyword == "description" {
			if description, ok := raw.(string); ok && containsHan(description) {
				*diagnostics = appendDiagnostic(*diagnostics, "script_description_not_english", filePath, fmt.Sprintf("%s.description must be written in English", fieldPath))
			}
		}
		switch keyword {
		case "properties", "patternProperties", "dependentSchemas":
			children, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			for name, child := range children {
				if schema, ok := child.(map[string]any); ok {
					validateScriptSchemaMap(schema, fieldPath+"."+keyword+"."+name, filePath, diagnostics)
				}
			}
		case "items", "contains", "additionalProperties", "propertyNames", "not", "if", "then", "else", "contentSchema":
			if child, ok := raw.(map[string]any); ok {
				validateScriptSchemaMap(child, fieldPath+"."+keyword, filePath, diagnostics)
			}
		case "allOf", "anyOf", "oneOf", "prefixItems":
			children, ok := raw.([]any)
			if !ok {
				continue
			}
			for index, child := range children {
				if schema, ok := child.(map[string]any); ok {
					validateScriptSchemaMap(schema, fmt.Sprintf("%s.%s[%d]", fieldPath, keyword, index), filePath, diagnostics)
				}
			}
		}
	}
}

func containsHan(value string) bool {
	for _, character := range value {
		if unicode.Is(unicode.Han, character) {
			return true
		}
	}
	return false
}
