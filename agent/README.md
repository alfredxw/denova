# Denova Agent

[English](README.en.md) | 简体中文

一个可嵌入 Go 程序的 Agent 运行库。下面从一次调用开始，逐步加入连续会话和工具能力。

## 1. Quickstart：完成一次任务

需要 Go 1.26.6 或更新版本。在自己的 Go module 中安装：

```sh
go get github.com/alfredxw/denova/agent
```

保存为 `main.go`，设置 `OPENAI_API_KEY`，运行 `go run .`：

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

`agent.New` 创建助手，`Run` 提交任务，`Events` 读取过程，`Wait` 获取执行结果。这里使用一次性会话，结束后不保留历史。

后两个示例沿用这里的 `ctx`、`model` 和错误处理方式，替换从创建助手开始的代码。

## 2. Session：连续对话、交互和会话管理

用户先让助手写一段文字，再要求缩短。把两次任务放在同一个 Session，第二轮就能接着第一轮的内容工作。

额外导入 `github.com/alfredxw/denova/agent/tools`，以及别名为 `sessionfile` 的 `github.com/alfredxw/denova/agent/session/file`：

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

`showQuestion` 代表你自己的界面显示函数。它展示问题后返回，用户提交答案时由另一个 UI 回调执行：

```go
// Called by the UI after the user submits the form.
// response contains the user's answers, permission choice, or cancellation.
_, _, err := conversation.Respond(ctx, request.ID, response)
if err != nil {
	log.Print(err)
}
```

`response` 是 `agent.InteractionResponse`：普通问题填写 `Answers`（使用请求中的 QuestionID 和选项 Value，或填写 Text）；权限问题填写 `Permission`；取消填写 `Cancelled: true`。问题没回答时，Run 会继续等待，不能只调用 `Wait`。

同一 Session 一次执行一个 Run；上一轮结束后再开始下一轮。文件 Store 会保留会话，程序重启后用相同目录和 `NamedSession("draft-42")` 再次打开即可。默认不配置 Store 时只保存在内存中。

**执行过程中，如何回应用户操作？**

下面是独立的 UI 操作入口，使用当前 `run` 或 `conversation`：

| 用户操作 | 调用 |
| --- | --- |
| “换个方向写” | `run.Steer(ctx, agent.Text("Focus on beginners."))` |
| “再补充一个要求” | `conversation.Queue(ctx, agent.Text("Keep it under 200 words."))` |
| “完成后再翻译一遍” | `conversation.FollowUp(ctx, agent.Text("Then translate it into Chinese."))` |
| “取消这条补充” | 对 Queue 返回的句柄调用 `queued.Cancel(ctx, agent.QueueControlRequest{})` |
| “现在就处理这条补充” | `queued.Interrupt(ctx, agent.QueueControlRequest{})` |
| “停止当前任务” | `run.Abort(ctx, agent.AbortRequest{Reason: "User cancelled."})` |

这些方法都有返回值和错误，需要处理。`Queue` 在空闲时只保存输入，`FollowUp` 在空闲时会启动新任务。跨网络重试时给 Input 或控制请求设置稳定的 `IdempotencyKey`；收到凭证表示已接收，不表示任务已完成。

**暂停和继续：**

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

暂停后原 Session 句柄关闭，需要时用 `assistant.Session(ctx, key)` 重新取得；重启后也先重新创建助手。打开 Session 不会自动继续任务。`SuspendTree` / `ResumeTree` 同时覆盖其子任务；取消整棵任务树用 `AbortTree`。

**页面重连和会话管理：**

| 需求 | 调用与含义 |
| --- | --- |
| 恢复页面 | `conversation.Snapshot(ctx)` 获取当前状态和未决问题，再用 `Observe(ctx, snapshot.Cursor)` 订阅后续变化；同时消费 Events 和 Errors |
| 找回运行句柄 | `conversation.AttachRun(ctx, runID)`；只连接，不自动继续 |
| 列出会话 | `assistant.ListSessions(ctx, agent.SessionSelector{All: true})` |
| 清空对话 | `conversation.Clear(ctx)`；保留会话身份，清除 Todo，保留 Goal |
| 关闭会话 | `conversation.Close(ctx)`；保留数据，但终结当前任务。要稍后继续，应先暂停 |
| 删除会话 | `conversation.Delete(ctx)`；永久删除，只在用户明确删除时调用 |

`Wait` 返回的 `error` 和 `Result.Status` 都要检查：`completed` 才表示完成，`suspended` 表示暂停，`aborted` 表示取消。页面断开只取消观测，不要把整个 Agent 的生命周期绑定到一次 HTTP 请求。

## 3. 完整组合：让助手检查一个真实项目

假设你在做一个项目助手：读取代码和文档，用 Todo 拆解检查项，加载审阅 Skill，委派独立审阅任务，执行检查，最后给出报告。中途提问、权限确认、暂停和继续，都沿用示例二。

主要变化集中在 `Definition`：把需要的能力组合进去即可。

下面是**接入示例**，使用应用已有的依赖：

- `projectRoot` 是项目目录，`model` 沿用示例一，`contextTokens` 是模型的实际上下文容量。
- `projectContext` 提供项目规则和状态；`skillCatalog` 列出可用 Skill，`skillLoader` 加载其内容。
- `commandRunner` 执行命令；`taskExecutor` 负责子任务；`artifactStorage` 保存完整工具输出。它们的接口与现成实现见代码后的链接。

额外导入 `agent` 模块下的 `compaction`、`goal`、`permission`、`toolresult` 包，以及示例二的 `tools` 和 `sessionfile`。

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
	Execution: agent.ExecutionPolicy{ToolParallelism: 4},
}, agent.WithSessionStore(store))
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

这段配置对应的工作过程是：

1. **获取证据**：Context 提供项目背景；Workspace 读取、搜索文件；Skills 加载审阅方法。
2. **推进任务**：Todo 记录步骤，Tasks 委派审阅，Shell 执行检查，Goal 保存完成目标。
3. **与用户协作**：Ask 补齐信息；Permission 决定哪些工具操作需要确认，回答仍走 `Session.Respond`。
4. **处理长任务**：Artifacts 保存完整结果，ResultProcessor 限制单项及整批工具输出，单一 Compaction checkpoint 合并已完成步骤；一个用户请求内可多次压缩，同时保留当前指令和最近原始结果。
5. **保留进度**：Session Store 保存会话和恢复记录，用户可以稍后继续。

**需要自己接入的部分，从这些接口开始：**

| 依赖 | 接口 / 可复用实现 |
| --- | --- |
| 项目上下文 | [`agent.ContextSource`](definition.go)：每段内容声明来源、用途和容量上限，稳定规则与变化状态分开注入 |
| Skills | [`tools.SkillLoader`](tools/skill_tool.go)：按名称加载完整内容，清单由应用提供 |
| 命令执行 | [`tools.CommandRunner`](tools/shell_tools.go)；本地可用 [`NewLocalCommandRunner`](tools/shell_local_runner.go) |
| 子任务 | [`tools.TaskExecutor`](tools/task_tool.go)；本地可用 [`NewLocalTasks`](tools/task_local_executor.go) |
| 大结果存储 | [`agent.ToolArtifactStorage`](tool_artifact.go) |
| 压缩定制（可选） | 默认使用当前模型；可替换 [`compaction.Summarizer`](compaction/standard.go)，或实现 [`agent.CompactionManager`](compaction.go) |

换成写作、研究或客服场景，主要替换 Instructions、Context、Skills 和 Tools；不需要的能力直接移除。需要写文件时，把 Workspace 改为 `WorkspaceReadWrite` 并提供 `MutationAdapter`。工作区只读不限制 Shell，命令权限仍需由执行器和权限策略控制。

简单场景使用上面的静态 Definition 即可。确实需要按会话动态选择模型和能力时，再实现 [`Source`](definition.go)；已有产品会话存储时，再接入 [`CanonicalAdapter`](canonical.go) 和 [`session.Store`](session/store.go)，让产品历史和 Agent 恢复记录共享同一份 journal。

### 压缩的三个接入级别

```go
// Default planning and summarization use the active Agent model snapshot.
Compaction: compaction.Standard(compaction.StandardConfig{ContextWindowTokens: 128_000})

// Customize summary generation while retaining the standard policy.
Compaction: compaction.Standard(compaction.StandardConfig{
    ContextWindowTokens: 128_000,
    Summarizer: compaction.SummarizerFunc{
        Capability: agent.CapabilityIdentity{Kind: "app.summary", Version: 1},
        Func: func(ctx context.Context, input compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
            return summarizeSelectedSource(ctx, input.Messages, input.Current)
        },
    },
})

// Customize triggering, selection and generation through the same runtime.
Compaction: myCompactionManager
```

`Standard` 默认保留最近的完整交互，使用当前模型快照生成摘要；容量不足时按顺序分批处理，模型失败不会偷偷切换执行方式。`Prompt` 可增加领域侧重点；`ModelSummarizer` 可指定替代模型，替代模型需声明稳定的 Identity。

自定义 Manager 的 `Plan` 接收完整交互组和最终模型快照，返回需要覆盖的前缀 `GroupCount`。`Compact` 只接收旧 checkpoint 与新选材料。两级扩展都返回 `CompactionCheckpoint{Summary, ContextData}`；ContextData 为可选的类型化、版本化 JSON（最多 8 MiB），随摘要原子保存，不自动注入模型。

Agent 统一保护当前用户要求、最近完整工具组及未完成步骤，检查最终请求容量和实际压缩进展，并负责 journal、revision、取消和恢复。应用通过 `Session.Snapshot().Compaction` 读取摘要视图，详细数据位于 `Inspect().CompactionMetrics` 和压缩事件。运行时返回的视图可调用 `Project` 检查有效历史；序列化后的展示数据不携带历史覆盖权限。
