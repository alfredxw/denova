# Denova Agent

[English](README.en.md) | 简体中文

`agent` 是 provider-neutral、可组合的 Go Agent 运行包。公共心智模型只有三层：

- `Agent` 持有 Definition 来源、Session Store 和进程生命周期。
- `Session` 表示一段稳定会话，保存 transcript 与能力状态，并串行执行 Run。
- `Run` 表示一次进程内执行，提供流式事件、转向、排队、交互和中断。

## Quickstart：运行一次 Agent

`agent` 是供宿主程序调用的 Go library，不是独立 CLI 或 Session Server。宿主通过 `agent.New` 创建 Agent，通过 `Run` 提交输入，并消费事件或等待最终结果。

下面是可作为 `main.go` 直接运行的最小示例。静态 `Definition` 自身就是 `Source`：

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

在一个 Go module 中保存该文件，设置 `OPENAI_API_KEY` 后运行：

```sh
go get github.com/alfredxw/denova/agent
go run .
```

`assistant.Run` 会创建并在结束后删除一次性 Session，适合无需保留上下文的单次任务。不读取流式输出时也可以直接调用 `run.Wait(ctx)`；面向 UI 或终端时，应持续消费 `run.Events()`。

## 连续对话与持久 Session

需要多轮对话时，先打开一个稳定 Session，再在该 Session 上执行 Run。默认 Store 只存在于内存；要跨进程重启保留 transcript 和能力状态，需要显式配置文件 Store 或自定义 Store：

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

同一 Session 一次只能有一个主动 Run；必须等当前 Run 结束，才能再次调用 `Session.Run`，否则会返回 `agent.ErrSessionBusy`。如果需要在当前 Run 尚未结束时提交后续输入，使用下一节的 `Queue`、`Steer` 或 `FollowUp`。

Session Key 是持久化身份。`Namespace`、`ID` 和 `Attributes` 都参与身份计算，因此不要将可变的显示名称放入其中。宿主重启后，用相同 Store 和相同 Key 调用 `owner.Session` 即可继续已完成的对话。简单场景可用 `agent.NamedSession("draft")`，它在默认 Namespace 中创建稳定 Key。

## 控制运行中的 Run

Run 是进程内句柄，控制操作只对当前进程中尚未结束的 Run 有效：

| 需求 | API | 语义 |
| --- | --- | --- |
| 立即修正当前方向 | `run.Steer(ctx, input)` | 将输入提到队首，在下一个安全的模型循环边界转向 |
| 在同一 Run 内追加工作 | `run.Queue(ctx, input)` | 排入当前 Run；返回的 `QueuedInput` 可以 `Cancel` 或 `Interrupt` |
| 创建下一轮 | `run.FollowUp(ctx, input)` | 立即返回新 Run 句柄，并在当前 Run 后串行执行 |
| 停止当前执行 | `run.Abort(ctx, request)` | 请求当前 Run 在可安全停止的位置结束 |
| 回答 Ask 或权限交互 | `run.Respond(ctx, interactionID, response)` | 解决 `InteractionRequested` 事件中的当前交互 |

`run.Events()` 输出 `AssistantDelta`、工具状态、交互请求和最终状态等类型化事件。`run.Wait(ctx)` 返回最终 `Result` 和 `error`。执行或存储失败会返回 error；`failed`、`incomplete` 和 `blocked` 也会以 `*agent.RunError` 返回。仍应检查 `Result.Status`，因为 `aborted` 通常不是 Go error。

## 页面重连与进程重启

页面刷新但 Agent 宿主进程仍在运行时，使用 Session 级观测重建 UI：

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

`Snapshot` 是界面恢复的基准，`Observe` 从 Cursor 之后补齐进程内历史事件并继续推送新事件。`Observation.Snapshot` 是建立观测时取得的更新快照；如果 UI 选择用它整体替换初始快照，则应忽略 Cursor 不大于该快照 Cursor 的重放事件，避免重复应用。`AttachRun` 只能重连当前进程持有的 Run。

后端进程重启后，已完成 transcript 会从 Store 恢复，但实时事件、队列、工具栈和 Interaction waiter 不会恢复。未完成回合会标记为 `incomplete`；宿主应展示该状态，并在用户需要时创建新 Run，而不是重放旧工具调用。

## 管理 Session

| 操作 | 结果 |
| --- | --- |
| `owner.ListSessions(ctx, selector)` | 列出 Store 中匹配的 Session Key |
| `owner.CountActiveSessions(ctx, selector)` | 统计当前进程中匹配的活动 Run |
| `session.Close(ctx)` | 停止活动 Run 并释放句柄和 Store lease，保留持久数据 |
| `session.Clear(ctx)` | 保留 Session 身份并开始空白对话；清除 Todo，保留 Goal |
| `session.Delete(ctx)` | 关闭 Session 并永久删除其 transcript |
| `owner.CloseSessions(ctx, selector)` | 关闭匹配 Session 及其子 Session，保留持久数据 |
| `owner.DeleteSessions(ctx, selector)` | 关闭并永久删除匹配 Session 及其子 Session |
| `owner.Close(ctx)` | 宿主停机时关闭全部进程内 Session 并释放 Store lease |

Selector 的非空字段使用 AND 语义；`IDPrefix` 只在显式设置时才进行前缀匹配。删除 API 拒绝无约束或 `All` Selector，避免意外删除全部会话：

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

## 完整能力组合

内置能力构造器返回声明值，不单独返回构造错误。下面把工作区、Shell、Todo、Ask、Skills、Tasks、Goal、大工具结果处理、Cleanup、Compaction、文件 Store 和 Trace 全部组合在一个 `agent.New` 中：

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
        tools.Todo(), // 默认使用当前 Session 的持久状态
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
        // MaxIterations=0、IdleTimeout=0 表示不设上限。
    },
},
    agent.WithSessionStore(sessionfile.New("/var/lib/myapp/agent-sessions")),
    agent.WithTrace(agent.TraceFunc(recordTrace)),
)
if err != nil {
    return err
}
```

`modelConfig`、`commandRunner`、`summarizer`、`skillSource` 和 `taskExecutor` 是宿主提供的配置或 Adapter。`builtin.Model` 会在初始化时同时生成不含密钥的稳定模型 Identity。读写工作区还需要提供产品 `MutationAdapter`；完整保存超大工具结果时，需要配置宿主 `ToolArtifactStorage`。

对于静态 `Definition`，`agent.New` 会初始化全部声明式能力，并把多个错误按 `Tools: Toolset[1]`、`Cleanup`、`Compaction` 等组件路径一起返回。模型调用、工具执行和 Session I/O 属于运行时错误，仍由对应操作返回。

## 定制示例：按项目动态组装

当模型、上下文或产品 Store 需要按 Session 选择时，实现 `Source`。`CanonicalInput` 只解析输入提交 Adapter，`Prepare` 再返回这一轮的完整 Definition；内置声明式能力仍由 Agent 在 Prepare 后统一校验：

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

自定义 `ContextSource` 的每个模型可见片段都必须声明来源、用途、资源、Revision、稳定性、位置和足够高的 `HardLimit`。自定义 Toolset、Canonical Adapter、Permission、Context 和模型配置应提供稳定且不包含密钥的 `CapabilityIdentity`。

## 职责边界

- Agent Session Store 只负责模型会话 transcript 和能力状态。
- Canonical Adapter 负责产品对话日志；`Definition.Effects` 独立处理工具变更的幂等回执，让子 Agent 在保留自己 Session 对话的同时，将变更提交到父级 Project。
- Trace 只负责观测，不是恢复或业务事实源。
- 实时 Run 控制、事件、队列、工具栈和 Interaction waiter 属于当前进程。

更详细的接口和设计边界见 [`docs/agent-package-api-design.md`](../docs/agent-package-api-design.md)。
