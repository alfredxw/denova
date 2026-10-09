# Denova Agent

English | [简体中文](README.md)

An Agent runtime you can embed in Go applications. Start with one call, then add conversations and tools.

## 1. Quickstart: complete one task

Requires Go 1.26.6 or later. Install in your own Go module:

```sh
go get github.com/alfredxw/denova/agent
```

Save this as `main.go`, set `OPENAI_API_KEY`, and run `go run .`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/lifecycle/event"
	"github.com/alfredxw/denova/agent/model/providers"
	"github.com/alfredxw/denova/agent/model/providers/builtin"
	"github.com/alfredxw/denova/agent/schema"
)

func main() {
	ctx := context.Background()
	model := builtin.Model(providers.ModelConfig{
		Provider: providers.ProviderOpenAI,
		Model:    "gpt-5",
		APIKey:   os.Getenv("OPENAI_API_KEY"),
	})
	assistant, err := agent.New(ctx, agent.Definition{
		Model:        model,
		Instructions: "Help the user write clear, precise prose.",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer assistant.Close(context.Background())

	run, err := assistant.Run(ctx, agent.Text("Draft an opening paragraph."))
	if err != nil {
		log.Fatal(err)
	}
	for item := range run.Events() {
		if delta, ok := item.Payload.(event.AssistantDelta); ok {
			fmt.Print(delta.Delta)
		}
	}
	result, err := run.Wait(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if result.Status != schema.ResultCompleted {
		log.Fatalf("Run ended with %s: %s", result.Status, result.Reason)
	}
	fmt.Println()
}
```

`agent.New` creates the assistant, `Run` submits a task, `Events` streams progress, and `Wait` returns its result. This uses a temporary conversation whose history is removed afterward.

The next two examples reuse `ctx`, `model`, and the error-handling style above, replacing the assistant construction and subsequent code.

## 2. Session: conversations, interactions, and management

A user requests a paragraph, then asks for a shorter version. Put both tasks in the same Session so the second turn can build on the first.

Also import `github.com/alfredxw/denova/agent/session`, `github.com/alfredxw/denova/agent/tool/builtin` as `tools`, and `github.com/alfredxw/denova/agent/session/file` with the alias `sessionfile`:

```go
store, err := sessionfile.New("./data/sessions")
if err != nil {
	log.Fatal(err)
}
assistant, err := agent.New(ctx, agent.Definition{
	Model:        model,
	Instructions: "Help the user write. Ask when required information is missing.",
	Tools:        tools.Ask(),
}, agent.WithSessionStore(store))
if err != nil {
	log.Fatal(err)
}
defer assistant.Close(context.Background())

key := session.Named("draft-42")
conversation, err := assistant.Session(ctx, key)
if err != nil {
	log.Fatal(err)
}

for _, prompt := range []string{
	"Draft an opening paragraph for first-time readers.",
	"Make it more concise, keeping the same audience.",
} {
	run, err := conversation.Run(ctx, agent.Text(prompt))
	if err != nil {
		log.Fatal(err)
	}
	for item := range run.Events() {
		switch payload := item.Payload.(type) {
		case event.AssistantDelta:
			fmt.Print(payload.Delta)
		case event.InteractionRequested:
			showQuestion(payload.Request) // Host UI; answer through Session.Respond.
		}
	}
	result, err := run.Wait(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if result.Status != schema.ResultCompleted {
		log.Fatalf("Run ended with %s: %s", result.Status, result.Reason)
	}
	fmt.Println()
}
```

`showQuestion` represents your own UI display function. It presents the question and returns; a separate UI callback submits the user's answer:

```go
// Called by the UI after the user submits the form.
// response contains the user's answers, permission choice, or cancellation.
_, _, err := conversation.Respond(ctx, request.ID, response)
if err != nil {
	log.Print(err)
}
```

Import `github.com/alfredxw/denova/agent/lifecycle/interaction` for the response type. `response` is an [`interaction.InteractionResponse`](lifecycle/interaction/interaction.go): set `Answers` for ordinary questions (using the request's QuestionID and option Value, or Text), `Permission` for permission choices, or `Cancelled: true` for cancellation. A Run keeps waiting for unanswered questions; calling only `Wait` is insufficient.

A Session executes one Run at a time; finish the previous turn before starting another. The file Store retains the conversation. After restarting, reopen it with the same directory and `session.Named("draft-42")`. Without a Store option, Sessions live only in memory.

**How do you handle user actions during execution?**

These are separate UI actions using the current `run` or `conversation`:

| User action | Call |
| --- | --- |
| “Change direction” | `run.Steer(ctx, agent.Text("Focus on beginners."))` |
| “One more constraint” | `conversation.Queue(ctx, agent.Text("Keep it under 200 words."))` |
| “Translate it afterward” | `conversation.FollowUp(ctx, agent.Text("Then translate it into Chinese."))` |
| “Remove that extra input” | Call `queued.Cancel(ctx, event.QueueControlRequest{})` on the handle returned by Queue |
| “Handle that input now” | `queued.Interrupt(ctx, event.QueueControlRequest{})` |
| “Stop this task” | `run.Abort(ctx, event.AbortRequest{Reason: "User cancelled."})` |

Handle each method's return values and errors. `Queue` only retains input while idle; `FollowUp` starts a new task while idle. For network retries, set a stable `IdempotencyKey` on the Input or control request. A receipt means accepted, not completed.

**Pause and continue:**

```go
// User clicks Pause while a Run is active.
paused, err := assistant.SuspendTree(ctx, key, agent.SuspendRequest{
	RunID: run.ID(), Reason: "User paused the task.",
})
if err != nil {
	log.Fatal(err)
}

// Later, the user clicks Continue. The same RunID gets a new handle.
resumed, err := assistant.ResumeTree(ctx, key, agent.ResumeRequest{
	RunID: paused.RunID,
})
if err != nil {
	log.Fatal(err)
}
// Consume resumed.Events() and check resumed.Wait(ctx), as above.
fmt.Println(resumed.ID())
```

Suspension closes the old Session handle; reacquire it with `assistant.Session(ctx, key)` when needed. Recreate the assistant first after restarting. Opening a Session does not resume execution. `SuspendTree` / `ResumeTree` include child tasks; use `AbortTree` to cancel the whole tree.

**Reconnect a page and manage conversations:**

| Need | Call and meaning |
| --- | --- |
| Restore the page | `conversation.Snapshot(ctx)` returns current state and pending questions; subscribe with `Observe(ctx, snapshot.Cursor)` and consume both Events and Errors |
| Retrieve a Run handle | `conversation.AttachRun(ctx, runID)` attaches without resuming |
| List conversations | `assistant.ListSessions(ctx, session.Selector{All: true})` |
| Start another conversation | `assistant.Session(ctx, session.Named("another-conversation-id"))` uses an independent identity and preserves the original conversation |
| Close the conversation | `conversation.Close(ctx)` retains data but terminates the current task; suspend first if it needs continuation |
| Delete the conversation | `conversation.Delete(ctx)` permanently deletes it; call only for an explicit delete action |

Check both the `Wait` error and `Result.Status`: `completed` means finished, `suspended` means paused, and `aborted` means cancelled. A page disconnect should cancel observation; do not bind the entire Agent lifetime to one HTTP request.

## 3. Complete composition: review a real project

Suppose you are building a project assistant: read code and documentation, track checks with Todo, load a review Skill, delegate an independent review, run checks, and produce a report. Questions, permission confirmation, pause, and continuation all reuse example 2.

Most changes stay in `Definition`: compose the capabilities you need.

This **integration example** uses dependencies supplied by your application:

- `projectRoot` is the project directory, `model` comes from example 1, and `contextTokens` is the actual model capacity.
- `projectContext` supplies project rules and state; `skillCatalog` lists available Skills, and `skillLoader` loads their content.
- `commandRunner` executes commands, `taskExecutor` handles child tasks, and `artifactStorage` retains complete tool output. Their interfaces and existing implementations are linked below the code.

Also import the `agent` module's `context/compaction`, `engine/execution`, `engine/goal`, `tool/permission`, and `tool/result` (aliased as `toolresult`), plus `context` aliased as `agentcontext`, and reuse `session`, `tools`, and `sessionfile` from example 2.

```go
store, err := sessionfile.New("./data/sessions")
if err != nil {
	log.Fatal(err)
}
assistant, err := agent.New(ctx, agent.Definition{
	Model: model,
	Instructions: "Check whether the project documentation matches the code. " +
		"Use Todo to track work, delegate independent review, run relevant checks, " +
		"and report findings with evidence. Ask when required information is missing. " +
		"Available Skills:\n" + skillCatalog,
	Context: projectContext,
	Tools: tools.Combine(
		tools.Workspace(tools.WorkspaceConfig{
			Root: projectRoot, Access: tools.WorkspaceReadOnly,
		}),
		tools.Shell(tools.ShellConfig{Runner: commandRunner}),
		tools.Todo(),
		tools.Ask(),
		tools.Skills(skillLoader),
		tools.Tasks(taskExecutor),
	),
	Permission: permission.SafeDefault(),
	Goal:       goal.Standard(),
	Artifacts:  artifactStorage,
	ResultProcessor: toolresult.Standard(toolresult.Policy{
		MaxBytes: 128 << 10, ContextWindowTokens: contextTokens,
	}),
	Compaction: compaction.Standard(compaction.StandardConfig{
		ContextWindowTokens: contextTokens,
	}),
	Elision: &agentcontext.ElisionPolicy{ContextWindowTokens: contextTokens},
	Execution: execution.ExecutionPolicy{ToolParallelism: 4},
}, agent.WithSessionStore(store))
if err != nil {
	log.Fatal(err)
}
defer assistant.Close(context.Background())

conversation, err := assistant.Session(ctx, session.Named("project-review-42"))
if err != nil {
	log.Fatal(err)
}
run, err := conversation.Run(ctx, agent.Input{
	Text: "Review this project's documentation, check its examples, and report inconsistencies.",
	Goal: &schema.GoalMutation{
		Kind:      schema.GoalSet,
		Objective: "Complete the documentation review with evidence and a list of unresolved issues.",
	},
})
if err != nil {
	log.Fatal(err)
}
// Reuse example 2's event loop, interaction handling, and result check.
fmt.Println(run.ID())
```

This configuration supports the following workflow:

1. **Gather evidence**: Context provides project background, Workspace reads and searches files, and Skills loads review instructions.
2. **Advance the task**: Todo tracks steps, Tasks delegates reviews, Shell runs checks, and Goal retains the objective.
3. **Collaborate with the user**: Ask requests missing information; Permission decides which tool operations require confirmation. Answers still use `Session.Respond`.
4. **Handle long tasks**: Artifacts retains full output, ResultProcessor bounds individual and aggregate tool output, and one incremental Compaction checkpoint folds completed steps. A single user Run can compact repeatedly while keeping its instructions and newest original results.
5. **Retain progress**: the Session Store persists conversation and recovery records for later continuation.

**Start with these interfaces for application integration:**

| Dependency | Interface / reusable implementation |
| --- | --- |
| Project context | [`agentcontext.ContextSource`](context/definition.go): attribute each fragment's source, purpose, and capacity; inject stable rules separately from changing state |
| Skills | [`tools.SkillLoader`](tool/builtin/skill_tool.go): load full content by name; the application supplies the catalog |
| Commands | [`tools.CommandRunner`](tool/builtin/shell_tools.go); use [`NewLocalCommandRunner`](tool/builtin/shell_local_runner.go) for local execution |
| Child tasks | [`tools.TaskExecutor`](tool/builtin/task_tool.go); use [`NewLocalTasks`](tool/builtin/task_local_executor.go) for local execution |
| Large result storage | [`tool.ToolArtifactStorage`](tool/tool_artifact.go) |
| Compaction customization (optional) | Uses the active model by default; replace [`compaction.Summarizer`](context/compaction/standard.go) or implement [`compaction.CompactionManager`](context/compaction/compaction.go) |

For writing, research, or support, primarily replace Instructions, Context, Skills, and Tools; remove capabilities you do not need. For file editing, select `WorkspaceReadWrite` and supply a `MutationAdapter`. A read-only workspace does not constrain Shell; command permissions still belong to the runner and permission policy.

Use the static Definition above for simple cases. Implement [`Source`](engine/definition.go) when models and capabilities must vary by Session. For existing product conversation storage, integrate [`CanonicalAdapter`](session/canonical/canonical.go) and [`session.Store`](session/store.go) so product history and Agent recovery records share one journal.

Query historical results with `Session.CommandSnapshot(ctx, commandID)` or `RunSnapshot(ctx, runID)`. These reads neither start work nor require historical Runs to remain resident. After durable settlement, Agent releases execution state while the canonical journal retains the full history. `AttachRun` returns a caller-owned terminal view for settled work; suspended work retains its recovery facts. Storage adapters may implement [`RecoveryLog`](lifecycle/lifecycle_session.go) with a rebuildable `RecoveryIndex` and revision-based record reads to accelerate cold recovery. Historical index entries contain no input, output, or tool-result bodies. Ordinary `session.Log` implementations still recover through streaming replay; the built-in file store collects transient record locations during replay, then reads only the target transaction for historical lookups.

### Three levels of compaction integration

```go
// Default planning and summarization use the active Agent model snapshot.
Compaction: compaction.Standard(compaction.StandardConfig{ContextWindowTokens: 128_000})

// Customize summary generation while retaining the standard policy.
Compaction: compaction.Standard(compaction.StandardConfig{
    ContextWindowTokens: 128_000,
    Summarizer: compaction.SummarizerFunc{
        Capability: schema.CapabilityIdentity{Kind: "app.summary", Version: 1},
        Func: func(ctx context.Context, input compaction.SummaryRequest) (compaction.CompactionCheckpoint, error) {
            return summarizeSelectedSource(ctx, input.Messages, input.Current)
        },
    },
})

// Customize triggering, selection and generation through the same runtime.
Compaction: myCompactionManager
```

`Standard` retains the most recent complete interaction and generates summaries with the active model snapshot. If capacity is insufficient, it processes the source in ordered batches; model failures do not silently switch execution modes. `Prompt` adds domain guidance. `ModelSummarizer` supports a replacement model with a stable Identity.

A custom manager's `Plan` receives complete interaction groups and the final model snapshot, then selects a prefix through `GroupCount`. During that `Plan` call, `EstimateAfter(GroupCount)` measures the projected complete request, including protected inputs, tool schemas and images but excluding the new summary; reserve its budget separately. The built-in policy selects history against the token recovery target and preserves native image inputs in cold summary batches. The final request is validated again before checkpoint publication. `Compact` receives only the previous checkpoint and newly selected material. Both extension points return `CompactionCheckpoint{Summary, ContextData}`. Optional ContextData is typed, versioned JSON (at most 8 MiB), persisted atomically with the summary and never injected into the model automatically.

Agent protects current user instructions, the newest complete tool group, and unfinished steps; measures final request capacity and actual progress; and owns journal commits, revision, cancellation, and recovery. Read the checkpoint view through `Session.Snapshot().Compaction`; detailed diagnostics live in `Inspect().CompactionMetrics` and compaction events. Runtime-issued views can `Project` effective history. Serialized display data does not carry history projection authority.

### Tool-result elision before summarization

`Definition.Elision` independently enables deterministic tool-result elision. At the default 60% window pressure, including request reserves, it replaces stale recoverable bodies and rebuilds the exact model request before Compaction decides whether a summary is still needed. It preserves the newest two complete steps, a recent history tail covering at least 30% of the window, user instructions, images, errors, protected results and unrecoverable outputs. Replay recovery requires complete, unredacted arguments and an available read-only tool. Complete artifacts use ordinary read access and retain operation receipts; recovering output must not repeat side effects.

Selection starts with the newest eligible groups to retain a longer cache prefix. Each result must save at least 256 tokens and each batch at least `max(256, 1% of the window)`, verified on the rebuilt provider request with an unchanged stable prefix. Raw history is never rewritten. An `agent.elision` v1 record in the same canonical journal stores only message coordinates and source hashes (at most 4096 entries; a Compaction boundary ignores coordinates it has absorbed), without copied bodies or host paths. Pause, cold restart, manual compaction and inspection share the projection; Clear or canonical history replacement invalidates it. `Inspect().ElisionMetrics` reports the last committed savings and estimated cache prefix.

Denova's native writing, game and persistent child Agents share the existing automatic-compaction switch. The soft trigger scales with the summary trigger: 60% elision before 85% summarization by default. SDK callers use `Elision: nil` to disable new elisions; committed projections remain active. Latest-release user data needs no migration; legacy Cleanup records remain historical diagnostics only.

### Native image input

[`Attachment`](schema/attachment.go) carries user images and `ToolResult.Attachments` carries tool images. `InputSize.Tokens` includes visual tokens; `InputSize.Bytes` counts the JSON envelope without image Base64. Both compaction and final input checks share these estimates. Custom models can implement [`model.ModelInputEstimator`](model/input_estimate.go); unknown models estimate 1,844 tokens per image.

The `read` tool in `tools.Workspace` reads UTF-8 text and PNG, JPEG, GIF, or WebP images (up to 20 MiB each). Image reads require `Definition.Artifacts` with local path resolution: the tool saves an immutable snapshot before returning native image input. User attachment paths are relative to `AttachmentRoot`; tool image paths are relative to their artifact storage boundary. Journals retain relative references and SHA256, never runtime absolute paths or Base64. Responses and Anthropic include images inside tool results; Chat Completions projects image messages after the entire tool result batch to preserve call pairing.

Built-in protocol adapters verify the original image SHA256 before sending and create in-memory resized copies according to known model rules. They retain the original bytes when resizing is unnecessary, preserve JPEG display orientation, and use the first GIF frame. Local conversion accepts source images up to 64 megapixels to bound decoding memory. Original files, attachment paths, and journals remain unchanged. Adapters enforce image and actual HTTP request limits for official endpoints; custom gateways report their own limits. Invalid images and oversized requests return stable error codes for the host application to localize.

## 4. Package responsibilities and extension points

The root [`agent.go`](agent.go) exposes `New`, `Definition / Source`, `Agent / Session / Run`, and common input/output values. It is a convenience entry point whose type aliases preserve handle identity. Implementations and extension contracts belong to these domain packages.

| Top-level package | Responsibility and subpackages |
| --- | --- |
| `lifecycle` | Session / Run, admission, queues, task trees, and journal commits; `event`, `interaction`, and `trace` own events, interaction, and diagnostics |
| `engine` | Definition preparation, model/tool cycles, and checkpoints; `middleware`, `execution`, and `goal` own execution extensions, policy, and goals |
| `model` | Model contracts and request snapshots; `providers` adapts protocols and `stream` supplies streams |
| `tool` | Tool contracts and registration; `builtin`, `permission`, `result`, `script`, and `plugins` supply tools and supporting capabilities |
| `context` | Context sources; `history` computes history projections, coordinates, and elision; `compaction` supplies compaction policy |
| `session` | Journal storage contracts; `file` implements file storage and `canonical` integrates atomic product commits |
| `schema` | Shared messages, inputs, attachments, and result values |
| `state` | Live file-backed Agent configuration, validation, and atomic management writes |

The core dependency direction is `agent → lifecycle → engine → context/history`. Lifecycle owns admission, concurrency, and journal commits. Each execution request supplies a narrow commit interface; lifecycle commits complete checkpoints without interpreting their private fields. History holds no Session handles, locks, or storage. `internal` contains async, retry, filesystem, and protocol implementation details plus integration tests.

Use `agent.New` for ordinary integration, or `lifecycle.New` to depend explicitly on the lifecycle layer. Hosts with their own scheduling and storage can use the public [`engine.New(engine.Config{Source: definition, Session: key})`](engine/definition_engine.go). Direct engine callers must:

- Serialize cycles for each Session and keep command, operation, cycle, and recovery snapshot identities consistent.
- Complete canonical input materialization and admission before execution. When those capabilities are configured, call `PlanInputMaterialization` / `MaterializeInput` and `PrepareAdmission`.
- Persist required state before `EventSink` acknowledges success. `TranscriptUpdated`, `AssistantFinal.State`, and capability updates form recovery state; consuming only text events is insufficient.
- Implement `CanonicalHost` to commit product changes and recovery checkpoints atomically. Omit it only when no product canonical journal is used.

The [public API integration test](internal/integration/engine_test.go) demonstrates opaque checkpoint persistence, continuing a conversation with a newly created engine, and stopping before model execution when a journal rejects a checkpoint. Engine does not own host queues, task trees, or external runtime selection.

This package refactor preserves journal versions, JSON fields, capability identifiers, and data paths; existing data needs no migration. SDK callers must update imports to the paths above. The module does not retain forwarding packages at old paths.

### Development and tests

Unit tests live alongside their implementations. Integration tests covering compaction, continuation, images, and tool-result elision through public APIs live in [`internal/integration`](internal/integration). Run these commands from `agent/`:

```sh
go test ./...
go vet ./...
```

Run only those integration tests with `go test ./internal/integration`. They are also included in `go test ./...` and the existing CI without additional build tags.
