package openai

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/openai/openai-go/v3/option"

	"github.com/alfredxw/denova/adk"
)

// Generate returns choice index zero from one Chat Completions response.
func (model *ChatModel) Generate(ctx context.Context, input []*adk.Message, opts ...adk.ModelOption) (*adk.Message, error) {
	params, requestOptions, err := model.request(input, false, opts...)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	requestOptions = append(requestOptions, option.WithResponseInto(&rawResponse))
	response, err := model.client.Chat.Completions.New(ctx, params, requestOptions...)
	if err != nil {
		return nil, modelCallError(ctx, err)
	}
	message := responseMessage(response, rawResponse)
	if message == nil {
		return nil, fmt.Errorf("openai chat completion: response has no choice with index 0")
	}
	return message, nil
}

// Stream converts the SDK stream directly into a capacity-one ADK pipe. The
// producer closes both streams and recovers panics so no provider goroutine can
// terminate the process.
func (model *ChatModel) Stream(ctx context.Context, input []*adk.Message, opts ...adk.ModelOption) (*adk.StreamReader[*adk.Message], error) {
	params, requestOptions, err := model.request(input, true, opts...)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	requestOptions = append(requestOptions, option.WithResponseInto(&rawResponse))
	providerStream := model.client.Chat.Completions.NewStreaming(ctx, params, requestOptions...)
	if err := providerStream.Err(); err != nil {
		_ = providerStream.Close()
		return nil, modelCallError(ctx, err)
	}

	reader, writer := adk.Pipe[*adk.Message](1)
	go func() {
		defer writer.Close()
		defer providerStream.Close()
		defer func() {
			if value := recover(); value != nil {
				writer.Send(nil, &adk.PanicError{Value: value, Stack: append([]byte(nil), debug.Stack()...)})
			}
		}()

		metadataPending := true
		for providerStream.Next() {
			message, emit := streamMessage(providerStream.Current(), rawResponse, metadataPending)
			if !emit {
				continue
			}
			metadataPending = false
			if writer.Send(message, nil) {
				return
			}
		}
		if err := providerStream.Err(); err != nil {
			writer.Send(nil, modelCallError(ctx, err))
		}
	}()
	return reader, nil
}

// WithTools returns a detached model; neither the receiver nor the caller's
// tool definitions can be mutated through the returned instance.
func (model *ChatModel) WithTools(tools []*adk.ToolInfo) (adk.ToolCallingChatModel, error) {
	if model == nil {
		return nil, fmt.Errorf("openai chat model: nil receiver")
	}
	clone := *model
	clone.options = adk.GetCommonOptions(model.options, adk.WithTools(tools))
	return &clone, nil
}

func modelCallError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return adaptAPIError(err)
}
