package chat

import (
	"encoding/json"
	"testing"
	"time"

	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

type publicEventCompactionConversation struct {
	Conversation
	state *agent.CompactionState
}

func (conversation *publicEventCompactionConversation) BindAgentCompaction(state *agent.CompactionState) error {
	conversation.state = state
	return nil
}

func TestPublicEventProjectorPreservesUsageAndStructuredToolDisplay(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", ProjectID: "project", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) {
		events = append(events, event)
	})
	projector.Project(agent.Event{RunID: "run", Payload: agent.AssistantDelta{Delta: "draft"}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ModelCompleted{
		Usage: agent.TokenUsage{
			PromptTokens: 100, PromptTokenDetails: agent.PromptTokenDetails{CachedTokens: 75},
			CompletionTokens: 20, TotalTokens: 120,
		},
		FinishReason: "tool_calls", RequestedTools: []string{"write"},
	}})
	receipt := json.RawMessage(`{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace","change_group_id":"group","review_thread_id":"review-run","change_set_id":"change","path":"chapters/one.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`)
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolFinished{
		CallID: "call", Name: "write", Result: "written",
		Projection: &agent.ToolResult{
			DisplayContent: "written", ModelContent: "written", Details: receipt,
			Status: agent.ToolResultSuccess,
			Metadata: agent.ToolResultMetadata{
				OriginalModelBytes: 20, ReturnedModelBytes: 7, Target: "chapters/one.md",
			},
			ResultRetention: agent.ToolResultProtected,
		},
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ModelCompleted{
		Usage: agent.TokenUsage{
			PromptTokens: 80, PromptTokenDetails: agent.PromptTokenDetails{CachedTokens: 40},
			CompletionTokens: 10, TotalTokens: 90,
		},
		FinishReason: "stop",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.RunSettled{Status: agent.ResultCompleted}})
	projector.Finalize(agent.ResultCompleted, "")

	indexes := map[string]int{}
	for index, event := range events {
		indexes[event.Type] = index
	}
	if indexes["workspace_change"] == 0 || indexes["tool_result"] == 0 || indexes["token_usage"] == 0 || indexes["done"] == 0 {
		t.Fatalf("projected event types = %#v", events)
	}
	if indexes["workspace_change"] > indexes["tool_result"] || indexes["token_usage"] > indexes["done"] {
		t.Fatalf("event ordering = %#v", events)
	}
	workspaceChange := events[indexes["workspace_change"]]
	if workspaceChange.DataString("id") != "change" || workspaceChange.DataString("project_id") != "project" ||
		workspaceChange.DataString("run_id") != "run" || workspaceChange.DataString("workspace") != "/workspace" ||
		workspaceChange.DataString("change_group_id") != "group" || workspaceChange.DataString("review_thread_id") != "review-run" ||
		workspaceChange.DataString("path") != "chapters/one.md" || workspaceChange.DataString("base_revision") != "sha256:before" ||
		workspaceChange.DataString("revision") != "sha256:after" || workspaceChange.DataString("review_status") != "pending" ||
		workspaceChange.DataString("apply_state") != "applied" {
		t.Fatalf("workspace change = %#v", workspaceChange.Data)
	}
	usage := events[indexes["token_usage"]]
	if usage.DataString("run_id") != "run" || eventDataInt(usage.Data, "model_calls") != 2 ||
		eventDataInt(usage.Data, "prompt_tokens") != 180 || eventDataInt(usage.Data, "cached_prompt_tokens") != 115 {
		t.Fatalf("token usage = %#v", usage.Data)
	}
	result := events[indexes["tool_result"]]
	if result.DataString("target") != "chapters/one.md" || result.DataString("status") != "success" {
		t.Fatalf("tool result = %#v", result.Data)
	}
}

func TestPublicEventProjectorStreamsToolInputWithoutDuplicatingExecutionStart(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	descriptor := &agent.ToolDescriptor{
		Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: agent.ToolResultDeferred, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 4096,
		Presentation: agent.UniformToolPresentation(agent.ToolPresentationSearch),
	}
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolInputStarted{
		CallID: "execution-call", ProviderCallID: "provider-call", ParentCallID: "script-call",
		Name: "read", Index: 2, Descriptor: descriptor,
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolInputDelta{
		CallID: "execution-call", ProviderCallID: "provider-call", Name: "read", Delta: `{"path":"`,
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolInputDelta{
		CallID: "execution-call", ProviderCallID: "provider-call", Name: "read", Delta: `draft.md"}`,
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolStarted{
		CallID: "execution-call", ProviderCallID: "provider-call", Name: "read", Index: 2,
		Arguments: json.RawMessage(`{"path":"draft.md"}`), Descriptor: descriptor,
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolFinished{
		CallID: "execution-call", ProviderCallID: "provider-call", Name: "read", Index: 2,
		Result: "draft", Descriptor: descriptor,
	}})

	wantTypes := []string{"tool_call", "tool_args_delta", "tool_target", "tool_args_delta", "tool_started", "tool_result"}
	if len(events) != len(wantTypes) {
		t.Fatalf("projected events = %#v", events)
	}
	for index, event := range events {
		if event.Type != wantTypes[index] {
			t.Fatalf("projected event types = %#v, want %v", events, wantTypes)
		}
	}
	call := events[0]
	if call.DataString("id") != "execution-call" || call.DataString("provider_call_id") != "provider-call" ||
		call.DataString("name") != "read" || call.DataString("args") != "" || call.DataString("source") != "read" ||
		call.DataString("parent_call_id") != "script-call" || eventDataInt(call.Data, "index") != 2 ||
		eventDataInt(call.Data, "max_result_bytes") != 4096 {
		t.Fatalf("tool call = %#v", call.Data)
	}
	presentation := eventDataToolPresentation(call.Data)
	if presentation == nil || presentation.Call != agent.ToolPresentationSearch || presentation.Result != agent.ToolPresentationSearch {
		t.Fatalf("tool presentation = %#v, want search", presentation)
	}
	if events[1].DataString("delta")+events[3].DataString("delta") != `{"path":"draft.md"}` ||
		events[2].DataString("target") != "draft.md" || eventDataInt(events[1].Data, "index") != 2 ||
		eventDataInt(events[3].Data, "index") != 2 {
		t.Fatalf("tool argument projection = %#v", events[1:4])
	}
	if events[4].DataString("parent_call_id") != "script-call" || events[5].DataString("parent_call_id") != "script-call" {
		t.Fatalf("nested tool lifecycle lost parent identity: %#v", events[4:])
	}
}

func TestPublicEventProjectorPreservesSubAgentInvocationIdentity(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	projector.Project(agent.Event{RunID: "run", Payload: agent.NestedEvent{
		ParentCallID: "task-call", SessionID: "child-session",
		Source: agent.EventSource{
			Name: "reviewer", Path: []string{"root", "reviewer"},
			InvocationID: "invocation-2", InvocationType: "reviewer",
		},
		Child: agent.Event{RunID: "child-run", Payload: agent.AssistantDelta{Delta: "reviewed"}},
	}})
	if len(events) != 1 || events[0].Type != "chunk" {
		t.Fatalf("projected events = %#v", events)
	}
	if events[0].DataString("subagent_session_id") != "invocation-2" ||
		events[0].DataString("subagent_type") != "reviewer" || events[0].DataString("parent_call_id") != "task-call" ||
		!eventDataBool(events[0].Data, "subagent") {
		t.Fatalf("subagent metadata = %#v", events[0].Data)
	}
}

func TestPublicEventProjectorPublishesAndBindsAgentCompaction(t *testing.T) {
	conversation := &publicEventCompactionConversation{}
	var events []agentrun.Event
	projector := NewPublicEventProjector(conversation, ChatRequest{}, agentrun.Options{
		AgentKind: "interactive_story", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	state := agent.CompactionState{
		ID: "checkpoint-1", Revision: 3, Summary: "bounded story state", TokenEstimate: 42,
		ReplacementFrom: 1, ReplacementTo: 5,
		Metrics: agent.CompactionMetrics{
			ObservedPromptTokens: 900, ObservedEstimateTokens: 750,
			ProjectedTokensBefore: 1_000, ProjectedTokensAfter: 420,
			CacheExpectedPrefixTokens: 800, CacheReadTokens: 600, RecoveryBandMet: true,
		},
	}
	projector.Project(agent.Event{RunID: "run", Payload: agent.CompactionCommitted{State: state}})
	if conversation.state == nil || conversation.state.ID != state.ID || conversation.state.Revision != state.Revision {
		t.Fatalf("bound Compaction=%#v", conversation.state)
	}
	if len(events) != 1 || events[0].Type != "context_compaction" ||
		events[0].DataString("status") != "completed" || events[0].DataString("summary") != state.Summary ||
		eventDataInt(events[0].Data, "source_message_count") != 4 ||
		eventDataInt(events[0].Data, "observed_estimate_tokens") != 750 ||
		eventDataInt(events[0].Data, "projected_tokens_after") != 420 ||
		eventDataFloat(events[0].Data, "cache_hit_ratio") != .75 {
		t.Fatalf("projected Compaction events=%#v", events)
	}
	projector.Project(agent.Event{RunID: "run", Payload: agent.CompactionRemoved{ID: state.ID, Revision: 4}})
	if conversation.state != nil {
		t.Fatalf("removed Compaction remained bound: %#v", conversation.state)
	}
}

func TestPublicEventProjectorPreservesCompleteCleanupLifecycleTelemetry(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	metrics := agent.CleanupMetrics{
		EstimatedTokensBefore: 1_000, EstimatedTokensAfter: 600, ReclaimedTokens: 400,
		LocalProjectedTokens: 900, ObservedPromptTokens: 1_000, EffectiveTokens: 1_000,
		ContextWindowTokens: 1_200, PressureBefore: .83, PressureAfter: .5,
		BodyPressureBefore: .8, BodyPressureAfter: .4, StablePrefixTokens: 100,
		CandidateTokens: 500, ProtectedResults: 2, EarliestChanged: 3, WarmSuffixTokens: 90,
		CacheViableCandidateTokens: 450, SkippedWarmSuffixCount: 1,
		EagerCandidateCount: 2, EagerSelectedCount: 1, SupersededCandidateCount: 1,
		DiscardableCandidateCount: 1, MinimumCleanupTokens: 100, PlaceholderTokens: 20,
		ReplacementCount: 1, EagerOnly: true, PressureScope: "body_after_prefix",
		ProviderCacheState: "warm", ExecutionMode: "agent_projection", RendererVersion: "agent.cleanup.placeholder.v1",
	}
	projector.Project(agent.Event{RunID: "run", Payload: agent.CleanupStarted{
		ID: "cleanup-1", Reason: "cleanup_recovery_target_met", Automatic: true, Metrics: metrics,
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.CleanupCompleted{
		ID: "cleanup-1", Reason: "cleanup_recovery_target_met", Automatic: true, Metrics: metrics,
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.CleanupFailed{
		ID: "cleanup-2", Reason: "projection failed", Automatic: true, Metrics: metrics,
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.CleanupSkipped{
		ID: "cleanup-3", Reason: "cleanup_not_cost_effective", Automatic: true, Metrics: metrics,
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.CleanupCommitted{
		Automatic: true,
		State: agent.CleanupState{ID: "cleanup-1", Revision: 2, SourceStart: 0, SourceEnd: 6,
			Renderer: "agent.cleanup.placeholder.v1", Replacements: []agent.CleanupReplacement{{MessageIndex: 3}}, Metrics: metrics},
	}})
	if len(events) != 5 {
		t.Fatalf("Cleanup lifecycle event count=%d: %#v", len(events), events)
	}
	wantStatuses := []string{"started", "completed", "failed", "skipped", "committed"}
	for index, want := range wantStatuses {
		if events[index].Type != "context_cleanup" || events[index].DataString("status") != want ||
			events[index].DataString("phase") != "model_step" ||
			eventDataInt(events[index].Data, "estimated_reclaimed_tokens") != 400 ||
			eventDataInt(events[index].Data, "stable_prefix_tokens") != 100 {
			t.Fatalf("Cleanup event %d=%#v, want status=%s", index, events[index], want)
		}
	}
	if events[2].DataString("error") != "projection failed" ||
		events[3].DataString("skipped_reason") != "cleanup_not_cost_effective" ||
		eventDataInt(events[4].Data, "epoch") != 2 || eventDataInt(events[4].Data, "replacement_count") != 1 ||
		events[4].DataString("renderer") != "agent.cleanup.placeholder.v1" {
		t.Fatalf("Cleanup lifecycle details=%#v", events)
	}
	if eventDataInt(events[0].Data, "observed_prompt_tokens") != 1_000 ||
		eventDataInt(events[0].Data, "cache_viable_candidate_tokens") != 450 ||
		eventDataInt(events[0].Data, "eager_receipt_fallback_count") != 1 ||
		events[0].DataString("cleanup_execution_mode") != "agent_projection" ||
		events[0].DataString("provider_cache_state") != "warm" {
		t.Fatalf("Cleanup detailed telemetry=%#v", events[0].Data)
	}
}

func TestPublicEventProjectorProjectsBoundedContextNormalizerTelemetry(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	projector.Project(agent.Event{RunID: "run", Payload: agent.ContextNormalized{
		RepairCount: 1, MessagesBefore: 4, MessagesAfter: 5,
	}})
	if len(events) != 1 || events[0].Type != "context_normalizer" ||
		events[0].DataString("status") != "repaired" ||
		eventDataInt(events[0].Data, "context_normalizer_repair_count") != 1 ||
		eventDataInt(events[0].Data, "messages_before") != 4 ||
		eventDataInt(events[0].Data, "messages_after") != 5 {
		t.Fatalf("normalizer projection=%#v", events)
	}
}

func TestPublicEventProjectorRestoresCycleBoundaryAndUsesRunIDForPlanEvents(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{
		CommandID: "command-2", Message: "continue", PlanMode: true,
	}, agentrun.Options{
		AgentKind: "ide", TaskID: "display-task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	projector.ProjectRunStarted("run-2", 2, "command-2", "follow_up", time.Now().UTC())
	projector.Project(agent.Event{RunID: "run-2", Payload: agent.AssistantDelta{
		Source: agent.EventSource{Name: "root", Path: []string{"root"}},
		Delta:  "<proposed_plan>inspect first</proposed_plan>",
	}})
	if len(events) < 2 || events[0].Type != "agent_cycle_started" {
		t.Fatalf("projected events = %#v", events)
	}
	cycle := events[0]
	if cycle.DataString("operation_id") != "run-2" || cycle.DataString("command_id") != "command-2" ||
		cycle.DataString("delivery") != "follow_up" || cycle.DataString("message") != "continue" || eventDataInt(cycle.Data, "cycle") != 2 {
		t.Fatalf("cycle event = %#v", cycle.Data)
	}
	for _, event := range events[1:] {
		if event.Type == "proposed_plan" && event.DataString("run_id") != "run-2" {
			t.Fatalf("plan event run ID = %#v, want public Run ID", event.Data)
		}
	}
}

func TestPublicEventProjectorMapsAgentStartDeliveryToPublicStartTurn(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{Message: "start"}, agentrun.Options{}, func(event agentrun.Event) {
		events = append(events, event)
	})
	projector.ProjectRunStarted("run-start", 1, "command-start", "start", time.Now().UTC())
	if len(events) != 1 || events[0].Type != "agent_cycle_started" || events[0].DataString("delivery") != "start_turn" {
		t.Fatalf("cycle event = %#v", events)
	}
}

func TestPublicEventProjectorEmitsStableRunTimingBeforeEveryTerminalEvent(t *testing.T) {
	tests := []struct {
		status       agent.ResultStatus
		terminalType string
	}{
		{status: agent.ResultCompleted, terminalType: "done"},
		{status: agent.ResultFailed, terminalType: "error"},
		{status: agent.ResultAborted, terminalType: "aborted"},
		{status: agent.ResultIncomplete, terminalType: "error"},
	}
	for _, test := range tests {
		t.Run(string(test.status), func(t *testing.T) {
			var events []agentrun.Event
			projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{}, func(event agentrun.Event) {
				events = append(events, event)
			})
			startedAt := time.Now().UTC().Add(-1500 * time.Millisecond)
			projector.ProjectRunStarted("run-timing", 1, "command", "start", startedAt)
			projector.ProjectRunStarted("run-timing", 2, "command", "queued", startedAt.Add(time.Second))
			projector.Finalize(test.status, "terminal reason")

			if len(events) != 4 || events[0].Type != "agent_cycle_started" || events[1].Type != "agent_cycle_started" ||
				events[2].Type != "execution_summary" || events[3].Type != test.terminalType {
				t.Fatalf("event order = %#v", events)
			}
			wantStartedAt := startedAt.Format(time.RFC3339Nano)
			if events[0].DataString("run_id") != "run-timing" || events[0].DataString("run_started_at") != wantStartedAt ||
				events[1].DataString("run_started_at") != wantStartedAt {
				t.Fatalf("cycle timing = %#v / %#v", events[0].Data, events[1].Data)
			}
			summary := events[2]
			if summary.DataString("run_id") != "run-timing" || summary.DataString("run_started_at") != wantStartedAt ||
				summary.DataString("run_finished_at") == "" || summary.DataString("status") != string(test.status) ||
				eventDataInt64(summary.Data, "duration_ms") < 1400 {
				t.Fatalf("execution summary = %#v", summary.Data)
			}
		})
	}
}

func TestPublicEventProjectorSummarizesSettledRunWithoutClosingSuccessorOperation(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{}, func(event agentrun.Event) {
		events = append(events, event)
	})
	projector.ProjectRunStarted("run-first", 1, "command", "start", time.Now().UTC().Add(-time.Second))
	projector.SummarizeRun(agent.ResultCompleted)
	projector.SummarizeRun(agent.ResultCompleted)

	if len(events) != 2 || events[0].Type != "agent_cycle_started" || events[1].Type != "execution_summary" {
		t.Fatalf("settled Run events = %#v", events)
	}
}

func TestPublicEventProjectorReclassifiesInteractiveToolPreamble(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, TaskID: "task", RootAgentName: "game",
	}, func(event agentrun.Event) { events = append(events, event) })
	root := agent.EventSource{Name: "game", Path: []string{"game"}}
	projector.Project(agent.Event{RunID: "run", Payload: agent.AssistantDelta{Source: root, Delta: "I will inspect lore first."}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolInputStarted{
		Source: root, CallID: "read-1", ProviderCallID: "provider-read", Name: "read",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.AssistantDelta{Source: root, Delta: "Checking details."}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ModelCompleted{Source: root, RequestedTools: []string{"read"}}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.AssistantDelta{Source: root, Delta: "The door opened."}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolInputStarted{
		Source: root, CallID: "submit-1", ProviderCallID: "provider-submit", Name: "submit_interactive_turn",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ModelCompleted{Source: root, RequestedTools: []string{"submit_interactive_turn"}}})

	content, thinking := projector.Output()
	if content != "The door opened." || thinking != "I will inspect lore first.Checking details." {
		t.Fatalf("projected interactive output content=%q thinking=%q", content, thinking)
	}
	var reclassified int
	for _, event := range events {
		if event.Type == interactiveContentReclassifiedEvent {
			reclassified++
			if event.DataString("content") != "I will inspect lore first." {
				t.Fatalf("reclassified event = %#v", event.Data)
			}
		}
	}
	if reclassified != 1 {
		t.Fatalf("interactive events = %#v, want one reclassification", events)
	}
}

func TestPublicEventProjectorKeepsNextInteractiveResponseNarrativeAfterToolOnlyResponse(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, TaskID: "task", RootAgentName: "game",
	}, func(event agentrun.Event) { events = append(events, event) })
	root := agent.EventSource{Name: "game", Path: []string{"game"}}

	projector.Project(agent.Event{RunID: "run", Payload: agent.ThinkingDelta{
		Source: root, Delta: "I need to prepare the turn.",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ModelCompleted{
		Source: root, RequestedTools: []string{"prepare_interactive_turn"},
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolInputStarted{
		Source: root, CallID: "prepare-1", ProviderCallID: "provider-prepare", Name: "prepare_interactive_turn",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ThinkingDelta{
		Source: root, Delta: "The check failed; now write the narrative.",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.AssistantDelta{
		Source: root, Delta: "The door opened.",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolInputStarted{
		Source: root, CallID: "submit-1", ProviderCallID: "provider-submit", Name: "submit_interactive_turn",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ModelCompleted{
		Source: root, RequestedTools: []string{"submit_interactive_turn"},
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.AssistantFinal{
		Content: "The door opened.", Thinking: "I need to prepare the turn.The check failed; now write the narrative.",
	}})

	content, thinking := projector.Output()
	if content != "The door opened." || thinking != "I need to prepare the turn.The check failed; now write the narrative." {
		t.Fatalf("projected interactive output content=%q thinking=%q", content, thinking)
	}
	if len(events) != 5 || events[0].Type != "thinking" || events[1].Type != "tool_call" ||
		events[2].Type != "thinking" || events[3].Type != "chunk" || events[4].Type != "tool_call" {
		t.Fatalf("interactive event order = %#v", events)
	}
	if events[3].DataString("content") != "The door opened." {
		t.Fatalf("streamed narrative event = %#v", events[3].Data)
	}
}

func TestPublicEventProjectorUsesAccumulatedInteractiveNarrativeAsCanonicalOutput(t *testing.T) {
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, TaskID: "task", RootAgentName: "game",
	}, nil)
	root := agent.EventSource{Name: "game", Path: []string{"game"}}
	projector.Project(agent.Event{RunID: "run", Payload: agent.AssistantDelta{Source: root, Delta: "门后传来脚步声。"}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolInputStarted{
		Source: root, CallID: "submit-1", ProviderCallID: "provider-submit", Name: "submit_interactive_turn",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ModelCompleted{Source: root, RequestedTools: []string{"submit_interactive_turn"}}})

	message, transcript := projector.ProjectCanonicalOutput(agent.AssistantMessage("", nil))
	if message == nil || message.Content != "门后传来脚步声。" || len(message.ToolCalls) != 0 {
		t.Fatalf("canonical interactive message = %#v", message)
	}
	if transcript == nil || transcript.Content != message.Content {
		t.Fatalf("canonical interactive transcript = %#v", transcript)
	}
}

func TestPublicEventProjectorUsesDenovaAskSchemaForPublicInteractions(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	request := agent.InteractionRequest{
		ID: "ask-public", Kind: agent.InteractionAsk, AllowOther: true,
		Questions: []agent.InteractionQuestion{{
			ID: "direction", Prompt: agent.LocalizedText{Chinese: "选择方向", English: "Choose direction"},
			Options: []agent.InteractionOption{{
				Value: "continue", Label: agent.LocalizedText{Chinese: "继续", English: "Continue"}, Recommended: true,
			}},
		}},
	}
	projector.Project(agent.Event{RunID: "run", Payload: agent.InteractionRequested{Request: request}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.InteractionResolved{
		ID: request.ID, Resolution: agent.InteractionResolution{Answers: []agent.InteractionAnswer{{
			QuestionID: "direction", Values: []string{"continue"},
		}}},
	}})
	if len(events) != 2 || events[0].Type != "ask_pending" || events[1].Type != "ask_resolved" {
		t.Fatalf("interaction events = %#v", events)
	}
	pending, _ := events[0].Data.(map[string]any)
	questions, _ := pending["questions"].([]map[string]any)
	if pending["schema"] != "ask.pending.v1" || pending["tool_call_id"] != request.ID ||
		pending["agent_kind"] != "ide" || pending["status"] != "pending" || pending["allow_other"] != true ||
		len(questions) != 1 || questions[0]["question"] != "选择方向 / Choose direction" ||
		questions[0]["recommended_option_id"] != "continue" {
		t.Fatalf("pending interaction = %#v", pending)
	}
	resolved, _ := events[1].Data.(map[string]any)
	answers, _ := resolved["answers"].([]map[string]any)
	if resolved["schema"] != "ask.pending.v1" || resolved["status"] != "answered" || len(answers) != 1 {
		t.Fatalf("resolved interaction = %#v", resolved)
	}
}

func TestPublicEventProjectorProjectsPublicGoalAndTodoAuthority(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	projector.Project(agent.Event{RunID: "run", Payload: agent.GoalUpdated{
		Present: true, State: agent.GoalState{ID: "goal-1", Objective: "Ship", Status: agent.GoalActive, Revision: 2},
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.TodoUpdated{State: agent.TodoState{
		Revision: 3, Items: []agent.TodoItem{{ID: "todo-1", Text: "Verify", Status: agent.TodoInProgress}},
	}}})
	if len(events) != 2 || events[0].Type != "goal_updated" || events[1].Type != "todo_updated" {
		t.Fatalf("capability events = %#v", events)
	}
	goal, _ := events[0].Data.(map[string]any)
	if goal["schema"] != "agent.goal.v1" || goal["id"] != "goal-1" || goal["status"] != agent.GoalActive || goal["revision"] != uint64(2) {
		t.Fatalf("goal projection = %#v", goal)
	}
	todo, _ := events[1].Data.(map[string]any)
	items, _ := todo["items"].([]map[string]any)
	if todo["schema"] != "agent.todo.v1" || todo["revision"] != uint64(3) || len(items) != 1 || items[0]["status"] != agent.TodoInProgress {
		t.Fatalf("todo projection = %#v", todo)
	}
}

func TestPublicEventProjectorUsesPermissionPolicyPresentationWithoutReclassification(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	request := agent.InteractionRequest{
		ID: "permission-call", Kind: agent.InteractionPermission,
		Permission: &agent.PermissionPresentation{
			Tool: "bash", CallID: "call-1", Arguments: []byte(`{"command":"go test ./..."}`),
			Reason: agent.LocalizedText{Chinese: "需要授权", English: "Approval required"},
			Mode:   "write", Command: "go test ./...", Cwd: "/workspace", Risk: "medium",
			RuleID: "bash_unlisted_command", ArgsHash: "policy-owned-hash", CanRemember: true,
			RuleMatcherVersion: 3, RuleCommandKey: `["go","test"]`, RuleCommandPattern: "go test ...",
		},
	}
	projector.Project(agent.Event{RunID: "run", Payload: agent.InteractionRequested{Request: request}})
	if len(events) != 1 || events[0].Type != "ask_pending" {
		t.Fatalf("permission events = %#v", events)
	}
	pending, _ := events[0].Data.(map[string]any)
	approval, _ := pending["approval"].(map[string]any)
	if pending["kind"] != "tool_approval" || pending["tool_call_id"] != "call-1" ||
		approval["mode"] != "write" || approval["command"] != "go test ./..." ||
		approval["cwd"] != "/workspace" || approval["risk"] != "medium" ||
		approval["rule_id"] != "bash_unlisted_command" || approval["args_hash"] != "policy-owned-hash" ||
		approval["can_remember"] != true || approval["rule_matcher_version"] != 3 ||
		approval["rule_command_key"] != `["go","test"]` || approval["rule_command_pattern"] != "go test ..." {
		t.Fatalf("permission presentation = %#v", pending)
	}
}

func TestPublicEventProjectorProjectsCompactionEdges(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	projector.Project(agent.Event{RunID: "run", Payload: agent.CompactionStarted{ID: "compact-1"}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.CompactionRemoved{ID: "compact-1", Revision: 2}})
	if len(events) != 2 || events[0].Type != "context_compaction" || events[0].DataString("status") != "started" ||
		events[1].Type != "context_compaction" || events[1].DataString("status") != "removed" {
		t.Fatalf("lifecycle projection = %#v", events)
	}
}
