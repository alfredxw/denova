# 工作区变更引擎：设计决策

## 结论

Denova 采用混合方案，而不是在“批量编辑”与“并行编辑”之间二选一：

- 单个文件内的多个改动点，由一次 `edit_file` batch 完成；所有 edit 基于同一快照校验，最终只提交一次文件写入。
- 不同文件且互不依赖的工具调用，可以在同一轮发出并聚合结果；经过证明的只读工具共享读锁，写工具、Shell 和未知工具取得工作区独占锁。当前写提交仍串行化，以保护统一 ledger 与 review 顺序。
- 正确性不依赖模型搬运 revision。Agent 写工具在调用开始时自行读取当前 revision，并由变更服务在提交前再次校验；编辑器保存等基于旧草稿提交的 API 仍显式使用 CAS。

这个边界把“语义原子性”放在最容易可靠保证的单文件范围内，同时保留跨文件并行带来的吞吐收益。

## 参考实现带来的判断

| 参考 | 可观察的设计 | 对 Denova 的启示 |
| --- | --- | --- |
| [Codex `parallel.rs`](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/parallel.rs) | 工具运行时用 `RwLock` 协调调用：允许并行的工具取得读锁，其余工具取得写锁。 | 并发应由显式能力分类控制，未分类工具默认独占，不能只根据模型是否一次发出多个调用来推断安全性。 |
| [Codex apply_patch 提示契约](https://github.com/openai/codex/blob/main/codex-rs/core/prompt_with_apply_patch_instructions.md) | 一次 patch 能表达多个文件操作和多个 hunk。 | 单次调用承载多个改动点能减少往返，并让调用意图保持完整。 |
| [Codex apply-patch 实现](https://github.com/openai/codex/blob/main/codex-rs/apply-patch/src/lib.rs) | 多文件 patch 会逐项落地；失败结果可能包含已经提交的 delta。 | “一个调用”不天然等于跨文件事务。Denova 不把多文件 batch 宣称为全有或全无。 |
| [Claude Agent SDK 的内置工具契约](https://platform.claude.com/docs/en/agent-sdk/python)、[Claude Code changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md) | 官方契约中的 Edit 使用 `old_string/new_string/replace_all`，Read、Edit、Write 是分离调用；changelog 还明确记录 read-before-edit、stale-edit 提示、并行 sibling 行为和有界 file-history/cache 的持续修复。 | 锚点适合表达模型的局部修改意图，revision 适合作为工具运行时内部的提交保护；并行执行、历史容量与写入协调仍是独立问题，不能由工具 schema 自动保证。 |

因此，纯多文件 Batch 会扩大冲突与部分失败的影响面；纯并行 Edit 会让同文件锚点竞争、结果依赖执行顺序；全局串行则会浪费只读和独立文件操作的并行性。混合方案分别在调用契约与执行调度层解决问题。

## 工具契约与单文件提交

`edit_file` 的 JSON 契约为：

```json
{
  "file_path": "chapters/01.md",
  "edits": [
    {
      "id": "optional-stable-id",
      "old_string": "原文",
      "new_string": "新文",
      "replace_all": false
    }
  ]
}
```

`read_file` 只向 Agent 返回有界文本及路径、offset、limit 等分页元数据，不返回 revision。`edit_file` 和 `write_file` 也不接受 `base_revision` 或 `"missing"`；工具适配层在每次调用开始时读取当前文件 revision，不存在的文件由 `write_file` 内部识别为创建。

一次 batch 按以下规则执行：

1. 工具读取当前文件及 revision，并把它作为本次调用的初始快照。
2. 每个 `old_string` 都在该初始快照上匹配，而不是依赖前一个 edit 的输出。
3. 未找到、非唯一且未设置 `replace_all`、重复 ID、空锚点、无实际变化或任意区间重叠，都会拒绝整个调用，并保证工作区零写入。
4. 全部验证通过后，从后向前合成新内容，通过临时文件、`fsync` 和 rename 原子替换目标文件；同一文件只发生一次可见写入。

`write_file` 复用同一套内部 revision、CAS 和原子提交协议。这样 batch 与全量重写在审阅、历史和故障恢复上具有一致语义，同时不要求 Agent 生成或转发并发控制字段。

## 并发调度

工作区使用按规范化绝对路径共享的读写门控：

| 场景 | 执行策略 |
| --- | --- |
| 同一文件的多个独立改动点 | 合并为一个 `edit_file.edits` batch |
| 不同文件、互不依赖的修改 | 在同一 assistant response 发出多个工具调用并聚合结果；调度可并发入队，实际可见写提交经过独占门控和 CAS 串行化 |
| 后续修改依赖前一调用结果 | 等待前一调用完成；后续写工具会自行取得新的当前 revision |
| 已证明只读的 Read、Lore、History、Web 工具 | 共享读锁并行执行 |
| Edit、Write、Shell 或未分类工具 | 工作区独占锁 |

`task` 是编排边界，本身不持有工作区锁，避免子 Agent 调用受门控工具时自锁；真正的子工具仍使用同一个工作区门控。流式工具的租约保持到工具结果真正结束，不能因为上层提前停止消费就放开写锁。

编辑器保存、文件树 Create/Delete/Rename/Copy/Move、版本恢复、手动版本快照和前台 Shell 进一步汇入同一个 workspace-change 租约。Shell 后台模式被禁用，因为请求返回后无法继续可靠持有租约；前台命令固定以当前工作区为 cwd。门控只协调 Denova 管理的入口，无法阻止操作系统进程或用户手工修改文件，所以 revision/CAS 仍作为工具与变更服务内部的第二道防线，而不是 Agent 协议的一部分。

## 持久化、提交与恢复

变更数据通常存放在工作区私有的 `.denova/changes` 中；已有旧工作区仍可沿用 `.nova/changes`：

- `ledger.jsonl` 是只追加事件日志，保存 prepared/applied、审阅决定、评论和历史状态等事件。
- `blobs/` 以 SHA-256 内容寻址保存 before/after 内容；ledger 只记录 blob 引用和结构化 hunk，避免重复内联整篇稿件。
- blob、ledger 和父目录都执行必要的 `fsync`；可见文件使用原子替换。
- ledger 与 blob 的 I/O 锚定到已打开的 workspace root，私有目录链、ledger 或 blob 出现符号链接时 fail closed；blob 读取时再校验内容哈希。

一次提交的顺序是：持久化 before/after blob → 追加 `prepared` → 再次校验工作区 head → 原子更新文件 → 追加 `applied`。第二次校验覆盖了写 blob 和 ledger 期间发生的外部修改。

启动时重放 ledger，并处理没有终态的单文件 `prepared`：

- 当前文件等于 after：补记为 recovered-applied；
- 当前文件仍等于 before：记为 aborted；
- 两者都不等：记为 conflicted，保留外部内容并暴露冲突，不擅自选择任一 blob。

namespace 已 rename/remove 成功、但父目录或终态 journal 尚未通过持久化屏障时，服务返回结构化 `durability_pending`，并明确标记 `workspace_mutated` 与可重试。后续任何写入先执行 reconcile；编辑器保存会保留幂等 intent，但重试前仍重新校验当前 revision，不因“重试”覆盖期间的外部改写。版本与自动化 hook 只在当前请求得到完整终态后执行；`workspace_mutated` 不能证明多路径操作已全部完成，也可能来自先前请求的 reconcile，因此不能被当作 hook 的因果凭证。若未来要求崩溃恢复后必达 hook，应增加绑定 operation ID 的 durable outbox。

JSONL 只自动截断没有换行提交边界的 torn tail；完整但损坏、未知类型或违反状态机的事件都会拒绝启动，不静默跳过。服务内存投影只保留 blob 引用、hunk 区间和状态元数据，详情、Reject 与 Undo/Redo 在需要时按 blob hydrate，避免历史正文常驻内存。

本地编辑器的连续自动保存使用相同的工作区写锁、CAS 与原子文件写入，但不为每次按键创建 blob/change group；编辑器自身负责本地输入历史，Agent 变更由持久化变更引擎负责审阅和恢复。

## Review、Comment 与 Undo/Redo

变更层级为 `ReviewThread → ChangeGroup（单次 Agent run / Undo 边界）→ ChangeSet（单文件）→ Edit → Hunk`。同一次 Agent run 可以把不同文件的 ChangeSet 放进同一个 group；用户把一批 inline comments 提交给 Agent 后，后续 run 会生成新的 group，但继续继承原 `review_thread_id`。因此历史恢复仍有清晰的单轮边界，而中央 Review 可以展示从最初 before 到最新 after 的累计 Diff。

Agent run 结束后，对话时间线会在对应 run 的最后一条消息后插入变更摘要卡，展示本轮文件数、增删行、文件列表、整轮 Undo 和 Review 入口。Review 只从这张摘要卡进入，是编辑器中间区域里的临时工作面，不占用右栏，也不会改变当前模式；右侧创作 Agent 可以继续与 Review 同屏。进入 Review 时会把项目侧栏在面板模型中真正折叠到 0，而不是只做视觉隐藏，避免隐藏面板继续参与百分比归一化并放大 Agent 最小宽度；Agent 面板通过 `react-resizable-panels` 的像素宽度保持策略沿用用户进入前的宽度，同时仍可实时拖拽，退出后继续使用调整后的尺寸。Review 工具栏保留 Agent 显隐按钮，因此关闭右栏后无需退出审阅即可重新打开。

工作面使用一个包含全部累计文件的纵向滚动页，而不是通过选择文件替换唯一 Diff；滚动页始终预留可见的纵向滚动槽，长 Diff 不依赖系统临时出现的覆盖式滚动条。每个文件有独立的可折叠头部，工具栏提供“折叠全部 / 展开全部”；桌面右侧文件导航只负责跳转和标记当前滚动位置，可手动收起，空间不足时收敛为 Diff 上方的组件库下拉跳转列表。跳转到已折叠文件会先展开再定位，且不会隐式切换历史范围。滚动位置的写权限只属于这次显式文件导航：首次进入、滚动同步当前文件、评论卡片测量、数据刷新、输入聚焦以及键盘 Home/End 都不执行应用级定位；用户开始滚轮、触摸、指针或键盘滚动会取消尚未提交的导航帧。工具栏默认选择“全部审阅变更”，也可通过组件库菜单切换到任一历史 Agent run；历史 run 使用该 group 自身的 ChangeSet before/after 构造只读投影。

Unified 使用 Monaco 单模型渲染客户端统一 Diff 投影，删除行、添加行和未修改行共用一列带颜色的行号；变更竖线位于行号左侧，配对的删除/新增行通过 `diffWordsWithSpace` 生成 UTF-16 Monaco 列范围并保留 word diff。Unified 与 Split 使用同一审阅主题，深色新增/删除行背景固定为 `rgb(31,49,36)` / `rgb(60,31,27)`，浅色主题使用独立的可读配色。鼠标经过任一可见源码行时，虚拟化 gutter 按钮会移动到该行并生成严格绑定 before/after revision 的 UTF-8 byte anchor。长段未修改内容仍可折叠，点击占位行后原地展开。Split 使用 Monaco Diff Editor 的显式双栏模式，不会按宽度静默降级。每个文件使用独立、稳定的 Monaco 模型身份，编辑器高度跟随实际换行内容和评论 view zone，由外层页面统一负责纵向滚动；邻近视口或被跳转到的实例按需创建，持有新评论、评论编辑草稿或已提交评论的展开实例会继续保留，避免滚动反复销毁并重建 comment view zone。外层滚动容器及其动态 Diff 子树显式关闭浏览器 scroll anchoring，评论 textarea 和 Monaco 隐藏输入都通过 `focus({preventScroll:true})` 激活。延迟创建的 Monaco 在 mount 时由外层宿主的真实宽高显式 `layout`，不依赖其 `5px` 初始测量回退；未聚焦编辑器在 mouse-down capture 阶段先按 `getTargetAtClientPoint` 设置光标，使后续 Monaco selection 在原坐标完成且不会让浏览器把首行滚入视口。comment view zone 按稳定 key、编辑器侧和行位置增量协调，并观察 portaled 评论卡片的真实高度；周期性 refetch 或等价投影更新不会把相同宿主从 DOM 中移除，卡片扩展也不会覆盖后续 Diff，因此 textarea 焦点和浏览器 selection 可以跨刷新保留。不同 anchor 的新评论草稿彼此独立，可以同时打开并继续操作已有评论；任一评论草稿存在时，React Query 仍可接收工作区事件并刷新缓存，但展示层冻结当前 thread/group 快照，直到全部草稿提交或取消后才采用新 revision，避免自动刷新打断输入。Review 的显式关闭入口始终可用；Agent 仍在追加或存在评论草稿时，会锁定快照变更、审阅结论和通过“打开文件”隐式退出 Review 的操作。布局选择只保存在浏览器本地。

`ReviewThread` 的每个文件只折叠 revision 连续的 ChangeSet。若编辑器、Shell 或外部进程在两轮之间改写文件，服务端返回 `continuity=conflicted`，保留最新连续段并报告被省略的迭代数，前端明确展示冲突而不伪造累计结果。累计段同时返回 before/after 两侧各自所属的 group 与 ChangeSet，确保旧版本侧 inline comment 也能通过严格 revision 校验。

状态机约束如下：

- Edit：`pending → accepted | rejected`，终态不可再次决策；group 的 `pending/accepted/rejected/mixed` 是聚合视图。
- ChangeSet：`prepared → applied | conflicted`，已撤销或完全拒绝后为 `reverted`。
- Accept 只记录审阅结论，不重复修改文件。
- Reject 为选中的 edit 规划逆向变更；同一路径的多个拒绝会聚合后写入一次。head 已变化时，只允许把 hunk 映射到 recorded-after 与 current 之间唯一、完全相等的 diff 区段；若原位置消失而相同文本只在其他位置出现，也会返回 revision/conflict，绝不按字面搜索猜测目标。
- Undo/Redo 以 group 为服务端历史单元，执行前验证所有涉及路径仍处于预期 head；普通新写入会使已有 redo 失效。

评论可以挂在 group、ChangeSet、Edit 或 Hunk 上，anchor 明确保存 `side`、`encoding=utf8-bytes-v1`、revision、UTF-8 字节区间、quote、prefix 和 suffix，支持新增、更新、resolve/unresolve 与删除。Monaco 中的中文、Emoji 与 CRLF 都先通过 UTF-8 索引转换；纯删除行锚定 before 侧。revision 或 quote 不再匹配时只标记过期，只有 quote 在当前侧唯一出现时 UI 才可重定位展示，不会把模糊位置提交回账本。

未解决评论会以引用 chip 聚合到创作 Agent 输入框上方。客户端发送的只有 `review_thread_id` 与 comment IDs；服务端从当前 canonical workspace 的 ledger 重新解析评论，拒绝伪造、跨 thread、已删除或已解决的 ID，并把带明确来源的结构化反馈限制在 256 KiB 内。请求失败时保留评论选择以便重试；请求成功后新 run 继承 thread，新的写入叠加到累计 Diff。Agent 正在向 thread 追加变更时，Review 的接受、驳回、Undo/Redo 与评论写操作都会禁用。

跨路径 Review、Undo、Redo 额外使用 group operation WAL。服务在修改第一个文件前持久化完整 `operation_prepared`（所有路径的 before/after blob 引用，以及最终 review/apply/history 投影），逐路径完成后追加进度，只有全部路径达到目标状态才通过 `operation_committed` 一次发布投影。崩溃恢复会把仍处于 before 的路径 roll-forward，识别已经处于 after 的路径；若发现第三种外部状态，则写入 `operation_conflicted` 并保留外部内容，不擅自覆盖。

Review API 返回当前操作作用域内的 `affected_paths`，而不是复用整个 group 的历史路径集合；自动化触发、SSE 失效通知和前端文件刷新都只消费这份提交回执，避免接受一个文件时误刷新或误触发同组其他文件。

编辑器接收到 Agent 的外部变更时会建立新的本地 undo 边界；若当前稿件有未保存输入，则保留草稿并进入冲突选择，而不是直接覆盖编辑器内容。

## Workspace identity 与事件一致性

revision 只能说明“某个文件版本”，不能说明“属于哪个工作区”。因此文件读取结果、可信的 Edit/Write 工具回执和 workspace-change SSE 事件都携带规范化 workspace identity；保存请求同时提交读取时捕获的 workspace 与 revision。

workspace identity 在 App runtime、工具调度门与变更服务三层统一解析为现有目录的真实路径。同一部作品即使通过符号链接别名打开，也只会获得一个 mutation service、一把工具门锁和一个事件身份。

前端以 workspace identity 和每文件 generation 过滤异步结果：切换工作区后，旧工作区的排队保存、迟到读取、旧 run 的工具回执或 SSE 事件都不得更新当前工作区。SSE 只作为失效通知，客户端收到后重新拉取权威状态，而不把事件到达顺序当作最终状态。

只有 `edit_file`/`write_file` 的结构化回执可以产生可信的变更事件；普通 Shell 输出中的相似 JSON 不能伪造工作区变更通知。回执只携带 group/change/path/revision 等固定结构，不枚举 batch 中每个 edit，因此不会因编辑项数量增长而在模型结果截断后破坏事件解析。

## 明确边界

- Shell 可以直接改文件。独占门控能避免它与其他受管工具同时执行，但当前无法完整 journal Shell 内部发生的每个写入；需要 review/history 的写作文件必须走 `edit_file` 或 `write_file`。未来若要覆盖 Shell，应通过文件系统代理或执行前后快照差分扩展，而不是解析命令输出猜测副作用。
- group 可以包含多个文件。Review、Undo、Redo 仍由多个单文件原子替换组成，因此外部进程可能短暂观察到逐路径推进，而不是同一 CPU 时刻全部切换；group WAL 提供的是可恢复的确定性 roll-forward 与最终投影原子性，不是假装文件系统支持跨路径 rename 事务。外部分叉会进入显式 conflict。
- 当前共享租约是进程内协调；另一个 Denova 进程或工作区之外的程序不受其约束。检测到 revision 变化时，默认策略始终是拒绝并重新读取，不做隐式 last-write-wins。若未来正式支持同一 workspace 多进程同时打开，需要在现有 CAS/WAL 之上增加 OS 级 advisory lock，而不是误把进程内 mutex 当作跨进程锁。
- 变更列表 API 当前支持 `status/path/run_id/session_id/review_thread_id` 过滤，但没有 `limit/cursor`；Agent 变更卡按当前会话查询，仍可能随该会话 ledger 增长拉取全部 summary。`ReviewThread` 累计投影也会 hydrate 所需 before/after 内容；前端只延迟创建离视口较远的 Monaco 组件，并不能减少 API 载荷。这是 beta 的明确容量边界，不应继续在客户端叠加全量消费者。进入大规模工作区前，服务端必须补稳定游标分页、pending count 和按文件加载的 Diff 内容接口。
- `workspacechange.ForWorkspace` 当前按 canonical path 维护进程生命周期的 service/root-FD 缓存；投影正文已改为按需 hydrate，但若未来支持在一个后端进程内轮换打开大量工作区，需要增加显式引用计数、LRU 与 Close，而不是无限扩张缓存。
- 编辑器保存、Reject、Undo 与 Redo 的自动化触发会捕获不可变 workspace runtime（workspace/config、BookState、BookService、SessionStore、ChatService）；同一工作区的快速 mutation 会串行合并检查，App 关闭时 cancel 并 drain evaluator。active run、trigger state、schedule LastRun、fingerprint 与 Inbox 去重都按 canonical workspace 隔离，异步运行不会因 active workspace 切换而漂移。
- 自动化 JSON Store 的 read-modify-write 使用进程内 path-keyed 锁，并通过 temp、`fsync`、rename 与 parent `fsync` 防止 torn 文件；它和 workspace 共享租约一样不提供多进程互斥，多后端实例共享同一用户目录前需要增加 OS 级锁或单写者服务。

本轮没有新增工作区配置：revision/CAS、原子落盘与 ledger 属于作品内容安全不变量；中央 Review 可按需打开，并不会阻止 Agent 完成写入。Unified/Split 是浏览器级展示偏好，默认 Unified，存储在 `nova:change-review-layout`。若未来需要不同团队策略，应增加明确的 `review required / auto accept` 枚举，而不是用含义模糊的布尔值绕过安全提交协议。

这些边界是有意保留的：先让常见的单文件写作修改具备确定、可审阅、可恢复的语义，再在确有需求时扩展跨文件事务或通用文件系统追踪。
