package openairesponses

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func (model *ChatModel) request(input []*agent.Message, opts ...agent.ModelOption) (responses.ResponseNewParams, error) {
	items, err := requestInput(input, model.config)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}
	common := agent.GetCommonOptions(model.options, opts...)
	tools := common.Tools
	if common.ToolChoice != nil && len(common.AllowedToolNames) != 0 {
		tools = filterTools(tools, common.AllowedToolNames)
	}
	requestTools, err := requestTools(tools)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}
	toolChoice, err := requestToolChoice(common.ToolChoice, len(requestTools))
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	params := responses.ResponseNewParams{
		Input:      responses.ResponseNewParamsInputUnion{OfInputItemList: items},
		Model:      shared.ResponsesModel(model.config.Model),
		Tools:      requestTools,
		ToolChoice: toolChoice,
		Store:      sdk.Bool(false),
		Include: []responses.ResponseIncludable{
			responses.ResponseIncludableReasoningEncryptedContent,
		},
	}
	if model.config.Temperature != nil {
		params.Temperature = sdk.Float(float64(*model.config.Temperature))
	}
	maxTokens := model.config.MaxOutputTokens
	if common.MaxTokens != nil {
		maxTokens = common.MaxTokens
	}
	if maxTokens != nil {
		params.MaxOutputTokens = sdk.Int(int64(*maxTokens))
	}
	applyThinkingLevel(&params, model.config.ThinkingLevel)
	if err := applyOutputFormat(&params, model.config.OutputFormat); err != nil {
		return responses.ResponseNewParams{}, err
	}
	return params, nil
}

func requestInput(messages []*agent.Message, config providers.ModelConfig) (responses.ResponseInputParam, error) {
	result := make(responses.ResponseInputParam, 0, len(messages))
	for index, message := range messages {
		if message == nil {
			return nil, fmt.Errorf("openai responses input message %d: nil message", index)
		}
		items, err := requestMessage(message, config)
		if err != nil {
			return nil, fmt.Errorf("openai responses input message %d: %w", index, err)
		}
		result = append(result, items...)
	}
	return result, nil
}

func requestMessage(message *agent.Message, config providers.ModelConfig) ([]responses.ResponseInputItemUnionParam, error) {
	switch message.Role {
	case agent.System, agent.User:
		role := responses.EasyInputMessageRole(message.Role)
		return []responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfMessage(message.Content, role),
		}, nil
	case agent.Assistant:
		if replay, found, err := replayResponseOutput(message, config); found || err != nil {
			return replay, err
		}
		result := make([]responses.ResponseInputItemUnionParam, 0, 1+len(message.ToolCalls))
		if message.Content != "" || len(message.ToolCalls) == 0 {
			result = append(result, responses.ResponseInputItemParamOfMessage(message.Content, responses.EasyInputMessageRoleAssistant))
		}
		for callIndex, call := range message.ToolCalls {
			if call.Type != "" && call.Type != "function" {
				return nil, fmt.Errorf("tool call %d has unsupported type %q", callIndex, call.Type)
			}
			if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Function.Name) == "" {
				return nil, fmt.Errorf("tool call %d requires id and function name", callIndex)
			}
			result = append(result, responses.ResponseInputItemParamOfFunctionCall(
				call.Function.Arguments,
				call.ID,
				call.Function.Name,
			))
		}
		return result, nil
	case agent.ToolRole:
		if strings.TrimSpace(message.ToolCallID) == "" {
			return nil, fmt.Errorf("tool result requires tool call id")
		}
		// A JSON-looking tool result deliberately remains a string. Changing it
		// to an object would alter the durable transcript's model projection.
		return []responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfFunctionCallOutput(message.ToolCallID, message.Content),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported role %q", message.Role)
	}
}

func replayResponseOutput(message *agent.Message, config providers.ModelConfig) ([]responses.ResponseInputItemUnionParam, bool, error) {
	if message == nil || message.Extra == nil {
		return nil, false, nil
	}
	var rawItems []json.RawMessage
	matched, err := providers.DecodeContinuation(message.Extra, config, &rawItems)
	if err != nil || !matched {
		return nil, matched, err
	}
	items := make([]responses.ResponseInputItemUnionParam, 0, len(rawItems))
	for index, raw := range rawItems {
		var item responses.ResponseInputItemUnionParam
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, true, fmt.Errorf("decode stored Responses output item %d: %w", index, err)
		}
		items = append(items, item)
	}
	return items, true, nil
}

func requestTools(tools []*agent.ToolInfo) ([]responses.ToolUnionParam, error) {
	if tools == nil {
		return nil, nil
	}
	result := make([]responses.ToolUnionParam, 0, len(tools))
	for index, tool := range tools {
		if tool == nil {
			return nil, fmt.Errorf("openai responses tool %d: nil tool", index)
		}
		if strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("openai responses tool %d: name is required", index)
		}
		parameters := map[string]any{"type": "object", "properties": map[string]any{}}
		if tool.ParamsOneOf != nil {
			schema, err := tool.ParamsOneOf.ToJSONSchema()
			if err != nil {
				return nil, fmt.Errorf("openai responses tool %q schema: %w", tool.Name, err)
			}
			if schema != nil {
				parameters, err = schemaMap(schema)
				if err != nil {
					return nil, fmt.Errorf("openai responses tool %q schema: %w", tool.Name, err)
				}
			}
		}
		definition := responses.ToolParamOfFunction(tool.Name, parameters, false)
		if tool.Desc != "" {
			definition.OfFunction.Description = sdk.String(tool.Desc)
		}
		result = append(result, definition)
	}
	return result, nil
}

func schemaMap(schema any) (map[string]any, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON Schema: %w", err)
	}
	result := map[string]any{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode JSON Schema: %w", err)
	}
	sortRequired(result)
	return result, nil
}

func sortRequired(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if required, ok := typed["required"].([]any); ok {
			values := make([]string, 0, len(required))
			for _, item := range required {
				text, ok := item.(string)
				if !ok {
					values = nil
					break
				}
				values = append(values, text)
			}
			if values != nil {
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

func requestToolChoice(choice *agent.ToolChoice, toolCount int) (responses.ResponseNewParamsToolChoiceUnion, error) {
	result := responses.ResponseNewParamsToolChoiceUnion{}
	if choice == nil {
		return result, nil
	}
	switch *choice {
	case agent.ToolChoiceForbidden:
		result.OfToolChoiceMode = sdk.Opt(responses.ToolChoiceOptionsNone)
	case agent.ToolChoiceAllowed:
		result.OfToolChoiceMode = sdk.Opt(responses.ToolChoiceOptionsAuto)
	case agent.ToolChoiceForced:
		if toolCount == 0 {
			return result, fmt.Errorf("openai responses: forced tool choice has no available tools")
		}
		result.OfToolChoiceMode = sdk.Opt(responses.ToolChoiceOptionsRequired)
	default:
		return result, fmt.Errorf("openai responses: unsupported tool choice %q", *choice)
	}
	return result, nil
}

func applyThinkingLevel(params *responses.ResponseNewParams, level providers.ThinkingLevel) {
	switch level {
	case providers.ThinkingLevelDefault, "":
		return
	case providers.ThinkingLevelOff:
		params.Reasoning.Effort = shared.ReasoningEffortNone
		return
	case providers.ThinkingLevelMinimal:
		params.Reasoning.Effort = shared.ReasoningEffortMinimal
	case providers.ThinkingLevelLow:
		params.Reasoning.Effort = shared.ReasoningEffortLow
	case providers.ThinkingLevelMedium:
		params.Reasoning.Effort = shared.ReasoningEffortMedium
	case providers.ThinkingLevelHigh:
		params.Reasoning.Effort = shared.ReasoningEffortHigh
	case providers.ThinkingLevelXHigh:
		params.Reasoning.Effort = shared.ReasoningEffortXhigh
	case providers.ThinkingLevelMax:
		params.Reasoning.Effort = shared.ReasoningEffortMax
	}
	params.Reasoning.Summary = shared.ReasoningSummaryAuto
}

func applyOutputFormat(params *responses.ResponseNewParams, format *providers.OutputFormat) error {
	if format == nil || format.Type == "" {
		return nil
	}
	switch format.Type {
	case providers.OutputFormatText:
		value := shared.NewResponseFormatTextParam()
		params.Text.Format.OfText = &value
	case providers.OutputFormatJSONObject:
		value := shared.NewResponseFormatJSONObjectParam()
		params.Text.Format.OfJSONObject = &value
	case providers.OutputFormatJSONSchema:
		schema, err := schemaMap(format.Schema)
		if err != nil {
			return fmt.Errorf("openai responses output format: %w", err)
		}
		value := responses.ResponseFormatTextJSONSchemaConfigParam{
			Name:   format.Name,
			Schema: schema,
			Strict: sdk.Bool(format.Strict),
		}
		if format.Description != "" {
			value.Description = sdk.String(format.Description)
		}
		params.Text.Format.OfJSONSchema = &value
	default:
		return fmt.Errorf("openai responses: unsupported output format %q", format.Type)
	}
	return nil
}
