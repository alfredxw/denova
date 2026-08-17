package anthropicmessages

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

const (
	extraKeyRequestID  = "anthropic-request-id"
	extraKeyProvider   = "provider"
	extraKeyProtocol   = "protocol"
	extraKeyResponseID = "response_id"
	extraKeyModel      = "model"
)

func responseMessage(response *anthropic.Message, rawResponse *http.Response, config providers.ModelConfig) (*agent.Message, error) {
	if response == nil {
		return nil, fmt.Errorf("anthropic messages: empty response")
	}
	message := &agent.Message{
		Role: agent.Assistant,
		ResponseMeta: &agent.ResponseMeta{
			FinishReason: string(response.StopReason),
			Usage:        responseUsage(response.Usage),
		},
		Extra: responseExtra(response, rawResponse, config),
	}
	var content strings.Builder
	var reasoning strings.Builder
	for _, block := range response.Content {
		switch block.Type {
		case "text":
			content.WriteString(block.Text)
		case "thinking":
			reasoning.WriteString(block.Thinking)
		case "tool_use":
			message.ToolCalls = append(message.ToolCalls, agent.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: agent.FunctionCall{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}
	message.Content = content.String()
	message.ReasoningContent = reasoning.String()
	continuation, err := responseContinuation(response, config)
	if err != nil {
		return nil, err
	}
	if continuation != nil {
		message.Extra[providers.ExtraKeyContinuation] = continuation
	}
	return message, nil
}

func responseContinuation(response *anthropic.Message, config providers.ModelConfig) (any, error) {
	if response == nil || len(response.Content) == 0 {
		return nil, nil
	}
	blocks := make([]any, 0, len(response.Content))
	for index, block := range response.Content {
		data, err := json.Marshal(block)
		if err != nil {
			return nil, fmt.Errorf("anthropic messages: preserve content block %d: %w", index, err)
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, fmt.Errorf("anthropic messages: preserve content block %d: %w", index, err)
		}
		blocks = append(blocks, value)
	}
	return providers.NewContinuation(config, blocks)
}

func responseUsage(usage anthropic.Usage) *agent.TokenUsage {
	promptTokens := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	return &agent.TokenUsage{
		PromptTokens:     int(promptTokens),
		CompletionTokens: int(usage.OutputTokens),
		TotalTokens:      int(promptTokens + usage.OutputTokens),
		PromptTokenDetails: agent.PromptTokenDetails{
			CachedTokens: int(usage.CacheReadInputTokens),
		},
		CompletionTokensDetails: agent.CompletionTokensDetails{
			ReasoningTokens: int(usage.OutputTokensDetails.ThinkingTokens),
		},
	}
}

func responseExtra(response *anthropic.Message, rawResponse *http.Response, config providers.ModelConfig) map[string]any {
	result := map[string]any{
		extraKeyProvider: string(config.Provider),
		extraKeyProtocol: string(providers.ProtocolAnthropicMessages),
	}
	if response != nil {
		if response.ID != "" {
			result[extraKeyResponseID] = response.ID
		}
		if response.Model != "" {
			result[extraKeyModel] = string(response.Model)
		}
	}
	if rawResponse != nil {
		for _, name := range []string{"request-id", "x-request-id"} {
			if value := strings.TrimSpace(rawResponse.Header.Get(name)); value != "" {
				result[extraKeyRequestID] = value
				break
			}
		}
	}
	return result
}

func streamMessage(event anthropic.MessageStreamEventUnion, accumulator *anthropic.Message, rawResponse *http.Response, config providers.ModelConfig) (*agent.Message, error) {
	switch event.Type {
	case "message_start":
		return &agent.Message{Role: agent.Assistant, Extra: responseExtra(accumulator, rawResponse, config)}, nil
	case "content_block_start":
		block := event.ContentBlock
		if block.Type != "tool_use" {
			return nil, nil
		}
		index := int(event.Index)
		return &agent.Message{
			Role: agent.Assistant,
			ToolCalls: []agent.ToolCall{{
				Index: &index,
				ID:    block.ID,
				Type:  "function",
				Function: agent.FunctionCall{
					Name: block.Name,
				},
			}},
		}, nil
	case "content_block_delta":
		delta := event.Delta
		switch delta.Type {
		case "text_delta":
			return &agent.Message{Role: agent.Assistant, Content: delta.Text}, nil
		case "thinking_delta":
			return &agent.Message{Role: agent.Assistant, ReasoningContent: delta.Thinking}, nil
		case "input_json_delta":
			index := int(event.Index)
			return &agent.Message{
				Role: agent.Assistant,
				ToolCalls: []agent.ToolCall{{
					Index:    &index,
					Type:     "function",
					Function: agent.FunctionCall{Arguments: delta.PartialJSON},
				}},
			}, nil
		default:
			return nil, nil
		}
	case "message_delta":
		return &agent.Message{
			Role: agent.Assistant,
			ResponseMeta: &agent.ResponseMeta{
				FinishReason: string(accumulator.StopReason),
				Usage:        responseUsage(accumulator.Usage),
			},
		}, nil
	case "message_stop":
		continuation, err := responseContinuation(accumulator, config)
		if err != nil {
			return nil, err
		}
		if continuation == nil {
			return nil, nil
		}
		return &agent.Message{
			Role:  agent.Assistant,
			Extra: map[string]any{providers.ExtraKeyContinuation: continuation},
		}, nil
	case "content_block_stop":
		return nil, nil
	default:
		return nil, fmt.Errorf("anthropic messages stream: unsupported event type %q", event.Type)
	}
}
