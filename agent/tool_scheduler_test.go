package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type schedulerTool struct {
	name string
	run  func(context.Context, string) (ToolResult, error)
}

func (tool *schedulerTool) Info(context.Context) (*ToolInfo, error) {
	return &ToolInfo{Name: tool.name, Desc: tool.name}, nil
}

func (tool *schedulerTool) Run(ctx context.Context, arguments string, _ ...ToolOption) (ToolResult, error) {
	return tool.run(ctx, arguments)
}

func schedulerReadDescriptor(steering SteeringPolicy) ToolDescriptor {
	return ToolDescriptor{
		Source: ToolSourceRead, Execution: ToolExecutionParallelRead,
		Recovery: ToolRecoveryReadOnly, ResultProjection: ToolResultBoundedModelContext,
		Steering: steering, MaxResultBytes: 4096,
	}
}

func schedulerWriteDescriptor() ToolDescriptor {
	return ToolDescriptor{
		Source: ToolSourceWrite, Execution: ToolExecutionWorkspaceExclusive,
		Recovery: ToolRecoveryReconcilable, ResultProjection: ToolResultBoundedModelContext,
		Steering: SteeringFinishCurrent, MutatesWorkspace: true,
		RequiresPostCheck: true, MaxResultBytes: 4096,
	}
}

func schedulerChildDescriptor() ToolDescriptor {
	return ToolDescriptor{
		Source: ToolSourceOther, Execution: ToolExecutionChild,
		Recovery: ToolRecoveryReadOnly, ResultProjection: ToolResultBoundedModelContext,
		Steering: SteeringFinishCurrent, MaxResultBytes: 4096,
	}
}

func schedulerDefinition(name string, descriptor ToolDescriptor, run func(context.Context, string) (ToolResult, error)) ToolDefinition {
	return ToolDefinition{Tool: &schedulerTool{name: name, run: run}, Descriptor: descriptor}
}

func schedulerCall(name string) ToolCall {
	return ToolCall{ID: name + "-call", Type: "function", Function: FunctionCall{Name: name, Arguments: `{}`}}
}

func receiveSchedulerStart(t *testing.T, starts <-chan string) string {
	t.Helper()
	select {
	case name := <-starts:
		return name
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for tool start")
		return ""
	}
}

func assertNoSchedulerStart(t *testing.T, starts <-chan string) {
	t.Helper()
	select {
	case name := <-starts:
		t.Fatalf("tool %q crossed a scheduler barrier", name)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestToolBatchSchedulerBoundsParallelReads(t *testing.T) {
	tests := []struct {
		name        string
		parallelism int
		want        int
	}{
		{name: "default", want: DefaultToolParallelism},
		{name: "configured", parallelism: 3, want: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const calls = 12
			toolCalls := make([]ToolCall, 0, calls)
			for index := 0; index < calls; index++ {
				call := schedulerCall("read")
				call.ID = fmt.Sprintf("read-%02d", index)
				toolCalls = append(toolCalls, call)
			}
			model := &scriptedModel{responses: []scriptedModelResponse{
				{message: AssistantMessage("", toolCalls)},
				{message: AssistantMessage("done", nil)},
			}}
			starts := make(chan string, calls)
			release := make(chan struct{})
			var active atomic.Int32
			var maximum atomic.Int32
			definition := schedulerDefinition("read", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
				current := active.Add(1)
				for {
					previous := maximum.Load()
					if current <= previous || maximum.CompareAndSwap(previous, current) {
						break
					}
				}
				starts <- "read"
				<-release
				active.Add(-1)
				return TextToolResult("ok"), nil
			})
			native, err := NewAgent(context.Background(), AgentConfig{
				Name: "parallel-limit", Model: model, Tools: []ToolDefinition{definition}, ToolParallelism: test.parallelism,
			})
			if err != nil {
				t.Fatal(err)
			}
			iterator := NewRunner(RunnerConfig{Agent: native}).Query(context.Background(), "go")
			if event, ok := iterator.Next(); !ok || event.Output == nil || event.Output.MessageOutput == nil || event.Output.MessageOutput.Role != Assistant {
				t.Fatalf("assistant event = %#v", event)
			}
			for index := 0; index < test.want; index++ {
				receiveSchedulerStart(t, starts)
			}
			assertNoSchedulerStart(t, starts)
			close(release)
			toolMessages := 0
			for {
				event, ok := iterator.Next()
				if !ok {
					break
				}
				if event.Err != nil {
					t.Fatal(event.Err)
				}
				if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
					toolMessages++
				}
			}
			if toolMessages != calls || maximum.Load() != int32(test.want) {
				t.Fatalf("tool messages=%d maximum=%d, want %d", toolMessages, maximum.Load(), test.want)
			}
		})
	}
}

func TestToolParallelismConfigurationNormalization(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("done", nil)}}}
	for _, test := range []struct {
		configured int
		want       int
	}{{configured: 0, want: 8}, {configured: -1, want: 8}, {configured: 1, want: 1}, {configured: 65, want: 64}} {
		native, err := NewAgent(context.Background(), AgentConfig{Name: "normalization", Model: model, ToolParallelism: test.configured})
		if err != nil {
			t.Fatal(err)
		}
		if native.toolParallelism != test.want {
			t.Fatalf("ToolParallelism %d normalized to %d, want %d", test.configured, native.toolParallelism, test.want)
		}
	}
}

func TestToolBatchSchedulerEnforcesReadExclusiveAndChildBarriers(t *testing.T) {
	names := []string{"read_one", "read_two", "write", "read_three", "child", "read_four"}
	calls := make([]ToolCall, 0, len(names))
	for _, name := range names {
		calls = append(calls, schedulerCall(name))
	}
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", calls)},
		{message: AssistantMessage("done", nil)},
	}}
	starts := make(chan string, len(names))
	readFirstRelease := make(chan struct{})
	writeRelease := make(chan struct{})
	readThreeRelease := make(chan struct{})
	childRelease := make(chan struct{})
	readFourRelease := make(chan struct{})
	definitions := []ToolDefinition{
		schedulerDefinition("read_one", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			starts <- "read_one"
			<-readFirstRelease
			return TextToolResult("one"), nil
		}),
		schedulerDefinition("read_two", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			starts <- "read_two"
			<-readFirstRelease
			return TextToolResult("two"), nil
		}),
		schedulerDefinition("write", schedulerWriteDescriptor(), func(context.Context, string) (ToolResult, error) {
			starts <- "write"
			<-writeRelease
			return TextToolResult("write"), nil
		}),
		schedulerDefinition("read_three", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			starts <- "read_three"
			<-readThreeRelease
			return TextToolResult("three"), nil
		}),
		schedulerDefinition("child", schedulerChildDescriptor(), func(context.Context, string) (ToolResult, error) {
			starts <- "child"
			<-childRelease
			return TextToolResult("child"), nil
		}),
		schedulerDefinition("read_four", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			starts <- "read_four"
			<-readFourRelease
			return TextToolResult("four"), nil
		}),
	}
	native, err := NewAgent(context.Background(), AgentConfig{Name: "barriers", Model: model, Tools: definitions})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: native}).Query(context.Background(), "go")
	if _, ok := iterator.Next(); !ok {
		t.Fatal("missing assistant event")
	}
	first := map[string]bool{receiveSchedulerStart(t, starts): true, receiveSchedulerStart(t, starts): true}
	if !first["read_one"] || !first["read_two"] {
		t.Fatalf("first stage = %#v", first)
	}
	assertNoSchedulerStart(t, starts)
	close(readFirstRelease)
	if got := receiveSchedulerStart(t, starts); got != "write" {
		t.Fatalf("after read stage = %q", got)
	}
	assertNoSchedulerStart(t, starts)
	close(writeRelease)
	if got := receiveSchedulerStart(t, starts); got != "read_three" {
		t.Fatalf("after write barrier = %q", got)
	}
	assertNoSchedulerStart(t, starts)
	close(readThreeRelease)
	if got := receiveSchedulerStart(t, starts); got != "child" {
		t.Fatalf("before child barrier = %q", got)
	}
	assertNoSchedulerStart(t, starts)
	close(childRelease)
	if got := receiveSchedulerStart(t, starts); got != "read_four" {
		t.Fatalf("after child barrier = %q", got)
	}
	close(readFourRelease)
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
	}
}

func TestToolCompletionEventsUseCompletionOrderWhileTranscriptUsesSourceOrder(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{schedulerCall("slow"), schedulerCall("fast")})},
		{message: AssistantMessage("done", nil)},
	}}
	slowStarted := make(chan struct{})
	fastStarted := make(chan struct{})
	slowRelease := make(chan struct{})
	fastRelease := make(chan struct{})
	definitions := []ToolDefinition{
		schedulerDefinition("slow", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			close(slowStarted)
			<-slowRelease
			return ToolResult{ModelContent: "model-slow", DisplayContent: "display-slow", Status: ToolResultSuccess}, nil
		}),
		schedulerDefinition("fast", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			close(fastStarted)
			<-fastRelease
			return ToolResult{ModelContent: "model-fast", DisplayContent: "display-fast", Status: ToolResultSuccess}, nil
		}),
	}
	native, err := NewAgent(context.Background(), AgentConfig{Name: "completion-order", Model: model, Tools: definitions})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: native}).Query(context.Background(), "go")
	if _, ok := iterator.Next(); !ok {
		t.Fatal("missing assistant event")
	}
	select {
	case <-slowStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("slow tool did not start")
	}
	select {
	case <-fastStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("fast tool did not start")
	}
	close(fastRelease)
	finished := make([]string, 0, 2)
	for len(finished) == 0 {
		event, ok := nextAgentEventWithin(t, iterator, 200*time.Millisecond)
		if !ok || event.Err != nil {
			t.Fatalf("event before fast completion = %#v", event)
		}
		if event.Output != nil && event.Output.ToolExecution != nil && event.Output.ToolExecution.Phase == ToolExecutionFinished {
			finished = append(finished, event.Output.ToolExecution.ToolName)
		}
	}
	if finished[0] != "fast" {
		t.Fatalf("first completion = %#v", finished)
	}
	close(slowRelease)
	toolMessages := make([]*Message, 0, 2)
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output != nil && event.Output.ToolExecution != nil && event.Output.ToolExecution.Phase == ToolExecutionFinished {
			finished = append(finished, event.Output.ToolExecution.ToolName)
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
			toolMessages = append(toolMessages, event.Output.MessageOutput.Message)
		}
	}
	if fmt.Sprint(finished) != fmt.Sprint([]string{"fast", "slow"}) {
		t.Fatalf("completion event order = %#v", finished)
	}
	if len(toolMessages) != 2 || toolMessages[0].ToolName != "slow" || toolMessages[0].Content != "model-slow" ||
		toolMessages[1].ToolName != "fast" || toolMessages[1].Content != "model-fast" {
		t.Fatalf("source-ordered transcript = %#v", toolMessages)
	}
}

func TestSteeringFillsCurrentStageTailAndAllLaterStages(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("", []ToolCall{
		schedulerCall("first"), schedulerCall("second"), schedulerCall("write"),
	})}}}
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	var executed atomic.Int32
	definitions := []ToolDefinition{
		schedulerDefinition("first", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			executed.Add(1)
			close(firstStarted)
			<-firstRelease
			return TextToolResult("first"), nil
		}),
		schedulerDefinition("second", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			executed.Add(1)
			return TextToolResult("unexpected"), nil
		}),
		schedulerDefinition("write", schedulerWriteDescriptor(), func(context.Context, string) (ToolResult, error) {
			executed.Add(1)
			return TextToolResult("unexpected"), nil
		}),
	}
	native, err := NewAgent(context.Background(), AgentConfig{Name: "steer-tail", Model: model, Tools: definitions, ToolParallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := WithCancel()
	iterator := NewRunner(RunnerConfig{Agent: native}).Query(context.Background(), "go", runOption)
	if _, ok := iterator.Next(); !ok {
		t.Fatal("missing assistant event")
	}
	select {
	case <-firstStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("first tool did not start")
	}
	handle, contributed := cancel(WithAgentCancelMode(CancelAfterToolCalls))
	if !contributed {
		t.Fatal("steering request did not contribute")
	}
	close(firstRelease)
	results := map[string]*ToolResultSummary{}
	var cancelErr *CancelError
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
			message := event.Output.MessageOutput.Message
			results[message.ToolName] = message.ToolResult
		}
		if event.Err != nil && !errors.As(event.Err, &cancelErr) {
			t.Fatal(event.Err)
		}
	}
	if executed.Load() != 1 || len(results) != 3 || results["first"].Status != ToolResultSuccess ||
		results["second"].SyntheticReason != ToolSyntheticSteeringBeforeStart ||
		results["write"].SyntheticReason != ToolSyntheticSteeringBeforeStart {
		t.Fatalf("executed=%d results=%#v", executed.Load(), results)
	}
	if cancelErr == nil || cancelErr.Info.Mode&CancelAfterToolCalls == 0 {
		t.Fatalf("cancel error = %#v", cancelErr)
	}
	if err := handle.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestSteeringInterruptsOnlyInterruptibleWaits(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("", []ToolCall{
		schedulerCall("wait"), schedulerCall("finish"), schedulerCall("write"),
	})}}}
	starts := make(chan string, 2)
	finishRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseFinish := func() { releaseOnce.Do(func() { close(finishRelease) }) }
	t.Cleanup(releaseFinish)
	var writeCalls atomic.Int32
	definitions := []ToolDefinition{
		schedulerDefinition("wait", schedulerReadDescriptor(SteeringInterruptibleWait), func(ctx context.Context, _ string) (ToolResult, error) {
			starts <- "wait"
			<-ctx.Done()
			return ToolResult{}, ctx.Err()
		}),
		schedulerDefinition("finish", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			starts <- "finish"
			<-finishRelease
			return TextToolResult("finished safely"), nil
		}),
		schedulerDefinition("write", schedulerWriteDescriptor(), func(context.Context, string) (ToolResult, error) {
			writeCalls.Add(1)
			return TextToolResult("unexpected"), nil
		}),
	}
	native, err := NewAgent(context.Background(), AgentConfig{Name: "steering-policy", Model: model, Tools: definitions})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := WithCancel()
	iterator := NewRunner(RunnerConfig{Agent: native}).Query(context.Background(), "go", runOption)
	if _, ok := iterator.Next(); !ok {
		t.Fatal("missing assistant event")
	}
	started := map[string]bool{receiveSchedulerStart(t, starts): true, receiveSchedulerStart(t, starts): true}
	if !started["wait"] || !started["finish"] {
		t.Fatalf("started = %#v", started)
	}
	handle, contributed := cancel(WithAgentCancelMode(CancelAfterToolCalls))
	if !contributed {
		t.Fatal("steering request did not contribute")
	}
	waitInterrupted := false
	for !waitInterrupted {
		event, ok := nextAgentEventWithin(t, iterator, 200*time.Millisecond)
		if !ok || event.Err != nil {
			t.Fatalf("event before wait interruption = %#v", event)
		}
		if event.Output != nil && event.Output.ToolExecution != nil &&
			event.Output.ToolExecution.Phase == ToolExecutionFinished && event.Output.ToolExecution.ToolName == "wait" {
			result := event.Output.ToolExecution.Result
			waitInterrupted = result != nil && result.SyntheticReason == ToolSyntheticSteeringInterrupted
		}
	}
	if writeCalls.Load() != 0 {
		t.Fatal("exclusive stage started while finish_current was still running")
	}
	releaseFinish()
	results := map[string]*ToolResultSummary{}
	var cancelErr *CancelError
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
			message := event.Output.MessageOutput.Message
			results[message.ToolName] = message.ToolResult
		}
		if event.Err != nil && !errors.As(event.Err, &cancelErr) {
			t.Fatal(event.Err)
		}
	}
	if len(results) != 3 || results["wait"].SyntheticReason != ToolSyntheticSteeringInterrupted ||
		results["finish"].Status != ToolResultSuccess || results["write"].SyntheticReason != ToolSyntheticSteeringBeforeStart ||
		writeCalls.Load() != 0 || cancelErr == nil {
		t.Fatalf("results=%#v writeCalls=%d cancel=%#v", results, writeCalls.Load(), cancelErr)
	}
	if err := handle.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestDurabilityFailureWaitsStartedStageAndPairsUnstartedCalls(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("", []ToolCall{
		schedulerCall("control"), schedulerCall("side_effect"), schedulerCall("later_write"),
	})}}}
	starts := make(chan string, 2)
	allowControl := make(chan struct{})
	sideRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseSide := func() { releaseOnce.Do(func() { close(sideRelease) }) }
	t.Cleanup(releaseSide)
	var laterCalls atomic.Int32
	definitions := []ToolDefinition{
		schedulerDefinition("control", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			starts <- "control"
			<-allowControl
			return ToolResult{}, MarkToolControlError(errors.New("journal unavailable"))
		}),
		schedulerDefinition("side_effect", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
			starts <- "side_effect"
			<-sideRelease
			return TextToolResult("completed receipt"), nil
		}),
		schedulerDefinition("later_write", schedulerWriteDescriptor(), func(context.Context, string) (ToolResult, error) {
			laterCalls.Add(1)
			return TextToolResult("unexpected"), nil
		}),
	}
	native, err := NewAgent(context.Background(), AgentConfig{Name: "durability", Model: model, Tools: definitions})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: native}).Query(context.Background(), "go")
	if _, ok := iterator.Next(); !ok {
		t.Fatal("missing assistant event")
	}
	started := map[string]bool{receiveSchedulerStart(t, starts): true, receiveSchedulerStart(t, starts): true}
	if !started["control"] || !started["side_effect"] {
		t.Fatalf("started = %#v", started)
	}
	close(allowControl)
	for {
		event, ok := nextAgentEventWithin(t, iterator, 200*time.Millisecond)
		if !ok {
			t.Fatal("iterator closed before control completion")
		}
		if event.Output != nil && event.Output.ToolExecution != nil &&
			event.Output.ToolExecution.Phase == ToolExecutionFinished && event.Output.ToolExecution.ToolName == "control" {
			break
		}
	}
	pending := make(chan nextAgentEventResult, 1)
	safeGo(func() {
		event, ok := iterator.Next()
		pending <- nextAgentEventResult{event: event, ok: ok}
	}, func(err error) {
		pending <- nextAgentEventResult{err: err}
	})
	select {
	case unexpected := <-pending:
		releaseSide()
		t.Fatalf("scheduler did not wait for started side effect: %#v", unexpected)
	case <-time.After(20 * time.Millisecond):
	}
	releaseSide()
	firstAfterRelease := <-pending
	if firstAfterRelease.err != nil || !firstAfterRelease.ok {
		t.Fatalf("event after side-effect release = %#v", firstAfterRelease)
	}
	toolResults := map[string]*ToolResultSummary{}
	var controlErr error
	consume := func(event *AgentEvent) {
		if event == nil {
			return
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
			message := event.Output.MessageOutput.Message
			toolResults[message.ToolName] = message.ToolResult
		}
		if event.Err != nil {
			controlErr = event.Err
		}
	}
	consume(firstAfterRelease.event)
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		consume(event)
	}
	if len(toolResults) != 3 || toolResults["control"].Status != ToolResultError ||
		toolResults["side_effect"].Status != ToolResultSuccess ||
		toolResults["later_write"].SyntheticReason != ToolSyntheticPolicyBlocked ||
		laterCalls.Load() != 0 || !IsToolControlError(controlErr) {
		t.Fatalf("results=%#v later=%d error=%v", toolResults, laterCalls.Load(), controlErr)
	}
}

type blockingToolMiddleware struct{ BaseMiddleware }

func (*blockingToolMiddleware) WrapToolCall(_ context.Context, _ ToolCallEndpoint, _ *ToolContext) (ToolCallEndpoint, error) {
	return func(context.Context, string, ...ToolOption) (ToolResult, error) {
		return SyntheticToolResult(ToolResultBlocked, ToolSyntheticPolicyBlocked, "blocked by policy"), nil
	}, nil
}

func TestPolicyBlockedToolProducesOnePairedStructuredResult(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{schedulerCall("blocked")})},
		{message: AssistantMessage("done", nil)},
	}}
	var calls atomic.Int32
	definition := schedulerDefinition("blocked", schedulerReadDescriptor(SteeringFinishCurrent), func(context.Context, string) (ToolResult, error) {
		calls.Add(1)
		return TextToolResult("unexpected"), nil
	})
	native, err := NewAgent(context.Background(), AgentConfig{
		Name: "policy", Model: model, Tools: []ToolDefinition{definition}, Middlewares: []Middleware{&blockingToolMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: native}).Query(context.Background(), "go")
	results := make([]*Message, 0, 1)
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
			results = append(results, event.Output.MessageOutput.Message)
		}
	}
	if calls.Load() != 0 || len(results) != 1 || results[0].ToolResult == nil ||
		results[0].ToolResult.Status != ToolResultBlocked || results[0].ToolResult.SyntheticReason != ToolSyntheticPolicyBlocked {
		t.Fatalf("calls=%d results=%#v", calls.Load(), results)
	}
}
