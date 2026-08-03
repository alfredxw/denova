package googlegenerativeai

import (
	"context"
	"fmt"
	"runtime/debug"

	agent "github.com/alfredxw/denova/agent"
)

func (model *ChatModel) Generate(ctx context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	contents, config, err := model.request(input, opts...)
	if err != nil {
		return nil, err
	}
	response, err := model.client.Models.GenerateContent(ctx, model.config.Model, contents, config)
	if err != nil {
		return nil, modelCallError(ctx, err)
	}
	return responseMessage(response, model.config)
}

func (model *ChatModel) Stream(ctx context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	contents, config, err := model.request(input, opts...)
	if err != nil {
		return nil, err
	}
	sequence := model.client.Models.GenerateContentStream(ctx, model.config.Model, contents, config)
	reader, writer := agent.Pipe[*agent.Message](1)
	go func() {
		defer writer.Close()
		defer func() {
			if value := recover(); value != nil {
				writer.Send(nil, &agent.PanicError{Value: value, Stack: append([]byte(nil), debug.Stack()...)})
			}
		}()
		state := &streamState{}
		for response, streamErr := range sequence {
			if streamErr != nil {
				writer.Send(nil, modelCallError(ctx, streamErr))
				return
			}
			messages, err := state.convert(response, model.config)
			if err != nil {
				writer.Send(nil, err)
				return
			}
			for _, message := range messages {
				if writer.Send(message, nil) {
					return
				}
			}
		}
		message, err := state.finish(model.config)
		if err != nil {
			writer.Send(nil, err)
			return
		}
		if message != nil {
			writer.Send(message, nil)
		}
	}()
	return reader, nil
}

func (model *ChatModel) WithTools(tools []*agent.ToolInfo) (agent.ToolCallingChatModel, error) {
	if model == nil {
		return nil, fmt.Errorf("google generative AI: nil receiver")
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
