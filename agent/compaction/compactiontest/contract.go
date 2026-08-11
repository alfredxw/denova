// Package compactiontest provides reusable behavioral checks for Compaction
// Manager implementations.
package compactiontest

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type Factory func(testing.TB) agent.CompactionManager

func RunManagerContract(t *testing.T, factory Factory) {
	t.Helper()
	manager := factory(t)
	identity := manager.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		t.Fatalf("identity = %#v", identity)
	}
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("old request ", 32)),
		agent.AssistantMessage(strings.Repeat("old answer ", 32), nil),
		agent.UserMessage("recent request"),
	}
	modelRequest := append([]*agent.Message{agent.SystemMessage("stable instructions")}, messages...)
	plan, err := manager.Plan(context.Background(), agent.CompactionPlanRequest{
		Messages: clone(messages), ModelRequest: clone(modelRequest), Force: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch plan.Action {
	case agent.CompactionNone:
		return
	case agent.CompactionCreate:
		if plan.SourceFrom < 0 || plan.SourceTo <= plan.SourceFrom || plan.SourceTo > len(messages) {
			t.Fatalf("plan = %#v", plan)
		}
	default:
		t.Fatalf("unsupported action %q", plan.Action)
	}
	checkpoint, err := manager.Compact(context.Background(), agent.CompactionCompactRequest{
		Messages: clone(messages), ModelRequest: clone(modelRequest), Plan: plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(checkpoint.Summary) == "" || checkpoint.TokenEstimate < 0 {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
}

func clone(messages []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = message.Clone()
	}
	return result
}
