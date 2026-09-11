# Denova Agent

English | [简体中文](README.md)

`agent` is a provider-neutral, composable Go runtime. Its public mental model has only three layers:

- `Agent` owns the Definition source, Session Store, and process lifetime.
- `Session` is a stable conversation that persists transcript, capability state, and recovery facts and serializes Runs.
- `Run` is one logical task. Its current process handle provides streaming events and controls; after suspension, a new handle can continue the same RunID.

## Quickstart: run an Agent once

`agent` is a Go library embedded by a host application, not a standalone CLI or Session Server. The host creates an Agent with `agent.New`, submits input through `Run`, and consumes events or waits for the final result.

The following minimal example can be saved as `main.go` and run directly. A static `Definition` implements `Source` itself:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/alfredxw/denova/agent"
    "github.com/alfredxw/denova/agent/providers"
    "github.com/alfredxw/denova/agent/providers/builtin"
)

func main() {
    ctx := context.Background()
    assistant, err := agent.New(ctx, agent.Definition{
        Key:  "writer.v1",
        Name: "writer",
        Model: builtin.Model(providers.ModelConfig{
            Provider: providers.ProviderOpenAI,
            Model:    "gpt-5",
            APIKey:   os.Getenv("OPENAI_API_KEY"),
        }),
        Instructions: "Help the user write clear, precise prose.",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer func() {
        if err := assistant.Close(context.Background()); err != nil {
            log.Printf("close Agent: %v", err)
        }
    }()

    run, err := assistant.Run(ctx, agent.Text("Draft an opening paragraph."))
    if err != nil {
        log.Fatal(err)
    }

    for event := range run.Events() {
        if delta, ok := event.Payload.(agent.AssistantDelta); ok {
            fmt.Print(delta.Delta)
        }
    }

    result, err := run.Wait(ctx)
    if err != nil {
        log.Fatal(err)
    }
    if result.Status != agent.ResultCompleted {
        log.Fatalf("Agent Run ended with %s: %s", result.Status, result.Reason)
    }
    fmt.Println()
}
```

Save the file in a Go module, set `OPENAI_API_KEY`, and run:

```sh
go get github.com/alfredxw/denova/agent
go run .
```

`assistant.Run` creates a temporary Session and deletes it after the Run settles, which is appropriate for one-shot work that does not need conversation history. If streaming output is unnecessary, the host can call `run.Wait(ctx)` directly. A terminal or UI should continuously consume `run.Events()`.

## Continuing conversations and durable Sessions

For a multi-turn conversation, open a stable Session and run each turn on that Session. The default Store is in-memory only. To retain transcript, capability state, accepted input, and task checkpoints across process restarts, explicitly configure the file Store or provide a custom durable Store:

```go
import (
    "context"
    "fmt"

    "github.com/alfredxw/denova/agent"
    sessionfile "github.com/alfredxw/denova/agent/session/file"
)

func continueConversation(ctx context.Context, definition agent.Definition) error {
    owner, err := agent.New(ctx, definition,
        agent.WithSessionStore(sessionfile.New("./data/agent-sessions")),
    )
    if err != nil {
        return err
    }
    defer owner.Close(context.Background())

    key := agent.SessionKey{
        Namespace: "example.conversation",
        ID:        "conversation-42",
        Attributes: map[string]string{
            "project_id": "project-7",
        },
    }
    conversation, err := owner.Session(ctx, key)
    if err != nil {
        return err
    }

    first, err := conversation.Run(ctx, agent.Text("Draft an opening paragraph."))
    if err != nil {
        return err
    }
    firstResult, err := first.Wait(ctx)
    if err != nil {
        return err
    }
    if firstResult.Status != agent.ResultCompleted {
        return fmt.Errorf("first Agent Run ended with %s: %s", firstResult.Status, firstResult.Reason)
    }

    second, err := conversation.Run(ctx, agent.Text("Make it more concise."))
    if err != nil {
        return err
    }
    result, err := second.Wait(ctx)
    if err != nil {
        return err
    }
    if result.Status != agent.ResultCompleted {
        return fmt.Errorf("second Agent Run ended with %s: %s", result.Status, result.Reason)
    }
    return nil
}
```

A Session has at most one active Run. Wait for the current Run to settle before calling `Session.Run` again, or the call returns `agent.ErrSessionBusy`. To submit input while the current Run is still active, use `Queue`, `Steer`, or `FollowUp` as described below.

The Session Key is its durable identity. `Namespace`, `ID`, and `Attributes` all participate in identity, so do not put mutable display names in them. After the host restarts, call `owner.Session` with the same Store and Key to continue the completed conversation. For a simple stable key in the default namespace, use `agent.NamedSession("draft")`.

## Durable input and execution controls

Sessions accept distinct follow-up tasks and supplemental input; steering still targets the current Run. Supply a stable `IdempotencyKey` for inputs and controls that may be retried across a network boundary: reuse it for the same operation and choose a new value for each independent operation. Changing input under the same key returns `agent.ErrIdempotencyConflict`.

| Intent | API | Semantics |
| --- | --- | --- |
| Correct the current direction | `run.Steer(ctx, input)` | Durably accepts input, returns `CommandReceipt`, and preempts at a safe boundary; a suspended Run only receives input |
| Add supplemental input | `session.Queue(ctx, input)` | Returns `*QueuedInput`; an idle Session keeps input for the next explicit activation without starting a Run |
| Create a follow-up task | `session.FollowUp(ctx, input)` | Returns `CommandReceipt`; starts when idle and queues when busy or suspended |
| Retrieve supplemental input | `session.Queued(ctx, commandID)` | Returns the handle, a found flag, and an error; remains queryable after reopening |
| Cancel / promote supplemental input | `queued.Cancel(ctx, request)` / `queued.Interrupt(ctx, request)` | Takes `QueueControlRequest` and returns a control receipt; promotion does not implicitly resume a suspended task |
| Terminate the current task | `run.Abort(ctx, request)` | Takes `AbortRequest` and terminates this Run; use `owner.AbortTree` for the entire task tree |
| Answer an interaction | `session.Respond(ctx, interactionID, response)` | Returns the request, accepted resolution, and error; works after reopening without starting execution |

For example, persist supplemental input and accept a distinct task after the current one:

```go
queued, err := conversation.Queue(ctx, agent.Input{
    Text:           "Keep the narrator's point of view consistent.",
    IdempotencyKey: "message-42",
})
if err != nil {
    return err
}
fmt.Println(queued.Receipt().CommandID)

receipt, err := conversation.FollowUp(ctx, agent.Input{
    Text:           "Then review the draft for repetition.",
    IdempotencyKey: "task-43",
})
if err != nil {
    return err
}
fmt.Println(receipt.CommandID, receipt.RunID)
```

A `CommandReceipt` proves acceptance, not that a model call has begun. Retain its CommandID / RunID and inspect progress through `Snapshot`, `Observe`, or `AttachRun`; do not generate a new key for an HTTP retry. The queue accepts at most 1024 items and 64 MiB in total, with a 16 MiB serialized limit per input. Exceeding a limit rejects input instead of truncating it.

`run.Events()` emits typed events such as `AssistantDelta`, tool state, interaction requests, and terminal state. `run.Wait(ctx)` returns the current handle's `Result` and `error`. Execution or storage failures return an error; `failed`, `incomplete`, and `blocked` also return an `*agent.RunError`. Still inspect `Result.Status`: `aborted` is normally not a Go error, and `suspended` means the logical task has not settled.

## Reconnecting a page and restarting a process

When a page reloads while the Agent host process is still running, rebuild the UI from Session-level observation:

```go
snapshot, err := conversation.Snapshot(ctx)
if err != nil {
    return err
}
renderSnapshot(snapshot)

observeCtx, cancelObserve := context.WithCancel(ctx)
defer cancelObserve()
observation, err := conversation.Observe(observeCtx, snapshot.Cursor)
if err != nil {
    return err
}
// Apply observation.Events in Cursor order and concurrently consume
// observation.Errors until observeCtx is cancelled. The event stream first
// catches up events after snapshot.Cursor, then remains live.

if runID := observation.Snapshot.ActiveRunID; runID != "" {
    active, found, err := conversation.AttachRun(ctx, runID)
    if err != nil {
        return err
    }
    if found {
        _ = active // Inspect or control this handle; attaching does not resume it.
    }
}
```

`Snapshot` is the baseline for UI restoration. `Observe` catches up process-local events after a Cursor and continues delivering new events. `Observation.Snapshot` is a newer view captured when observation begins; if the UI replaces the initial snapshot with it, ignore replayed events whose Cursor is not greater than that newer snapshot's Cursor to avoid applying them twice. After a process restart, start from a fresh Snapshot rather than treating old live events as a durable log.

Opening a Session restores accepted input, pending interactions, tool results, and task checkpoints from its journal; `AttachRun` can retrieve the restored handle. Opening, observing, and attaching never start execution. Inspect `ActiveStatus`, `ActiveRunID`, and `PendingInteractions`, and let the user explicitly continue or cancel.

## Pausing and continuing the original task

Use `Agent.SuspendTree` for a product's whole-task pause action. It prevents new model, tool, and child-task admission, stops descendants, then releases the root Session writer. The same entry point works without descendants:

```go
paused, err := owner.SuspendTree(ctx, key, agent.SuspendRequest{
    RunID:          run.ID(),
    Reason:         "User paused the task.",
    IdempotencyKey: "pause-44",
})
if err != nil {
    return err
}

// Call only after the user chooses to continue. After a process restart,
// recreate owner with the same Store, Source, and stable Session key first.
resumed, err := owner.ResumeTree(ctx, key, agent.ResumeRequest{
    RunID:          paused.RunID,
    IdempotencyKey: "resume-45",
})
if err != nil {
    return err
}
result, err := resumed.Wait(ctx)
if err != nil {
    return err
}
fmt.Println(resumed.ID(), result.Status)
```

- `Session.SuspendAndClose` / `Session.ResumeRun` control one Session only; reopen it with the same key after suspension. Pair a tree suspension with `ResumeTree` rather than bypassing its admission barrier through individual Sessions.
- Successful suspension means producers have stopped, checkpoints are durable, and the writer is released. Model waits and backoff are cancellable; `finish_current` tools must finish before suspension succeeds. Partial failures return errors; retain the actual state and retry the original command.
- The original handle's Wait returns `ResultSuspended`, without `RunSettled` or a child-task completion notification. Resume returns a new handle with the same RunID, Cycle, and consumed input; establish new event subscriptions.
- Resume rebuilds dependencies through the current Source and validates Definition and product identity. Incompatible model or capability configuration fails explicitly; do not mutate the old SessionKey or HostData to bind another task.
- `owner.AbortTree(ctx, key, agent.AbortRequest{...})` terminates the root and unfinished descendants and returns `CommandReceipt`. It also works while suspended or awaiting verification. Ordinary `Close` is not a pause API.

Recovery does not preserve Go stacks, partial model streams, or tool-internal progress, and does not guarantee identical generated text. Confirmed tool results are reused; unconfirmed external writes require verification instead of blind replay.

## Durable interactions and uncertain tool effects

Ordinary Ask requests and answers are durable. After reopening, restore forms from `Snapshot.PendingInteractions` and answer through `Session.Respond`. Repeating the same answer returns the accepted resolution; a different answer cannot overwrite a settled one. Answering while suspended does not resume the task.

`InteractionRequest.Verification != nil` identifies an external operation that started without a confirmed result. Display `Verification.Tool` and its original `Arguments` for user verification. Submit stable question IDs and option values from the request:

```go
// request comes from Snapshot.PendingInteractions or InteractionRequested.
// Replace this value only with the user's verified choice.
choice := "unknown"
_, _, err := conversation.Respond(ctx, request.ID, agent.InteractionResponse{
    Answers: []agent.InteractionAnswer{{
        QuestionID: "effect",
        Values:     []string{choice},
    }},
})
if err != nil {
    return err
}
```

| Choice | Effect |
| --- | --- |
| `executed` | Records the user's confirmation; explicitly reports unavailable original result details without fabricating output or repeating the operation |
| `not_executed` | Records that the effect did not occur; if still needed, a later model call requests the operation through normal tool execution and permission checks |
| `unknown` or cancelling this verification question | Keeps the task waiting; the user can still cancel the entire task directly |

All model and tool admission for the Run remains closed until verification completes. Queue, Steer, and Resume cannot bypass it. Cancelling an ordinary Ask follows that tool's interaction contract. Arbitrary external tools have no universal exactly-once guarantee.

## Continuing work in the same child Session

Durable child Sessions in `tools.LocalTasks` are addressed by `TaskRef{Agent, Session, Run}`. Retain returned references; when the same Session accepts a new task, use its newly returned Run reference:

| API | Semantics |
| --- | --- |
| `tasks.FollowUp(ctx, ref, input)` | Returns `Task` and accepts a new Run in the existing child Session; queues while busy |
| `tasks.SendMessage(ctx, ref, input)` | Returns `CommandReceipt` for supplemental input only; idle or suspended tasks do not start |
| `tasks.Resume(ctx, ref, request)` | Returns `Task` and continues the referenced Run; takes `agent.ResumeRequest` |

For tree controls, parent and child Sessions must share an Agent / Store that can rebuild their respective Definitions. Merge `agent.ChildSessionAttributes(parent.Key())` into `LocalTaskAgent.Attributes` and retain stable parent selectors in `LookupAttributes`; setting `CompletionParent` alone does not establish this relationship.

Child SessionKeys contain stable identity only. `FollowUp` captures HostData from the current parent invocation, whereas `Resume` retains the original Run's HostData, so subsequent tasks do not reuse the first invocation's product route. Previously allocated keys and filenames stay unchanged. `tools.Tasks(tasks)` exposes the corresponding `follow_up`, `send_message`, and `resume` actions to the model, with per-item results for batches; an input receipt is not task completion.

## Model retries and output repair

The library attempts each model response once by default. To enable transient retries, configure both the attempt budget and a stable policy identity:

```go
definition.Execution = agent.ExecutionPolicy{
    ModelMaxAttempts: 4,
    Retry:           &agent.RetryConfig{Decide: agent.TransientRetry},
    RetryIdentity:   agent.CapabilityIdentity{Kind: "example.retry.transient", Version: 1},
    ToolParallelism: 4,
}
```

`ModelMaxAttempts` includes the first call and shares one budget between network failures and business repair for a logical model response; `0` means `1`. `RetryConfig.Decide` returns only an action, delay, and reason, without replacing the model, request, or history. The built-in policy supports cancellable backoff, jitter, and Retry-After, and rejects authentication and permanent quota failures. Built-in provider adapters disable implicit SDK retries.

Business validation returns `ModelOutputReview` from `Middleware.ReviewModelOutput(ctx, output)`: `ModelOutputAccept` accepts the response, while `ModelOutputRepair` supplies attributed, bounded `ContextFragment` feedback. For the same response, new feedback replaces that middleware's previous feedback; its total limit is 64 KiB and exceeding it returns an error. Repair shares the attempt budget with network failures, and tools from unaccepted responses never execute.

UIs distinguish attempts by `AssistantDelta.ResponseOrdinal`. On `ModelRetry`, discard that attempt's failed preview instead of appending it to the next answer. Only accepted output enters the canonical transcript. Independent calls such as compaction can use `ModelRequestSnapshot.Complete(ctx, execution)` to reuse the attempt mechanism; each side call counts separately, buffers output until success, and retains its model. This budget is not a total Run duration or iteration limit.

## Managing Sessions

| Operation | Effect |
| --- | --- |
| `owner.ListSessions(ctx, selector)` | Lists matching Session Keys in the Store |
| `owner.CountActiveSessions(ctx, selector)` | Counts matching active Runs in the current process |
| `session.Close(ctx)` | Terminates the active Run and releases its handle and Store lease while retaining data; explicitly suspend first to continue the original task |
| `session.Clear(ctx)` | Keeps Session identity and starts a blank conversation; clears Todo and retains Goal |
| `session.Delete(ctx)` | Closes the Session and permanently deletes its transcript |
| `owner.CloseSessions(ctx, selector)` | Closes matching Sessions and their child Sessions while retaining durable data |
| `owner.DeleteSessions(ctx, selector)` | Closes and permanently deletes matching Sessions and their child Sessions |
| `owner.Close(ctx)` | Closes every in-process Session and releases Store leases; explicitly suspend tasks that need continuation before shutdown |

Non-empty Selector fields use AND semantics. `IDPrefix` performs prefix matching only when explicitly populated. Deletion rejects an unconstrained or `All` Selector to prevent accidental removal of every conversation:

```go
selector := agent.SessionSelector{
    Namespace: "example.conversation",
    Attributes: map[string]string{
        "project_id": "project-7",
    },
}
keys, err := owner.ListSessions(ctx, selector)
if err != nil {
    return err
}
for _, key := range keys {
    fmt.Println(key.ID)
}

// Close retains transcripts. DeleteSessions is permanent and should be
// exposed behind an explicit user action in the host product.
err = owner.CloseSessions(ctx, selector)
// err = owner.DeleteSessions(ctx, selector)
```

## Complete capability composition

Built-in capability constructors return declarations rather than separate construction errors. This example composes workspace access, Shell, Todo, Ask, Skills, Tasks, Goal, large tool-result processing, Cleanup, Compaction, a file Store, and Trace in one `agent.New`:

```go
owner, err := agent.New(ctx, agent.Definition{
    Key:           "project-agent.v1",
    Name:          "project-agent",
    Model:         builtin.Model(modelConfig),
    Instructions:  "Work from project evidence. Ask only when required input cannot be inferred.",

    Tools: tools.Combine(
        tools.Workspace(tools.WorkspaceConfig{
            Root:   projectRoot,
            Access: tools.WorkspaceReadOnly,
        }),
        tools.Shell(tools.ShellConfig{Runner: commandRunner}),
        tools.Todo(), // Uses durable state in the current Session.
        tools.Ask(),
        tools.Skills(skillSource),
        tools.Tasks(taskExecutor),
    ),
    ResultProcessor: toolresult.Standard(toolresult.Policy{
        MaxBytes:            128 << 10,
        ContextWindowTokens: 128_000,
    }),
    Goal: goal.Standard(),
    Cleanup: cleanup.Standard(cleanup.StandardConfig{
        ContextWindowTokens: 128_000,
        ReservedTokens:      16_000,
        CleanupThreshold:    0.70,
        CompactionThreshold: 0.85,
    }),
    Compaction: compaction.Standard(compaction.StandardConfig{
        Summarizer:        summarizer,
        TriggerBytes:      2 << 20,
        KeepRecentBytes:   512 << 10,
        KeepRecentTurns:   2,
        HardLimitBytes:    4 << 20,
        SummaryLimitBytes: 256 << 10,
    }),
    Execution: agent.ExecutionPolicy{
        ToolParallelism: 4,
        // MaxIterations=0 and IdleTimeout=0 mean unlimited.
    },
},
    agent.WithSessionStore(sessionfile.New("/var/lib/myapp/agent-sessions")),
    agent.WithTrace(agent.TraceFunc(recordTrace)),
)
if err != nil {
    return err
}
```

`modelConfig`, `commandRunner`, `summarizer`, `skillSource`, and `taskExecutor` are host-provided configuration or adapters. `builtin.Model` also derives the stable credential-free model identity during initialization. A read-write workspace requires a product `MutationAdapter`; preserving complete oversized tool results requires a host `ToolArtifactStorage`.

For a static `Definition`, `agent.New` initializes every declarative capability and joins failures with component paths such as `Tools: Toolset[1]`, `Cleanup`, and `Compaction`. Model calls, tool execution, and Session I/O remain runtime errors returned by their respective operations.

## Custom example: per-project composition

Implement `Source` when the model, context, or product Store must vary by Session. `CanonicalInput` resolves only the input-commit adapter, while `Prepare` returns the complete Definition for the cycle. Agent still validates built-in declarations together after Prepare:

```go
type projectSource struct {
    models    ModelRegistry
    projects ProjectRepository
}

func (source *projectSource) CanonicalInput(
    ctx context.Context,
    request agent.PrepareRequest,
) (agent.CanonicalAdapter, error) {
    project, err := source.projects.Open(ctx, request.Session.Key.Attributes["project_id"])
    if err != nil {
        return nil, err
    }
    return project.CanonicalAdapter(), nil
}

func (source *projectSource) Prepare(
    ctx context.Context,
    request agent.PrepareRequest,
) (agent.Definition, error) {
    projectID := request.Session.Key.Attributes["project_id"]
    project, err := source.projects.Open(ctx, projectID)
    if err != nil {
        return agent.Definition{}, err
    }
    model, identity, err := source.models.ForProject(ctx, projectID)
    if err != nil {
        return agent.Definition{}, err
    }
    return agent.Definition{
        Key:           "project-agent.v1",
        Name:          "project-agent",
        Model:         model,
        ModelIdentity: identity,
        Instructions:  "Use the injected project rules and current project state.",
        Context:       project.ContextSource(),
        Tools: tools.Combine(
            project.Toolset(),
            tools.Todo(project.TodoStore()),
            tools.Ask(),
        ),
        Canonical:  project.CanonicalAdapter(),
        Effects:    project.EffectApplier(),
        Permission: project.PermissionPolicy(),
    }, nil
}

owner, err := agent.New(ctx,
    &projectSource{models: models, projects: projects},
    agent.WithSessionStore(projectSessionStore),
)
if err != nil {
    return err
}
session, err := owner.Session(ctx, agent.SessionKey{
    Namespace: "myapp.project",
    ID:        conversationID,
    Attributes: map[string]string{
        "project_id": projectID,
    },
})
```

`projectSessionStore` is a host implementation of `agent/session.Store` that shares the product journal with that project's Canonical Adapter. Every model-visible fragment from a custom `ContextSource` must declare its source, purpose, resource, Revision, stability, placement, and a generous `HardLimit`. Custom Toolsets, Canonical Adapters, Permission policies, Context sources, and model configurations should expose stable, credential-free `CapabilityIdentity` values.

## Storage contracts and ownership boundaries

- A standalone Agent Session journal owns transcript, input, tool confirmations, interactions, and capability recovery state. Indexes and Trace are rebuildable and are not recovery authorities.
- When a product owns the canonical journal, its Session Store should embed Agent records in that same journal rather than maintaining a second recovery history. Input, context, and output commits through `CanonicalAdapter` must preserve `CommitIdentity`, validate the `ExpectedRevision` returned by `Checkpoint`, and atomically append product acceptance and `Records` in one transaction. Invoke the callback once while preparing the transaction, never reenter the same Session, and never return success before appending the checkpoint.
- Product drafts belong to the host. An optional `CanonicalPreparedOutput.PendingOutput` can return a fully accepted draft so recovery uses normal output commit without another model call. Commit final product state and draft completion in the same final product transaction.
- `Definition.Effects` independently handles idempotent tool effects, so a child Agent can keep its own Session transcript while applying changes to the parent Project. Task-tree relationships and per-Run HostData do not change journal ownership.
- `session.ErrCommitUnknown` means the write outcome is uncertain: close the handle, reopen, and replay the journal to establish what committed instead of retrying blindly on the same handle. Custom Stores must uphold their atomic append and writer-ownership contracts.
- Live events, Go stacks, and Interaction waiters belong to the current process; durable queues and accepted answers do not depend on those objects surviving.

## v0.4.5 data and downgrades

Old transcript, capability state, and settled tasks remain readable; a read-only open does not rewrite the journal. Old unfinished tasks without input-receipt facts remain `incomplete`; missing input or checkpoints cannot be invented to resume them.

The strict v0.4.5 reader cannot read new recovery records. Before the first upgraded write to an existing journal, the built-in file Store creates `<journal>.pre-resilience-v1.bak`, preserves that original backup, and refuses the write if backup fails. Hosts that embed records in a product journal must provide the same backup guarantee at their transaction boundary.

Before downgrading, stop the host and preserve a separate copy of the current data directory. Restore the corresponding backup as the original journal and remove its rebuildable index. Content added after backup and Sessions created by the newer version are absent from the old-version recovery result; retain the current data to return to the newer version.
