package googlegenerativeai

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

const (
	extraKeyRequestID  = "google-request-id"
	extraKeyProvider   = "provider"
	extraKeyProtocol   = "protocol"
	extraKeyResponseID = "response_id"
	extraKeyModel      = "model"
)

func responseMessage(response *genai.GenerateContentResponse, config providers.ModelConfig) (*agent.Message, error) {
	if response == nil {
		return nil, fmt.Errorf("google generative AI: empty response")
	}
	candidate := firstCandidate(response)
	if candidate == nil || candidate.Content == nil {
		if response.PromptFeedback != nil && response.PromptFeedback.BlockReasonMessage != "" {
			return nil, fmt.Errorf("google generative AI: %s", response.PromptFeedback.BlockReasonMessage)
		}
		return nil, fmt.Errorf("google generative AI: response has no candidate")
	}
	message := &agent.Message{
		Role: agent.Assistant,
		ResponseMeta: &agent.ResponseMeta{
			FinishReason: string(candidate.FinishReason),
			Usage:        responseUsage(response.UsageMetadata),
		},
		Extra: responseExtra(response, config),
	}
	var content strings.Builder
	var reasoning strings.Builder
	toolIndex := 0
	for _, part := range candidate.Content.Parts {
		if part == nil {
			continue
		}
		if part.Thought {
			reasoning.WriteString(part.Text)
		} else if part.Text != "" {
			content.WriteString(part.Text)
		}
		if part.FunctionCall != nil {
			arguments, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				return nil, fmt.Errorf("google generative AI: encode tool call %q: %w", part.FunctionCall.Name, err)
			}
			index := toolIndex
			toolIndex++
			message.ToolCalls = append(message.ToolCalls, agent.ToolCall{
				Index: &index,
				ID:    part.FunctionCall.ID,
				Type:  "function",
				Function: agent.FunctionCall{
					Name:      part.FunctionCall.Name,
					Arguments: string(arguments),
				},
			})
		}
	}
	message.Content = content.String()
	message.ReasoningContent = reasoning.String()
	continuation, err := responseContinuation(candidate.Content.Parts, config)
	if err != nil {
		return nil, err
	}
	if continuation != nil {
		message.Extra[providers.ExtraKeyContinuation] = continuation
	}
	return message, nil
}

func firstCandidate(response *genai.GenerateContentResponse) *genai.Candidate {
	if response == nil {
		return nil
	}
	for _, candidate := range response.Candidates {
		if candidate != nil && candidate.Index == 0 {
			return candidate
		}
	}
	if len(response.Candidates) != 0 {
		return response.Candidates[0]
	}
	return nil
}

func responseContinuation(parts []*genai.Part, config providers.ModelConfig) (any, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	return providers.NewContinuation(config, parts)
}

func responseUsage(usage *genai.GenerateContentResponseUsageMetadata) *agent.TokenUsage {
	if usage == nil {
		return nil
	}
	return &agent.TokenUsage{
		PromptTokens:     int(usage.PromptTokenCount),
		CompletionTokens: int(usage.CandidatesTokenCount + usage.ThoughtsTokenCount),
		TotalTokens:      int(usage.TotalTokenCount),
		PromptTokenDetails: agent.PromptTokenDetails{
			CachedTokens: int(usage.CachedContentTokenCount),
		},
		CompletionTokensDetails: agent.CompletionTokensDetails{
			ReasoningTokens: int(usage.ThoughtsTokenCount),
		},
	}
}

func responseExtra(response *genai.GenerateContentResponse, config providers.ModelConfig) map[string]any {
	result := map[string]any{
		extraKeyProvider: string(config.Provider),
		extraKeyProtocol: string(providers.ProtocolGoogleGenerativeAI),
	}
	if response == nil {
		return result
	}
	if response.ResponseID != "" {
		result[extraKeyResponseID] = response.ResponseID
	}
	if response.ModelVersion != "" {
		result[extraKeyModel] = response.ModelVersion
	}
	if response.SDKHTTPResponse != nil {
		for _, name := range []string{"x-request-id", "request-id", "x-goog-request-id"} {
			if value := strings.TrimSpace(response.SDKHTTPResponse.Headers.Get(name)); value != "" {
				result[extraKeyRequestID] = value
				break
			}
		}
	}
	return result
}

type streamState struct {
	parts         []*genai.Part
	lastResponse  *genai.GenerateContentResponse
	nextToolIndex int
	sentMetadata  bool
}

func (state *streamState) convert(response *genai.GenerateContentResponse, config providers.ModelConfig) ([]*agent.Message, error) {
	if response == nil {
		return nil, nil
	}
	state.lastResponse = response
	candidate := firstCandidate(response)
	result := make([]*agent.Message, 0)
	if !state.sentMetadata {
		result = append(result, &agent.Message{Role: agent.Assistant, Extra: responseExtra(response, config)})
		state.sentMetadata = true
	}
	if candidate != nil && candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			if part == nil {
				continue
			}
			copy := *part
			state.parts = append(state.parts, &copy)
			message := &agent.Message{Role: agent.Assistant}
			if part.Thought {
				message.ReasoningContent = part.Text
			} else {
				message.Content = part.Text
			}
			if part.FunctionCall != nil {
				arguments, err := json.Marshal(part.FunctionCall.Args)
				if err != nil {
					return nil, fmt.Errorf("google generative AI stream: encode tool call %q: %w", part.FunctionCall.Name, err)
				}
				index := state.nextToolIndex
				state.nextToolIndex++
				message.ToolCalls = []agent.ToolCall{{
					Index: &index,
					ID:    part.FunctionCall.ID,
					Type:  "function",
					Function: agent.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(arguments),
					},
				}}
			}
			if message.Content != "" || message.ReasoningContent != "" || len(message.ToolCalls) != 0 {
				result = append(result, message)
			}
		}
		if candidate.FinishReason != "" || response.UsageMetadata != nil {
			result = append(result, &agent.Message{
				Role: agent.Assistant,
				ResponseMeta: &agent.ResponseMeta{
					FinishReason: string(candidate.FinishReason),
					Usage:        responseUsage(response.UsageMetadata),
				},
			})
		}
	}
	return result, nil
}

func (state *streamState) finish(config providers.ModelConfig) (*agent.Message, error) {
	continuation, err := responseContinuation(state.parts, config)
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
}
