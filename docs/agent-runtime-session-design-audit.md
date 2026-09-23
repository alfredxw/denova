# 多运行时会话：分层决策与验收

更新：2026-09-21。本文保留实现边界与取舍，代码、测试和 AGENTS.md 是事实源。

## 分层

```text
产品入口：internal/app/conversation、internal/app/interactive
    │ 产品上下文准备、输入与输出提交、领域工具
    ▼
统一控制：internal/agents/runtime
    │ Session：控制、状态、Goal 入口；Engines：选择、连接、模型发现
    ├─ Native → internal/agents/execution → 独立 agent/ module
    └─ ExternalController → external → codex / claude 协议适配器
```

“应用层”指 Denova 的职责层级，不等于 `internal/app` 目录。多运行时聚合在 `internal/agents/runtime` 独立 package 及其子包中，不增加 Go module。`internal/app/agentruntime` 仅保留产品组装、展示任务恢复和叙述风格等既有辅助职责。

- `agent/` 是独立 Native runtime，不感知 Codex、Claude 或产品的运行时选择。
- `Session` 是绑定会话的共通控制入口，不新增状态或第二套队列。Native 与外部实现分别拥有执行生命周期；公共入口不要求它们使用同一种循环。
- `ExternalController` 负责两个外部引擎共用的产品指令调度和恢复，避免每个产品、每个 provider 各写一套。协议适配器只处理本运行时的原生协议。
- 写作、游戏保留不同的提交边界与工具。协议适配器通过 Host 调用它们，不直接改写产品内容。
- API 错误分类通过 runtime 边界归一化，不依赖具体 provider package。

## 能力归属与取舍

| 能力 | Native | Codex / Claude | 产品层职责 |
| --- | --- | --- | --- |
| 模型循环、自动压缩 | Native runtime | 各自原生运行时 | 展示已确认事件；不另开常驻摘要循环 |
| 跨回合上下文 | Native Session | 对齐后 resume 原生 thread/session | canonical 历史和缓存失效边界 |
| 手动压缩 | Native compaction | 原生 compact | 共用压缩卡片，不伪造摘要或 token 指标 |
| Plan / Todo | Native 工具 | Codex plan 事件、Claude Task 工具 | 确认快照与既有展示，不再注入第二套 Todo 工具 |
| Steer | Native Session 的追加机制 | Codex `turn/steer`；Claude 原生 interrupt 后 resume | 接收回执、待投递输入、产品历史 |
| 排队、暂停、恢复 | Native 生命周期 | 外部公共控制层与适配器 | 产品 journal、共用控制组件 |
| Goal | Native Goal manager 自行验收 | 外部专用只读 fork 评估 | 仅写作与通用对话支持；统一入口和展示 |
| 模型发现 | 现有模型配置 | Codex 模型列表；Claude initialize 返回的模型与 effort 列表 | 展示运行时返回值，不维护 Claude 模型目录 |

### Native 多 Agent 协作

Native 向模型提供三个工具，复用现有 Session、Run、输入回执与 canonical journal，不新增任务状态源或跨 runtime 执行器：

| 工具 | 职责与实现 |
| --- | --- |
| `send` | 批量发送 `delegate / message / followup / steer / interrupt / resume / abort`，逐项返回结果；[输入、输出及校验](../agent/tools/agent_send.go) |
| `await` | `timeout_ms = 0` 显式观察，可读取有界结果；正值只同步就绪状态，终态结果由 mailbox 投递；[契约](../agent/tools/task_wait_tool.go) |
| `list_agents` | 发现定义、恢复可访问子实例引用；读取不启动 Agent、不获取写租约、不改写 journal；[契约](../agent/tools/agent_list.go) |

`message` 不启动工作，`followup` 开始新 Run；`interrupt / resume` 暂停和继续同一个 Run。父 Agent 在子任务运行时继续独立工作，只在依赖点和最终收口等待。独立上下文和并行处理是收益来源，不引入自动组队、空闲池、共享任务 DAG、广播或固定角色。

协作说明仅在 Native 且启用委派能力时注入，提示词预览使用相同条件。外部 runtime 的工具、执行与恢复协议独立；[提示词边界](../internal/agents/prompts/runtime_contract.go)、[外部入口回归](../internal/agents/builder_external_test.go)覆盖写作、通用对话和游戏。模型实际接收的 schema 与行为以代码和[协作测试](../agent/tools/agent_coordination_test.go)为准，不维护第二份设计期 schema。

### Goal

游戏不支持 Goal：输入框入口、API 能力和运行时构建均关闭；旧 journal 内容保留，不触发游戏自动续跑。游戏仍支持 Todo、排队、转向和暂停。

Native 的 `internal/agents/lifecycle/goal.go` 委托独立 `agent/goal` 执行验收；外部的 `internal/agents/runtime/external_goal_evaluation.go` 只服务外部运行时。可以共用公开状态结构和 UI，但不共享验收执行器，也不把外部能力注入 Native Definition。

外部 Goal 暂不直接启用 provider 的自动循环。产品完成提交、待处理用户输入和暂停恢复仍需协调；当前可验证的公开协议不足以低成本替代这些边界。保留外部专用只读评估，其调用成本明确；不读取引擎私有 transcript 作为 Goal 完成依据。持久子 Agent 不在范围内。

Codex 的 `thread/fork` 不支持覆盖动态工具列表，会继承原线程的工具描述。评估适配器使用空宿主工具白名单拒绝执行，并强制只读沙箱阻止内置工具修改文件，不能仅凭提示词或 `dynamicTools: []` 声称隔离。真实 CLI 验收覆盖从完全访问的主线程派生评估后，内置文件修改被拒绝。

### Steer

Codex 向当前 turn 发送 `turn/steer`，带 `expectedTurnId`，检查响应归属。成功后才提交产品侧投递记录；已结束的 turn 不会吞掉追加内容，它会留待下一产品周期。产品记录“运行时已接受输入”，不声称模型已经完成处理。

Claude 的 stream 输入会排队，不能仅凭 stdin 写入成功宣称同一 turn 已转向。当前适配器复用原生 interrupt，收齐终结结果后 resume；不为形式一致新增另一套多 turn 进程调度器。这项差异封装在 Claude 适配器内，公共控制层不再强制两个引擎都中断。

写作把追加消息与 `guidance.delivered` 回执原子写入原 Session journal。游戏复用既有 draft model-context batch，保留已确认的叙事、骰点和草稿。晚到指令不会改写已经提交的游戏回合。

游戏接收原生转向后，由 provider 自行结束该 turn；不能因结构化提交已经完成就提前 interrupt，从而丢掉尚待下一个模型步骤处理的输入。

### 模型发现

Claude 模型选择使用原生 `control_request initialize` 响应中的 `models`：保留 ID、展示名称、effort 和顺序。发现不发送用户 prompt，不开启模型 turn；协议失败时明确返回错误，不退回手写模型表。用户显式配置的 API 模型继续使用其配置，不受 CLI 账号模型目录限制。

## 恢复与数据

每个逻辑会话只有一份 canonical journal：写作/通用对话使用原 Session JSONL，游戏使用原 Story JSONL。队列、暂停、确认输入、工具效果和状态恢复都以该 journal 为准。

provider thread/session ID、工作目录和私有压缩上下文属于宿主缓存。只有运行时、配置和历史边界匹配才可续用；切换、回退或缓存丢失时从产品历史重建。重建 checkpoint 只服务缓存失效恢复，不取代正常的原生压缩。

切换时检查并释放空闲 Native actor，返回 Native 后重新读取 canonical journal，避免它沿用外部执行前的 revision。游戏在产品提交边界中断 provider 后，仅在原生 turn 已确认终结且产品提交成功时保留续用缓存。

不新增用户配置。本次只调整未发布实现和目录组织，不增加最近 Release 以外的格式兼容层。已有新 journal 格式继续使用升级前备份；回滚旧版本应恢复相应备份，并另存升级后的新内容。

## open-design 参考

参考版本：`39de577a5d4cd78b79da568af02c08b4f65676f1`。对照仅放在本文，不进入模型提示词。

| 做法 | 采用与边界 |
| --- | --- |
| [Codex start/resume](https://github.com/nexu-io/open-design/blob/39de577a5d4cd78b79da568af02c08b4f65676f1/apps/daemon/src/agent-protocol/codex-app-server/session.ts#L1)；[Claude resume](https://github.com/nexu-io/open-design/blob/39de577a5d4cd78b79da568af02c08b4f65676f1/apps/daemon/src/runtimes/defs/claude.ts#L132) | 复用原生会话比常驻进程更重要；不要求额外进程池。 |
| [恢复与历史准备联动](https://github.com/nexu-io/open-design/blob/39de577a5d4cd78b79da568af02c08b4f65676f1/apps/daemon/src/agent-session-resume.ts#L115) | 仅在确认对齐时省略旧历史；Denova 使用稳定产品身份与 journal 边界。 |
| [原生计划归一化](https://github.com/nexu-io/open-design/blob/39de577a5d4cd78b79da568af02c08b4f65676f1/apps/daemon/src/agent-protocol/codex-app-server/normalize.ts#L590) | 复用计划展示，保留运行时原生工具。 |
| [停止后追加的交互](https://github.com/nexu-io/open-design/blob/39de577a5d4cd78b79da568af02c08b4f65676f1/apps/web/src/components/ProjectView.tsx#L10580) | 作为 Claude 协议取舍参考；Codex 已改用原生 steer。浏览器 localStorage 队列不满足 Denova 的持久恢复要求。 |

协议参考：[Codex App Server](https://developers.openai.com/codex/app-server)、[Claude Agent SDK](https://platform.claude.com/docs/en/agent-sdk/overview)。已安装 CLI 的协议实测优先于对未来版本的假设。

## 本轮验收

环境：Windows 原生 amd64、PowerShell 7.6.5、Go 1.26.6、Node 24.15.0、Playwright Chromium。使用完整 Windows 产物，包含主程序、updater、前端及内嵌资源、Skills、ripgrep 15.2.0。

### 真实 Codex 产品链路

Codex 0.154.0 使用隔离 `CODEX_HOME` 和本地 Responses 模型服务。替代的是模型响应；CLI、应用 API、领域工具、控制、产品提交及 journal 恢复均执行真实代码，没有读取用户登录凭据或调用付费远程模型。

| 产品 | 已通过的核心行为 |
| --- | --- |
| 写作 | 文件编辑、审阅、撤销重做、刷新续接、暂停后刷新与恢复、手动和原生自动压缩、原生计划恢复、Native → Codex → Native 历史连续性 |
| 游戏 | 完整回合、行动选项、刷新、分支、分支规划开关、重新生成失败保留旧回合并重试、排队与原生转向、暂停恢复、压缩与计划恢复、双向运行时切换、工具卡片完成状态 |
| 通用会话 | 独立 Project 文件工具、并发会话隔离、Follow Up 定向投递、刷新后队列恰好执行一次 |
| Goal | 写作只读 fork 验收后自动续跑并完成；评估输入不污染主线程；游戏 API 明确拒绝 Goal |

共 25 个浏览器 E2E 场景完成验收。自动压缩测试让真实 CLI 根据 usage 触发压缩，并核对 live tool 结果进入压缩输入、后续请求含压缩摘要、刷新后继续使用同一 `prompt_cache_key`。单独的真实 CLI 集成测试覆盖 Ask、宿主工具、内置修改拒绝、取消、图片、计划、转向，以及 CLI/API 两种配置下的写作和通用会话。

验收发现并修复：Native actor 缓存过期、写作变更组与通知不对齐、游戏提交后丢失原生会话缓存、游戏恢复缺少 command identity、失败重试误入队、错误 key 未本地化、Goal fork 权限未收紧、转向输入被提前中断、已完成工具刷新后仍显示执行中。复用既有会话关闭、工具结果投影、恢复及卡片更新接口，未扩展 `agent/`。

测试记录：Goal 的首次断言误将“只读”理解为 fork 返回空工具目录，另一处断言误期待未翻译的错误 key；根据真实协议和现有 API 修正。小屏暂停菜单出现过一次渲染时序失败，独立复验通过；未为此修改无关 UI 行为。

### 原链路及静态检查

- 根 module 生产源码树 `go test ./cmd/... ./config/... ./internal/...`、`go vet` 通过；`agent/` module 全量 test、vet 通过。根目录既存 `dist/readme-ci-validation` 混合 package 产物不属于生产测试树。
- 切换、转向、确认回执、游戏中断恢复和 provider 缓存接受边界测试通过；相关 runtime 与游戏路径 race 检查通过。
- 前端 129 文件 / 730 项测试、TypeScript / Vite 构建、4437 项中英文资源对齐检查通过。
- Native 核心 E2E 20 项与共享 UI 浏览器检查 16 项，共 36 项一次通过。覆盖中英文、深浅主题、390 / 1280 / 1440 宽度，已检查写作与游戏截图；共享 UI 使用展示 fixture，与上述真实 Codex E2E 分别计数。
- 无新增 Go module；`agent/` 无差异；runtime 生产包不依赖 `internal/app`。

复跑真实 Codex 验收时，设置 `DENOVA_TEST_CODEX_EXE` 为已安装的 `codex.exe` 绝对路径；`DENOVA_E2E_PACKAGE_DIR` 可指向当前构建产物，否则测试入口会构建隔离后端并启动独立前端。运行：

```powershell
pnpm --dir web exec playwright test tests/e2e/codex-runtime.spec.ts tests/e2e/writing-agent.spec.ts tests/e2e/game.spec.ts tests/e2e/agent-chat.spec.ts tests/e2e/task-pause.spec.ts tests/e2e/composer-pause.spec.ts tests/e2e/context-compaction.spec.ts --project=e2e --grep-invert 'preserves the live tool tail|keeps three interleaved'
```

`preserves the live tool tail` 检查 Native 专属压缩协议，Codex 使用独立的自动压缩场景；持久子 Agent 不在外部 runtime 范围。取消 `DENOVA_TEST_CODEX_EXE` 后运行既有核心测试即为 Native 回归。

验收边界：macOS / Linux 未实机验证，未做远程模型质量及长会话压测。Claude 2.1.259 的模型发现、工具循环及 interrupt 已在上一轮真实 CLI 集成验证，本轮未重复整组 Claude 产品 E2E。Claude 继续使用原生 interrupt/resume；provider 自动 Goal 循环与外部持久子 Agent 不在范围内。没有新增用户数据迁移或最近 Release 的格式不兼容点。

### 2026-09-20 实际游戏会话回归补充

`external-game-7d073845f8cb8b67896b64299fad188c` 暴露了此前固定模型输出未覆盖的情况：上一轮遗留的运行中卡片抑制新一轮的等待提示，且模型先提交模块、后输出正文。该轮约 154 秒，模块提交后约 33 秒才完成正文。

- 公共列表只用当前回复的活动消息决定是否隐藏 Shimmer。游戏历史中分开记录的工具结果合并到对应卡片展示，原 journal 不改写。
- 外部游戏执行在工具分派前检查正文前置条件。过早提交返回模型纠正反馈，不接受模块、不生成已执行卡片；仍由当前 runtime 回合继续，正确正文后的提交和部分模块修复保留既有链路。Native middleware 与 `agent/` 不变。
- Windows 上读取了实际 Denova Codex 子进程的代理环境，并通过相同适配器向真实模型发送最小请求，观测到子进程连接 `127.0.0.1:7890`。代理发现与继承没有丢失。该会话 Codex effort 留空，真实 thread/start 回执为 `max`，来源是 CLI 配置；Native 的 `minimal` 不作用于 Codex。适配器增加实际生效强度和会话复用日志，不静默修改用户模型设置。
- 新增回归覆盖首字前等待、过早提交后等待、真实 CLI 在同一回合纠正、正文先于提交卡片、刷新恢复及旧工具结果展示。深色 1440、浅色 390 页面验收；Codex 与 Native 各 11 项既有写作/游戏核心流程回归通过。此次真实远程请求仅证明连通性，不代表完整游戏性能基准。
