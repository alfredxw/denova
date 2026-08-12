# Agent Package Interface 设计方案（Composable Capabilities）

> 状态：目标 Interface、Denova 生产迁移与已识别的能力差异均已收口；最终测试、构建和产品链路验收状态以 `agent-package-migration-parity-audit.md` 为准。
>
> 当前 `agent` module 尚未发布，不保留旧公开 Interface 的兼容层。目标是一次性形成简单、完整、可组合、可替换，并足以承载 Denova 与独立 Coding Agent 的长期设计。

## 1. 结论

公开使用心智模型保持为：

```text
Agent -> Session -> Run
```

同时，Agent 由一组职责清晰的 Capability Interface 组成：

```text
Definition / Source
  |- Model
  |- Toolset
  |- Tool Result Processor
  |- Artifact Storage
  |- Context Source
  |- Goal Manager
  |- Cleanup Manager
  |- Compaction Manager
  |- Permission Policy
  |- Interaction Policy
  |- Canonical Adapter
  `- Middleware / Execution Policy

Agent Options
  |- Session Store
  |- Trace Sink
  `- Limits
```

设计原则是：

- Agent 固定协调 durable admission、permission fence、model/tool loop、canonical commit、recovery 和 Event publication；
- 使用者可以选择内置实现，也可以为真实 seam 提供自定义 Adapter；
- Capability 不能任意插入或重排安全阶段，只能通过自己的窄 Interface 贡献行为；
- Basic Agent 不要求理解 Session Store、goal、compaction、runtime 或 recovery；
- 复杂宿主可以替换 Session、Goal、Compaction、Permission、Toolset、Context、Canonical 等能力；
- Denova 特有的 lore、book/story mutation、Interactive Agent、Director 等保留在 Denova；
- read、glob、grep、write、edit、bash/pwsh、ask、todo、skill、task 等通用工具由 `agent` module 提供；
- web search、web fetch、browser 等复杂能力作为可选 plugin package 提供，不增加 core 依赖；
- 公共 module 提供 Denova 当前需要的完整通用 Agent lifecycle，而不是只提供一个 model/tool loop。

## 2. 设计目标

### 2.1 使用目标

- Basic Agent 在 model 准备好后约 10 行即可运行；
- 普通用户只理解 Definition、Tools、Run 和 Event；
- Session 始终存在，`Agent.Run` 只是临时 Session 的快捷入口；
- 持久化只需替换 Session Store；
- 内置能力可以按需选择，不要求使用整套 Coding preset；
- 每个内置 Capability 都有公开 Interface，复杂使用者可以替换其实现；
- 同一套 Agent 可以支持 terminal、HTTP/SSE、GUI、Coding Agent 和 Denova；
- Denova 可以按 Session、产品模式、辅助任务和每个 cycle 动态选择 model、prompt、tools、context 与 Capability Adapter；
- steer、follow-up、abort、goal continuation、interaction 和 recovery 通过 public Run/receipt 使用，不要求普通调用者理解内部 runtime identity；需要跨进程精确对账的宿主可以持久化公开的 `RunID`、`CommandID` 与 `Cursor`；
- 进程重启后可通过 Session 恢复 active Run、pending interaction、canonical recovery 和 Event cursor。
- 需要分析或解释模型上下文的宿主通过 `Session.Inspect` 获取与真实 Run 共用装配路径的精确 provider-neutral request；宿主不复制 cleanup、compaction、Goal、Middleware 或 prefix-cache 逻辑。

### 2.2 架构目标

- `Agent` 是深 Module，调用者不编排 lifecycle stage；
- Capability Interface 位于真实 seam，不制造只转发参数的浅 package；
- 内置实现与自定义实现通过同一 Interface 接入；
- Session Store 是唯一通用 durable state seam；
- Denova product store 继续拥有 book、story、workspace、automation 等用户数据；
- Agent 通过 Canonical Adapter 与 product store 做幂等提交和恢复协调；
- Tools 和 Middleware 保持单一来源，不产生 runtime 专用版本；
- permission 必须在任何 tool host effect 之前完成；
- Interaction 必须先 durable pending，再发布 Event，再接受响应；
- context fragment 必须包含 source、purpose、resource、placement、稳定 revision/ref 和 hard limit；
- display、trace、thinking 和 tool details 不自动进入模型上下文；
- 默认没有总运行超时或最大迭代次数；
- 模型上下文设计优先保证 prefix cache 命中率；
- 一个 Go module、一个版本，不制造 core/harness/plugin 版本矩阵。

### 2.3 非目标

- 不将 Denova 的 writing、game、lore、book、story、Hertz 或 AI SDK UI 类型放入公共 module；
- 不通过 `EnableX bool` 组合能力；
- 不提供让调用者重排 durability、permission、commit、recovery 阶段的 recipe DSL；
- 不允许通用 Middleware 替换 Agent 内建安全阶段；
- 不承诺旧 `Runner`、`runtime.Engine`、`Harness`、`BindingRef` 等公开路径；
- 不为尚未出现真实替换需求的内部 reducer、actor、journal codec 建立公开 seam。

## 3. 核心设计：固定生命周期，可组合能力

自由组合不等于自由编排执行阶段。

调用者可以替换“做什么”的实现：

- Session 如何存储；
- Tools 如何产生；
- 工具结果如何 lossless materialize、投影与恢复；
- Context 如何装配；
- Goal 如何校验 transition、生成模型上下文/工具并决定 continuation；
- Cleanup 如何选择可逆的工具结果投影；
- Compaction 如何规划和生成 checkpoint；
- Permission 如何判断和保存规则；
- Product canonical state 如何提交和恢复；
- Trace 如何输出。

调用者不能改变“什么时候做”：

```text
durable input admission
-> canonical input materialization
-> exact Definition restore and canonical-receipt validation
-> context/tool/goal preparation
-> caller model Middleware produces the exact ModelCall
-> at most one context-maintenance action (cleanup or compaction)
-> successful compaction rematerializes product Context in the same cycle
-> model call
-> canonical tool-input validation
-> permission fence
-> tool execution
-> fixed ToolResultProcessor / Artifact Storage
-> canonical effect outbox and reconciliation
-> tool transcript commit
-> repeat model/tool seam as needed
-> canonical output commit
-> staged cleanup + transcript + final settlement atomically commit
-> goal continuation
```

Trace 与 typed Event 在对应 durable boundary 到达时持续发布，terminal Event 只在最终结算可见后发布。实际 model/tool engine 会多次经过 model、permission 和 tool 阶段，但阶段间的不变量由 Agent Implementation 固定。

Capability 不是 lifecycle hook。每个 Capability 只能在自己的 seam 返回 typed result，不能直接提交 Agent State，也不能调用下一个阶段。

## 4. 最终使用方式

### 4.1 Basic Agent

```go
toolset, err := agent.StaticTools(lookupTool)
if err != nil {
    return err
}
assistant, err := agent.New(appCtx, agent.Definition{
    Name:         "assistant",
    Model:        model,
    Instructions: "Use tools when they materially help.",
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

默认行为：

- 使用内存 Session Store；
- 创建一次性临时 Session；
- Tools 为空时只调用模型；
- permission 使用安全默认策略；
- compaction 默认不启用；需要压缩的宿主显式选择 `compaction.Standard(...)` 或自定义 Manager，避免一次看似 Basic 的调用隐式增加模型请求；
- goal 默认不启用；
- trace 默认 no-op；
- ctx 只控制当前调用等待，显式 `Run.Abort` 才终止已经 durable accepted 的 Run。

### 4.2 选择内置通用工具

```go
workspaceTools, err := tools.Workspace(tools.WorkspaceConfig{
    Root:     ".",
    Access:   tools.WorkspaceReadWrite,
    Mutation: mutationAdapter,
})
if err != nil {
    return err
}
shellTools, err := tools.Shell(tools.ShellConfig{Runner: commandRunner})
if err != nil {
    return err
}
askTools, err := tools.Ask()
if err != nil {
    return err
}
todoTools, err := tools.Todo()
if err != nil {
    return err
}
skillTools, err := tools.Skills(skillSource)
if err != nil {
    return err
}
taskTools, err := tools.Tasks(taskExecutor)
if err != nil {
    return err
}

codingTools, err := tools.Combine(
    workspaceTools,              // read, glob, grep, write, edit
    shellTools,                   // bash or pwsh by platform
    askTools,
    todoTools,
    skillTools,
    taskTools,
)
if err != nil {
    return err
}

assistant, err := agent.New(appCtx, agent.Definition{
    Name:         "coding",
    Model:        model,
    Instructions: codingPrompt,
    Tools:        codingTools,
    Goal:       goal.Standard(),
    Compaction: compactionManager,
    Permission: permission.Coding(),
})
```

每个 Tool 仍可以单独选择。`Workspace`、`Shell` 等只是内聚 Toolset，不是必须使用的 preset。Toolset 工厂与 `Combine` 在构造期验证 schema 和每个行为 Adapter 的稳定 identity；无效组合直接返回错误，不会潜伏到首次模型调用。

### 4.3 替换 Capability 实现

```go
assistant, err := agent.New(
    appCtx,
    agent.Definition{
        Name:         "custom-agent",
        Model:        model,
        Instructions: prompt,
        Tools:        customToolset,
        Goal:         customGoalManager,
        Compaction:   customCompactionManager,
        Permission:   customPermissionPolicy,
    },
    agent.WithSessionStore(customSessionStore),
    agent.WithTrace(customTraceSink),
    agent.WithLimits(customLimits),
)
```

替换 Session Store 不改变 Agent、Session 或 Run 的使用方式。替换 Goal 或 Compaction 也不要求调用者接管 successor、revision、recovery 或 Event publication。

### 4.4 具名持久化 Session

```go
state, err := sessionfile.New(".coding-agent")
if err != nil {
    return err
}

assistant, err := agent.New(
    appCtx,
    definition,
    agent.WithSessionStore(state),
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

### 4.5 Steer、Follow-up、Abort 与 Interaction

```go
run, err := session.Run(ctx, agent.Text("Refactor the parser."))
if err != nil {
    return err
}

if _, err := run.Steer(ctx, agent.Input{
    Text:           "Keep the public syntax unchanged.",
    IdempotencyKey: steerRequestID,
}); err != nil {
    return err
}

next, err := run.FollowUp(ctx, agent.Text("Then update the docs."))
if err != nil {
    return err
}

for event := range run.Events() {
    if request, ok := event.Payload.(agent.InteractionRequested); ok {
        response := terminal.Ask(request)
        if err := run.Respond(ctx, request.ID, response); err != nil {
            return err
        }
    }
}

if _, err := next.Abort(ctx, agent.AbortRequest{
    Reason:         "user cancelled",
    IdempotencyKey: abortRequestID,
}); err != nil {
    return err
}
```

### 4.6 无副作用的精确上下文检查

```go
inspection, err := session.Inspect(ctx, agent.Text("Explain the next change."))
if err != nil {
    return err
}

renderContextAnalysis(inspection.ModelRequest.Messages)
renderProvenance(inspection.ContextFragments)
renderTokenEstimate(estimate(inspection.ModelRequest))
```

`Inspect` 只在 Session idle 时成功，并复用真实 Run 的 Definition/Goal materialization、active cleanup、active compaction、Context、tool schema、caller Middleware、provider cache key 与 stable-prefix 计算。返回值是 detached、不可执行的 provider-neutral snapshot，不暴露 model adapter，也不产生 command、Run、canonical commit、tool side effect 或 Session journal 写入。

`ModelRequest` 是唯一精确的 provider-visible payload；`ContextFragments` 是同次装配得到的有界来源、用途、资源、revision、placement 与 hard-limit 诊断，不能被宿主重新拼成另一份模型请求。因为 cleanup、compaction 或 caller Middleware 可能移除/改写片段，展示来源时必须以最终 `ModelRequest` 为准，只报告仍实际可见的 fragment。这样上下文分析既不复制装配逻辑，也不会在迁移后丢失 review feedback 等动态注入来源。

动态 Capability 在 inspection context 中仍必须产生与真实 Run 相同的 model-visible 内容；可通过 `agent.IsInspection(ctx)` 抑制可选 telemetry，但不能借此改变 prompt。尚未提交的 Goal mutation 不允许预览，因为它没有 durable revision fence。

### 4.7 HTTP/SSE 重连

```go
session, err := assistant.Session(ctx, key)
if err != nil {
    return err
}

observation, err := session.Observe(ctx, agent.Cursor(lastEventID))
if err != nil {
    return err
}

return sse.Write(ctx, observation)
```

`Observation` 原子地包含 bounded Snapshot 与 replay/live Event。SSE 只做编码，不参与执行生命周期。

## 5. 公开 Interface

### 5.1 Definition 与 Source

静态和动态 Agent 使用同一个构造函数：

```go
type Source interface {
    Prepare(context.Context, PrepareRequest) (Definition, error)
    CanonicalInput(context.Context, PrepareRequest) (CanonicalAdapter, error)
}

type SourceFunc func(context.Context, PrepareRequest) (Definition, error)

type Definition struct {
    // Key is the stable Source lookup identity. Agent derives and verifies the
    // exact RestoreKey and model-visible PrefixFingerprint from the assembly.
    Key string

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

func New(
    lifecycle context.Context,
    source Source,
    options ...Option,
) (*Agent, error)
```

`Definition` 自身实现 `Source`，普通用户直接传静态 Definition。`SourceFunc` 适用于不需要 product canonical input 的简单动态组合；Denova 这类持久化宿主实现完整 `Source`，使 accepted input 能在任何 model/context/tool preparation 之前经过 provider-free canonical admission。

所有可恢复 Capability 使用同一种稳定 identity：

```go
type CapabilityIdentity struct {
    Kind       string
    Version    uint16
    ConfigHash string
}
```

- Kind 表示实现/协议身份，不使用 Go concrete type 名称；
- Version 表示恢复语义版本；
- ConfigHash 只包含影响行为的 canonical 配置，不包含 credential、临时 token 或内存地址；
- Agent 对 Definition 与全部 CapabilityIdentity 做 canonical encoding，生成 RestoreKey；
- Definition.Key 用于让动态 Source 找回业务配置，不能替代 Agent 对实际 assembly 的校验；
- 静态 Definition 可以省略 Key；当 Model 与全部 Capability 都有稳定 identity 时由 Agent 派生 DefinitionKey，否则仅允许内存 Session；
- 无法声明稳定 identity 的 Capability 只能用于内存 Session。

空 Capability 的含义必须明确：

| 字段 | 空值行为 |
| --- | --- |
| Tools | 无模型工具 |
| ResultProcessor | 不额外改写工具结果；仍执行 Agent 固定的结果校验与 transcript 配对 |
| Artifacts | 不提供外部大结果存储；需要 lossless 大结果时必须显式配置 |
| Context | 只使用 Session transcript 与当前 Input |
| Goal | 不启用 goal |
| Cleanup | 不启用可逆工具结果投影 |
| Compaction | 不启用 compaction；需要时显式选择内置或自定义 Manager |
| Permission | 使用 safe default policy |
| Interaction | 使用 standard validation/persistence policy |
| Canonical | 无外部 canonical product store |
| Middlewares | 无 caller Middleware |
| Execution | 默认无限迭代，使用标准 retry/parallelism |

### 5.2 PrepareRequest 与恢复

```go
type PrepareRequest struct {
    Session SessionView
    Run     RunView
    Input   Input
    Reason  TurnReason

    DefinitionKey string
    RestoreKey    string
    HostData      *HostData
    Compaction    *CompactionState
    Cleanup       *CleanupState
}
```

约束：

- Source 可以读取宿主依赖，但不能写 Agent canonical state；
- `CanonicalInput` 只能解析 provider-free Canonical Adapter，不能组装 prompt、调用模型或执行工具；
- `RunView.StartedAt` 由 durable `CycleStarted` 只捕获一次；任何 model-visible 的本轮时间片段必须使用该值，不能在 ContextSource 恢复时重新读取 wall clock；checkpoint 与 Interaction 恢复必须保留原值；
- 没有真实 cycle 的结构维护会收到零值 `StartedAt`，宿主必须省略临时时间片段；只读 `Session.Inspect` 使用非持久的 synthetic run，只用于本次精确装配；
- 同一个 cycle 的 Definition 不可变化；
- Source 必须并发安全；
- durable recovery 时 Source 使用 `DefinitionKey` 找回配置，并重建语义等价的 Model、Toolset、Context、Goal、Compaction、Permission、Canonical 和 Middleware；
- Definition 中所有 Adapter 必须声明稳定 identity；
- 一次性 closure 只能用于内存 Session，不能用于 durable Session；
- Tool schema、顺序与 cache-sensitive options 必须稳定；
- Agent 计算并持久化最终 prefix fingerprint，不能只信任调用方的 Key；
- Agent 恢复时重新计算并同时校验 RestoreKey 与 prefix fingerprint；
- Tool 实现或 Goal revision 等非 model-visible 语义变化由 RestoreKey 区分，不能污染稳定 prefix fingerprint。

### 5.3 Session identity

Session ID 不能只是一段无作用域字符串。公共 identity 必须足以隔离 Coding Agent、Denova Project、Writing Session、Story Branch 和 Automation Task。

```go
type SessionKey struct {
    Namespace  string
    ID         string
    Attributes map[string]string
}

type SessionSelector struct {
    Namespace  string
    ID         string
    IDPrefix   string
    Attributes map[string]string
}

func NamedSession(id string) SessionKey
```

规则：

- Namespace、ID 和 Attributes 共同参与 durable identity；
- 所有字段有独立 byte limit；
- Attributes key/value 排序后参与 canonical hash；
- identity 创建后不可变化；
- workspace path 等可变元数据不能作为长期 identity，除非宿主明确选择该语义；
- `NamedSession("main")` 为普通用户提供简洁默认 Namespace；
- Denova 由 application adapter 负责从 product identity 构造 SessionKey。
- Selector 的所有非空字段都是 AND 条件，且至少需要一个约束；
- `IDPrefix` 只能由明确使用层级式 ID 的宿主启用，普通调用优先使用 exact ID 或 Attributes。

### 5.4 Agent

```go
type Agent struct {
    // private
}

func (a *Agent) Run(context.Context, Input) (*Run, error)
func (a *Agent) Session(context.Context, SessionKey) (*Session, error)
func (a *Agent) CloseSessions(context.Context, SessionSelector) error
func (a *Agent) DeleteSessions(context.Context, SessionSelector) error
func (a *Agent) Close(context.Context) error
```

`CloseSessions` 释放 workspace、project、story 等 scope 的执行资源；`DeleteSessions` 在同一 selector fence 下级联终止 child Session 并持久删除数据。Selector 至少包含一个约束，不能默认匹配所有 Session。

Agent lifecycle 必须来自进程/App/workspace owner，不能来自 HTTP request。

### 5.5 Session

```go
type Session struct {
    // private
}

func (s *Session) Key() SessionKey
func (s *Session) Run(context.Context, Input) (*Run, error)
func (s *Session) Active(context.Context) (*Run, bool, error)
func (s *Session) AttachRun(context.Context, string) (*Run, bool, error)
func (s *Session) RunInput(context.Context, string) (Input, bool, error)
func (s *Session) Observe(context.Context, Cursor) (Observation, error)
func (s *Session) Snapshot(context.Context) (SessionSnapshot, error)
func (s *Session) Inspect(context.Context, Input) (Inspection, error)
func (s *Session) SyncTranscript(context.Context, TranscriptSyncRequest) (TranscriptSyncResult, error)

func (s *Session) Goal(context.Context) (GoalState, bool, error)
func (s *Session) UpdateGoal(context.Context, GoalMutation) (GoalState, error)

func (s *Session) Compact(context.Context, CompactionRequest) (CompactionResult, error)
func (s *Session) RemoveCompaction(context.Context, CompactionRemoveRequest) (bool, error)
func (s *Session) RecoveryActions(context.Context) ([]RecoveryAction, error)
func (s *Session) Recover(context.Context, string) error
func (s *Session) Clear(context.Context) error
func (s *Session) Delete(context.Context) error
func (s *Session) Close(context.Context) error
```

这些方法是 Agent 已拥有状态的必要操作，不是让调用者编排内部阶段：

- `UpdateGoal` 仍由 Goal Manager 验证 revision 和 transition；
- `Compact` 仍由 Compaction Manager 生成结果，由 Agent 原子提交 checkpoint；
- `RemoveCompaction` 只提交 effective-context marker，不删除 raw history；
- `Inspect` 复用真实 model request assembly，并同时返回 bounded `ContextFragments` provenance；`ModelRequest` 才是最终可见事实，调用者不得从 fragments 重建请求；该操作不 admission、不调用 provider/tool、不写状态；
- `SyncTranscript` 由拥有外部 canonical history 的宿主在 idle 状态原子同步 source revision/hash，重建时统一清理失效的维护投影；
- `Clear` 写 durable clear marker，不物理删除用户历史；
- `Delete` 终止并删除当前 Session 及其 durable child tree；
- `Close` 终止 actor、释放 Store lease，不删除 Session 数据。

如果自定义 Goal 使用完全不同的产品语义，可以不实现标准 Goal Manager，并通过自定义 Toolset/Context/Canonical Adapter 表达；此时标准 `Goal/UpdateGoal` 返回 typed unsupported error。

### 5.6 Run

一个 public Run 表示用户眼中的一次完整任务。goal continuation 可以包含多个内部 operation，但仍属于同一个 public Run。

```go
type Run struct {
    // private
}

func (r *Run) ID() string
func (r *Run) CommandID() string
func (r *Run) Replayed() bool
func (r *Run) Receipt() CommandReceipt
func (r *Run) Events() <-chan Event
func (r *Run) Steer(context.Context, Input) (CommandReceipt, error)
func (r *Run) FollowUp(context.Context, Input) (*Run, error)
func (r *Run) Queue(context.Context, Input) (*QueuedInput, error)
func (r *Run) Queued(context.Context, string) (*QueuedInput, bool, error)
func (r *Run) Abort(context.Context, AbortRequest) (CommandReceipt, error)
func (r *Run) Respond(context.Context, string, InteractionResponse) error
func (r *Run) Wait(context.Context) (Result, error)

func (q *QueuedInput) ID() string
func (q *QueuedInput) Receipt() CommandReceipt
func (q *QueuedInput) Cancel(context.Context, QueueControlRequest) (CommandReceipt, error)
func (q *QueuedInput) Interrupt(context.Context, QueueControlRequest) (CommandReceipt, error)
```

不变量：

- `Run` 从 durable admission receipt 的 cursor 建立 observation；即使执行先于调用者开始消费，早期 durable Event 也会从 Store replay，不会丢失；
- `Steer` 永远绑定当前 Run，不可能命中另一个 Run；
- `FollowUp` 创建新的 user-level Run；
- `Queue` 接受同一 Run 的下一 cycle 输入；queued input 可以通过自己的 handle 取消或提升为 interrupt；
- 调用 ctx 取消只停止等待，不撤销 durable accepted work；
- `Abort` 是唯一通用显式终止操作；
- 所有会改变执行状态的控制命令返回 durable `CommandReceipt`；调用者提供稳定 `IdempotencyKey` 后，冷重启重试会返回原 receipt 并设置 `Replayed`；
- `Wait` 返回 typed settlement，不要求解析 Event 或错误字符串。

### 5.7 Input

```go
type Input struct {
    Text           string
    IdempotencyKey string
    Context        []ContextFragment
    Goal           *GoalMutation
    HostData       *HostData
}

type HostData struct {
    Type    string
    Version uint16
    Data    json.RawMessage
}
```

`HostData` 只用于 Source 和 Adapter 恢复，永不自动进入模型上下文。它必须 versioned、bounded，并参与幂等 fingerprint。

### 5.8 Event、Observation 与 Result

```go
type Event struct {
    Cursor     Cursor
    Durability EventDurability
    RunID      string
    Payload    EventPayload
}

type Observation struct {
    Snapshot SessionSnapshot
    Events   <-chan Event
    Errors   <-chan error
}
```

Event 分为：

- durable：推进 Cursor，可 replay；
- ephemeral：不推进 Cursor，只在 live stream 中出现；
- missed ephemeral delta 必须可以通过 Snapshot 的 active output 重建；
- transport 不得假设每个 Event 都有唯一的新 Cursor。

核心 typed Event vocabulary：

- `RunAccepted` / `RunStarted` / `RunSettled`；
- `AssistantDelta` / `AssistantFinal` / `ThinkingDelta`；
- `ToolInputStarted` / `ToolInputDelta` / `ToolStarted` / `ToolProgress` / `ToolFinished`；
- `InteractionRequested` / `InteractionResolved`；
- `GoalUpdated`；
- `CompactionStarted` / `CompactionCommitted` / `CompactionFailed` / `CompactionSkipped` / `CompactionRemoved`；
- `TodoUpdated`；
- `RecoveryRequired` / `RecoveryResumed`；
- `ContextLimitReached`；
- `ArtifactProduced`。

`SessionSnapshot` 至少包含：

- durable Cursor 与 retention start；
- active/queued Run；
- active assistant/thinking output 及 truncated 标记；
- open tool calls；
- pending interaction；
- goal state；
- active compaction checkpoint；
- recovery pending/paused；
- recent terminal Run index；
- snapshot truncation 与 rehydrate 提示。

`Result` 使用穷尽状态：completed、failed、aborted、incomplete、blocked。需要用户交互时 Run 不会被错误标记为 completed。

## 6. Capability Interface

### 6.1 Session Store

Session Store 是 Agent durable state 的唯一公共存储 seam。它保存：

- input admission 与幂等记录；
- Run、queue、transcript 与 Event cursor；
- standard goal state；
- compaction checkpoint 与 marker；
- pending interaction；
- canonical intent/receipt 与 host-effect outbox metadata；
- recovery evidence。

它不保存 Denova book/story/workspace 的完整产品数据。

```go
package session

type Store interface {
    Open(context.Context, Key) (Log, error)
    List(context.Context, Selector) ([]Key, error)
    Delete(context.Context, Key) error
}

type Log interface {
    Replay(context.Context, func(Record) error) (ReplayStats, error)
    Append(context.Context, Revision, ...Record) (Revision, error)
    Close() error
}

type Record struct {
    Revision Revision
    Kind    string
    Version uint16
    Data    json.RawMessage
}
```

Store contract：

- `Open` 获得该 Session 的独占 execution lease；
- `Replay` 必须按 revision 流式读取，不能要求一次性加载完整历史；
- `Append` 对完整 batch 做原子 CAS；
- context cancellation 返回时必须能区分 definite no-commit 与 ambiguous commit；
- ambiguous commit 必须由 Adapter 自行查询并返回确定结果，或返回非 context typed error 触发 Agent 关闭 lease 后 replay；
- `Close` 释放 lease；
- `List` 只返回完整可打开的 Session，删除中的条目必须隐藏；
- `Delete` 幂等并与 `Open` 串行化，不能让已删除 Session 被并发旧 lease 复活；
- checkpoint、command index、generation GC 等属于 Store 实现的私有性能策略，不暴露 runtime reducer/checkpoint SPI，也不增加自定义 Store 的实现负担；
- 自定义 Store 通过同一 contract 获得完整正确性与恢复语义，冷重开性能取决于其自身索引策略；Agent 不通过 concrete type 为同一 Session 选择另一条日志。

内置实现：

- `session.Memory()`；
- `session/file` production file store：checksummed canonical Log 是唯一 durable authority；opaque reducer checkpoint、per-command receipt 与 coverage index 是同一 Log 上可丢弃、可验证、可重建的 sidecar；
- `sessiontest` Store contract suite，供自定义 Adapter 验证 CAS、lease、原子 batch、close/reopen replay、cancellation、List 与 Delete；具体介质的断电、尾部损坏和索引降级由该 Store 自己的 fault-injection suite 证明，公共 contract 不虚构通用 crash 注入能力。

内置 file Store 的 checkpoint 只回放配置阈值内的 canonical tail；历史 command 正向查询用 anchored receipt，未出现的 command 由 coverage index 判定。sidecar 缺失、损坏或与 canonical head 不一致时只作为 cache miss，回放唯一 canonical Log 后原地重建；sidecar 写失败不能改变 canonical commit 的成功语义。普通 decorator 隐藏私有加速时仍读取相同 Log，只可能退化为 O(history)，不会形成 durable identity 分叉。

### 6.2 Toolset

```go
type Toolset interface {
    Identity() CapabilityIdentity
    PrepareTools(context.Context, ToolRequest) ([]ToolDefinition, error)
}
```

Toolset 只产生本 cycle 的 immutable ToolDefinition，不拥有 lifecycle。

Agent 会：

- 按 Toolset 声明顺序展开工具；
- 在调用模型前一次性读取并验证 schema；
- 拒绝重名、无效 schema 或不完整 descriptor；
- 持久化 toolset identity 与 schema fingerprint；
- 恢复时验证相同顺序和 schema；
- 将 permission wrapper 安装在最靠近实际 `Tool.Run` 的内部位置。

保留唯一工具注册单元：

```go
type ToolDefinition struct {
    Tool                   Tool
    Descriptor             ToolDescriptor
    ImplementationIdentity CapabilityIdentity
}
```

`ImplementationIdentity` 描述 schema/descriptor 看不见的 endpoint、scope 与配置语义，并进入 RestoreKey、但不进入 provider-visible tool snapshot 或 prefix fingerprint。便捷的 schema-only `StaticTools` 只允许一次性内存 Agent；持久 Session 必须逐项声明 `ImplementationIdentity`，或由 `StaticToolsIdentified` 在 Toolset 层统一声明。所有 Adapter-backed 内置工具逐项声明，避免闭包行为变化却错误恢复旧 Run。

ToolResult 继续区分：

- `ModelContent`：唯一进入模型上下文的内容；
- `DisplayContent`：只用于 UI/terminal；
- `Details`：bounded structured metadata；
- `Artifacts`：存储在 transcript 外的大结果；
- `Effects`：交给 Canonical Adapter 的 typed host effects。

批量 Effects 支持部分成功与逐项 receipt，不能因单项失败丢弃整个批次的恢复证据。

### 6.3 Tool Result Processor、Artifact Storage 与 Cleanup

工具 endpoint 的原始结果不能先被宿主截断再交给通用 Agent。Agent 在 caller tool middleware 之后、事件发布和 transcript 持久化之前固定调用：

```go
type ToolResultProcessor interface {
    Identity() CapabilityIdentity
    Process(context.Context, ToolResultProcessRequest) (ToolResult, error)
}

type ToolArtifactStorage interface {
    ToolArtifactStore
    ToolArtifactVerifier
    Identity() CapabilityIdentity
}

type CleanupManager interface {
    Identity() CapabilityIdentity
    Plan(context.Context, CleanupPlanRequest) (CleanupPlan, error)
}
```

- `ResultProcessor` 是唯一固定 post-tool projection authority，负责 lossless artifact materialization、bounded model/display projection、protected receipt 与 recovery hint；
- `ArtifactStorage` 可替换为本地文件、对象存储或宿主实现；同一 backend 必须同时发布并验证 immutable reference，工具自行返回的引用不能仅凭字段看似完整就成为可恢复证据；其 identity 影响恢复但不污染模型稳定前缀；
- `CleanupManager` 只规划已结算、可恢复工具结果的可逆 replacement；Agent 拥有 raw history、target freeze、CAS、原子 final settlement、clear/recovery 与事件；
- Cleanup 与 Compaction 在每个 model seam 由 Agent 统一仲裁，普通路径最多提交一个 durable context mutation；hard overflow 可以只在 compaction side fork 中临时应用 cleanup projection；
- 内置 `toolresult.Standard` 与 `cleanup.Standard` 和自定义实现走完全相同的 Interface。

### 6.4 Context Source

```go
type ContextSource interface {
    Identity() CapabilityIdentity
    Materialize(context.Context, ContextRequest) ([]ContextFragment, error)
}
```

每个 Fragment 必须包含：

- source；
- purpose；
- placement；
- resource；
- 稳定 revision/ref；
- hard byte limit；
- model-visible content。

Context Source 可以组合，但最终顺序由 Agent 固定为：

```text
stable instructions
-> stable tool schemas
-> stable long-lived context
-> compaction checkpoint
-> recent transcript
-> current input context
```

恢复策略只能二选一：

- 持久化 materialized content；
- 持久化稳定 resource/revision/ref，并在恢复时验证完全一致。

不能在恢复时静默读取已经变化的 workspace 内容。

`ContextSource` 是动态上下文进入 Agent lifecycle 的唯一入口。`agent/context.Assembler` 只是可复用的纯装配器；需要复用它的宿主通过 `context.ExportLifecycleFragments` 一次性导出带完整 provenance、revision 和 hard limit 的 `ContextFragment`，不再维护第二套 Context projector/lifecycle。

### 6.5 Goal Manager

Goal Manager 是完整的 goal capability，不只是一个 model tool：

```go
type GoalManager interface {
    Identity() CapabilityIdentity
    Apply(context.Context, GoalApplyRequest) (GoalState, error)
    Prepare(context.Context, GoalPrepareRequest) (GoalPreparation, error)
    AfterRun(context.Context, GoalAfterRunRequest) (GoalContinuation, error)
}
```

职责：

- 定义 goal 状态和 revision transition；
- 生成 goal context；
- 提供 model-visible goal tools；
- 决定 completed、blocked 或继续；
- 产生 stable continuation identity；
- 支持 crash recovery 和幂等 Apply。

Agent 负责：

- 将状态变化与 Session revision 协调；
- 发布 `GoalUpdated`；
- 创建或恢复 continuation；
- 将多个 continuation 聚合到同一个 public Run；
- 防止 Goal Manager 自己绕过 Run queue。

内置 `goal.Standard()` 提供 Denova 当前 objective、active/paused/completed/blocked/cleared 语义。自定义实现可以替换 transition 校验、tool schema、prompt 和 continuation policy；Goal state 的 CAS、持久化与恢复仍由 Agent 通过调用者选择的 Session Store 统一负责，Manager 不创建第二份 Goal 存储。

公共 `GoalState` 只固定稳定 ID、开放的 status、单调 revision 与 mutation fence，并提供 manager-owned opaque JSON `Data`；`GoalMutation` 同样允许自定义 kind 和 opaque payload。Agent 校验幂等 mutation ID、JSON、精确 revision 和非空 lifecycle envelope，自定义 Manager 拥有其余状态机语义。`goal.Standard()` 不接受自定义 Data，避免把标准协议与应用私有协议混在一起。

### 6.6 Compaction Manager

```go
type CompactionManager interface {
    Identity() CapabilityIdentity
    SummaryLimitBytes() int
    Plan(context.Context, CompactionPlanRequest) (CompactionPlan, error)
    Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error)
}
```

Compaction Manager 只负责：

- 判断是否需要 compact；cleanup 由独立 `CleanupManager` 规划，二者由 Agent 固定仲裁；
- 选择 source range；
- 使用当前真实 model request snapshot 生成 checkpoint；
- 返回有来源、hash、token estimate 和 replacement marker 的结果。

`CompactionManager` 是受信任的结构能力，不是 model visibility 的保密沙箱；选择可逆 raw range 必然需要访问完整 transcript。`CompactionPlanRequest` 同时携带 raw `Messages`、最终 provider-visible `ModelRequest`，以及 model step 的 immutable `ModelRequestSnapshot`。`CompactionCompactRequest` 进一步区分：

- `Messages`：Agent 保存的完整 raw transcript，只用于 source-range hash、CAS、remove 与恢复；
- `SourceMessages`：真正交给 Summarizer 的 canonical 增量源；默认首次为选中 raw range，后续为当前 checkpoint marker 加新选中的 raw tail。Agent 不会猜测任意 caller middleware 与 raw index 的反向映射；宿主若有 model-only visibility policy 或领域历史，必须用带稳定 Identity 的 Manager Adapter 显式投影该字段；
- `ModelSnapshot`（类型为 `*ModelRequestSnapshot`）：caller middleware、model、tools、thinking、provider options 与 cache route 均已确定的最终请求，可用于 append-only side fork；automatic 与 manual structural compaction 都提供 exact snapshot。

两种消息不能复用同一个字段。否则第二次 compaction 会尝试从 provider-visible request 中查找已经被 checkpoint 隐藏的 raw history，导致无法继续推进。

Agent 负责：

- source snapshot 与 expected revision；
- manual/automatic admission；
- checkpoint 与 effective-context marker 原子提交；
- raw history 保留；
- crash recovery；
- `Compact/RemoveCompaction` Session Interface；
- Event publication。

Automatic compaction 是 recoverable side fork：摘要失败会发布 `CompactionFailed`、更新 Agent Store 中的结构 fingerprint/连续失败计数，并继续执行未改写的 primary model request；同一结构连续失败达到 `ExecutionPolicy.MaxAutomaticCompactionFailures` 后发布 `CompactionSkipped` 并打开 durable fuse。最终 model request、active checkpoint、clear revision、model/manager identity 或 options 发生变化时会得到新的尝试；manual compact 不受该 fuse 阻断。provider input hard limit 仍由最终输入 guard 强制执行。

内置实现：

- `compaction.Standard(config)`：基于真实 model request pressure 规划 source range，并通过注入的 Summarizer 生成 side-fork checkpoint；tool-result artifact/receipt 由 Tool result policy 独立负责；
- `compaction.Disabled(hardLimitBytes, summaryLimitBytes)`：显式禁用摘要生成，但仍以调用者给定的 hard limit 做最终保护并返回 typed error；
- `compactiontest` contract suite。

### 6.7 Permission Policy

```go
type PermissionPolicy interface {
    Identity() CapabilityIdentity
    Evaluate(context.Context, PermissionRequest) (PermissionDecision, error)
    Resolve(context.Context, PermissionResolveRequest) (PermissionResolvedDecision, error)
}
```

Decision 只允许 allow、ask、deny。

`Resolve` 用于在 interaction waiter 恢复前提交“always allow”一类持久规则。Agent 必须按以下顺序执行：

```text
persist InteractionRequested
-> publish Event
-> receive response
-> Permission.Resolve / persist remembered rule
-> persist InteractionResolved
-> resume exact tool call
```

内置策略：

- `permission.SafeDefault()`；
- `permission.ReadOnly()`；
- `permission.Coding()`；
- `permission.FullAccess()` 仍拒绝 critical deny rules；
- `permission.SafeDefaultWithRules(store)` / `permission.CodingWithRules(store)` 显式绑定唯一可持久 RuleStore；构造器拒绝 nil、无稳定 identity 或隐式多 Store 组合；
- `permissiontest` contract suite。

### 6.8 Interaction Policy

Interaction 是 Agent 内建 durable lifecycle。Policy 只定义 typed vocabulary、validation 和 presentation limits：

```go
type InteractionPolicy interface {
    Identity() CapabilityIdentity
    ValidateRequest(context.Context, InteractionRequest) error
    Resolve(context.Context, InteractionRequest, InteractionResponse) (InteractionResolution, error)
}
```

标准 vocabulary 同时支持：

- 1-3 个问题；
- free text；
- single/multi-select；
- recommended option；
- host-provided Other；
- cancel；
- tool approval allow-once、remember、deny。

`tools.Ask()` 使用当前 Run 的 interaction client，不直接依赖 terminal、HTTP 或 Denova Session。

### 6.9 Canonical Adapter

Canonical Adapter 是复杂产品宿主的高级 seam。Basic/Coding Agent 没有 product store 时使用 no-op Adapter。

```go
type CanonicalAdapter interface {
    Identity() CapabilityIdentity
    MaterializeInput(context.Context, InputCommitRequest) (CommitReceipt, error)
    CommitOutput(context.Context, OutputCommitRequest) (OutputCommitReceipt, error)
    Reconcile(context.Context, ReconcileRequest) (ReconcileResult, error)
    ApplyEffects(context.Context, []EffectRequest) ([]EffectResult, error)
}
```

Agent 持有并持久化 commit identity、semantic hash、intent、receipt 和 effect outbox。Adapter 不接触 Agent journal codec，也不决定执行顺序。

约束：

- MaterializeInput、CommitOutput 和 ApplyEffects 必须按 exact identity 幂等；
- Reconcile 只能查询，不能猜测性重放写入；
- receipt revision 必须稳定；
- effect 返回逐项结果，支持部分成功和后续重试；
- output receipt 未确认前不能交付下游 host effect；
- Canonical Adapter 不得将 product content 塞入 Agent State record；
- `canonicaltest` 提供 fault-injection contract suite。

Denova 为 writing session、project conversation、story branch、automation 等提供自己的 Adapter。lore/book/story 仍然属于 Denova。

### 6.10 Middleware

Middleware 继续支持 provider-neutral model customization，但被定义为 trusted in-process extension：

- 可以重写 model messages/options；
- 可以包装 model；
- 可以包装 tool endpoint 的调用前后逻辑；
- 不能替换 Session Store、permission fence、canonical commit 或 recovery；
- internal permission wrapper 永远位于实际 `Tool.Run` 之前；
- 会影响模型前缀的 Middleware 必须提供稳定 cache identity；
- durable Session 使用的 Middleware 必须可由 RestoreKey 重建；
- Stateful Middleware 必须把 run state 放入传入 context/state，不能在共享实例中保存未同步状态。

### 6.11 Execution Policy 与 Limits

不使用大量 Option 或 feature flag，统一为两个具名结构：

```go
type ExecutionPolicy struct {
    Retry                          *RetryConfig
    RetryIdentity                  CapabilityIdentity
    ToolParallelism                int
    MaxIterations                  int
    IdleTimeout                    time.Duration
    MaxAutomaticCompactionFailures int
}

type Limits struct {
    ObservationBuffer      int
    MaxOpenSessions        int
    RetainedEventLimit     int
    RetainedMessageLimit   int
    RetainedCommandLimit   int
    ProjectionTextMaxBytes int

    MaxInputBytes          int
    MaxContextFragments    int
    MaxEngineStateBytes    int64
    MaxInteractionBytes    int64
    MaxPendingInteractions int
}
```

- `MaxIterations == 0` 表示无限；
- `IdleTimeout == 0` 表示无限；正值只限制连续没有 model chunk、tool lifecycle/progress 或 Interaction 的空闲窗口，不是总运行时限；
- durable Session 启用 `Retry` 时必须提供稳定的 `RetryIdentity`，不能把 closure 地址当作恢复身份；
- `MaxAutomaticCompactionFailures == 0` 使用 Agent 内置默认值 3；该限制只阻止同一 model request 结构反复执行自动摘要，不限制 Run 时间或模型迭代次数；
- 不提供隐式总超时；
- limits 默认高于实际模型上下文片段常见规模，不能在低于 50 KB 的位置随意截断；
- 每个 limit 有默认值、作用域、错误类型和是否影响模型上下文的说明；
- 超限返回 typed error 或 incomplete settlement，不能静默丢失。

## 7. 内置 Toolsets 与 Plugins

### 7.1 Core 通用工具

`agent/tools` 提供以下可独立选择、也可组合的 Toolset：

| Toolset | 内置工具 | 依赖 seam |
| --- | --- | --- |
| Workspace | read、glob、grep、write、edit | Workspace/FileSystem Adapter |
| Shell | bash、pwsh | Process Adapter |
| Ask | ask | Run interaction client |
| Todo | todo read/update/replace/clear | Todo Store 或 standard Session records |
| Skills | skill list/read | Skill Source |
| Tasks | task start/observe/steer | Task Executor |

要求：

- 每个工具都可以单独构造；
- Toolset preset 只负责组合，不维护第二份 Tool registry；
- Workspace tool 支持 read-only/read-write 明确访问模式；
- Shell tool 复用成熟 process/PTY 能力，支持 bash/pwsh 与进程树终止；
- Ask 复用 durable Interaction；
- Todo 产生标准 `TodoUpdated` Event；
- Skills 只读取明确 Source，不假设 Denova 的 skills 路径；
- Tasks 依赖注入的 Executor，可以连接本地 subagent、远端 worker 或宿主任务系统；
- 所有工具使用同一 ToolDefinition、permission、scheduler、recovery 和 result projection。

### 7.2 Tool dependency Interfaces

```go
type SkillSource interface {
    List(context.Context, SkillQuery) ([]Skill, error)
    Read(context.Context, SkillRef) (SkillContent, error)
}

type TaskExecutor interface {
    Identity() agent.CapabilityIdentity
    Start(context.Context, TaskRequest) (Task, error)
    Observe(context.Context, TaskRef, string) (TaskObservation, error)
    Steer(context.Context, TaskRef, agent.Input) error
    Respond(context.Context, TaskRef, string, agent.InteractionResponse) error
    Abort(context.Context, TaskRef, agent.AbortRequest) error
}
```

实现还可以提供可选的 `TaskAgents() []TaskAgentInfo` 目录，让模型按稳定名称选择 child。Task 批量输入与结果必须支持部分成功，避免一个无效子任务导致整个高成本调用失败；内置 `LocalTasks` 把具名 child 接到公共 `Agent → Session → Run`，不维护 authoritative 内存 task map。

### 7.3 可选 Plugins

复杂工具放在独立 import path，但仍跟随同一个 `agent` module 版本：

```text
agent/plugins/websearch
agent/plugins/webfetch
agent/plugins/browser
```

原则：

- core 不依赖搜索供应商、HTML parser 或浏览器运行时；
- plugin 通过 Toolset 接入，不获得特殊 lifecycle；
- websearch 注入 Search Provider；
- webfetch 复用成熟 HTTP、robots、content extraction 与 size limit 依赖；
- browser 注入 Browser Controller，可由 Playwright、CDP 或远端 browser Adapter 实现；
- plugin tools 仍经过 permission、result limits、artifact 和 recovery；
- provider credential、network policy、proxy 和审计由宿主配置；
- 不在第一阶段把 Denova 浏览器实现搬进公共 module，只抽取真正通用的 Interface 与 Adapter。

## 8. Package 设计

```text
github.com/alfredxw/denova/agent
  agent                     Agent / Session / Run / Definition / Event
  agent/providers           provider-neutral model configuration
  agent/providers/...       concrete protocol Adapters
  agent/session             Store Interface + memory implementation
  agent/session/file        production file Store
  agent/session/sessiontest Store contract tests
  agent/context             Fragment / Source / composition helpers
  agent/goal                Manager + standard implementation
  agent/goal/goaltest       Manager contract tests
  agent/compaction          Manager + standard implementation
  agent/compaction/compactiontest
  agent/permission          Policy + built-in policies
  agent/permission/permissiontest
  agent/interaction         standard interaction vocabulary
  agent/canonical           product canonical Adapter Interface
  agent/canonical/canonicaltest
  agent/tools               read / glob / grep / write / edit / bash / pwsh /
                            ask / todo / skills / tasks 与 Toolset composition
  agent/toolresult          固定 post-tool processor 的内置实现
  agent/cleanup             CleanupManager 内置策略
  agent/plugins/websearch
  agent/plugins/webfetch
  agent/plugins/browser
  agent/internal/runtime    facade 私有的 durable actor/reducer/execution
  agent/internal/sessionfile built-in file Store 私有优化
```

durable actor、reducer、engine checkpoint 与 journal codec 是 facade 的私有实现，不是高级消费者的扩展点；故障注入测试留在内部 package。Denova application 不 import 或构造它们。第一版不提供通用 SSE package：HTTP/SSE、terminal、GUI 与 AI SDK UI 的事件编码属于宿主 transport，统一从 `Session.Observe` 消费即可，避免 core 绑定某种网络栈。

公共 primitive 放在拥有它的 package，不能为了避免 import cycle 建立使用者需要理解的 `core`、`contracts` 或 `common` package。若内部需要共享 primitive，可以使用私有 contracts package，并由正确的公开 package alias。

## 9. Denova 组合方式

### 9.1 Denova 保留的职责

- General、Project/IDE、Interactive Story、Config Manager、Image、Interactive Director 的 prompt 与 product policy；
- Version Summary、Tool Agent、Automation Trigger 等有界一次性 model task 的配置、prompt、输出校验和调用记录；
- Context Compaction summarizer 的配置与 prompt；
- Automation 的任务编排、调度和 product policy；Automation execution Run 使用所属 Project Agent，不建立独立的公共 Agent 类型；
- model/provider/profile 配置解析；
- workspace、project、story、branch、automation identity 到 SessionKey 的映射；
- lore、book、story、review、image、automation 等 product Toolset；
- writing/game/product Context Source；
- book/story/workspace canonical Adapter；
- permission rule 的用户配置和持久化；
- project/workspace lifecycle 与 admission；
- bilingual UI DTO、Hertz SSE 与 AI SDK projection；
- 产品 trace metadata。

### 9.2 公共 Agent 接管的职责

- model/tool loop；
- Session durable state、transcript 与 revision；
- Run identity、幂等和 queue；
- steer/follow-up/abort；
- goal lifecycle 与 continuation coordination；
- automatic/manual compaction coordination；
- durable interaction；
- permission execution fence；
- canonical intent/receipt barrier；
- host-effect outbox/reconciliation；
- crash recovery；
- cursor replay/live observation；
- context provenance、prefix fingerprint 和 byte budgets；
- common Tool scheduler、recovery 与 artifacts。

### 9.3 Dynamic Source

```go
func buildDenovaDefinition(
    ctx context.Context,
    request agent.PrepareRequest,
) (agent.Definition, error) {
    binding, err := denovaBindingFromSessionKey(request.Session.Key)
    if err != nil {
        return agent.Definition{}, err
    }

    switch binding.DefinitionKind {
    case DefinitionGeneral:
        return buildGeneralDefinition(ctx, binding, request)
    case DefinitionProject:
        return buildProjectDefinition(ctx, binding, request)
    case DefinitionInteractiveStory:
        return buildInteractiveStoryDefinition(ctx, binding, request)
    case DefinitionConfigManager:
        return buildConfigManagerDefinition(ctx, binding, request)
    case DefinitionImage:
        return buildImageDefinition(ctx, binding, request)
    case DefinitionInteractiveDirector:
        return buildInteractiveDirectorDefinition(ctx, binding, request)
    default:
        return agent.Definition{}, fmt.Errorf(
            "unsupported Denova definition kind %q",
            binding.DefinitionKind,
        )
    }
}
```

这里的 `DefinitionKind` 是 Denova composition key，不是公共 module 的 enum：

- Writing 与 AgentChat IDE 共享 Project Definition 的长期边界，但可按 Session binding 选择不同 Context、Toolset 和 product policy；
- Version Summary、Tool Agent 与 Automation Trigger 保持为 Denova 内部 bounded model task，直接复用 provider-neutral Model Interface、Trace 和 context limits；它们不进入 admitted conversational turn，因此不伪造 Session/Run；
- Context Compaction 配置作为 summarizer/model dependency 注入 `compaction.Manager`，不伪装成用户 Session；
- Automation execution 将 task identity 放入所属 Project Session 的 Input/HostData，以普通 Run 进入 queue、canonical commit 和 recovery，不再保留 standalone Automation Agent 路径。

Writing 示例组合：

```go
writingTools, err := tools.Combine(
    standardWorkspaceTools,
    standardAskTodoSkillTaskTools,
    denovatools.Book(bookService),
    denovatools.Lore(loreService),
    denovatools.Review(reviewService),
)
if err != nil {
    return agent.Definition{}, err
}
return agent.Definition{
    Key:          builtDefinitionKey,
    Name:         "writing",
    Model:        model,
    Instructions: writingPrompt,
    Tools:        writingTools,
    Context:     denovacontext.Writing(...),
    Goal:        goal.Standard(),
    Compaction:  denovaCompaction,
    Permission:  denovaPermission,
    Canonical:   denovacanonical.Writing(...),
    Middlewares: denovaModelMiddleware,
    Execution:   denovaExecutionPolicy,
}, nil
```

Interactive Agent 不是公共 module 内置 preset。Denova 使用：

- Game/Director prompt；
- story/branch SessionKey；
- Denova interactive Toolset；
- story Context Source；
- story canonical Adapter；
- 公共 Session、Run、interaction、permission、compaction、recovery 和 Event。

这样 product 能力留在 Denova，通用 Agent lifecycle 不重复实现。

Game 的 compaction 需要额外遵守一个产品投影边界：public Agent transcript 保留 canonical raw story turn，而本轮渲染后的完整 story prompt 只属于 model-visible projection，不能反向覆盖 raw history。Denova Game conversation 在 `CompactionManager` Adapter seam 提供：

- 从当前 Agent checkpoint 的 `SourceTurnCount` 到新安全边界的 canonical story turn 增量；
- 未完成/待恢复玩家输入之前的原子 source boundary；
- versioned、bounded 的 story/branch/turn cursor `ContextData`。

Adapter 只替换 `SourceMessages`、应用同一 model-only visibility policy，并附加 product cursor；其 Identity 必须包含 canonical source 与 visibility policy。Agent 仍独占 raw transcript replacement range、hash/CAS、checkpoint revision、automatic/manual admission、failure fuse、remove 和 recovery。Writing 的 canonical source 直接使用通用 Agent transcript，但 Denova Manager 仍显式应用 Tool Result 可见性策略，不能依赖 Summarizer 偶然忽略隐藏正文。旧 Session/Story automatic compaction、post-settlement checkpoint，以及 product health API、journal vocabulary 和 projection state 已删除，不能与 Agent checkpoint 双写。

### 9.4 当前 Denova 能力覆盖矩阵

| 当前能力 | 新设计归属 | 等价验收重点 |
| --- | --- | --- |
| General Chat | General Definition + 内置通用 Toolsets | model/tool loop、history、SSE、approval、skills/tasks 不缺失 |
| Writing 与 AgentChat IDE | Project Definition + Denova Context/Toolset/Canonical Adapter | workspace/book/lore/review、版本状态、用户输入与模型输出精确提交 |
| Interactive Story/Game | Interactive Story Definition + Denova story/branch Adapter | branch identity、story context、互动工具和恢复语义保持 |
| Interactive Director | Denova Director Definition/Toolset | event/lore 读取、输出提交和 scoped Session 保持 |
| Config Manager | Denova Config Toolset/Canonical Adapter | read/apply、幂等、审批、配置 revision 保持 |
| Image Agent | 内置 workspace/web Toolsets + Denova Image Toolset/Adapter | generation artifact、host effect、失败恢复保持 |
| Version Summary | Denova bounded model task + provider-neutral Model Interface | 独立 model/prompt 配置、输出约束和调用记录保持，不额外制造 Session |
| Tool Agent | Denova bounded model task，可被 product Toolset 调用 | tool payload、结构化校验、fallback、错误和调用记录保持 |
| Context Compaction | 可替换 Compaction Manager + Denova summarizer 配置 | automatic/manual compact、remove、health、side-fork 与 raw history 保持 |
| Automation execution | 所属 Project Agent 上的普通 Run + Denova automation Adapter | task identity、queue、canonical materialization、host effect 和 recovery 保持；不保留 obsolete standalone Agent |
| Automation trigger evaluation | Denova bounded model task，复用所属 Project model 配置 | bounded context、structured output、trace 和失败处理保持 |
| Goal/Todo | 公共 Goal Manager/Todo Toolset；Denova 组合公共状态并投影现有 UI | set/pause/resume/clear/finish/blocked、continuation 和 UI Event 保持；自定义实现不要求复制 Agent lifecycle |
| Ask 与 Tool Approval | 公共 durable Interaction 与 `Definition.Permission` 单一 authority | pending 恢复、allow-once/remember/deny、双语 projection 保持且不发生二次审批 |
| Steer/Follow-up/Abort | 公共 Run control | 精确绑定、queued cancellation、重启后 public Run identity 保持 |
| Session/History/Observation | Session Store + Snapshot + durable Event cursor | scoped close、replay/live 无缺口、cursor expired rehydrate 保持 |
| Tool recovery 与 product commit | Tool descriptor + Canonical Adapter + effect outbox | read-only/idempotent/reconcilable/non-idempotent 与所有 crash window 保持 |
| Web Search/Fetch/Browser | 可选 plugin Toolsets | 原配置开关、permission、network policy、artifact/result limits 保持 |

矩阵中的“保持”指行为能力与用户可见结果保持，不要求保留旧类型、旧 package 或旧 journal shape。当前 module 未发布，迁移应直接删除 obsolete 路径，不添加 compatibility wrapper。

### 9.5 Denova 与公共 Agent 的最终边界

最终 Denova production composition 不直接构造或 import `runtime.Engine`、`Harness`、`JournalStore`、runtime domain-commit/host-effect record 或 structural restore descriptor；这些 durable protocol 由公共 `Agent -> Session -> Run` facade 统一拥有。

初稿把所有 `OperationID` 和 registry 都列为必须删除，实际实现后将其修正为更准确的边界：

- 公共 Agent 只暴露 `RunID`、`CommandID`、`Cursor` 与 `CommandReceipt`，不向使用者暴露内部 runtime command 类型；
- Denova 的 Automation ledger、游戏 turn metadata 和现有 transport DTO 仍可把公开 `RunID` 投影为 product-owned `operation_id` 字段，用于跨 product store 的精确对账。这是宿主数据身份，不是对 Agent journal 的访问；
- Denova 保留 Definition resolver registry，用 `DefinitionKey` 重建动态 Source。它是 composition registry，不是旧 runtime Profile registry；
- Denova adapter 内仍有 `NextTurn`、recovery action 等产品 DTO，用于保持现有 UI/Automation 语义；adapter 最终只调用 public `Run.FollowUp`、`Run.Queue`、`QueuedInput.Interrupt/Cancel` 和 `Session.Recover`；
- Version Summary、Tool Agent 与 Automation Trigger 这类不参与会话恢复的 bounded model task 可以直接使用 provider-neutral Model Interface；通用 `task`/subagent 则必须通过 child `Agent -> Session -> Run`，使 detached observe、steer、interaction、abort、maintenance 和冷恢复共享同一公共生命周期。Denova 的 conversational root 不存在第二条 Runner 路径。

这样既没有在公共 package 中保留兼容层，也没有为了统一术语而破坏 Denova product ledger 的精确恢复能力。

## 10. 缓存、上下文与恢复不变量

### 10.1 三种 identity

必须区分：

- DefinitionKey：Source 查找业务配置的稳定身份；
- RestoreKey：Agent 根据实际 assembly 计算的完整恢复身份；
- PrefixFingerprint：标识 model-visible 稳定前缀。

DefinitionKey 相同但实际 Capability identity 改变时，RestoreKey 必须改变；恢复旧 Run 时如果 Source 无法重建旧 RestoreKey，返回 `ErrDefinitionUnavailable`，不能使用新 assembly 继续旧 Run。

以下变化必须改变 RestoreKey：

- Tool 实现语义；
- Goal/Compaction/Permission/Canonical Adapter identity；
- Middleware 行为；
- Execution Policy；
- 恢复 descriptor 版本。

以下变化必须改变 PrefixFingerprint：

- provider、protocol、model、endpoint；
- system instructions；
- tool name/schema/order；
- provider cache-sensitive options；
- stable long-lived context revision。

Run ID、timestamp、cursor、trace ID、task display ID 不得进入稳定前缀。

### 10.2 Raw history 与 compaction

- raw transcript 永不因 compaction 删除；
- checkpoint 可追溯、可替换、可移除；
- summary 和 effective marker 同 revision 提交；
- context cleanup 保留可恢复 artifact/ref；
- manual 和 automatic compaction 共享同一 Manager 与 commit path；
- 每个 model step 最多选择一个可持久化上下文维护动作：普通 cleanup 成功后不再 compact；hard overflow 可先在同一 side fork 临时收缩 recoverable tool body，再由 Agent 只提交 checkpoint；
- automatic planning 必须发生在全部 caller model middleware 之后，使用 exact `ModelRequestSnapshot`，而不是重新猜测 model/options/tools；
- checkpoint 的 raw replacement range 与 Summarizer 的 incremental `SourceMessages` 是两个独立概念；Manager 是有意读取 raw 的受信任能力，host 的 model-only visibility 必须由带 Identity 的 source Adapter 显式应用；
- product-owned rendered context 可以通过窄 Adapter 提供 canonical summary source 与 opaque cursor，但不能提交第二份 checkpoint；
- automatic summary failure 不终止可执行的 primary request；failure/skipped lifecycle 和 durable fuse 必须可观测；
- compaction 不能改变 Definition/tool prefix；
- hard limit 超限不能静默截断。

### 10.3 Recovery

- durable accept 后 Definition 暂时不可用时保持 recovery pending，不取消输入；
- unfinished read-only/idempotent/reconcilable/non-idempotent tools 按 descriptor 穷尽处理；
- ambiguous canonical commit 必须 query/reconcile，不能猜测性重放；
- remembered permission rule 必须在 waiter resume 前持久化；
- pending interaction 必须绑定 Run、tool call 和 canonical revision，只能 resolve 一次；
- Snapshot 必须向 UI 暴露 typed recovery state；
- 恢复成功后发布 `RecoveryResumed`，继续原 public Run。

## 11. 公开错误

只公开调用者可以采取行动的 typed error：

```go
ErrAgentClosed
ErrInvalidInput
ErrSessionBusy
ErrSessionClosed
ErrRunSettled
ErrNoActiveRun
ErrInteractionStale
ErrDefinitionUnavailable
ErrDefinitionMismatch
ErrContextLimit
ErrRecoveryRequired
ErrRecoveryStale
ErrCursorExpired
ErrPermissionDenied
ErrPermissionArgumentsChanged
ErrIdleTimeout
ErrCanonicalCommitRejected
ErrTranscriptSyncConflict
ErrCapabilityUnsupported
```

CAS conflict、lease protocol、record codec、actor failure等内部错误必须重试、恢复或包装。调用者不解析内部字符串。

## 12. 实施与迁移验收状态（2026-08-13）

本表只作里程碑摘要；逐项策略、迁移前测试映射、当前证据和未闭环问题统一记录在 `agent-package-migration-parity-audit.md`。任何一项在审计台账仍为 P1、审查中或待复核时，本节不得宣称整体迁移完成。

| 方案项 | 状态 | 最终落点 |
| --- | --- | --- |
| `Agent -> Session -> Run` facade | 已实现 | Basic 临时 Session、具名 Session、active/attach/observe/snapshot/clear/close 均由同一 facade 提供 |
| memory/file Session Store | 已实现 | 单一 canonical append-only CAS Log、exclusive lease、streaming replay、checksum/不完整尾部恢复与可复用 contract suite；内置 file Store 的 checkpoint、anchored command receipt 与 coverage index 是同一 Log 上的可重建 sidecar，cold replay 有界且损坏后 fail-open 重建，详见 LIFE-016 |
| Definition/Source 恢复 | 已实现 | durable CycleStarted 只捕获一次 `StartedAt`；accepted input 随后闭合 provider-free canonical outbox，preparation 持久化 exact restore descriptor，并在执行前校验 Canonical identity、RestoreKey、PrefixFingerprint 与 materialized context；Interaction/checkpoint 恢复不会重新读取 wall clock |
| Run control | 已实现 | start/steer/follow-up/queue、`QueuedInput.Interrupt/Cancel` 与 abort 均有 caller-owned idempotency 和 durable receipt；public backend 已覆盖按 TargetCommandID 取消已接受 follow-up，恢复动作的 opaque ActionID 可经 transport/UI 原样往返 |
| Goal/Todo | 已实现 | 标准 Manager、Session API、model tool、continuation 与 typed Event 共享 Agent-owned capability state；Denova 直接组合公共 Goal/Todo 并保持原 UI 投影 |
| Compaction | 已实现 | exact final model-request snapshot、增量 SourceMessages、automatic/manual compact/remove、raw history 保留、同 cycle context rematerialization、非致命失败/fuse 与 checkpoint recovery；Game 使用 canonical turn source + opaque story cursor |
| Interaction/Permission | 已实现 | 公共 durable ask/approval、双语 vocabulary、remembered rule 先持久化再恢复 tool call；Denova 通过 `Definition.Permission` 绑定动态 rule store，公共 permission fence 是唯一 authority |
| Canonical/effect | 已实现 | input/output intent、query-only reconcile、逐项 effect receipt 与 crash-window recovery |
| Context/cache identity | 已实现 | fragment provenance/hard limit、exact model request snapshot、stable prefix fingerprint 与 restore identity 分离 |
| Core tools | 已实现 | read、glob、grep、write、edit、bash、pwsh、ask、todo、skill、task 可单独选择或组合 |
| Optional plugins | 已实现 | websearch、webfetch、browser 均以注入 Provider/Controller 的 Toolset 提供，支持批量部分成功 |
| Denova root migration | 已实现 | General/Project/Writing/Game/Config/Image/Director 已接 public facade；Game existing-branch/cold/fork/edit/regenerate transcript sync、Writing/Game exact context inspection 与旧 Story maintenance 删除均有生产证据 |
| Automation cold recovery | 已实现 | 直接从 public Agent Session Store 恢复并用 receipt/fingerprint 精确对账，不读取旧 `agent-runtime` journal |
| 单一 compaction authority | 已实现 | Writing 与 Game 旧 maintenance authority 均已删除；公共 Session 独占 raw transcript、Cleanup、Compaction、cutoff/remove/recovery |
| Nested task lifecycle | 已实现 | child 使用 public Session/Run，具备 stable route、完整 typed event、interaction/cursor、冷恢复、级联删除；legacy Runnable 已清零 |
| Public surface | 已实现 | runtime actor/reducer 已移入 `agent/internal/runtime`；旧 Loop/Runner/Runnable/AgentInput/AgentEvent 与 engine cancel/interrupt 入口已删除或私有化 |
| 兼容策略 | 已实现 | module 未发布，obsolete 类型、协议、journal authority 和产品旁路已直接删除，没有兼容层 |

已经建立的测试面覆盖 Basic loop、tool schema/order、实时 tool name/argument delta、完整 tool descriptor/projection、幂等 replay/conflict、steer/follow-up/queued cancel/interrupt、NextTurn、各阶段 abort、Goal/Todo、Plan/Ask、automatic compaction failure/fuse、hard-overflow cleanup、repeated checkpoint、ask/approval、cursor observation、scoped close、context limit、canonical/effect fault injection、Definition recovery、same-run trace、Automation 冷恢复，以及 Writing/Game/nested task 的生产 Adapter 链路。

公共 contract suite 位于 `session/sessiontest`、`goal/goaltest`、`compaction/compactiontest`、`permission/permissiontest` 与 `canonical/canonicaltest`。其中 `goaltest` 验证内置 standard protocol；采用开放 status/mutation kind 与 opaque Data 的自定义 Goal 通过公共 lifecycle 集成测试验证自己的协议。第三方 Store 可直接复用，例如：

```go
func TestMySessionStore(t *testing.T) {
    sessiontest.RunStoreContract(t, newMyStore)
}
```

## 13. 实现中确认的设计修正

以下是相对初稿的主动修正，均以长期边界和实际故障窗口为依据：

1. **nil Compaction 表示禁用。** Basic Agent 不应隐式多发一次昂贵模型请求；需要 hard-limit-only 行为时显式使用 `compaction.Disabled`。
2. **input admission 早于 host context materialization。** 原始 Input、HostData 与 restore descriptor 先 durable accepted，再读取会变化的 workspace/story 状态，避免 crash 后重建成另一份输入。
3. **Provider-free canonical input 先于 Definition preparation。** accepted input 的 canonical outbox 先闭合，随后才允许 Source.Prepare、Context/Tool materialization 与 Definition checkpoint；checkpoint 再校验 prepared Canonical identity 与 durable receipt 一致，避免上下文准备阻塞或失败造成用户输入分叉。
4. **compaction 使用两阶段 context materialization。** 先 materialize 得到真实 model request 并规划 checkpoint；同 cycle 成功 compact 后再 materialize 一次，使新 checkpoint 当轮生效，而不是延迟到下一轮。
5. **CompactionState 增加 opaque `ContextData`.** 公共 Agent 不理解 story branch、lore 或 product cursor；Denova ContextSource 可用有界、versioned HostData 恢复自己的 context boundary。
6. **控制 API 返回 durable receipt，恢复 API 保留 opaque ActionID。** 仅返回 `error` 无法支持 HTTP/Automation 在冷重启后精确判断一次控制命令是否已提交；最终控制 API 统一返回 `CommandReceipt`。恢复动作的展示类型、command ID 与实际恢复授权不是同一身份，因此 `RecoveryAction.ID` 必须经过 transport/UI 原样返回给 `Session.Recover`，不能由前端推导或替代。
7. **display Task ID 不进入 durable fingerprint。** 只把影响执行语义的 canonical input 纳入 fingerprint，避免展示身份破坏幂等与 prefix cache。
8. **图片和非文本内容进入 exact restore snapshot。** provider-neutral Message 的多模态内容不能只靠文本 transcript 重建。
9. **Session 是唯一 transcript/compaction authority。** Denova product journal 只保存用户内容和必要投影；Agent checkpoint 可携带 opaque host boundary，但不存在第二份可推进有效上下文的 root middleware。
10. **SSE 保持 host-owned。** `Session.Observe` 已提供 transport-neutral snapshot + replay/live stream；内置某一 SSE/HTTP 框架只会增加 core 耦合，不构成能力缺口。
11. **runtime protocol 必须私有。** 普通消费者和 Denova composition 只使用 facade；actor/reducer/fault harness 仍可作为内部深模块和故障注入测试面，但不能形成第二套公共生命周期或被宿主 import。
12. **区分 bounded model task 与 delegated Agent。** Version Summary、Tool Agent、Automation Trigger 这类无对话控制面的单次模型任务直接使用 Model Interface；一旦能力承诺 task observe/steer/respond/abort、interaction 或冷恢复，就必须创建 child Session/Run，不能退回 Runnable/in-memory map。
13. **自动 compaction gate 位于最终 ModelCall seam。** 初稿只说“使用真实 model request”仍不够精确；必须等 caller middleware 完成 message/options/model rewrite 后再冻结 exact model-request snapshot，否则 side fork 的 provider、tools、thinking 或 cache route 可能与 primary call 不同。
14. **raw CAS range 与摘要增量源分离。** Agent 用完整 raw transcript 校验 replacement hash、支持 remove/recovery；Summarizer 只接收 active checkpoint marker 与新 tail。该区分是 repeated compaction 能持续推进的必要条件。
15. **automatic compaction 失败不等于 Run 失败。** 自动摘要是可恢复 side fork；只要 unchanged primary request 未违反 hard limit，就继续模型调用。Agent 以最终请求结构 fingerprint 持久化失败计数并打开 fuse，manual 操作始终可以重试。
16. **一个 model step 只有一个 durable context mutation owner。** cleanup-only middleware 可提交 cleanup，Agent gate 提交 checkpoint；普通 cleanup 一旦被选择就跳过 compaction。hard overflow 的 cleanup 只能作为 checkpoint side fork 内的 transient projection，不能形成第二份 durability。
17. **Game 摘要源必须来自 canonical story turns。** 当前 user prompt 是 story、state、pending input 与指令的渲染结果，会重复包含历史。Denova Adapter 因此提供 canonical turn delta 与 `SourceTurnCount` cursor，Agent 继续拥有事务语义。这是产品 ContextSource 定制，不应进入公共 Game preset。
18. **Event identity 分为 provider pairing 与 lifecycle execution。** `ProviderCallID` 只保证 assistant tool call/result 配对；`CallID`/execution namespace 用于 UI、trace、subagent path 和多 cycle 去重。工具输入流、开始、进度、完成必须贯通同一对身份，不能到执行开始才创建另一张工具卡。
19. **行为 Adapter identity 必须显式且在构造期校验。** 闭包、进程执行器、workspace mutation、read/search 与自定义 Toolset 的语义不能只靠 schema 或 Go concrete type 推导；所有内置工厂和组合器返回 error，零 identity 不会被哈希成看似稳定的组合。
20. **应用自有 ToolSource 是开放词汇。** 公共 package 只保留通用 source，Denova 的 `denova.lore` 等产品来源由产品 package 定义；安全决策依赖完整 descriptor 的 mutation scope、recovery 与 permission，而不是把所有未来来源硬编码进公共 enum。
21. **Artifact 的发布与验证是一个组合能力。** Agent 只把经过所选 backend 验证的 immutable reference 当作完整恢复证据；工具自行构造的 URL/path/metadata 不能绕过 lossless materialization 与 cleanup 保护。
22. **自定义 Goal 复用 lifecycle envelope，而不是标准状态机。** Agent 固定 ID、开放 status、revision、mutation fence 与持久化；Manager 通过开放 mutation kind 和 opaque JSON Data 拥有自己的状态机、模型 context、tool 和 continuation。
23. **动态 Context 只有一个 lifecycle seam。** `ContextSource` 是 Definition 能力；纯 assembler 通过正式导出桥生成相同 `ContextFragment`，不再存在另一套 projector identity、恢复或顺序协议。
24. **持久化加速不能成为第二权威。** 内置 file Store 的 checkpoint/index/receipt 只绑定并验证 canonical Log anchor；丢失加速最多增加一次回放成本，不能改变 Session 内容、幂等结果或恢复判断。

## 14. 最终适用性结论

该目标设计能够覆盖 Denova 当前综合 Agent 能力，也能作为其他 Coding Agent 的组合基础：简单场景只需要 Definition；复杂场景可以独立替换 Store、Toolset、ResultProcessor、ArtifactStorage、Context、Goal、Cleanup、Compaction、Permission、Interaction、Canonical、Middleware 与 Trace。固定 lifecycle 保证替换能力时不破坏 durability、permission fence、canonical commit 和 recovery 顺序。

最终刻意保留的边界只有两类：浏览器、搜索、抓取依赖宿主注入实际 Provider/Controller；Denova 特有 lore/book/story/interactive/automation policy 继续由 Denova 定义。二者都是预期的依赖反转点。实现已达到这条结构边界；全量测试、构建与 Writing/Game 实际产品链路已按迁移审计清单完成最终验收。
