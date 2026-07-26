package tools

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type contextWindowTestController struct {
	checkpoint agent.ContextCheckpointRequest
	rewind     agent.ContextRewindRequest
}

func (c *contextWindowTestController) BeforeModel(_ context.Context, messages []*agent.Message) ([]*agent.Message, error) {
	return messages, nil
}
func (c *contextWindowTestController) BeforeComplete(_ context.Context, messages []*agent.Message) ([]*agent.Message, bool, error) {
	return messages, false, nil
}
func (c *contextWindowTestController) Checkpoint(_ context.Context, request agent.ContextCheckpointRequest) (agent.ContextCheckpointResult, error) {
	c.checkpoint = request
	return agent.ContextCheckpointResult{ID: "cp-1", Purpose: request.Purpose, Staged: true}, nil
}
func (c *contextWindowTestController) Rewind(_ context.Context, request agent.ContextRewindRequest) (agent.ContextRewindResult, error) {
	c.rewind = request
	return agent.ContextRewindResult{CheckpointID: "cp-1", Staged: true}, nil
}
func (*contextWindowTestController) ObserveTool(context.Context, agent.ContextToolObservation) {}

func TestContextWindowToolsUseRunController(t *testing.T) {
	controller := &contextWindowTestController{}
	ctx := agent.ContextWithContextWindowController(context.Background(), controller)
	checkpoint, err := Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checkpoint.Run(ctx, `{"purpose":"research"}`); err != nil {
		t.Fatal(err)
	}
	if controller.checkpoint.Purpose != "research" {
		t.Fatalf("checkpoint request = %#v", controller.checkpoint)
	}
	rewind, err := Rewind()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rewind.Run(ctx, `{"checkpoint_id":"cp-1","report":"answer"}`); err != nil {
		t.Fatal(err)
	}
	if controller.rewind.CheckpointID != "cp-1" || controller.rewind.Report != "answer" {
		t.Fatalf("rewind request = %#v", controller.rewind)
	}
}
