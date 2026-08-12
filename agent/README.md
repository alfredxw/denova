# Denova Agent Package

`agent` 是 provider-neutral、可组合、可持久恢复的 Go Agent module。公开使用心智模型只有三层：

```text
Agent -> Session -> Run
```

`Agent` 固定拥有 durable admission、model/tool loop、permission fence、canonical commit、interaction、tool-result projection、cleanup、compaction、recovery 和事件发布。使用者通过 `Definition` 选择或替换 model、tools、result processor、artifact storage、context、goal、cleanup、compaction、permission、interaction 与 canonical adapter，不需要编排内部阶段。

## 最小示例

```go
toolset, err := agent.StaticTools(lookupTool)
if err != nil {
    return err
}
assistant, err := agent.New(ctx, agent.Definition{
    Name:         "assistant",
    Model:        model,
    Instructions: "Use tools only when they materially help.",
    Tools:        toolset,
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

    Tools           Toolset
    ResultProcessor ToolResultProcessor
    Artifacts       ToolArtifactStorage
    Context         ContextSource
    Goal            GoalManager
    Cleanup         CleanupManager
    Compaction      CompactionManager
    Permission      PermissionPolicy
    Interaction     InteractionPolicy
    Canonical       CanonicalAdapter

    Middlewares []Middleware
    Execution   ExecutionPolicy
}
```

静态 `Definition` 自身实现 `Source`。动态宿主实现 `Source.Prepare`，按 Session、产品模式或每个 cycle 重建 Definition；需要写 product canonical input 时还必须实现 provider-free `Source.CanonicalInput`，使已接受输入在任何 prompt/model/tool preparation 之前完成幂等提交。持久 Session 会校验完整 capability identity、restore key 与 model-visible prefix fingerprint；不能用变化后的组合猜测性恢复旧 Run。

空 Capability 的语义是显式的：

- `Tools`、`Context`、`Goal`、`Cleanup`、`Compaction` 和 `Canonical` 为空时禁用相应扩展；
- `ResultProcessor` 与 `Artifacts` 为空时不做外部 lossless 大结果物化；需要大结果恢复时应显式选择 `toolresult.Standard` 和持久 Artifact Store；
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
ask, err := tools.Ask()
if err != nil {
    return err
}
todo, err := tools.Todo()
if err != nil {
    return err
}
skills, err := tools.Skills(skillSource)
if err != nil {
    return err
}
tasks, err := tools.Tasks(taskExecutor)
if err != nil {
    return err
}
definition.Tools, err = tools.Combine(
    workspace,
    shell,
    ask,
    todo,
    skills,
    tasks,
)
if err != nil {
    return err
}
```

所有 Toolset 构造与组合都会立即校验 schema 和稳定 identity 并返回错误；无效组件不会等到首次模型调用才失败。`ToolDefinition.ImplementationIdentity` 描述 schema/descriptor 看不见的 endpoint、scope 与配置语义，只进入恢复 identity，不污染模型前缀。便捷的 schema-only `StaticTools` 只适合一次性内存 Agent；持久 Session 必须为每个定义设置 `ImplementationIdentity`，或用 `StaticToolsIdentified` 为整组行为声明 identity。write/edit 和 shell 等行为 Adapter 必须显式声明 identity，公共 module 不绕过宿主的审计、并发、权限或持久化语义。

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

需要展示“模型下一步实际会看到什么”时，使用同一 Session 的只读检查：

```go
inspection, err := session.Inspect(ctx, agent.Text("Explain the next change."))
if err != nil {
    return err
}
render(inspection.ModelRequest.Messages)
```

`Inspect` 只在 Session idle 时成功，复用真实 Run 的 Definition、Goal、cleanup、compaction、Context、tools、Middleware、provider cache key 与 stable-prefix 装配，但不调用模型或工具，也不写 command、transcript、capability 或 canonical state。`ModelRequest` 是唯一精确的 provider-visible payload；`ContextFragments` 仅解释同次装配的来源、用途、revision 与 hard bound，诊断层应过滤掉最终请求中已不可见的片段，不能据此重建另一份上下文。动态能力可用 `agent.IsInspection(ctx)` 抑制可选 telemetry；model-visible 结果必须与真实 Run 保持一致。

## 存储、恢复与产品数据

`session.Store` 是 Agent durable state 的唯一公共存储 seam。内置实现包括：

- `session.Memory()`：测试和一次性进程；
- `session/file.New(root)`：带 lease、校验和、原子 append batch、可重建 checkpoint 与历史 command index 的本地持久 Store；
- `session/sessiontest.RunStoreContract`：外部 Store 的复用 contract suite。

Agent Store 保存命令、Run、queue、transcript、goal、cleanup、compaction、interaction、canonical receipt、host-effect outbox 和恢复证据；它不保存 Denova book、story、workspace 或 automation 领域数据。产品数据由自定义 `CanonicalAdapter` 幂等提交，并通过 query/reconcile 处理不确定 crash window。内置 file Store 始终以同一 checksummed canonical Log 为唯一权威；checkpoint、历史 command receipt 与 coverage index 只是可丢弃 sidecar，缺失或损坏时从 canonical Log 重建。普通自定义 Store 只需实现公开 contract，正确性相同，冷重开性能取决于自身索引策略。

工具 endpoint 的原始结果先经过固定 `ToolResultProcessor`，再进入事件、transcript、cleanup 和 compaction。内置 `toolresult.Standard` 可以把超大结果完整写入 Artifact Store，再生成 bounded model/display projection、protected receipt 与 recovery hint；宿主不能先截断原始 bytes。`ToolArtifactStorage` 同时负责发布和验证引用，工具自己返回的 artifact 不能仅凭字段完整就被信任。`CleanupManager` 只规划已结算、可恢复结果的可逆 replacement，Agent 负责 target freeze、CAS、final settlement、clear 与 crash recovery。

## Provider 与上下文边界

`providers` 保存 provider-neutral 配置和 identity；具体 wire protocol 位于 `providers/protocols/*`。`ContextSource` 是动态模型上下文的唯一 lifecycle seam；纯 `context.Assembler` 可通过 `context.ExportLifecycleFragments` 导出带来源、用途、资源、placement、revision 和 hard limit 的片段，不存在第二套 projector lifecycle。上下文超限会失败而不是静默截断。

真实 cycle 的 `RunView.StartedAt` 由 durable lifecycle 只捕获一次。需要把本轮时间注入模型的 ContextSource 必须使用该值，不能在 Interaction 或 checkpoint 恢复时重新读取 wall clock；没有真实 cycle 的结构维护收到零值并应省略临时时间片段。

稳定 system instruction、tool schema/order 与长期 context revision 参与 prefix fingerprint。Run ID、cursor、trace ID、时间戳和展示 Task ID 不得污染稳定前缀。

## Compaction 边界

`CompactionManager` 只负责规划 raw replacement range 与生成 checkpoint；Agent 独占 source hash/CAS、raw history、revision、manual/automatic admission、remove、failure fuse、recovery 和事件发布。

Automatic compaction 在全部 caller model middleware 之后读取最终 `ModelRequestSnapshot`。`CompactionManager` 是受信任的结构能力：`CompactionCompactRequest.Messages` 是用于选 range、hash/CAS 与 remove 的完整 raw transcript；`ModelRequest` 才是 caller middleware 之后的精确 provider-visible 请求；`SourceMessages` 默认是交给 Summarizer 的 canonical raw 增量源，重复压缩时只包含当前 checkpoint marker 和新 tail。Agent 不会猜测任意 middleware 改写与 raw index 的反向映射。宿主若有 model-only 可见性策略，必须通过带稳定 Identity 的 Manager Adapter 显式投影 `SourceMessages`，避免摘要重新引入 provider 已隐藏的正文。摘要 side fork 失败会发布 `CompactionFailed` 并继续未改写的 primary request；同一请求结构连续失败达到 `ExecutionPolicy.MaxAutomaticCompactionFailures` 后发布 `CompactionSkipped`。最终 provider input guard 仍负责 hard limit。

产品 ContextSource 如果把领域历史渲染成当前 prompt，可以在自定义 Manager Adapter 中将 `SourceMessages` 替换为 canonical domain delta、再应用同一 model visibility policy，并通过 bounded `ContextData` 返回产品游标；Adapter 的 Identity 必须覆盖这些策略，它不能提交第二份 checkpoint。

## 验证

```bash
cd agent
go test ./...
```

公共 Store、标准 Goal 协议、Compaction、Permission 和 Canonical Adapter 提供可复用 contract suite。自定义 Goal Manager 可在 Agent 持有的稳定 ID/status/revision/mutation fence 内使用自有 mutation kind、status 与 opaque JSON state；它不需要另建 Goal Store。Denova application 通过动态 Source、ContextSource、CanonicalAdapter、CompactionManager 和 product Toolset 使用同一 Agent lifecycle。
