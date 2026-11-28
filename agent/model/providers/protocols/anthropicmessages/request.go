package anthropicmessages

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentmodel "github.com/alfredxw/denova/agent/model"
	"github.com/alfredxw/denova/agent/model/providers"
	agentschema "github.com/alfredxw/denova/agent/schema"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func (model *ChatModel) request(input []*agentschema.Message, opts ...agentmodel.ModelOption) (anthropic.MessageNewParams, []option.RequestOption, error) {
	common := agentmodel.GetCommonOptions(model.options, opts...)
	system, messages, prefixMessageIndex, err := requestMessages(input, model.config, common.CachePrefixMessages)
	if err != nil {
		return anthropic.MessageNewParams{}, nil, err
	}
	tools := common.Tools
	if common.ToolChoice != nil && len(common.AllowedToolNames) != 0 {
		tools = filterTools(tools, common.AllowedToolNames)
	}
	requestTools, err := requestTools(tools)
	if err != nil {
		return anthropic.MessageNewParams{}, nil, err
	}
	toolChoice, err := requestToolChoice(common.ToolChoice, len(requestTools), *model.compatibility.SupportsToolChoice)
	if err != nil {
		return anthropic.MessageNewParams{}, nil, err
	}
	maxTokens := int(model.compatibility.DefaultMaxOutputTokens)
	if model.config.MaxOutputTokens != nil {
		maxTokens = *model.config.MaxOutputTokens
	}
	if common.MaxTokens != nil {
		maxTokens = *common.MaxTokens
	}
	params := anthropic.MessageNewParams{
		MaxTokens:  int64(maxTokens),
		Messages:   messages,
		Model:      anthropic.Model(model.config.Model),
		System:     system,
		Tools:      requestTools,
		ToolChoice: toolChoice,
	}
	if model.config.Temperature != nil {
		params.Temperature = anthropic.Float(float64(*model.config.Temperature))
	}
	if err := applyThinking(&params, model.compatibility, model.config.ThinkingLevel); err != nil {
		return anthropic.MessageNewParams{}, nil, err
	}
	if err := applyOutputFormat(&params, model.config.OutputFormat); err != nil {
		return anthropic.MessageNewParams{}, nil, err
	}
	applyPromptCacheControl(&params, model.compatibility, prefixMessageIndex)
	return params, requestOptions(model.compatibility.ExtraBody, model.config.SessionKeyMapping, common.SessionKey), nil
}

func requestMessages(input []*agentschema.Message, config providers.ModelConfig, cachePrefixMessages int) ([]anthropic.TextBlockParam, []anthropic.MessageParam, int, error) {
	system := make([]anthropic.TextBlockParam, 0)
	messages := make([]anthropic.MessageParam, 0, len(input))
	imageCount := providers.NativeImageCount(input)
	prefixMessageIndex := -1
	previousWasTool := false
	for index, message := range input {
		if message == nil {
			return nil, nil, -1, fmt.Errorf("anthropic messages input %d: nil message", index)
		}
		switch message.Role {
		case agentschema.System:
			previousWasTool = false
			system = append(system, anthropic.TextBlockParam{Text: message.Content})
		case agentschema.User:
			previousWasTool = false
			blocks := []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(agentschema.ModelUserContent(message))}
			for _, attachment := range message.Attachments {
				if !agentschema.IsNativeImageMediaType(attachment.MediaType) {
					continue
				}
				prepared, err := config.PrepareImage(attachment, imageCount)
				if err != nil {
					return nil, nil, -1, fmt.Errorf("anthropic messages input %d: %w", index, err)
				}
				blocks = append(blocks, anthropic.NewImageBlockBase64(prepared.MediaType, prepared.Base64))
			}
			messages = append(messages, anthropic.NewUserMessage(blocks...))
			if index < cachePrefixMessages {
				prefixMessageIndex = len(messages) - 1
			}
		case agentschema.Assistant:
			previousWasTool = false
			blocks, err := assistantBlocks(message, config)
			if err != nil {
				return nil, nil, -1, fmt.Errorf("anthropic messages input %d: %w", index, err)
			}
			messages = append(messages, anthropic.NewAssistantMessage(blocks...))
			if index < cachePrefixMessages {
				prefixMessageIndex = len(messages) - 1
			}
		case agentschema.ToolRole:
			if strings.TrimSpace(message.ToolCallID) == "" {
				return nil, nil, -1, fmt.Errorf("anthropic messages input %d: tool result requires tool call id", index)
			}
			isError := message.ToolResult != nil && message.ToolResult.Status == agentschema.ToolResultError
			result := anthropic.NewToolResultBlock(message.ToolCallID, message.Content, isError)
			for _, attachment := range message.Attachments {
				prepared, err := config.PrepareImage(attachment, imageCount)
				if err != nil {
					return nil, nil, -1, fmt.Errorf("anthropic messages input %d: %w", index, err)
				}
				image := anthropic.NewImageBlockBase64(prepared.MediaType, prepared.Base64)
				result.OfToolResult.Content = append(result.OfToolResult.Content, anthropic.ToolResultBlockParamContentUnion{OfImage: image.OfImage})
			}
			if previousWasTool && len(messages) > 0 {
				messages[len(messages)-1].Content = append(messages[len(messages)-1].Content, result)
			} else {
				messages = append(messages, anthropic.NewUserMessage(result))
			}
			previousWasTool = true
			if index < cachePrefixMessages {
				prefixMessageIndex = len(messages) - 1
			}
		default:
			return nil, nil, -1, fmt.Errorf("anthropic messages input %d: unsupported role %q", index, message.Role)
		}
	}
	return system, messages, prefixMessageIndex, nil
}

// applyPromptCacheControl places at most four ephemeral cache breakpoints
// (tools, system, committed cross-turn history tail, and the final cacheable
// message block) in Anthropic's tools -> system -> messages evaluation order.
func applyPromptCacheControl(params *anthropic.MessageNewParams, compatibility Compatibility, prefixMessageIndex int) {
	if params == nil || compatibility.PromptCacheMode == PromptCacheModeDisabled {
		return
	}
	ephemeral := anthropic.NewCacheControlEphemeralParam()
	if len(params.Tools) > 0 && compatibility.SupportsCacheControlOnTools != nil && *compatibility.SupportsCacheControlOnTools {
		if lastTool := params.Tools[len(params.Tools)-1].OfTool; lastTool != nil {
			lastTool.CacheControl = ephemeral
		}
	}
	if len(params.System) > 0 {
		for index := len(params.System) - 1; index >= 0; index-- {
			if params.System[index].Text != "" {
				params.System[index].CacheControl = ephemeral
				break
			}
		}
	}
	lastMarked := applyMessageCacheControl(params.Messages, len(params.Messages)-1, ephemeral)
	if prefixMessageIndex >= 0 && prefixMessageIndex < lastMarked {
		applyMessageCacheControl(params.Messages, prefixMessageIndex, ephemeral)
	}
}

func applyMessageCacheControl(messages []anthropic.MessageParam, maxIndex int, cacheControl anthropic.CacheControlEphemeralParam) int {
	for msgIdx := min(maxIndex, len(messages)-1); msgIdx >= 0; msgIdx-- {
		content := messages[msgIdx].Content
		for blockIdx := len(content) - 1; blockIdx >= 0; blockIdx-- {
			if setBlockCacheControl(&content[blockIdx], cacheControl) {
				return msgIdx
			}
		}
	}
	return -1
}

func setBlockCacheControl(block *anthropic.ContentBlockParamUnion, cacheControl anthropic.CacheControlEphemeralParam) bool {
	switch {
	case block.OfText != nil:
		if block.OfText.Text == "" {
			return false
		}
		block.OfText.CacheControl = cacheControl
		return true
	case block.OfImage != nil:
		block.OfImage.CacheControl = cacheControl
		return true
	case block.OfDocument != nil:
		block.OfDocument.CacheControl = cacheControl
		return true
	case block.OfSearchResult != nil:
		block.OfSearchResult.CacheControl = cacheControl
		return true
	case block.OfToolUse != nil:
		block.OfToolUse.CacheControl = cacheControl
		return true
	case block.OfToolResult != nil:
		block.OfToolResult.CacheControl = cacheControl
		return true
	default:
		return false
	}
}

func assistantBlocks(message *agentschema.Message, config providers.ModelConfig) ([]anthropic.ContentBlockParamUnion, error) {
	if message.Extra != nil {
		var rawBlocks []json.RawMessage
		matched, err := providers.DecodeContinuation(message.Extra, config, &rawBlocks)
		if err != nil {
			return nil, err
		}
		if matched {
			retainedToolCalls := make(map[string]agentschema.ToolCall, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				if callID := strings.TrimSpace(call.ID); callID != "" {
					retainedToolCalls[callID] = call
				}
			}
			blocks := make([]anthropic.ContentBlockParamUnion, 0, len(rawBlocks))
			for index, raw := range rawBlocks {
				var identity struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				}
				if err := json.Unmarshal(raw, &identity); err != nil {
					return nil, fmt.Errorf("decode stored Anthropic content block %d identity: %w", index, err)
				}
				projected := append(json.RawMessage(nil), raw...)
				if identity.Type == "tool_use" {
					retained, ok := retainedToolCalls[strings.TrimSpace(identity.ID)]
					if !ok {
						continue
					}
					var input any = map[string]any{}
					if strings.TrimSpace(retained.Function.Arguments) != "" {
						if err := json.Unmarshal([]byte(retained.Function.Arguments), &input); err != nil {
							return nil, fmt.Errorf("tool call %d arguments: %w", index, err)
						}
					}
					var item map[string]any
					if err := json.Unmarshal(raw, &item); err != nil {
						return nil, fmt.Errorf("decode stored Anthropic tool block %d: %w", index, err)
					}
					item["id"] = retained.ID
					item["name"] = retained.Function.Name
					item["input"] = input
					projected, err = json.Marshal(item)
					if err != nil {
						return nil, fmt.Errorf("project stored Anthropic tool block %d: %w", index, err)
					}
				}
				var block anthropic.ContentBlockParamUnion
				if err := json.Unmarshal(projected, &block); err != nil {
					return nil, fmt.Errorf("decode stored Anthropic content block %d: %w", index, err)
				}
				blocks = append(blocks, block)
			}
			return blocks, nil
		}
	}
	blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(message.ToolCalls))
	if message.Content != "" || len(message.ToolCalls) == 0 {
		blocks = append(blocks, anthropic.NewTextBlock(message.Content))
	}
	for index, call := range message.ToolCalls {
		if call.Type != "" && call.Type != "function" {
			return nil, fmt.Errorf("tool call %d has unsupported type %q", index, call.Type)
		}
		var arguments any = map[string]any{}
		if strings.TrimSpace(call.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
				return nil, fmt.Errorf("tool call %d arguments: %w", index, err)
			}
		}
		blocks = append(blocks, anthropic.NewToolUseBlock(call.ID, arguments, call.Function.Name))
	}
	return blocks, nil
}

func requestTools(tools []*agentschema.ToolInfo) ([]anthropic.ToolUnionParam, error) {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for index, tool := range tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("anthropic messages tool %d: name is required", index)
		}
		schema := map[string]any{"properties": json.RawMessage(`{}`)}
		if tool.ParamsOneOf != nil {
			var err error
			schema, err = tool.ParamsOneOf.ToJSONSchemaMap()
			if err != nil {
				return nil, fmt.Errorf("anthropic messages tool %q schema: %w", tool.Name, err)
			}
		}
		inputSchema := anthropic.ToolInputSchemaParam{Properties: schema["properties"]}
		if required, ok := schema["required"].(json.RawMessage); ok {
			if err := json.Unmarshal(required, &inputSchema.Required); err != nil {
				return nil, fmt.Errorf("anthropic messages tool %q required fields: %w", tool.Name, err)
			}
		}
		inputSchema.ExtraFields = make(map[string]any)
		for key, value := range schema {
			if key != "type" && key != "properties" && key != "required" {
				inputSchema.ExtraFields[key] = value
			}
		}
		definition := anthropic.ToolParam{Name: tool.Name, InputSchema: inputSchema}
		if tool.Desc != "" {
			definition.Description = anthropic.String(tool.Desc)
		}
		result = append(result, anthropic.ToolUnionParam{OfTool: &definition})
	}
	return result, nil
}

func requestToolChoice(choice *agentmodel.ToolChoice, toolCount int, supported bool) (anthropic.ToolChoiceUnionParam, error) {
	if choice == nil {
		return anthropic.ToolChoiceUnionParam{}, nil
	}
	if !supported {
		return anthropic.ToolChoiceUnionParam{}, fmt.Errorf("anthropic messages: endpoint does not support tool_choice")
	}
	switch *choice {
	case agentmodel.ToolChoiceForbidden:
		value := anthropic.NewToolChoiceNoneParam()
		return anthropic.ToolChoiceUnionParam{OfNone: &value}, nil
	case agentmodel.ToolChoiceAllowed:
		return anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}}, nil
	case agentmodel.ToolChoiceForced:
		if toolCount == 0 {
			return anthropic.ToolChoiceUnionParam{}, fmt.Errorf("anthropic messages: forced tool choice has no available tools")
		}
		return anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{}}, nil
	default:
		return anthropic.ToolChoiceUnionParam{}, fmt.Errorf("anthropic messages: unsupported tool choice %q", *choice)
	}
}

func applyThinking(params *anthropic.MessageNewParams, compatibility Compatibility, level providers.ThinkingLevel) error {
	if level == "" || level == providers.ThinkingLevelDefault {
		return nil
	}
	if level == providers.ThinkingLevelOff {
		disabled := anthropic.NewThinkingConfigDisabledParam()
		params.Thinking.OfDisabled = &disabled
		return nil
	}
	switch compatibility.ThinkingMode {
	case ThinkingModeNone:
	case ThinkingModeAdaptive:
		params.Thinking.OfAdaptive = &anthropic.ThinkingConfigAdaptiveParam{}
	case ThinkingModeBudget:
		budget := compatibility.thinkingBudget(level)
		if budget >= params.MaxTokens {
			return fmt.Errorf("anthropic messages: thinking budget %d must be lower than max output tokens %d", budget, params.MaxTokens)
		}
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
	}
	if effort, ok := compatibility.mappedEffort(level); ok {
		params.OutputConfig.Effort = anthropic.OutputConfigEffort(effort)
	}
	return nil
}

func applyOutputFormat(params *anthropic.MessageNewParams, format *providers.OutputFormat) error {
	if format == nil || format.Type == "" || format.Type == providers.OutputFormatText {
		return nil
	}
	if format.Type != providers.OutputFormatJSONSchema && format.Type != providers.OutputFormatJSONObject {
		return fmt.Errorf("anthropic messages: unsupported output format %q", format.Type)
	}
	schema := map[string]any{"type": "object"}
	if format.Type == providers.OutputFormatJSONSchema {
		data, err := json.Marshal(format.Schema)
		if err != nil {
			return fmt.Errorf("anthropic messages output format: %w", err)
		}
		if err := json.Unmarshal(data, &schema); err != nil {
			return fmt.Errorf("anthropic messages output format: %w", err)
		}
	}
	params.OutputConfig.Format = anthropic.JSONOutputFormatParam{Schema: schema}
	return nil
}

func requestOptions(extraBody map[string]any, mapping *providers.SessionKeyMapping, sessionKey string) []option.RequestOption {
	keys := make([]string, 0, len(extraBody))
	for key := range extraBody {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]option.RequestOption, 0, len(keys))
	for _, key := range keys {
		result = append(result, option.WithJSONSet(escapeJSONPathKey(key), extraBody[key]))
	}
	if mapping != nil && sessionKey != "" {
		switch mapping.Location {
		case providers.SessionKeyLocationHeader:
			result = append(result, option.WithHeader(mapping.Name, sessionKey))
		case providers.SessionKeyLocationBody:
			result = append(result, option.WithJSONSet(escapeJSONPathKey(mapping.Name), sessionKey))
		}
	}
	return result
}

func filterTools(tools []*agentschema.ToolInfo, names []string) []*agentschema.ToolInfo {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	result := make([]*agentschema.ToolInfo, 0, len(tools))
	for _, tool := range tools {
		if tool != nil {
			if _, ok := allowed[tool.Name]; ok {
				result = append(result, tool)
			}
		}
	}
	return result
}

func escapeJSONPathKey(key string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `.`, `\.`, `:`, `\:`, `#`, `\#`, `@`, `\@`)
	return replacer.Replace(key)
}
