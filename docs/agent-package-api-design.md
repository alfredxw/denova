# Agent Package API Design

## Public lifecycle

```go
owner, err := agent.New(ctx, agent.Definition{
    Model: builtin.Model(modelConfig),
    Tools: tools.Combine(tools.Workspace(workspaceConfig), tools.Todo(), tools.Ask()),
    Cleanup: cleanup.Standard(cleanupConfig),
},
    agent.WithSessionStore(sessionfile.New(storeRoot)),
    agent.WithTrace(trace),
)

session, err := owner.Session(ctx, agent.NamedSession("main"))
run, err := session.Run(ctx, agent.Text("Continue the draft."))
result, err := run.Wait(ctx)
```

`Source` can be a static `Definition` or a host implementation that resolves Definition, context, and canonical adapters for each Turn. Built-in models, Toolsets, Cleanup, and Compaction are declarative Definition components: static compositions are initialized and validated together by `agent.New`; dynamic Definitions use the same boundary immediately after `Source.Prepare`.

### Agent

- `Run` creates a temporary Session for one-shot work.
- `Session`, `ListSessions`, `CloseSessions`, and `DeleteSessions` own conversation lifecycle.
- `Close` aborts live Runs and releases Store leases.

### Session

- `Run` starts work only when the Session lane is idle.
- `Snapshot` returns transcript-independent UI state: active Run, output, queue, open tools, recent settlements, interactions, and capability projections.
- `Observe` returns a snapshot plus live events after a process-local cursor.
- `AttachRun` reattaches only to a Run owned by the current process.
- `Clear`, Cleanup, Compaction, Goal, Todo, State, and canonical-message refresh share the same serial Session lane.

### Run

- `Events` streams typed lifecycle events.
- `Wait` returns the terminal `Result`.
- `Steer` preempts at the next safe model-loop boundary.
- `Queue` continues inside the same Run; `FollowUp` creates a serial successor Run.
- `Abort` stops active work; `Respond` resolves a current in-process interaction.

Command receipts identify accepted process-local commands and carry a monotonic Session cursor. They are not a cross-process replay token.

## Persistence API

```go
type Store interface {
    Open(context.Context, Key) (Log, error)
    List(context.Context, Selector) ([]Key, error)
    Delete(context.Context, Key) error
}

type Log interface {
    Replay(context.Context, func(Record) error) (ReplayStats, error)
    Append(context.Context, Revision, ...Record) (Revision, error)
    Close() error
}
```

The Store persists one logical Session stream. A standalone Agent stream contains its provider-neutral messages, versioned capability updates, and turn settlements. A host-canonical stream can implement `CanonicalMessageLog`: the host journal owns the messages, while the Agent records in that same journal contain only message checkpoints, `capability_set` / `capability_delete`, and turn settlements. Sidecar indexes are rebuildable and never own recovery state.

A custom database adapter does not need to implement indexes, recovery actions, or runtime schemas. After restart, unfinished turns are projected as interrupted; Agent does not resume a partial model call or tool execution.

## Canonical product writes

```go
type CanonicalAdapter interface {
    Identity() CapabilityIdentity
    MaterializeInput(context.Context, InputCommitRequest) (CommitReceipt, error)
    CommitOutput(context.Context, OutputCommitRequest) (OutputCommitReceipt, error)
    ApplyEffects(context.Context, []EffectRequest) ([]EffectResult, error)
}

type CanonicalContextAdapter interface {
    CommitContext(context.Context, ContextCommitRequest) (CommitReceipt, error)
}
```

Calls are direct. Implementations must be idempotent for the supplied identity. Hosts that own the canonical conversation implement `CanonicalContextAdapter` so Context State, complete tool-call/result batches, and child-task completion messages enter the same message lane. `ApplyEffects` returns one result per request so a tool batch can report partial item failures without rejecting unrelated items. There is no Agent-owned reconcile callback or host-effect outbox.

## Events and snapshots

Events are live UI signals, not an event-sourced authority. The parent Session assigns an event cursor; nested child events retain child identity for display. Consumers that reconnect inside the same process should read `Session.Snapshot` first and then observe after its cursor.

The snapshot intentionally excludes tool arguments, raw tool results, full prompts, and restore descriptors. Open-tool entries contain only call ID, name, Run, cycle, and source.

## Breaking changes in this unreleased cycle

- Built-in Definition capability constructors now return declarative values instead of `(value, error)`; composition errors are joined by component path at the Agent boundary.
- Removed public limits and file-store tuning options.
- Removed exact replay, cold recovery actions, reconcile callbacks, runtime checkpoints, command indexes, and host-effect receipts.
- Removed recovery-specific event types and task observation fields.
- Standalone file persistence uses one self-contained Session JSONL. Denova embeds root Agent records in the owning Product Session or Story JSONL and keeps persistent child Sessions below the same Project Store; no Denova-wide Agent transcript root is created.
- Unfinished work after restart becomes interrupted and must be retried as a new Run.

These are intentional Beta changes. Workspace content and product-owned Writing/Game conversation data are not deleted or rewritten.
