package adk

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestHostIdentityAndSummaryBounds(t *testing.T) {
	identity := RunIdentity{RequestID: "request", RunID: "run", SessionID: "session"}
	if err := identity.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (RunIdentity{RequestID: "request"}).Validate(); err == nil {
		t.Fatal("partial identity was accepted")
	}
	summary := ModelVisibleSummary{
		Source: "workspace-index", Purpose: "answer the current turn", Content: "bounded", MaxBytes: 7,
	}
	if err := summary.Validate(); err != nil {
		t.Fatal(err)
	}
	summary.Content = "too large"
	if err := summary.Validate(); err == nil {
		t.Fatal("oversized model-visible summary was accepted")
	}
	start := StartRequest{Identity: identity, Task: "创作", TaskMaxBytes: 5}
	if err := start.Validate(); err == nil {
		t.Fatal("task exceeding its byte limit was accepted")
	}
	start.TaskMaxBytes = len(start.Task)
	if err := start.Validate(); err != nil {
		t.Fatal(err)
	}
}

type fakeLifecycleHost struct {
	identity RunIdentity
	task     string
	next     uint64
	calls    []string
	canceled bool
}

func (host *fakeLifecycleHost) Start(_ context.Context, request StartRequest) (*AsyncIterator[*HostEvent], error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	host.identity = request.Identity
	host.task = request.Task
	host.calls = append(host.calls, "start")
	return host.event("started"), nil
}

func (host *fakeLifecycleHost) Resume(_ context.Context, request ResumeRequest) (*AsyncIterator[*HostEvent], error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.Identity != host.identity {
		return nil, errors.New("resume identity mismatch")
	}
	host.calls = append(host.calls, "resume")
	return host.event("resumed:" + string(request.Token)), nil
}

func (host *fakeLifecycleHost) Cancel(_ context.Context, request CancelRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Identity != host.identity {
		return errors.New("cancel identity mismatch")
	}
	host.calls = append(host.calls, "cancel")
	host.canceled = true
	return nil
}

func (host *fakeLifecycleHost) Context(_ context.Context, request HostContextRequest) (*HostContext, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.Identity != host.identity {
		return nil, errors.New("context identity mismatch")
	}
	host.calls = append(host.calls, "context")
	return &HostContext{Summary: ModelVisibleSummary{
		Source:   "fake-host",
		Purpose:  "resume the external task",
		Content:  host.task,
		MaxBytes: request.SummaryMaxBytes,
	}}, nil
}

type boundedContextHost struct {
	fakeLifecycleHost
	response *HostContext
	err      error
	request  HostContextRequest
	calls    int
}

func (host *boundedContextHost) Context(_ context.Context, request HostContextRequest) (*HostContext, error) {
	host.calls++
	host.request = request
	return host.response, host.err
}

func TestResolveHostContextUsesCallerSummaryLimit(t *testing.T) {
	identity := RunIdentity{RequestID: "request", RunID: "run", SessionID: "session"}
	response := &HostContext{Summary: ModelVisibleSummary{
		Source:   "forged-host",
		Purpose:  "continue the current run",
		Content:  "123456",
		MaxBytes: 1 << 20,
	}}
	host := &boundedContextHost{response: response}
	request := HostContextRequest{
		Identity:        identity,
		Sources:         []string{"workspace"},
		SummaryMaxBytes: 5,
	}

	if _, err := ResolveHostContext(context.Background(), host, request); err == nil ||
		!strings.Contains(err.Error(), "6 bytes exceeds limit 5") {
		t.Fatalf("oversized host response error = %v", err)
	}
	if response.Summary.MaxBytes != 1<<20 {
		t.Fatalf("boundary mutated host response MaxBytes = %d", response.Summary.MaxBytes)
	}
	if host.calls != 1 || !reflect.DeepEqual(host.request, request) {
		t.Fatalf("host call = %d, request = %#v", host.calls, host.request)
	}

	response.Summary.Content = "12345"
	resolved, err := ResolveHostContext(context.Background(), host, request)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == response {
		t.Fatal("boundary returned the host-owned context pointer")
	}
	if resolved.Summary.MaxBytes != request.SummaryMaxBytes {
		t.Fatalf("resolved MaxBytes = %d, want caller limit %d", resolved.Summary.MaxBytes, request.SummaryMaxBytes)
	}
	if response.Summary.MaxBytes != 1<<20 {
		t.Fatalf("boundary mutated accepted host response MaxBytes = %d", response.Summary.MaxBytes)
	}
}

func TestResolveHostContextRejectsInvalidBoundaryResults(t *testing.T) {
	identity := RunIdentity{RequestID: "request", RunID: "run", SessionID: "session"}
	request := HostContextRequest{Identity: identity, SummaryMaxBytes: 16}

	t.Run("nil response", func(t *testing.T) {
		host := &boundedContextHost{}
		if _, err := ResolveHostContext(context.Background(), host, request); err == nil ||
			!strings.Contains(err.Error(), "nil context") {
			t.Fatalf("nil response error = %v", err)
		}
	})

	t.Run("missing source", func(t *testing.T) {
		host := &boundedContextHost{response: &HostContext{Summary: ModelVisibleSummary{
			Purpose:  "continue the current run",
			Content:  "bounded",
			MaxBytes: 1 << 20,
		}}}
		if _, err := ResolveHostContext(context.Background(), host, request); err == nil ||
			!strings.Contains(err.Error(), "source is required") {
			t.Fatalf("missing source error = %v", err)
		}
	})

	t.Run("invalid request before host call", func(t *testing.T) {
		host := &boundedContextHost{response: &HostContext{}}
		invalid := request
		invalid.SummaryMaxBytes = 0
		if _, err := ResolveHostContext(context.Background(), host, invalid); err == nil ||
			!strings.Contains(err.Error(), "SummaryMaxBytes must be positive") {
			t.Fatalf("invalid request error = %v", err)
		}
		if host.calls != 0 {
			t.Fatalf("invalid request reached host %d time(s)", host.calls)
		}
	})
}

func TestCancelRequestValidateExhaustsSupportedModes(t *testing.T) {
	identity := RunIdentity{RequestID: "request", RunID: "run", SessionID: "session"}
	for _, test := range []struct {
		name string
		mode CancelMode
	}{
		{name: "immediate zero value", mode: CancelImmediate},
		{name: "after chat model", mode: CancelAfterChatModel},
		{name: "after tool calls", mode: CancelAfterToolCalls},
		{name: "either safe point", mode: CancelAfterChatModel | CancelAfterToolCalls},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := (CancelRequest{Identity: identity, Mode: test.mode}).Validate(); err != nil {
				t.Fatalf("valid mode %d rejected: %v", test.mode, err)
			}
		})
	}

	const (
		reservedLowBit CancelMode = 1
		unknownHighBit CancelMode = 1 << 20
	)
	for _, test := range []struct {
		name string
		mode CancelMode
	}{
		{name: "reserved low bit", mode: reservedLowBit},
		{name: "reserved low bit with safe point", mode: reservedLowBit | CancelAfterChatModel},
		{name: "unknown high bit", mode: unknownHighBit},
		{name: "unknown high bit with safe points", mode: unknownHighBit | CancelAfterChatModel | CancelAfterToolCalls},
		{name: "negative mode", mode: CancelMode(-1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := (CancelRequest{Identity: identity, Mode: test.mode}).Validate(); err == nil {
				t.Fatalf("unsupported mode %d was accepted", test.mode)
			}
		})
	}
}

func (host *fakeLifecycleHost) event(value string) *AsyncIterator[*HostEvent] {
	host.next++
	iterator, generator := NewAsyncIteratorPair[*HostEvent]()
	generator.Send(&HostEvent{
		Identity: host.identity,
		Sequence: host.next,
		Event:    &AgentEvent{Output: &AgentOutput{CustomizedOutput: value}},
	})
	generator.Close()
	return iterator
}

func TestHostExternalAgentLifecycle(t *testing.T) {
	identity := RunIdentity{RequestID: "request", RunID: "run", SessionID: "session"}
	initialContext := &HostContext{Summary: ModelVisibleSummary{
		Source: "workspace", Purpose: "start the external task", Content: "outline", MaxBytes: 16,
	}}
	host := &fakeLifecycleHost{}
	var lifecycle Host = host

	started, err := lifecycle.Start(context.Background(), StartRequest{
		Identity: identity, Task: "draft", TaskMaxBytes: 5, Context: initialContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostEvent(t, started, identity, 1, "started")

	hostContext, err := ResolveHostContext(context.Background(), lifecycle, HostContextRequest{
		Identity: identity, Sources: []string{"fake-host"}, SummaryMaxBytes: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := hostContext.Validate(); err != nil {
		t.Fatal(err)
	}
	if hostContext.Summary.Content != "draft" {
		t.Fatalf("host context = %#v", hostContext)
	}

	resumed, err := lifecycle.Resume(context.Background(), ResumeRequest{
		Identity: identity, Token: json.RawMessage(`{"approved":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostEvent(t, resumed, identity, 2, `resumed:{"approved":true}`)

	if err := lifecycle.Cancel(context.Background(), CancelRequest{Identity: identity, Mode: CancelImmediate}); err != nil {
		t.Fatal(err)
	}
	if !host.canceled {
		t.Fatal("fake host did not observe cancellation")
	}
	if want := []string{"start", "context", "resume", "cancel"}; !reflect.DeepEqual(host.calls, want) {
		t.Fatalf("lifecycle calls = %v, want %v", host.calls, want)
	}
}

func assertHostEvent(t *testing.T, events *AsyncIterator[*HostEvent], identity RunIdentity, sequence uint64, value string) {
	t.Helper()
	event, ok := events.Next()
	if !ok || event == nil || event.Identity != identity || event.Sequence != sequence || event.Event == nil ||
		event.Event.Output == nil || event.Event.Output.CustomizedOutput != value {
		t.Fatalf("host event = %#v", event)
	}
	if _, ok := events.Next(); ok {
		t.Fatal("unexpected extra host event")
	}
}
