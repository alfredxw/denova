package openaichatcompletions

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func (model *ChatModel) request(input []*agent.Message, stream bool, opts ...agent.ModelOption) (sdk.ChatCompletionNewParams, []option.RequestOption, error) {
	messages, err := requestMessages(input, model.compatibility, model.config.ThinkingLevel)
	if err != nil {
		return sdk.ChatCompletionNewParams{}, nil, err
	}
	common := agent.GetCommonOptions(model.options, opts...)
	tools := common.Tools
	if common.ToolChoice != nil && len(common.AllowedToolNames) != 0 {
		tools = filterTools(tools, common.AllowedToolNames)
	}
	requestTools, err := requestTools(tools)
	if err != nil {
		return sdk.ChatCompletionNewParams{}, nil, err
	}
	toolChoice, err := requestToolChoice(common.ToolChoice, len(requestTools), *model.compatibility.SupportsToolChoice)
	if err != nil {
		return sdk.ChatCompletionNewParams{}, nil, err
	}

	params := sdk.ChatCompletionNewParams{
		Messages:   messages,
		Model:      shared.ChatModel(model.config.Model),
		Tools:      requestTools,
		ToolChoice: toolChoice,
	}
	if model.config.Temperature != nil {
		params.Temperature = sdk.Float(float64(*model.config.Temperature))
	}
	maxTokens := model.config.MaxOutputTokens
	if common.MaxTokens != nil {
		maxTokens = common.MaxTokens
	}
	if maxTokens != nil {
		switch model.compatibility.MaxTokensField {
		case MaxTokensFieldMaxCompletionTokens:
			params.MaxCompletionTokens = sdk.Int(int64(*maxTokens))
		case MaxTokensFieldMaxTokens:
			params.MaxTokens = sdk.Int(int64(*maxTokens))
		}
	}
	if stream && *model.compatibility.SupportsStreamUsage {
		params.StreamOptions.IncludeUsage = sdk.Bool(true)
	}

	return params, model.requestOptions(), nil
}

func requestMessages(messages []*agent.Message, compatibility Compatibility, thinkingLevel providers.ThinkingLevel) ([]sdk.ChatCompletionMessageParamUnion, error) {
	result := make([]sdk.ChatCompletionMessageParamUnion, 0, len(messages))
	for index, message := range messages {
		if message == nil {
			return nil, fmt.Errorf("openai request message %d: nil message", index)
		}
		mapped, err := requestMessage(message, compatibility, thinkingLevel)
		if err != nil {
			return nil, fmt.Errorf("openai request message %d: %w", index, err)
		}
		result = append(result, mapped)
	}
	return result, nil
}

func requestMessage(message *agent.Message, compatibility Compatibility, thinkingLevel providers.ThinkingLevel) (sdk.ChatCompletionMessageParamUnion, error) {
	switch message.Role {
	case agent.System:
		result := sdk.SystemMessage(message.Content)
		if message.Name != "" {
			result.OfSystem.Name = sdk.String(message.Name)
		}
		return result, nil
	case agent.User:
		result := sdk.UserMessage(message.Content)
		if message.Name != "" {
			result.OfUser.Name = sdk.String(message.Name)
		}
		return result, nil
	case agent.Assistant:
		assistant := sdk.ChatCompletionAssistantMessageParam{}
		if message.Content != "" || len(message.ToolCalls) == 0 || compatibility.RequiresAssistantToolContent {
			assistant.Content.OfString = sdk.String(message.Content)
		}
		if message.Name != "" {
			assistant.Name = sdk.String(message.Name)
		}
		if compatibility.shouldReplayReasoning(message, thinkingLevel) && message.ReasoningContent != "" {
			assistant.SetExtraFields(map[string]any{compatibility.ReasoningContentField: message.ReasoningContent})
		}
		for callIndex, call := range message.ToolCalls {
			if call.Type != "" && call.Type != "function" {
				return sdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("tool call %d has unsupported type %q", callIndex, call.Type)
			}
			assistant.ToolCalls = append(assistant.ToolCalls, sdk.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &sdk.ChatCompletionMessageFunctionToolCallParam{
					ID: call.ID,
					Function: sdk.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      call.Function.Name,
						Arguments: call.Function.Arguments,
					},
				},
			})
		}
		return sdk.ChatCompletionMessageParamUnion{OfAssistant: &assistant}, nil
	case agent.ToolRole:
		// The string constructor is deliberate: tool outputs that happen to be
		// valid JSON must remain JSON strings in Chat Completions history.
		return sdk.ToolMessage(message.Content, message.ToolCallID), nil
	default:
		return sdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("unsupported role %q", message.Role)
	}
}

func requestTools(tools []*agent.ToolInfo) ([]sdk.ChatCompletionToolUnionParam, error) {
	if tools == nil {
		return nil, nil
	}
	result := make([]sdk.ChatCompletionToolUnionParam, 0, len(tools))
	for index, tool := range tools {
		if tool == nil {
			return nil, fmt.Errorf("openai request tool %d: nil tool", index)
		}
		if strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("openai request tool %d: name is required", index)
		}
		definition := shared.FunctionDefinitionParam{Name: tool.Name}
		if tool.Desc != "" {
			definition.Description = sdk.String(tool.Desc)
		}
		if tool.ParamsOneOf != nil {
			schema, err := tool.ParamsOneOf.ToJSONSchema()
			if err != nil {
				return nil, fmt.Errorf("openai request tool %q schema: %w", tool.Name, err)
			}
			if schema != nil {
				parameters, err := functionParameters(schema)
				if err != nil {
					return nil, fmt.Errorf("openai request tool %q schema: %w", tool.Name, err)
				}
				definition.Parameters = parameters
			}
		}
		result = append(result, sdk.ChatCompletionFunctionTool(definition))
	}
	return result, nil
}

func functionParameters(schema any) (shared.FunctionParameters, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON Schema: %w", err)
	}
	parameters := shared.FunctionParameters{}
	if err := json.Unmarshal(data, &parameters); err != nil {
		return nil, fmt.Errorf("decode JSON Schema: %w", err)
	}
	sortRequired(parameters)
	return parameters, nil
}

func sortRequired(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if required, ok := typed["required"].([]any); ok {
			values := make([]string, 0, len(required))
			allStrings := true
			for _, item := range required {
				text, ok := item.(string)
				if !ok {
					allStrings = false
					break
				}
				values = append(values, text)
			}
			if allStrings {
				sort.Strings(values)
				typed["required"] = values
			}
		}
		for _, child := range typed {
			sortRequired(child)
		}
	case []any:
		for _, child := range typed {
			sortRequired(child)
		}
	}
}

func filterTools(tools []*agent.ToolInfo, names []string) []*agent.ToolInfo {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	result := make([]*agent.ToolInfo, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		if _, ok := allowed[tool.Name]; ok {
			result = append(result, tool)
		}
	}
	return result
}

func requestToolChoice(choice *agent.ToolChoice, toolCount int, supported bool) (sdk.ChatCompletionToolChoiceOptionUnionParam, error) {
	if choice == nil {
		return sdk.ChatCompletionToolChoiceOptionUnionParam{}, nil
	}
	if !supported {
		return sdk.ChatCompletionToolChoiceOptionUnionParam{}, fmt.Errorf("openai request: endpoint does not support tool_choice")
	}
	result := sdk.ChatCompletionToolChoiceOptionUnionParam{}
	switch *choice {
	case agent.ToolChoiceForbidden:
		result.OfAuto = sdk.String("none")
	case agent.ToolChoiceAllowed:
		result.OfAuto = sdk.String("auto")
	case agent.ToolChoiceForced:
		if toolCount == 0 {
			return result, fmt.Errorf("openai request: forced tool choice has no available tools")
		}
		result.OfAuto = sdk.String("required")
	default:
		return result, fmt.Errorf("openai request: unsupported tool choice %q", *choice)
	}
	return result, nil
}

func (model *ChatModel) requestOptions() []option.RequestOption {
	extraFields := make(map[string]any, len(model.extraFields)+1)
	for key, value := range model.extraFields {
		extraFields[key] = value
	}
	for key, value := range model.compatibility.thinkingFields(model.config.ThinkingLevel) {
		extraFields[key] = value
	}
	result := make([]option.RequestOption, 0, len(extraFields)+2)
	if effort, ok := model.compatibility.mappedEffort(model.config.ThinkingLevel); ok {
		result = append(result, option.WithJSONSet("reasoning_effort", effort))
	}
	if format := chatResponseFormat(model.config.OutputFormat); format != nil {
		result = append(result, option.WithJSONSet("response_format", format))
	}
	keys := make([]string, 0, len(extraFields))
	for key := range extraFields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, option.WithJSONSet(escapeJSONPathKey(key), extraFields[key]))
	}
	return result
}

func chatResponseFormat(format *providers.OutputFormat) any {
	if format == nil || format.Type == "" {
		return nil
	}
	result := map[string]any{"type": string(format.Type)}
	if format.Type != providers.OutputFormatJSONSchema {
		return result
	}
	jsonSchema := map[string]any{
		"name":   format.Name,
		"schema": format.Schema,
		"strict": format.Strict,
	}
	if format.Description != "" {
		jsonSchema["description"] = format.Description
	}
	result["json_schema"] = jsonSchema
	return result
}

func escapeJSONPathKey(key string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`.`, `\.`,
		`:`, `\:`,
		`#`, `\#`,
		`@`, `\@`,
	)
	return replacer.Replace(key)
}
