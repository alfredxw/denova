# Denova Agent

[English](README.en.md) | 简体中文

`agent` 是 provider-neutral、可组合的 Go Agent 运行包。公共心智模型只有三层：

- `Agent` 持有 Definition 来源、Session Store 和进程生命周期。
- `Session` 表示一段稳定会话，保存 transcript 与能力状态，并串行执行 Run。
- `Run` 表示一次进程内执行，提供流式事件、转向、排队、交互和中断。

## Quickstart

静态 `Definition` 自身就是 `Source`。内置模型、Definition 和配置直接写进 `agent.New`；组合错误只在这里处理一次：

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

`assistant.Run` 使用一次性内存 Session。需要连续对话时，使用 `assistant.Session(ctx, agent.NamedSession("draft"))` 打开稳定 Session。

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
    agent.WithSessionStore(sessionfile.New("/var/lib/myapp/agent-transcripts")),
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

## 运行与持久化语义

- 同一 Session 同时只执行一个 Run；`Steer` 改变当前 Run，`Queue` 排入同一 Run，`FollowUp` 创建串行下一轮，`Abort` 中断当前 Run。
- 页面刷新但后端进程仍在时，可通过 `Session.Snapshot`、`AttachRun` 和 `Observe` 重新连接。
- 文件 Store 保存 provider-neutral transcript、能力状态和最小回合边界；实时事件、队列、工具栈与 Interaction waiter 留在当前进程。
- 后端重启后，已完成 transcript 继续可用；未完成回合标记为 `incomplete`，不会精确重放旧工具调用。
- Agent Session Store 负责模型会话，Canonical Adapter 负责产品事实，Trace 只负责观测。

详细接口与架构说明见 [`docs/agent-package-api-design.md`](../docs/agent-package-api-design.md) 和 [`docs/agent-architecture.md`](../docs/agent-architecture.md)。
