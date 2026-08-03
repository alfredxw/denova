package googlegenerativeai

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func (model *ChatModel) request(input []*agent.Message, opts ...agent.ModelOption) ([]*genai.Content, *genai.GenerateContentConfig, error) {
	system, contents, err := requestContents(input, model.config)
	if err != nil {
		return nil, nil, err
	}
	common := agent.GetCommonOptions(model.options, opts...)
	tools := common.Tools
	if common.ToolChoice != nil && len(common.AllowedToolNames) != 0 {
		tools = filterTools(tools, common.AllowedToolNames)
	}
	requestTools, err := requestTools(tools)
	if err != nil {
		return nil, nil, err
	}
	toolConfig, err := requestToolConfig(common.ToolChoice, common.AllowedToolNames, len(requestTools))
	if err != nil {
		return nil, nil, err
	}
	config := &genai.GenerateContentConfig{
		SystemInstruction: system,
		Tools:             requestTools,
		ToolConfig:        toolConfig,
	}
	if model.config.Temperature != nil {
		value := *model.config.Temperature
		config.Temperature = &value
	}
	maxTokens := model.config.MaxOutputTokens
	if common.MaxTokens != nil {
		maxTokens = common.MaxTokens
	}
	if maxTokens != nil {
		config.MaxOutputTokens = int32(*maxTokens)
	}
	applyThinking(config, model.compatibility, model.config.ThinkingLevel)
	if err := applyOutputFormat(config, model.config.OutputFormat); err != nil {
		return nil, nil, err
	}
	if len(model.compatibility.ExtraBody) != 0 {
		config.HTTPOptions = &genai.HTTPOptions{ExtraBody: model.compatibility.ExtraBody}
	}
	return contents, config, nil
}

func requestContents(input []*agent.Message, config providers.ModelConfig) (*genai.Content, []*genai.Content, error) {
	systemParts := make([]*genai.Part, 0)
	contents := make([]*genai.Content, 0, len(input))
	toolNames := make(map[string]string)
	for index, message := range input {
		if message == nil {
			return nil, nil, fmt.Errorf("google generative AI input %d: nil message", index)
		}
		switch message.Role {
		case agent.System:
			systemParts = append(systemParts, genai.NewPartFromText(message.Content))
		case agent.User:
			contents = append(contents, genai.NewContentFromParts([]*genai.Part{genai.NewPartFromText(message.Content)}, genai.RoleUser))
		case agent.Assistant:
			parts, err := assistantParts(message, config)
			if err != nil {
				return nil, nil, fmt.Errorf("google generative AI input %d: %w", index, err)
			}
			for _, call := range message.ToolCalls {
				toolNames[call.ID] = call.Function.Name
			}
			contents = append(contents, genai.NewContentFromParts(parts, genai.RoleModel))
		case agent.ToolRole:
			name := strings.TrimSpace(message.ToolName)
			if name == "" {
				name = toolNames[message.ToolCallID]
			}
			if name == "" {
				return nil, nil, fmt.Errorf("google generative AI input %d: tool result requires tool name", index)
			}
			response := map[string]any{"output": message.Content}
			if message.ToolResult != nil && message.ToolResult.Status == agent.ToolResultError {
				response = map[string]any{"error": message.Content}
			}
			contents = append(contents, genai.NewContentFromParts([]*genai.Part{{
				FunctionResponse: &genai.FunctionResponse{
					ID:       message.ToolCallID,
					Name:     name,
					Response: response,
				},
			}}, genai.RoleUser))
		default:
			return nil, nil, fmt.Errorf("google generative AI input %d: unsupported role %q", index, message.Role)
		}
	}
	var system *genai.Content
	if len(systemParts) != 0 {
		system = genai.NewContentFromParts(systemParts, genai.RoleUser)
	}
	return system, contents, nil
}

func assistantParts(message *agent.Message, config providers.ModelConfig) ([]*genai.Part, error) {
	if message.Extra != nil {
		var parts []*genai.Part
		matched, err := providers.DecodeContinuation(message.Extra, config, &parts)
		if err != nil {
			return nil, err
		}
		if matched {
			return parts, nil
		}
	}
	parts := make([]*genai.Part, 0, 1+len(message.ToolCalls))
	if message.Content != "" || len(message.ToolCalls) == 0 {
		parts = append(parts, genai.NewPartFromText(message.Content))
	}
	for index, call := range message.ToolCalls {
		if call.Type != "" && call.Type != "function" {
			return nil, fmt.Errorf("tool call %d has unsupported type %q", index, call.Type)
		}
		arguments := map[string]any{}
		if strings.TrimSpace(call.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
				return nil, fmt.Errorf("tool call %d arguments: %w", index, err)
			}
		}
		parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{
			ID: call.ID, Name: call.Function.Name, Args: arguments,
		}})
	}
	return parts, nil
}

func requestTools(tools []*agent.ToolInfo) ([]*genai.Tool, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	declarations := make([]*genai.FunctionDeclaration, 0, len(tools))
	for index, tool := range tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("google generative AI tool %d: name is required", index)
		}
		schema := any(map[string]any{"type": "object", "properties": map[string]any{}})
		if tool.ParamsOneOf != nil {
			value, err := tool.ParamsOneOf.ToJSONSchema()
			if err != nil {
				return nil, fmt.Errorf("google generative AI tool %q schema: %w", tool.Name, err)
			}
			if value != nil {
				data, err := json.Marshal(value)
				if err != nil {
					return nil, fmt.Errorf("google generative AI tool %q schema: %w", tool.Name, err)
				}
				if err := json.Unmarshal(data, &schema); err != nil {
					return nil, fmt.Errorf("google generative AI tool %q schema: %w", tool.Name, err)
				}
			}
		}
		declarations = append(declarations, &genai.FunctionDeclaration{
			Name: tool.Name, Description: tool.Desc, ParametersJsonSchema: schema,
		})
	}
	return []*genai.Tool{{FunctionDeclarations: declarations}}, nil
}

func requestToolConfig(choice *agent.ToolChoice, allowedNames []string, toolCount int) (*genai.ToolConfig, error) {
	if choice == nil {
		return nil, nil
	}
	functionCalling := &genai.FunctionCallingConfig{AllowedFunctionNames: append([]string(nil), allowedNames...)}
	switch *choice {
	case agent.ToolChoiceForbidden:
		functionCalling.Mode = genai.FunctionCallingConfigModeNone
	case agent.ToolChoiceAllowed:
		functionCalling.Mode = genai.FunctionCallingConfigModeAuto
	case agent.ToolChoiceForced:
		if toolCount == 0 {
			return nil, fmt.Errorf("google generative AI: forced tool choice has no available tools")
		}
		functionCalling.Mode = genai.FunctionCallingConfigModeAny
	default:
		return nil, fmt.Errorf("google generative AI: unsupported tool choice %q", *choice)
	}
	return &genai.ToolConfig{FunctionCallingConfig: functionCalling}, nil
}

func applyThinking(config *genai.GenerateContentConfig, compatibility Compatibility, level providers.ThinkingLevel) {
	if level == "" || level == providers.ThinkingLevelDefault || compatibility.ThinkingMode == ThinkingModeNone {
		return
	}
	thinking := &genai.ThinkingConfig{IncludeThoughts: level != providers.ThinkingLevelOff}
	switch compatibility.ThinkingMode {
	case ThinkingModeLevel:
		if level == providers.ThinkingLevelOff {
			zero := int32(0)
			thinking.ThinkingBudget = &zero
		} else {
			mapped := compatibility.ThinkingLevels[string(level)]
			if mapped == "" {
				mapped = strings.ToUpper(string(level))
			}
			thinking.ThinkingLevel = genai.ThinkingLevel(mapped)
		}
	case ThinkingModeBudget:
		budget := compatibility.ThinkingBudgets[string(level)]
		thinking.ThinkingBudget = &budget
	}
	config.ThinkingConfig = thinking
}

func applyOutputFormat(config *genai.GenerateContentConfig, format *providers.OutputFormat) error {
	if format == nil || format.Type == "" || format.Type == providers.OutputFormatText {
		return nil
	}
	if format.Type != providers.OutputFormatJSONObject && format.Type != providers.OutputFormatJSONSchema {
		return fmt.Errorf("google generative AI: unsupported output format %q", format.Type)
	}
	config.ResponseMIMEType = "application/json"
	if format.Type == providers.OutputFormatJSONSchema {
		data, err := json.Marshal(format.Schema)
		if err != nil {
			return fmt.Errorf("google generative AI output format: %w", err)
		}
		var schema any
		if err := json.Unmarshal(data, &schema); err != nil {
			return fmt.Errorf("google generative AI output format: %w", err)
		}
		config.ResponseJsonSchema = schema
	}
	return nil
}

func filterTools(tools []*agent.ToolInfo, names []string) []*agent.ToolInfo {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	result := make([]*agent.ToolInfo, 0, len(tools))
	for _, tool := range tools {
		if tool != nil {
			if _, ok := allowed[tool.Name]; ok {
				result = append(result, tool)
			}
		}
	}
	return result
}
