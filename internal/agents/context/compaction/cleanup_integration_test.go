package compaction_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type cleanupCompactionModel struct{ inputs [][]*agent.Message }

func (model *cleanupCompactionModel) Generate(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	cloned := make([]*agent.Message, len(messages))
	for index, message := range messages {
		cloned[index] = message.Clone()
	}
	model.inputs = append(model.inputs, cloned)
	return agent.AssistantMessage("A concise checkpoint.", nil), nil
}

func (model *cleanupCompactionModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.Generate(ctx, messages, options...)
	return agent.StreamReaderFromArray([]*agent.Message{message}), err
}

type compactionCleanupFixture struct{ action agent.CleanupAction }

func (compactionCleanupFixture) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "test.compaction-cleanup", Version: 1}
}

func (fixture compactionCleanupFixture) Plan(_ context.Context, request agent.CleanupPlanRequest) (agent.CleanupPlan, error) {
	for index, message := range request.ModelRequest {
		if message.Role == agent.ToolRole && strings.Contains(message.Content, "OLD_TOOL_BODY") {
			return agent.CleanupPlan{
				Action: fixture.action, Reason: "test cleanup", Renderer: "test.cleanup.v1",
				Metrics: agent.CleanupMetrics{PressureBefore: 1.1},
				Replacements: []agent.CleanupReplacement{{MessageIndex: index, ToolCallID: message.ToolCallID,
					Placeholder: "[Older tool result removed; use read on chapter.md.]"}},
			}, nil
		}
	}
	return agent.CleanupPlan{Action: agent.CleanupNone}, nil
}

// Only the trigger is controlled; planning, source projection, summary forks,
// validation, and checkpoint persistence use the production implementation.
type cleanupCompactionFixture struct {
	agent.CompactionManager
	force    bool
	requests []agent.CompactionCompactRequest
}

func (fixture *cleanupCompactionFixture) Plan(ctx context.Context, request agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	request.Force = request.Force || fixture.force
	return fixture.CompactionManager.Plan(ctx, request)
}

func (fixture *cleanupCompactionFixture) Compact(ctx context.Context, request agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	fixture.requests = append(fixture.requests, request)
	return fixture.CompactionManager.Compact(ctx, request)
}

func TestSessionCompactionAfterCleanup(t *testing.T) {
	for _, scenario := range []string{"manual", "automatic", "transient_overflow"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			cfg := &config.Config{OpenAIContextWindowTokens: 128_000}
			model := &cleanupCompactionModel{}
			manager, err := agentcompaction.NewAgentManager(cfg, config.AgentKindIDE, model,
				agent.CapabilityIdentity{Kind: "test.compaction-cleanup-model", Version: 1})
			if err != nil {
				t.Fatal(err)
			}
			cleanup := compactionCleanupFixture{action: agent.CleanupProject}
			compaction := &cleanupCompactionFixture{CompactionManager: manager}
			if scenario == "transient_overflow" {
				cleanup.action = agent.CleanupCompact
				compaction.force = true
			}
			store := agentsession.Memory()
			owner, err := agent.New(ctx, agent.Definition{
				Name: "writing", Model: model, Cleanup: cleanup, Compaction: compaction,
				Middlewares: []agent.Middleware{agentchat.NewModelHistoryProjectionMiddleware(toolresult.ResolveContextPolicy(cfg, config.AgentKindIDE))},
			}, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.Close(ctx) })
			session, err := owner.Session(ctx, agent.NamedSession("cleanup-compaction"))
			if err != nil {
				t.Fatal(err)
			}
			raw := []*agent.Message{
				agent.UserMessage(strings.Repeat("Remember the old chapter. ", 1500)),
				agent.AssistantMessage("", []agent.ToolCall{{ID: "read-old", Type: "function",
					Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"chapter.md"}`}}}),
				agent.ToolMessage(agent.TextToolResult(strings.Repeat("OLD_TOOL_BODY ", 1000)), "read-old", agent.WithToolName("read")),
				agent.AssistantMessage("The old chapter is reviewed.", nil),
				agent.UserMessage("More recent request"), agent.AssistantMessage("More recent answer", nil),
			}
			if err := session.LoadCanonicalMessages(ctx, raw); err != nil {
				t.Fatal(err)
			}
			runTurn := func(text string) {
				t.Helper()
				run, err := session.Run(ctx, agent.Text(text))
				if err != nil {
					t.Fatal(err)
				}
				if result, err := run.Wait(ctx); err != nil || result.Status != agent.ResultCompleted {
					t.Fatalf("run: result=%+v error=%v", result, err)
				}
			}
			runTurn("Continue after cleanup")
			if scenario != "transient_overflow" {
				if cleanup, present, err := session.Cleanup(ctx); err != nil || !present || len(cleanup.Replacements) != 1 {
					t.Fatalf("cleanup state: %+v present=%v error=%v", cleanup, present, err)
				}
				if scenario == "manual" {
					result, err := session.Compact(ctx, agent.CompactionRequest{Force: true})
					if err != nil || !result.Changed {
						t.Fatalf("compaction after cleanup: changed=%v error=%v", result.Changed, err)
					}
				} else {
					compaction.force = true
					runTurn("Continue with automatic compaction")
				}
			}
			snapshot, err := session.Snapshot(ctx)
			if err != nil || snapshot.Compaction == nil || snapshot.Cleanup != nil {
				t.Fatalf("checkpoint did not absorb Cleanup: %+v error=%v", snapshot, err)
			}
			if len(compaction.requests) != 1 {
				t.Fatalf("summary requests = %d, want 1", len(compaction.requests))
			}
			request := compaction.requests[0]
			if !reflect.DeepEqual(request.Messages[:len(raw)], raw) {
				t.Fatal("Cleanup modified canonical history")
			}
			encoded, err := json.Marshal(request.Messages[request.Plan.SourceFrom:request.Plan.SourceTo])
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(encoded)
			if snapshot.Compaction.SourceHash != hex.EncodeToString(hash[:]) {
				t.Fatal("checkpoint source hash no longer authenticates raw history")
			}
			var fork []*agent.Message
			for _, input := range model.inputs {
				if strings.HasPrefix(input[len(input)-1].Content, "[Denova runtime context compaction request]") {
					fork = input
				}
			}
			primary := request.ModelSnapshot.Messages()
			if len(fork) != len(primary)+1 || !reflect.DeepEqual(fork[:len(primary)], primary) {
				t.Fatal("compaction did not preserve the exact primary request prefix")
			}
			if request.ContextMessages[2].Content != "[Older tool result removed; use read on chapter.md.]" {
				t.Fatal("compaction context did not carry Cleanup in raw history positions")
			}
			for _, message := range fork {
				if strings.Contains(message.Content, "OLD_TOOL_BODY") {
					t.Fatal("compaction resurrected a cleaned tool body")
				}
			}
			compaction.force = false
			removed, err := session.RemoveCompaction(ctx, agent.CompactionRemoveRequest{ID: snapshot.Compaction.ID})
			if err != nil || !removed {
				t.Fatalf("remove checkpoint: removed=%v error=%v", removed, err)
			}
			if err := owner.Close(ctx); err != nil {
				t.Fatal(err)
			}
			// Reopening without Cleanup observes the raw history restored by removal.
			owner, err = agent.New(ctx, agent.Definition{Name: "writing", Model: model}, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			session, err = owner.Session(ctx, agent.NamedSession("cleanup-compaction"))
			if err != nil {
				t.Fatal(err)
			}
			runTurn("Read the full original history again")
			if model.inputs[len(model.inputs)-1][2].Content != raw[2].Content {
				t.Fatal("checkpoint removal and reopen lost the original tool body")
			}
		})
	}
}
