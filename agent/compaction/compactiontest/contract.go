// Package compactiontest provides reusable behavioral checks for Compaction
// Manager implementations.
package compactiontest

import (
	"context"
	"reflect"
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
	if manager.SummaryLimitBytes() <= 0 {
		t.Fatalf("summary limit = %d", manager.SummaryLimitBytes())
	}
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("old request ", 32)),
		agent.AssistantMessage(strings.Repeat("old answer ", 32), nil),
		agent.UserMessage("recent request"),
	}
	modelRequest := append([]*agent.Message{agent.SystemMessage("stable instructions")}, messages...)
	modelSnapshot := (&agent.ModelCall{
		Messages: modelRequest, Options: []agent.ModelOption{agent.WithSessionKey("contract-session")}, Streaming: true,
	}).Snapshot()
	plan, err := manager.Plan(context.Background(), agent.CompactionPlanRequest{
		Messages: clone(messages), ModelRequest: clone(modelRequest), ModelSnapshot: modelSnapshot, Force: true,
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
	sourceMessages := clone(messages[plan.SourceFrom:plan.SourceTo])
	probe := &contractManager{CompactionManager: manager}
	checkpoint, err := probe.Compact(context.Background(), agent.CompactionCompactRequest{
		Messages: clone(messages), ModelRequest: clone(modelRequest), SourceMessages: sourceMessages,
		ModelSnapshot: modelSnapshot, Plan: plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(checkpoint.Summary) == "" || checkpoint.TokenEstimate < 0 || len(checkpoint.Summary) > manager.SummaryLimitBytes() {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
	if probe.request.ModelSnapshot != modelSnapshot ||
		!reflect.DeepEqual(probe.request.SourceMessages, sourceMessages) {
		t.Fatalf("Compaction contract lost exact ModelSnapshot or SourceMessages: %#v", probe.request)
	}
}

type contractManager struct {
	agent.CompactionManager
	request agent.CompactionCompactRequest
}

func (manager *contractManager) Compact(ctx context.Context, request agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	manager.request = request
	return manager.CompactionManager.Compact(ctx, request)
}

func clone(messages []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = message.Clone()
	}
	return result
}
