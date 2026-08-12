# Denova Agent Package

`agent` 是 provider-neutral、可组合、可持久恢复的 Go Agent module。公开使用心智模型只有三层：

```text
Agent -> Session -> Run
```

`Agent` 固定拥有 durable admission、model/tool loop、permission fence、canonical commit、interaction、compaction、recovery 和事件发布。使用者通过 `Definition` 选择或替换 model、tools、context、goal、compaction、permission、interaction 与 canonical adapter，不需要编排内部阶段。

## 最小示例

```go
assistant, err := agent.New(ctx, agent.Definition{
    Name:         "assistant",
    Model:        model,
    Instructions: "Use tools only when they materially help.",
    Tools: agent.StaticTools(
        lookupTool,
    ),
})
if err != nil {
    return err
}
defer assistant.Close(context.Background())

run, err := assistant.Run(ctx, agent.Text("Explain this repository."))
if err != nil {
    return err
}
for event := range run.Events() {
    render(event)
}
_, err = run.Wait(ctx)
return err
```

`Agent.Run` 使用一次性内存 Session。需要连续对话或崩溃恢复时，显式打开具名 Session 并注入持久 Store：

```go
store, err := sessionfile.New(".agent-state")
if err != nil {
    return err
}
assistant, err := agent.New(
    appCtx,
    source,
    agent.WithSessionStore(store),
    // Run identity is an application policy. Agent validates and persists this
    // value but does not define its prefix, trace filename, or storage layout.
    agent.WithRunIDGenerator(func(agent.RunIDRequest) (string, error) {
        return applicationRunID(), nil
    }),
)
if err != nil {
    return err
}

session, err := assistant.Session(ctx, agent.NamedSession("main"))
if err != nil {
    return err
}
run, err := session.Run(ctx, agent.Input{
    Text:           "Fix the failing tests.",
    IdempotencyKey: requestID,
})
```

## 可组合能力

一个 `Definition` 是单次 cycle 的完整、不可变组合根：

```go
type Definition struct {
    Key           string
    Name          string
    Description   string
    Model         BaseChatModel
    ModelIdentity CapabilityIdentity
    Instructions  string

    Tools       Toolset
    Context     ContextSource
    Goal        GoalManager
    Compaction  CompactionManager
    Permission  PermissionPolicy
    Interaction InteractionPolicy
    Canonical   CanonicalAdapter

    Middlewares []Middleware
    Execution   ExecutionPolicy
}
```

静态 `Definition` 自身实现 `Source`。动态宿主可以实现 `Source.Prepare`，按 Session、产品模式或每个 cycle 重建 Definition。持久 Session 会校验完整 capability identity、restore key 与 model-visible prefix fingerprint；不能用变化后的组合猜测性恢复旧 Run。

空 Capability 的语义是显式的：

- `Tools`、`Context`、`Goal`、`Compaction` 和 `Canonical` 为空时禁用相应扩展；
- `Permission` 为空时使用安全默认策略，写操作或受保护访问会请求许可；
- `Interaction` 为空时使用标准双语交互校验；
- `Execution` 零值不设置总运行超时或最大迭代次数。

自动 compaction 不会被偷偷启用；需要时应明确选择 `compaction.Standard` 或自定义 `CompactionManager`。

## 内置通用工具

`agent/tools` 提供可单独选择或自由组合的通用 Toolset：

- `Workspace`：read、glob、grep，以及可选的 write、edit；
- `Shell`：bash 和/或 pwsh；
- `Ask`：durable 双语用户交互；
- `Todo`：Session 内置状态或自定义 Store；
- `Skills`：自定义 SkillSource，批量读取逐项返回结果；
- `Tasks`：自定义 TaskExecutor，批量操作支持部分成功。

```go
workspace, err := tools.Workspace(tools.WorkspaceConfig{
    Root:     ".",
    Access:   tools.WorkspaceReadWrite,
    Mutation: mutationAdapter,
})
if err != nil {
    return err
}
shell, err := tools.Shell(tools.ShellConfig{Runner: commandRunner})
if err != nil {
    return err
}

definition.Tools = tools.Combine(
    workspace,
    shell,
    must(tools.Ask()),
    must(tools.Todo()),
    must(tools.Skills(skillSource)),
    must(tools.Tasks(taskExecutor)),
)
```

write/edit 和 shell 都依赖宿主注入的稳定 Adapter identity；公共 module 不绕过宿主的审计、并发、权限或持久化语义。

复杂网络工具位于可选 plugin package：

- `agent/plugins/websearch`
- `agent/plugins/webfetch`
- `agent/plugins/browser`

它们通过普通 Toolset 接入，同样经过 permission、result limits、artifact 与 recovery，不拥有特殊生命周期。

## Run 控制与重连

所有可能跨 transport 重试的命令都返回真实 durable receipt：

```go
receipt, err := run.Steer(ctx, agent.Input{
    Text:           "Keep the public syntax unchanged.",
    IdempotencyKey: steerRequestID,
})

abortReceipt, err := run.Abort(ctx, agent.AbortRequest{
    Reason:         "user cancelled",
    IdempotencyKey: abortRequestID,
})
```

`CommandReceipt` 包含 `CommandID`、`RunID`、`Cursor` 和 `Replayed`。同一个幂等 ID 在进程重启、甚至目标 Run 已终结后，仍可重放原 receipt；不同语义复用同一 ID 会失败。

`Session.Observe` 原子返回 bounded Snapshot 与 replay/live Event：

```go
observation, err := session.Observe(ctx, lastCursor)
if err != nil {
    return err
}
sendSnapshot(observation.Snapshot)
for event := range observation.Events {
    sendEvent(event)
}
```

missed ephemeral delta 可从 Snapshot 的 active output 恢复。超出 retention 时返回 `ErrCursorExpired`，调用者应重新获取 Snapshot。

## 存储、恢复与产品数据

`session.Store` 是 Agent durable state 的唯一公共存储 seam。内置实现包括：

- `session.Memory()`：测试和一次性进程；
- `session/file.New(root)`：带 lease、校验和及原子 append batch 的本地持久 Store；
- `session/sessiontest.RunStoreContract`：外部 Store 的复用 contract suite。

Agent Store 保存命令、Run、queue、transcript、goal、compaction、interaction、canonical receipt、host-effect outbox 和恢复证据；它不保存 Denova book、story、workspace 或 automation 领域数据。产品数据由自定义 `CanonicalAdapter` 幂等提交，并通过 query/reconcile 处理不确定 crash window。

## Provider 与上下文边界

`providers` 保存 provider-neutral 配置和 identity；具体 wire protocol 位于 `providers/protocols/*`。上下文片段必须声明 source、purpose、resource、placement 与 hard limit，超限会失败而不是静默截断。

稳定 system instruction、tool schema/order 与长期 context revision 参与 prefix fingerprint。Run ID、cursor、trace ID、时间戳和展示 Task ID 不得污染稳定前缀。

## 验证

```bash
cd agent
go test ./...
```

公共 Store、Goal、Compaction、Permission 和 Canonical Adapter 均提供可复用 contract suite。Denova application 通过动态 Source、ContextSource、CanonicalAdapter、CompactionManager 和 product Toolset 使用同一 Agent lifecycle。
