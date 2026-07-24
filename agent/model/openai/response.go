package openai

import (
	"encoding/json"
	"net/http"
	"strings"

	sdk "github.com/openai/openai-go/v3"

	agent "github.com/alfredxw/denova/agent"
)

const (
	// ExtraKeyRequestID stores the transport request ID as a plain string.
	ExtraKeyRequestID         = "openai-request-id"
	ExtraKeyProvider          = "provider"
	ExtraKeyResponseID        = "response_id"
	ExtraKeyModel             = "model"
	ExtraKeyCreated           = "created"
	ExtraKeyServiceTier       = "service_tier"
	ExtraKeySystemFingerprint = "system_fingerprint"
)

func responseMessage(response *sdk.ChatCompletion, rawResponse *http.Response) *agent.Message {
	choice, found := responseChoice(response.Choices)
	if !found {
		return nil
	}
	message := &agent.Message{
		Role:             responseRole(string(choice.Message.Role)),
		Content:          choice.Message.Content,
		ReasoningContent: rawReasoningContent(choice.Message.RawJSON()),
		ToolCalls:        responseToolCalls(choice.Message.ToolCalls),
		ResponseMeta: &agent.ResponseMeta{
			FinishReason: choice.FinishReason,
		},
		Extra: responseExtra(
			rawResponse,
			response.ID,
			response.Model,
			response.Created,
			string(response.ServiceTier),
			response.SystemFingerprint,
		),
	}
	if response.JSON.Usage.Valid() {
		message.ResponseMeta.Usage = responseUsage(response.Usage)
	}
	return message
}

func responseChoice(choices []sdk.ChatCompletionChoice) (sdk.ChatCompletionChoice, bool) {
	for _, choice := range choices {
		if choice.Index == 0 {
			return choice, true
		}
	}
	return sdk.ChatCompletionChoice{}, false
}

func streamChoice(choices []sdk.ChatCompletionChunkChoice) (sdk.ChatCompletionChunkChoice, bool) {
	for _, choice := range choices {
		if choice.Index == 0 {
			return choice, true
		}
	}
	return sdk.ChatCompletionChunkChoice{}, false
}

func streamMessage(chunk sdk.ChatCompletionChunk, rawResponse *http.Response, includeMetadata bool) (*agent.Message, bool) {
	choice, hasChoice := streamChoice(chunk.Choices)
	hasUsage := chunk.JSON.Usage.Valid()
	if !hasChoice && !hasUsage {
		return nil, false
	}

	message := &agent.Message{Role: agent.Assistant}
	if hasChoice {
		message.Role = responseRole(choice.Delta.Role)
		message.Content = choice.Delta.Content
		message.ReasoningContent = rawReasoningContent(choice.Delta.RawJSON())
		message.ToolCalls = streamToolCalls(choice.Delta.ToolCalls)
		if choice.FinishReason != "" {
			message.ResponseMeta = &agent.ResponseMeta{FinishReason: choice.FinishReason}
		}
	}
	if hasUsage {
		if message.ResponseMeta == nil {
			message.ResponseMeta = &agent.ResponseMeta{}
		}
		message.ResponseMeta.Usage = responseUsage(chunk.Usage)
	}
	if includeMetadata {
		message.Extra = responseExtra(
			rawResponse,
			chunk.ID,
			chunk.Model,
			chunk.Created,
			string(chunk.ServiceTier),
			chunk.SystemFingerprint,
		)
	}
	return message, true
}

func responseRole(role string) agent.RoleType {
	if role == "" {
		return agent.Assistant
	}
	return agent.RoleType(role)
}

func responseToolCalls(calls []sdk.ChatCompletionMessageToolCallUnion) []agent.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]agent.ToolCall, 0, len(calls))
	for _, call := range calls {
		callType := call.Type
		if callType == "" {
			callType = "function"
		}
		result = append(result, agent.ToolCall{
			ID:   call.ID,
			Type: callType,
			Function: agent.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return result
}

func streamToolCalls(calls []sdk.ChatCompletionChunkChoiceDeltaToolCall) []agent.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]agent.ToolCall, 0, len(calls))
	for _, call := range calls {
		index := int(call.Index)
		result = append(result, agent.ToolCall{
			Index: &index,
			ID:    call.ID,
			Type:  call.Type,
			Function: agent.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return result
}

func responseUsage(usage sdk.CompletionUsage) *agent.TokenUsage {
	return &agent.TokenUsage{
		PromptTokens:     int(usage.PromptTokens),
		CompletionTokens: int(usage.CompletionTokens),
		TotalTokens:      int(usage.TotalTokens),
		PromptTokenDetails: agent.PromptTokenDetails{
			CachedTokens: int(usage.PromptTokensDetails.CachedTokens),
		},
		CompletionTokensDetails: agent.CompletionTokensDetails{
			ReasoningTokens: int(usage.CompletionTokensDetails.ReasoningTokens),
		},
	}
}

func rawReasoningContent(raw string) string {
	if raw == "" {
		return ""
	}
	value := struct {
		ReasoningContent string `json:"reasoning_content"`
	}{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return ""
	}
	return value.ReasoningContent
}

func responseExtra(rawResponse *http.Response, responseID, model string, created int64, serviceTier, fingerprint string) map[string]any {
	result := map[string]any{ExtraKeyProvider: "openai"}
	if requestID := responseRequestID(rawResponse); requestID != "" {
		result[ExtraKeyRequestID] = requestID
	}
	if responseID != "" {
		result[ExtraKeyResponseID] = responseID
	}
	if model != "" {
		result[ExtraKeyModel] = model
	}
	if created != 0 {
		result[ExtraKeyCreated] = created
	}
	if serviceTier != "" {
		result[ExtraKeyServiceTier] = serviceTier
	}
	if fingerprint != "" {
		result[ExtraKeySystemFingerprint] = fingerprint
	}
	return result
}

func responseRequestID(response *http.Response) string {
	if response == nil {
		return ""
	}
	for _, name := range []string{"x-request-id", "openai-request-id", "request-id", "x-ms-request-id"} {
		if value := strings.TrimSpace(response.Header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
