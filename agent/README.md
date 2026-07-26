# Denova Agent Development Kit

`agent` 是可独立复用的 Go module，提供 provider-neutral 的原生 Agent loop、消息与事件协议、单一工具注册边界、上下文装配、会话抽象和可选 durable runtime。它不依赖 Denova 应用代码或任何模型 SDK。

## 依赖边界

```text
业务 Agent / application host
        │
        ├── provider adapter（例如 model/openai，可替换）
        │
        └── agent
             ├── context   有来源、有用途、有硬上限的上下文装配
             ├── session   append-only 模型 transcript 与 CAS store
             ├── tools     标准工具实现、workspace adapter 与 definition factory
             └── runtime   可选 durable command / recovery / host-effect 协调
```

模型 adapter、具体工具、system prompt、产品 session、UI event 和领域存储由使用方组合；公共 module 不反向引用它们。

## 包职责

| Package | 职责 |
| --- | --- |
| `agent` | `Message`、`BaseChatModel`、`Tool`、`ToolDefinition`、唯一 `Registry`、native loop、middleware、typed event、runner 与 Agent/Host registry |
| `agent/context` | 将带 `source`、`purpose` 和 byte limit 的片段增量装配为模型输入 |
| `agent/session` | append-only transcript、`/clear` marker、revision/CAS store；不保存 UI 日志或领域状态 |
| `agent/tools` | 基础 `read` / `glob` / `grep` / `write` / `edit` / `bash` / `pwsh`、可扩展读取 adapter 和 definition factory；不维护第二份 registry |
| `agent/runtime` | durable command journal、binding、恢复、input materialization、domain commit barrier 和 host-effect reconciliation |

`agent/tools.OpenWorkspace` 默认从 PATH 查找 ripgrep。需要可重复分发的宿主可用 `OpenWorkspaceWithOptions` 注入固定 `RipgrepExecutable`；Denova Release 用该 seam 指向安装目录中的内置版本。

`Agent` 和 `Runner` 不会隐式启用 `runtime`。普通嵌入只需要核心 loop；需要崩溃恢复、幂等命令或跨领域提交时，由宿主在应用 Agent 边界接入 `runtime`。

## 工具契约

工具只有一个执行接口和一个注册单元：

```go
type Tool interface {
    Info(context.Context) (*ToolInfo, error)
    Run(context.Context, string, ...ToolOption) (ToolResult, error)
}

type ToolDefinition struct {
    Tool       Tool
    Descriptor ToolDescriptor
}
```

每次 run 在所有 `BeforeAgent` middleware 完成后创建不可变 registry snapshot，并一次性校验名称、重复项、JSON Schema、descriptor 枚举和跨字段约束。provider 只接收 snapshot 中的 `ToolInfo`；调度、middleware 和 lifecycle event 使用同一 snapshot。

`ToolResult` 将三个用途明确隔离：

- `ModelContent` 是唯一进入下一轮模型上下文的内容；
- `DisplayContent` 只用于 UI 工具卡片、日志和展示恢复；
- `Details` 是有界合法 JSON，只供 lifecycle receipt、host effect 和领域处理，不会默认写入 transcript。

模型与展示内容分别按 descriptor 的 `MaxResultBytes` 做 UTF-8 安全截断。tool message 只持久化 `ModelContent`、状态、synthetic reason 和截断摘要；旧历史只有 `Content` 时仍按成功文本结果读取。流式工具通过 `agent.EmitToolProgress(ctx, delta)` 发进度，最后仍返回普通 `ToolResult`。

## 调度与 steering

native loop 按 descriptor 在每个模型工具批次中建立阶段：

- 连续 `parallel_read` 有界并发，默认并发数为 8，可通过 `AgentConfig.ToolParallelism` 配置为 1–64；
- `workspace_exclusive` 与 `child` 各自形成严格前后屏障；
- typed `finished` event 按真实完成顺序发出，tool transcript 始终按模型调用顺序追加；
- unknown、无效调用、无效参数、policy block、模型长度截断和 steering skip 都生成唯一 paired synthetic result。

安全 steering 不消费宿主 runtime 的用户队列。`finish_current` 调用会完成并留下 receipt，`interruptible_wait` 可通过 per-call context 中断；尚未启动的调用会收到 `steering_before_start`。立即 Abort 仍会停止调度且不等待不协作工具，durable runtime 将不确定副作用恢复为不可自动重试的 `effect_unknown`。

## 最小组合

```go
ctx := context.Background()

lookup, err := agent.InferTool(
    "lookup",
    "Retrieve one value by key.",
    func(ctx context.Context, input struct {
        Key string `json:"key"`
    }) (map[string]string, error) {
        return map[string]string{"value": input.Key}, nil
    },
)
if err != nil {
    return err
}

definition := agent.ToolDefinition{
    Tool: lookup,
    Descriptor: agent.ToolDescriptor{
        Source:           agent.ToolSourceRead,
        Execution:        agent.ToolExecutionParallelRead,
        Recovery:         agent.ToolRecoveryReadOnly,
        ResultProjection: agent.ToolResultBoundedModelContext,
        Steering:         agent.SteeringFinishCurrent,
        MaxResultBytes:   128 * 1024,
    },
}

native, err := agent.NewAgent(ctx, agent.AgentConfig{
    Name:            "assistant",
    Instruction:     "Use tools when they materially help.",
    Model:           model, // 任意实现 agent.BaseChatModel 的 adapter
    Tools:           []agent.ToolDefinition{definition},
    ToolParallelism: 8,
})
if err != nil {
    return err
}

events := agent.NewRunner(agent.RunnerConfig{
    Agent:           native,
    EnableStreaming: true,
}).Query(ctx, "Find the answer")

for {
    event, ok := events.Next()
    if !ok {
        break
    }
    if event.Err != nil {
        return event.Err
    }
    // 将 typed event 投影到 API、日志或 UI。
}
```

## Session 与上下文

- `session.Store` 是持久化 seam；内置 `MemoryStore` 适合测试，生产环境实现自己的 CAS store。
- `Snapshot.EffectiveMessages()` 通过 clear marker 过滤旧历史，不物理删除 transcript。
- `context.Assembler` 只接收显式片段；每个片段必须说明来源、用途、placement 和上限。
- thinking、工具卡片、`DisplayContent`、`Details` 和调试日志不应默认写入模型 transcript。

## 外部 Agent Host

Codex、Claude Code 或其他完整 Agent 应实现 `agent.Host`，由 root Agent 通过 `HostRegistry` 调度。`HostContext` 将有界模型摘要与仅供恢复/诊断的 host details 分离，避免把外部 Agent 的内部状态直接注入主模型。

## Durable runtime 的接入原则

`agent/runtime` 只负责通用协调，不拥有产品数据：

- runtime journal 保存命令、状态转换和有界恢复证据，不持久化完整展示结果或 `Details`；
- application/domain store 仍是用户内容的事实真源；
- 不确定的工具副作用必须通过 host-effect reconciliation 确认，不能自动重试；
- domain commit 必须跨越显式 prepare/commit/ack barrier；
- API 和业务 service 应依赖宿主定义的 DTO/接口，由应用 Agent 层完成 runtime 类型转换。

## 验证

```bash
cd agent
go test ./...
```

provider adapter 是独立 module，需要分别验证，例如：

```bash
cd agent/model/openai
go test ./...
```
