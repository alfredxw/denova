package compaction

import (
	"context"
	"io"

	agent "github.com/alfredxw/denova/agent"
)

type compactionForkCaptureModel struct {
	response *agent.Message
	inputs   [][]*agent.Message
	options  []*agent.Options
	streams  int
	requests int
}

func (model *compactionForkCaptureModel) Generate(_ context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	model.capture(input, opts)
	if model.response == nil {
		return nil, io.EOF
	}
	return model.response.Clone(), nil
}

func (model *compactionForkCaptureModel) Stream(_ context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	model.capture(input, opts)
	model.streams++
	if model.response == nil {
		return agent.StreamReaderFromArray([]*agent.Message{}), nil
	}
	return agent.StreamReaderFromArray([]*agent.Message{model.response.Clone()}), nil
}

func (model *compactionForkCaptureModel) capture(input []*agent.Message, opts []agent.ModelOption) {
	model.requests++
	messages := make([]*agent.Message, len(input))
	for index, message := range input {
		if message != nil {
			messages[index] = message.Clone()
		}
	}
	model.inputs = append(model.inputs, messages)
	model.options = append(model.options, agent.GetCommonOptions(nil, opts...))
}
