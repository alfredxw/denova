package openai

import (
	"encoding/json"
	"net/http"
	"strings"

	sdk "github.com/openai/openai-go/v3"

	"github.com/alfredxw/denova/adk"
)

const (
	// ExtraKeyRequestID retains the key used by Denova's existing request-ID
	// tracing while storing only a plain string.
	ExtraKeyRequestID         = "openai-request-id"
	ExtraKeyProvider          = "provider"
	ExtraKeyResponseID        = "response_id"
	ExtraKeyModel             = "model"
	ExtraKeyCreated           = "created"
	ExtraKeyServiceTier       = "service_tier"
	ExtraKeySystemFingerprint = "system_fingerprint"
)

func responseMessage(response *sdk.ChatCompletion, rawResponse *http.Response) *adk.Message {
	choice, found := responseChoice(response.Choices)
	if !found {
		return nil
	}
	message := &adk.Message{
		Role:             responseRole(string(choice.Message.Role)),
		Content:          choice.Message.Content,
		ReasoningContent: rawReasoningContent(choice.Message.RawJSON()),
		ToolCalls:        responseToolCalls(choice.Message.ToolCalls),
		ResponseMeta: &adk.ResponseMeta{
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

func streamMessage(chunk sdk.ChatCompletionChunk, rawResponse *http.Response, includeMetadata bool) (*adk.Message, bool) {
	choice, hasChoice := streamChoice(chunk.Choices)
	hasUsage := chunk.JSON.Usage.Valid()
	if !hasChoice && !hasUsage {
		return nil, false
	}

	message := &adk.Message{Role: adk.Assistant}
	if hasChoice {
		message.Role = responseRole(choice.Delta.Role)
		message.Content = choice.Delta.Content
		message.ReasoningContent = rawReasoningContent(choice.Delta.RawJSON())
		message.ToolCalls = streamToolCalls(choice.Delta.ToolCalls)
		if choice.FinishReason != "" {
			message.ResponseMeta = &adk.ResponseMeta{FinishReason: choice.FinishReason}
		}
	}
	if hasUsage {
		if message.ResponseMeta == nil {
			message.ResponseMeta = &adk.ResponseMeta{}
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

func responseRole(role string) adk.RoleType {
	if role == "" {
		return adk.Assistant
	}
	return adk.RoleType(role)
}

func responseToolCalls(calls []sdk.ChatCompletionMessageToolCallUnion) []adk.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]adk.ToolCall, 0, len(calls))
	for _, call := range calls {
		callType := call.Type
		if callType == "" {
			callType = "function"
		}
		result = append(result, adk.ToolCall{
			ID:   call.ID,
			Type: callType,
			Function: adk.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return result
}

func streamToolCalls(calls []sdk.ChatCompletionChunkChoiceDeltaToolCall) []adk.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]adk.ToolCall, 0, len(calls))
	for _, call := range calls {
		index := int(call.Index)
		result = append(result, adk.ToolCall{
			Index: &index,
			ID:    call.ID,
			Type:  call.Type,
			Function: adk.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return result
}

func responseUsage(usage sdk.CompletionUsage) *adk.TokenUsage {
	return &adk.TokenUsage{
		PromptTokens:     int(usage.PromptTokens),
		CompletionTokens: int(usage.CompletionTokens),
		TotalTokens:      int(usage.TotalTokens),
		PromptTokenDetails: adk.PromptTokenDetails{
			CachedTokens: int(usage.PromptTokensDetails.CachedTokens),
		},
		CompletionTokensDetails: adk.CompletionTokensDetails{
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
