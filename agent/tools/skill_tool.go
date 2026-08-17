package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type SkillQuery struct {
	Query string
	Limit int
}

type SkillRef struct {
	Source string `json:"source" jsonschema:"minLength=1,maxLength=1024" jsonschema_description:"Exact source identifier returned by list."`
	ID     string `json:"id" jsonschema:"minLength=1,maxLength=4096" jsonschema_description:"Exact Skill identifier returned by list."`
}

type Skill struct {
	Ref         SkillRef `json:"ref"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
}

type SkillContent struct {
	Skill
	Revision     string `json:"revision,omitempty"`
	Instructions string `json:"instructions"`
}

type SkillSource interface {
	Identity() agent.CapabilityIdentity
	List(context.Context, SkillQuery) ([]Skill, error)
	Read(context.Context, SkillRef) (SkillContent, error)
}

type skillToolInput struct {
	Action string     `json:"action" jsonschema:"enum=list,enum=read" jsonschema_description:"List available skills or read one or more exact skill references."`
	Query  string     `json:"query,omitempty" jsonschema:"maxLength=4096"`
	Limit  int        `json:"limit,omitempty" jsonschema:"minimum=1,maximum=1000"`
	Refs   []SkillRef `json:"refs,omitempty" jsonschema:"maxItems=64" jsonschema_description:"Exact references to read. Every item produces its own success or error result."`
}

type skillListInput struct {
	Action string `json:"action" jsonschema:"required,enum=list" jsonschema_description:"List configured Skills."`
	Query  string `json:"query,omitempty" jsonschema:"maxLength=4096" jsonschema_description:"Optional name or description query; omit to browse."`
	Limit  int    `json:"limit,omitempty" jsonschema:"minimum=1,maximum=1000" jsonschema_description:"Maximum results; omit to use the source default."`
}

type skillReadInput struct {
	Action string     `json:"action" jsonschema:"required,enum=read" jsonschema_description:"Read exact Skill references."`
	Refs   []SkillRef `json:"refs" jsonschema:"required,minItems=1,maxItems=64" jsonschema_description:"Exact references from list; every item returns its own success or error."`
}

type skillReadResult struct {
	Ref     SkillRef      `json:"ref"`
	Content *SkillContent `json:"content,omitempty"`
	Error   string        `json:"error,omitempty"`
}

// Skills exposes explicit list/read operations over a host-provided source.
// Batch reads return per-item outcomes so one bad reference does not discard
// successful high-cost reads.
func Skills(source SkillSource) agent.Toolset {
	return defineToolset(func(context.Context) (agent.Toolset, error) {
		return buildSkills(source)
	})
}

func buildSkills(source SkillSource) (agent.Toolset, error) {
	if source == nil {
		return nil, errors.New("skills Toolset requires a SkillSource")
	}
	invoke := func(ctx context.Context, input skillToolInput) (agent.ToolResult, error) {
		switch strings.TrimSpace(input.Action) {
		case "list":
			items, err := source.List(ctx, SkillQuery{Query: input.Query, Limit: input.Limit})
			if err != nil {
				return agent.ToolResult{}, err
			}
			return JSONResult(struct {
				Skills []Skill `json:"skills"`
			}{Skills: items})
		case "read":
			if len(input.Refs) == 0 {
				return agent.ToolResult{}, errors.New("skill read requires at least one ref")
			}
			results := make([]skillReadResult, len(input.Refs))
			for index, ref := range input.Refs {
				results[index].Ref = ref
				content, readErr := source.Read(ctx, ref)
				if readErr != nil {
					results[index].Error = readErr.Error()
					continue
				}
				results[index].Content = &content
			}
			return JSONResult(struct {
				Results []skillReadResult `json:"results"`
			}{Results: results})
		default:
			return agent.ToolResult{}, fmt.Errorf("unsupported skill action %q", input.Action)
		}
	}
	tool, err := newUnionTool(
		"skill", "List configured Skills or read exact references. Batch reads return one outcome per reference.", invoke,
		toolSchemaFor[skillListInput](), toolSchemaFor[skillReadInput](),
	)
	if err != nil {
		return nil, err
	}
	identity := source.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return nil, errors.New("skills SkillSource requires a stable Identity")
	}
	definition := agent.ToolDefinition{Tool: tool, Descriptor: readDescriptor()}
	return agent.StaticToolsIdentified(toolsetIdentity("tools.skills", identity), definition)
}

// JSONResult constructs a successful bounded structured Tool result.
func JSONResult(value any) (agent.ToolResult, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode tool result: %w", err)
	}
	return agent.TextToolResult(string(encoded)), nil
}
