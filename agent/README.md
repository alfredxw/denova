# Denova Agent

[English](README.en.md) | 简体中文

`agent` 是 provider-neutral、可组合的 Go Agent 运行包。公共心智模型只有三层：

- `Agent` 持有 Definition 来源、Session Store 和进程生命周期。
- `Session` 表示一段稳定会话，保存 transcript、能力状态和恢复事实，并串行执行 Run。
- `Run` 表示一个逻辑任务；当前进程的句柄提供流式事件与控制，暂停后可用相同 RunID 创建新句柄继续。

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

需要多轮对话时，先打开一个稳定 Session，再在该 Session 上执行 Run。默认 Store 只存在于内存；要跨进程重启保留 transcript、能力状态、已接收输入和任务检查点，需要显式配置文件 Store 或自定义持久 Store：

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

## 持久输入与运行控制

独立后续任务和补充输入由 Session 接收；改变当前任务方向仍使用 Run。可能跨网络重试的输入和控制应提供稳定的 `IdempotencyKey`：同一个操作重试时复用，下一次独立操作使用新值。同一 Key 改变输入内容会返回 `agent.ErrIdempotencyConflict`。

| 需求 | API | 语义 |
| --- | --- | --- |
| 修正当前方向 | `run.Steer(ctx, input)` | 持久接收并返回 `CommandReceipt`，在安全边界转向；暂停时只接收 |
| 补充输入 | `session.Queue(ctx, input)` | 返回 `*QueuedInput`；空闲时不启动 Run，输入留给下一次显式启动 |
| 创建后续任务 | `session.FollowUp(ctx, input)` | 返回 `CommandReceipt`；空闲时启动，忙碌或暂停时排队 |
| 找回补充输入 | `session.Queued(ctx, commandID)` | 返回句柄、是否找到和错误；重开 Session 后仍可查询 |
| 取消 / 提前投递补充输入 | `queued.Cancel(ctx, request)` / `queued.Interrupt(ctx, request)` | 使用 `QueueControlRequest`，返回控制凭证；提前投递不隐式继续暂停任务 |
| 终结当前任务 | `run.Abort(ctx, request)` | 使用 `AbortRequest`，终结当前 Run；整个任务树使用 `owner.AbortTree` |
| 回答交互 | `session.Respond(ctx, interactionID, response)` | 返回请求、已接纳结果和错误；可在重开 Session 后回答，不启动执行 |

例如，宿主先保存补充输入，再把一项独立工作排到当前任务之后：

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

`CommandReceipt` 证明操作已接收，不代表模型已经开始。保存其中的 CommandID / RunID，随后通过 `Snapshot`、`Observe` 或 `AttachRun` 获取状态；不要因为 HTTP 重试生成新 Key。队列最多保存 1024 项、合计 64 MiB，单项序列化输入最多 16 MiB；超限拒绝接收，不截断输入。

`run.Events()` 输出 `AssistantDelta`、工具状态、交互请求和最终状态等类型化事件。`run.Wait(ctx)` 返回当前句柄的 `Result` 和 `error`。执行或存储失败会返回 error；`failed`、`incomplete` 和 `blocked` 也会以 `*agent.RunError` 返回。仍应检查 `Result.Status`：`aborted` 通常不是 Go error，`suspended` 表示逻辑任务尚未结束。

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
        _ = active // Inspect or control this handle; attaching does not resume it.
    }
}
```

`Snapshot` 是界面恢复的基准，`Observe` 从 Cursor 之后补齐进程内历史事件并继续推送新事件。`Observation.Snapshot` 是建立观测时取得的更新快照；如果 UI 选择用它整体替换初始快照，则应忽略 Cursor 不大于该快照 Cursor 的重放事件，避免重复应用。进程重启后从新 Snapshot 开始，不把旧实时事件当作持久日志。

打开 Session 会从 journal 恢复已接收输入、未决交互、工具结果和任务检查点；`AttachRun` 可获取恢复后的句柄。打开、观测和 Attach 都不会自动执行。检查 `ActiveStatus`、`ActiveRunID` 和 `PendingInteractions`，由用户显式选择继续或取消。

## 暂停与继续原任务

产品的“暂停整个任务”使用 `Agent.SuspendTree`，它阻止新模型、工具和子任务入场，先停止后代再释放根 Session 的 writer。没有后代时也可使用同一入口：

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

- `Session.SuspendAndClose` / `Session.ResumeRun` 仅控制一个 Session；暂停后先用相同 Key 重新打开 Session。任务树暂停必须配对 `ResumeTree`，不能逐个 Session 绕过树的入场屏障。
- 暂停成功表示生产者已停止、检查点已持久化且 writer 已释放。模型等待和退避可取消；`finish_current` 工具需要完成后才报告暂停。部分失败返回错误，宿主保留实际状态并使用原命令重试。
- 原句柄的 Wait 返回 `ResultSuspended`，不发布 `RunSettled`，也不发送子任务完成通知。继续返回新句柄，保留原 RunID、Cycle 和已消费输入；重新建立事件订阅。
- 继续通过当前 Source 重建依赖并校验 Definition 与产品身份。模型或能力配置不匹配会明确失败；不要修改旧 SessionKey 或 HostData 来强行绑定另一个任务。
- `owner.AbortTree(ctx, key, agent.AbortRequest{...})` 终结根与未结束后代，返回 `CommandReceipt`；暂停或等待核实时也可直接取消。普通 `Close` 不是暂停 API。

恢复不保存 Go 调用栈、半条模型流或工具内部进度，也不保证中断前后生成相同文本。已确认的工具结果复用，未确认的外部写效果先核实，不盲目重放。

## 持久交互与未知工具效果

普通 Ask 的请求和回答持久化，重开 Session 后可从 `Snapshot.PendingInteractions` 恢复表单，调用 `Session.Respond` 回答。重复提交相同回答返回已接纳结果；已确定的回答不能被不同内容覆盖。暂停期间回答不会自动继续任务。

`InteractionRequest.Verification != nil` 表示某次外部操作已开始，但结果未确认。宿主应展示 `Verification.Tool` 和原始 `Arguments`，让用户核实；使用请求中的稳定 QuestionID 和选项值提交：

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

| 选项 | 结果 |
| --- | --- |
| `executed` | 保存用户确认已生效；原结果不可得时明确说明，不伪造输出或重复操作 |
| `not_executed` | 保存未生效事实；后续模型若仍需操作，重新经过正常工具与权限流程 |
| `unknown` 或取消该核实问题 | 保持等待；用户仍可直接取消整个任务 |

核实未完成前，整个 Run 的模型与工具入口保持关闭，Queue、Steer 或 Resume 都不能绕过。普通 Ask 的取消则按原交互契约交给工具处理。任意外部工具不具备通用的 exactly-once 保证。

## 在同一子会话中继续工作

`tools.LocalTasks` 的持久子会话通过 `TaskRef{Agent, Session, Run}` 定位。宿主保留返回的引用；同一子会话接受新任务时使用返回的新 Run 引用：

| API | 语义 |
| --- | --- |
| `tasks.FollowUp(ctx, ref, input)` | 返回 `Task`，在原来的子 Session 中接受新 Run；忙碌时排队 |
| `tasks.SendMessage(ctx, ref, input)` | 返回 `CommandReceipt`，仅补充输入；空闲或暂停的子任务不会因此启动 |
| `tasks.Resume(ctx, ref, request)` | 返回 `Task`，继续引用中的原 Run；使用 `agent.ResumeRequest` |

使用树控制时，父子 Session 必须由同一个能够重建各自 Definition 的 Agent / Store 管理。将 `agent.ChildSessionAttributes(parent.Key())` 合并到 `LocalTaskAgent.Attributes`，并在 `LookupAttributes` 中保留稳定的父级筛选条件；仅设置 `CompletionParent` 不会建立这一关系。

子 SessionKey 只携带稳定身份。`FollowUp` 从当前父调用取得 HostData，`Resume` 保留原 Run 的 HostData，因此后续任务不会沿用第一次调用的产品路由。旧已分配 Key 与文件名保持不变。`tools.Tasks(tasks)` 向模型暴露对应的 `follow_up`、`send_message` 和 `resume` 操作，批量请求逐项返回结果；输入接收凭证不等于任务完成。

## 模型重试与输出修复

库默认只尝试一次。开启 transient 重试时同时设置次数和稳定策略身份：

```go
definition.Execution = agent.ExecutionPolicy{
    ModelMaxAttempts: 4,
    Retry:           &agent.RetryConfig{Decide: agent.TransientRetry},
    RetryIdentity:   agent.CapabilityIdentity{Kind: "example.retry.transient", Version: 1},
    ToolParallelism: 4,
}
```

`ModelMaxAttempts` 包含首次调用，限制一个逻辑模型响应的网络失败与业务修复总次数；`0` 等同于 `1`。`RetryConfig.Decide` 只返回是否重试、延迟和原因，不替换模型、请求或历史。内置策略支持可取消退避、jitter、Retry-After，并拒绝认证或永久额度错误；内置 provider adapter 已关闭 SDK 隐式重试。

业务校验在 `Middleware.ReviewModelOutput(ctx, output)` 返回 `ModelOutputReview`：`ModelOutputAccept` 接纳，`ModelOutputRepair` 提供有来源和容量声明的 `ContextFragment` 反馈。同一次响应中，反馈替换该 middleware 的旧反馈，合计最多 64 KiB，超限报错。修复与网络失败共用上述次数，未接纳响应中的工具不会执行。

UI 按 `AssistantDelta.ResponseOrdinal` 区分每次尝试；收到 `ModelRetry` 时撤去该次失败预览，不能把断流片段拼到重试后的回答。只有已接纳输出进入 canonical transcript。Compaction 等独立调用可用 `ModelRequestSnapshot.Complete(ctx, execution)` 复用重试机制；每次 side call 独立计数，成功前缓冲输出，不切换模型。该次数限制不等于 Run 总时长或迭代数限制。

## 管理 Session

| 操作 | 结果 |
| --- | --- |
| `owner.ListSessions(ctx, selector)` | 列出 Store 中匹配的 Session Key |
| `owner.CountActiveSessions(ctx, selector)` | 统计当前进程中匹配的活动 Run |
| `session.Close(ctx)` | 终结活动 Run 并释放句柄和 Store lease，保留持久数据；需继续原任务时先显式暂停 |
| `session.Clear(ctx)` | 保留 Session 身份并开始空白对话；清除 Todo，保留 Goal |
| `session.Delete(ctx)` | 关闭 Session 并永久删除其 transcript |
| `owner.CloseSessions(ctx, selector)` | 关闭匹配 Session 及其子 Session，保留持久数据 |
| `owner.DeleteSessions(ctx, selector)` | 关闭并永久删除匹配 Session 及其子 Session |
| `owner.Close(ctx)` | 关闭全部进程内 Session 并释放 Store lease；需继续的任务应在停机前显式暂停 |

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
        tools.Todo(), // Uses the current Session's durable state by default.
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
        // MaxIterations=0 and IdleTimeout=0 leave these limits unset.
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

`projectSessionStore` 是宿主实现的 `agent/session.Store`，与该项目的 Canonical Adapter 共享同一产品 journal。自定义 `ContextSource` 的每个模型可见片段都必须声明来源、用途、资源、Revision、稳定性、位置和足够高的 `HardLimit`。自定义 Toolset、Canonical Adapter、Permission、Context 和模型配置应提供稳定且不包含密钥的 `CapabilityIdentity`。

## 存储契约与职责边界

- 独立 Agent 的 Session journal 保存 transcript、输入、工具确认、交互和能力恢复状态；索引与 Trace 可重建，不是恢复事实源。
- 产品拥有 canonical journal 时，Session Store 应把 Agent 记录写入同一份 journal，避免第二份恢复历史。`CanonicalAdapter` 的输入、上下文和输出提交必须使用原 `CommitIdentity`，校验 `Checkpoint` 返回的 `ExpectedRevision`，把产品接纳与 `Records` 原子写入同一事务。准备事务时仅调用一次回调，不从回调重入同一 Session，也不能先返回成功再补写检查点。
- 产品草稿由宿主保存；完整草稿可通过可选 `CanonicalPreparedOutput.PendingOutput` 返回已接纳输出，让恢复走正常提交路径，而无需再次调用模型。正式产品状态与草稿完成应在最终产品事务中一起提交。
- `Definition.Effects` 独立处理工具变更的幂等回执，让子 Agent 保留自己的 Session 对话，同时向父级 Project 提交变更。任务树关系与每个 Run 的 HostData 不改变 journal 的所属身份。
- `session.ErrCommitUnknown` 表示写入结果不明：关闭当前句柄并重新打开、回放 journal 查明事实，不在原句柄上盲重试。自定义 Store 必须实现其原子追加和 writer 所有权契约。
- 实时事件、Go 调用栈和 Interaction waiter 属于当前进程；持久队列与已接纳回答不依赖这些对象存活。

## v0.4.5 数据与降级

旧 transcript、能力状态和已结束任务仍可读取；只读打开不会改写 journal。旧版本缺少接收事实的未完成任务保留 `incomplete`，不能补造输入或检查点来继续。

新恢复记录无法由 v0.4.5 严格解析器读取。内置文件 Store 在首次向旧 journal 写入新格式前创建 `<journal>.pre-resilience-v1.bak`；保留原始备份，备份失败则拒绝写入。嵌入产品 journal 的宿主需在自己的事务边界提供同等备份保证。

降级前先停止宿主并另存当前数据目录，再把对应备份恢复为原 journal，删除对应可重建索引。备份后的新增内容、新版新建的会话不包含在旧版恢复结果中，应保留当前数据以便重新使用新版。
