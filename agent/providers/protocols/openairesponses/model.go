package openairesponses

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/openai/openai-go/v3/option"

	agent "github.com/alfredxw/denova/agent"
)

func (model *ChatModel) Generate(ctx context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	params, requestOptions, err := model.request(input, opts...)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	requestOptions = append(requestOptions, option.WithResponseInto(&rawResponse))
	response, err := model.client.Responses.New(ctx, params, requestOptions...)
	if err != nil {
		return nil, modelCallError(ctx, err)
	}
	if err := responseFailure(response, rawResponse); err != nil {
		return nil, err
	}
	message, err := responseMessage(response, rawResponse, model.config)
	if err != nil {
		return nil, err
	}
	return message, nil
}

func (model *ChatModel) Stream(ctx context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	params, requestOptions, err := model.request(input, opts...)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	requestOptions = append(requestOptions, option.WithResponseInto(&rawResponse))
	providerStream := model.client.Responses.NewStreaming(ctx, params, requestOptions...)
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

		state := newStreamState(rawResponse, model.config)
		for providerStream.Next() {
			message, eventErr := state.convert(providerStream.Current())
			if eventErr != nil {
				writer.Send(nil, eventErr)
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
		return nil, fmt.Errorf("openai responses: nil receiver")
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
