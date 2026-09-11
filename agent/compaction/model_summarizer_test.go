package compaction

import (
	"context"
	"errors"
	agent "github.com/alfredxw/denova/agent"
	"reflect"
	"strings"
	"testing"
)

type summaryCaptureModel struct {
	inputs   [][]*agent.Message
	options  []*agent.Options
	response *agent.Message
	err      error
}

func (m *summaryCaptureModel) Generate(_ context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	m.inputs = append(m.inputs, cloneMessages(messages))
	m.options = append(m.options, agent.GetCommonOptions(nil, options...))
	return m.response, m.err
}
func (m *summaryCaptureModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	result, err := m.Generate(ctx, messages, options...)
	return agent.StreamReaderFromArray([]*agent.Message{result}), err
}
func TestBuiltinSummaryUsesSnapshotAndNeverFallsBackAfterProviderFailure(t *testing.T) {
	for _, failure := range []string{"none", "provider", "tool", "oversized", "mismatch"} {
		t.Run(failure, func(t *testing.T) {
			model := &summaryCaptureModel{response: agent.AssistantMessage("Checkpoint.", nil)}
			source := []*agent.Message{agent.UserMessage("original task"), agent.AssistantMessage("completed work", nil)}
			primary := append([]*agent.Message{agent.SystemMessage("stable system")}, source...)
			primary = append(primary, agent.UserMessage("latest"))
			options := []agent.ModelOption{agent.WithTools([]*agent.ToolInfo{{Name: "read"}}), agent.WithSessionKey("stable-cache"), agent.WithToolChoice(agent.ToolChoiceAllowed), agent.WithMaxTokens(12000)}
			snapshot := (&agent.ModelCall{Model: model, Messages: primary, Options: options}).Snapshot()
			switch failure {
			case "provider":
				model.err = errors.New("provider failed")
			case "tool":
				model.response = agent.AssistantMessage("", []agent.ToolCall{{ID: "unexpected", Function: agent.FunctionCall{Name: "read"}}})
			case "oversized":
				model.response = agent.AssistantMessage(strings.Repeat("too large ", 1000), nil)
			case "mismatch":
				source = []*agent.Message{agent.UserMessage("hidden source")}
			}
			summarizer, err := ModelSummarizer(ModelSummarizerConfig{})
			if err != nil {
				t.Fatal(err)
			}
			result, err := summarizer.Summarize(t.Context(), SummaryRequest{Messages: source, ModelSnapshot: snapshot, ContextWindowTokens: 16000, SummaryLimitBytes: 4096, HardLimitBytes: 1 << 20})
			if failure == "none" {
				if err != nil || result.Summary != "Checkpoint." {
					t.Fatalf("result=%#v err=%v", result, err)
				}
			} else if err == nil {
				t.Fatal("invalid checkpoint accepted")
			}
			if failure == "mismatch" {
				if len(model.inputs) != 0 {
					t.Fatal("hidden source sent to model")
				}
				return
			}
			if len(model.inputs) != 1 || !reflect.DeepEqual(model.inputs[0][:len(primary)], primary) {
				t.Fatal("fork changed prefix or retried as cold call")
			}
			if !reflect.DeepEqual(model.options[0].Tools, snapshot.ResolvedOptions().Tools) || model.options[0].SessionKey != snapshot.ResolvedOptions().SessionKey {
				t.Fatal("fork changed captured options")
			}
			if !reflect.DeepEqual(snapshot.Messages(), primary) {
				t.Fatal("primary snapshot mutated")
			}
		})
	}
}
