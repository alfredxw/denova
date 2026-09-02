# Denova Agent

English | [简体中文](README.md)

`agent` is a provider-neutral, composable Go runtime. Its public mental model has only three layers:

- `Agent` owns the Definition source, Session Store, and process lifetime.
- `Session` is a stable conversation that persists transcript and capability state and serializes Runs.
- `Run` is one in-process execution with streaming events, steering, queues, interactions, and abort control.

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

For a multi-turn conversation, open a stable Session and run each turn on that Session. The default Store is in-memory only. To retain the transcript and capability state across process restarts, explicitly configure the file Store or provide a custom Store:

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

## Controlling a live Run

A Run is a process-local handle. Controls apply only while that Run remains live in the current process:

| Intent | API | Semantics |
| --- | --- | --- |
| Correct the current direction immediately | `run.Steer(ctx, input)` | Places input at the front and preempts at the next safe model-loop boundary |
| Add work inside the same Run | `run.Queue(ctx, input)` | Queues input in the current Run; the returned `QueuedInput` supports `Cancel` and `Interrupt` |
| Create the next turn | `run.FollowUp(ctx, input)` | Returns a new Run handle immediately and executes it serially after the current Run |
| Stop current execution | `run.Abort(ctx, request)` | Requests that the Run settle at a safe stopping point |
| Answer an Ask or permission interaction | `run.Respond(ctx, interactionID, response)` | Resolves the current `InteractionRequested` event |

`run.Events()` emits typed events such as `AssistantDelta`, tool state, interaction requests, and terminal state. `run.Wait(ctx)` returns the terminal `Result` and `error`. Execution or storage failures return an error; `failed`, `incomplete`, and `blocked` also return an `*agent.RunError`. Still inspect `Result.Status`, because `aborted` is normally not a Go error.

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
        _ = active // Use the handle for Steer, Queue, Abort, Respond, or Wait.
    }
}
```

`Snapshot` is the baseline for UI restoration. `Observe` catches up process-local events after a Cursor and continues delivering new events. `Observation.Snapshot` is a newer view captured when observation begins; if the UI replaces the initial snapshot with it, ignore replayed events whose Cursor is not greater than that newer snapshot's Cursor to avoid applying them twice. `AttachRun` can reattach only to a Run owned by the current process.

After the backend process restarts, completed transcript is restored from the Store, but live events, queues, tool stacks, and Interaction waiters are not. An unfinished turn is recorded as `incomplete`; the host should display that state and start a new Run when requested instead of replaying old tool calls.

## Managing Sessions

| Operation | Effect |
| --- | --- |
| `owner.ListSessions(ctx, selector)` | Lists matching Session Keys in the Store |
| `owner.CountActiveSessions(ctx, selector)` | Counts matching active Runs in the current process |
| `session.Close(ctx)` | Stops the active Run and releases the handle and Store lease while retaining durable data |
| `session.Clear(ctx)` | Keeps Session identity and starts a blank conversation; clears Todo and retains Goal |
| `session.Delete(ctx)` | Closes the Session and permanently deletes its transcript |
| `owner.CloseSessions(ctx, selector)` | Closes matching Sessions and their child Sessions while retaining durable data |
| `owner.DeleteSessions(ctx, selector)` | Closes and permanently deletes matching Sessions and their child Sessions |
| `owner.Close(ctx)` | Closes every in-process Session and releases Store leases during host shutdown |

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
        Permission: project.PermissionPolicy(),
    }, nil
}

owner, err := agent.New(ctx,
    &projectSource{models: models, projects: projects},
    agent.WithSessionStore(sessionfile.New(transcriptRoot)),
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

Every model-visible fragment from a custom `ContextSource` must declare its source, purpose, resource, Revision, stability, placement, and a generous `HardLimit`. Custom Toolsets, Canonical Adapters, Permission policies, Context sources, and model configurations should expose stable, credential-free `CapabilityIdentity` values.

## Ownership boundaries

- Agent Session Store owns only model conversation transcript and capability state.
- Canonical Adapter owns product facts and their idempotent writes; product state should not be mixed into the Session Store.
- Trace is observational only and is not a recovery or business authority.
- Live Run controls, events, queues, tool stacks, and Interaction waiters belong to the current process.

See [`docs/agent-package-api-design.md`](../docs/agent-package-api-design.md) for more detailed API and design boundaries.
