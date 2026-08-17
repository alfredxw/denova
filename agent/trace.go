package agent

import (
	"context"
	"encoding/json"
	"time"
)

type TraceKind string

const (
	TraceRunAccepted   TraceKind = "run.accepted"
	TraceCycleStarted  TraceKind = "cycle.started"
	TraceModelStarted  TraceKind = "model.started"
	TraceModelFinished TraceKind = "model.finished"
	TraceToolStarted   TraceKind = "tool.started"
	TraceToolFinished  TraceKind = "tool.finished"
	TraceRunSettled    TraceKind = "run.settled"
)

type TraceEvent struct {
	Kind       TraceKind
	Time       time.Time
	Session    SessionKey
	RunID      string
	Cycle      int
	ToolCallID string
	ToolName   string
	Data       json.RawMessage
	Err        error
}

// TraceSink is observational and cannot affect execution or durable state.
// Implementations must be concurrency-safe; Agent recovers sink panics.
type TraceSink interface {
	Record(context.Context, TraceEvent)
}

type TraceFunc func(context.Context, TraceEvent)

func (record TraceFunc) Record(ctx context.Context, event TraceEvent) {
	if record != nil {
		record(ctx, event)
	}
}

func emitTrace(ctx context.Context, sink TraceSink, event TraceEvent) {
	if sink == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	defer func() { _ = recover() }()
	sink.Record(ctx, event)
}
