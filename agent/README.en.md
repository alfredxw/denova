# Denova Agent

English | [简体中文](README.md)

`agent` is a provider-neutral, composable Go runtime. Its public mental model has only three layers:

- `Agent` owns the Definition source, Session Store, and process lifetime.
- `Session` is a stable conversation that persists transcript and capability state and serializes Runs.
- `Run` is one in-process execution with streaming events, steering, queues, interactions, and abort control.

## Quickstart

A static `Definition` implements `Source` itself. Put the built-in model, Definition, and configuration directly in `agent.New`; composition errors have one handling point:

```go
func answer(ctx context.Context) error {
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
        return err
    }
    defer assistant.Close(context.Background())

    run, err := assistant.Run(ctx, agent.Text("Draft an opening paragraph."))
    if err != nil {
        return err
    }
    for event := range run.Events() {
        if delta, ok := event.Payload.(agent.AssistantDelta); ok {
            fmt.Print(delta.Delta)
        }
    }
    _, err = run.Wait(ctx)
    return err
}
```

`assistant.Run` uses a temporary in-memory Session. For a continuing conversation, open a stable Session with `assistant.Session(ctx, agent.NamedSession("draft"))`.

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
    agent.WithSessionStore(sessionfile.New("/var/lib/myapp/agent-transcripts")),
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

## Runtime and persistence semantics

- A Session executes one Run at a time. `Steer` changes the current Run, `Queue` adds work inside it, `FollowUp` creates a serial next turn, and `Abort` stops it.
- While the backend process remains alive, a browser refresh can reconnect through `Session.Snapshot`, `AttachRun`, and `Observe`.
- The file Store persists provider-neutral transcript, capability state, and minimal turn boundaries. Live events, queues, tool stacks, and Interaction waiters remain process-local.
- Completed transcript survives a backend restart. An unfinished turn becomes `incomplete`; old tool calls are not replayed exactly.
- Agent Session Store owns model conversation state, Canonical Adapter owns product facts, and Trace is observational only.

See [`docs/agent-package-api-design.md`](../docs/agent-package-api-design.md) and [`docs/agent-architecture.md`](../docs/agent-architecture.md) for detailed API and architecture notes.
