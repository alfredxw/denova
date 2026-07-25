# Denova Agent Development Kit

`agent` 是一个可独立复用的 Go module，提供 provider-neutral 的原生 Agent loop、消息与事件协议、工具注册、上下文装配、会话抽象和可选的 durable runtime。它不依赖 Denova 的应用代码，也不依赖某个模型 SDK。

## 依赖边界

```text
业务 Agent / application host
        │
        ├── provider adapter（例如 model/openai，可替换）
        │
        └── agent
             ├── context   有来源、有用途、有硬上限的上下文装配
             ├── session   append-only 模型 transcript 与 CAS store
             ├── tools     工具 Definition、Descriptor、Registry 和基础 workspace 工具
             └── runtime   可选的 durable command / recovery / host-effect 协调
```

依赖方向只能指向 `agent`。模型 adapter、具体工具、system prompt、产品 session、UI event 和领域存储由使用方组合；核心包不会反向引用它们。

## 包职责

| Package | 职责 |
| --- | --- |
| `agent` | `Message`、`BaseChatModel`、`BaseTool`、native model/tool loop、middleware、stream、runner、Agent/Host registry |
| `agent/context` | 将带 `source`、`purpose` 和 byte limit 的片段增量装配为模型输入 |
| `agent/session` | append-only transcript、`/clear` marker、revision/CAS store；不保存 UI 日志或领域状态 |
| `agent/tools` | 完整的工具定义与 descriptor、去重 registry、基础 `read_file` / `grep` / `ls` / `glob` / `write_file` / `edit_file` / `execute` |
| `agent/runtime` | durable command journal、binding、恢复、input materialization、domain commit barrier 和 host-effect reconciliation |

`agent/tools.OpenWorkspace` 默认从 PATH 查找 ripgrep。需要可重复分发的宿主可使用 `OpenWorkspaceWithOptions` 注入固定的 `RipgrepExecutable`；Denova Release 使用这一 seam 指向安装目录中的内置版本，公共 `agent` module 不感知产品发行布局。

`Agent` 和 `Runner` 本身不隐式启用 `runtime`。普通嵌入只需要核心 loop；需要崩溃恢复、幂等命令或跨领域提交时，再由宿主在应用 Agent 边界接入 `runtime`。

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

registry, err := agenttools.Build(ctx, agenttools.Definition{
    Tool: lookup,
    Descriptor: agenttools.Descriptor{
        Source:           agenttools.SourceRead,
        Execution:        agenttools.ExecutionParallelRead,
        Recovery:         agenttools.RecoveryReadOnly,
        ResultProjection: agenttools.ResultBoundedModelContext,
        MaxResultBytes:   128 * 1024,
    },
})
if err != nil {
    return err
}

native, err := agent.NewAgent(ctx, agent.AgentConfig{
    Name:        "assistant",
    Instruction: "Use tools when they materially help.",
    Model:       model, // 任意实现 agent.BaseChatModel 的 adapter
    Tools:       registry.Tools(),
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
    // 将 event 投影到你的 API、日志或 UI。
}
```

推荐先用 `agenttools.Definition` 明确声明每个工具的执行方式、恢复语义和结果上限，再把 `Registry.Tools()` 交给 Agent。`agent.NewAgent` 会拒绝重名工具，业务层仍应通过 middleware 做权限、并发、审计和结果投影策略。

## Session 与上下文

- `session.Store` 是持久化 seam；内置 `MemoryStore` 适合测试，生产环境实现自己的 CAS store。
- `Snapshot.EffectiveMessages()` 通过 clear marker 过滤旧历史，不物理删除 transcript。
- `context.Assembler` 只接收显式片段；每个片段必须说明来源、用途、placement 和上限。
- thinking、工具卡片、调试日志和 UI 恢复数据不应默认写入模型 transcript。

## 外部 Agent Host

Codex、Claude Code 或其他完整 Agent 应实现 `agent.Host`，由 root Agent 通过 `HostRegistry` 调度。`HostContext` 将有界的模型摘要与仅供恢复/诊断的 host details 分离，避免把外部 Agent 的内部状态直接注入主模型。

## Durable runtime 的接入原则

`agent/runtime` 只负责通用协调，不拥有产品数据：

- runtime journal 保存命令、状态转换和恢复证据；
- application/domain store 仍是用户内容的事实真源；
- 不确定的工具副作用必须通过 host-effect reconciliation 确认；
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
