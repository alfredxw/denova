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
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/builtin"
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
		log.Fatalf("Run ended with %s: %s", result.Status, result.Reason)
	}
	fmt.Println()
}
```

`agent.New` creates the assistant, `Run` submits a task, `Events` streams progress, and `Wait` returns its result. This uses a temporary conversation whose history is removed afterward.

The next two examples reuse `ctx`, `model`, and the error-handling style above, replacing the assistant construction and subsequent code.

## 2. Session: conversations, interactions, and management

A user requests a paragraph, then asks for a shorter version. Put both tasks in the same Session so the second turn can build on the first.

Also import `github.com/alfredxw/denova/agent/tools` and `github.com/alfredxw/denova/agent/session/file` with the alias `sessionfile`:

```go
assistant, err := agent.New(ctx, agent.Definition{
	Model:        model,
	Instructions: "Help the user write. Ask when required information is missing.",
	Tools:        tools.Ask(),
}, agent.WithSessionStore(sessionfile.New("./data/sessions")))
if err != nil {
	log.Fatal(err)
}
defer assistant.Close(context.Background())

key := agent.NamedSession("draft-42")
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
	for event := range run.Events() {
		switch payload := event.Payload.(type) {
		case agent.AssistantDelta:
			fmt.Print(payload.Delta)
		case agent.InteractionRequested:
			showQuestion(payload.Request) // Host UI; answer through Session.Respond.
		}
	}
	result, err := run.Wait(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if result.Status != agent.ResultCompleted {
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

`response` is an `agent.InteractionResponse`: set `Answers` for ordinary questions (using the request's QuestionID and option Value, or Text), `Permission` for permission choices, or `Cancelled: true` for cancellation. A Run keeps waiting for unanswered questions; calling only `Wait` is insufficient.

A Session executes one Run at a time; finish the previous turn before starting another. The file Store retains the conversation. After restarting, reopen it with the same directory and `NamedSession("draft-42")`. Without a Store option, Sessions live only in memory.

**How do you handle user actions during execution?**

These are separate UI actions using the current `run` or `conversation`:

| User action | Call |
| --- | --- |
| “Change direction” | `run.Steer(ctx, agent.Text("Focus on beginners."))` |
| “One more constraint” | `conversation.Queue(ctx, agent.Text("Keep it under 200 words."))` |
| “Translate it afterward” | `conversation.FollowUp(ctx, agent.Text("Then translate it into Chinese."))` |
| “Remove that extra input” | Call `queued.Cancel(ctx, agent.QueueControlRequest{})` on the handle returned by Queue |
| “Handle that input now” | `queued.Interrupt(ctx, agent.QueueControlRequest{})` |
| “Stop this task” | `run.Abort(ctx, agent.AbortRequest{Reason: "User cancelled."})` |

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
| List conversations | `assistant.ListSessions(ctx, agent.SessionSelector{All: true})` |
| Clear the conversation | `conversation.Clear(ctx)` keeps identity, clears Todo, and retains Goal |
| Close the conversation | `conversation.Close(ctx)` retains data but terminates the current task; suspend first if it needs continuation |
| Delete the conversation | `conversation.Delete(ctx)` permanently deletes it; call only for an explicit delete action |

Check both the `Wait` error and `Result.Status`: `completed` means finished, `suspended` means paused, and `aborted` means cancelled. A page disconnect should cancel observation; do not bind the entire Agent lifetime to one HTTP request.

## 3. Complete composition: review a real project

Suppose you are building a project assistant: read code and documentation, track checks with Todo, load a review Skill, delegate an independent review, run checks, and produce a report. Questions, permission confirmation, pause, and continuation all reuse example 2.

Most changes stay in `Definition`: compose the capabilities you need.

This **integration example** uses dependencies supplied by your application:

- `projectRoot` is the project directory, `model` comes from example 1, and `contextTokens` is the actual model capacity (greater than the 16,000 reserved tokens in this example).
- `projectContext` supplies project rules and state; `skillCatalog` lists available Skills, and `skillLoader` loads their content.
- `commandRunner` executes commands, `taskExecutor` handles child tasks, `artifactStorage` retains complete tool output, and `summarizer` summarizes conversations. Their interfaces and existing implementations are linked below the code.

Also import the `agent` module's `cleanup`, `compaction`, `goal`, `permission`, and `toolresult` packages, plus `tools` and `sessionfile` from example 2.

```go
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
	Cleanup: cleanup.Standard(cleanup.StandardConfig{
		ContextWindowTokens: contextTokens, ReservedTokens: 16_000,
		CleanupThreshold: 0.70, CompactionThreshold: 0.85,
	}),
	Compaction: compaction.Standard(compaction.StandardConfig{
		Summarizer:          summarizer,
		ContextWindowTokens: contextTokens, ReservedTokens: 16_000,
		HardLimitBytes: 4 << 20, SummaryLimitBytes: 256 << 10,
	}),
	Execution: agent.ExecutionPolicy{ToolParallelism: 4},
}, agent.WithSessionStore(sessionfile.New("./data/sessions")))
if err != nil {
	log.Fatal(err)
}
defer assistant.Close(context.Background())

conversation, err := assistant.Session(ctx, agent.NamedSession("project-review-42"))
if err != nil {
	log.Fatal(err)
}
run, err := conversation.Run(ctx, agent.Input{
	Text: "Review this project's documentation, check its examples, and report inconsistencies.",
	Goal: &agent.GoalMutation{
		Kind:      agent.GoalSet,
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
4. **Handle long tasks**: Artifacts retains full output, ResultProcessor bounds individual model-visible results, and Cleanup / Compaction manage accumulated context.
5. **Retain progress**: the Session Store persists conversation and recovery records for later continuation.

**Start with these interfaces for application integration:**

| Dependency | Interface / reusable implementation |
| --- | --- |
| Project context | [`agent.ContextSource`](definition.go): attribute each fragment's source, purpose, and capacity; inject stable rules separately from changing state |
| Skills | [`tools.SkillLoader`](tools/skill_tool.go): load full content by name; the application supplies the catalog |
| Commands | [`tools.CommandRunner`](tools/shell_tools.go); use [`NewLocalCommandRunner`](tools/shell_local_runner.go) for local execution |
| Child tasks | [`tools.TaskExecutor`](tools/task_tool.go); use [`NewLocalTasks`](tools/task_local_executor.go) for local execution |
| Large result storage | [`agent.ToolArtifactStorage`](tool_artifact.go) |
| Summaries | [`compaction.Summarizer`](compaction/standard.go); reuse [`ModelSummarizer`](compaction/model_summarizer.go) |

For writing, research, or support, primarily replace Instructions, Context, Skills, and Tools; remove capabilities you do not need. For file editing, select `WorkspaceReadWrite` and supply a `MutationAdapter`. A read-only workspace does not constrain Shell; command permissions still belong to the runner and permission policy.

Use the static Definition above for simple cases. Implement [`Source`](definition.go) when models and capabilities must vary by Session. For existing product conversation storage, integrate [`CanonicalAdapter`](canonical.go) and [`session.Store`](session/store.go) so product history and Agent recovery records share one journal.
