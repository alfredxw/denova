package anthropicmessages

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	agent "github.com/alfredxw/denova/agent"
)

// Generate deliberately consumes the streaming endpoint. The Anthropic SDK's
// non-streaming helper injects a model-dependent timeout; Denova leaves model
// execution unbounded except for caller context cancellation.
func (model *ChatModel) Generate(ctx context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	opts = agent.BindContextSessionKey(ctx, model.options, opts...)
	params, requestOptions, err := model.request(input, opts...)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	requestOptions = append(requestOptions, option.WithResponseInto(&rawResponse))
	stream := model.client.Messages.NewStreaming(ctx, params, requestOptions...)
	defer stream.Close()
	accumulator := &anthropic.Message{}
	for stream.Next() {
		if err := accumulator.Accumulate(stream.Current()); err != nil {
			return nil, fmt.Errorf("anthropic messages stream accumulation: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, modelCallError(ctx, err)
	}
	return responseMessage(accumulator, rawResponse, model.config)
}

func (model *ChatModel) Stream(ctx context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	opts = agent.BindContextSessionKey(ctx, model.options, opts...)
	params, requestOptions, err := model.request(input, opts...)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	requestOptions = append(requestOptions, option.WithResponseInto(&rawResponse))
	providerStream := model.client.Messages.NewStreaming(ctx, params, requestOptions...)
	if err := providerStream.Err(); err != nil {
		_ = providerStream.Close()
		return nil, modelCallError(ctx, err)
	}
	reader, writer := agent.Pipe[*agent.Message](1)
	go func() {
		defer writer.Close()
		defer providerStream.Close()
		defer func() {
			if value := recover(); value != nil {
				writer.Send(nil, &agent.PanicError{Value: value, Stack: append([]byte(nil), debug.Stack()...)})
			}
		}()
		accumulator := &anthropic.Message{}
		for providerStream.Next() {
			event := providerStream.Current()
			if err := accumulator.Accumulate(event); err != nil {
				writer.Send(nil, fmt.Errorf("anthropic messages stream accumulation: %w", err))
				return
			}
			message, err := streamMessage(event, accumulator, rawResponse, model.config)
			if err != nil {
				writer.Send(nil, err)
				return
			}
			if message != nil && writer.Send(message, nil) {
				return
			}
		}
		if err := providerStream.Err(); err != nil {
			writer.Send(nil, modelCallError(ctx, err))
		}
	}()
	return reader, nil
}

func (model *ChatModel) WithTools(tools []*agent.ToolInfo) (agent.ToolCallingChatModel, error) {
	if model == nil {
		return nil, fmt.Errorf("anthropic messages: nil receiver")
	}
	clone := *model
	clone.options = agent.GetCommonOptions(model.options, agent.WithTools(tools))
	return &clone, nil
}

func modelCallError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return adaptAPIError(err)
}
