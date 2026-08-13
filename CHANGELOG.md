# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- Project 级模型上下文现在按固定顺序将根目录 `AGENTS.md` 与 `CREATOR.md` 作为两个独立、带来源和 revision 的 User-role stable leading fragment 注入全部项目 Agent；缺失文件独立跳过，单文件变更只从对应边界旋转缓存，正文不再进入内建 system definition。内建模型可见文案同时移除对其他 Agent 产品、CLI 与品牌的提及，并增加跨 Agent contract test；提示词与上下文架构对标审计已补充可变状态增量、缓存分叉观测、静态 Prompt 资产和压缩恢复建议。Beta 行为变化：已有 workspace 根目录 `AGENTS.md` 将开始对 Denova Agent 生效。无需新增配置。
- Project-level model context now injects root `AGENTS.md` and `CREATOR.md` in a fixed order as separate, attributed, revisioned User-role stable-leading fragments for every project Agent. Missing files are skipped independently, changing one file rotates the cache only from its own boundary, and neither body is part of the built-in system definition. Built-in model-visible copy also no longer mentions other Agent products, CLIs, or brands, with a cross-Agent contract test enforcing the rule; the prompt/context architecture audit now covers mutable-state deltas, cache-divergence attribution, static prompt assets, and compaction rehydration. Beta behavior change: an existing root `AGENTS.md` now applies to Denova Agents. No new setting is required.
- 上下文压缩完成 side-fork 收口：删除 `context_compaction` 的独立 Agent kind、Agents 页面入口，以及 model/tool/Skill/prompt/context 五套专用配置与无调用的 `GenerateSummary` 模型适配器；正常路径继续从主 Agent 最终请求 append 一条内部 checkpoint 指令并复用原模型、system、tools、options 与缓存前缀，cold layered fallback 也改为继承源 Agent 的模型窗口、provider 输入上限和模型实例。`context_compaction` 仅保留为 event/trace/展示投影类型，写作与游戏共用同一实现。Beta 不兼容：既有 `context_compaction` Agent 配置不再读取；无需新增配置。
- Context compaction is now fully consolidated as a side fork. The independent `context_compaction` Agent kind, Agents-page entry, model/tool/Skill/prompt/context overrides, and unused `GenerateSummary` model adapter were removed. The normal path still appends one internal checkpoint instruction to the primary Agent's final request and reuses its model, system, tools, options, and cacheable prefix; the cold layered fallback now also inherits the source Agent's model window, provider-input limit, and model instance. `context_compaction` remains only as an event/trace/display projection type, shared by Writing and Game. Beta breaking: existing `context_compaction` Agent settings are no longer read. No new setting is required.
- Agent package 已重构为唯一的 `Agent → Session → Run` 公共生命周期与基于 Interface 的能力组合：model、Toolset、Tool Result Processor、Artifact Backend、Context、Goal、Cleanup、Compaction、Permission、Interaction、Canonical Adapter、Session Store 与 Trace 均可独立选择或自定义；read/glob/grep/write/edit、bash/pwsh、ask、todo、skill、task 作为内置通用工具提供，web search/fetch/browser 作为可选插件提供。Denova 只保留 lore、book/story、Interactive/Director 及产品提交/展示 Adapter；旧 Runner/Runnable/AgentInput/AgentEvent、公开 runtime engine 和产品侧重复 Goal/Ask/Todo/cleanup/compaction authority 已直接删除。Beta 不兼容：所有自定义 Toolset 与行为 Adapter 必须提供稳定 identity，构造与组合 API 现在显式返回错误；本开发周期内由旧产品 Goal/Ask/Todo/cleanup/compaction authority 写入的中间 journal 记录不提供迁移层。无需新增配置。
- The Agent package now has one public `Agent → Session → Run` lifecycle with interface-based composition. Models, Toolsets, Tool Result Processors, Artifact Backends, Context, Goal, Cleanup, Compaction, Permission, Interaction, Canonical Adapters, Session Stores, and Trace sinks can be selected or replaced independently. Built-in common tools cover read/glob/grep/write/edit, bash/pwsh, ask, todo, skills, and tasks, while web search/fetch/browser remain optional plugins. Denova owns only lore, book/story, Interactive/Director, and product commit/display adapters; legacy Runner/Runnable/AgentInput/AgentEvent, the public runtime engine, and duplicate product Goal/Ask/Todo/cleanup/compaction authorities were removed. Beta breaking: custom Toolsets and behavior adapters now require stable identities, construction/composition APIs return explicit errors, and intermediate journal records written by the obsolete product Goal/Ask/Todo/cleanup/compaction authorities in this development cycle are not migrated. No new setting is required.
- Agent 迁移完成了逐项能力对齐与恢复审计：工具原始结果在事件和 transcript 前统一完成 lossless artifact 化、校验和 bounded projection；cleanup/compaction 在每个最终模型调用 seam 上共享单一仲裁、精确 middleware-final snapshot、prefix-cache identity、原子 settlement 与 crash recovery；自定义 Goal 可持久化开放状态与 opaque 数据。Session file Store 以单一 checksummed canonical Log 为权威，并用可重建 checkpoint、anchored command receipt 和 coverage index 保持有界冷恢复；Writing/Game/AgentChat/Director 的上下文分析、Game branch 同步、子 Agent 冷恢复、Ask/Permission、Diff view 与实时工具参数均走同一公共生命周期。无需新增配置。
- Agent migration received a capability-by-capability parity and recovery audit. Raw tool output is now losslessly materialized, verified, and bounded before events or transcript persistence; cleanup and compaction share one final-model-call arbitration point with the exact middleware-final snapshot, prefix-cache identity, atomic settlement, and crash recovery; custom Goals can persist open statuses and opaque state. The file Session Store keeps one checksummed canonical Log and adds rebuildable checkpoints, anchored command receipts, and a coverage index for bounded cold recovery. Writing, Game, AgentChat, and Director context inspection, Game branch synchronization, child-Agent cold recovery, Ask/Permission, Diff views, and live tool arguments all use the same public lifecycle. No new setting is required.
- Agents 中的「持续进化」已重构并更名为「Harness 优化」：默认展示过去 24 小时的 Session 与 Run trace，可查看脱敏详情、按时间范围筛选并逐条选择本次分析依据；定时学习改为紧凑的「定时优化」用户配置；State 为空时明确展示当前生效的系统内置 State；Git 历史复用统一版本时间线交互，并保留 Diff、未保存保护与基本回滚。所选 trajectory 会作为受限资源列表传给 Harness Optimizer，空选择不会自动扩大分析范围。现有 trajectory 条数与定时间隔配置继续使用，无新增配置；写作与游戏模式共享该用户级能力。
- The Agents “Continual Learning” surface is now “Harness optimization.” It defaults to Sessions and Run traces from the past 24 hours, with redacted detail inspection, time-range filtering, and per-item evidence selection. Scheduled learning is now a compact Scheduled optimization control; an empty user State clearly shows the active system built-in State; and Git history reuses the shared version timeline while retaining diffs, unsaved-edit protection, and basic rollback. Selected trajectories are passed to Harness Optimizer as a bounded resource list, and an empty selection never silently broadens the analysis. Existing trajectory-cap and schedule-interval settings remain in use with no new configuration, shared by Writing and Game modes.
- 持续进化实现完成一轮故障恢复与边界审计：State 发布将已提交结果与后置清理告警严格分离，回滚失败保留恢复标记，草稿可在冷恢复后修复暂时非法的中间状态；Harness Optimizer 在结束前会把全部校验诊断反馈给同一 Run 继续修复。应用层版本拒绝裸 Git hash 与不安全 `.git`，trajectory 递归脱敏机器路径并排除模型 thinking，服务可从临时初始化失败恢复，页面也不会用 Restore 覆盖未保存编辑。无需新增配置。
- Continual Learning received a recovery and boundary audit. State publication now separates committed results from post-commit cleanup warnings, failed rollbacks retain their recovery marker, and cold-restored drafts can repair temporarily invalid intermediate files. Before completion, Harness Optimizer returns all validation diagnostics to the same Run for repair. Application history rejects raw Git hashes and unsafe `.git` entries; trajectory recursively redacts machine paths and excludes model thinking; service initialization can recover from transient failures; and Restore no longer overwrites unsaved editor content. No new setting is required.
- Denova 应用层的唯一运行身份统一恢复为 `run-` 前缀，移除本开发周期内未发布的 `operation-` ID 生成路径，不增加别名或兼容分支；Agent package 仅提供应用可注入的 `RunIDGenerator` interface，并把身份作为 opaque value 校验、持久化和重放，不再定义产品前缀、Trace 文件名或存储位置。写作、游戏及其他 Agent 的对话事件、控制命令与 Agent 追踪共享同一个 Denova Run ID，追踪按该 ID 记录模型调用、缓存用量、工具执行与最终状态。会话历史同时修复“进展 + 最终回复”场景下规范回复被再次展示在折叠执行过程中的问题。无需新增配置。
- Denova's application-owned execution identity now uses the `run-` prefix again, removing the unreleased `operation-` ID generation path from this development cycle without aliases or compatibility branches. The Agent package now exposes only an injectable `RunIDGenerator` interface and treats the resulting identity as an opaque value to validate, persist, and replay; it no longer defines the product prefix, Trace filename, or storage location. Conversation events, controls, and Agent traces across Writing, Game, and other Agents share the exact same Denova Run ID, with traces recording model calls, cache usage, tool executions, and settlement. Session history also no longer repeats a canonical final reply inside the collapsed execution process when a run contains both progress and final display segments. No new setting is required.
- 通用 `edit` 工具新增可选的 `operation=replace|delete`：省略时仅在提供非空 `edits` 时按原有编辑执行，删除必须显式指定 `delete` 且不能携带 `edits`。删除仅允许工作区内、大小受限的 UTF-8 普通非符号链接文件，并通过统一持久化变更链路生成完整 Diff，支持审阅驳回、撤销与重做，同时精确恢复原文件权限；写作与其他启用通用工作区工具的 Agent 共用该能力，无需新增配置。
- The shared `edit` tool now accepts optional `operation=replace|delete`. Omitting it performs the existing replacement flow only when non-empty `edits` are supplied; deletion must be explicit and cannot include `edits`. Deletes are limited to bounded UTF-8 regular non-symlink files inside the workspace and use the durable change pipeline, producing a complete review diff with rejection, undo, redo, and exact permission restoration. Writing and other Agents with shared workspace tools use the same capability; no new setting is required.
- Agent 核心迁移为可组合、可持久恢复的 `Agent -> Session -> Run` 公共生命周期：Session Store、Tools、Context、Goal、Compaction、Permission、Interaction、Canonical Adapter 与 Middleware 均可按 Interface 选择内置实现或自由替换；read/glob/grep/write/edit、bash/pwsh、ask、todo、skill、task 作为通用工具集提供，web search、web fetch、browser 作为可选插件提供。Denova 的 Writing、Game、General、Config、Image、Director 与 Automation 主执行链统一使用公共 Agent Session/Run，同时继续由产品层定义 lore、book、story、interactive 以及现有 Goal/Ask/approval UI 契约。控制命令现在返回可跨重启重放的 durable receipt；公共 Agent Session 统一拥有 transcript、compaction、canonical commit、host effect 与 recovery，Denova 自定义交互仍作为 product capability 接入；本开发周期内旧 execution coordinator、旧 runtime journal 恢复和双重 compaction middleware 已删除，不提供兼容层。
- The Agent core now uses a composable, durably recoverable `Agent -> Session -> Run` public lifecycle. Session Store, Tools, Context, Goal, Compaction, Permission, Interaction, Canonical Adapter, and Middleware can each use built-ins or custom Interface implementations. Common read/glob/grep/write/edit, bash/pwsh, ask, todo, skill, and task toolsets are included, while web search, web fetch, and browser remain optional plugins. Denova Writing, Game, General, Config, Image, Director, and Automation root execution now share public Agent Sessions/Runs while lore, book, story, interactive, and the existing Goal/Ask/approval UI contracts remain product-owned capabilities. Control commands return restart-replayable durable receipts. Public Agent Session owns transcript, compaction, canonical commit, host effects, and recovery, while Denova custom interactions plug in as product capabilities. The obsolete execution coordinator, legacy runtime-journal recovery, and dual compaction middleware from this unreleased cycle were removed without compatibility layers.

### Fixed

- 修复 Agent 对话在短会话中经历思考、工具调用与正文尾行替换后只能短暂锁底、随后内容继续增长但视口停止跟随的问题。流式控制器现在从稳定滚动容器解析真实尾行，不再依赖虚拟列表一定重新触发复用行的 ref；响应跑道独占运行中的垂直定位，短列表底部对齐仅在结束后恢复。写作、游戏、工作台与子 Agent 会话共用该修复，无需新增配置。
- Fixed Agent conversations briefly following the bottom and then stopping after thinking, tool-call, and prose tail-row replacements in short sessions. The streaming controller now resolves the rendered tail from the stable scroller instead of assuming a virtualized reused row will re-fire its ref; the response runway exclusively owns active vertical placement, and short-list bottom alignment resumes after completion. Writing, Game, Workspace, and child-Agent conversations share the fix, with no new setting required.
- 修复写作 Agent 的工作区变更流式事件遗漏当前 Project 身份，导致新一轮运行中的 Diff review 查询缓存不会实时刷新、审阅卡片只能在页面重载后出现的问题。首轮、续轮与恢复运行现在统一携带 Project 路由身份，同时 Writing Agent Session 继续严格按 workspace/session 定位，不会因 Project 元数据产生会话分叉；无需新增配置。
- Fixed Writing Agent workspace-change stream events omitting the active Project identity, which prevented the current session's Diff review query cache from refreshing during a new run and left the review card visible only after a page reload. Initial, continued, and recovered turns now share one Project-aware runtime route, while Writing Agent Sessions remain strictly workspace/session-scoped and cannot fork on Project metadata. No new setting is required.
- 修复右下角红色错误提示在使用本地化通用文案时丢失服务端请求标识的问题；API、Agent 流式事件、文档审阅、自动保存与变更审阅等错误现在会在可用时统一保留「日志 ID / Log ID」，且不会重复追加。纯前端校验错误不会伪造 ID，无需新增配置。
- Fixed bottom-right error toasts losing the server request identifier when localized fallback copy replaced the original failure. API, Agent stream, document-review, autosave, and change-review errors now consistently retain the localized Log ID when available without duplicating it; client-only validation failures do not invent an ID. No new setting is required.
- 修复关闭 Agent 自动上下文压缩后，Tool Result Cleanup 因 disabled Compaction 边界管理器仍存在而误判压缩可用、在高上下文压力下反复安排无效压缩的问题；Cleanup 现在同时遵守产品配置与公共 capability 可用性，并在未发生投影时保持前后 body pressure 遥测一致。写作与游戏模式共用该修复，无需新增配置。
- Fixed Tool Result Cleanup treating the disabled Compaction boundary manager as an executable compactor after automatic context compaction was turned off, repeatedly scheduling no-op compaction under high context pressure. Cleanup now requires both product configuration and public capability availability, and preserves before/after body-pressure telemetry when no projection occurs. Writing and Game share the fix, with no new setting required.
- 修复写作 Agent 完成一轮后会话消息数不实时刷新、已删除文件在刷新后恢复为空白标签的问题；会话摘要现在随 Run 完成更新，文件恢复会识别 404、清理失效标签并保留可用回退页。无需新增配置。
- Fixed Writing Agent session counts not refreshing after a completed turn and deleted files returning as blank tabs after reload. Session summaries now refresh when a Run finishes, while workspace restoration recognizes missing files, removes stale tabs, and keeps an available fallback. No new setting is required.
- 修复 Agent 把 grep 输出条目数误当成去重文件数，以及在新一轮未实际调用工具时沿用历史结果并声称已检索的问题；搜索结果现显式标注计数单位，工具描述要求精确文件数使用 `-l`，本轮上下文边界禁止无本轮结果的工具声明。无需新增配置。
- Fixed Agents interpreting grep output-entry counts as distinct-file counts and claiming a new search from historical results without invoking a tool in the current turn. Search envelopes now expose their count unit, the tool contract requires `-l` for exact file counts, and the current-turn boundary forbids tool claims without current-turn results. No new setting is required.
- 修复写作 Agent 的 Diff review 摘要在运行结束、自动跟随滚动停止后才挂载，导致已生成的审阅卡片落在可视区下方、只有刷新后才明显可见的问题。首次工作区写入后会立即在对应 Run 的时间线预留轻量摘要；运行中不加载大体积 Diff，也不允许审阅或撤销，结束后在原位加载完整统计与审阅操作。无需新增配置。
- Fixed Writing Agent Diff review summaries mounting only after a run stopped automatic scroll following, which placed an already-generated review card below the visible viewport and made it reliably obvious only after a refresh. The timeline now reserves a lightweight summary beside the matching Run after its first workspace write; while the Run is active it neither loads the full Diff nor allows review or undo, then hydrates the complete statistics and actions in place after completion. No new setting is required.
- 修复写作、游戏与 AgentChat 会话在历史内容稍晚完成测量时脱离底部，以及短回复结束后继续保留流式响应跑道、在回复下方留下大块空白的问题。共享消息列表现在仅在用户未主动上翻时跟随非流式布局变化，生成结束会立即收起临时跑道；短会话使用虚拟列表原生贴底布局，最新回复稳定停在输入区上方。无需新增配置。
- Fixed Writing, Game, and AgentChat conversations drifting away from the bottom when restored content finished measuring late, and short responses retaining their streaming runway after completion and leaving a large blank area below. The shared message list now follows idle layout changes only while the user has not explicitly scrolled up, removes the temporary runway as soon as generation ends, and uses the virtualizer's native bottom alignment for short conversations so the latest reply remains above the composer. No new setting is required.
- 修复游戏会话末尾状态面板把展开态最高 Tab 的高度继续保留到预览态、导致下方出现大块空白的问题。会话尾部现在按折叠、预览、展开状态分别记录已访问 Tab 的最高高度：同一状态内切换 Tab 仍保持滚动位置稳定，不同状态之间则立即适应各自内容高度；无需新增配置。
- Fixed the Game conversation's trailing state panel retaining the tallest expanded tab height in preview mode and leaving a large blank area below. The conversation footer now tracks the tallest visited tab independently for collapsed, preview, and expanded layouts, preserving scroll stability within one layout while adapting immediately between layouts. No new setting is required.
- 修复写作模式因单个已淘汰或损坏的旧会话 journal 阻断整个会话列表，继而以空 `session_id` 发送新指令，并把明确的绑定或上下文变化错误误判为“正在恢复已接受运行”的问题。会话列表现在隔离不可读项且不改写原始 journal；发送前会解析精确会话绑定，只有响应流已经开始或 HTTP 接受结果不确定的请求才会检查持久运行，服务端明确拒绝的请求会立即恢复可操作状态。写作与 AgentChat 共用绑定和恢复边界；游戏会话列表共用损坏项隔离能力。无需新增配置。
- Fixed Writing mode allowing one obsolete or damaged Session journal to block the entire Session list, then sending a new instruction with an empty `session_id` and misclassifying a deterministic binding or context-change error as an accepted run being recovered. Session listing now isolates unreadable entries without rewriting their journals; sends resolve an exact Session binding first, and durable runtime inspection occurs only after the response stream starts or when HTTP acceptance is uncertain. A definite server rejection immediately returns the composer to an actionable state. Writing and AgentChat share the binding and recovery boundary, while Game Session listing shares unreadable-entry isolation. No new setting is required.
- 修复写作与游戏模式在 Agent 流式生成期间输入追问草稿后，中断按钮被发送按钮替换、导致无法主动停止当前运行的问题。共享输入组件现在始终保留有作用域的中断操作，并在存在可发送草稿时并列显示发送操作；写作流会直接采用已持久接受的 `agent_cycle_started` Run 身份放开中断，不再等待可能与 Task 注册竞态的额外状态查询，较早的空闲查询也不能覆盖该身份。中断已提交或尚未取得精确 Run 身份时仍保留禁用态反馈。无需新增配置。
- Fixed the Stop action disappearing in Writing and Game when a follow-up draft was entered during an active Agent stream, leaving no way to interrupt the current run. The shared composer now always retains the operation-scoped Stop action and shows Send beside it when a draft is available. Writing activates Stop directly from the durably accepted `agent_cycle_started` Run identity instead of waiting for an additional status read that can race Task registration, and an older idle response cannot overwrite that identity. Stop remains visibly disabled after abort submission or before an exact Run identity is available. No new setting is required.
- 修复 Agent 在 Ask/Permission 等交互等待跨秒恢复，或 compaction 等结构维护重新装配上下文时读取新的 wall clock，导致 materialized Definition fingerprint 漂移、同一 Run 无法继续的问题。Session actor 现在把本轮开始时间与 durable `CycleStarted` 一次性持久化，并通过 `RunView.StartedAt` 交给写作和游戏 ContextSource；无真实 cycle 的结构维护不会注入临时时间。无需新增配置。
- Fixed Agent runs failing to resume after Ask/Permission crossed a clock boundary, or during structural maintenance such as compaction, because context assembly read a new wall clock and changed the materialized Definition fingerprint. The Session actor now persists the turn start time once with durable `CycleStarted` and exposes it to Writing and Game ContextSources through `RunView.StartedAt`; structural maintenance without a real cycle no longer injects transient time. No new setting is required.
- 修复统一迁移到公共 `Agent -> Session -> Run` 生命周期后，模型生成工具调用时的名称与参数增量未进入公共事件流、前端只能在工具执行开始后一次性看到完整输入的问题。Agent runtime 现在实时投影工具输入开始与参数增量，执行开始沿用同一调用身份且不会重复创建工具卡；写作与游戏模式共用该修复，无需新增配置。
- Fixed model-generated tool names and argument deltas being omitted from the public event stream after the migration to the shared `Agent -> Session -> Run` lifecycle, which made complete input appear only when execution began. Agent runtime now projects tool-input start and argument deltas in real time, while execution start keeps the same call identity without duplicating the tool card. Writing and Game share the fix, with no new setting required.
- 修复迁移到公共 `Agent -> Session -> Run` 生命周期后，`write` / `edit` 产生的工作区变更缺少当前 Run、Session 与审阅线程身份，导致会话中的 Diff review 查询不到任何文件的问题。Denova 公共 Host 现在显式注入工作区变更作用域，创建、编辑和删除文件都会归入当前会话的持久化 Diff review；无需新增配置。
- Fixed workspace changes made by `write` / `edit` losing their current Run, Session, and review-thread identity after the migration to the public `Agent -> Session -> Run` lifecycle, which left conversation Diff review empty. Denova's public host now explicitly injects the workspace-change scope, so file creates, edits, and deletes all remain attached to the current conversation's durable Diff review. No new setting is required.
- 修复 Harness Optimizer 因 `skill_reference` 与 `trajectory` 的共享 `read.path` Schema 不一致而无法构建的问题；trajectory URI 的路由约束保留在适配器校验中，不新增配置。
- Fixed Harness Optimizer startup failing when `skill_reference` and `trajectory` exposed conflicting schemas for the shared `read.path` parameter. Trajectory URI routing remains enforced by the adapter without adding configuration.

### Removed

- 清理本开发周期内未发布的过渡兼容层：Agent 工具结果与 runtime journal 只保留最终契约，项目 registry / migration receipt 与自动化 runtime receipt 只接受最终版本，游戏故事只写共享 Conversation Journal 格式，玩家输入 checkpoint 只接受最终字段，AgentChat 工作台与终端只恢复最终状态结构。同步移除旧字段、双写、回填、推断、一次性修复及其专用测试；master 已发布的配置、项目、会话、故事和自动化数据读取能力保持不变。
- Removed compatibility layers for intermediate formats introduced during this unreleased development cycle. Agent tool results and runtime journals now expose only their final contracts; project registry/migration receipts and automation runtime receipts accept only their final versions; Game stories write only the shared Conversation Journal format; player-input checkpoints require their final fields; and AgentChat workbenches and terminals restore only the final state schema. Obsolete fields, dual writes, backfills, inference paths, one-time repairs, and their dedicated tests were removed, while reading configuration, project, session, story, and automation data released on `master` remains supported.
- 移除模型可调用的 `checkpoint` / `rewind`（`context_rewind`）能力，以及对应的运行时上下文改写、会话 boundary 记录和持久化 rewind 投影。上下文管理统一由阈值驱动的 Tool Result Cleanup 与自动 Compaction 负责；运行时内部 journal checkpoint 和前端展示断点不受影响。Beta 不提供旧手动 rewind 状态的兼容或迁移层。
- Removed the model-callable `checkpoint` / `rewind` (`context_rewind`) capability together with its runtime transcript rewriting, session boundary records, and durable rewind projection. Context management now relies exclusively on threshold-driven Tool Result Cleanup and automatic Compaction; internal journal checkpoints and UI display checkpoints are unchanged. As a Beta breaking change, existing manual rewind state has no compatibility or migration layer.

### Added

- 新增默认关闭的用户级「持续进化」Lab：已有 Session、Run trace 与显式 Outcome 共同构成 trajectory，用户可在 Agents 页手动或定时启动 Harness Optimizer，用通用文件、Shell、Skill 与 Task 工具在隔离草稿中更新 system prompt、动态上下文、工具描述和 SubAgent spec，校验后原子发布给后续所有新 Run。Agent package 只负责可复用的 State 快照、CAS、草稿与发布语义；State 的 go-git 版本、Diff 和恢复完全由 Denova 应用层管理。功能、配置与页面均支持中英文。
- Added an opt-in user-level Continual Learning Lab. Existing Sessions, Run traces, and explicit Outcomes form the trajectory; users can start the Harness Optimizer manually or on a schedule from Agents, using the general file, shell, Skill, and Task tools to update system prompts, dynamic context, tool descriptions, and SubAgent specs in an isolated draft before validated atomic publication to all subsequent Runs. The Agent package owns only reusable State snapshots, CAS, drafts, and publication semantics; Denova's application layer exclusively owns go-git State history, diffs, and restore. The feature, settings, and UI are bilingual.
- AgentChat 与创作 Agent 新增会话级 `/goal`：通过 `/goal` 或输入动作菜单顶部的「目标」开关显式进入，未激活时不在输入框常驻；激活后在权限右侧显示可关闭的模式提示，并与同样样式的「计划」提示严格互斥。提交后可编辑、暂停、清除目标，活动目标会以紧凑卡片固定展示在输入框上方；游戏模式不提供 Goal。目标通过 Conversation Journal 持久化并以稳定的最终用户前缀注入模型上下文；运行成功后若目标仍活动，会创建 UI 隐藏但模型可见的持久化续跑指令，直至模型通过仅根 Agent 可用的 `goal_finish` 标记完成或阻塞。Goal 能力沿用 Agent capability 配置，不增加用户配置项；所有状态、API、恢复链路和交互均支持中英文。
- AgentChat and the Writing Agent now support session-scoped `/goal`: Goal mode is entered explicitly through `/goal` or the Goal toggle at the top of Input Actions and never remains visible in an inactive composer. Once active, its removable mode indicator appears to the right of permission and is strictly exclusive with the identically styled Plan indicator. Submitted goals can be edited, paused, or cleared, while the active goal remains in a compact card directly above the composer; Game mode does not expose Goal. Goals persist through the Conversation Journal and are injected as a stable final-user prefix for model context. After a successful run, an active goal creates a durable continuation input that stays hidden from the UI but visible to the model until the root Agent calls `goal_finish` with completed or blocked status. Goal reuses the existing Agent capability configuration without adding a user setting, and all states, APIs, recovery paths, and interactions support Chinese and English.
- 新增面向 macOS/Linux 的 `scripts/install.sh` 一键安装器：自动识别 ARM64/x64，下载最新或指定版本 GitHub Release，依据发布清单校验 SHA-256，并以无需 sudo 的用户级目录原子安装；安装时会确认用户数据目录，默认 `~/.denova`，可交互选择其他目录或通过 `DENOVA_DIR` 覆盖，重复安装会保留该选择与已有全局启动配置。README 已补充中英文 `curl -fsSL` 安装方式，无需新增应用配置项。
- Added `scripts/install.sh` for one-command macOS/Linux setup. It detects ARM64/x64, downloads the latest or a selected GitHub Release, verifies SHA-256 against the published manifest, and atomically installs into user-owned directories without sudo. Installation confirms the user-data directory, defaults to `~/.denova`, accepts another path interactively or through `DENOVA_DIR`, and preserves that selection plus the existing global startup configuration on reinstall. Both READMEs now document the `curl -fsSL` flow; no application setting is added.
- 图像 Agent 配置页现在明确分开展示“模型与思考”和真正执行生成的“出图模型”，可从已有图像 Profile 中选择写作与游戏共用的默认出图模型；写作与游戏输入框左侧的输入动作菜单同步新增 Image Agent 语言模型、图像模型两列快捷切换，直接复用 Agents 页的分层配置并即时保存：语言模型保持用户级，出图模型会延续当前 Project 已有的工作区覆盖，否则更新共享用户默认；配置并发变化时自动基于最新版本重放本次选择。设置页的每个图像 Profile 同时新增“测试连接”，使用当前未保存草稿并仅在同一 Endpoint 源站继承密钥，通过一次不写入工作区的最小真实出图校验地址、鉴权、模型与响应解析。测试请求不附加 LLM 超时，图像服务商可能计费；复用现有默认图像 Profile 配置，无需新增配置项。
- The Image Agent settings now clearly separate Model and Thinking from the Output Image Model that performs generation, allowing an existing image profile to be selected as the shared Writing/Game default. The input-actions menu on the left side of both Writing and Game composers now also provides quick Image Agent Language Model and Image Model selectors, reusing the same layered settings as the Agents page and saving immediately. The language model remains user-scoped; the output model preserves an existing Project workspace override and otherwise updates the shared user default. A concurrent settings change causes the selection to be replayed over the latest revision. Every image profile in Settings also gains Test Connection, which combines the current unsaved draft with saved credentials only on the same endpoint origin and performs one minimal real generation without writing into a workspace, validating routing, authentication, model availability, and response parsing. The probe adds no LLM timeout and may incur provider charges; it reuses the existing default-image-profile setting, so no new configuration field is required.
- 工作台 AgentChat 的「添加项目」现在先打开统一选择弹窗，可打开任意本地目录，或复用完整书籍表单直接在 Nova 数据目录中创建新书；创建前会保存当前草稿，完成后自动刷新并选中新 Project，同时保持在 AgentChat，不会切换写作/游戏模式。中英文与深浅主题共用该流程，无需新增配置。
- Workspace AgentChat's Add Project action now opens a unified chooser first, allowing users to open any local folder or create a new Book in the Nova data directory through the complete shared Book form. Pending drafts are saved before creation; the new Project is refreshed and selected afterward while the app remains in AgentChat without switching Writing/Game mode. The flow supports Chinese/English and light/dark themes, with no new setting required.
- 写作、游戏、工作台 AgentChat、配置管理与自动化的共享输入框现在同时支持 `/` 与中文顿号 `、` 调起命令 / Skills 选择器；该兼容保持无感，不改变原有 `/` 操作提示，通过 `、` 选择 Skill 后仍写入标准 `/skill-name` token，后端协议与已有内容格式保持不变。无需新增配置。
- The shared composer in Writing, Game, Workspace AgentChat, Config Manager, and Automations now opens the command / Skills picker with either `/` or the Chinese enumeration comma `、`. This remains an unobtrusive input-method compatibility path without changing the existing `/` hint, and selecting a Skill through `、` still inserts the canonical `/skill-name` token, preserving the backend protocol and existing content format. No new setting is required.
- Agent 输入框的 `@` 文件 / 资料选择器现在复用命令 / Skills 选择器的自适应弹层与紧凑单行候选布局：文件名、路径或资料说明及类型在同一行展示并安全截断。保持输入框焦点时可用上下方向键循环高亮，并用 Tab 或 Enter 插入当前候选；同一公共交互也覆盖写作与游戏的 `#` 场景风格选择。中英文、深浅主题、窄屏与长文本共用该行为，无需新增配置。
- The Agent composer's `@` file / lore picker now reuses the adaptive command / Skills menu shell and compact single-line option layout, keeping the file name, path or lore description, and type on one safely truncated row. While focus stays in the composer, Up/Down cycles the active option and Tab or Enter inserts it; the same shared interaction also covers `#` scene-style choices in Writing and Game. Chinese/English, light/dark themes, narrow screens, and long text share the behavior, with no new setting required.
- 写作页与 AgentChat 的文档标签栏现在支持直接拖动排序，并沿用现有工作区/Project 状态持久化顺序；统一的拖拽交互支持鼠标、触摸长按与键盘操作，固定标签仍稳定置前。AgentChat 标签还可在左右分栏间直接拖动迁移，跨栏时会显示跟随指针的标签预览和与最终前后位置一致的插入线。无需新增配置。
- Writing and AgentChat document tab strips now support direct drag reordering and persist the result through their existing workspace/Project state. The shared interaction supports mouse, touch-and-hold, and keyboard operation, while pinned tabs remain stably grouped first. AgentChat tabs can also be dragged between split panes, with a pointer-following tab preview and insertion line that matches the final before/after placement. No new setting is required.
- AgentChat 的阅读器与资料库 Tab 现在按稳定 Project ID 打开任意 Book，并复用写作页的章节目录、Markdown 编辑器、资料目录、自动保存、冲突恢复和文本审阅能力；打开或编辑后台 Book 不再切换当前书籍或写作/游戏模式。新增统一的 `/api/projects/:project_id/book` 资源边界，覆盖书籍快照、目录、统计、章节状态、资料条目与文本审阅；主写作页同步改用同一 Project API，Files、写作页与 Agent/Review 变更按项目双向刷新且不会产生自刷新回声。文本审阅账本同样归属稳定 Project 状态，目录重定位后仍保持唯一。无需新增配置。
- AgentChat Reader and Lore tabs can now open any Book by stable Project ID and reuse Writing's chapter outline, Markdown editor, lore directory, autosave, conflict recovery, and document-review capabilities; opening or editing a background Book no longer switches the active Book or Writing/Game mode. A unified `/api/projects/:project_id/book` resource boundary now covers Book snapshots, trees, summaries, chapter status, lore items, and document review, and the main Writing surface uses the same Project APIs. Files, project pages, and Agent/Review mutations invalidate one another per project without refresh echoes. Document-review ledgers now belong to stable Project state as well and remain singular across directory relinks. No new setting is required.
- 模型设置中的语言与图像 API Key 统一改为紧凑的密钥输入组：使用同一边框容纳密钥图标与显示/隐藏操作，默认继续遮蔽内容，并保持深浅主题与窄屏布局一致。
- Language and image API keys in Model Settings now share a compact key input group, keeping the key icon and show/hide action inside one control while remaining masked by default and consistent across themes and narrow layouts.
- 语言模型接入改为「服务商预设 + API 协议适配器」两层：内置 OpenAI Chat Completions、OpenAI Responses 与 Anthropic Messages 三种通用协议，并补充 DeepSeek、Anthropic、Gemini、MiniMax、OpenRouter、Groq、Mistral、xAI、Moonshot、硅基流动、火山方舟、Ollama、LM Studio、LiteLLM、vLLM 等常用服务商预设。Provider 收敛为内置目录；自定义端点统一使用「兼容 / 自定义端点」服务商，再组合协议、Base URL、自定义 Header 与协议兼容选项；Gemini 使用 Google 官方 OpenAI 兼容路由。DeepSeek 现完整支持 Chat Completions 的 thinking 开关、模型感知的思考强度（含 `xhigh`）与工具调用 reasoning 回传，同时支持其 Responses API 和 Anthropic API 路由；MiniMax 默认使用可完整续传 thinking block 的 Anthropic 路由。各协议均通过同一 Agent 消息、工具、结构化输出、流式响应、用量与续传边界运行。旧 `model_profiles` 中 `openai_api_key` / `openai_base_url` / `openai_model` 会自动迁移为通用 `api_key` / `base_url` / `model`，且不覆盖已存在的新字段。
- Language-model integration now has two explicit layers: provider presets and API protocol adapters. Denova ships three common adapters—OpenAI Chat Completions, OpenAI Responses, and Anthropic Messages—plus presets for DeepSeek, Anthropic, Gemini, MiniMax, OpenRouter, Groq, Mistral, xAI, Moonshot, SiliconFlow, Volcengine Ark, Ollama, LM Studio, LiteLLM, vLLM, and other common endpoints. Provider IDs now come from the built-in catalog; custom endpoints use the Compatible / Custom Endpoint provider and then combine a protocol, Base URL, custom headers, and protocol compatibility options. Gemini uses Google's official OpenAI-compatible route. DeepSeek supports the Chat Completions thinking toggle, model-aware effort including `xhigh`, and tool-call reasoning replay, together with its Responses API and Anthropic API routes; MiniMax defaults to the Anthropic route so thinking blocks can be continued intact. Every protocol shares the same Agent message, tool, structured-output, streaming, usage, and continuation boundary. Legacy `openai_api_key`, `openai_base_url`, and `openai_model` fields in `model_profiles` are migrated automatically to `api_key`, `base_url`, and `model` without overriding canonical fields already present.
- 设置中的每个语言模型 Profile 新增「测试连接」，会使用当前未保存草稿、且仅在同一 Endpoint 源站继承已保存密钥，通过真实 Agent 协议适配器发送一次最小生成请求，同时校验路由、鉴权、模型可用性、请求序列化与响应解析并展示实际协议与耗时；编辑或移除 Profile 会取消并清除过期结果，请求不附加 LLM 超时。
- Every language-model profile in Settings now has Test Connection. It combines the current unsaved draft with saved secrets inherited only within the same endpoint origin and sends a minimal real generation through the Agent protocol adapter, validating routing, authentication, model availability, request serialization, and response parsing while reporting the effective protocol and latency. Editing or removing a profile cancels and clears stale results, and the probe adds no LLM timeout.
- Chat Completions 与 Responses Profile 新增 OpenAI-compatible `GET /models` 候选获取；请求复用当前未保存路由、自定义 Header 与同源密钥继承规则，跳过空 ID 并对部分有效结果去重排序。返回列表始终只作为建议：模型文本框保持完全可编辑，未列出的自定义模型 ID 仍可保存、测试和运行，上游不支持或请求失败也不会阻断手动输入。
- Chat Completions and Responses profiles can now load suggestions from an OpenAI-compatible `GET /models` endpoint. Requests reuse the current unsaved route, custom headers, and same-origin secret inheritance, while blank IDs are skipped and partial valid results are deduplicated and sorted. The returned list is always advisory: the model text field stays fully editable, unlisted custom model IDs can still be saved, tested, and used, and unsupported or failed discovery never blocks manual input.
- 工作台 AgentChat 新增通用 Files Tab，Book 与任意本地目录均通过稳定 Project ID 独立浏览和编辑，不会切换当前写作/游戏模式。Files 采用左侧 Monaco 源码编辑器与右侧虚拟化文件树；首次加载会将根目录、已展开分支和当前文件祖先合并为有预算的批量请求，自动解析单子目录链，并以可校验游标分页超宽目录，避免深层目录逐级等待，同时保持超大项目按需加载。文件树支持一键全部折叠、Alt/Option 递归折叠已加载分支、键盘导航、多选拖拽、定位当前文件、项目级布局/展开状态持久化、生成目录与普通目录一致展示和窄屏抽屉。Markdown 默认使用安全预览并可切换源码，项目内相对链接与图片可直接导航；源码默认自动换行且记住用户偏好。Monaco 改为模型驱动更新，移除逐键受控同步和多余滚动效果，长文件滚动与输入保持流畅。Files 同时支持自动/手动保存、revision CAS 与非重叠并发三方合并、安全图片预览，以及新建、重命名、复制、移动和删除；符号链接本身可安全重命名、移动或删除，但写入与复制不会沿链接访问目标。批量文件操作逐项返回结果，Review 中打开源码会复用同项目 Files Tab；project-scoped 文件 API 不依赖当前前台 workspace。
- Workspace AgentChat now includes a generic Files tab for both Books and arbitrary local directories, scoped by stable Project ID and never switching the active Writing/Game mode. Files places a Monaco source editor on the left and a virtualized explorer on the right. Initial loading batches the root, restored branches, and current-file ancestors behind one entry budget, follows single-directory chains, and paginates exceptionally wide directories with validated cursors, avoiding round trips at every depth while retaining on-demand loading for very large projects. The explorer supports collapse all, Alt/Option recursive folding of loaded branches, keyboard navigation, multi-select drag and drop, reveal current file, per-project layout and expansion persistence, generated folders shown like ordinary folders, and a narrow-screen drawer. Markdown opens in a safe preview by default with a Source toggle and project-relative file/image navigation; source editing wraps by default and remembers the user preference. Monaco now uses model-driven updates instead of per-keystroke controlled synchronization and disables unnecessary scroll effects, keeping long-file scrolling and typing responsive. Files also provides automatic/manual saves, revision CAS with non-overlapping three-way rebases, safe image previews, and create, rename, copy, move, and delete operations. Symbolic links themselves can be renamed, moved, or deleted safely, while writes and copies never follow them to their targets. Batch operations report each item independently, Review source links reuse the same project's Files tab, and the project-scoped file API does not depend on the foreground workspace.
- 新增轻量 `GET /api/activity/summary`，只返回消息未读数、自动化待处理数和运行中任务数；顶栏改为轮询该摘要，完整消息及自动化内容仅在用户打开对应界面时读取。普通 API 响应在客户端支持时启用 gzip，SSE 与协议升级连接保持流式直传。
- Added lightweight `GET /api/activity/summary`, returning only message unread, pending automation, and active-run counts. The header now polls this summary, while full message and automation content loads only when the user opens the corresponding surface. Regular API responses use gzip when supported, while SSE and protocol-upgrade connections remain streamed directly.
- 后端日志（含 Hertz 框架日志）统一迁移到标准库 `log/slog` 的 context-aware API，并保留源码位置与现有按日日志文件；每个 HTTP 请求由服务端生成 UUIDv7 `request_id`，通过 `X-Request-ID` 响应头、JSON 错误体、SSE / AI UI 流式错误和请求完成日志保持一致。请求 context 会继续传入 handlers、SSE、WebSocket 及脱离 HTTP 生命周期的 Agent/Automation 后台任务；启动与定时任务等无请求所有权的日志显式使用后台 context。前端 API 错误现在以中英文本地化的「日志 ID / Log ID」展示该标识，便于从界面报错直接定位服务端日志。
- Backend logging, including Hertz framework diagnostics, now consistently uses the standard library's context-aware `log/slog` APIs while retaining source locations and the existing daily log files. The server creates a UUIDv7 `request_id` for every HTTP request and keeps it identical across the `X-Request-ID` response header, JSON error payload, SSE/AI UI streaming error, and request-completion log. Request context continues through handlers, SSE, WebSockets, and Agent/Automation tasks detached from the HTTP lifetime; startup and scheduled work without request ownership use an explicit background context. Frontend API errors now show the identifier through the localized “日志 ID / Log ID” label so a visible failure maps directly to server logs.
- Settings 写入收敛为单一 `PATCH /api/settings`：请求显式携带 `user` / `workspace` 层与 RFC 7386 `changes`，只修改出现的字段，省略字段保持原值、`null` 清除继承值，并保留 revision CAS、严格字段校验与工作区作用域边界。Beta 不兼容：旧 `PUT /api/settings/user`、`PUT /api/settings/workspace` 和 `PUT /api/settings/agent-approval-mode` 路径已移除。
- Settings writes now use one `PATCH /api/settings` endpoint with an explicit `user` or `workspace` layer and RFC 7386 `changes`. Only present fields change, omitted fields stay intact, and `null` clears an inherited value, while revision CAS, strict field validation, and workspace scope boundaries remain enforced. Beta breaking: the former `PUT /api/settings/user`, `PUT /api/settings/workspace`, and `PUT /api/settings/agent-approval-mode` routes are removed.
- 写作、工作台 Agent Chat、游戏分支与配置管理对话现在分别持久化模型 Profile、思考强度和权限等级，并通过统一的会话配置 CAS 接口修改；旧会话首次访问时从当前 Settings 冻结一次，此后不再随全局设置漂移，新会话继承同类最近配置，游戏新分支复制来源分支配置。会话 journal 在冷启动与多句柄刷新时保持同一 revision 序列；Agent Chat 空白草稿只预览继承值，不会因打开模型选择器而创建空会话。
- Writing, Workspace Agent Chat, Game branches, and Config Manager conversations now persist their own model profile, thinking effort, and permission level behind one compare-and-swap conversation configuration API. Existing conversations freeze current Settings once on first access and no longer drift with global changes; new conversations inherit the latest same-kind selection, and a new Game branch copies its source branch. Conversation journals preserve one revision sequence across cold starts and multi-handle refreshes; blank Agent Chat drafts preview inherited values without creating empty durable sessions.
- 工作台 AgentChat 新增可持久化、可调宽的右侧工作区：首次开启可直接选择对话、终端、阅读器、资料库等现有 Tab 类型，隐藏时保留 Tab、编辑状态与后台运行，窄屏自动切换为右侧抽屉。开关始终锚定当前最右侧可见 Tab 栏，双栏打开时随控制权移动到右栏，交互与状态文案覆盖中英文和深浅主题。
- Workspace AgentChat now has a persistent, resizable secondary workspace. Its first open can create any existing tab type—including chat, terminal, Reader, and Lore—while hiding preserves tabs, editor state, and background runs; narrow screens use a right-side drawer. The toggle stays anchored to the rightmost visible tab strip, moving to the secondary strip while split view is open, with bilingual copy and light/dark theme support.
- 修复可持久化分栏在刷新时以隐藏状态挂载后，首次展开丢失已保存宽度并退回最小值的问题；分栏现在会显式恢复当前布局上下文的持久化比例，动态切换项目也不会误用上一个项目的宽度。AgentChat 右侧工作区首次打开默认占可用分栏空间的 66%，左侧对话至少保留 360px，用户拖拽后的项目级宽度仍优先恢复，右侧可继续扩展至 75%。
- Fixed persisted split panes losing their saved width and falling back to the minimum when a reload mounted them hidden before their first expansion. Panes now explicitly restore the persisted percentage for the active layout context, and dynamic project switches no longer inherit the previous project's width. AgentChat's secondary workspace now starts at 66% of the available split area, keeps at least 360px for the primary chat, still restores user-resized project layouts first, and can expand to 75%.
- 游戏剧情的每条已落盘 AI 回复新增就地创建分支操作，历史分页回复同样可用；分支创建与完整路线图共用同一套命名、来源预览、错误处理和自动切换流程。原「分支路线」一级菜单移除，导演控制台新增可持久化的「分支」页签，用紧凑列表预览、切换剧情线并进入保留现有创建/删除能力的完整路线图；路线图内仍保持「剧情」一级菜单为唯一选中项。所有入口覆盖中英文、窄屏和深浅主题。
- Every persisted Game AI reply now offers in-place branch creation, including replies loaded through history paging. Reply actions and the full route map share one naming, source-preview, error, and automatic-switch flow. The former top-level Story Map item is removed; a persistent Branches tab in the Director Console provides compact route previews, branch switching, and entry to the full map with its existing create/delete controls. The Story top-level item remains the sole selection while the map is open. All entry points support Chinese and English, narrow layouts, and light/dark themes.
- Agent 工具执行新增用户级 Ask / Write / Full access 安全模式；新用户直接使用 Write 默认值，不再经过首次选择弹窗。写作输入框下方的当前模式仍可直接切换；游戏输入框默认使用当前模式，只在「输入动作」选项中提供切换，避免干扰回合输入。设置页提供新会话首次初始化时的用户级默认值，已有会话各自保留输入框中的独立选择，每次运行固定使用启动时的会话值。Ask 自动允许结构化工作区编辑与常用只读 Shell，Write 额外允许常用开发、构建、依赖和网络读取，Full access 只保留极高危黑名单。网络、外部写入、远端修改及未知命令按模式直接在对应工具卡内展示无超时、可恢复的审批状态，不再生成打断执行折叠的独立卡片；原因通过规则 ID 使用当前界面语言展示。审批提供「拒绝」「仅允许本次」，以及仅对可安全复用静态命令开放的「在此工作区始终允许」。持久规则按版本化的工具、Project、实际工作区与策略派生命令族匹配（例如 `go test ...`），精确参数 hash 仅用于审计；每次调用仍会重新解析并经过当前安全策略，动态 Shell、歧义命令和危险参数变体不会复用，项目重连其他目录也不会继承旧目录权限。规则在继续工具执行前原子写入用户设置，可在 Agent 设置中逐条撤销；工作区配置不能降低用户安全级别。Unix Agent 默认使用本机 Bash，并通过用户登录 Shell 捕获 export 环境以支持 GUI Release 下的 Homebrew/nvm/npm 等 PATH 配置；Windows 保持原生 PowerShell。该能力是执行前护栏而非 OS 沙箱，已放行的命令和项目脚本仍拥有 Denova 进程的本机权限。
- Agent tool execution now has user-scoped Ask, Write, and Full access safety modes. New users start directly in Write without a mandatory first-choice dialog. Writing keeps the current mode as a direct composer switcher; Game uses the current mode by default and exposes switching only under Input actions so it does not distract from turn input. Settings supplies the user-level fallback used when a conversation is initialized for the first time; existing conversations keep independent composer selections, and each run snapshots its conversation value at start. Ask auto-allows structured workspace edits and common read-only Shell commands; Write also allows common development, build, dependency, and network-read commands; Full access retains only a critical blocklist. Depending on the mode, network access, external writes, remote mutations, and unknown commands now show a durable, timeout-free approval state inside the owning tool card instead of creating a separate card that breaks the execution fold; reasons are localized from stable rule IDs. Approvals offer Deny, Allow once, and—only for safely reusable static commands—Always allow in this workspace. Persisted rules match a versioned policy-derived tool, Project, concrete workspace, and command family such as `go test ...`; the exact argument hash is audit-only. Every invocation is reparsed and rechecked, so dynamic Shell syntax, ambiguous commands, and dangerous parameter variants never inherit approval, and relinking a Project cannot carry permissions into a different directory. A rule is atomically persisted to user settings before the tool resumes and can be revoked individually in Agent settings; workspace settings cannot weaken the user's posture. Unix Agents use local Bash with exported environment captured from the user's login shell so GUI releases inherit Homebrew/nvm/npm PATH setup; Windows keeps native PowerShell. This is a pre-execution guardrail rather than an OS sandbox; allowed commands and project scripts retain the local permissions of the Denova process.
- 工作台现在以稳定 Project ID 统一管理 Book 与任意本地目录：Project 支持重命名、排序、目录重连和归档，目录缺失时保留项目与历史以便重新关联。添加或重新关联项目会直接打开系统目录选择器；新增项目默认使用文件夹名，后端根据书籍结构自动选择 Writing Agent 或 General Agent，不再要求用户填写名称、路径或 Agent 类型。Denova 数据目录不会作为默认 Project 暴露，但用户显式添加后与其他目录完全一致。打开 Project、对话或终端不会切换当前写作/游戏模式。
- Workspace now manages Books and arbitrary local directories through one stable Project identity. Projects can be renamed, reordered, relinked, and archived; a missing directory keeps its identity and history for later relinking. Adding or relinking a Project opens the system folder picker directly; a new Project uses the folder name, and the backend derives Writing Agent or General Agent behavior from the Book structure without asking for a name, path, or Agent type. The Denova data directory is not exposed as a default Project, but behaves exactly like any other directory when explicitly added. Opening a Project, conversation, or terminal never switches the active Writing/Game mode.
- 新增 General Agent，定位与 Codex、Claude Code、OMP 一致，提供文件读写、Shell、Web、Browser、Ask、Todo、Skills、上下文恢复与 SubAgent 等通用能力。`glob` / `grep` 默认遵守 `.gitignore`，显式路径读写不做硬屏蔽，Shell 保持原生语义，Denova 不会自动创建或修改 `.gitignore`。
- Added the General Agent for Codex/Claude Code/OMP-style work with file editing, shell, web, browser, Ask, Todo, Skills, context recovery, and SubAgent capabilities. `glob` and `grep` respect `.gitignore` by default, explicit-path reads and writes are not hard-blocked, shell behavior remains native, and Denova never creates or changes `.gitignore` automatically.
- Project 对话、配置、变更记录、运行状态、工具产物和 Project 自动化统一保存到 Denova 用户数据目录下的 `project-state/<project-id>/`，不再写入内容目录；互动故事等创作内容继续属于 workspace。首次访问会从旧 `.denova` / `.nova` 位置执行仅复制迁移，保留源数据与迁移回执作为回滚路径。Beta 行为变化：工作台 API、标签页和终端绑定只使用 Project ID，不再接受路径作为资源身份。
- Project conversations, configuration, change journals, run state, tool artifacts, and Project automations now live under `project-state/<project-id>/` in the Denova user data directory instead of the content folder; authored content such as interactive stories remains in the workspace. First access performs a copy-only migration from legacy `.denova` / `.nova` locations, retaining the source and a migration receipt for rollback. Beta behavior change: Workspace APIs, tabs, and terminal bindings use only Project IDs and no longer accept paths as resource identity.

- Agent 工具结果与上下文维护改为统一的压力驱动链路：写作和游戏模式都在当前 run 及低压力后续轮保留结构化 rich result，超大可恢复结果可在成功沉淀后按缓存成本提前转换为含普通 `read` 路径/重放参数与大小的 receipt；完整大输出原子外置到 workspace/session 有界 artifact，失败时保持 inline 上限并保护不可恢复证据。达到 70% 后仅在收益覆盖 warm prefix 改写成本且可一次恢复到 60% 时批量清理，最近 3 组、错误、写操作和 protected 结果始终保留；85% 时无论可清理量是否达标都会进入 checkpoint。
- Agent tool results and context maintenance now share one pressure-driven pipeline across Writing and Game. Structured rich results remain available throughout the current run and later low-pressure turns; exceptionally large recoverable results may become receipts only after successful settlement and a cache-cost gate, retaining an ordinary `read` path/replay parameters and size. Complete oversized output is atomically materialized into workspace/session-bounded artifacts, with fail-bounded inline output and protected irreplaceable evidence. Above 70%, cleanup runs only when savings cover warm-prefix mutation and one batch reaches the 60% recovery target; the latest three groups, errors, mutations, and protected results remain rich, while 85% always advances to checkpoint compaction even when cleanup savings are insufficient.
- 上下文压缩改为复用主 Agent 最终 model、system、tools、thinking、cache route 与历史前缀的 append-only side fork，只追加稳定的 Markdown prompt schema，不使用 JSON/section validation schema；压缩后重新注入确定性状态并执行 tool call/result 协议归一化、真实 post-context 水位校验、degraded/no-progress 防循环和连续失败熔断。新增按 Agent 可继承的 cleanup/cache/recovery 配置、双语设置项，以及实际回收量、cache read、fallback、artifact 持久化和保留策略观测。
- Context compaction now reuses the primary Agent's final model, system, tools, thinking settings, cache route, and history prefix through an append-only side fork with one stable Markdown prompt schema—without JSON or section-validation schemas. Deterministic state is re-injected before tool call/result normalization and true post-context recovery checks, with degraded/no-progress loop prevention and a consecutive-failure fuse. Per-Agent inherited cleanup/cache/recovery settings, bilingual controls, and observability for actual reclaimed tokens, cache reads, fallbacks, artifact persistence, and retention policy are included.

- 终端启动命令收敛为用户级可排序注册表，可在设置中新增、编辑、启用或停用任意 CLI，并同时作用于写作与游戏模式；Codex CLI 与 Claude Code 作为可编辑预设保留。工作台新建菜单只展示已启用命令，终端会话通过稳定 ID 在后端解析真实命令，运行时菜单接口不暴露命令行内容。本开发周期内早期标量配置和旧终端标签属于未发布中间态，不再迁移或恢复。
- Terminal launch commands now use one ordered user-level registry shared by Writing and Game modes. Settings can add, edit, enable, or disable any CLI, while Codex CLI and Claude Code remain editable presets. The Workspace New menu shows only enabled commands, terminal sessions resolve stable IDs on the backend, and runtime menu metadata does not expose command lines. Earlier scalar settings and terminal-tab schemas from this unreleased development cycle are no longer migrated or restored.

- 写作工作区新增独立「设定」Tab，并在左侧作品设定栏与灵感、状态并列提供入口。该 Tab 以适应式分类目录和聚焦编辑器呈现资料库，支持富文本手动编辑、自动保存、元数据维护、引用到创作 Agent、跳转完整资料库，以及与正文一致的选区/行级评论。评论现在绑定通用的 `workspace_file` / `lore_item` 文本资源目标；提交给 Agent 后会携带服务端解析的资料身份、版本与锚点，并明确通过 Lore 工具修改对应条目。Agent 与手动编辑并发时使用三方合并，重叠修改和远端删除会先归档可恢复冲突。Lore API 同步绑定显式 workspace 身份，防止切换作品时把读写或图片任务落入另一部作品。旧版文件评论 ledger 与已保存 Tab / 作品设定 Pin 偏好会自动迁移。
- Writing now has a dedicated Lore tab, opened from a new Lore shortcut beside Ideas and State in the Book Settings sidebar. Its adaptive categorized directory and focused editor support rich-text editing, autosave, metadata, Writing Agent references, full-library navigation, and the same selection/line comments as manuscripts. Comments now target generic `workspace_file` or `lore_item` text resources; Agent feedback carries server-resolved resource identity, revision, and anchors and explicitly routes Lore edits through Lore tools. Concurrent Agent and manual edits use three-way rebasing, with recoverable conflict archives for overlaps and remote deletion. Lore APIs are bound to an explicit workspace identity so reads, writes, and image tasks cannot follow a book switch into another workspace. Legacy file-comment ledgers and saved tab/Book Settings pin preferences migrate automatically.

- 新增「工作台」一级菜单：左侧按项目分组列出全部书籍的对话（每个项目默认展示 5 条，可「展示更多」，并可直接在项目行新建对话，不随当前打开的书籍切换而变化），中间是可折叠的多标签工作台。标签页可以是内置 Agent 对话、本机终端，或直接挂载阅读器、资料库、预设、Skills、Agents、自动化等项目页面。终端基于后端 pty（`github.com/creack/pty` 系的 `xpty`）与 WebSocket，前端使用 xterm.js，支持直接新建 Shell、Codex CLI 与 Claude Code；终端会话独立于页面存在，刷新或切走标签页不会终止正在运行的 CLI，只有关闭标签页才结束进程。设置页新增「终端」分组，可配置开关、Shell、Codex / Claude 启动命令、并发会话上限与回滚缓冲大小。
- Added the Workspace top-level menu. The left pane groups conversations by project across the whole library — five per project by default with Show more, a per-project New chat action, and no dependency on which book is currently open — while the centre is a collapsible multi-tab workbench. A tab hosts a built-in Agent conversation, a local terminal, or an existing project page (Reader, Lore, Presets, Skills, Agents, Automations). Terminals are backed by backend ptys and a WebSocket with xterm.js on the client, with direct Shell, Codex CLI, and Claude Code creation. Terminal sessions outlive the page: reloading or switching tabs keeps a running CLI alive, and only closing the tab ends the process. Settings gains a Terminal section for the master switch, shell, Codex / Claude launch commands, concurrent-session cap, and scrollback size.

- 配置管理器的六类预设目录现在支持组内拖拽排序，顺序按工作区保存为本机 UI 偏好，不修改预设文件或产生内置覆盖；分组标题除右侧新建按钮外的整行都可展开或折叠。
- All six Config Manager preset directories now support drag reordering within their group. The order is stored per workspace as a local UI preference without modifying preset files or creating built-in overrides. The full group header, except its Create action, now expands or collapses the group.
- 内置叙事风格扩展为五套长期预设：新增默认的「节奏叙事」，将原 `classic` 优化为「稳健叙事」，将「编剧风格」改为忠实承接情节的标准剧本格式，并重写「暗黑压抑」的压力、代价与人物能动性表达；「直白情色」提示保持不变。叙事风格现在可声明适用于写作、游戏或两种模式，旧自定义风格未声明时继续在两种模式可用。写作与游戏分别记住用户最近选择；只有从未选择或所选风格在当前模式不可用时才回退到「节奏叙事」。Beta 行为变化：未覆盖的内置预设和默认故事导演会刷新到新版内容，用户覆盖与显式保存的既有选择不变。
- Built-in narrative styles now provide five durable presets: the new default Momentum Narrative, the former `classic` preset refined as Steady Narrative, Screenplay Style converted to plot-preserving standard screenplay form, and Bleak Narrative rewritten around pressure, cost, and character agency; the Direct Erotica prompts are unchanged. Styles can target Writing, Game, or both modes, while legacy custom styles without mode metadata remain shared. Writing and Game remember their most recent selections independently and fall back to Momentum Narrative only when no selection exists or the selection is unavailable in that mode. Beta behavior change: untouched built-ins and the default story director refresh to the new definitions; user overrides and explicitly saved selections remain intact.
- Agent 内置工具集完成 OMP 对齐重构：新增 Adapter 驱动且支持不同参数 schema 的统一 `read`（本地文本、目录、Skill reference 与 EventCard），独立 `web_search` / `web_fetch`，平台原生 `bash` / `pwsh`，整表替换 `todo`，进程内可恢复且冷重启安全取消的 `ask`，invocation 级隔离的命名标签页 `browser`，以及只回退模型有效上下文的持久化 `checkpoint` / `rewind`。子 Agent 现在拥有独立 InvocationScope、run-owned 资源与 provider-independent execution ID，同时继承只记录副作用的 checkpoint receipt sink，不再继承父会话改写权。
- The built-in Agent toolset is now aligned with OMP: an adapter-driven unified `read` supports resource-specific parameter schemas for local text, directories, Skill references, and EventCards; `web_search` and `web_fetch` remain separate; hosts expose native `bash` or `pwsh`; `todo` replaces the complete plan; `ask` resumes while its process-local continuation remains alive and cancels safely after a cold restart; named-tab `browser` sessions are invocation-owned; and durable `checkpoint` / `rewind` rewrites model context only. Delegated Agents now receive isolated InvocationScopes, run-owned resources, and provider-independent execution IDs while inheriting only the parent checkpoint's mutation-receipt sink—not transcript rewrite authority.
- 配置管理工具收敛为 `config_read` / `config_apply` 与一个分层 `config-manager` Skill；十类资源通过 typed Adapter 暴露各自 scope、operation、revision 与 reference 契约，`agent_profile` 使用固定 ID `registry` 的单例快照。Automation 读写严格限制在用户或当前 workspace scope，配置 CAS 与 Skill 目录 revision 原子化，Config Manager 不能通过修改自身 profile 提升敏感工具权限。
- Configuration management is consolidated into `config_read` / `config_apply` plus one layered `config-manager` Skill. Ten typed resource adapters expose explicit scope, operation, revision, and reference contracts, and `agent_profile` is a singleton snapshot with the fixed ID `registry`. Automation access is confined to user or the active workspace scope, configuration CAS and Skill directory revisions are atomic, and Config Manager cannot elevate its own sensitive tool permissions through profile edits.
- Session checkpoint 的 exact effective / canonical boundary 现在只在规范 journal 的独立 `context_boundary` 记录中保存一次；checkpoint、rewind 与可重建 sidecar 仅持久化 `boundary_id + locator`。即使活跃日志超过 200 个事务并触发 resident trim，冷恢复仍会按 locator 读取原记录并校验 journal payload 与两份投影 hash；损坏会在模型调用前 fail closed。
- Exact effective and canonical Session checkpoint boundaries are now stored once in an independent canonical `context_boundary` journal record; checkpoint, rewind, and the rebuildable sidecar persist only `boundary_id + locator`. Cold recovery follows that locator and validates both the journal payload and projection hashes even after resident trimming beyond 200 transactions, failing closed before any model call on corruption.
- 写作页作品目录栏（大纲 tab）针对长章节列表重新组织导航：可 Pin 的书籍设定保留在等高顶部容器，当前细纲回到下方滚动目录；如需固定，可在书籍设定管理中选择语义化的「当前细纲」快捷项，它会始终跟随最新一份细纲。Pin / 取消 Pin 以不同图标表达且记忆偏好，不再通过高亮或切换时的 8px 位移表达状态；最新章与回顶移到 Pin 左侧，原有底部浮标全部移除，折叠分卷时不会再压住卷标题。当前细纲直接按名称打开，旧细纲收进历史细纲；最新章、目录定位、保存和编辑器设置统一收敛为带悬浮提示的纯图标操作，其中目录定位可打开作品目录、展开所在卷并定位当前章节。章节和细纲支持右键或悬停更多菜单；已 Pin 的书籍设定快捷项隐藏可见的更多按钮并将文案居中，文件操作仍保留在右键菜单。上述文件操作均可引用到 Chat、在项目文件中显示、重命名文件和确认删除。Chat 输入框中的文件引用只显示文件名，完整 workspace 路径仍保留用于发送、移除和悬停提示。`ChapterOutline` 相关组件同步从 `ModeRouter.tsx` 抽离至 `components/workbench/outline/`。
- The Writing outline sidebar now reorganizes navigation for long chapter lists. Pinnable Book Settings remain in the equal-height header while the current chapter outline stays in the scrolling navigation below; users who want it fixed can enable a semantic Current Chapter Outline shortcut in Manage Book Settings, and that shortcut always follows the latest outline. Distinct Pin/Unpin icons replace active highlighting and remove the former 8px layout shift. Latest Chapter and Back to Top now sit beside Pin, all bottom floaters are removed, and collapsed volumes no longer have controls covering their titles. The current outline opens directly by name while older outlines stay under Outline History. Latest Chapter, Reveal, Save, and Editor Settings are consistently presented as icon-only actions with hover tooltips; Reveal opens the outline, expands the volume, and locates the current chapter. Chapters and outlines retain both right-click and hover action menus; pinned Book Settings shortcuts hide the visible More button, center their labels, and keep file operations in the right-click menu. Those operations support Chat references, Project Files reveal, file rename, and confirmed delete. File-reference tokens show only the basename in the Chat composer while retaining the canonical workspace path for sending, removal, and hover disclosure. The `ChapterOutline` components were extracted from `ModeRouter.tsx` into `components/workbench/outline/`.
- 新增内部共享的 Conversation Journal：写作、配置、图片等固定 Agent 会话与游戏故事复用带 CAS、文件租约、generation、checksum 链、fsync、短写处理和不完整尾行恢复的 append-only JSONL 内核；每个会话/故事仍使用独立 `.jsonl + .idx.json`，索引可丢弃并可从规范日志有界内存重建。离线验收现覆盖真实游戏 100,000 逻辑回合、索引重开续写、前缀不变和重建内存边界。
- Added the internal shared Conversation Journal. Writing, Config, Image, and other fixed Agent sessions now reuse the same append-only JSONL kernel as Game stories, with CAS, file leases, generations, checksum chains, fsync, short-write handling, and incomplete-tail recovery. Every session/story still owns a separate `.jsonl + .idx.json`; indexes are disposable and rebuild from the canonical log with bounded retained memory. Offline acceptance now covers 100,000 real Game turns, indexed reopen-and-continue, unchanged prefixes, and rebuild memory bounds.
- 写作及固定 Agent 会话新增真实存储分页、稳定 compaction cursor 和流式完整历史导出；游戏快照与 UI 历史改为有界最近窗口及向上游标分页。两种模式的热追加、最近历史和模型上下文均不再物化完整 JSONL；除 active checkpoint / latest rewind 所需的有界精确模型与规范前缀外，索引不保存可分页的完整历史正文、展示专用 thinking 或工具结果。
- Writing and fixed Agent sessions now provide storage-backed paging, stable compaction cursors, and explicit streaming full-history export; Game snapshots and UI history use a bounded recent window with upward cursor paging. Hot appends, recent history, and model context in both modes no longer materialize the complete JSONL. Except for the bounded exact model and canonical prefixes required by the active checkpoint and latest rewind, indexes do not retain pageable full-history prose, display-only reasoning, or tool results.

- GitHub Release 与 `scripts/build.sh` 产物现在内置固定版本、SHA-256 校验的 ripgrep 15.2.0，覆盖 macOS/Linux 的 ARM64、x64 与 Windows x64；`grep` 优先使用安装目录中的受控二进制并禁用外部 ripgrep 配置，源码开发仍可回退 PATH。更新安装会校验、替换并回滚内置工具及其 MIT/Unlicense 许可文件，用户不再需要为 Release 单独安装 ripgrep。
- GitHub Release archives and `scripts/build.sh` output now bundle pinned, SHA-256-verified ripgrep 15.2.0 binaries for macOS/Linux ARM64 and x64 plus Windows x64. `grep` prefers the controlled executable in the install directory with external ripgrep configuration disabled, while source development retains the PATH fallback. Update installation validates, replaces, and rolls back the runtime tool together with its MIT/Unlicense files, so Release users no longer install ripgrep separately.
- 工作区新增基于 `fsnotify` 的实时文件同步，写作与游戏模式共用同一条全局事件流：递归监听可见目录并按 VS Code 语义合并创建、更新、删除与原子替换，断线或事件溢出时通过 resync 重读权威状态。打开的文件会自动载入外部更新，脏草稿沿用三方合并；磁盘删除后保留编辑器内容、暂停自动保存，手动保存可重新创建文件，图像预览也会自动刷新缓存。
- Workspaces now synchronize filesystem changes in real time through `fsnotify` and one global event stream shared by Writing and Game modes. Visible directories are watched recursively, with VS Code-style coalescing for creates, updates, deletes, and atomic replacements; reconnects and overflow fall back to authoritative resync. Open files reload external updates, dirty drafts keep three-way merge protection, deleted files retain editor content with Auto Save paused until an explicit recreate, and image previews refresh their cache automatically.
- 写作与游戏 Agent 现在会在首轮模型请求前确定性加载用户消息任意位置显式指定的一个或多个 `/<skill-name>`：只解析当前 Agent 已启用的 Skill，按首次出现顺序去重，将完整有界说明注入本轮上下文，并在模型开始思考前复用现有 `skill` 工具卡逐项展示加载结果；未显式指定时仍保留由 Agent 按描述调用 `skill` 的渐进加载方式。
- Writing and Game Agents now deterministically preload one or more explicit `/<skill-name>` references found anywhere in a user message before the first model request. Only Skills enabled for the current Agent are resolved, duplicates keep first-occurrence order, bounded full instructions enter the turn context, and each load is shown through the existing `skill` tool card before model reasoning begins; implicit description-based `skill` calls remain available when no Skill is explicitly selected.
- 所有经共享上下文装配运行的 Agent 现在会在本轮用户请求前收到带来源和预算审计的现实运行环境快照，包括当前日期时间、时区/UTC 偏移和活动 workspace；真实运行与 Context Analysis 共用唯一的纯 turn preparation，只有真实运行会显式提交原始用户消息及引用元数据。该快照不进入 system prompt 或持久历史，并明确不会覆盖写作作品或互动故事中的世界时间。
- Every Agent using the shared context assembly now receives an audited real-world runtime snapshot before the current user request, including the current date/time, time zone/UTC offset, and active workspace. Runtime execution and Context Analysis share one pure turn preparation, while only execution explicitly commits the raw user message and reference metadata. The snapshot is excluded from the system prompt and persisted history, and explicitly cannot override in-story time in Writing or Game modes.
- 新增共享内置 `web-research` Skill：在 Agent 已启用 `web_search` 与 `web_fetch` 时，将开放式网页研究组织为问题界定、多角度检索、正文读取、来源与日期核验、矛盾处理和贴近结论的 URL 引用；搜索摘要仅用于发现来源，网页正文始终按不可信数据处理。
- Added the shared built-in `web-research` Skill. When an Agent has `web_search` and `web_fetch` enabled, it turns open-ended web research into scoped questions, distinct search angles, source reading, source/date verification, contradiction handling, and claim-adjacent URL citations; search snippets are discovery-only and fetched pages remain untrusted data.
- 网页访问工具新增统一的证据引用契约：搜索结果仅用于发现来源，回答使用成功抓取的事实时，会在对应段落或列表项末尾添加 `[来源标题](final_url)`；聊天中的外部 Markdown 链接现在具有明确样式，并会安全地在新标签页打开。
- Web Access tools now share one evidence-citation contract: search results are discovery-only, while claims based on a successful fetch place `[source title](final_url)` at the end of the supporting paragraph or list item. External Markdown links in chat are now visibly styled and open safely in a new tab.
- Agent 新增 `web_fetch`：仅抓取公开 HTTP(S) 页面，先使用带常见 Chrome 导航头的 HTTP 快路径，再默认尝试固定的 Jina Reader；direct/Jina 返回需 JavaScript、访问拒绝、网络错误或 provider unavailable 等可降级结果时，使用隔离的本机 Chrome/Rod fallback，并可针对 challenge 在全新 stealth context 重试。Jina Markdown 直接规范化，HTML 才经过 Readability 与 `html-to-markdown`。该工具支持 Unicode 字符分页、响应/正文配置上限、重定向逐跳校验、私网与云元数据地址阻断，并把外部正文明确标记为不可信来源。设置页新增用户级网页访问配置，可填写 SearXNG 实例、单个搜索服务超时及搜索/抓取上限。
- Agents now include `web_fetch`, which retrieves only public HTTP(S) pages through a Chrome-header HTTP fast path and the default fixed Jina Reader service. Degradable direct/Jina outcomes—such as JavaScript-required pages, access denial, network errors, or provider unavailability—fall back to an isolated installed Chrome/Rod renderer, with a fresh stealth context retry for challenge pages. Jina Markdown is normalized directly, while HTML alone passes through Readability and `html-to-markdown`. The tool supports Unicode-character pagination, configurable response/content limits, redirect-by-redirect validation, private/cloud-metadata address blocking, and explicit untrusted-source labeling. Settings now has user-level Web Access configuration for a SearXNG instance, per-provider search timeout, and search/fetch limits.
- 资料库正文编辑器新增紧凑的 富文本/Raw 切换：默认富文本（所见即所得），可切换为等宽 Markdown 源码编辑，切换资料条目时保留所选模式；Raw 模式下 Cmd/Ctrl+S 仍会触发保存，目录搜索高亮暂仅在富文本模式生效。
- The lore content editor now has a compact Rich text/Raw toggle: rich text (WYSIWYG) stays the default, with an optional monospace Markdown source mode that persists across item selection. Cmd/Ctrl+S still flushes saving in Raw mode; directory search highlighting currently applies in rich mode only.
- 新增可独立复用的 `github.com/alfredxw/denova/agent` Go module：root package 提供与供应商无关的 Message、Model、Tool、Registry、Middleware、Runner、取消/中断、原生 Agentic Loop 与完整外部 Agent Host；`runtime`、`context`、`session`、`tools` 子 package 分别提供持久运行、来源有界的上下文、append-only transcript 和基础工具实现。
- Added the independently reusable `github.com/alfredxw/denova/agent` Go module. Its root package provides provider-neutral messages, models, tools, registries, middleware, runners, cancellation/interruption, a native agentic loop, and a complete external Agent Host; the `runtime`, `context`, `session`, and `tools` subpackages provide durable execution, provenance-bounded context, append-only transcripts, and standard tool implementations.
- Beta 不兼容：原 `github.com/alfredxw/denova/agent/model/openai` 独立 module 已并入公共 `agent` module；`agent/providers` 参考 Pi AI package 改为 provider 与 wire protocol 正交的显式 registry：`builtin` 定义 OpenAI、DeepSeek 和兼容 endpoint，`protocols` 分别实现 Chat Completions 与 Responses。OpenAI 默认 Responses，DeepSeek 默认 Chat Completions，旧 profile 在未显式选择 provider 时继续使用 Chat Completions；设置页新增 provider / API protocol 选择。Responses 使用 `store=false`，将纯 JSON output items（含 encrypted reasoning）放入绑定 provider/protocol/model/endpoint 的版本化 continuation 信封，并随 assistant/tool 序列持久化，在下一轮和进程重启后原样回放；其他 transport telemetry 仍不会进入模型历史。core、registry 与产品 Agent 不依赖模型 SDK，产品 Agent 同时完全移除对 Eino 消息、工具、流与运行时抽象的依赖。
- Beta breaking: the former standalone `github.com/alfredxw/denova/agent/model/openai` module is merged into the public `agent` module. Following Pi's AI-package shape, `agent/providers` now has an explicit registry where provider identity is orthogonal to wire protocol: `builtin` defines OpenAI, DeepSeek, and compatible endpoints, while `protocols` implements Chat Completions and Responses independently. OpenAI defaults to Responses, DeepSeek defaults to Chat Completions, and legacy profiles remain on Chat Completions until a provider is explicitly selected; Settings now exposes provider and API protocol. Responses runs with `store=false`; plain-JSON output items, including encrypted reasoning, are stored in a versioned continuation envelope bound to provider/protocol/model/endpoint and replayed exactly across tool turns and process restarts, while other transport telemetry stays out of model history. Core, registry, and Product Agents remain model-SDK-free, and Product Agents no longer depend on Eino message, tool, stream, or runtime abstractions.
- Beta 不兼容：Agent 模型思考配置删除 `enable_thinking` 与 `reasoning_effort`，统一为单一 `thinking_level`；规范值为 `default`、`off`、`minimal`、`low`、`medium`、`high`、`xhigh`、`max`。`default` 明确省略 provider 思考参数，其他级别由 Chat Completions、Responses 和兼容 endpoint adapter 分别映射；OpenAI Go SDK 同步升级以原生支持 `max`。旧字段不再读取。
- Beta breaking: Agent model thinking configuration removes `enable_thinking` and `reasoning_effort` in favor of one `thinking_level` with canonical values `default`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, and `max`. `default` explicitly omits provider thinking parameters, while Chat Completions, Responses, and compatible-endpoint adapters map every other level to their wire format. The OpenAI Go SDK is upgraded for native `max` support. Legacy fields are no longer read.
- 写作与游戏模式现在复用公共 `agent/runtime`：Denova 将 workspace/session/story/branch 映射到开放且有界的 `BindingRef{Kind, Profile, Key, Labels}`，Runtime 负责 append-only 校验日志、单写 Actor、operation/cycle 状态机、幂等 command ID、精确 target operation、可恢复队列和保守工具崩溃恢复；公开投影不泄露内部工具参数。
- Writing and Game modes now share the public `agent/runtime`. Denova maps workspace/session/story/branch identity into the open, bounded `BindingRef{Kind, Profile, Key, Labels}`, while Runtime owns checksummed append-only journals, single-writer actors, operation/cycle state machines, idempotent command IDs, exact operation targeting, recoverable queues, and conservative tool crash recovery without exposing internal tool arguments.
- `agent/context` 新增显式片段装配器，分别记录 system、turn context、state memory、工具结果和用户消息的来源、用途、位置、hash 与高上限预算；root `agent.ToolDefinition` 将实现、schema 和执行/恢复/结果投影策略绑定为单一注册单元，`agent/tools` 只提供标准实现、workspace adapter 与 definition factory，未知副作用按保守策略恢复。
- `agent/context` adds an explicit fragment assembler that records source, purpose, placement, hash, and high bounded budgets for system, turn-context, state-memory, tool-result, and user-message fragments. Root `agent.ToolDefinition` binds implementation, schema, and execution/recovery/result-projection policy into one registration unit; `agent/tools` now contains only standard implementations, workspace adapters, and definition factories, while unknown effects recover conservatively.
- 写作输入区在 Agent 运行时继续可用，新指令发送后统一作为 Follow Up 进入持久化队列，不再要求用户先选 Follow Up 或 Steer；同一运行可连续追加多条指令并按接收顺序逐条执行，正常排队不再触发冲突提示。队列卡片紧贴输入框展示，并可原子地立即转向、删除或返回编辑（当前页面还会恢复本次发送的引用上下文），Stop 与 Send 保持独立；命令提交、恢复和运行错误统一使用可关闭且自动消失的浮动 Toast，不再写入或叠加在消息流中。Beta 不兼容：流式输入工具栏的发送方式选择器已移除。游戏模式保持严格回合制：运行中可预先编辑下一条输入，但需等当前回合结算后发送，运行态仅提供 Stop；游戏直接命令入口不再接受 Follow Up、Steer 或 Next Turn，底层队列投影仅用于恢复已经持久化的运行。
- The Writing composer remains usable while an Agent runs. New instructions are always accepted into the durable Follow Up queue instead of requiring a Follow Up/Steer choice before sending; multiple instructions can be appended to one run and execute in accepted order without a normal queue-conflict warning. A queue card sits directly above the composer and can atomically steer now, delete, or return the instruction to the editor (restoring its live reference context on the current page), while Stop and Send remain independent. Command submission, recovery, and runtime errors now use closable, auto-expiring floating toasts instead of being written into or accumulated in the message stream. Beta breaking: the streaming delivery selector has been removed from the toolbar. Game mode is strictly turn-based: the next draft remains editable while a turn runs but cannot be sent until settlement, and only Stop is exposed during a run. The direct Game command endpoint no longer accepts Follow Up, Steer, or Next Turn; its underlying queue projection remains only for recovery of already-durable work.

### Changed

- Automation 创建边界现在只接受可编辑 `TaskDefinition`：ID、目录身份、revision、触发状态、运行历史、时间戳与归档状态全部由服务端创建并持有；前端即使误传完整任务对象也只序列化定义字段，配置 Agent 复用同一契约。此为未发布分支内部契约收紧，无需兼容或迁移。
- Automation creation now accepts only the editable `TaskDefinition`. IDs, catalog identity, revisions, trigger state, run history, timestamps, and archive tombstones are created and owned exclusively by the server; the web client serializes only definition fields even if a full task object is passed accidentally, and Config Agent uses the same boundary. This tightens an unreleased internal contract without compatibility or migration work.
- Agent execution Profile 改为按需实现 `QueuedCycleProfile`、`InputProfile`、`DomainCommitProfile` 与 `StructuralProfile`；Director 等 start-only Profile 不再发布空回调或必失败的输入能力，注册时会拒绝无能力及缺少领域提交依赖的非法组合。Writing、Game、AgentChat、Config Manager、Image、Director 与 Automation 的能力矩阵由接口本身穷尽表达。
- Agent execution profiles now implement only the capabilities they support through `QueuedCycleProfile`, `InputProfile`, `DomainCommitProfile`, and `StructuralProfile`. Start-only profiles such as Director no longer publish nil callbacks or input methods designed to fail, while registration rejects identity-only profiles and capability combinations missing domain-commit dependencies. The Writing, Game, AgentChat, Config Manager, Image, Director, and Automation matrix is expressed exhaustively by interfaces.
- Agent 消息展示模型改为以 `role` 判别的联合类型，`MessageItem` 收敛为可穷尽检查的薄分派层，并将内容、元数据、计划、工具、媒体与 Todo 展示按职责拆分；未发布的 `plan_question` 前后端协议已直接移除，规划澄清只使用 Ask。游戏 SSE 统一在单一 Zod 解码边界验证已处理事件，未知、忽略与格式错误事件分别显式分类，单个坏载荷不会再让后续流事件失效；中英文错误提示同步更新，无需新增配置。
- The Agent presentation model is now a `role`-discriminated union. `MessageItem` is reduced to a thin exhaustively checked dispatcher, with content, metadata, plan, tool, media, and Todo rendering split by responsibility. The unreleased `plan_question` frontend/backend protocol is removed directly, leaving Ask as the sole clarification path. Game SSE now validates every handled event through one Zod decoder, explicitly separates unknown, ignored, and malformed events, and isolates one bad payload without losing later stream events. Bilingual diagnostics are updated, with no new setting.
- Agent 产品执行链收敛为唯一的 `internal/agents/execution` 深模块：Writing、Game、AgentChat、Config Manager、Image、Director 与 Automation 在进程启动时按稳定 durable profile 显式注册完整能力，实时排队与冷恢复共用同一个 Cycle 准备入口，输入物化、领域提交对账和结构化恢复不再依赖中央 Agent-kind 分发。首轮只通过 `StartRequest{Cycle, Emit}` 在 durable acceptance 前完成准备，后续命令只携带可持久化语义，`Runner`、Conversation、BookService、Prepare 与 Successor 不再泄漏到公共命令 API；未发布的 Harness/Turn 构造入口、回调选项和兼容包装已直接删除。Journal、API、SSE、模型上下文与用户数据格式不变，无需配置或迁移。
- The product Agent execution path now converges on the single deep `internal/agents/execution` module. Writing, Game, AgentChat, Config Manager, Image, Director, and Automation explicitly register complete capabilities by stable durable profile at process startup; live queued work and cold recovery use the same Cycle preparation path, while input materialization, domain-commit reconciliation, and structural recovery no longer depend on a central Agent-kind dispatch. Initial work prepares before durable acceptance through only `StartRequest{Cycle, Emit}`; later commands carry durable semantics without exposing Runner, Conversation, BookService, Prepare, or Successor. Unreleased Harness/Turn constructors, callback options, and compatibility wrappers are removed directly. Journal, API, SSE, model-context, and user-data formats are unchanged, with no setting or migration required.
- 工作台路由选择、延迟呈现、layout 投影与“首次访问后保持挂载”语义收敛到独立 `WorkbenchRouteHost`；Writing 侧栏、编辑/审阅主区域、共享全屏页面、AgentChat 保活路由与前台 Writing Agent 稳定 Portal 分别由职责明确的 route capsule 持有，`ModeRouter` 只保留跨区域协调和装配，不再维护第二套路由状态或内联页面实现。一级菜单继续由现有 workspace store 显式驱动，共享页面不会切换 Writing/Game，隐藏的 Writing editor、AgentPanel、Game 与 AgentChat 工作台保持存活。无需新增配置。
- Workbench route selection, deferred presentation, layout projection, and retain-after-first-visit behavior now belong to a dedicated `WorkbenchRouteHost`. Focused route capsules separately own the Writing sidebar, editor/review main surface, shared full-workbench pages, retained AgentChat route, and stable foreground Writing Agent portal; `ModeRouter` now keeps only cross-surface coordination and composition instead of a second routing state machine or inline page implementations. Primary navigation remains explicitly driven by the existing workspace store, shared pages never switch Writing/Game mode, and hidden Writing editor, AgentPanel, Game, and AgentChat workbenches stay alive. No new setting is required.
- 工作台 AgentChat 项目侧栏现在默认展示最近 5 个会话，并可通过无背景、左对齐的“展示更多”按 5 个一组继续展开；折叠项目会重置临时展开量，下次展开重新从最近 5 个开始。列表底部不再显示会话总数、额外分隔线或重复的“全部会话”文字入口。项目行不再常驻活动会话数量，改为只在悬停或键盘聚焦时显示进入全部会话的时钟快捷入口；项目、会话与终端的选中态移除左侧亮条，仅保留整行背景反馈，键盘焦点也改用轻背景而不再绘制深浅主题下突兀的外框。写作与游戏模式共用该行为，无需新增配置。
- The Workspace AgentChat project sidebar now shows the five most recent conversations by default and reveals five more at a time through a background-free, left-aligned Show more action; collapsing a Project clears that temporary expansion so reopening starts from five again. The list footer no longer shows a conversation total, extra divider, or duplicate All Conversations text action. Project rows no longer keep an activity count visible and instead reveal an All Conversations clock shortcut only on hover or keyboard focus. Selected Projects, conversations, and terminals no longer use a bright left rail and rely on their row background instead; keyboard focus uses the same quiet fill rather than a conspicuous light/dark outline. Writing and Game share this behavior with no new setting.
- 版本系统改为一次读取工作区即写入 go-git blob，并以单次 index 更新提交精确快照；状态与差异直接比较 Git blob hash，不再重复解压完整 commit 内容。每个新版本将文件统计、变更路径和最近自动版本时间写入 commit 元数据，历史列表通过独立线性 ref 按 limit 提前停止，同时以独立 current ref 保留恢复旧版本后的当前基线；版本说明只读取最多八个实际变更文件，不再逐文件触发全工作区 diff。已发布的旧 commit 仅在进入当前分页时按 Git tree 回退解析。写作与游戏共用的版本/恢复语义不变，无需新增配置。
- Versioning now writes go-git blobs during the single workspace read and commits the exact snapshot through one index update; status and diff compare Git blob hashes without repeatedly decompressing every file in a commit. Each new version embeds file totals, changed paths, and the latest automatic-version time in commit metadata. History follows a dedicated linear ref and stops at the requested limit, while a separate current ref preserves the active baseline after restoring an older snapshot. Version-summary generation reads only up to eight actual changed files instead of triggering a workspace-wide diff per file. Released commits without metadata use a Git-tree fallback only when they enter the requested page. Writing and Game keep the same version/restore behavior, with no new setting.
- 全链路性能边界收敛：macOS 工作区监听改用单个原生 FSEvents 递归流并跳过生成目录，书籍章节统计与章节路径按文件/目录版本增量复用；AgentChat 复用会话元数据目录并以轻量 activity API 跟踪后台任务，不再周期性重扫全部 Project journal；空闲 Agent binding、Project runtime、会话与书籍服务采用安全 LRU 上限，活跃任务和已连接观察者不会被驱逐。Canonical journal 仍在每次事务执行文件同步，但只在创建目录项时同步目录；可重建索引不再重复 fsync，流式展示以 64 KiB 批量落盘。前端流式消息只增量归一化当前尾部，输出期间使用稳定纯文本节点，完成后才解析实际 Markdown。正常 2xx/3xx 请求降为 Debug，文件日志按 16 MiB 轮转、保留 4 个压缩备份并清理旧版无界日志。流式阶段不再实时渲染 Markdown 是 Beta 行为变化；写作与游戏共用该策略，无需新增配置。
- End-to-end performance boundaries are now explicit. macOS workspaces use one native recursive FSEvents stream and skip generated directories; Book chapter summaries and chapter-path projections reuse unchanged file/directory versions. AgentChat shares a session metadata catalog and tracks background work through a lightweight activity API instead of periodically rescanning every Project journal. Idle Agent bindings, Project runtimes, sessions, and Book services use safe LRU bounds, while active work and connected observers are never evicted. Canonical journals still sync every transaction file, but sync the directory only when publishing a new entry; rebuildable indexes avoid duplicate fsyncs, and streamed display text persists in 64 KiB batches. The frontend incrementally normalizes only the growing message tail, keeps one stable plain-text node during streaming, and parses actual Markdown only after completion. Successful 2xx/3xx requests move to Debug, while file logs rotate at 16 MiB, retain four compressed backups, and prune unbounded logs from older versions. Deferring Markdown rendering until stream completion is a Beta behavior change; Writing and Game share the policy with no new setting.
- Beta 不兼容：自动化不再创建独立 Automation Agent 或私有运行页面，而是把每次触发作为普通轮次交给任务所属 Project 的 Writing / General Agent，并在 AgentChat 中持久化完整会话、工具过程与恢复状态。每个任务可选择“每次运行新建会话”（默认，隔离上下文）或“固定复用一个会话”（按顺序跨运行共享上下文）；模型和工具能力直接使用 Project Agent 的有效配置，任务层不再重复配置执行模式、写入范围、输出策略或输出路径，读写与产出要求统一由 Prompt 表达。Automation 仅负责定义、触发、会话策略和运行账本。点击新建会直接在右侧打开未保存草稿，默认归属当前 Project；Project 和起始模板都可在右侧配置中切换，不再经过创建弹窗，只有点击创建后才会持久化任务。左侧目录同时展示全部 Project，支持点击整行 Project 标题展开或折叠，并在每个可用 Project 的任务计数旁提供 `+` 快捷新建入口，直接打开归属该 Project 的草稿。立即运行和历史记录会直接打开对应 AgentChat 会话，原 Automation chat/stream/messages/abort API 与 Agents 设置中的独立 Automation Agent 已移除。已有任务中的四个旧策略字段读取时会被忽略并在后续保存时清除；已有用户级全局任务数据会保留但不再展示、触发、执行或由配置 Agent 创建。Book 与 General Project 均支持该流程，无需新增用户级配置。
- Beta breaking: Automations no longer create a separate Automation Agent or private run page. Every trigger is admitted as a normal turn of the task's owning Project Writing or General Agent, with the complete conversation, tool activity, and recovery state persisted in AgentChat. Each task can use a new conversation per run (the isolated-context default) or serialize runs through one fixed conversation with shared context. Model and tool capabilities use the Project Agent's effective configuration directly; the task layer no longer duplicates execution mode, write scope, output policy, or output path, and the Prompt is the single place to express read, mutation, and output intent. Automation owns only definitions, triggers, conversation strategy, and the run ledger. New opens an unsaved draft directly in the configuration panel, owned by the current Project by default; both Project and starting template can be changed inline without a creation dialog, and the task is persisted only after Create. The left catalog shows every Project, consistently toggles a group from the full Project header row, and places a `+` shortcut beside each available Project's task count to open a draft already owned by that Project. Run Now and historical records open the corresponding AgentChat conversation directly; the former Automation chat/stream/messages/abort APIs and standalone Automation Agent settings are removed. The four legacy task-policy fields are ignored when read and disappear on the next save; existing user-global task data is retained but is no longer shown, triggered, executed, or creatable by Config Manager. The flow supports both Book and General Projects without a new user-level setting.
- 写作区与 AgentChat 共用的文档标签栏改为紧凑圆角样式：未选中标签融入栏背景并用短竖线分组，选中标签才显示独立实色块与轻边框，当前标签的关闭操作保持可见，新建标签改为轻量透明加号；标签同步缩小最小宽度与横向留白，溢出滚动、标题提示、固定及跨栏拖拽行为保持不变。无需新增配置。
- The document tab strip shared by Writing and AgentChat now uses a compact rounded treatment: inactive tabs blend into the strip and are grouped by short dividers, while only the selected tab gains an independent solid fill and subtle border. The active tab's close action stays visible, and New Tab is now a lightweight transparent plus control. Tabs also use a smaller minimum width and tighter horizontal padding, while overflow scrolling, title hints, pinning, and cross-pane dragging remain unchanged. No new setting is required.
- 工作台 AgentChat 对话 Tab 新增自适应居中阅读列：消息、执行过程、变更卡片、排队指令与输入区在宽屏单栏下统一限制为 56rem 并居中，输入框边缘与正文起始线精确对齐；窗口变窄、左右分栏或移动端时自动占满可用宽度。滚动区域仍覆盖完整工作台，Terminal、Files、Reader、Lore 等非对话 Tab 维持原布局。无需新增配置。
- Workspace AgentChat conversation tabs now use one adaptive centered reading column: messages, execution details, change cards, queued commands, and the composer share a 56rem maximum width in wide single-pane layouts, with the composer edge aligned precisely to the text start line; they naturally fill the available width in narrower windows, split panes, and mobile. The scroll surface still spans the full workbench, and non-conversation tabs such as Terminal, Files, Reader, and Lore retain their existing layouts. No new setting is required.
- 写作与游戏共用的项目文件目录不再为已选文件、目录或当前键盘焦点行叠加亮色边框，选中态保留主题高亮背景与文字，未选中的键盘焦点改用轻量背景反馈；拖放目标提示保持不变。无需新增配置。
- The project file explorer shared by Writing and Game no longer stacks a bright outline on selected files, directories, or the current keyboard-focus row. Selection retains the theme-aware highlighted background and text, while unselected keyboard focus uses a quieter background cue; drag-and-drop target feedback remains unchanged. No new setting is required.
- AgentChat 项目侧栏支持通过右键菜单重命名 Project 与已保存会话；Project 右键菜单与“更多操作”保持一致，并将“更多操作”移到新建会话按钮左侧。Project 名称使用更明确的字重；普通会话、临时会话、Terminal 与“全部会话”统一到同一紧凑文字基线，移除前置图标和多余缩进。Terminal 改用等宽文字区分，“全部会话”通过轻分隔线与右侧数量表达汇总层级，状态仍稳定对齐在行尾。无需新增配置。
- The AgentChat project sidebar now supports renaming Projects and saved conversations from context menus. Project context actions match the existing More menu, which now sits to the left of New Conversation. Project names use a stronger weight, while saved conversations, drafts, terminals, and All Conversations share one compact text baseline without leading icons or excessive indentation. Terminals use monospaced text for type distinction, and All Conversations uses a quiet divider plus its trailing count to express the summary level; statuses remain aligned at the row end. No new setting is required.
- AgentChat 页面将全局顶栏的当前书籍切换器替换为独立 Project 切换器，桌面端与移动端复用书籍切换器的紧凑样式；下拉列表展示项目名称、目录与会话数，切换时只更新 AgentChat 的独立标签页工作台，不切换当前书籍或创作模式。写作、游戏及其他页面的书籍切换入口不受影响，无需新增配置。
- The AgentChat page replaces the current-book switcher in the global top bar with its own Project switcher on both desktop and mobile, reusing the Book switcher's compact visual treatment. The menu shows project names, directories, and conversation counts, and switching changes only AgentChat's independent tab workbench without switching the current Book or creation mode. The Book switcher remains unchanged on Writing, Game, and other pages; no new setting is required.
- 默认状态系统不再预置 D&D 风格的六维、攻击 AC、防御 DC、法力、持续效果和冷却，只为主角、重要角色与敌对对象保留最通用的等级和生命。开局状态结构契约现在明确：固定 d20 仅是随机裁定方式，不能作为 D&D 字段依据；凡有开局事实、已读取 Lore 或 TRPG 状态绑定支持，且会独立变化、消耗、触发阈值、参与检定或单独展示的状态，Agent 必须主动创建专用字段。等级与生命不适用时同样必须移除或替换。此变化作用于新故事与未被用户覆盖的内置默认模块；既有故事的冻结 Schema 和用户覆盖保持不变，无需新增配置。
- The default state system no longer predeclares D&D-style six attributes, attack AC, defense DC, mana, ongoing effects, or cooldowns, retaining only general level and health fields for protagonists, important characters, and opponents. The opening schema contract now states that fixed d20 is only a random adjudication method and never evidence for D&D fields; the Agent must proactively create dedicated fields for state supported by opening facts, loaded Lore, or TRPG state bindings when it changes independently, is consumed, crosses thresholds, participates in checks, or needs its own display. Level and health must also be removed or replaced when inapplicable. This affects new stories and untouched built-in defaults; frozen schemas in existing stories and user overrides remain unchanged. No new setting is required.
- 作品目录行内的“确认成章 / 标回初稿”和“更多操作”按钮不再显示悬停提示，仅保留无障碍名称；这些高频且含义明确的动作不再产生不必要的视觉干扰。写作目录中的同类文件操作共用该行为，无需新增配置。
- Removed hover hints from the inline “Confirm as chapter / Mark as initial draft” and “More actions” controls in the Book Outline while retaining their accessible names. These frequent, self-explanatory actions no longer add avoidable visual noise, and the behavior is shared by equivalent Writing outline file actions with no new setting required.
- 书籍设定标题栏的“跳到最新章”与“回到顶部”提示改为稳定悬停约 700ms 后显示，不再移入按钮即弹出；低频的“固定顶部区域”操作移入“管理”弹层并改为开关，未保存偏好时仍默认固定，原有本地偏好继续生效。中英文、深浅主题与窄屏共用该交互，无需新增配置。
- Book Settings header hints for “Jump to latest chapter” and “Back to top” now require a stable hover of about 700ms instead of opening on entry. The infrequent “Keep the top section pinned” action has moved into the Manage popover as a switch; it remains pinned by default when no preference exists, and existing local preferences are preserved. The interaction is shared across Chinese/English, light/dark themes, and narrow layouts with no new setting required.
- 全站鼠标悬停与键盘聚焦提示统一使用主题感知的 Radix/shadcn Tooltip：遗留 DOM、第三方编辑器动态节点与新组件中的原生 `title` 会由根级兼容层自动转换，稳定悬停约 700ms 后才显示；显式 Tooltip 继续沿用各自原有时序，不对所有按钮一刀切。提示支持长文本、换行、碰撞避让、深浅主题及触屏忽略，离开后恢复原属性。写作、游戏和共享工作台共同生效，无需新增配置。
- Standardized hover and keyboard-focus hints across the app on a theme-aware Radix/shadcn Tooltip. A root compatibility layer now converts native `title` hints from legacy DOM, third-party editor nodes, and new components after a stable hover of about 700ms; explicit Tooltips retain their existing timing instead of applying one rule to every button. Long text, line breaks, collision avoidance, light/dark themes, and touch suppression are supported, and original attributes are restored on leave. Writing, Game, and the shared Workbench all inherit the behavior with no new setting required.
- 正文与工作台阅读器右侧的行级评论入口改为单一、无边框的圆角矩形加号按钮，移除评论图标外框、阴影与悬停发亮边框；入口只在鼠标移至对应行右侧窄区域时出现，正文内移动不再持续显示，同时保留原有添加评论交互、双语提示及深浅主题反馈。无需新增配置。
- The manuscript and Workspace Reader line-comment affordance is now a single borderless rounded-rectangle plus button. The redundant comment-icon frame, shadow, and glowing hover border are removed; it appears only in the narrow gutter beside the matching line instead of remaining visible while the pointer moves through the manuscript. Comment creation, bilingual labels, and restrained light/dark-theme feedback remain unchanged. No new setting is required.
- Agent 流式会话现在按每段中间进展分隔执行过程：已完成的 thinking、工具调用、审批与 SubAgent 过程自动折叠，进展正文在运行期间保持可见，当前流式 thinking 继续展开。运行结束后，最终结论前的中间进展和工具记录汇总为一个默认折叠的执行过程，结论后的 post 工具/thinking 则按时序保持在正文之后；写作、游戏、Workspace AgentChat、配置管理与自动化共用该行为，无需新增配置。
- Agent streaming conversations now separate execution disclosures at every intermediate progress segment. Completed thinking, tool calls, approvals, and SubAgent activity collapse automatically while progress prose stays visible during the run and the current streamed thinking remains expanded. Once the run settles, progress and tool history before the terminal answer is aggregated into one collapsed execution process, while post-answer tools/thinking remain chronologically after the prose. Writing, Game, Workspace AgentChat, Config Manager, and Automation share this behavior without a new setting.
- 模型调用新增统一的 provider-neutral `SessionKey`：写作、游戏、AgentChat、自动化与后台模型任务按持久会话生成稳定且不泄露原始标识的缓存亲和键，模型请求快照与上下文压缩 fork 会完整保留。Provider preset 分别映射为 OpenAI `prompt_cache_key`、OpenRouter `X-Session-Id` 和 Fireworks `X-Session-Affinity`；OpenAI 兼容端点可在设置中自定义 Header/JSON Body 字段或明确关闭。
- Model calls now carry one provider-neutral `SessionKey`. Writing, Game, AgentChat, Automation, and standalone model jobs derive stable opaque cache-affinity keys from their durable identity, and model-request snapshots/context-compaction forks preserve them. Provider presets map the key to OpenAI `prompt_cache_key`, OpenRouter `X-Session-Id`, and Fireworks `X-Session-Affinity`; OpenAI-compatible endpoints can customize a header or JSON body field, or explicitly disable the mapping in Settings.
- Agent 上下文总注入上限不再按 Fragment 到达顺序静默截断内容：默认安全上限由 1 MiB 调整为与完整 Provider 输入一致的 4 MiB，超出显式配置时直接拒绝。正常流程仍在 70%/85% 水位提前清理工具结果或压缩历史；若完整请求已达到硬窗口，会优先投影已消费且可恢复的最大工具结果，仍不足时再压缩最早的会话历史；system prompt、stable context、当前用户轮次与本轮动态段不会因总注入预算而被静默裁剪，最终仍超限则由 Provider 输入门禁明确报错。无需新增配置。
- The Agent context assembler no longer silently truncates fragments by arrival order at the total-injection ceiling. The default safety ceiling rises from 1 MiB to 4 MiB to match the complete provider-input byte boundary, and an explicitly configured overflow is rejected. Normal runs still clean tool results or compact history early at the 70%/85% watermarks; if a complete request has already reached the hard window, Denova first projects the largest consumed recoverable tool results and then compacts the oldest conversation history. The system prompt, stable context, current user turn, and turn-scoped dynamic context are never silently selected for truncation by the total-injection budget. The provider input guard reports any remaining overflow. No new setting is required.
- Beta 不兼容：所有 Project 自有能力统一使用 `/api/projects/:project_id/...` 资源边界，由同一个中间件解析稳定 Project ID、校验可用性并持有重定位/归档生命周期租约；覆盖 Files、书籍快照/目录/统计、资料库分类与图片、文本审阅、变更审阅、文件监听、Skills、分层设置、Config Manager、运行追踪、搜索替换、版本、AgentChat 对话、终端创建及会话配置。写作等一级页面先把当前书籍解析为 Project ID，再复用同一页面组件和接口；AgentChat 则始终使用所选 Tab 的 Project ID，不再回退到前台书籍。用户级预设、模型目录等全局资源继续保持全局作用域，自动化则严格归属任务所在 Project 并由其 Agent 执行。旧的无作用域 Project API 已直接移除，不提供 Beta 兼容转发；无需新增配置。
- Beta breaking: every Project-owned capability now uses the `/api/projects/:project_id/...` resource boundary. One middleware resolves stable Project identity, validates availability, and holds the relink/archive lifecycle lease for Files, Book snapshots/trees/summaries, Lore classification and images, document and change review, file events, Skills, layered Settings, Config Manager, run traces, search/replace, versions, AgentChat conversations, terminal creation, and conversation configuration. First-level surfaces such as Writing resolve the foreground Book to a Project ID and reuse the same components and APIs, while AgentChat always uses the Project selected by its tab and never falls back to the foreground Book. User presets, model catalogs, and other truly global resources remain global, while Automations are strictly owned and executed by their task's Project Agent. Former unscoped Project APIs are removed without Beta forwarding routes. No new setting is required.
- 书籍列表迁移为用户级稳定 Project Registry；名称、内容目录与 Project ID 分离，重命名或重定位不会改变身份，归档和重定位会先排空该 Project 的 Agent、Watcher、Terminal、版本调度及文件操作。旧 `books.json` 自动升级并保留版本化备份；旧内容目录中的 Sessions、Config、Changes、Reviews、Runs、Artifacts、Automations 及版本 `.git` 会仅复制到 `project-state/<project_id>/...`，原数据保留为回滚源，迁移收据可幂等升级且并发解析串行化。版本仓库现位于 Project 状态目录，不再占用内容目录的 `.git`，重定位后历史继续跟随 Project。已发布 Automation `workspace_id` 仍可读取并规范写回 `project_id`，冲突双字段会明确拒绝。
- The Book catalog now migrates into a user-level stable Project Registry where display name, content directory, and Project ID are separate. Rename or relink preserves identity, and archive/relink drains that Project's Agents, watchers, terminals, version schedulers, and file operations first. Released `books.json` data upgrades with a versioned backup. Sessions, Config, Changes, Reviews, Runs, Artifacts, Automations, and version `.git` data beneath released content directories are copied into `project-state/<project_id>/...`; sources remain as rollback copies, receipts upgrade idempotently, and concurrent first resolution is serialized. Version repositories now live in Project state instead of occupying content `.git`, so history follows a relink. Released Automation `workspace_id` remains readable and is normalized to canonical `project_id`, while conflicting dual fields are rejected explicitly.
- AgentChat 的通用页面能力按 Project 类型显式声明：Book 支持阅读器、资料库、预设、Skills、Agents、Automations 与版本；General Project 继续支持 Files、终端、对话以及可复用的预设、Skills、Agents、Automations，仅隐藏真正依赖书籍语义的阅读器、资料库、版本和 Review。Tab 持久化以 Project ID 为键并在重定位后刷新内容路径，后台运行与双栏状态不会随前台书籍切换。
- AgentChat page capabilities are now declared explicitly per Project type. Books expose Reader, Lore, Presets, Skills, Agents, Automations, and Versions. General Projects retain Files, terminal, chat, and the reusable Presets, Skills, Agents, and Automations pages, while only Book-semantic Reader, Lore, Versions, and Review are hidden. Persisted tabs are keyed by Project ID and refresh their content path after relink, and background runs and split-pane state no longer follow foreground Book switches.
- Files Tab 与写作页「项目文件」现在复用同一个 Project Explorer、项目级 API、懒加载缓存和逐项目展开偏好，不再维护两套目录树与文件操作协议。交互统一对齐 vscode.dev：移除目录内可见的「项目文件」标题与生成目录开关，生成目录始终参与按需加载；新建文件和目录在当前树位置直接命名，并统一支持剪切/复制/粘贴、路径复制、F2 重命名、Enter 打开、Delete 删除、多选和拖拽移动。写作页仍通过扩展接口保留章节字数/状态、引用到对话、自然章节排序和删除前版本备份，空目录也可直接新建；Skill 编辑器改用独立的轻量只读包目录。Beta 不兼容：前端文件树写操作统一使用 `/api/projects/:project_id/files/operations`，旧 `/api/workspace/{create,delete,rename,copy,move}` 路由已移除。
- The Files tab and Writing's Project Files view now reuse one Project Explorer, project-scoped API, lazy directory cache, and per-project expansion preferences instead of maintaining separate trees and mutation protocols. Interaction consistently follows vscode.dev: the visible in-tree “Project files” heading and generated-folder toggle are removed; generated folders always participate in lazy loading; files and folders are named inline at the current tree location; and cut/copy/paste, path copying, F2 rename, Enter open, Delete, multi-selection, and drag-to-move share one implementation. Writing retains chapter word/status metadata, chat references, natural chapter ordering, and pre-delete version backups through explicit extensions, while empty directories can create items directly; the Skill editor uses a small dedicated read-only package tree. Beta breaking: frontend tree mutations now use `/api/projects/:project_id/files/operations`, and the former `/api/workspace/{create,delete,rename,copy,move}` routes are removed.
- Monaco 深浅主题提高正文、注释、行号与全部语法色的对比度，小字号文本统一达到增强级可读性阈值，降低长时间查看源码和差异内容的视觉疲劳。
- Monaco light and dark themes now increase contrast for body text, comments, line numbers, and every syntax color. Small text consistently meets an enhanced readability threshold to reduce fatigue during extended source and diff review.
- 所有 Monaco 源码编辑器与差异编辑器统一使用 Denova 深浅主题，编辑画布、行号、选区、查找、缩进/括号指引、浮层、滚动条和增删语义色与应用的黑白中性色体系保持一致，同时继承并保留完整语法高亮及编辑能力。Unicode 安全高亮继续检测不可见字符和易混淆字符，但将简体中文、繁体中文与日文语言环境列为允许范围，常用 `zh-hans`、`zh-hant`、`ja` 字符不再误报。
- All Monaco source and diff editors now share Denova light/dark themes across the canvas, line numbers, selections, search, indentation/bracket guides, widgets, scrollbars, and added/removed semantics, while inheriting the full syntax-highlighting and editing feature set. Unicode safety highlighting still detects invisible and confusable characters, but Simplified Chinese, Traditional Chinese, and Japanese locales are allowed so common `zh-hans`, `zh-hant`, and `ja` characters are no longer flagged.
- Beta 不兼容：通用 `grep` 工具由 `pattern`、`paths`、`mode`、大小写、ignore、上下文与 `limit` 等分散字段，收敛为 `command + cursor` 的受控 rg 接口。`command` 使用可复制的 ripgrep 搜索语法并直接执行内置/宿主配置的 rg，不经过 shell；支持安全的正则、大小写、类型、glob、上下文、文件列表、计数参数，以及仅由字面量 `rg` 组成的管道。第一段可搜索显式工作区相对路径，后续段只能过滤上一段输出；重定向、变量/命令替换、非 `rg` 管道段、外部配置、可启动进程或越出工作区的参数会在执行前拒绝，未知参数默认关闭。Denova 仍强制稳定排序、无颜色、无链接跟随、无外部 ignore/config，并保留 `.git` 排除。分页按完整上下文结果块切分，v2 游标同时绑定命令策略与前序结果摘要，可检测搜索期间的前序内容漂移；默认隐藏文件与 `.gitignore` 行为回归原生 rg 语义。结果继续复用现有工作区条目/字节预算，无需新增配置。
- Beta breaking: the general `grep` tool replaces its separate `pattern`, `paths`, `mode`, case, ignore, context, and `limit` fields with a controlled `command + cursor` rg interface. `command` uses copyable ripgrep search syntax and directly invokes the bundled or host-configured rg binary without a shell. Safe regex, case, type, glob, context, file-list, and count flags are supported, together with pipelines made only of literal `rg` stages. The first stage may search explicit workspace-relative paths; later stages may only filter preceding output. Redirects, variable/command substitution, non-`rg` pipeline stages, external configuration, process-spawning flags, and workspace escapes are rejected before execution, while unknown flags fail closed. Denova still enforces stable ordering, color-free output, no symlink following, no external ignore/config, and `.git` exclusion. Pagination preserves complete context result blocks, and v2 cursors bind both the command policy and a digest of preceding results so mutations during paging are detected. Hidden-file and `.gitignore` defaults now follow native rg behavior. Existing workspace entry/byte budgets remain in force, so no setting is added.
- Beta 不兼容：通用 `edit` 工具收敛为单文件原子批量协议 `edit({path, edits[]})`，单点编辑也使用一项列表，不再接受根级 `old_string` / `new_string`。所有替换基于调用时的同一文件快照，区间必须互不重叠，成功时只写入一次；任一项无效时文件保持不变，并一次返回带原始索引、服务端 edit ID、稳定错误码和细节的逐项问题。工具提示、交互导演字符统计与工具卡流式预览已同步支持多项替换；无需新增配置。
- Beta breaking: the general `edit` tool now has one single-file atomic batch contract, `edit({path, edits[]})`; a one-point edit still uses a one-item list, and root-level `old_string` / `new_string` are no longer accepted. Every replacement is evaluated against the same file snapshot captured for the call, ranges must not overlap, and a successful batch writes once. Any invalid item leaves the file unchanged and returns all independently discoverable item issues with original indexes, server-assigned edit IDs, stable codes, and details. Tool guidance, Interactive Director character counts, and streaming tool-card previews now understand every replacement; no setting is added.
- Plan Mode 的交互式澄清统一复用可恢复的 `ask` 工具：问题与回答在同一个 Agent run 内持久化、恢复并继续规划，不再生成 `<plan_questions>` 或隐藏的回答跟进消息。运行时会在模型调用前移除 workspace、配置、外部及业务会话写入工具，并在执行边界再次拒绝伪造的变更调用；只读检索、SubAgent 以及 Ask / Todo / 上下文控制保持可用且子 Agent 继承同一限制。无需新增配置，继续使用现有 Agent Tools 的 Ask 开关。Beta 不兼容：未发布的旧 `plan_question` 展示与提交协议已直接移除，历史载荷不再识别为交互卡片。
- Plan Mode now uses the recoverable `ask` tool for every interactive clarification. Questions and answers persist, recover, and resume within the same Agent run, replacing `<plan_questions>` and hidden answer follow-up messages. Before each model call, the runtime removes workspace, configuration, external, and domain-session mutation tools, then rejects forged mutation calls again at the execution boundary; read-only research, SubAgents, and Ask / Todo / context controls remain available, with child Agents inheriting the same restriction. No new setting is needed—the existing Agent Tools Ask toggle remains authoritative. Beta breaking: the unreleased legacy `plan_question` display and submission protocol is removed directly, and historical payloads are no longer recognized as interactive cards.
- `internal/app` 按应用用例与生命周期进一步收敛：Project 对话、配置管理、自动化、活动摘要、书籍资产与目录、图像、资料库、审阅、上下文压缩、会话准备、分层设置、共享资源目录及 Task admission/replay 均由独立内聚子包直接承接；Teller、导演模块、图片预设、文风参考和用户/工作区 Skills 统一由 `app/resourcecatalog` 管理，Book 列表/排序/元信息统一由 `app/book` 管理，Project 变更在 `app/agentchat` 关闭对应运行时后操作唯一的 `project.Registry`。根 `App` 只保留服务装配、当前前台 Book 的原子运行时协调和明确 Host adapter，小型同生命周期文件已合并；结构测试禁止子包反向导入根包、已下沉前缀回流、根包文件数失控或单文件超过 800 行。HTTP 契约不变；Beta 不兼容：内部 Go 调用方需改用 `AgentChat()`、`ConfigManager()`、`Automation()`、`Activity()`、`BookAssets()`、`Images()`、`Lore()`、`ResourceCatalog()` 与 `SettingsService()` 的直接服务 API。
- `internal/app` is further organized by application use case and lifecycle. Project conversations, Config Manager, Automation, activity summaries, Book assets/catalogs, images, Lore, review, context compaction, conversation preparation, layered Settings, reusable resource catalogs, and Task admission/replay now live in cohesive subpackages reached directly by callers. `app/resourcecatalog` owns Tellers, Director modules, image presets, style references, and user/workspace Skills; `app/book` owns Book listing, ordering, and metadata; Project mutations close their runtime in `app/agentchat` before changing the sole `project.Registry`. The root `App` retains only service composition, atomic coordination of the foreground Book runtime, and explicit Host adapters, with small same-lifecycle files merged. Architecture tests prevent child packages from importing the root, extracted prefixes from returning, unbounded root file growth, and files above 800 lines. HTTP contracts are unchanged. Beta breaking: internal Go callers now use the direct `AgentChat()`, `ConfigManager()`, `Automation()`, `Activity()`, `BookAssets()`, `Images()`, `Lore()`, `ResourceCatalog()`, and `SettingsService()` APIs.
- 后端 package 按职责重新收敛：工作区能力统一到 `internal/workspace/{change,autosave,documentreview,filewatch}`，宿主环境统一到 `internal/hostruntime`，图像生成/预设/创作资产统一到 `internal/image`，叙事模式与参考库统一到 `internal/style`；书籍角色/资料、互动 Teller/Director/状态变更、Agent 上下文/工具结果/配置资源以及 App Task 均成为有真实实现的领域子包，生命周期与 keyed lock 合并为 `internal/concurrency`。`internal/agents` 根包只保留 Agent 构建、Director 组装和稳定消息门面；`execution` 统一拥有产品级 durable admission、排队、恢复与提交边界，`chat` 只执行已准备的模型 Cycle，Director 展示协议归 `interactive`，一次性模型任务归 `modeltask`，Writing Skill 选择归现有 `skills`。依赖固定为 `agents → execution → chat / agent/runtime` 且可复用子包禁止反向导入根包。Beta 不兼容：未发布的 Harness 导入路径与 API 已直接删除，不保留转发兼容包；文件 Agent journal API 从 `runtime.NewFileJournalStore` 移至 `runtime/filejournal.NewStore`。
- Backend packages are organized by responsibility: workspace capabilities live under `internal/workspace/{change,autosave,documentreview,filewatch}`, host environment support under `internal/hostruntime`, image generation/presets/creative assets under `internal/image`, and narrative modes/reference files under `internal/style`. Book character/lore, Interactive Teller/Director/state mutation, Agent context/tool-result/config-resource, and App Task are real domain subpackages, while lifecycle scopes and keyed locks are combined in `internal/concurrency`. The `internal/agents` root retains only Agent builders, Director composition, and the stable message facade; `execution` owns product-level durable admission, queueing, recovery, and commit boundaries, while `chat` only executes a prepared model Cycle. Director display protocol belongs to `interactive`, one-shot model workloads to `modeltask`, and Writing Skill selection to `skills`. The enforced direction is `agents → execution → chat / agent/runtime`, with reusable subpackages forbidden from importing the composition root. Beta breaking: unreleased Harness imports and APIs are deleted directly without forwarding packages, and the file-backed Agent journal constructor moves from `runtime.NewFileJournalStore` to `runtime/filejournal.NewStore`.
- 内置 Actor 状态预设的「好感度」改为 `-100` 至 `100`、默认 `0`：负值表示敌意，正值表示好感；关系类型与关系阶段按朋友、恋爱、亲属、师徒等具体关系分别表达，不再由好感度自动推导。默认预设的等级、六维、攻击 AC 与防御 DC 移除硬上限；开局状态适配会逐项审查这些继承字段，并可显式保留、替换或删除，避免非 D20 故事沿用默认 `10`。故事状态与导演面板中的正负数值改为以 `0` 为中心展示。Beta 行为变化：未覆盖的内置状态预设会刷新；内置覆盖与已经冻结开局结构的故事保持不变。
- Built-in Actor State favorability now ranges from `-100` to `100` with a default of `0`: negative values represent hostility and positive values represent affinity. Relationship types and stages are expressed independently for friendship, romance, family, mentorship, and other story-specific relationships rather than inferred from favorability. The default preset no longer hard-caps level, the six attributes, attack AC, or defense DC; opening adaptation audits each inherited field and can explicitly keep, replace, or remove it so non-D20 stories do not inherit a factual default of `10`. Signed values are rendered around zero in Story State and Director views. Beta behavior change: untouched built-in state presets refresh, while explicit built-in overrides and stories with a frozen opening schema remain unchanged.
- 写作、游戏、配置管理与自动化共用的输入框斜杠命令 / Skills 选择器改为紧凑单行布局：名称、说明和用途保持同一行，长文本以省略号收敛，弹层宽度自适应并同时匹配深色与浅色主题。
- The shared slash-command / Skills picker used by Writing, Game, Config Manager, and Automations now uses a compact single-line layout. Names, descriptions, and intent stay on one row, long text truncates cleanly, and the adaptive panel matches both dark and light themes.
- 本地文件持久化原语收敛到内聚的 `internal/localfs`：目录同步与文件租约不再分散为平行小 package；Agent File Journal 的租约实现则由 `agent/runtime/filejournal` 自身封装。两边统一复用成熟的跨平台文件锁依赖，在保留可取消等待、稳定路径身份和幂等释放语义的同时补齐 Windows 跨进程保护。
- Local-file persistence primitives now converge in the cohesive `internal/localfs` package instead of separate directory-sync and file-lease packages, while `agent/runtime/filejournal` encapsulates the Agent File Journal lease implementation. Both reuse a mature cross-platform file-lock dependency, preserving cancellable waits, stable path identity, and idempotent release while adding real Windows inter-process protection.
- 修复 Agent 工具描述前端类型误用 `context_retention`、与后端 `result_retention` JSON 契约不一致的问题，并新增由 Go JSON 标签校验前端字段集合的跨语言门禁。写作 Session、游戏 Story 与结构命令恢复计划现在复用同一个 storage-neutral `CompactionCheckpoint`：既有扁平日志格式保持可读，同时完整保留 provider token 校准、触发原因、恢复带和候选身份；执行/cache 归因继续只进入 run ledger，不膨胀模型上下文 checkpoint。
- Fixed the frontend Agent tool descriptor using `context_retention` instead of the backend `result_retention` JSON contract, with a cross-language gate that checks the frontend field set against Go JSON tags. Writing Sessions, Game Stories, and structural-command restore plans now share one storage-neutral `CompactionCheckpoint`: existing flat journals remain readable while provider token calibration, trigger reason, recovery band, and candidate identity round-trip completely; execution/cache attribution remains in the run ledger instead of inflating model-context checkpoints.
- Beta 不兼容：Agent 上下文配置收敛为 `compaction_enabled`、`compaction_threshold`、`tool_result_context_enabled` 与五项注入硬上限；删除旧 `tool_result_retention_enabled` 及 cleanup 水位、保护窗口、恢复带、失败熔断和 checkpoint 形态等低层配置，不提供兼容读取。后端现在通过 `resolved_agent_contexts` 返回各 Agent 的规范终态，前端不再重复默认值、继承与关联参数归一化；缓存安全清理、恢复余量和失败保护作为一套后端策略统一派生。
- Beta breaking: Agent context configuration is reduced to `compaction_enabled`, `compaction_threshold`, `tool_result_context_enabled`, and five injection hard limits. The former `tool_result_retention_enabled` plus low-level cleanup watermarks, protection windows, recovery bands, failure fuses, and checkpoint-shape settings are removed without compatibility reads. The backend now returns canonical per-Agent state through `resolved_agent_contexts`; the frontend no longer duplicates defaults, inheritance, or coupled normalization, while cache-safe cleanup, recovery headroom, and failure protection are derived as one backend policy.
- 工具结果清理只接受经当前 run artifact store 校验的 complete-output receipt；空 purpose 的旧路径/哈希形状不再获得恢复权限。写作 Session 与游戏 Journal 的清理记录规范化、幂等比较及压缩健康状态机现复用 `internal/agents/context` 纯领域实现，各自仍独立负责 CAS、时间戳和持久化格式。
- Tool-result cleanup now accepts only complete-output receipts verified by the current run artifact store; legacy empty-purpose path/hash shapes no longer gain recovery authority. Writing Sessions and Game Journals now share the storage-neutral `internal/agents/context` rules for cleanup normalization, idempotency, and compaction health transitions while retaining their own CAS, timestamps, and persistence formats.
- 写作与游戏工作区、Agent Chat、Agents / Skills 配置、Skill 文件树和变更审阅文件导航的侧栏开关统一为短促的基础动效：侧栏宽度与主内容布局连续收拢或展开，内容轻微淡出且保持挂载，从而保留用户调整的宽度与面板内部状态；移动端抽屉补齐打开与关闭滑动，并遵循现有的完整、减弱和关闭动效偏好。
- Sidebar toggles across Writing and Game workspaces, Agent Chat, Agents / Skills configuration, the Skill file tree, and Change Review now share one crisp base motion. Pane width and the main layout contract or expand continuously, content exits subtly while staying mounted to preserve user sizing and local state, and mobile drawers now slide both in and out. Existing Full, Reduced, and Off motion preferences are respected.
- Lore 批量读取不再设置单次 16 条、单次正文大小或任务累计正文大小的专属拒绝上限，资料目录的显式 `limit` 与关键词数量也不再被工具层任意封顶。`read_lore_items` 在部分 ID / 唯一名称不存在时会正常返回已命中的完整资料并附带未找到清单，只有整批均未命中时才报错；进入模型上下文的工具结果仍由统一、可配置的高上限投影边界保护。
- Lore batch reads no longer impose Lore-specific rejection caps on 16 items, per-call body size, or cumulative body size, and explicit catalogue limits and keyword counts are no longer arbitrarily capped by the tool. `read_lore_items` now returns every matched full entry plus a missing-item list when only some IDs or unique names are stale, and errors only when the entire batch misses; model-context projection remains protected by the shared configurable high ceiling.
- Beta 不兼容：`config_read` 的 `list` / `get` 统一改为带 `items`、状态、总数和 `next_cursor` 的分页信封，不再限制 32 个 ID。精确读取会逐项返回成功项、`missing_ids` 与非缺失故障，部分失败不再拖垮整批，只有完整请求一个成功项也没有时才报错；目录游标会绑定查询和目录版本，配置变化后明确要求重新分页。`config_apply` 改为返回小型持久化回执，不再回显可能超出共享预算的完整配置，规范值按既有流程用 `get` 验证。
- Beta breaking: `config_read` `list` and `get` now use one paginated envelope with `items`, status, totals, and `next_cursor`, and exact reads no longer cap requests at 32 IDs. Existing items, `missing_ids`, and non-missing failures are reported independently so partial failure does not discard the batch; only a completed request with no successful item errors. Catalog cursors bind both the query and catalog revision and explicitly require a restart after configuration changes. `config_apply` now returns a compact persistence receipt instead of echoing a potentially over-budget complete resource; the canonical value is verified with `get` as before.
- `search_story_history` 不再静默丢弃第 9 个关键词或把结果上限改写为 12；`match=all` 会使用全部关键词，结果按共享工具字节预算返回稳定游标分页，并冻结首个历史头以免续页混入新回合。游戏配置归一化不再截断事件卡、规则模板与绑定、状态模板/字段/初始 Actor/词条池等集合；必须保留的大小约束会在写盘前带字段路径拒绝，而不是截断后保存成功。
- `search_story_history` no longer silently drops the ninth keyword or rewrites the result limit to 12. `match=all` evaluates every keyword, byte-budgeted pages return a stable cursor, and the first history head is frozen so later pages cannot absorb newly appended turns. Game configuration normalization no longer truncates event cards, rule templates or bindings, state templates/fields/initial Actors, trait pools, or related collections; retained size constraints reject with a field path before persistence instead of saving truncated data.
- Event Card `read` 不再按 Director Run 累计限制 8 张卡；`initialize_story_state_schema` 不再在 16 项时整批拒绝，而是继续逐项返回 accepted/rejected/blocked；Browser `select` 也不再限制 16 个值。这些调用仍受冻结资源权限、共享结果预算及既有浏览器资源边界保护。
- Event Card `read` no longer caps a Director Run at eight cards; `initialize_story_state_schema` no longer rejects an entire batch above 16 items and instead continues reporting accepted/rejected/blocked per item; Browser `select` no longer caps the value list at 16. Frozen-resource authorization, shared result budgets, and existing browser resource boundaries still apply.
- 通用 `read` 在单行超过结果预算时新增 `byte_offset` / `next_byte_offset` 行内续读；`grep` / `glob` 不再限制 256 个请求路径或 10,000 条游标窗口，`glob` 新增绑定查询的 `next_cursor`，两者继续对部分不存在路径返回 warnings。
- Generic `read` now returns `byte_offset` / `next_byte_offset` continuation when one line exceeds the result budget. `grep` / `glob` no longer cap requests at 256 paths or reject cursor windows above 10,000 entries, `glob` adds a query-bound `next_cursor`, and both continue reporting partially missing paths as warnings.
- Agent Chat 左侧栏改为跨项目的活动工作导航，只展示已打开的对话、终端和仍在运行的后台会话，并标明运行、连接、退出与异常状态；完整历史迁移到独立的紧凑主从浏览器，左侧可折叠项目导航默认将当前项目置顶，右侧按所选项目搜索和分页展示会话，点击活动项或历史项会直接恢复并聚焦对应工作台标签。展开的项目保持中性，仅当前会话使用整行选中底色；折叠的当前项目改用弱侧线提示，避免项目与会话的父子层级黏连。
- The Agent Chat sidebar is now a cross-project active-work navigator for open conversations, terminals, and detached running sessions, with explicit runtime states. Full history moves to a compact master-detail browser: a collapsible project navigator places the current project first, while the session list searches and paginates within the selected project. Selecting an activity or history result restores and focuses its exact workbench tab. Expanded projects remain neutral so only the focused activity owns a full-row selection; a collapsed current project uses a subtle side marker instead, keeping parent and child hierarchy visually distinct.
- 独立 Lore Tab 的资料编辑器补齐删除入口，复用资料库的应用内危险操作确认；删除前会先保存当前草稿，成功后自动切换到下一条资料，避免延迟自动保存重新写回已删除条目。
- The standalone Lore tab now exposes entry deletion with the library's in-app destructive-action confirmation. It saves the current draft before deletion and selects the next entry afterward, preventing delayed autosave from recreating deleted lore.
- 工作台变更审阅移除与外层真实标签重复的内部「审阅」标签行，关闭操作统一由工作台标签承担；共享标签栏的新增按钮在标签溢出前后都保持垂直居中。
- Workspace change review no longer renders an internal Review tab row that duplicated the real workbench tab; closing is now owned by the workbench tab. The shared tab-strip add button remains vertically centered before and after tab overflow.
- 可拖拽排序的侧边列表不再用抓取手型强调低频排序操作；资源目录及工作台项目/会话行在可拖拽和拖动过程中均保持默认箭头光标，拖拽能力与独立拖拽把手的反馈不变。
- Draggable sidebar lists no longer use a grab cursor to emphasize low-frequency sorting. Resource-directory rows and Workspace project/conversation rows keep the default arrow before and during dragging, while drag behavior and dedicated-handle feedback remain unchanged.
- Light mode 重新整理为严格的中性灰阶：内容画布、顶部栏、一级导航、侧栏、聊天工作区、控件与选中态使用明确且 RGB 等通道的分层色板，移除结构性表面和阴影中的暖灰/蓝灰偏色；写作与游戏共用界面、预设工作区、导演台、终端及分支时间线同步采用该体系，彩色仅保留给成功、警告、错误和终端 ANSI 等语义状态。
- Light mode now uses a strict neutral grayscale hierarchy. Content canvases, the top bar, primary navigation, sidebars, chat workspaces, controls, and selected states have distinct equal-channel RGB surfaces, while warm/cool tints are removed from structural layers and shadows. Shared Writing/Game UI, the preset workspace, Director console, terminal, and branch timeline follow the same system; color remains only for semantic status and terminal ANSI output.
- 写作工作区的活动标签现在沿用与 AgentChat 一致的明确底色和顶部状态线；文件与设定标签支持右键 Pin / 取消 Pin，固定后移到标签栏前部、显示固定标记、不会被标签数量上限的 LRU 策略淘汰，偏好按作品持久化。重命名或移动已打开文件时会保留固定状态。
- Active Writing tabs now use the same explicit surface color and top status line as AgentChat. File and Lore tabs support Pin/Unpin from the context menu; pinned tabs move to the front, show a pin marker, survive LRU enforcement of the configured tab limit, and persist per book. Renaming or moving an open file preserves its pinned state.
- 工作台终端标签会使用内置启动配置名作为初始标题，并跟随前台程序通过标准 OSC 0/2 发出的窗口标题变化；Claude Code、Codex、Vim 与 Neovim 的装饰性窗口标题会收敛为简洁程序名，用户手动重命名仍具有最高优先级。
- Workspace terminal tabs now start with their built-in launch profile name and follow standard OSC 0/2 window-title updates emitted by the foreground program. Decorative Claude Code, Codex, Vim, and Neovim window titles are reduced to concise program names, while an explicit user rename still has the highest priority.
- 工作台项目/会话侧栏补齐用户级导航偏好：项目主体整行均可展开或折叠，项目和会话都可 Pin，并可按最近更新、最近打开或手动顺序排列；手动排序直接长按对应行拖拽，不显示额外手柄，顺序与打开时间仅保存在本机 UI 状态。写作区和 AgentChat 现在复用同一个文档标签栏：标签默认保持统一最大宽度，空间不足时会等比收缩至统一最小宽度，之后才进入隐藏原生滚动条的横向滚动状态，并自动保证活动标签可见；仅当标题实际被截断时，悬停约半秒才会向下显示全文。AgentChat 的新建标签按钮调整为带四周间距的紧凑圆角矩形。
- The Workspace project/conversation sidebar now has user-level navigation preferences: the full project body expands or collapses, projects and conversations can both be pinned, and the tree can sort by last update, last open, or manual order. Manual ordering uses a direct press-and-hold drag on each row with no separate handle; order and open timestamps remain local UI state. Writing and AgentChat now reuse one document-tab strip: tabs stay at one shared maximum width by default, shrink uniformly to a shared minimum as space tightens, and only then enter horizontal scrolling with the native scrollbar hidden and the active tab kept visible. The full title appears below only after an actually clipped label has been hovered for about half a second. AgentChat's New Tab control is a compact rounded rectangle with breathing room on every side.
- 工作台 AgentChat 的资料库页签现在复用写作工作区的聚焦 Lore 目录与编辑器，不再嵌入一级资料库页面；阅读器页签复用写作页的完整作品目录与 Markdown 编辑器，并在窄屏收进可唤出的目录抽屉。两类页签均支持正文选区/行级评论，待处理意见可从所属项目对话直接定位到对应页签并随下一轮提交；关闭页签、重定位或归档项目前会先保存草稿，所有读写始终由页签所属 Project ID 定位，不会切换前台书籍或模式。
- Workspace AgentChat Lore tabs now reuse the focused Writing Lore directory and editor instead of embedding the top-level library. Reader tabs reuse the full Writing outline and Markdown editor, with an on-demand outline drawer on narrow screens. Both tabs support selection/line comments; pending feedback can open the matching tab from its project conversation and travel with the next turn. Drafts flush before tab close, relink, or archive, and every read or write remains bound to the tab's Project ID without switching the foreground Book or mode.
- Beta 不兼容：Lore 条目更新统一为全量 `PUT /api/lore/items/:id`，移除语义不一致的 `PATCH` 路由。前端更新必须提交完整的可编辑字段快照，后端会在写入前拒绝缺字段或正文 ID 与路径不一致的请求，避免省略字段被零值清空。
- Beta breaking: Lore item updates now use the single full-replacement contract `PUT /api/lore/items/:id`; the semantically inconsistent `PATCH` route is removed. Clients must submit a complete snapshot of editable fields, and the backend rejects missing fields or a body ID that conflicts with the resource path before mutation, preventing omitted fields from being cleared by zero values.
- 书籍资料库的单一权威文件从隐藏目录迁至用户可见的 `setting/lore/items.json`，仍保持一个 `items.json` 集中管理全部条目且 JSON 格式不变；打开该文件会直接进入 Lore Tab。工作区初始化会按现有数据优先级读取旧 `.denova/lore/items.json` / `.nova/lore/items.json` 并原子写入新位置，旧文件保留为可恢复副本，后续不再双写。用户或外部工具仍可按普通文件管理它；LoreStore 在加载和保存时校验 JSON 版本、结构、必需身份及重复 ID/名称，发现无效外部改动时明确报错且不会覆盖原文件；资料库继续随整书版本历史恢复。
- The book Lore library now has one user-visible authoritative file at `setting/lore/items.json`, while retaining the existing single-file JSON format for all entries. Opening that file routes directly to the Lore tab. Workspace initialization reads legacy `.denova/lore/items.json` / `.nova/lore/items.json` by existing data precedence and atomically writes the public copy; legacy files remain recoverable backups and receive no further writes. Users and external tools can still manage it as a normal file. LoreStore validates the JSON version, shape, required identities, and duplicate IDs/names when loading and saving, reports invalid external edits without overwriting them, and remains part of whole-book version restore.
- 书籍级 Skills 现在以根目录 `skills/<skill>/SKILL.md` 及同目录 supporting files 为权威位置，写作和游戏 Agent 共用；首次解析旧工作区时会把 `.denova/skills` / `.nova/skills` 中缺失的 bundles 原子复制到根目录，根目录内容优先，旧目录保留为备份。此变更让用户可直接查看、版本管理和维护书籍自己的 Skills，用户级与内置 Skills 的作用域不变。
- Book-scoped Skills now use the public `skills/<skill>/SKILL.md` bundle layout at the book root, shared by Writing and Game agents. On first resolution, missing bundles from `.denova/skills` / `.nova/skills` are atomically copied into the root directory, public bundles win conflicts, and legacy trees remain as backups. Users can therefore inspect, version, and maintain book-owned Skills directly; user and built-in Skill scopes are unchanged.

### Changed

- 语言模型 Profile 不再向用户暴露 `max_output_tokens` 硬上限；已有值会在下一次设置保存时自动清理，不再继承或限制其他模型。OpenAI 兼容协议在模型未知时继续省略该参数并使用服务商行为，已知模型由共享 Provider Registry 使用官方能力上限，Anthropic Messages 等必填协议仍由适配器提供安全的模型/协议级值；测试探测等明确有界的内部请求保持独立限额。写作与游戏共用同一策略，无需新增配置。
- Language-model Profiles no longer expose the `max_output_tokens` request ceiling. Existing values are removed on the next settings write and no longer inherit into or constrain other models. OpenAI-compatible protocols continue omitting the parameter for unknown models, known models use their documented capability through the shared Provider Registry, and required protocols such as Anthropic Messages retain safe model/protocol-owned values; explicitly bounded internal requests such as connection probes keep their local limits. Writing and Game share the same policy, with no new setting required.

### Fixed

- 修复 Agent Runtime 在容量淘汰忙碌 Binding 与显式关闭并发时可能错误报告关闭成功的问题；显式关闭现在会等待空闲淘汰探测完成并建立权威关闭屏障，确保 Actor 与 Journal lease 已真正释放。无需新增配置。
- Fixed Agent Runtime incorrectly reporting success when explicit binding close raced with capacity eviction of a busy binding. Explicit close now waits for the idle-eviction probe and then establishes an authoritative close barrier, ensuring the actor and journal lease are actually released. No new setting is required.
- 修复 Unix PTY 进程组回归测试在 Linux CI 上引用不可用 `syscall.Getsid` 而无法编译的问题；测试统一使用现有 `x/sys/unix` 平台 API，macOS 与 Linux 共用相同验证。
- Fixed the Unix PTY process-group regression test failing to compile on Linux CI because `syscall.Getsid` is unavailable there. The test now uses the existing `x/sys/unix` platform APIs on both macOS and Linux.
- 修复游戏状态表首次从默认页签切换到较矮页签时，消息列表因 Footer 高度骤减而把垂直滚动位置钳制到新的底部、返回原页签也无法恢复的问题。状态表现在以当前回合内已展示过的最大高度稳定消息尾部几何，并在页签、展开或折叠交互触发布局变化后恢复交互前的滚动位置；隐藏页签仍保持挂载和内部状态，写作及其他共享消息列表行为不变。无需新增配置。
- Fixed the Game state ledger clamping the message list to a new bottom when its first switch from the default tab revealed a shorter panel, leaving the previous vertical position unrecoverable after switching back. The ledger now stabilizes the message-footer geometry to the tallest state shown in the current turn and restores the pre-interaction scroll position after tab, expand, or collapse layout changes; hidden tabs remain mounted with their local state, while Writing and other shared message-list behavior is unchanged. No new setting is required.
- 修复 Agent 对话流式输出一次新增多行时，文字可能先出现在输入区上方的下一行、随后整体上抬造成抖动，以及首批多行内容或 transport 阶段切换后偶发断开底部锁定的问题。写作、游戏、工作台 AgentChat、配置管理及 SubAgent 会话现在共用按视口自适应的回复生长区：新回合先保留约三分之一前文、让输出在其余空间内自然向下生长，填满后才接管滚动；滚动校正改为维持绝对可见底部，并在 Virtuoso 延迟发布完整高度后继续应用未完成的差值，不再把被浏览器截断的多行补偿误记为完成。底部锁定只会被明确的用户上滚意图解除，并覆盖整个活跃执行阶段；短回复结束时仅保留尚未消费的空白，避免完成态跳动。无需新增配置。
- Fixed streamed Agent responses briefly rendering one or more new lines below the intended viewport position before the whole block jumped upward, as well as bottom following occasionally disconnecting after a multiline first batch or a transport-phase transition. Writing, Game, Workspace AgentChat, Config Manager, and SubAgent sessions now share a viewport-adaptive response runway: a new turn keeps roughly one third of the viewport as context and grows naturally through the remaining room before scroll following takes over. Scroll correction now preserves an absolute visible-bottom invariant and retries after Virtuoso publishes deferred total-height changes, so browser-clamped multiline compensation is never treated as complete. Only explicit upward user intent releases the bottom lock, which now spans the full active execution; short replies retain only their unconsumed gutter at completion to avoid a final-state jump. No new setting is added.
- 修复通用 `grep` 工具无法执行 `rg ... | rg ...` 二次过滤的问题：现在支持仅由字面量 `rg` 命令组成的安全管道，各段通过进程标准流直接连接而不经过 shell；第一段仍限制为工作区相对路径，后续段只能过滤上一段输出，重定向、替换、非 `rg` 管道段及越界路径继续在执行前拒绝。写作、游戏、AgentChat、配置管理与自动化共用该能力，无需新增配置。
- Fixed the general `grep` tool rejecting `rg ... | rg ...` secondary filtering. It now accepts safe pipelines made exclusively of literal `rg` commands and connects stages directly through process streams without invoking a shell. The first stage remains confined to workspace-relative paths, later stages may only filter preceding output, and redirects, substitutions, non-`rg` stages, and escaping paths are still rejected before execution. Writing, Game, AgentChat, Config Manager, and Automations share the capability with no new setting.
- 修复游戏模式手动生成互动图像失败时因持久化回合过滤掉临时错误消息而表现为“按钮无反应”的问题；生成期间目标回合持续显示 loading，并禁用其他回合的重复出图请求，失败和不可生成原因改为中英文 Toast 明确提示，不再依赖容易被过滤的消息流错误项。无需新增配置。
- Fixed manual interactive-image generation in Game appearing to do nothing when persisted-turn projection filtered out its temporary error message. The target turn now keeps a visible loading state while generation is active, other turns reject duplicate image requests, and failures or unavailable targets surface through bilingual toasts instead of a filterable message-stream error. No new setting is required.
- 修复 Image Agent 在固定 `image-agent` journal 尚未创建时先接收 durable 命令，导致首次生成或冷恢复报“会话不存在”的问题；journal 现在会在 execution admission/recovery 前创建并显式绑定，但只负责持久化，不会冻结或覆盖 Agents 中当前选择的模型。生成结果通过统一的幂等 cycle commit 写入，不再在运行结束后重复追加整组消息。写作与游戏共用该修复，无需新增配置。
- Fixed Image Agent accepting a durable command before its fixed `image-agent` journal existed, which made first-time generation or cold recovery fail with “session does not exist.” The journal is now created and explicitly bound before execution admission/recovery, but remains persistence-only and cannot freeze or override the model currently selected in Agents. Generated output uses the shared idempotent cycle commit instead of appending the full message pair again after completion. Writing and Game share the fix, with no new setting required.
- 修复资料库、正文与工作台阅读器的行级评论入口从右侧 gutter 移入时不出现、必须先进入正文的问题；共享 TipTap 行定位现在将 gutter 坐标约束到实际文本内容区，不再因编辑器右内边距与外层滚动条相邻而形成不可触发区域。无需新增配置。
- Fixed line-comment quick actions in Lore, manuscripts, and Workspace Reader not appearing when approached from the right gutter until the pointer entered the text. Shared TipTap row targeting now constrains gutter coordinates to the actual text content box, removing the dead hover zone created where editor padding meets an adjacent scrollbar. No new setting is required.
- 修复 Project 资源作用域重构后 AgentChat 的资料库 Tab 再次嵌入一级资料库页面、与写作页“设定”Tab 交互不一致的问题；AgentChat 现在恢复复用同一聚焦 Lore 目录与编辑器，同时继续按 Tab 所属 Project ID 处理刷新、评论导航和草稿保存，不会切换前台书籍或模式。无需新增配置。
- Fixed the AgentChat Lore tab embedding the top-level library again after the Project resource-scope refactor and diverging from Writing's Lore tab. AgentChat now reuses the same focused Lore directory and editor while keeping refreshes, review navigation, and draft flushing bound to the tab's Project ID without switching the foreground Book or mode. No new setting is required.
- 修复游戏与写作在模型按字符高速流式输出时，每个 delta 都携带完整 SSE 元数据并触发 React 提交，导致长 thinking 回合出现主线程卡顿、输入与滚动响应变慢的问题。共享传输层现在只在短暂显示窗口内合并来源一致的相邻文本/思考/工具参数 delta，浏览器再按统一渲染预算提交有序更新；工具、规则判定、上下文压缩与完成边界仍立即刷新，所有文本、thinking、工具参数、最终游标和模型上下文保持逐字等价，不做裁剪。无需新增配置。
- Fixed Game and Writing streams committing React state and repeating the complete SSE envelope for every character-level model delta, which caused long-thinking turns to block the main thread and degrade typing and scrolling. The shared transport now combines only adjacent text, reasoning, and tool-argument deltas from the same source within a short display window, while the browser applies ordered updates on one render budget. Tool, rule-resolution, context-compaction, and completion boundaries still flush immediately; text, reasoning, tool arguments, final cursors, and model context remain byte-for-byte complete with no trimming. No new setting is required.
- 修复后台 Director 在进程退出后遗留 `running` / 已开始的 `waiting_opening` 状态时，游戏页每秒重复下载完整故事快照和分支列表且永不结束的问题。Project Runtime 启动时现在按分支核对无主运行，将其明确标记为失败并保留已有规划文档与可用状态；正常运行期间只轮询轻量 Director 状态，确需等待状态落盘时也不再重复读取分支列表。无需新增配置。
- Fixed orphaned `running` or already-started `waiting_opening` Director state surviving a process exit and making the Game page download a complete story snapshot and branch list every second forever. Project Runtime startup now reconciles unowned runs per branch, marks them explicitly failed, and preserves existing plan documents and usable state; live runs poll only the lightweight Director status, and state-settlement polling no longer reloads branch lists. No new setting is required.
- 修复游戏开局状态结构工具因模型在 remove / ignored 审查中只提供 `reason`、或为非布尔字段误带布尔 `default` 而整体拒绝并重跑的问题。开局专用适配会把非空 reason 复用为缺失 requirement，并移除不可能合法的跨类型布尔默认标记；其余字段、类型、来源、依赖与逐项 accepted / rejected / blocked 校验保持严格，不会伪造 Actor 初始值或绕过最终状态提交。无需新增配置。
- Fixed Game opening-state schema batches being rejected and regenerated wholesale when the model supplied only `reason` for a remove/ignored review or attached a boolean `default` marker to a non-boolean field. The opening-specific adapter reuses a non-empty reason for the omitted requirement and removes an impossible cross-type boolean default marker; all other field, type, source, dependency, and per-item accepted/rejected/blocked validation remains strict, without inventing Actor values or bypassing final state submission. No new setting is required.
- 修复游戏页与开局预设编辑器把缺失的可选书籍开局配置当作 HTTP 404，并在开发模式重复请求、污染控制台的问题。Project Files 新增显式可选读取语义：路径与 Project 边界仍严格校验，文件缺失以正常 `found=false` 数据返回；并发读取只共享进行中的请求而不缓存内容，因此新建或外部修改仍会立即生效。无需新增配置。
- Fixed the Game page and opening-preset editor treating an absent optional Book opening configuration as HTTP 404, causing duplicate development requests and console noise. Project Files now has explicit optional-read semantics: path and Project boundaries remain strict, while absence returns normal `found=false` data. Concurrent consumers share only an in-flight request without caching content, so later creation or external edits remain immediately visible. No new setting is required.
- 已结算的游戏回合不再把工具协议消息中的同一篇正文重复注入下一轮，并会折叠旧 Beta 日志中重复的已解决输入；有界日志投影同时记录尚未消费的玩家输入 ID，避免旧输入在消费回合离开近期页后被误判为待处理并重复注入。 / Settled game turns no longer inject the same narrative twice through tool-protocol messages and collapse duplicate resolved inputs from older Beta journals; the bounded journal projection also checkpoints unconsumed player-input IDs so old inputs cannot reappear after their consuming turn leaves the recent page.
- 游戏模式现在会在字段只属于唯一基础 Actor 时，安全修正模型把 `state_changes` 挂到错误 Actor 的明确笔误；存在多个候选时仍严格拒绝，避免一次局部字段错误触发整回合正文重生成。 / Game mode now safely repairs an unambiguous `state_changes` Actor-target typo when the field belongs to exactly one foundational Actor; ambiguous candidates remain rejected, avoiding full narrative regeneration for a local field-placement error.

- 修复游戏输入接收、Runtime 回执核对与模型上下文构建仍可能反复重放完整故事 journal、导致首个 thinking 前出现多段数秒空等的问题。活动运行回执现在通过有界投影中的精确事务 cursor 查询，新命令缺失无需扫描历史；sidecar 在成功吸收未索引尾部后立即 checkpoint，故事物理稀疏锚点与 128 条历史分页窗口对齐，避免倒序分页重复读取重叠前缀。此前由逐 delta 持久化放大的故事会在 Project Runtime 初始化时自动合并已被替代的展示修订：原 JSONL 先保存为相邻 `.presentation-v1.*.bak`，再原子写入语义等价的紧凑 journal 并重建可丢弃索引。无需新增配置。
- Fixed multi-second gaps before the first thinking event caused by Game input acceptance, Runtime receipt reconciliation, and model-context assembly still replaying complete story journals. Active receipts now resolve one exact transaction cursor from the bounded projection, and a missing new command requires no history scan; sidecars checkpoint immediately after absorbing an unindexed tail, while story sparse anchors now match the 128-transaction backwards paging window to avoid rereading overlapping prefixes. Stories enlarged by the former per-delta persistence are automatically migrated when the Project Runtime initializes: the original JSONL is first preserved as an adjacent `.presentation-v1.*.bak`, then atomically replaced by a semantically equivalent journal with superseded display revisions folded into their turns and a disposable index rebuilt. No new setting is required.
- 修复游戏回合可能一直保持空流式响应的问题：后台 Director 的结构化规划提交及规划文件写入参数现在始终压缩为有界进度摘要，不再受章节正文实时展示配置影响，也不会把大段 Markdown 的每个参数 delta 同步追加到故事 journal。这样可避免 journal 反复读取放大、后端 CPU 飙高以及 Director 投影屏障长期阻塞下一回合；同一 journal head 的有界故事快照现在还会复用隔离副本，使已被旧故障放大的现有故事不再被页面轮询重复扫描。写作与游戏的章节正文展示配置行为保持不变，无需新增配置。
- Fixed Game turns sometimes remaining on an empty stream indefinitely. Background Director structured plan submissions and plan-file write arguments are now always reduced to bounded progress summaries, independently of chapter-body live-display settings, instead of synchronously appending every large Markdown argument delta to the story journal. This prevents repeated journal-read amplification, high backend CPU, and the Director projection barrier blocking the next turn. Bounded story snapshots for an unchanged journal head now also reuse isolated copies, so page polling no longer repeatedly scans existing stories already enlarged by the former failure. Chapter-body display behavior remains unchanged across Writing and Game. No new setting is required.
- 修复故事日志写入被文件监听误判为所有变更审阅数据都需刷新、导致变更组详情接口持续高频请求的问题。流式展示只在稳定边界落盘，实时进度保留在内存；watcher 事件只失效路径相关的审阅缓存，同一 Project 的页面监听与隐藏 Agent 面板也不再重复放大请求。无需新增配置。
- Fixed continuous high-frequency change-group detail requests caused by story-journal writes broadly invalidating all change-review data. Streamed display state now persists only at stable boundaries while live progress stays in memory; watcher events invalidate only path-related review caches, and page-level Project listeners plus hidden Agent panels no longer amplify requests. No new setting is required.
- 修复模型输出上限被不合理压低的问题：DeepSeek V4 Pro / Flash 及当前 `deepseek-chat` / `deepseek-reasoner` 兼容别名现按官方模型能力发送 384K `max_output_tokens`，不再落入服务商约 8K 的默认值；MiniMax M2 系列以及支持 128K 同步输出的 Claude 5、Opus 4.6–4.8、Sonnet 4.6 也不再受 Anthropic 通用 64K 兜底压低，仍为 64K 的 Claude 4.5 型号保持原限制。未知与自定义模型继续尊重其服务商默认。游戏回合也不再把按目标字数计算的上下文预留量误用作 2–3K 的生成硬上限，写作与游戏共享的模型配置边界保持一致。无需新增配置。
- Fixed unreasonably low model-output limits. DeepSeek V4 Pro / Flash and the current `deepseek-chat` / `deepseek-reasoner` compatibility aliases now send the documented 384K `max_output_tokens` capability instead of falling through to the provider's roughly 8K default. MiniMax M2 models and Claude 5, Opus 4.6–4.8, and Sonnet 4.6 models with 128K synchronous output are no longer reduced by the generic Anthropic 64K fallback, while Claude 4.5 variants that remain 64K keep that limit. Unknown and custom models continue to respect their provider defaults. Game turns also stop misusing the target-length context reserve as a 2–3K generation hard cap, keeping the shared Writing/Game model boundary consistent. No new setting is required.
- 修复模型生成的工具参数仅因可确定修复的 JSON 语法瑕疵（例如多余右括号、缺失闭合符、未加引号的键、单引号或 Markdown 代码围栏）而在执行前反复失败的问题。所有 Agent 工具参数现在统一先严格解析，失败后通过 MIT 许可的 `jsonrepair` 做一次确定性语法修复，再重新严格解析并执行原有 Schema 归一化与约束校验；修复不会绕过字段、类型或数值约束。无需新增配置。
- Fixed repeated pre-execution tool failures caused solely by deterministically repairable JSON syntax defects in model output, such as an extra closing delimiter, missing closing syntax, unquoted keys, single quotes, or Markdown code fences. Every Agent tool argument now follows one shared path: strict decoding first, one deterministic repair pass through the MIT-licensed `jsonrepair` library on failure, then strict decoding again plus the existing schema normalization and constraint validation. Repair cannot bypass field, type, or numeric constraints. No new setting is required.
- 修复游戏故事 recent cache 被大量展示/审计旁路事件填满后丢失仍然有效的活动分支 head，进而在流式中断后的新请求中把合法玩家输入误判为脱离 ancestry、导致 rich tool batch 持久化冲突的问题。缓存现在优先保留活动分支骨架与近期玩家输入/模型工具批次，再用展示事件填充剩余预算；热缓存与进程重启后的冷恢复行为一致，无需新增配置。
- Fixed Game story recent caches dropping the still-active branch head after a large volume of display/audit side events, which made valid player input look detached from branch ancestry and caused rich tool batch persistence conflicts on requests following an interrupted stream. The cache now prioritizes the active branch backbone and recent player-input/model-tool commits before filling the remaining budget with presentation events, consistently across warm state and cold restart recovery. No new setting is required.
- 修复游戏回合完成后，正文之后的状态/选项提交工具与 thinking 被合并到正文前执行过程、导致历史时序错乱的问题。共享会话投影现在只汇总正文前的中间进展，并将 post-正文工具/thinking 保持为正文后的独立折叠组；流式到持久化历史替换时也保持 Run、正文和前后分组稳定挂载。
- Fixed completed Game turns moving post-narrative state/choice submission tools and thinking into the pre-narrative execution disclosure, which made persisted history non-chronological. The shared conversation projection now aggregates only progress before the terminal prose and keeps post-prose tools/thinking in a separate disclosure after it, while preserving stable Run, prose, and process mounts when live output is replaced by persisted history.
- 修复写作页作品目录使用虚拟列表后，“跳到最新章”和“回到顶部”会在平滑滚动途中反复挂载、测量章节行，导致定位分段卡顿的问题。章节列表继续统一使用虚拟化，不增加长短列表分支或用户配置；最新章、回顶、外部章节选择与目录定位现在都使用一次性的瞬时定位。
- Fixed segmented, stuttering navigation in the Writing outline after chapter virtualization caused “Jump to latest chapter” and “Back to top” to mount and measure rows throughout a smooth scroll. The chapter list remains consistently virtualized without a short-list branch or user setting; latest-chapter jumps, back-to-top, external chapter selections, and reveal requests now use a single immediate-positioning path.
- 修复鼠标点击“更多操作”等按钮并关闭 Radix 菜单后，焦点回到触发按钮会被误判为键盘聚焦，导致作品目录的提示绕过悬停延迟、立即出现或串位的问题。全局兼容层现在只对真正由 Tab 导航产生的焦点立即显示提示；鼠标点击会关闭当前与待显示提示，菜单恢复焦点不会重新触发。无需新增配置。
- Fixed pointer clicks on controls such as “More actions” being mistaken for keyboard focus when a Radix menu closed and restored focus, which caused Book Outline hints to bypass their dwell delay, open immediately, or appear against the wrong row. The compatibility layer now grants immediate hints only to genuine Tab navigation; pointer presses dismiss visible and pending hints, and menu focus restoration does not reopen them. No new setting is required.
- 修复手动中断后切换或继续创作会话时，前端显示会话与后端实际选中会话可能在异步窗口内错位，导致新输入沿用另一会话上下文、且输出落入另一会话的问题。写作 Agent 的启动、活跃状态、续流、控制命令与恢复请求现在全部显式携带并由后端原子校验 `session_id`；切换会话在服务端确认、历史替换与运行恢复完成前会冻结输入，过期的历史/恢复响应不会再覆盖新会话。无需新增配置。Beta 不兼容：前台写作 `/api/chat`、`/api/chat/active`、`/api/chat/stream`、`/api/chat/commands` 与 `/api/chat/recovery` 现在要求 `session_id`。
- Fixed an async binding window after manual interruption where switching or continuing a Writing conversation could leave the visible Session out of sync with the backend-selected Session, causing a new turn to inherit another conversation's context and persist its output there. Writing Agent start, active projection, stream reconnect, control commands, and recovery now all carry an explicit `session_id` that the backend validates atomically; input remains frozen until the server confirms the switch, replaces history, and restores runtime state, and stale history/recovery responses can no longer overwrite the new Session. No new setting is required. Beta breaking: foreground Writing `/api/chat`, `/api/chat/active`, `/api/chat/stream`, `/api/chat/commands`, and `/api/chat/recovery` now require `session_id`.
- 修复快速流式 thinking 首批内容一次合并为多行时，等待占位行先被新内容撑开、虚拟列表下一帧才贴底，导致末行短暂出现在下方后整体上抬的问题。共享消息列表现在跨占位、thinking、工具与正文行的 DOM 替换持续保存末尾布局锚点，并在浏览器绘制前同步补偿滚动位置；活动流式末行不再执行纵向入场位移。写作、游戏、工作台与 SubAgent 会话共用此修复，无需新增配置。
- Fixed fast streaming thinking briefly rendering its first multiline batch below the viewport before the virtualized list followed on the next frame. The shared message list now preserves one tail-layout anchor across placeholder, thinking, tool, and prose DOM replacements and compensates the scroll position before paint; active streaming tail rows also skip vertical entrance movement. Writing, Game, Workspace, and SubAgent conversations share the fix with no new setting required.
- 修复 OpenAI Responses 无状态续传在工具参数 JSON 无效或工具调用协议存在歧义时，context normalizer 已成对移除调用与错误结果、但原始 continuation 又把该调用重新带回，导致 DeepSeek 等严格 endpoint 以“缺少工具输出”返回 400 并中断整段会话的问题。Responses 重放现在以规范化后的工具调用集合为准，原子过滤已隔离的原始调用，同时保留其余原始 assistant / reasoning output；普通工具错误会继续交还 Agent 修正和重试，写作与游戏模式共用，无需新增配置。
- Fixed stateless OpenAI Responses continuation resurrecting a raw tool call after the context normalizer had atomically removed its malformed or ambiguous call/result pair, causing strict endpoints such as DeepSeek to return HTTP 400 for a missing tool output and terminate the run. Responses replay now follows the normalized tool-call set, atomically filters isolated raw calls while preserving all other original assistant/reasoning output, and leaves ordinary tool errors recoverable for Agent correction and retry across Writing and Game modes. No new setting is required.
- 修复超长作品在章节切换时完整挂载并重复渲染全部大纲条目与菜单、造成数秒主线程阻塞的问题。章节目录现在仅渲染视口附近条目，卷折叠、最新章与外部定位通过虚拟索引保持准确；目录回调保持稳定，「更多操作」与右键共用按需菜单。切换大文档时的对白高亮也改为空闲分片构建，普通编辑只重算变更文本块。无需新增配置。
- Fixed multi-second main-thread stalls when long books mounted and repeatedly rendered every outline row and menu during chapter navigation. The chapter outline now renders only rows near the viewport, while volume folding, latest-chapter navigation, and external reveals remain accurate through virtual indexes; row callbacks stay stable and More Actions shares one on-demand context menu. Dialogue highlighting for newly opened documents is also built in idle slices, with normal edits limited to changed text blocks. No new setting is required.
- 修复写作、游戏、工作台与移动端可调面板在悬停或拖动分隔条时，由面板依赖注入全局 `*, *:hover` 光标规则并触发整页样式重算的问题。所有共享及直接面板组现在统一使用项目已有的局部分隔条光标样式，不再修改全局选择器。无需新增配置。
- Fixed resizable panels across Writing, Game, Workspace, and mobile layouts injecting a global `*, *:hover` cursor rule on separator hover or drag and forcing whole-page style recalculation. Shared and direct panel groups now consistently use Denova's existing local separator cursor styles without mutating global selectors. No new setting is required.
- 修复多个项目页面首次并发打开同一旧 Book 时可能竞争迁移 Project 状态目录、使其中一个请求短暂返回 400/500 的问题；迁移现按 Registry 串行执行并保持原子、可重入。
- Fixed concurrent first-time surfaces for the same legacy Book racing its Project-state migration and briefly returning HTTP 400/500 from one request; migration is now serialized per Registry while remaining atomic and idempotent.
- 修复 Chat 工具调用、工具结果、执行审批与上下文卡片详情被超长路径、JSON、命令或无空格文本撑宽、出现横向滚动的问题；共享的写作、游戏与 Workspace Agent Chat 卡片现在会将详情约束在当前卡片宽度内，仅在垂直方向滚动并对超长内容自适应换行。
- Fixed Chat tool-call, tool-result, execution-approval, and context-card details being widened by long paths, JSON, commands, or unbroken text and exposing horizontal scrolling. Shared Writing, Game, and Workspace Agent Chat cards now constrain details to the current card width, keep vertical scrolling only, and adaptively wrap oversized content.
- 修复工作台 AgentChat 的常驻后台对话在隐藏期间继续接收流式输出后，重新激活 Tab、项目或右侧工作区时消息列表不会恢复锁定底部的问题。共享虚拟消息列表现在暂停隐藏布局的滚动测量，并在重新可见时恢复此前仍有效的底部锁；用户主动向上滚动后的解锁状态继续保留，写作与游戏等始终可见的聊天行为不变。
- Fixed persistent Workspace AgentChat conversations failing to restore bottom-follow after receiving streamed output while their tab, project, or secondary workspace was hidden. The shared virtualized message list now suspends scroll measurement for hidden layouts and restores a still-valid bottom lock when visible again, while preserving an explicit user scroll-away and leaving always-visible Writing and Game chat behavior unchanged.
- 修复 OpenAI Responses 无状态续传在回放 assistant output message 时被 SDK union 解码静默丢弃 `content`、`id`、`status` 与 `phase`，导致 DeepSeek 等严格 endpoint 在工具调用后的下一轮返回 `input: missing field content` 400 的问题；该故障与 thinking 档位无关，在 `off` 与 `minimal` 下均可复现。持久化 output item 现在先校验再通过 SDK raw override 原样发送，并补充 `thinking=off` 工具续轮回归测试，写作与游戏模式共用此修复。
- Fixed stateless OpenAI Responses continuation silently losing assistant output-message `content`, `id`, `status`, and `phase` through SDK union decoding, which made strict endpoints such as DeepSeek return `input: missing field content` with HTTP 400 on the turn after tool calls. The failure was independent of the thinking level and reproduced with both `off` and `minimal`. Persisted output items are now validated and replayed unchanged through the SDK raw override path, with a `thinking=off` tool-continuation regression test covering the fix for both Writing and Game modes.
- 移除设置读取失败时不可关闭的「无法加载安全模式」阻断弹窗；现在会记录明确错误并临时回退到权限最小的 Ask 模式，Agent 与其他页面仍可继续使用，后续设置刷新会恢复用户实际配置。
- Removed the non-dismissible “Safety mode unavailable” dialog shown when settings cannot be read. Denova now logs the failure and temporarily falls back to the least-permissive Ask mode so Agent and the rest of the app remain usable; a later settings refresh restores the user's actual configuration.
- 修复共享项目文件树在普通多分支目录中仍需逐层请求的问题。首次加载现在会在一次有界请求中按广度尽可能解析完整目录树，默认用户级上限为 100,000 条、硬上限为 1,000,000 条；只有真正超限时才保留未解析分支和游标分页继续按需加载。`project_file_tree_entry_limit` 可在用户配置中调整，写作与工作台 Files 共用该行为。
- Fixed the shared project explorer still requiring one request per directory level in ordinary multi-branch trees. Initial loading now breadth-first resolves as much of the tree as possible in one bounded request, with a user-level default of 100,000 entries and a 1,000,000 hard ceiling; unresolved branches and cursor pagination remain available only when that limit is genuinely exceeded. `project_file_tree_entry_limit` configures the boundary for both Writing and Workspace Files.
- 修复旧版 Model Profile 中 `openai_api_key` / `openai_base_url` / `openai_model` 在新版被忽略、保存后可能丢失的问题；读取时会默认迁移到通用字段，明确配置的新字段优先，下次写入使用标准字段名。火山方舟和 Gemini 旧 Base URL 会分别归一到对应的内置服务商。
- Fixed legacy `openai_api_key`, `openai_base_url`, and `openai_model` fields in Model Profiles being ignored by the new schema and potentially lost on save. Reads now migrate them into generic fields by default, canonical fields win when both exist, and the next write uses canonical names. Legacy Volcengine Ark and Gemini Base URLs are normalized to their respective built-in providers.
- 修复设置页服务商选择器使用可编辑 datalist 导致选中后难以再切换的问题；现在复用通用弹层与列表交互、只展示内置服务商目录，支持反复切换。列表一次性渲染全部选项，最多显示 8 项后在固定容器内普通滚动，不再因自动对齐导致可见数量和高度突变。
- Fixed the Settings provider picker using an editable datalist that made later changes unreliable. It now reuses shared popover/list interactions, shows only the built-in provider catalog, and supports repeated switching. Every option is rendered at once; the fixed container shows up to eight rows before using ordinary scrolling, so auto-alignment no longer changes the visible count or height.
- 修复分层 Model Profile 在切换服务商或 Base URL 后仍继承上一 endpoint 的 API Key、自定义 Header 与协议兼容选项的问题；凭据现在只在同一 endpoint origin 内继承，协议兼容选项只在同一 wire protocol 内继承，避免草稿路由变化时把密钥静默发送到其他网络目标。
- Fixed layered Model Profiles retaining the previous endpoint's API key, custom headers, and protocol compatibility options after a provider or Base URL change. Credentials now inherit only within the same endpoint origin, while protocol options inherit only within the same wire protocol, preventing draft route changes from silently sending secrets to another network destination.
- 模型连接测试日志不再直接记录可能带认证信息或查询参数的完整 Base URL；日志仅保留已解析的 scheme、host 与 path，无效地址统一显示为 `<invalid>`。
- Model connection-test logs no longer record a full Base URL that may contain embedded credentials or query parameters. Logs retain only the parsed scheme, host, and path, while invalid addresses are reported as `<invalid>`.
- 修复 Files Tab 仅打开源码文件或 Monaco 同步模型时被误判为编辑并触发自动保存的问题；保存通道现在只在内容真实偏离服务端基线后启用，干净文件切换不会再提交写入。共享项目目录树进一步对齐 VS Code Explorer：新建与粘贴以当前选择为目标、点击空白区后回到项目根目录，内联名称支持一次创建多级路径，创建、重命名、粘贴、拖动和删除后会恢复合理选择；同目录无效拖动与移入自身后代会直接拒绝。拖拽改用紧贴指针的轻量预览，明确显示文件/目录名与多选数量，同时保留源行和目标反馈，消除深层缩进造成的预览远距离偏移并降低拖动重绘开销。
- Fixed Files Tab treating source-file opens and Monaco model synchronization as edits that could trigger autosave. The save lane now activates only when content actually diverges from the server baseline, so switching clean files never submits a write. The shared project tree now follows VS Code Explorer semantics more closely: create and paste target the current selection, blank-space selection returns operations to the project root, inline names can create nested paths in one step, and create, rename, paste, move, and delete restore a sensible selection. Same-parent no-op drops and moves into a folder's own descendants are rejected immediately. Dragging now uses a lightweight pointer-anchored preview with the item name and multi-selection count, plus source-row and destination feedback, removing indentation-based preview offset and reducing drag-time rendering work.
- 修复 Files Tab 与写作页项目目录在文件保存或切换时误刷新全部已加载目录的问题。工作区变更现在明确区分内容与结构影响，Files Tab 的本地操作不会回灌自身；外部内容变化只重载编辑器，目录结构变化才刷新文件树。文件树行同时改为严格随可用宽度收缩，深层长文件名在扩展名前省略，选中背景和操作按钮不再溢出侧栏。
- Fixed Files Tab and the Writing project explorer reloading every loaded directory after file saves or switches. Workspace invalidations now distinguish content from structure, Files-tab mutations no longer echo into their own tree, external content changes reload only the editor, and only structural changes refresh explorers. Tree rows now stay within the available pane width, truncating long deep filenames before the extension so selection backgrounds and actions cannot overflow the sidebar.
- 减少 Bash 低危文本统计命令的误审批：安全策略现在可静态跟踪工作区内 `cd`、小写字面量变量及其带引号复用，并自动允许 `echo`、无 Shell 状态写入的 `printf` 和受约束的只读 `awk`。默认 Write 模式将非空 `python` / `python3 -c` 与 `go run`、`npm test` 一样按本地代码执行放行，不在审批层解析 Python 语义；Ask 模式仍需确认。外部路径、未解析变量、条件或管线中的不确定状态、命令替换、Shell 重定向，以及 AWK `system` / `getline` / 输出重定向继续走审批；极高危命令仍由全局规则拦截。Write 模式允许的解释器代码和项目脚本一样拥有 Denova 进程的本机权限。
- Reduced unnecessary approvals for low-risk Bash text analysis. The safety policy now statically follows workspace-local `cd`, lowercase literal variables, and their quoted reuse, while auto-allowing `echo`, `printf` without shell-state mutation, and constrained read-only `awk`. Default Write mode treats non-empty `python` / `python3 -c` like `go run` and `npm test`: it is local code execution and is not semantically parsed by the approval layer; Ask still prompts. External paths, unresolved variables, uncertain conditional or pipeline state, substitutions, shell redirects, and AWK `system` / `getline` / output redirects continue to prompt, while globally critical commands remain blocked. Interpreter code and project scripts allowed in Write mode have the same host permissions as the Denova process.
- 修复写作页左侧栏只持久化拖拽中间位移、前端热更新或刷新后恢复成偏窄宽度的问题。共享可调面板现在会等 React 提交最终拖拽布局后再结束持久化；写作左栏额外保存精确像素宽度，并在首次升级时只把旧的过窄状态迁移到 240px，已有更宽自定义值和后续主动调整均保持不变。「书籍设定」三列标签在非覆盖式滚动条下也有稳定空间。
- Fixed the Writing sidebar persisting an intermediate drag position and therefore restoring too narrowly after frontend hot updates or reloads. Shared resizable panels now remain persistence-eligible through React's final post-pointer layout commit; the Writing sidebar additionally retains its exact pixel width and performs a one-time migration only for legacy narrow states to 240px, preserving wider existing preferences and all subsequent user adjustments. The three-column Book Settings shortcuts remain stable with non-overlay scrollbars.
- 修复写作、游戏、工作台与配置页面的可调宽侧边栏拖拽滞后指针的问题。共享面板组现在会在分隔条按下时同步关闭 flex 宽度过渡，且不再为纯拖拽态重渲染面板内容；松手后收展动效与宽度持久化仍正常恢复。
- Fixed resizable sidebars lagging behind the pointer across Writing, Game, Workspace, and configuration surfaces. Shared panel groups now disable flex-size transitions synchronously without re-rendering panel content for drag-only state, while toggle motion and persisted widths resume normally after release.
- 修复显式文件引用先按字节截断、再被上下文总预算二次截断后，模型可能收到无提示的残缺 UTF-8 或未闭合 Markdown 围栏的问题。所有来源现在只由共享 Context Assembler 做 UTF-8 安全截断，renderer 统一追加中英双语可见提示并保留完整来源/用途审计信息，截断提示本身也计入上下文预算。
- Fixed explicit file references being byte-truncated before the shared context budget could truncate them again, which could expose incomplete UTF-8 or an unclosed Markdown fence without telling the model. The shared Context Assembler now solely owns UTF-8-safe truncation, renderers add a bilingual model-visible notice while retaining source/purpose audit metadata, and the notice itself is included in budget accounting.
- Beta 不兼容：叙事风格、图像方案、故事导演、事件包、TRPG 检定与状态系统不再用 `updated_at` 充当并发版本。统一 JSON 存储层现在以持久化字节的 SHA-256 revision 执行原子 Create/Update CAS，编码或冲突失败不会破坏旧文件；前端自动保存、冲突合并与 Config Manager 同步使用 `revision`，`updated_at` 仅保留展示语义。
- Beta breaking: narrative styles, image presets, story directors, event packages, TRPG rule systems, and actor-state systems no longer use `updated_at` as a concurrency token. A shared JSON store now performs atomic create/update CAS with the SHA-256 revision of the exact persisted bytes, preserving the previous file on encoding failure or conflict; frontend autosave, conflict rebasing, and Config Manager use `revision`, while `updated_at` remains display metadata only.
- 修复游戏 Agent SSE 与前端投影静默漂移：新增可穷尽的事件处理表，工具 started/progress 事件会更新活动状态，明确无展示副作用的事件被显式标记，未知事件会记录诊断并给出双语提示。`interactive_turn_persisted` 前端类型同时移除后端明确不发送的 `director_plan`，Approval Rule scope 收紧为后端实际支持的 `workspace`。
- Fixed silent drift between Game Agent SSE events and the frontend projection with an exhaustive event handling table. Tool started/progress events now update activity, display-neutral events are explicitly classified, and unknown events produce diagnostics plus bilingual feedback. The frontend `interactive_turn_persisted` contract also drops the backend-omitted `director_plan`, and Approval Rule scope is narrowed to the backend-supported `workspace` value.
- 修复单个超大工具结果低于旧 1 MiB 默认上限时可直接占用大部分模型上下文、继而触发无法缩减最新回合的无效压缩问题。通用工具结果默认模型可见预算调整为 128 KiB：`read` / `glob` / `grep` 等可恢复工具会提供有界分页，其他超限结果仍先完整写入 run-scoped artifact，再向模型提供可恢复的 head/tail 预览；用户显式配置的正数上限保持生效。
- Fixed oversized individual tool results below the former 1 MiB default consuming most of the model context and triggering compaction that could not reduce the latest turn. The default model-visible tool-result budget is now 128 KiB: recoverable `read` / `glob` / `grep` results paginate within the bound, while other oversized results are fully materialized into a run-scoped artifact before the model receives a recoverable head/tail preview. Explicit positive user limits remain effective.
- 修复上下文清理把最新用户输入之后的整个 turn 一律视为不可替换、导致已被模型消费的巨大可恢复工具结果仍阻塞压缩的问题。清理现在始终保留用户消息、assistant 文本、工具调用协议和尚未消费的结果；最近结果保护窗内已出现后续 assistant 步骤且可恢复的大结果正文，可在压力与缓存成本门槛同时满足时替换为稳定 receipt。
- Fixed context cleanup treating the entire turn after the latest user input as irreplaceable, allowing already-consumed, recoverable large tool results to block compaction. Cleanup now always preserves user messages, assistant text, tool-call protocol, and unconsumed results; a large recoverable result body inside the recent-results window may become a stable receipt after a later assistant step when both pressure and cache-cost gates pass.
- 修复写作、游戏与工作台 Agent 对话中，超长工具名、委派目标或 SubAgent 来源会挤出「详情」操作的问题；工具卡片头部现在按状态、可收缩元数据和固定操作列自适应布局。一次 SubAgent 委派的全部模型循环也会稳定更新同一张进度卡，后端为消息与工具事件保留一致的完整运行路径，并在详情栏展示该委派的完整时间线；已落盘且受旧会话 ID 分裂影响的历史记录会按相邻运行来源兼容聚合，不跨越新的委派边界。
- Fixed long tool names, delegation targets, or SubAgent source labels pushing the Details action out of Writing, Game, and Workspace Agent chats. Tool headers now use adaptive status, shrinkable metadata, and fixed action columns. Every model loop in one SubAgent delegation also updates one stable progress card: message and tool events retain the same full backend run path, the details pane shows the complete delegated timeline, and persisted history affected by the former split session IDs is compatibly grouped by adjacent run ancestry without crossing a new delegation boundary.
- 修复 Files 源码编辑器在长行自动换行场景滚动不流畅的问题：面板拖拽状态不再通过祖先 `:has()` 关系选择器推导，避免 Monaco 回收可见行节点时让四层工作区面板重复失效；改为由通用面板组显式管理拖拽生命周期，并隔离编辑器宿主的布局与样式计算。编辑器保持与 vscode.dev 一致的稳定 DOM 文本渲染、原生 EditContext、缩进/括号引导线、代码小地图、字形栏和当前行样式。
- Fixed uneven scrolling in the Files source editor with long wrapped lines. Panel resize state no longer relies on an ancestor `:has()` selector that invalidated four nested workspace groups whenever Monaco recycled visible line nodes; the shared panel group now owns an explicit resize lifecycle, and editor-host layout/style work is contained. The editor retains vscode.dev-style stable DOM text rendering, native EditContext, indentation/bracket guides, minimap, glyph margin, and current-line styling.
- 修复游戏模式剧情页左侧回合导航错误地把相对行号叠加到虚拟列表分页基准、导致点击历史回合仍定位到最新回合附近的问题；回合跳转现在使用当前消息窗口内的真实相对索引，并以即时滚动保证长篇、变高消息也能准确定位。
- Fixed the Game story turn navigator adding its relative row offset to Virtuoso's pagination baseline, which made historical turn clicks land near the latest turn. Turn jumps now use the actual index within the current message window and scroll instantly for deterministic positioning across long, variable-height messages.
- 修复游戏模式故事线选择器误把内部 journal 事件条数显示为回合数的问题；列表现在展示当前分支的真实回合数，并在回合落盘、分支创建或切换后实时同步。旧故事索引会从规范 journal 原子重建一次，无需手动迁移。
- Fixed the Game story picker showing internal journal event totals as turn counts. It now displays the actual turn count of each story's current branch and stays synchronized after turn persistence, branch creation, or branch switching. Existing story indexes are rebuilt atomically once from the canonical journal with no manual migration required.
- 修复浏览器自动发送 `Accept-Encoding: gzip` 时，服务端 gzip 中间件会等待 Agent / 配置管理 / 自动化等 SSE body 完整结束再压缩，导致 Ask、思考和工具进度只能在刷新或任务完成后显示的问题；所有事件流路由现在由服务端统一排除压缩，写作、游戏与其他流式入口均可即时消费。
- Fixed the server gzip middleware buffering Agent, Config Manager, Automation, and other SSE bodies to completion when browsers automatically sent `Accept-Encoding: gzip`, which delayed Ask, reasoning, and tool progress until reload or task settlement. Every event-stream route is now excluded from compression at the server boundary so Writing, Game, and other streaming clients consume updates immediately.
- 修复全局替换在 Denova 数据目录配置于当前 workspace 内且非隐藏时，可能扫描并改写 `project-state/<project-id>` 会话与变更日志的问题；替换预扫描和 CAS 写入现在都显式排除当前 Workspace Change Service 的持久状态根目录，并补充重复运行回归测试。
- Fixed global replacement potentially scanning and rewriting session/change journals under `project-state/<project-id>` when the Denova data directory was configured as a visible directory inside the active workspace. Both replacement preflight and compare-and-swap writes now explicitly exclude the current Workspace Change Service state root, with repeated-run regression coverage.
- 优化页面刷新请求：Settings 在启动消费者之间共享已完成快照并按更新事件合并刷新，Skills 与 workspace 初始化等待明确作用域且抵御 React StrictMode 重放；失效的工作台对话标签不再恢复，空闲后台标签不再挂载完整 Agent 面板，非当前项目也不会访问前台 workspace 的 Skills 或变更审阅接口。确定性的 4xx 不再被全局查询策略重试，因此刷新时不再放大 404 / 409。
- Optimized reload traffic: Settings shares a completed startup snapshot and coalesces refreshes per update event, while Skills and workspace initialization wait for a concrete scope and tolerate React StrictMode replay. Stale Workspace chat tabs are no longer restored, idle background tabs no longer mount full Agent panels, and non-current projects do not access foreground-workspace Skills or change-review APIs. Deterministic 4xx responses are no longer retried by the global query policy, preventing reloads from amplifying 404/409 responses.
- 修复工作台同时挂载多个 Agent 会话时，每个面板的命令与写作工作流选择器分别重复请求 Skills，且全局 Skills / Settings 事件会按会话数量放大请求的问题。两类选择器现在共享按 workspace 缓存的单一查询与单例失效订阅；会话级模型或思考强度更新不会刷新 Skills，同一 workspace 的多个会话只触发一次目录请求。
- Fixed Workspace Agent sessions independently reloading Skills for both command and writing-workflow selectors, with global Skills or Settings events multiplying requests by the number of mounted conversations. Both selectors now share one workspace-scoped query and one invalidation subscription; conversation-level model or thinking-effort changes do not refresh Skills, and multiple conversations in one workspace issue a single catalog request.
- 修复刷新空闲对话时，启动恢复探测会把已完成的思考/执行过程先展开再折叠的问题；恢复探测与真实执行态现在分开传递，历史思考内容从首帧开始即保持折叠，写作与工作台 Agent 对话行为一致。
- Fixed idle conversation reloads briefly expanding completed thinking/execution details during the startup recovery probe. Recovery inspection and real execution state are now projected separately, so historical reasoning is collapsed from the first frame consistently in Writing and Workspace Agent chats.
- 修复同一 Agent Run 在正文后继续思考或调用工具时，完成回填会把后置执行内容移动到正文前并触发重挂载抖动的问题。Run 展示现在以结果正文为锚按原始时序拆分前后执行过程：Game 保持 `work → text → work`，Agent Chat 保持 `work → text`；执行过程在运行完成后于绘制前自动收起，流式与历史消息通过稳定段标识复用同一渲染节点。
- Fixed completed Agent runs moving post-result thinking or tool calls ahead of the result and visibly remounting during history reconciliation. Run presentation now anchors on the result and preserves ordered process sections: Game keeps `work → text → work`, while Agent Chat keeps `work → text`; process disclosures collapse before paint when execution settles, and streaming/history messages reuse the same rendered nodes through stable segment identities.
- 修复前端重新加载、热更新或侧栏显隐时，面板库的初始化/程序化布局回调偶发覆盖用户已调宽度的问题。写作、游戏、工作台与各共享配置页现在只在用户实际拖拽或使用键盘调宽后保存布局，挂载和折叠只恢复、不回写；一级菜单宽度也改为仅在用户调整时持久化。
- Fixed initialized or programmatic panel callbacks occasionally overwriting user-adjusted widths after frontend reloads, hot updates, or sidebar visibility changes. Writing, Game, Workspace, and shared configuration surfaces now persist layouts only after an explicit pointer or keyboard resize; mounting and collapsing restore without writing back, and the primary navigation width is likewise saved only on user adjustment.
- 修复侧栏拖拽宽度时被过渡动画追赶、到达最小宽度后继续拖动会自动折叠的问题。拖拽期间宽度现在逐帧跟随指针并在最小值处停止，折叠只由专用按钮触发；工作台、Agent Chat、Settings、Agents、Skills、自动化、资料库、阅读器和预设配置等一级页面侧栏统一支持可持久化调宽。
- Fixed sidebar resizing lagging behind the pointer and collapsing when dragged beyond its minimum. Width now tracks the pointer directly and stops at the minimum, while collapse remains button-only; persisted resizing is consistently available across Workspace, Agent Chat, Settings, Agents, Skills, Automations, Lore, Reader, and preset configuration sidebars.
- 优化工作台 Agent Chat 侧栏切换：完整项目树与 40px 工具轨现在在同一列连续过渡，切换状态不再触发已挂载对话和终端的整棵工作台重渲染；布局动画改用无回弹缓动，并继续遵循 Reduced / Off 动效偏好。
- Improved the Workspace Agent Chat sidebar toggle. The full project tree and 40px utility rail now transition within one continuous column, and toggling no longer re-renders the mounted conversation and terminal workbench. Layout motion uses a non-overshooting easing curve and continues to respect Reduced and Off motion preferences.
- 修复 Project 首次迁移会把旧 `books.json` 中已经缺失的临时/测试目录重新显示为“目录不可用”，以及历史隐藏书籍被自动发现再次激活的问题。Project 注册表 v2 只激活仍存在且未隐藏的旧书；可确认来自旧注册表的缺失项和隐藏项会保留为归档 tombstone，不删除目录、对话或旧注册表。已有 v1 注册表升级前会保存 `projects.v1.backup.json` 作为完整回滚副本，用户后来添加并暂时缺失的普通 Project 仍保持可重新关联。
- Fixed initial Project migration exposing stale temporary/test paths from legacy `books.json` as unavailable directories and automatic discovery resurrecting historically hidden Books. Project registry v2 activates only legacy Books that still exist and are not hidden; confirmed stale or hidden legacy entries remain archived tombstones without deleting directories, conversations, or the legacy registry. Existing v1 registries receive a complete `projects.v1.backup.json` rollback copy before upgrade, while user-added Projects that later go missing remain relinkable.
- 修复游戏模式在工具调用后中断时，输入与工具证据会在后续成功回合结算后被移动、重复或冷重载丢失的问题。历史中断输入现在以独立的 durable context 保留，并始终投影回原始接受边界；预压缩和 checkpoint 只会原子覆盖“接受边界到结算回合”的完整区间，pending 输入继续阻止跨边界压缩，模型专用 cleanup 不改写原始富结果。
- Fixed Game inputs and tool evidence moving, duplicating, or disappearing after a later successful turn settled an interrupted tool cycle. Historical interrupted inputs now remain in a dedicated durable context projected at their original acceptance boundary; cleanup and checkpoints cover the complete acceptance-to-settlement interval atomically, pending inputs continue to block boundary crossing, and model-only cleanup never rewrites the rich canonical result.
- 修复 Agent 运行账本可能持久化用户/工具正文、原始错误或宽松文件权限，以及上下文维护指标在任务结束后丢失的问题。默认 ledger 现在只保存白名单大小、状态、策略、缓存和恢复元数据，目录/文件固定为 `0700` / `0600`；cleanup、compaction、normalizer、artifact 重读与跳过归因可持久审计，checkpoint summary 和流式 delta 不会落盘。
- Fixed Agent run ledgers retaining user/tool bodies, raw errors, or permissive file modes, and context-maintenance metrics disappearing after a task. The default ledger now persists only allowlisted size, status, policy, cache, and recovery metadata with `0700` directories and `0600` files; cleanup, compaction, normalizer, artifact rereads, and skip attribution remain auditable, while checkpoint summaries and streaming deltas never reach disk.
- 修复手动压缩可能以归一化前历史匹配最终 provider snapshot、缓存未命中无法区分本地 identity 与 provider usage，以及 protected receipt 泄露凭据形态 target / artifact path 的问题。手动与自动压缩现在共享同一协议投影，精确记录有界 cache identity/usage/miss 状态；receipt 对敏感恢复引用 fail closed，artifact store 会把既有宽权限目录和文件收紧为 `0700` / `0600`。
- Fixed manual compaction matching pre-normalized history against the final provider snapshot, ambiguous cache-miss attribution, and credential-shaped target or artifact paths leaking into protected receipts. Manual and automatic compaction now share one protocol projection with bounded cache identity/usage/miss status; receipts fail closed on sensitive recovery references, and the artifact store tightens pre-existing broad directory/file modes to `0700` / `0600`.
- 修复会话 resident window 超过 200 条后，增量压缩只总结内存尾部、在非用户消息边界错误漏掉最新工具结果，以及大型来源直接超过摘要模型限制的问题。压缩现在按精确 journal locator 流式读取完整 canonical 区间，支持分层摘要；展示历史与模型上下文仍保持分离，原 journal 不会因压缩被重写或删除。
- Fixed incremental compaction summarizing only the in-memory tail after the 200-message resident window, incorrectly dropping the latest tool result at a non-user boundary, and sending oversized sources directly beyond summarizer limits. Compaction now reads the complete canonical range through exact journal locators and supports layered summarization; display history remains separate from model context, and compaction never rewrites or deletes the source journal.
- 修复 Claude Code 等交互式 CLI 退出后对应终端 Tab 卡死的问题：Unix PTY 根 shell 现在拥有独立 session 和正确的 controlling TTY，前台进程结束后会可靠恢复到原工作目录的 zsh 提示符。
- Fixed terminal tabs freezing after Claude Code and other interactive CLIs exit. The Unix PTY root shell now owns an independent session and the correct controlling TTY, so foreground-process completion reliably returns to the zsh prompt in the original working directory.
- Lore Tab 的 Raw 模式改为 TipTap 承载的纯 Markdown 源码文档，与富文本模式共享搜索高亮、选区浮动操作和评论线程；移除 textarea 专用评论实现及光标坐标依赖。Raw 评论现在会把冻结选区安全映射到服务端规范化后的 canonical 正文，修复末尾换行差异导致无法添加评论的问题，并可在富文本与 Raw 之间继续定位唯一的可见文本锚点。
- Lore Tab Raw mode now uses a literal Markdown source document hosted by TipTap and shares search highlighting, selection actions, and comment threads with rich mode; the textarea-specific review implementation and caret-coordinate dependency are removed. Frozen Raw selections are safely mapped onto the server-normalized canonical content, fixing comment creation when trailing newlines differ and preserving uniquely resolvable visible anchors across Rich/Raw switches.
- 修复文本编辑器切换文件后丢失各自阅读位置、返回文件总是跳到顶部的问题；文件切换现在复用稳定的编辑器视图并恢复独立滚动位置，同时继续重建 TipTap 状态以隔离不同文件的撤销历史。
- Fixed the text editor losing each file's reading position and jumping to the top after file switches. File navigation now keeps a stable editor view and restores independent scroll positions while still rebuilding TipTap state to isolate undo history between files.
- 修复 `generate_image` 多图生成时，后续图片保存失败会让工具隐藏前面已经落盘图片的问题；现在返回已保存图片与逐项失败回执，只有全部失败时才整体报错。
- Fixed multi-image `generate_image` calls hiding files already persisted when a later image failed to save. The tool now returns saved images alongside per-item failures and errors the whole call only when every image fails.
- 修复首轮 Writing / Workspace Agent 对话要等整个 Run 结束才消费已提交评论、页面刷新或流重连后导致输入框与资料 Tab 再次显示评论的问题。首轮、排队后续轮次与恢复轮次现在统一在用户消息及评论引用持久化成功后、首次模型请求前消费一次性反馈，并在终态事件前刷新当前 workspace 的评论状态。
- Fixed submitted comments reappearing in the composer and Lore tab after a reload or stream reconnect because initial Writing and Workspace Agent turns consumed them only after the whole run ended. Initial, queued follow-up, and recovered turns now share the same one-shot boundary: consume feedback after the user message and comment references are durably committed, before the first model request and before the terminal event refreshes the active workspace.
- 修复内置 Claude Code / Codex Terminal 将 CLI 错误作为 PTY 根进程启动的问题；两种 Profile 现在都在 workspace 的交互式 Shell 内启动，退出 CLI 会回到原工作目录。真正退出根 Shell 时会先送完最终 PTY 输出再报告退出状态，写队列拥塞也会触发可恢复断开，不再留下卡死的 Terminal Tab。
- Fixed the built-in Claude Code and Codex terminals launching the CLI itself as the PTY root process. Both profiles now start inside an interactive shell in the workspace, so leaving the CLI returns to the original working directory. A real root-shell exit drains final PTY output before reporting completion, and write-queue saturation triggers a recoverable disconnect instead of leaving a frozen terminal tab.
- 修复资料编辑器在切换资料项后仍把 TipTap 的失焦内部选区误认为当前选区、错误显示「添加评论」浮层的问题；选区操作现在只在编辑器保持焦点且确有可见文本选区时出现，点击浮层本身仍会保留原选区。
- Fixed the Lore editor showing Add Comment after switching entries because TipTap retained an unfocused internal selection. Selection actions now appear only for a visible non-empty selection in the focused editor, while interacting with the toolbar preserves that selection.
- 修复已打开终端在应用切换 light/dark 主题时仍保留旧背景的问题；xterm 现在会在主题属性和 CSS 变量完成提交后重新解析调色板并重绘现有屏幕。
- Fixed already-open terminals retaining their previous background after the app switched between light and dark themes. xterm now resolves the palette after the theme attribute and CSS variables commit, then repaints the existing screen.
- 修复 Denova 宿主进程的 `NO_COLOR` 环境变量泄漏到内置 PTY、导致 Claude Code 等遵循该约定的程序在 light/dark 终端中退化为无彩色界面的问题；终端子进程现在独立声明 `xterm-256color` / truecolor 能力并清理宿主无色标记。
- Fixed Denova's host-level `NO_COLOR` environment variable leaking into embedded PTYs and forcing programs such as Claude Code into a colorless UI in both light and dark themes. Terminal child processes now own their `xterm-256color` / truecolor capabilities and discard the host's no-color marker.
- 修复工作台 AgentChat 的 Ask 在任务结束释放运行对象后被误报为 404 的问题；回答、取消与重复提交现在始终从项目持久 Session 读取规范终态，冷恢复待处理 Ask 会按既有安全规则取消，未知 Ask ID 仍返回 404。
- Fixed Workspace AgentChat Ask requests being misreported as 404 after a completed task released its process-local run. Answers, cancellations, and repeated submissions now resolve against the project's persisted canonical Session; cold pending Asks still cancel safely under the existing recovery rule, while unknown Ask IDs remain 404.
- 修复终端慢消费者被淘汰后只停止输出转发、WebSocket 读循环仍保持假活的问题；订阅结束现在会关闭其连接，前端沿用现有断线提示重新连接，并从有界 scrollback 恢复屏幕。
- Fixed terminal slow-consumer eviction stopping only the output relay while leaving a falsely live WebSocket read loop. Ending a live output subscription now closes its owning connection, allowing the existing reconnect flow to surface the disconnect and restore the screen from bounded scrollback.
- 修复新建游戏故事尚无回合时，空回合列表被序列化为 `null`，导致剧情舞台读取最新回合时崩溃的问题；后端现在稳定返回空数组，前端同时兼容已有的空值响应。
- Fixed a crash in the Game story stage when a new story had no turns and its empty turn list was serialized as `null`; the backend now returns an empty array consistently, while the frontend remains defensive against existing null responses.

## [v0.3.3] - 2026-07-25

### Fixed

- 切换写作界面与对话框时不再强制重置输入法为半角英文；移除聊天输入框的 `inputmode`/`autocapitalize` 属性，保持与写作编辑器一致，避免焦点切换打断中文输入。
- Switching between the Writing editor and the chat dialog no longer forces the input method back to half-width English. The chat composer's `inputmode`/`autocapitalize` attributes were removed to match the writing editor, so IME composition is no longer interrupted on focus change.
- 打开多个 md 文件时，Ctrl+Z 撤回不再把其他标签页的文本带回当前文件；每个文件现在挂载独立的编辑器实例，拥有各自独立的撤销历史栈。
- With multiple md files open, Ctrl+Z no longer pulls text from another tab into the current file. Each file now mounts its own editor instance with an independent undo history stack.

## [v0.3.2] - 2026-07-23

### Changed

- Agent 对话完成后现在按一次运行统一整理展示：中间进展正文、思考与工具调用收进同一个可展开的「执行过程」，最终或中断时的部分结果保持可见；运行中继续实时展开，历史会话默认折叠。新会话持久化 `candidate/progress/final/partial` 正文阶段，旧历史按同一运行最后一段根级正文兼容恢复，不改变模型上下文或规范会话内容；写作、工作台 AgentChat 与游戏模式共用该投影。
- Completed Agent conversations are now presented as one coherent run: intermediate progress prose, reasoning, and tool calls fold into a single expandable Execution section, while the final—or interrupted partial—result stays visible. Active runs remain live and expanded, completed history defaults to collapsed. New sessions persist `candidate/progress/final/partial` prose phases, while legacy history falls back to the last root assistant segment in the run without changing canonical conversation data or model context. Writing, Workspace AgentChat, and Game share the same projection.
- 工作台 AgentChat 以用户级、跨项目方式运行：打开或切换项目对话不会修改顶栏当前书籍，也不会切换写作页的前台会话；每个项目拥有右侧常驻的新建对话入口和完全独立的双栏 Tab 组，每个对话 Tab 绑定自己的历史、SSE 流、Ask 与持久 Agent Runtime，因此同项目或不同项目的多个对话可真正并行执行，运行中的单个对话也不会阻塞其他项目的新建、切换或重命名。工作台只恢复最终的 Project ID 绑定状态，本开发周期内的较早 Tab schema 不迁移；AgentChat 路由统一使用显式 `workspace + session_id` 边界。
- Workspace AgentChat runs at user scope across projects. Opening or switching a project conversation does not change the current book in the title bar or the foreground Writing session. Every project has a persistent right-side New chat action and an independent two-pane tab set; each conversation tab binds its own history, SSE stream, Ask interactions, and durable Agent Runtime, so conversations within or across projects can run concurrently without blocking creation, selection, or renaming elsewhere. The workbench restores only the final Project-ID-bound state; earlier tab schemas from this unreleased cycle are not migrated, and AgentChat routes consistently use an explicit `workspace + session_id` boundary.
- 一级菜单的默认顺序将「工作台」固定在写作/游戏剧情之后；旧的自定义顺序若尚未包含工作台，会在保留其余相对顺序的同时把工作台补到第二项。叙事风格列表现在默认将「节奏叙事」置顶。
- The default primary-menu order now places Workspace immediately after Writing/Game Story. Older custom orders that do not yet contain Workspace retain their remaining relative order while inserting Workspace second. Narrative-style lists now place Momentum Narrative first by default.
- 编辑器设置现在可直接调整阅读字体与阅读字号，并与全局外观设置共享同一份用户配置；修改会即时预览并自动保存，不新增独立覆盖项。
- Editor Settings now adjusts Reading Font and Reading Font Size directly while sharing the same user configuration as Appearance. Changes preview immediately and save automatically without introducing a separate override.
- `config-manager` Skill 的十类资源 reference 全面补齐字段类型、枚举、默认值、大小/数量边界、只读字段、层级继承、revision 与跨资源约束，并加入可直接映射到 `config_read` / `config_apply` 的创建、完整替换、局部 patch、分区更新和 State Binding 示例；根 Skill 现在会先区分不同资源的写入语义，再要求回读有效结果及保留敏感字段。
- The ten `config-manager` resource references now document field types, enums, defaults, size/count limits, host-owned fields, layering, revisions, and cross-resource constraints, with concrete `config_read` / `config_apply` examples for creates, complete replacements, sparse patches, sectional updates, and State Bindings. The root Skill now distinguishes mutation semantics before applying a change and requires post-read verification of both requested and preservation-sensitive fields.
- Writing Skill 选择现在只把当前 Agent 实际可用且声明写作流程能力的 Skill 发送给模型；已失效的默认值会回退到 `novel-lite`，用户/工作区同名自定义 Skill 仍按其有效元数据判断。`novel-standard` 改用内置 `general-purpose` SubAgent 审稿，并在委派不可用时由主 Agent 自检。章节与互动图像请求统一使用规范 `/skill-name` 语法并在首轮模型请求前确定性加载；工作台“继续写作”快捷键只发送自然语言意图，由当前 Writing Skill 决定流程。
- Writing Skill selection now sends only a Skill that is available to the current Agent and declares the writing-workflow capability. Stale defaults fall back to `novel-lite`, while same-name user or workspace overrides are evaluated from their effective metadata. `novel-standard` delegates review to the built-in `general-purpose` SubAgent and falls back to main-Agent self-review when delegation is unavailable. Chapter and interactive image requests now use canonical `/skill-name` syntax and preload deterministically before the first model call; the workbench Continue Writing action sends natural-language intent and leaves execution strategy to the selected Writing Skill.
- Skills 新增独立的 `category` 与 `capabilities` 元数据：管理页可按分类筛选，而 Writing Skill 选择器和后端只接受显式声明 `writing-workflow` 能力的完整写作流程，不再把所有 IDE 用户 Skill 默认视为写作预设。现有未标记自定义 Skill 仍可通过斜杠命令调用，但若被配置为 Writing Skill 会回退到 `novel-lite`。Skill 目录包含 supporting files 时，编辑器现在默认展开目录侧边栏，并在当前会话内尊重用户手动开关。
- Skills now expose separate `category` and `capabilities` metadata. The management view can filter by category, while both the Writing Skill selector and backend accept only complete workflows that explicitly declare `writing-workflow` instead of treating every user IDE Skill as a writing preset. Existing unmarked custom Skills remain available through slash commands, but a configured Writing Skill without that capability falls back to `novel-lite`. The editor now opens its directory sidebar by default when a Skill has supporting files and respects the user's manual toggle for the session.
- Agent 工具执行边界收敛到 `ToolExecutionRecord`：有效 workspace receipt 是 mutation 的唯一真源，即使工具随后返回错误也视为已提交；blocked、未执行或无 receipt 的调用不产生 mutation，缺 receipt 只记录 warning。HostEffect、post-run verification 与产品 callback 共用同一转换；`read_only` 仅关闭 workspace / Lore 直接写工具和自动文件输出，继续保留 Shell、Browser、Web 与子 Agent，并把相同 `write_mode / write_scope` 约束传给根 Agent 和子 Agent；意外写入只告警，不回滚或判定运行失败。
- Agent tool execution now converges on `ToolExecutionRecord`: a valid workspace receipt is the sole mutation source and remains authoritative even when the tool subsequently returns an error; blocked, unexecuted, or receipt-less calls produce no mutation, with missing receipts reported only as warnings. HostEffects, post-run verification, and product callbacks share one projection. `read_only` disables only direct workspace/Lore writes and automatic file output while retaining Shell, Browser, Web, and delegated Agents under the same inherited `write_mode / write_scope`; unexpected writes warn without rollback or run failure.
- 工具参数边界由严格拒绝改为 `NormalizeToolArguments` 宽容规范化：继续向模型提供完整 Schema，自动补默认值、忽略无害额外字段并转换无歧义的数字/布尔字符串；路径、枚举、数组和业务值不做猜测，关键字段缺失、类型冲突及分支歧义以结构化 `invalid_arguments` 返回给 Agent 自行修正。公共层只执行稳定核心规则，复杂语义仍由具体工具校验。
- Tool arguments now use tolerant `NormalizeToolArguments` normalization instead of strict rejection. Models still receive the complete schema; stable corrections apply defaults, ignore harmless extra properties, and coerce unambiguous numeric or boolean strings. Paths, enums, arrays, and domain values are never guessed, while missing required fields, type conflicts, and branch ambiguity return structured `invalid_arguments` for the Agent to correct; concrete tools retain complex semantic validation.
- Beta 不兼容：删除 checkpoint boundary v1 内嵌结构、clone 路径与兼容读取，旧开发期 checkpoint / rewind 数据不再有效。Ask 的已回答/已取消终态现在对任意重复提交幂等返回原 canonical resolution；丢失 waiter 的 pending Ask 会原子取消并返回该终态，未知 ID 返回 404、无效答案返回 400，Writing 与 Config Manager 共用即时卡片更新和历史刷新逻辑。
- Beta breaking: embedded checkpoint-boundary v1 data, clone paths, and compatibility reads are removed, so development-era checkpoint/rewind records are no longer valid. Answered or cancelled Ask interactions return their original canonical resolution for every repeated submission; pending Asks without a waiter are atomically cancelled and return that terminal result, unknown IDs return 404, invalid answers return 400, and Writing plus Config Manager share immediate card updates followed by history refresh.
- Agent capability registry 现在是不可绕过的权限上限，而不是仅用于展示排序：Interactive Story 永久排除通用 workspace write 与 shell，Interactive Director 只允许 event/Lore read，`event_read` 按运行时依赖报告可用性；互动回合的 prepare、submit 与 state-schema mutation 同时改为正确的 session-scoped 领域事务，不再被 workspace 安全策略误拦截。
- The Agent capability registry is now an authoritative, non-bypassable ceiling rather than display ordering. Interactive Story permanently excludes general workspace writes and shell access; Interactive Director allows only Event/Lore reads; and `event_read` reports runtime-dependent availability. Interactive turn preparation, submission, and state-schema mutations now use their correct Session-scoped domain transaction contract instead of being rejected by workspace safety policy.
- Runtime journal generation replay、Web Access 资源与 Ask 并发边界进一步收紧：最新 manifest 是唯一规范 generation，损坏 active tail 不发布部分状态；每次 invocation 结束都会关闭 Rod/Chrome/launcher/proxy；Ask 规范化深拷贝调用方输入，多个 waiter 不再竞争改写共享切片。
- Runtime journal generation replay, Web Access ownership, and Ask concurrency are now stricter. The latest manifest is the sole canonical generation and a corrupt active tail never publishes partial state; every invocation closes its Rod/Chrome/launcher/proxy resources; and Ask normalization deep-copies caller input so multiple waiters never race while rewriting shared slices.
- Beta 不兼容：删除 `ls`、`read_file`、`write_file`、`edit_file`、`execute`、`write_todos`、`read_event_cards` 及旧配置 CRUD 工具，不提供 alias；终态通用工具名为 `read`、`write`、`edit`、`glob`、`grep`、当前平台 shell、`browser`、`web_search`、`web_fetch`、`todo`、`ask`、`skill` 与 `task`。`write(path,content)` 不再要求快照，`edit(path,old_string,new_string)` 只要求当前文件中仍精确唯一匹配。
- Beta breaking: `ls`, `read_file`, `write_file`, `edit_file`, `execute`, `write_todos`, `read_event_cards`, and the former configuration CRUD tools are removed without aliases. The final general surface is `read`, `write`, `edit`, `glob`, `grep`, the host-native shell, `browser`, `web_search`, `web_fetch`, `todo`, `ask`, `skill`, and `task`. `write(path,content)` requires no snapshot, while `edit(path,old_string,new_string)` succeeds when the current file still contains one exact unique match.
- 工具描述符、权限与运行时调度现在区分 workspace、config、session 与 external 副作用；`web_search` / `web_fetch` 可独立授权，shell 明确属于宿主级外部副作用并在游戏模式 fail closed。工具结果 envelope、分页 cursor、shell progress 和配置读取均按最终模型可见预算有界；Browser SSRF 同步阻断 NAT64、6to4 与 Teredo 中嵌入的私网/元数据 IPv4。
- Tool descriptors, permissions, and scheduling now distinguish workspace, config, session, and external effects. `web_search` and `web_fetch` can be authorized independently; shell execution is explicitly host-external and fails closed in Game mode. Result envelopes, pagination cursors, shell progress, and configuration reads are bounded by the final model-visible budget, while Browser SSRF validation also rejects private or metadata IPv4 addresses embedded through NAT64, 6to4, or Teredo.
- Beta 不兼容：Agent 工具接口收敛为 `Tool.Info + Tool.Run -> ToolResult`、root `ToolDefinition` 和每次 run 的唯一不可变 Registry；删除 `BaseTool`、`InvokableTool`、`StreamableTool`、重复 Registry、`Bind/FromTool` 及 descriptor-in-`ToolInfo.Extra` 兼容层。现有用户会话和 runtime journal 继续兼容读取，不执行破坏性迁移。
- Beta breaking: Agent tools now use only `Tool.Info + Tool.Run -> ToolResult`, root `ToolDefinition`, and one immutable per-run Registry. `BaseTool`, `InvokableTool`, `StreamableTool`, duplicate registries, `Bind/FromTool`, and descriptor-in-`ToolInfo.Extra` compatibility layers are removed. Existing user sessions and runtime journals remain readable without destructive migration.
- 工具结果现在将唯一模型输入 `ModelContent`、仅展示的 `DisplayContent` 和仅 lifecycle/receipt/host-effect 使用的有界 JSON `Details` 完全分离；typed started/progress/finished 事件按实际完成顺序更新 UI，transcript 仍按调用顺序配对。native scheduler 默认最多并发 8 个连续只读工具（用户/工作区可配置 1–64），写入与 child 工具形成屏障；durable steering 可跳过未启动调用、只中断 `interruptible_wait`，并保留 workspace gate、journal、host effect、commit barrier 和 `effect_unknown` 恢复语义。
- Tool results now isolate model-only `ModelContent`, display-only `DisplayContent`, and bounded JSON `Details` used only by lifecycle receipts and host effects. Typed started/progress/finished events update UI in completion order while transcripts retain source order. The native scheduler runs up to eight consecutive read-only tools concurrently by default (user/workspace configurable from 1–64), keeps writes and child tools as barriers, and applies durable steering only to unstarted or `interruptible_wait` calls while preserving workspace gates, journals, host effects, commit barriers, and `effect_unknown` recovery.
- 游戏模式将剧情 JSONL、模型上下文和前端显示历史拆分为独立投影：主游戏 Agent、自动/手动上下文压缩及导演 Agent 现在按压缩检查点和逻辑回合范围直接读取规范日志，不再受 UI 最近 100 回合分页影响。前端剧情缓存限制为最多 10,000 回合或约 96 MiB，超限后切换为可返回最新剧情的连续历史窗口；回合导航将长历史聚合为有界标记，并始终保留当前与最新回合。
- Game mode now projects canonical story JSONL independently into model context and display history. The Game Agent, automatic/manual compaction, and Director Agent read checkpoint-relative logical turn ranges directly from the journal instead of inheriting the UI's latest-100-turn page. The frontend story cache is bounded to 10,000 turns or approximately 96 MiB, switches to a contiguous historical window with an explicit return-to-latest action when exceeded, and aggregates long turn navigation into bounded markers while retaining the active and latest turns.
- Beta 不兼容：游戏模式只允许修改、重生成、规则重掷或切换当前分支的最新逻辑回合。编辑最新用户输入会追加替代回合，编辑最新 AI 回复会追加正文修订事件并保留回合 ID、状态、选项、图片与轨迹；历史回合必须先显式创建分支。会话与游戏历史分页响应新增稳定分页元数据，普通应用读取只返回有界窗口。
- Beta breaking: Game mutations, regeneration, rule rerolls, and version switching now target only the current branch's latest logical turn. Editing its user input appends a replacement turn; editing its AI reply appends a narrative-revision event while preserving the turn ID, state, choices, images, and trace. Historical turns require an explicit branch first. Session and Game history responses now expose stable page metadata, and ordinary application reads return bounded windows.
- `web_search` 重构为可替换 provider 的网页访问模块：配置 SearXNG 时优先使用并在失败/空结果后回退；默认并发查询 DuckDuckGo 与 Bing，等待两边后交错去重合并，单边失败/超时仍返回另一边结果及诊断。Bing 优先读取完整 HTML、按查询语言选择市场，RSS 仅作降级且会拒绝未覆盖完整查询的明显无关结果。Beta 不兼容：移除原有 Baidu/Google 聚合，暂不支持 Brave；`web_search` 与 `web_fetch` 现在是可独立授权的 capability。
- `web_search` is rebuilt on a replaceable-provider Web Access module. A configured SearXNG instance is tried first with fallback after failure or empty results; otherwise DuckDuckGo and Bing run concurrently, and their completed results are interleaved, deduplicated, and combined. One provider may fail or time out while the other still returns results with diagnostics. Bing prefers full HTML, selects a market from the query language, and uses RSS only as a fallback with whole-query relevance rejection. Beta breaking: the former Baidu/Google aggregation is removed, Brave is not yet supported, and `web_search` / `web_fetch` are now independently authorized capabilities.
- Agent 依赖方向收敛为 `API → App → internal/agents → agent`；`internal/agents` 负责产品组合并按需使用公共 `agent/runtime` 与 `runtime/filejournal`，App 不绕过产品层调用公共 root、model、context、session 或 tools。产品层仅保留具有真实领域边界的子 package，包括 `context`、`session`、`skills`、`tools`、`toolresult`、`configresource` 及会话配置/日志；重复的产品 Runtime package 已删除。
- Agent dependencies now flow as `API → App → internal/agents → agent`. `internal/agents` owns product composition and uses the public `agent/runtime` and `runtime/filejournal` where needed; App does not bypass the product layer for public root, model, context, session, or tools. Product subpackages are retained only for real domain boundaries, including `context`, `session`, `skills`, `tools`, `toolresult`, `configresource`, and conversation configuration/journaling; the duplicate product Runtime package is removed.
- CI 与 GitHub Release 构建现在分别验证 Denova 根 module 和公共 `agent` module；OpenAI adapter 由 `agent` 的依赖一致性检查、测试、静态检查与可达漏洞扫描统一覆盖，不再维护第三个 nested module。
- CI and GitHub Release builds now validate the Denova root module and public `agent` module separately. The OpenAI adapter is covered by the `agent` dependency-drift checks, tests, static analysis, and reachable-vulnerability scan instead of a third nested module.
- Agent 执行链完全移除 Eino，改用原生循环和原生文件、搜索、Skill、Todo、Task 工具；模型调用、流读取和工具批次均可立即响应取消，默认不写死迭代次数或运行超时。
- The Agent execution path removes Eino completely in favor of the native loop and native filesystem, search, Skill, Todo, and Task tools. Model calls, stream reads, and tool batches respond immediately to cancellation, with no hard-coded default iteration or runtime timeout.
- `agent` root 公共接口收敛为 `AgentConfig`、`Middleware`、`RetryConfig`、`NewAgent` 与 `NewRunner` 一套原生命名；删除无实际语义的 checkpoint、recursive cancel 和旧 ChatModel Agent 兼容入口。OpenAI adapter 同步只保留 `Config` 与 `New`。
- The public `agent` root interface now has one native vocabulary built around `AgentConfig`, `Middleware`, `RetryConfig`, `NewAgent`, and `NewRunner`. No-op checkpoint/recursive-cancel options and legacy ChatModel Agent compatibility entry points are removed, while the OpenAI adapter exposes only `Config` and `New`.
- Beta 不兼容：Config Manager 的 `deep_agent_parent` / `deep_agent_parents` 字段更名为 `subagent_parent` / `subagent_parents`；产品组合迁移到 `internal/agents`，Runtime 迁移到公共 `agent/runtime`，产品 `context`、`session` 与 `skills` 保留为深 module，旧内部路径不再提供兼容转发。
- Beta breaking: Config Manager fields `deep_agent_parent` / `deep_agent_parents` are renamed to `subagent_parent` / `subagent_parents`. Product composition moves to `internal/agents`, Runtime moves to public `agent/runtime`, and the product `context`, `session`, and `skills` remain as deep modules; former internal paths have no compatibility forwarding.
- Beta 不兼容：新的 `runtime.BindingRef{Kind, Profile, Key, Labels}` 以完整规范 JSON 生成 journal identity，不会自动发现旧 Binding 路径下的 Runtime journal。旧 journal 不会被删除，canonical Session、Story 和工作区用户内容也不会被删除；升级时仍处于 active 的任务需在新版中重新发起。
- Beta breaking: the new `runtime.BindingRef{Kind, Profile, Key, Labels}` derives journal identity from its complete canonical JSON and does not automatically discover Runtime journals stored under legacy Binding paths. Legacy journals are not deleted, nor are canonical Sessions, Stories, or workspace user content; tasks still active during upgrade must be started again on the new version.
- 编辑器自动保存统一为修改后延迟保存：连续输入会重置延迟，停止输入后只保存最新草稿，不再依赖固定周期；目录与章节统计也不再每 3 秒扫描整本作品。
- Writing-editor Auto Save now consistently uses after-delay semantics: continued typing resets the delay, and only the latest draft is saved after input stops. Workspace tree and chapter statistics also no longer scan the whole project every three seconds.
- Git 自动版本改为修改后延迟创建：工作区修改会重置 30 秒空闲计时，停止修改且达到用户配置的最小间隔后才在后台创建版本。该功能默认开启、默认间隔 10 分钟，并可在设置中修改或关闭。
- Automatic Git versions now use an after-change delay: workspace changes reset a 30-second idle timer, and a version is created in the background only after editing stops and the user-configured minimum interval has elapsed. The feature defaults to on with a 10-minute interval and can be changed or disabled in Settings.
- 编辑器、写作 Agent、游戏 Agent 和配置 Agent 的文件修改统一使用同一套自动版本策略；独立的 Agent 字数阈值设置已移除，旧的 `version_agent_enabled` 与 `version_agent_char_threshold` 配置不再生效。
- Editor, Writing Agent, Game Agent, and Configuration Agent file changes now share the same automatic-version policy. The separate Agent character threshold has been removed, and the legacy `version_agent_enabled` and `version_agent_char_threshold` settings no longer take effect.
- 所有 Agent 的 system prompt 改为一次性、不可变的组合产物：实际运行、上下文分析、日志和预算校验复用同一份 instruction、来源 manifest、字节数与 SHA-256；动态风格、Skills、Teller、Automation、图像和 SubAgent 片段均使用显式高上限，超限会返回中英文错误而不是发送半份提示词。配置管理资源 Skill 单源上限为 512 KiB、总上限为 1.5 MiB。Beta 不兼容：旧的风格规则固定 32k 字符截断不再生效，改由统一可配置预算控制。
- Every Agent system prompt is now one immutable composition artifact: production runs, Context Analysis, logs, and budget checks reuse the exact same instruction, provenance manifest, byte counts, and SHA-256 hashes. Dynamic style, Skill, Teller, Automation, image, and SubAgent fragments use explicit high limits and fail with bilingual errors instead of sending partial prompts. Config Manager resource Skills allow 512 KiB per source and 1.5 MiB total. Beta breaking: the former fixed 32k style-rule truncation is superseded by the unified configurable budget.
- Config Manager 现在与写作、游戏共用持久化 Agent 控制面：活动查询只读投影精确 Runtime identity，刷新或随机 SSE 断线会按 task ID + cursor 恢复同一展示流，`/clear` 先 drain 对应 Task 与 binding 再写清理标记。Task display checkpoint 升级为 v2，结算状态、有限终态原因和游戏回合待持久化语义即使展示事件被逐出也能恢复；v1 checkpoint 仍可读取。
- Config Manager now shares the durable Agent control plane used by Writing and Game: active inspection read-only projects the exact Runtime identity, refreshes and unexpected SSE disconnects reattach the same display stream by task ID + cursor, and `/clear` drains the exact Task and binding before recording its clear marker. Task display checkpoints are upgraded to v2 so settlement, bounded terminal reasons, and Game turn-persistence obligations survive display eviction; v1 checkpoints remain readable.
- Writing、Game、Lore、Config Manager 与 workspace/root Automation 的活动展示 Task 统一在注册生命周期内申请和释放进程级回放预算；默认最多 8 个活动 Task、合计 512 MiB，admission 失败发生在 Runtime 或 Run ledger 写入之前，不再由各产品独立放大内存上限。
- Active display Tasks from Writing, Game, Lore, Config Manager, and workspace/root Automation now reserve and release one process-level replay budget through their registration lifecycle. The default ceiling is eight active Tasks and 512 MiB total, with admission failure occurring before Runtime or Run-ledger writes instead of each product multiplying the memory limit independently.
- Agent Runtime 日志会在安全边界压缩为不可变、带校验和的 snapshot/tail 世代，仅保留 active 与 previous 两代；启动时流式重放有界 tail，最新 manifest 或 snapshot 损坏时可退回上一完整世代并继续读取当前 tail，同时保留跨 checkpoint 的 command 幂等索引。
- Agent Runtime journals now compact at safe boundaries into immutable checksummed snapshot/tail generations, retaining only active and previous generations. Startup streams the bounded tail, can fall back from a corrupt newest manifest or snapshot to the prior complete generation and continue through the current tail, and preserves command idempotency across checkpoints.
- 工具变更后的 Automation 触发改为 Runtime 持有的 durable HostEffect outbox：effect 与 tool-finished 在同一日志事务写入，仅在同一 operation/cycle 的 canonical output commit receipt 落盘后转移并确认；输出提交失败会记录可审计的 abandon 事件且不会触发 Automation。Automation successor 在写入新 operation 前必须 drain 该 Run 跨 epoch 的全部 effect，转移失败保持原 operation ID 并显式拒绝继续。
- Automation after tool mutations now flows through a Runtime-owned durable HostEffect outbox. Each effect shares the tool-finished journal transaction and is transferred and acknowledged only after the canonical output-commit receipt for the exact operation/cycle is durable; a failed output commit records an auditable abandonment without firing Automation. Before admitting a successor operation, Automation drains every effect for that Run across epochs, preserving the original operation ID and rejecting continuation if transfer fails.
- Agent 冷恢复改为由服务端日志投影唯一授权、且恢复请求不再携带用户原文：写作与游戏客户端只会先恢复展示流，再执行一个严格按队首优先级选出的状态动作；正常运行、未知工具结果和未确认模型调用均不会被自动重放，Stop 使用服务端生成的稳定 Abort 身份，安全终态则直接从 Session / Story 的规范历史恢复。Beta 不兼容：旧版恢复 payload 会被忽略且不能影响恢复内容，同时提交全部恢复动作的调用方式不再受支持。
- Agent cold recovery is now authorized solely by server journal projections, and recovery requests no longer carry user-authored payloads. Writing and Game clients first reattach display, then execute at most one state-changing action selected by strict queue priority; ordinary running work, uncertain model calls, and unknown tool results are never replayed automatically. Stop uses a stable server-derived Abort identity, while safe terminal UI restores from canonical Session / Story history. Beta breaking: legacy recovery payloads are ignored and cannot influence recovery content, and clients must no longer submit every projected action.
- Agent Runtime 日志恢复改为流式归约事件，不再把完整事件历史一次性载入内存；历史命令仅保留紧凑的回执索引以维持跨重启幂等。命令指纹统一为带版本的规范 JSON，消除 Go 调试格式、map 顺序与进程实现细节对持久协议的影响。Beta 不兼容：旧调试格式指纹下尚未结算的命令不能复用原 command ID，需用新版客户端重新发起；已落盘的用户内容不会被删除。
- Agent Runtime recovery now reduces journal events as a stream instead of materializing the complete event history, retaining only a compact historical command-receipt index for cross-restart idempotency. Command fingerprints use versioned canonical JSON, removing Go debug formatting, map ordering, and process implementation details from the durable protocol. Beta breaking: unsettled commands written with the legacy debug fingerprint cannot reuse their old command ID and must be started again by an updated client; canonical user content remains intact.
- 写作与游戏的结构化上下文命令现在必须持久化精确、有界、可校验 hash 的恢复计划；冷启动只按该计划对 Session / Story 执行 revision CAS，不再猜测旧进程内对象或走兼容回退。分层压缩对每层只进行一次摘要调用，目标压缩比仅用于提示与审计，不再触发隐藏重试或固定次数的额外模型成本。
- Writing and Game structural-context commands now require an exact, bounded, hash-verified restore plan. Cold start applies that plan to Session / Story through revision CAS rather than guessing process-local objects or using a compatibility fallback. Layered compaction makes one summary call per layer; target ratios guide the prompt and telemetry without hidden retries or fixed-count extra model calls.
- 写作与游戏模式首次 Agent 请求现在必须携带由客户端持有的 `command_id`：网络错误与 5xx 会复用同一 ID，明确 4xx 或已确认接受后才释放；服务端在创建展示 Task 前校验同 ID 的 payload/binding，活跃重试直接复用原 Task，重启后的已结算或中断重试从有界 Runtime operation 索引重建终态流且不会再次调用模型或工具。Beta 不兼容：旧客户端缺少 `command_id` 时返回双语 400。
- Initial Writing and Game Agent requests now require a client-owned `command_id`: network errors and 5xx responses retain the same ID, while definite 4xx rejection or confirmed acceptance releases it. The server validates payload and binding before allocating a display Task, reuses the original active Task, and reconstructs settled or interrupted post-restart terminal streams from a bounded Runtime operation index without invoking the model or tools again. Beta breaking: legacy clients without `command_id` receive a bilingual 400 response.
- Automation 的语义、定时与章节批次触发统一为持久化 `claim → decide → action → complete` 协议：有界观察和模型判定先落盘，inbox / run 使用确定性 ID 与跨进程 lease，Agent `StartTurn` 接受后先写入带 command / operation receipt 的 running RunRecord，再推进触发完成；重启会按 runtime journal 对账，确认操作也会先以 CAS 绑定确定性 RunID，避免崩溃窗口造成丢执行或重复执行。
- Semantic, scheduled, and chapter-batch Automations now share a durable `claim → decide → action → complete` protocol. Bounded observations and model decisions persist before effects; inboxes and runs use deterministic IDs plus cross-process leases; an accepted Agent `StartTurn` persists a running RunRecord with its command/operation receipt before trigger completion; restart reconciles against the runtime journal, and inbox confirmation CAS-binds its deterministic RunID first, closing crash windows that could lose or duplicate execution.
- Agent Runtime 现在把已接受的 NextTurn 视为独立的持久化后继 operation：前序运行正常 settle、失败或 Abort 后会继续 NextTurn；进程崩溃恢复只做 canonical 对账并保留 NextTurn，绝不隐式启动 Engine，客户端重放同一 command ID 后才显式继续。Execution adapter 持久化版本化有界恢复描述，并按 durable profile 使用与实时队列相同的 Cycle 准备能力重建进程内依赖；暂不可恢复时保留为可见、可重试 pending。已确认的 canonical output receipt 始终优先于迟到的 engine/display 错误，operation 以 success + warning 结算。
- Agent Runtime now treats an accepted NextTurn as a durable successor operation. It continues after normal predecessor settlement, failure, or Abort; crash recovery only reconciles canonical state and keeps NextTurn pending without implicitly starting the Engine, and the client explicitly resumes it by replaying the same command ID. The execution adapter persists a versioned bounded restore descriptor and rebuilds process-local dependencies through the same profile Cycle preparation used by live queued work; unavailable dependencies remain visible and retryable instead of being cancelled. An acknowledged canonical output receipt always outranks late engine/display errors and settles as success with a warning.
- Agent 输入接受边界新增独立的 input intent / receipt outbox：Runtime 在任何 Engine、模型或工具调用前，先从有界 v2 恢复描述生成稳定语义 hash，并把 canonical 用户输入精确物化；写作 Session 对同一 command / operation / cycle 只保留一条用户消息，游戏 Story 先追加 `player_input_accepted`，完成 Turn 再按 ID/hash 消费。崩溃恢复只查询或补写输入并对账丢失回执，不会重跑 Engine；Review Feedback 也以重新解析后的同一引用内容参与 hash。Beta 不兼容：v1 恢复描述不能用于冷恢复，需由新版客户端重新发起。
- Agent input acceptance now has a dedicated input intent/receipt outbox. Before any Engine, model, or tool call, the Runtime derives a stable semantic hash from a bounded v2 restore descriptor and materializes the exact canonical user input. Writing Sessions retain exactly one user message per command/operation/cycle, while Game Stories append `player_input_accepted` first and later consume it by ID/hash from the completed Turn. Crash recovery only queries or repairs input and reconciles a lost receipt without rerunning the Engine; Review Feedback hashes the same freshly resolved reference content. Beta breaking: v1 restore descriptors cannot cold-recover and must be started again by an updated client.
- Agent 模型输入改为严格的“两阶段”边界：`AssembleModelContext` 只生成一次精确模型消息与来源审计且不写入状态，生产运行随后通过 `CommitModelInput` 发布已接受的用户输入；上下文分析复用同一装配路径但永不提交，因此不会新增会话历史、领域意图或游戏状态。Beta 不兼容：移除旧 `PrepareMessages`、`context.Build`、`ModeAdapter` 与 Source 兼容适配入口。
- Agent model input now has a strict two-phase boundary: `AssembleModelContext` produces the exact model messages and provenance audit once without writes, and production then publishes the accepted user input through `CommitModelInput`. Context Analysis reuses the same assembly path but never commits, so it cannot append session history, stage domain intents, or change game state. Beta breaking: the legacy `PrepareMessages`, `context.Build`, `ModeAdapter`, and Source compatibility adapter entry points are removed.
- 写作与游戏模式的上下文压缩改为同一 Binding 上的持久化结构命令：自动压缩只在主回合成功结算后以独立 operation 发布，手动压缩与移除同样经过 typed phase、domain intent/receipt 和 revision CAS；失败、取消或崩溃不会留下半提交 checkpoint，精确重试会按稳定 ID 对账而不会重复写入。
- Context compaction in Writing and Game modes now runs as a durable structural command on the same binding. Automatic checkpoints publish in a separate operation only after the parent turn succeeds; manual compaction and removal use the same typed phase, domain intent/receipt, and revision CAS. Failure, cancellation, or recovery cannot leave a half-committed checkpoint, and exact retries reconcile by stable ID without duplicate writes.
- Agent 产品执行构造入口收敛为显式的 `execution.NewDurableRuntime`，测试和短生命周期本地执行使用 `execution.NewEphemeralRuntime`；低层 model Cycle executor 与 durable engine 均为模块内部实现。Agent journal 与 Automation 持久化协调共用无超时、可取消等待的进程/文件双层 lease。Beta 不兼容：未发布的 ChatService、Harness Service 及其多套 Start/Run 构造入口已删除。
- Product Agent execution now starts explicitly through `execution.NewDurableRuntime`, while tests and short-lived local execution use `execution.NewEphemeralRuntime`; the low-level model Cycle executor and durable engine remain internal to the module. Agent journals and Automation persistence share a cancellation-aware, timeout-free process/file lease. Beta breaking: unreleased ChatService and Harness Service types plus their multiple Start/Run construction paths are removed.
- Agent 主流程改为 `Profile → Engine → Journal → ContextProjector → ToolDescriptor` 的稳定依赖方向，旧写作与游戏 SSE 仅保留为带独立显示游标的适配层；原先“运行中再次发送会中断并替换当前任务”的行为改为显式 Follow Up / Steer / Abort。Beta 不兼容：移除无作用域的旧 abort 路由，所有中止命令必须携带精确 operation ID。
- The Agent path now follows the stable dependency direction `Profile → Engine → Journal → ContextProjector → ToolDescriptor`, with legacy Writing/Game SSE retained only as display adapters with independent resumable cursors. Sending during a run no longer implicitly aborts and replaces it; callers explicitly choose Follow Up, Steer, or Abort. Beta breaking: unscoped legacy abort routes are removed, and every abort command must carry the exact operation ID.
- 会话持久化升级为 append-only JSONL v2，并将 codec、replay、repair、apply 与 write 职责拆分；读取仍兼容旧快照格式，尾部撕裂会安全修复并保留备份。高频流式展示按有界批次 append，并在工具/子 Agent 终态强制 flush，避免逐 token fsync。
- Session persistence is upgraded to append-only JSONL v2 with separate codec, replay, repair, apply, and write responsibilities. Legacy snapshots remain readable, torn tails are safely repaired with backups, and high-frequency display deltas append in bounded batches with mandatory tool/sub-agent terminal flushes instead of per-token fsync.

### Removed

- Beta 不兼容：内置 Skills 一次性收敛为 `config-manager`、`novel-lite`、`novel-standard`、`chapter-illustration`、`interactive-image` 和 `web-research`。删除与系统提示或统一工具边界重复的 `outline`、`group-plan`、`continue`、`rewrite`、`lore`、`lore-init`、`skills-creator`，以及依赖旧 Agent/Goal 编排的 `novel-heavy`、`orchestrate-projects`；输入框不再硬编码四个写作斜杠命令，但用户与工作区自定义 Skills 及其动态快捷命令保持不变。
- Beta breaking: built-in Skills are consolidated in one step to `config-manager`, `novel-lite`, `novel-standard`, `chapter-illustration`, `interactive-image`, and `web-research`. Removed `outline`, `group-plan`, `continue`, `rewrite`, `lore`, `lore-init`, and `skills-creator`, which duplicated the system prompt or unified tool contracts, plus `novel-heavy` and `orchestrate-projects`, which depended on obsolete Agent/Goal orchestration. The composer no longer hardcodes the four writing slash commands; user and workspace custom Skills and their dynamic shortcuts remain unchanged.

### Fixed

- 修复写作工作区「设定」Tab 的评论没有锚点划线、折叠后无法点击划线重新展开、提交给 Agent 后仍留在编辑器中的问题；资料与正文评论现在共用一致的锚点、悬停、聚焦、折叠展开与评论线程布局，并会在发送成功后立即隐藏，发送失败时恢复。
- Fixed Lore-tab comments missing their anchored underline, failing to reopen from that underline after collapse, and remaining visible after submission to the Agent. Lore and manuscript comments now share consistent anchor, hover, focus, collapse/reopen, and thread layout behavior, disappear immediately after a successful send, and return if submission fails.
- 修复 `write_lore_items` 将局部写入参数错误声明为全部必填的问题：创建、更新和删除现在只需提交各自相关字段，更新省略字段会保留原值，纯删除不再需要空 `items`，并拒绝没有实际变化的空更新。
- Fixed `write_lore_items` incorrectly declaring every sparse-mutation field as required. Create, update, and delete calls now submit only their relevant fields, omitted update fields preserve existing values, delete-only calls no longer require empty `items`, and no-op updates are rejected.

- 修复 Agent Ask 选择答案并提交后，消息 `parts` 被原地追加时仍复用旧去重身份缓存，导致对话页读取 `primaryKey` 崩溃并进入错误边界的问题；缓存现在会识别引用未变但内容已变的 part 数组，并正确以 answered 终态覆盖 pending Ask。
- Fixed the conversation crash after selecting and submitting an Agent Ask answer. In-place appends to a message's `parts` array no longer reuse stale deduplication identities, so the answered state correctly replaces the pending Ask instead of reading a missing `primaryKey` and entering the error boundary.
- 工作台双会话分栏现在显示贯穿内容区的细分隔线，并在悬停、键盘聚焦和拖拽时增强反馈；可见线与 8px 拖拽命中区仍属于同一个分隔控件，不会出现手柄与可操作区域分离。
- Workspace split conversations now show a continuous divider through the content area with stronger hover, keyboard-focus, and drag feedback. The visible line and its 8px hit area remain one separator control rather than separate handle and interaction targets.
- 优化工作台首次刷新：项目会话树改为从有界 journal 元数据投影读取，不再为每本书物化所有会话的最近消息或重写未变化的索引；React Strict Mode、启动配置消费者和工作台项目树的并发同源读取会共享进行中的请求，完成后仍可正常刷新。
- Improved the initial Workspace refresh. The project conversation tree now reads bounded journal metadata projections instead of materializing every session's recent messages or rewriting unchanged indexes across all books. Concurrent same-source reads from React Strict Mode, startup settings consumers, and the Workspace project tree share the in-flight request while remaining refreshable after completion.
- Agent 工具卡现在区分工具执行错误与 Shell 子进程的非零退出：诊断命令的非零退出改为“命令已结束”注意提示，不再误称工具失败或要求修正命令；明确有 continuation 的有界读取/搜索分页按正常完成态展示，并将 offset/cursor 作为可选后续信息。超时、取消、无续读点的截断及真实工具异常仍保留错误或警告。
- Agent tool cards now distinguish tool-execution errors from a Shell child process's non-zero exit. Diagnostic non-zero exits are shown as completed command outcomes instead of tool failures or automatic correction prompts; bounded read/search pages with a continuation render as successful pages with optional offset/cursor follow-up metadata. Timeouts, cancellations, truncation without continuation, and actual tool errors remain errors or warnings.
- 工作台项目侧栏不再把所有已打开的会话显示为整行高亮；当前 Tab 继续由标签栏表达，侧栏会话只保留悬停反馈与运行中状态，避免多个会话同时像被选中。
- The Workspace project sidebar no longer gives every open conversation a full-row selected background. The tab bar remains the source of truth for the current tab, while sidebar conversations retain only hover feedback and the running indicator, avoiding multiple rows that look selected at once.
- 修复 React Strict Mode 重放 effect 时会使工作台对话的首次权威历史请求失效、导致旧对话偶尔暂时显示为空并记录竞态错误的问题；请求顺序保护现在只处理同一挂载实例内真正的新旧请求竞争。
- Fixed React Strict Mode effect replay invalidating a Workspace conversation's first authoritative history request, which could briefly render an existing conversation as empty and log a race error. Request-order protection now applies only to genuine newer requests within the same mounted hook instance.
- 修复顶栏消息铃铛 hover 背景覆盖底部分隔线，以及工作台标签列表未填满标签栏高度、下方残留空隙的问题。
- Fixed the top-bar message bell hover background covering the bottom divider and the Workspace tab list leaving an empty strip below its tabs.
- 修复写作与工作台等一级页面切换时，延迟渲染的旧内容先被新页面侧栏布局挤压或拉伸、造成内部组件抖动的问题；页面框架、状态栏与移动端导航现在都跟随实际呈现的路由原子切换，菜单选中反馈仍保持即时。
- Fixed internal components visibly shifting when switching between Writing, Workspace, and other primary pages because deferred outgoing content was briefly squeezed or stretched by the incoming page chrome. The page frame, status bar, and mobile navigation now switch atomically with the route actually being painted while selected-menu feedback remains immediate.
- 修复工作台已打开 Tab 没有选中高亮、用户无法判断当前页面的问题；工作台 Tab 现在使用不受右键菜单状态覆盖的 `aria-selected` 显示选中背景、文字和顶部强调线，共享 Radix Tabs 及游戏模式中的同类选中样式也统一匹配正确的 active 状态。
- Fixed open Workspace tabs lacking a visible selected state. Workspace tabs now use the context-menu-safe `aria-selected` contract for their selected fill, text, and top accent, while shared Radix Tabs and equivalent Game-mode tabs consistently match the correct active state.
- 优化一级菜单切换的响应速度：菜单选中反馈与重页面渲染解耦，首次进入的路由立即挂载并各自加载，通过稳定属性和记忆化边界减少已访问页面与写作 Agent 的无关重渲染；写作 Agent 在前台侧栏内保持稳定实例，工作台则使用按对话隔离的 Agent 实例。一级菜单仍以整个按钮作为点击和拖拽区域，并提高轻微手抖时的点击容错。
- Improved top-level menu responsiveness by decoupling selected-state feedback from heavy page rendering, mounting first-visit routes immediately behind isolated loading boundaries, and reducing unrelated rerenders for retained pages and the Writing Agent with stable props and memoized boundaries. The foreground Writing Agent keeps its stable sidebar instance, while Workspace uses conversation-isolated Agent instances. Each primary menu item remains one unified click-and-drag target, with better tolerance for small pointer movement.
- 修复 React 隐藏并恢复变更审阅区后 Monaco 仍复用已销毁编辑器、触发 `InstantiationService has been disposed` 并让整个页面进入错误边界的问题；单栏与并排 diff 现在都会在 effect 重连时创建新编辑器，同时保留并安全回收审阅模型。
- Fixed Monaco reusing a disposed editor after React hides and restores the change-review surface, which raised `InstantiationService has been disposed` and sent the page into its error boundary. Unified and split diffs now create a fresh editor when effects reconnect while retaining and safely releasing review models.
- 修复写作目录中的当前细纲缺少分区标题、条目字号与行高偏大的问题；现在显示双语“当前细纲”小标题，并使用 11px 紧凑文件行，在窄栏中保持完整或自然截断。
- Fixed the current chapter outline in the Writing directory lacking a section label and using an oversized row. It now has a bilingual Current Chapter Outline label and a compact 11px file row that remains readable or truncates naturally in narrow rails.
- 修复写作工作台的项目结构栏可被拖拽折叠到完全不可见、输入动作菜单中 Plan 快捷键提示未与右侧信息对齐，以及模型菜单将四档推理强度纵向铺开的问题；可见项目结构栏现在严格保留 180px 最小宽度，推理强度改为单行四档选择。
- Fixed the Writing workspace project rail being draggable until fully hidden, the Plan shortcut hint not aligning with right-side menu metadata, and the model menu stacking four reasoning-effort choices vertically. A visible project rail now keeps a strict 180px minimum, and reasoning effort uses one compact four-option row.
- 修复同一浏览器打开多个 Denova 标签页时，每页各自持有 workspace SSE 并耗尽 HTTP/1.1 连接池、进而阻塞新页面加载的问题。工作区文件事件现在由同源 SharedWorker 统一持有一条连接并通过 MessagePort 分发；新标签页仍会获得权威 resync，最后一个订阅退出后连接自动释放，远程访问认证也保持可恢复。
- Fixed new Denova pages being blocked after same-browser tabs each occupied a workspace SSE connection and exhausted the HTTP/1.1 pool. A same-origin SharedWorker now owns one workspace event stream and fans changes out over MessagePorts; newly joined tabs still receive an authoritative resync, the stream closes after the last subscriber leaves, and remote-access authentication remains recoverable.
- 修复叙事风格的「注入位置」选择器在窄布局中同时显示说明而溢出，以及图像规则的长英文标签将启用开关挤出列表的问题；紧凑选择器现在只显示位置名，规则元数据会在可用宽度内截断。
- Fixed the Narrative Style injection-target selector overflowing narrow layouts by showing only the target name in its compact trigger, and fixed long English image-rule metadata pushing the enabled switch outside the rule rail. Rule metadata now truncates within the available width.
- 修复 Agent 流结束或刷新后 thinking 与正文各自聚合、打乱原始时间顺序的问题：写作与游戏共用的展示记录现在按真实事件边界持久化 thinking、正文和工具卡片，为流式与历史 part 复用稳定分段 ID，并仅在分段完整重建同一 Run 正文时从展示层隐藏聚合副本，模型上下文仍保留完整规范回复；Plan 模式继续只恢复结构化计划，SubAgent 分段也保持原顺序。既有已完成会话没有保存分段边界，无法可靠反推，仍按旧格式展示。
- Fixed Agent reasoning and prose being regrouped by type after stream completion or refresh, which reordered the original timeline. The shared Writing/Game display transcript now persists reasoning, prose, and tool cards at their real event boundaries, reuses stable segment IDs across live and historical parts, and hides a Run's aggregate display copy only when the segments reconstruct it completely, while retaining the canonical reply for model context. Plan mode still restores only structured plans, and SubAgent segments retain their order. Existing completed sessions did not store segment boundaries and therefore remain in the legacy layout.
- 修复写作 Agent 请求从提交阶段进入流式响应时未重新读取运行投影，导致首次查询早于后端 Task 注册后，Stop 按钮可能整轮保持禁用的问题；响应流建立后现在会再次获取精确 operation ID，中断仍只通过有作用域的 Abort 执行。
- Fixed Writing Agent Stop sometimes remaining disabled for an entire run when the first runtime-projection read raced server-side Task registration. The projection is now refreshed as the response begins streaming to obtain the exact operation ID, while cancellation continues to use only operation-scoped Abort.
- Windows 目录持久化统一收敛到共享文件系统 module：普通文件仍在发布前完整同步，Windows 跳过不受支持的只读目录 `FlushFileBuffers`，其他平台继续同步目录元数据；正文评论、会话、Automation、自动保存冲突、资料库、版本恢复、游戏存储和工作区变更不再各自维护兼容分支，也不会因目录同步误报保存失败。
- Windows directory durability now lives in one shared filesystem module: regular files remain fully synced before publication, Windows skips unsupported `FlushFileBuffers` calls on read-only directory handles, and other platforms still flush directory metadata. Document comments, sessions, Automations, autosave conflicts, lore, version restore, Game storage, and workspace changes no longer carry separate compatibility branches or falsely report successful writes as failures.
- 修复用户级 SearXNG 地址省略协议时被静默忽略，以及公开实例禁用 JSON 后只能回退到 DuckDuckGo/Bing 的问题：裸域名默认补全 `https://`，JSON 被拒绝或直接返回 HTML 时会复用响应或改用 SearXNG HTML POST 表单；403、非 SearXNG 页面与反机器人挑战会作为有端点来源的诊断随降级结果返回。
- Fixed user-level SearXNG hosts without a URL scheme being silently ignored and public instances falling through to DuckDuckGo/Bing when JSON is disabled. Bare hosts now default to `https://`; HTML returned to the JSON request is reused, otherwise search falls back to the SearXNG HTML POST form. HTTP 403, non-SearXNG pages, and anti-bot challenges remain endpoint-attributed diagnostics alongside fallback results.
- 修复 Bing 相关性检查把整批结果拼接判断、让一条命中结果放行无关噪音，并把全部过滤误报为 provider 故障的问题：现在逐条过滤、允许结果省略年份等次要限定；空结果与全部 provider 不可用分别返回 `no_results` / `providers_unavailable`、明确的重试策略及中英文下一步建议，Agent 不再原样连续重试。
- Fixed Bing relevance checks evaluating a concatenated result batch, allowing one match to admit unrelated noise, and reporting a fully filtered response as a provider failure. Results are now filtered individually while tolerating omitted date-like qualifiers; empty results and total provider outages return distinct `no_results` / `providers_unavailable` statuses, explicit retry strategies, and bilingual next actions so Agents do not immediately repeat the same call.
- 修复 `web_fetch` 对 403、JavaScript shell、Jina 外层 200/目标 403、托管服务故障及浏览器失败只返回裸错误，导致 Agent 原样重试的问题：结果现在包含逐层 `attempts`、稳定状态、重试策略和中英文下一步建议；网页工具卡会直接显示本地化恢复摘要，真实工具错误也会在折叠态露出原因。
- Fixed `web_fetch` returning opaque errors for HTTP 403, JavaScript shells, Jina outer-200/target-403 responses, hosted-service outages, and browser failures, which led Agents to repeat the same call. Results now include per-layer `attempts`, stable statuses, retry strategies, and bilingual next actions; Web Access tool cards show a localized recovery summary, while actual tool errors expose their reason even when collapsed.
- 修复 `web_fetch` 的 Rod 浏览器只按导航 403 判定失败、且普通无头浏览器遇到反自动化挑战后没有二次渲染的问题：浏览器现在会在独立上下文中使用 `go-rod/stealth` 再试一次，并以最终 DOM 是否仍是验证页、正文是否足够完整作为结果依据；成功跨过挑战但保留 403 导航状态的页面也可提取正文，诊断会明确标注 stealth 兜底。
- Fixed `web_fetch` treating every Rod navigation HTTP 403 as terminal and lacking a second rendering attempt after an anti-automation challenge. The browser now retries in a fresh isolated context with `go-rod/stealth`, then judges the final DOM by challenge markers and substantive readable content; pages that complete the challenge while retaining a 403 navigation status can still be extracted, with the stealth fallback recorded in diagnostics.
- 游戏模式的临时流式回合切换为已落盘历史时，现在会优先沿用同一个稳定渲染标识；用户输入和 AI 正文不再在流结束的瞬间整行卸载、重新挂载并重播进场动效，避免看起来像页面 reload 的抖动。
- Game Mode now preserves the same stable render identity when an optimistic streamed turn becomes canonical history. The player input and AI prose no longer unmount, remount, and replay their entrance animation at stream completion, removing the reload-like page jolt.
- 修复全局 `button/input/textarea/select { font: inherit }` 以非分层规则压过 Tailwind `@layer utilities`，导致表单控件上的 `font-mono`、`font-medium` 等字体工具类整体失效的问题；该 reset 已移入 `@layer base`，Raw 源码编辑等场景现在能正确渲染等宽字体，技能页 Raw 编辑同步受益。
- Fixed the global `button/input/textarea/select { font: inherit }` reset being unlayered and overriding Tailwind `@layer utilities`, which silently disabled font utilities like `font-mono` and `font-medium` on all form controls. The reset now lives in `@layer base`, so monospace rendering works for Raw source editing, including the Skills page Raw editor.
- 修复 Agent 流式正文、thinking 与子 Agent 会话在新增一行时底边先向下闪动、随后再被锁底滚动抬回的问题；写作、游戏、配置、自动化及独立子 Agent 对话现在会在实际虚拟行提交布局时同步补偿高度增量，同一节点短暂解绑 ref 也不会丢失高度基线，异步图片和字体变化仍由尺寸观察兜底，同时保留用户上滚后不被抢回底部的行为。
- Fixed a one-frame bottom-edge jump when streamed prose, reasoning, or sub-agent output gained a line. Writing, Game, Config, Automation, and standalone sub-agent conversations now compensate growth in the actual virtual-row layout commit, retain the height baseline across transient ref releases, and keep resize observation as a fallback for asynchronous image or font changes, while preserving manual scroll-away behavior.
- 修复原生 Agentic Loop 在运行 goroutine panic 时可能先关闭事件迭代器、首帧前流错误无法重试、零帧流被当作成功、流错误已经暴露后仍浪费供应商重试、并行工具中断丢失已完成兄弟结果、工具中断被当成普通 JSON 继续运行，以及排队等待 workspace 独占权的工具在取消后仍可能启动的问题；panic、stream error、interrupt 和 cancel 现在都有唯一且可审计的终态。
- Fixed native agentic-loop cases where a recovered run panic could close the iterator before publishing its error, pre-first-frame stream failures could not retry, empty streams looked successful, retries continued after exposed stream failures, parallel interruption discarded completed sibling results, tool interruption became ordinary JSON, or a queued workspace tool started after cancellation. Panics, stream failures, interruptions, and cancellations now each have one auditable terminal outcome.
- 修复外部 Agent Host 可通过自报更大上限绕过调用方上下文预算、取消请求接受未知模式位，以及非标准 OpenAI 兼容端点的文本工具调用在多轮中重复合成 ID 的问题；公共 `agent` 现在在自有 seam 执行响应预算与取消模式校验，adapter 生成跨响应和进程唯一的工具调用 ID。
- Fixed external Agent Hosts being able to widen caller-owned context budgets, cancellation requests accepting unknown mode bits, and textual tool calls from non-standard OpenAI-compatible endpoints reusing synthetic IDs across turns. Public `agent` now enforces response budgets and cancellation modes at its own seam, while the adapter generates IDs unique across responses and processes.
- 修复 OpenAI 兼容 adapter 过滤心跳时同时吞掉 SSE 空事件边界、导致有效流被读取为零帧的问题；过滤现在按完整事件工作，保留普通数据与 `[DONE]` 边界，并覆盖 CRLF、逐字节分片、上游错误和下游提前关闭。
- Fixed the OpenAI-compatible adapter swallowing SSE event boundaries while filtering heartbeats, which could turn a valid stream into zero frames. Filtering now operates on complete events, preserves normal data and `[DONE]` boundaries, and covers CRLF, byte fragmentation, upstream errors, and early downstream close.
- 写作编辑器不再把尚未结束的本地保存回灌误判为并发修改；保存期间继续输入并手动保存时，最新草稿会按新 revision 立即排队，且旧快照完成后仍准确显示未保存状态。
- The Writing editor no longer mistakes an in-flight local save echo for a concurrent edit. Typing and manually saving during persistence now queues the latest draft against the new revision, while an older acknowledgement keeps the unsaved status accurate.
- 文件成功落盘后会立即结束编辑器保存状态；章节字数统计改为后台合并刷新，不再让统计扫描阻塞手动保存或堆积并行请求。
- The editor now finishes its saving state as soon as the file is durably written. Chapter statistics refresh in a coalesced background task instead of blocking manual saves or piling up parallel scans.
- 普通文件保存不再同步检查或创建 Git 版本；保存期间触发的 Git 自动版本会串行后台执行，继续编辑只会重置下一轮空闲计时，不再阻塞保存响应。
- Ordinary file saves no longer synchronously check or create Git versions. Automatic Git versions run serially in the background, and edits made during a version operation only reset the next idle cycle instead of blocking the save response.
- 本地草稿与外部版本真实重叠时会保留双方版本并暂停自动保存；用户明确选择保留合并结果或载入工作区版本后才继续，非重叠修改仍自动合并。
- When a local draft truly overlaps an external version, both versions are preserved and Auto Save pauses until the user keeps the merged result or loads the workspace version. Non-overlapping edits still merge automatically.
- 修复 Game 已落盘回合在刷新后仍把保留的已结算展示 Task 当作活动运行、从零游标重放最新一轮的问题；普通终态现在直接使用规范 Story 历史，显式恢复边界仍会精确重连。
- Fixed Game refreshes treating a retained settled display Task as active and replaying the latest persisted turn from cursor zero. Ordinary terminal views now use canonical Story history directly, while explicit recovery boundaries still reattach exactly.
- 修复内置 `write_todos` 缺少显式 Tool Descriptor、在首次模型请求前被安全校验终止而无法对话的问题；原生 Agent 与完整创作工具面的运行测试现覆盖该边界，并校验写作、游戏、后台导演、配置管理、图像、自动化和网页搜索的全部具体工具均已声明恢复契约。
- Fixed Creation Agent conversations being terminated before the first model request because the built-in `write_todos` tool lacked an explicit Tool Descriptor. Native Agent and complete Writing tool-surface tests now cover the runtime boundary, while every concrete Writing, Game, Director, Config Manager, Image, Automation, and web-search tool is checked for a declared recovery contract.
- 修复 execution runtime 在 emit、提交或回调 panic/提前返回时未取消 Cycle 子上下文，以及已完成/启动失败的展示 Task 关闭 `Done` 后仍保留活动上下文的问题；桥接控制、Agent 子任务和 `AfterFunc` 现在都会随精确生命周期 seam 释放。
- Fixed Cycle child contexts surviving execution runtime emit/commit/callback panics or early returns, plus completed or start-rejected display Tasks publishing `Done` while retaining a live context. Control bridges, Agent child work, and `AfterFunc` callbacks now release at the exact lifecycle seam.
- 修复 Agent 冷恢复在结构操作已落盘、但规范 Session / Story 刷新失败后丢失精确重试入口，以及旧完成 Task 被重复挂接、连续队首恢复动作跨 Task 交接、取消后继 NextTurn 时复用旧 Abort 身份等问题；恢复义务现在作为服务级准入栅栏保留到规范投影成功，写作与游戏始终沿用同一可观察 Task，并按当前 operation 生成控制身份。
- Fixed Agent cold recovery losing its exact retry after a durable structural operation succeeded but canonical Session refresh failed, reattaching settled Tasks, replacing the display Task between consecutive head-of-line actions, and reusing a stale Abort identity for a successor NextTurn. Recovery obligations now remain service-owned admission fences until canonical projection succeeds; Writing and Game keep one observable Task and derive controls from the current operation.
- 修复普通 Session 消息、展示卡片、中断与上下文记录和独立恢复实例的 canonical commit 未共享追加租约，可能让两次写入都成功却把本地 journal offset 留在另一条 JSON 中间的问题；所有持久 Session mutation 现在统一按“跨进程文件租约 → 实例锁 → 增量刷新 → CAS / 对账 → append”串行，后续恢复与下一轮不会被已成功的并发写毒坏。
- Fixed ordinary Session messages, display cards, interruptions, and context records appending outside the canonical-commit lease used by independently loaded recovery Sessions. Every durable Session mutation now follows one file-lease → instance-lock → incremental-refresh → CAS/reconciliation → append path, preventing two successful concurrent writes from leaving a projection offset inside another JSON record and poisoning later recovery.
- 修复 Agent 在读取历史后、提交用户输入前发生上下文结构变化时，可能把旧模型输入绑定到新 revision 的竞态；会话现会原子快照有效消息、压缩记录与 cursor，并以 CAS 拒绝过期提交。另新增不可关闭的完整供应商输入硬上限（默认 4 MB，按 Agent 可配置）与压缩器分层摘要，关闭语义压缩或遇到超长历史时也不会向模型发送无限上下文或在单次摘要请求中装入全部来源。
- Fixed a race where structural context could change after history was read but before user-input publication, binding stale model input to a newer revision. Sessions now atomically snapshot effective messages, compaction, and cursor, then reject stale publication by CAS. A non-disableable complete provider-input hard limit (4 MB by default, configurable per Agent) and layered compactor summaries also prevent unbounded model inputs when semantic compaction is disabled or history is exceptionally large.
- 修复持久化 Agent Runtime 在并发重试、关闭与日志模糊写入时可能丢失已接受 TurnSpec、悬挂队列输入或错误覆盖终态的问题；注册租约、单项 NextTurn、取消释放、append 失败重放与 abort 优先级现在都有明确状态机覆盖。
- Fixed durable Agent Runtime edge cases where concurrent retries, shutdown, or ambiguous journal writes could lose an accepted TurnSpec, retain a cancelled queued input, or overwrite a terminal state. Registration leases, one pending NextTurn, cancellation release, append-failure replay, and abort precedence now have explicit state-machine coverage.
- 修复工作区切换、Agent Task 注册与 SSE 恢复之间的竞态：任务先原子绑定资源再运行，切换会阻止新任务并等待旧任务退出，重连必须携带精确 task ID；写作 UI 流采用完整重放，游戏原始 SSE 使用独立显示游标。
- Fixed races across workspace switching, Agent Task registration, and SSE recovery. Tasks bind resources atomically before execution, transitions fence new tasks and await old exits, reconnects require an exact task ID, Writing UI streams use full replay, and Game raw SSE uses a separate display cursor.
- 修复游戏回合已原子替换但目录同步返回错误时被误报失败的问题，并为“规范回合 → 后台 Director”增加持久化回执与下一 cycle 执行屏障；崩溃恢复、Follow Up、Steer 与重生成都不会跳过或重复同一派生维护。
- Fixed false failures when a Game turn's atomic replacement succeeded but directory sync reported an error, and added a durable receipt plus next-cycle execution barrier from canonical turns to the background Director. Crash recovery, Follow Up, Steer, and regeneration no longer skip or repeat the same derived maintenance.
- 修复后台 Director 使用临时内存 Runtime、且 Agent 成功结算早于最终规划落盘的问题；Director 现在复用 App 持有的持久 Runtime，并以 command / operation / cycle、内容 hash 和规划 revision 原子记录 output commit 回执。取消不会发布草稿，精确重试会复用原回执，迟到的运行错误也不会覆盖已提交规划。
- Fixed the background Director creating a temporary in-memory Runtime and settling successfully before its final plan was canonical. Director runs now reuse the App-owned durable Runtime and atomically record output-commit receipts with command/operation/cycle identity, content hash, and plan revision. Cancellation cannot publish a draft, exact retries reuse the original receipt, and late runtime errors cannot overwrite a committed plan.
- 修复 Agent 上下文总预算遗漏包装文本、片段元数据或 UTF-8 边界，以及工具错误链可绕过大小限制的问题；所有模型可见片段和工具成功/失败结果现在都按实际字节统一有界。
- Fixed Agent context totals that omitted wrappers, fragment metadata, or UTF-8 boundaries, plus tool error chains that could bypass result limits. Every model-visible fragment and successful or failed tool result is now bounded consistently by actual bytes.
- 修复 Agent 兼容事件流在 `emit` 与 `finish` 并发时可能向已关闭 channel 发送而 panic 的问题；慢订阅者现在会被明确断开并通过完整快照重连，不再保持连接却静默丢失单个事件。
- Fixed a possible send-on-closed-channel panic when legacy Agent stream emission raced with task settlement. Lagging subscribers are now explicitly disconnected and recover through the complete replay snapshot instead of silently losing individual events while staying connected.
- 修复游戏 Follow Up 排队后仍绑定旧 branch head 导致的 CAS 冲突，以及 command 重试为同一逻辑输入生成不同 TurnSpecRef 的幂等失效；排队中、已消费重试和同 ID 异输入冲突均有覆盖测试。
- Fixed queued Game follow-ups retaining a stale branch head and failing CAS, plus command retries generating different TurnSpecRefs for the same logical input. Tests cover queued retries, consumed retries, and conflicting payloads under one command ID.

## [v0.3.1] - 2026-07-23

### Changed

- 书籍设定的缺失文件现在在快捷按钮和管理列表中都以虚线轮廓与低饱和状态图标呈现；文件仍可点击，以便按需请求创作 Agent 创建。
- Missing Book Settings files now use a dashed outline and a muted status icon in both shortcuts and the management list; they remain selectable so users can ask the Creation Agent to create them when needed.
- 示例配置的默认服务端口更新为后端 `8011`、前端 `5174`。
- The sample configuration now defaults to backend port `8011` and frontend port `5174`.

### Added

- `./scripts/bootstrap.sh fe` 新增 `--backend-port <port>`，可在前后端独立启动时显式指定 Vite 代理的后端端口。
- `./scripts/bootstrap.sh fe` now accepts `--backend-port <port>` to explicitly set the Vite proxy backend port when frontend and backend start independently.
- Agent Trace 现可一键复制运行 ID，并直接导出该次运行完整的原始 JSONL trace 文件，便于用户向开发者提供可复现的诊断资料；写作与游戏模式共用此入口。
- Agent Trace now supports one-click Run ID copying and download of the complete original JSONL trace for the selected run, making reproducible support diagnostics easy to share; Writing and Game modes use the same entry point.

### Fixed

- 未显式传入 `--port` 时，后端端口冲突会原子地切换到后续可用端口并输出最终地址；显式指定的端口冲突会提供中英文原因和操作提示，等待用户按键后再退出。
- When `--port` is not explicitly supplied, backend port conflicts now atomically fall back to a later available port and print the final address; an explicitly selected port reports a bilingual explanation and waits for user input before exiting.
- 写作 Agent 运行中的 thinking 现在会自动展开；回合结束后仍保持可折叠，避免历史消息持续占据对话区域。
- Running Writing Agent thinking now expands automatically, while completed turns remain collapsible to keep history compact.
- Windows 自动保存不再将已落盘的配置误报为失败：Windows 不支持对配置目录句柄执行同步时，会跳过该目录同步；新增但尚未填写模型名的语言模型草稿也会在自动保存后保留，不再中断编辑。
- Windows autosave no longer reports a persisted configuration as failed when Windows rejects directory-handle synchronization. Newly added language-model drafts without a model name are now retained after autosave instead of interrupting editing.
- 修复游戏模式 Agent 的互动正文候选在异步 TurnResult 提交完成时可能被后续重试正文重复追加的问题；流式与非流式输出现在都只保留按事件顺序消费到的首个正文候选。
- Fixed a Game Agent race where a later retry could be appended to the locked interactive narrative after an asynchronous TurnResult submission; streaming and non-streaming output now retain only the first narrative candidate consumed in event order.

## [v0.3.0] - 2026-07-22

### Brief / 简要说明

#### 中文

- Beta 不兼容：Agent 文件编辑、审阅反馈、游戏回合与状态结构协议已更新；后台 Shell、旧状态结构复审与三项工具结果配置移除，原先依赖 `hidden` 的 Actor 状态会直接可见。
- 写作模式新增持久化 Change Review、正文评论与跨重启 Undo/Redo，并支持编辑器正则替换及带自动备份的工作区全局替换；完整章节修订后会直接同步进度与角色状态。
- 游戏模式新增后台导演运行策略、故事级状态结构策略、可回放的 Actor 归档与恢复、自定义状态布局、已保存回复修正和全屏导演台。
- 统一自动保存、revision 感知的三方合并、工作区变更账本、崩溃恢复与活动任务重连，更可靠地保护长期项目和并发编辑。
- 上下文检查与复制、完整 Trace 展示、更平滑的流式消息，以及 Unicode 规范化安全升级，共同提升跨平台稳定性与问题诊断效率。

#### English

- Beta breaking: Agent editing, review feedback, Game turn, and state-schema contracts changed; background Shell, legacy schema review, and three tool-result settings were removed, and formerly `hidden` Actor state is now visible.
- Writing adds durable Change Review, document comments, restart-safe undo/redo, regex editor replacement, and recoverable workspace-wide replacement; complete chapter revisions now synchronize progress and character state.
- Game adds Director schedules, story-specific schema policies, replayable Actor archive/restore, customizable state layouts, saved-response correction, and a full-screen Director Desk.
- Unified autosave, revision-aware three-way merging, workspace journaling, crash recovery, and active-run reconnection better protect long projects and concurrent edits.
- Context inspection and copying, complete trace display, smoother streaming, and a Unicode-normalization security upgrade improve reliability and diagnosis across platforms.

### Added

- 写作模式新增 Change Review：每轮 Agent 修改都会生成摘要卡，可查看累计或单轮的多文件 Unified / Split Diff、添加行内评论，并执行可跨重启保留的 Undo / Redo。
- Writing Mode adds Change Review: every Agent edit run produces a summary card with cumulative or per-run multi-file Unified / Split diffs, inline comments, and restart-safe undo/redo.
- 正文编辑器新增持久化评论，可从文本选区或行级入口创建、原位编辑和删除；待处理评论会随下一条用户消息作为可信引用交给创作 Agent，发送失败时自动恢复。
- The manuscript editor adds durable comments from text selections or line actions, with in-place editing and deletion. Pending comments become trusted references for the next Agent message and are restored if sending fails.
- 游戏模式新增已保存 AI 回复的手动修正，以及覆盖主舞台的全屏导演台；规划、事件、规则审计和执行过程集中展示，角色与世界变化保留在状态感知侧栏。
- Game Mode adds manual correction of saved AI replies and a full-screen Director Desk over the story stage, centralizing plans, events, rule audits, and execution while actor and world changes remain visible in a state-aware sidebar.
- 顶部书名升级为带封面的书籍快捷切换器，桌面和移动端都可在不离开当前页面、不切换写作/游戏模式的前提下换书；书籍管理与快捷切换共享最近打开或手动拖拽排序。
- The top-bar title becomes a cover-aware book switcher on desktop and mobile. It changes books without leaving the page or changing Writing/Game mode, and shares recent or manual drag ordering with Book Management.
- 新增持久化工作区变更账本，为 Agent 修改保存内容寻址快照和跨文件操作记录；原子写入、启动恢复和显式冲突共同保护 Review、Undo / Redo 与外部编辑。
- A durable workspace-change ledger records content-addressed Agent edits and cross-file operations; atomic writes, startup recovery, and explicit conflicts protect Review, undo/redo, and external edits.

- 写作与游戏模式共用的工作台通知槽新增一次性 GitHub Star 提示：仅在当前会话完成有效 Agent 回合后出现，新版本提醒始终优先；关闭或打开仓库后不再提示，并完整支持中英文、明暗主题、桌面与移动布局。
- The shared Writing and Game workbench notice slot now includes a one-time GitHub Star prompt after a successful Agent turn in the current session. Update notices always take priority; dismissing the prompt or opening the repository keeps it quiet, with bilingual, light/dark, desktop, and mobile support.
- 游戏模式 Actor 状态新增可回放的归档与恢复：敌人死亡或角色永久退场时保留完整历史状态，仅从活动上下文、检定与状态页签中退出；重新登场必须显式恢复。舞台状态面板与导演台提供中英文只读归档索引，并展示归档原因和来源回合。
- Game Mode Actor state now supports replayable archive and restore transitions. Dead enemies and permanently departed characters retain their complete historical state while leaving active model context, rule checks, and state tabs; returning requires an explicit restore. The stage ledger and Director Console expose a bilingual read-only archive index with reason and source-turn provenance.
- 游戏模式新故事开局新增故事级后台导演运行策略：支持“按需自动（推荐）”“仅手动”和“每 X 回合自动”；自动模式会在首回合落盘后先初始化规划，固定间隔从首回合起按配置节奏运行，手动模式仍可从导演台显式触发。
- New Game Mode stories now have a story-scoped Background Director schedule with Automatic When Needed (recommended), Manual Only, and Automatic Every X Turns modes. Automatic modes initialize planning after the first persisted turn, interval mode follows its configured cadence from that opening turn, and manual mode remains explicitly runnable from the Director backstage.
- 上下文分析器新增全上下文、SystemPrompt 区、消息组与来源片段的一键复制；游戏模式的“本轮互动指令与动态上下文”会在保留模型实际收到的完整原文同时，继续按本轮行动、导演本轮规则、`agent-brief.md`、`StoryDirector`、Actor 状态手册、动态策略等来源展开。
- Context Analysis now supports one-click copying for the full context, the SystemPrompt section, message groups, and individual source parts. In Game Mode, the current-turn instruction and dynamic context retain the exact model-visible message while expanding into sources such as the current action, turn-specific director rules, `agent-brief.md`, `StoryDirector`, the Actor state guide, and dynamic strategy prompts.
- 游戏模式的故事线选择器新增批量删除：可在同一面板中多选或全选故事线，查看受影响清单后统一确认删除；操作支持中英文、明暗主题和窄屏布局。
- The Game Mode story picker now supports batch deletion: select multiple or all stories in one panel, review the affected list, and confirm once, with bilingual, light/dark, and narrow-screen support.
- 游戏模式的新故事线配置新增故事级“状态结构”策略：可选择“按模板动态适配”“固定使用模板”或“为故事动态生成”，并在同一区域选择基础状态模板；配置支持中英文、明暗主题与自适应布局。
- New Game Mode story setup adds a story-level State Schema policy with Adapt a Template, Use a Fixed Template, and Generate for This Story modes, plus an integrated base-template picker with bilingual, theme-aware, adaptive UI.
- 状态模板字段新增可选的 `group` 与 `display` 展示提示，状态结构树按“模板 → 分组 → 字段”展示；状态面板新增“自定义布局”，可通过鼠标或键盘拖动分区和字段、跨分区移动字段、在窄屏使用方向按钮并恢复默认。布局按“故事 + 模板”保存在本地 UI 偏好中，同模板 Actor 共享且不会进入模型上下文；Schema 字段数组仅作为兜底顺序，旧 Beta `order` / `display_groups` 输入会被忽略。
- Actor state fields now accept optional `group` and `display` presentation hints, and the state structure tree renders Template → Group → Field nesting. The stage ledger adds a custom layout editor for pointer/keyboard section and field sorting, cross-section moves, narrow-screen direction controls, and reset. Layouts persist locally by story + template, are shared by Actors using that template, and never enter model context; the schema field array is only the fallback order, and legacy Beta `order` / `display_groups` inputs are ignored.
- TRPG 状态绑定的 modifier 与公式项支持可选 `value_path`，用于读取用户自定义 object 中的嵌套数值，并在校验、计算和审计结果中保留结构化来源；内置面板现已使用可直接绑定的普通 number 字段，不依赖该能力。
- TRPG state-binding modifiers and formula terms support optional `value_path` reads from user-defined nested object values, retaining the structured source through validation, computation, and audit output. Built-in panels now use directly bindable number fields and do not depend on this capability.
- 写作模式编辑器查找栏新增替换与正则匹配：可展开替换输入框，支持替换当前匹配或全部替换；开启正则后查找与替换均按正则表达式执行，替换文本支持 `$1` 等捕获组引用。
- The Writing Mode editor search bar now supports replace and regex matching: expand a replace field to replace the current match or all matches; with regex enabled, both find and replace use regular expressions, and the replacement text supports capture group references like `$1`.
- 写作模式全局搜索新增正则匹配与全局替换：搜索面板可切换正则模式（RE2 语法、大小写敏感，非法正则内联提示），并可展开替换行对整个工作区执行全部替换；替换文本支持 `$1`、`$&`、`$<name>` 捕获组引用（与编辑器内替换语义一致），执行前自动创建“全局替换前自动备份”可恢复版本，替换期间被并发修改的文件会安全跳过并提示。
- Writing Mode global search now supports regex matching and global replace: the search panel can toggle regex mode (RE2 syntax, case-sensitive, with inline feedback for invalid patterns) and expand a replace row to replace all matches across the workspace. Replacement text supports `$1`, `$&`, and `$<name>` capture group references (consistent with in-editor replace); a restorable “before global replace” version is created automatically, and files changed concurrently during the replace are safely skipped and reported.

### Changed

- Beta 不兼容：`edit_file` 改为单文件批量协议 `{file_path, edits[]}`，文件 revision 不再暴露给 Agent；写入由服务端以精确内容 revision 原子校验。同轮只读工具仍可并行，写工具与前台 Shell 按工作区串行；后台 Shell 模式不再支持。
- Beta breaking: `edit_file` now uses the single-file batch contract `{file_path, edits[]}`, and file revisions are no longer exposed to the Agent. The server atomically validates writes against exact content revisions. Read-only tools may still run in parallel, while write tools and foreground Shell are serialized per workspace; background Shell is no longer supported.
- Beta 不兼容：聊天请求的 `review_feedback` 改为按可信账本来源分组的数组；Diff 与正文评论统一只保留编辑和删除，不再提供 Resolve / Reopen，旧 `resolved` 标记按普通未删除评论读取。
- Beta breaking: chat request `review_feedback` is now an array grouped by trusted ledger source. Diff and document comments keep Edit and Delete only; Resolve/Reopen is removed, and legacy `resolved` markers are read as ordinary non-deleted comments.
- Beta 不兼容：游戏正文 Agent 使用统一的 `submit_interactive_turn` 提交状态变化与选项，以结构化 Actor / 字段 ID 替代模型生成的 JSON Pointer；各模块独立验收并只重试失败部分，旧工具名仅用于历史展示。
- Beta breaking: the Game Agent now submits state changes and choices through `submit_interactive_turn`, using structured actor and field IDs instead of model-authored JSON Pointers. Modules are accepted independently so only failed parts retry; legacy tool names remain for historical display only.
- 游戏正文 Agent 改用从有效 schema 与当前分支状态生成的有界状态手册，明确当前值、类型约束和更新规则；正文后的 thinking、工具调用与重试统一折叠为可展开 Trace，写作、自动化和导演台保持一致。
- The Game Agent now receives a bounded state handbook generated from the effective schema and current branch state, including current values, constraints, and update rules. Thinking, tool calls, and retries after prose collapse into one expandable trace consistently across Writing, Automations, and the Director Desk.
- 资料库、方案预设和 Skills 统一资源目录的搜索、分组、计数、新建与空状态；资料库正文使用与章节一致的所见即所得 Markdown 编辑器，并可在渲染内容中直接搜索、高亮和继续编辑。
- Lore, Presets, and Skills now share consistent resource-directory search, grouping, counts, creation, and empty states. Lore content uses the same WYSIWYG Markdown editor as chapters, with search and highlighting directly in editable rendered content.
- 自动化不再为新工作区预置停用任务；“续写章节”和“自动 Review”改为新建时主动选择的双语模板。升级仅清理从未使用的旧种子，并保留所有已有使用痕迹的任务。
- New workspaces no longer receive disabled automation tasks. Continue Writing and Automatic Review are now bilingual templates selected explicitly during creation; upgrades remove only untouched legacy seeds and preserve every task with usage history.
- 设置页移除“用户配置 / 当前工作区”双层切换，通用、写作和游戏偏好统一保存为用户配置；工作区只保留 Agents 页明确提供的定制。写作与游戏输入区的模型菜单同时提供 reasoning effort 快捷选择。
- Settings removes the User/Current Workspace split: common, Writing, and Game preferences are user-scoped, while workspace customization is limited to options explicitly exposed on the Agents page. The shared model menu also provides quick reasoning-effort selection.
- 删除 Skill、恢复内置 Skill、放弃未保存修改和删除空剧情线等操作统一使用支持异步错误提示的应用内确认弹窗；从源码运行的依赖现在明确包含 `ripgrep`。
- Destructive actions such as deleting or restoring Skills, discarding drafts, and deleting empty branches now use in-app confirmation with asynchronous error feedback. Source builds now explicitly require `ripgrep`.

- 写作模式简化章节状态同步：新写完整章节或实质性改写章节时，Agent 在完成本轮修订后直接同步 `setting/progress.md` 与 `setting/character-states.md`；“初稿 / 成章”仅保留为 UI 编辑标记，不再作为下一章判断、上下文选择或状态同步的门槛，实际章节路径与非空正文优先于进度摘要。
- Writing Mode simplifies chapter-state synchronization: after writing a complete chapter or making a substantive story rewrite, the Agent now updates `setting/progress.md` and `setting/character-states.md` after the run's final revision. Draft/Final remains a UI editing label only and no longer gates next-chapter selection, context, or state synchronization; actual chapter paths and non-empty chapter content take precedence over the progress summary.
- Agent 工具上下文收敛为单一边界：工具结果仅在执行完成时按全局 `agent_tool_result_limit_kb` 有界化，后续轮次原样保留有效的 call/result 对，旧历史交给普通上下文压缩；`read_lore_items` 仍使用来源回执避免重复正文。Beta 不兼容：移除 `tool_result_keep_recent`、`tool_result_context_budget_kb` 与 `tool_result_preview_chars`，旧配置键读取时忽略。
- Agent tool context now has one size boundary: results are bounded once at completion by the global `agent_tool_result_limit_kb`, valid call/result pairs remain exact across subsequent turns, and normal context compaction owns older history; `read_lore_items` still uses source receipts to avoid duplicating bodies. Beta breaking: `tool_result_keep_recent`, `tool_result_context_budget_kb`, and `tool_result_preview_chars` are removed, and legacy keys are ignored when read.
- 游戏模式移除单次 `state_changes` 的操作数量硬上限，工具 Schema、提交解码、状态编译与 `TurnResult` 不再因第 25 项或更多操作拒绝复杂回合。Agent 仍会收到“常规回合建议不超过 24 项”的软提示，并明确复杂开局或确有更多事实变化时可以超过。
- Game Mode removes the hard operation-count limit from one `state_changes` submission, so the tool schema, submission decoder, state compiler, and `TurnResult` no longer reject complex turns at the 25th operation or beyond. The Agent still receives soft guidance to keep routine turns at 24 changes or fewer, with an explicit allowance for complex openings or genuinely larger fact changes.
- 游戏模式新建故事线配置页重构为紧凑布局：名称、导演、目标字数与行动建议数量合并为同一自适应行，继承导演模块的四个选择器并排一行，分区标题与说明同行展示，底部操作改为吸底栏，滚动时始终可见。
- Game Mode's new story setup panel is rebuilt as a compact layout: name, director, target length, and choice count share one adaptive row, the four inherited-module pickers sit in a single row, section titles show inline with their descriptions, and the action footer stays pinned at the bottom while scrolling.
- 游戏模式简化 Actor State Schema：字段与特质移除 `visibility`，旧数据中的 `visible` / `hidden` / `spoiler` 会在读取时忽略，所有历史状态均按普通可见信息进入状态面板与 Agent 上下文；结构初始化同时移除 `evidence_kind`，继续保留 `source`、`requirement` 与 `reason`。Beta 不兼容：原先依赖 `hidden` 隐藏的状态将直接可见，幕后规划应维护在 Director 私有文件中。
- Game Mode simplifies the Actor State Schema by removing `visibility` from fields and traits. Legacy `visible`, `hidden`, and `spoiler` values are ignored on read, so all historical state is treated as ordinary visible information in the state panel and Agent context. Schema initialization also removes `evidence_kind` while retaining `source`, `requirement`, and `reason`. Beta breaking: state previously hidden with `hidden` becomes directly visible; private future planning belongs in the Director's private files.
- 游戏模式输入栏的末端操作统一按“模型选择 → 行动选择 → 发送”排列，让模型入口固定在左、行动选择位于中间，并与写作模式的模型优先顺序保持一致。
- Game Mode composer actions now follow Model → Choices → Send, keeping model selection on the left, action choices in the middle, and the model-first order consistent with Writing Mode.
- 游戏模式状态面板的 Object 字段不再把嵌套对象与数组拼成一行文字；叶子项改用紧凑的 `key: value` 列表，只有下一层对象才缩进，长内容可使用完整字段宽度换行，并保持明暗主题一致。
- Object fields in the Game Mode state panel no longer flatten nested objects and arrays into one text line. Leaf entries now use compact `key: value` lists, only deeper objects are indented, long content wraps across the full field width, and light/dark theming stays consistent.
- 状态结构初始化从后台 Director 完整移交给前台 Game Agent：动态模式先通过专用结构工具暂存模板/字段，再通过 `state_changes` 初始化 Actor 与值；结构、开局正文、初始状态和行动建议在首回合一次性原子提交，失败不会留下半成品。固定模板模式不暴露结构工具，开局前返回配置页会安全重建冻结结构，首回合提交后策略即锁定。Beta 不兼容：旧故事统一固定使用已有冻结结构，不再启动 Director 迁移或复审；旧 `state_schema_adaptation_mode` 配置、后台运行 API 和复审操作已移除。
- State-schema initialization moves completely from the background Director to the foreground Game Agent: dynamic modes stage templates and fields through a dedicated tool, then initialize Actors and values through `state_changes`. Schema, opening prose, initial state, and choices commit atomically with the first turn, leaving no partial result on failure. Fixed mode exposes no schema tool; returning to setup before the opening safely rebuilds the frozen schema, and the policy locks after the first turn. Beta breaking: old stories keep their existing frozen schema and never start a Director migration or review; the legacy `state_schema_adaptation_mode`, background run APIs, and review actions are removed.
- 方案预设刷新后默认定位到配置管理 Agent，同时展开当前模式下的全部资源分组，并按“故事导演 → 叙事风格 → 状态系统 → TRPG 检定 → 图像方案 → 事件包”排列；用户仍可单独折叠分组或一键收起全部。
- Presets now open on Config Manager Agent after refresh, expand every resource group visible in the current mode, and order them as Story Director → Narrative Style → State System → TRPG Checks → Image Presets → Event Packages; users can still collapse individual groups or collapse all at once.
- 五套内置状态系统预设重构为精简的集中式结构：只预建故事、主角和世界实体三个 Actor；场景、世界与任务归入故事状态，地点与势力归入世界实体，技能、物品和关系归入所属角色。“面板”和“状态”改为普通字段分组，不再要求用户编辑纯 JSON；默认 TRPG 提供等级、六维、攻击 AC、防御 DC、生命与法力，修仙、西幻、末世和无限流分别生成符合题材的字段，末世不会制造空面板。故事状态移除场景要素、生效规则和世界背景三个重复字段，并新增可直接承接下一段的“可承接钩子”。
- The five built-in actor-state presets now use a lean centralized structure with only three initial Actors (story, protagonist, and world entities). Scene, world, and quests live in story state; locations and factions share the world entity; abilities, items, and relationships live on their owning Actor. Panel and State are now groups of ordinary fields instead of raw JSON objects. Default TRPG provides level, six attributes, attack AC, defense DC, health, and mana, while cultivation, Western fantasy, apocalypse, and infinite-flow each generate setting-appropriate fields; apocalypse creates no empty panel. Story state removes the redundant Scene Elements, Active Rules, and World Background fields and adds a directly actionable Continuation Hook.
- 状态布局拖拽按分区与字段分别计算碰撞目标，拖动时使用本地预览与独立浮层、松手后仅持久化一次，并为清空后的分区保留明确放置区域，减少嵌套拖动抖动和跳位。
- State-layout dragging now resolves section and field collisions separately, uses a local live preview and independent drag overlay, persists only once on drop, and keeps an explicit drop zone for emptied sections to reduce nested-drag jitter and position jumps.
- 兼容性说明：内置状态预设的 Beta Schema 变更只影响新故事及主动恢复内置预设的配置；已有故事继续使用冻结的故事级 Schema，不会自动重写状态。
- Compatibility note: this Beta schema change affects new stories and configurations explicitly restored to a built-in preset; existing stories continue using their frozen story-local schema and are not rewritten automatically.
- 游戏模式正文后的状态面板按分区一页平铺，每个分区使用带图标标题与字段数的独立区块，组内使用自适应多列网格；默认分区来自字段形状及 `group` / `display` 提示，用户布局可覆盖最终顺序。预览态展示当前布局排序最前的两个分区并可一键展开全部；世界状态的对象值会展开为独立字段参与分组，空世界页签不再渲染。
- The state panel after game-mode prose lays sections out on one page with icon headers, field counts, and adaptive grids. Default grouping comes from value shape plus `group` / `display` hints, while the user's layout controls final ordering. Preview shows the first two sections in the current layout with one-click expansion; record-valued world facts expand into individual grouped fields, and empty World tabs are omitted.
- 本回合状态变化收敛为头部一行摘要（数值 ±delta、增删项、已更新），变更字段以左侧色条和值旁 chip 标记，不再为每个字段重复展示“本回合已更新”说明行。
- Turn state changes now collapse into a one-line header summary (numeric ±delta, added/removed items, updated) with changed fields marked by a left accent bar and an inline chip, replacing the repeated per-field “updated this turn” notes.
- 兼容性说明：状态显示偏好为“默认预览/默认展开/默认折叠/仅导演台”四档，预览态由旧的按高度裁剪改为按分区切割；旧版“默认显示”偏好自动并入“默认展开”。
- Compatibility note: the state display preference offers Preview/Expanded/Collapsed/Director-only, and the preview mode now cuts by section instead of clamping height; the short-lived "Visible" preference migrates to "Expanded" automatically.
- 自动保存统一到一套 after-delay 内核与场景适配层：仅在用户停止编辑达到配置延迟后写入，同一资源最多一个请求执行中，并把后续修改合并为最新待保存快照；手动保存、切换前 flush 与自动保存共享同一队列。
- Autosave now uses one after-delay core with thin scenario adapters: writes start only after the configured quiet period, each resource permits one in-flight request, later edits collapse to the latest pending snapshot, and manual save plus navigation flush share that queue.
- 资料库与方案预设移除重复的写作工作区标题栏，关闭入口并入当前资源工具栏；配置管理 Agent 统一置于目录搜索之前，资料库不再重复展示目录说明。
- Lore and Presets no longer show a duplicate writing-workspace title bar; closing now lives in the active resource toolbar, Config Manager is consistently placed before directory search, and Lore no longer repeats its directory description.
- Agents 与 Skills 的配置 Agent 面板现在支持键盘和鼠标拖拽调宽，并分别记忆用户调整后的宽度。
- Config Agent panes in Agents and Skills can now be resized with the keyboard or pointer and remember their widths independently.
- Settings、Agents、资料库、方案预设、创作者设定、开场方案、Automations、Skills、共享文风参考和回复目标字数统一改为自动保存，并移除重复的通用“保存”按钮；新建、导入、重命名/迁移、删除及运行时文档提交仍保留为明确操作。
- Settings, Agents, Lore, Presets, creator configuration, opening presets, Automations, Skills, shared style references, and reply-length targets now autosave without redundant generic Save buttons; create, import, rename/move, delete, and runtime-document submission remain explicit actions.
- 配置页共享“等待 / 保存中 / 已保存 / 校验阻止 / 失败重试”状态，并把 Cmd/Ctrl+S 统一为不可见的立即 flush 命令；Skills 文档与目录文件 API 新增精确内容 revision，以保护外部编辑。
- Configuration surfaces share pending, saving, saved, validation-blocked, and retryable-error feedback, while Cmd/Ctrl+S consistently flushes without exposing a button. Skill documents and supporting-file APIs now include exact content revisions to protect external edits.
- 兼容性说明：Automations 更新接口现在必须携带 `base_revision`；配置管理 Agent 的 `write_automations` update 必须携带 `read_automations` 返回的 `revision`。旧调用方需先读取最新任务再更新。
- Compatibility note: Automation updates now require `base_revision`, and Config Manager Agent `write_automations` updates must carry the `revision` returned by `read_automations`. Existing callers must read the latest task before updating it.
- Home、Settings、Agents、Skills 和 Automations 统一使用共享页面框架、分区导航、表单字段、资源目录、空状态与确认弹窗；资料库和方案预设同时复用自适应面板与移动端入口。
- Home, Settings, Agents, Skills, and Automations now share page shells, section navigation, form fields, resource directories, empty states, and confirmation dialogs; Lore and Presets also reuse adaptive panes and mobile entry points.
- 写作与游戏模式的 Agent 对话统一为单一挂载的聊天面板，并共享持久化输入偏好、上下文分析展示、文本测量和底部滚动控制，避免布局切换时重复初始化会话状态。
- Writing and Game modes now use a single-mounted Agent chat pane with shared persisted composer preferences, context-analysis disclosure, text measurement, and bottom-scroll control, avoiding duplicate session initialization during layout changes.
- 互动资源选择器和方案预设编辑器统一使用可访问的选择、字段、分区、JSON 校验和状态组件；删除仅供旧实现自身使用的重复面板与辅助组件。
- Interactive resource pickers and preset editors now share accessible selection, field, section, JSON-validation, and status components; duplicate panels and helpers used only by legacy implementations were removed.
- 自动化左侧任务目录支持按项目独立展开或折叠；折叠后仍保留运行中数量和任务数量，全局任务组使用同一交互。
- The Automations task catalog can expand or collapse each project independently while keeping running and task counts visible; the global task group follows the same interaction.

### Fixed

- 更新故事导演配置的发布回归测试，使其匹配“新故事默认运行方式”的当前双语字段命名，避免有效构建被旧文案断言阻断。
- Updated the Story Director release regression test to match the current bilingual Default New-Story Schedule field, preventing valid builds from being blocked by a stale copy assertion.
- Change Review 的多文件滚动、文件跳转、Diff 选区、评论草稿、面板尺寸和延迟加载更加稳定；后台刷新不再打断输入，窄屏导航和 Skills 工具栏也能自适应展示。
- Change Review now keeps multi-file scrolling, file jumps, diff selection, comment drafts, panel sizing, and lazy loading stable. Background refreshes no longer interrupt input, and compact navigation and the Skills toolbar adapt to narrow screens.
- 正文评论修复多行重叠选区卡死、等价 Markdown 被误判为外部修改、提交闪烁、键盘编辑失效和行级入口难以命中等问题；评论锚点仍会拒绝真正的正文或 revision 冲突。
- Document comments no longer freeze on overlapping multi-line selections, misclassify equivalent Markdown as an external edit, flicker on submit, lose keyboard editing, or make line actions difficult to target; real content and revision conflicts are still rejected.
- Agent 修改到达时不再静默覆盖未保存草稿；编辑器自动保存、Review、Undo / Redo、版本恢复和自动化触发均绑定正确工作区，避免切换作品、乱序事件或符号链接别名导致内容串写、死锁或错误刷新。
- Agent changes no longer silently overwrite unsaved drafts. Editor autosave, Review, undo/redo, version restore, and automation triggers stay bound to the correct workspace, preventing cross-workspace writes, deadlocks, and stale refreshes during switches, out-of-order events, or symbolic-link aliases.
- 旧故事或尚未冻结 schema 的运行时会在缺失时恢复内置初始 Actor；游戏回合提交明确显示 accepted / rejected / pending 与待重试模块，并按故事目标字数约束首段生成，降低正文过长挤占状态提交的概率。
- Legacy stories and runtimes without a frozen schema restore built-in initial Actors when missing. Game turn traces distinguish accepted, rejected, and pending submissions with retry modules, while story target length bounds the opening generation so long prose is less likely to crowd out state submission.
- 修复写作模式发送消息后输入框未清空，以及作品作者字段无法清空的问题；书架会用本地化文案展示空作者。
- Fixed the Writing composer retaining sent content and book metadata refusing to clear the author field; bookshelf cards now show a localized empty-author label.
- 文件、资料、风格、选区与审阅意见引用会随已发送消息原子持久化，失败时恢复；已成功提交给 Agent 的评论不会继续残留在 Diff 中。
- File, lore, style, selection, and review references now persist atomically with sent messages and recover on failure; comments successfully submitted to the Agent no longer remain in the diff.

- 将 `golang.org/x/text` 升级至 `v0.39.0`，修复无效 Unicode 输入可能在 Actor ID 规范化路径中触发无限循环的 `GO-2026-5970` 漏洞。
- Upgraded `golang.org/x/text` to `v0.39.0`, fixing `GO-2026-5970`, where invalid Unicode input could trigger an infinite loop in the Actor ID normalization path.
- 修复 Agent `prefill failed: unexpected control character ... char 2000`：上下文层不再按 JSON 外形猜测并截断或替换工具结果，OpenAI 请求始终把 tool content 作为不透明字符串发送。
- Fixed Agent `prefill failed: unexpected control character ... char 2000`: the context layer no longer guesses from JSON shape or truncates/replaces tool results, and OpenAI requests always send tool content as an opaque string.
- 游戏模式的 `submit_interactive_turn` 现在会在冻结 Schema 校验前，将可无歧义解释的数字、布尔、object 和 list 字符串编码规范为原生 JSON；冲突或模糊值仍会原子拒绝，Object 内部记录则保持原样。一次提交中的独立状态错误会合并到同一回执并精确定位 `initial_state` 字段，工具说明也改用原生 JSON 示例，避免弱模型逐字段重试。
- Game Mode's `submit_interactive_turn` now normalizes unambiguous string encodings of numbers, booleans, objects, and lists into native JSON before frozen-schema validation. Conflicting or ambiguous encodings remain atomically rejected, while entries inside Object values are preserved as submitted. Independent state errors from one submission are returned together with precise `initial_state` paths, and the tool guide uses native JSON examples to avoid field-by-field retries from weaker models.
- 游戏模式切换故事线时会将当前选择写入工作区故事索引，并在加载时优先采用该共享值；同一工作区从其他浏览器打开时会恢复最后选择的故事线，不再被各浏览器独立的本地缓存覆盖。
- Switching stories in Game Mode now persists the current selection in the workspace story index and treats that shared value as authoritative on load. Opening the same workspace in another browser restores the last selected story instead of being overridden by per-browser local cache.
- 状态面板预览不再固定优先“概览/持有”或“面板/状态”，而是严格展示当前排序最前的两个分区；展开全部继续沿用同一顺序，不会改变用户的布局配置。
- The state-panel preview no longer hard-codes Overview/Holdings or Panel/State; it strictly shows the first two sections in the current order, and Expand All preserves that order without modifying the saved layout.
- 流式 thinking 与正文现在统一先以不可见目标内容预留下一帧高度，在消息列表完成锁底后再揭示新增文字；写作、游戏及子 Agent 消息共用同一暂存层，避免新行先出现在视口下方再被瞬间抬升。
- Streaming reasoning and prose now reserve the next frame's target height invisibly and reveal new text only after the message list reaches the bottom. Writing, Game, and sub-agent messages share the same staging layer, preventing new lines from flashing below the viewport before jumping upward.
- 消息列表在没有活跃流式输出时会完全关闭自动锁底与内容增高跟随，状态面板展开、折叠或切换页签不再先跳到底部再复位；流式输出期间的底部跟随、切换会话时的一次性定位和用户显式“回到底部”仍保持原有行为。
- Message lists now fully disable automatic bottom-following and size-growth scrolling while no output is actively streaming, so expanding, collapsing, or switching tabs in the state panel no longer jumps to the bottom and then snaps back. Streaming follow, one-shot positioning after session switches, and explicit Back to Bottom actions retain their existing behavior.
- 游戏流式回合现在按同一有序提交链路处理 thinking、工具参数与正文；正文开始时，已完成的前置思考会立即收进独立执行过程，正文保持在折叠区外，与回合完成后的历史展示一致。流式结束时会取消旧页脚上的延迟锁底任务并保留完成前的视口坐标，状态面板挂载或旧面板卸载造成的布局变化不再被误判为用户上滚，也不会移动阅读位置；滚轮、键盘、滚动条及触摸上滚仍会明确解除自动跟随。所有上下文与输出保持完整，无需新增配置。
- Game streams now commit reasoning, tool arguments, and prose through one ordered update lane. Once prose starts, completed pre-narrative reasoning moves immediately into its own process disclosure while prose remains outside it, matching the persisted completed view. When streaming ends, deferred following against the old footer is cancelled and the pre-completion viewport is preserved, so mounting the new state panel or removing the previous one is neither mistaken for user scrolling nor allowed to move the reading position; upward wheel, keyboard, scrollbar, and touch intent still explicitly releases following. Context and output remain complete, with no new setting required.
- 写作模式创作 Agent 的运行中思考与工具轨迹改为默认收起并保留动态状态提示，仍可按需完整展开；流式消息去重与展示模型现在复用未变化的历史消息引用，避免超长 thinking 持续挂载并触发整段历史 Markdown 重渲染导致浏览器失去响应。游戏模式保留运行中实时展开行为，所有 thinking 与工具内容均未截断。
- In Writing Mode, active reasoning and tool traces now stay collapsed by default with a live status indicator while remaining fully expandable on demand. Streaming deduplication and view projection preserve unchanged historical message identities, preventing very long thinking output from staying mounted and repeatedly re-rendering the entire Markdown history. Game Mode retains its live expanded traces, and no thinking or tool content is truncated.
- 状态面板中的名称型记录统一直接使用可读的 map key 作为 ID，不再生成英文、拼音或 slug 标识；Object 字段只校验根值为对象，不再要求每条记录必须是对象、补写重复的名称字段或校验键名与内部名称一致。已有故事状态不会被自动改写，提示词仍引导 Agent 使用稳定、可读的键。
- Named state-panel records use their readable map keys directly as IDs instead of generating English, transliterated, or slug identifiers. Object fields validate only that the root value is an object; individual entries no longer have to be objects, receive duplicate name fields, or match an inner name. Existing story state is not rewritten, while prompts continue guiding the Agent toward stable, readable keys.
- 游戏模式正文后的状态面板会预先挂载全部 Actor 与世界页签内容，切换页签不再因首次加载重置消息区滚动；展开全部会保持预览分区在原位并将其余分区追加在后，切换页签、展开或折叠等直接查看操作也不会再触发自动锁底。
- The state panel after Game Mode prose now mounts every Actor and World tab up front, preventing first-load scroll resets when switching tabs. Expand All keeps preview sections in place and appends the remaining sections, while direct viewing actions such as switching, expanding, or collapsing no longer trigger bottom-following.
- 游戏新故事开局会按独立变化边界审查状态结构，氧气、完整度、警戒值和倒计时等资源不再被通用叙事字段错误覆盖；结构工具在一次 finalize 回执中给出精确的初值清单，首个 `submit_interactive_turn` 会原子校验所有可写初始字段均有具体值。开局结构项若误用可唯一映射的初始 Actor ID，会安全归一化并始终保存规范 Template ID，避免 `story` / `story_context` 混淆触发无意义重试。
- New Game openings now review state schemas by independent change boundaries, so resources such as oxygen, integrity, alert levels, and countdowns are no longer hidden in generic narrative fields. The schema tool returns an exact initialization checklist in its first finalized receipt, and the first `submit_interactive_turn` atomically verifies that every writable initial field has a concrete value. Opening schema items that use an unambiguously mapped initial Actor ID are safely canonicalized and always persist the canonical Template ID, avoiding pointless `story` / `story_context` retries.
- 游戏 Agent 默认仍关闭供应商扩展 thinking（可在 Agents 配置中显式开启），但只要 Provider 实际返回 thinking，前台就会逐帧原样展示并完整持久化；简短规划与意图分析只通过提示词约束，不再由输出链路截断或改写。工具输入也移除了非流式 200 字节、会话恢复 32 KiB、Plan 展示 32 KiB/12,000 字符和写文件流式预览 500 字符等隐式上限，展开、完成及恢复时均保留完整原文；下一轮模型上下文继续保留原始工具参数，并复用执行完成时生成的有界工具结果。
- The Game Agent still disables provider-specific extended thinking by default, with an explicit Agents override, but any thinking actually returned by the provider is now streamed verbatim and persisted in full. Concise planning and intent analysis are guided only by the prompt rather than output rewriting. Tool inputs also no longer have the implicit 200-byte non-streaming, 32 KiB session/Plan, 12,000-character Plan-card, or 500-character live write-preview display limits; expanded, completed, and restored views retain the full original input. The next model turn keeps the original tool arguments and reuses the result already bounded at tool completion.
- 游戏首回合的状态结构与回合提交工具现在提供开局来源、证据类型、字段类型、决策枚举及列表上限的严格 schema；`state_changes` 明确要求原生 JSON 数组并对合法的单层字符串编码数组做有界兼容，重试提示会保留正文已经成立的状态事实，减少反复工具报错和状态丢失。
- Opening Game turns now expose strict schemas for source provenance, evidence kinds, field types, decisions, and list bounds. `state_changes` explicitly requires a native JSON array while tolerating one valid string-encoded array layer, and retry guidance preserves state facts already established in prose to reduce repeated tool failures and dropped state.
- 状态预设编辑器遇到“面板/状态”这类嵌套 object 默认值时改用 JSON 编辑，浅层 object 仍保留结构化编辑，不再把嵌套内容显示或误写为 `[object Object]`。
- The actor-state preset editor now uses JSON editing for nested Panel/State object defaults while preserving structured editing for shallow objects, preventing nested values from appearing or being overwritten as `[object Object]`.
- Agent 会话列表现在整行单击即可切换；生成中切换会立即停止旧流并显示目标会话，不再需要反复点击。会话统计保持固定宽度，过长的当前会话标题会在剩余空间内截断，不再挤压计数。
- Agent session rows now switch from a single click anywhere on the main row. Switching during generation stops the old stream and selects the target immediately instead of requiring retries. Session counts keep their width while long current-session titles truncate within the remaining space.
- 点击创作 Agent 输入区的待提交正文评论引用，现在会先打开评论所属章节，再按持久化文本锚点将对应评论滚动到可视区域并展开；同一章节内重复点击也会重新定位。
- Clicking a pending document-review reference in the Writing Agent composer now opens its chapter first, then scrolls to and expands the exact comment from its durable text anchor; repeated clicks within the same chapter locate it again.
- Automations 配置现在携带稳定 revision 并由后端 CAS 校验；外部文件更新会自动 reload，冲突时先三方合并并归档双方版本，再以本地优先结果重试，不再静默后写覆盖。
- Automation definitions now carry stable revisions enforced by backend CAS. External file changes reload automatically; conflicts are three-way merged and both versions are archived before retrying the local-preferred result instead of silently applying last-write-wins.
- 编辑器跨文档兜底保存改为按文档键批量排队：单个后台文档失败不会阻断后续文档，失败项会保留并提示重试；关闭当前文档自动保存也不会取消其他文档的待保存草稿。
- Editor fallback saves now use a document-keyed batch: one background failure no longer blocks later documents, failed drafts remain retryable with visible feedback, and disabling autosave for the current document no longer cancels other queued drafts.
- Agent 输入偏好保存失败会保留本地值和待保存请求；关闭面板或切换工作区不会丢失 after-delay 草稿，切换操作也不会被无关偏好保存失败硬阻塞。
- Failed Agent composer-preference saves retain their local value and queued request. Closing the panel or switching workspaces no longer drops after-delay drafts, and unrelated preference failures no longer hard-block navigation.
- Agent/外部文件更新会先 reload，并以草稿自身绑定的 revision 做三方合并；非重叠修改静默合并，真正重叠时先把 base、用户稿、外部稿和默认本地优先结果原子写入恢复记录，再继续保存并提示用户，不再用弹窗硬阻塞编辑。
- Agent and external file updates now reload first and three-way merge against the revision bound to the draft. Non-overlapping edits merge silently; true overlaps atomically archive the base, local, external, and local-preferred merged versions before saving and notifying the user, without a blocking conflict dialog.
- 设置文件与 Agent 配置写入共享按规范路径串行的 revision/原子替换内核，锁内读取并变更最新内容；并发修改不再用旧快照覆盖无关字段，失败写入也不会留下半文件。
- Settings and Agent configuration writes now share a canonical-path revision and atomic-replacement core that mutates the latest content under one lock, preventing stale snapshots from dropping unrelated concurrent fields or leaving partial files.
- 叙事风格方案右侧除顶部工具栏外统一为单一连续滚动区；名称、描述、文风参考、注入规则和规则正文会一起滚动，并铺满可用高度，不再把正文裁切在狭窄的嵌套面板中。
- Narrative-style presets now use one continuous scroll surface below the top toolbar, so metadata, style references, injection rules, and rule content move together, fill the available height, and no longer clip the editor inside a narrow nested pane.
- 资料库正文移除重复的“正文”外框，点击段落后的光标会精确落在所点位置，外部内容回灌也不再把光标移到文末；主写作编辑器同步保护同一文档的当前选区。
- The Lore body no longer has a redundant Content frame. Clicks now place the caret at the selected paragraph, external refreshes no longer move it to the document end, and the main writing editor preserves the current selection during same-document syncs as well.
- 自动保存按资源和设置层串行执行，在切换条目、配置类型、页面或进入删除确认前 flush，并在前一次写入后立即沿用新 revision；失败的方案保存会先重试再允许切换，迟到响应不会覆盖更新的本地草稿。
- Autosave now serializes per resource and settings layer, flushes before resource, configuration-kind, page, or delete-confirmation transitions, and immediately advances revisions between queued writes. Failed preset saves retry before navigation, and late responses cannot replace newer local drafts.
- 未编辑的配置会直接加载外部最新值且绝不触发回写；编辑中的文本和结构化草稿会在 revision 变化时按原始基线三方合并，并自动携带最新 revision 重试，不再要求用户刷新页面处理常规冲突。
- Clean configuration drafts now load external updates without writing anything back. Dirty text and structured drafts three-way rebase from their original baseline and transparently retry with the latest revision, so ordinary conflicts no longer require a page refresh.
- Automations 配置写入不再回传触发状态、最近运行等服务端运行时字段；Skills 刷新会重新读取当前文档，过期 revision 返回 409 而不会静默覆盖磁盘上的外部修改。
- Automation configuration writes no longer echo server-owned trigger state or recent runs. Skills refresh reloads the selected document, and stale revisions return 409 instead of silently overwriting external file changes.
- 写作模式现在会隔离参数不是合法 JSON 的工具调用及其结果；已经保存的异常调用链也会在下次请求前被过滤，长参数则使用合法 JSON 回执保留上下文，避免会话被永久冻结。
- Writing Mode now isolates tool calls with invalid JSON arguments and their results; previously saved malformed pairs are filtered before the next request, while large arguments use a valid JSON receipt so sessions do not become permanently frozen.
- 设置与 Agents 的分层草稿、自动保存和输入区偏好持久化现在会串行写入，并在 revision 冲突时按原始基线重新拉取、合并和重试；卸载或过期请求不再回写状态。
- Layered drafts in Settings and Agents, autosave, and composer preference persistence now serialize writes and refetch, rebase, and retry from the original baseline on revision conflicts; unmounted or stale requests no longer publish state.
- 自动化后台刷新、运行结束和语言切换不再覆盖未保存任务草稿，乱序工作区响应会被忽略；窄屏操作区、资源选择器和当前项语义也保持完整可用。
- Automation background refreshes, run completion, and language changes no longer overwrite unsaved task drafts, and out-of-order workspace responses are ignored; narrow-screen actions, resource pickers, and current-item semantics remain fully usable.
- 游戏模式现在会在刷新页面后重新连接当前故事与分支的活动 Agent 任务，回放本轮玩家输入、思考、工具调用和流式正文，并在持久化确认后继续合并同一回合。
- Game Mode now reconnects to the active Agent task for the current story and branch after a page refresh, replaying the player action, reasoning, tool calls, and streamed prose before merging the same turn on persistence confirmation.
- Windows 新建或切换书籍时不再因工作区变更存储对 `.denova` 目录执行不受支持的同步而失败；账本、内容 blob 和作品文件仍保留完整的文件级持久化同步。
- Creating or switching books on Windows no longer fails when workspace-change storage encounters unsupported directory synchronization under `.denova`; ledger, content blob, and manuscript files retain full file-level durability synchronization.

## [v0.2.0] - 2026-07-15

### Brief / 简要说明

#### 中文

- Beta 不兼容提醒：0.2.0 开发期间出现但从未正式发布的中间格式不提供迁移；v0.1.18 已发布的故事、工作区、资料库、会话和配置数据继续保留必要的读取与数据保护逻辑。
- 状态字段名称现在禁止使用路径分隔符 `/`，所有内置预设已改用无歧义名称；开发期自定义状态系统需要先重命名相关字段，已开始故事的冻结 Schema 不会被自动改写。
- 写作工作台与可玩的 AI 互动叙事现在建立在同一套长期创作架构上；Story Director、Actor State、事件包、TRPG 检定、资料工作集和可检索 Turn 历史拥有明确的数据边界。
- Agent/Subagent 工作流新增可配置 Skills、上下文压缩、工具结果保留策略、运行计划和本地 Trace，长对话与复杂工具调用更可控、更易排查。
- 游戏主舞台的当前状态采用摘要优先的自适应布局，方案预设也能安全识别旧版无效覆盖并让用户手动恢复内置数据。

#### English

- Beta compatibility notice: intermediate formats created during 0.2.0 development were never released and are not migrated. Required read and data-protection paths remain for stories, workspaces, lore, sessions, and settings released in v0.1.18.
- State field names can no longer use the `/` path separator, and all built-in presets now use unambiguous names. Development custom State Systems must rename affected fields, while frozen schemas in started stories are not rewritten automatically.
- The writing workbench and playable AI narrative now share one long-lived creation architecture, with clear boundaries for Story Director plans, Actor State, Event Packages, TRPG checks, lore worksets, and searchable Turn history.
- Agent and Subagent workflows add configurable Skills, context compaction, tool-result retention, run plans, and local traces, making long conversations and complex tool runs easier to control and inspect.
- Game Mode's Current State uses a summary-first adaptive layout, while Presets can safely identify invalid legacy overrides and let users restore built-ins manually.

### Added

- 游戏模式新增分支级 Story Director 三文档：`director.md` 保存私密规划，`agent-brief.md` 只向正文 Agent 暴露可执行简报，`lore-context.md` 以当前、候场和暂离场管理资料工作集；逐文件 Patch、revision/hash 校验与原子提交避免无关文档被覆盖。
- Game Mode adds three branch-scoped Story Director documents: private planning in `director.md`, prose-facing instructions in `agent-brief.md`, and Active/Candidate/Offstage lore in `lore-context.md`, with per-file patches, revision/hash checks, and atomic commits.
- 新增可插拔故事导演模块：叙事风格、多个事件包、固定 d20 的 TRPG 检定、状态系统和图像方案可独立选择或关闭；事件卡支持铺垫、推进、回收、放弃和运行态审计。
- Story Directors can now compose independently switchable Narrative Styles, multiple Event Packages, fixed-d20 TRPG Checks, State Systems, and image presets; event cards support seeding, progression, payoff, abandonment, and runtime audit.
- 新增冻结的 Actor State Schema、初始 Actor、通用词条池与模板抽取规则；首轮后状态结构审查使用增量 Batch、来源证据、值策略、revision 校验和原子 Actor 操作完成一次故事级适配。
- Added frozen Actor State schemas, initial Actors, reusable trait pools, and template draw rules. Post-opening schema review uses incremental batches, sourced evidence, value policies, revision checks, and atomic Actor operations for one story-scoped adaptation.
- Game Agent 每轮通过独立的 `submit_actor_state_patches` 与 `submit_choices` 提交隐藏结果；模块可分别接受和重试，并支持故事级 2–10 个行动建议、终局空选项与 Unicode 去重。
- The Game Agent now submits hidden turn data through independent `submit_actor_state_patches` and `submit_choices` tools, with per-module acceptance/retry, configurable 2–10 choices, terminal empty choices, and Unicode-aware duplicate checks.
- 新增 `search_story_history`、带来源 Turn 的历史检查点和 Director 资料读取凭证，使历史事实、当前状态、稳定设定与未来意图分别归属 Turn、Actor State、Lore 和 Director。
- Added `search_story_history`, source-Turn history checkpoints, and Director lore-read receipts, separating historical facts, current state, stable canon, and future intent across Turns, Actor State, Lore, and Director plans.
- 资料库新增常驻/按需加载、名称目录、批量读取、语义类型整理、revision 冲突保护，以及酒馆角色卡 `ccv3`/`chara` 世界书导入、运行时清洗和来源审计。
- Lore adds Resident/On-demand loading, name catalogs, batch reads, semantic type organization, revision-conflict protection, and Tavern card `ccv3`/`chara` world-book import with runtime cleanup and provenance.
- Agent 系统新增可配置 Subagent、Skills、上下文压缩、工具结果保留策略、运行计划、自动化工作流与本地结构化 Trace；WebUI 提供统一 AI SDK 消息时间线、Director Console、方案预设和运行追踪。
- The Agent system adds configurable Subagents, Skills, context compaction, tool-result retention, plans, automation workflows, and local structured traces; the WebUI adds unified AI SDK timelines, Director Console, presets, and run inspection.

### Changed

- Beta 不兼容：状态字段名称禁止包含路径分隔符 `/`；内置预设中的相关名称统一改用“与”，前端可视化编辑、后端预设保存、故事创建与 AI 状态结构适配都会拒绝该符号。开发期自定义状态系统需要先重命名字段；已开始故事继续保留冻结 Schema，不自动改写用户状态。
- Beta breaking: State field names cannot contain the `/` path separator. Affected built-in preset names now use “and,” and the visual editor, server-side preset writes, story creation, and AI schema adaptation all reject the separator. Development custom State Systems must rename affected fields; started stories keep their frozen schemas without automatic state rewrites.
- 游戏模式主舞台的“当前状态”改为摘要优先的自适应布局：数值指标会填满不完整末行，短字段与长文本分层排列，储物袋、功法、器物和故事上下文等结构化对象按字段进入可切换的二级 Tab，同一时间只展示一项，减少空白和纵向占用；宽窄屏及浅深色主题保持一致。
- Game Mode's main-stage Current State now uses an adaptive, summary-first layout: numeric metrics fill incomplete final rows, compact facts and long-form text use separate flows, and structured objects such as Inventory, Techniques, Artifacts, and Story Context become switchable field-level sub-tabs that show one section at a time, reducing whitespace and vertical length across narrow/wide viewports and light/dark themes.
- Beta 不兼容：0.2.0 最终导演格式直接使用三文档和最终模块引用；不再读取或迁移开发期的混合导演文档、Event System、Opening Selector、Teller orchestration、内嵌导演资源、`auto` 状态适配模式或旧合并提交协议。使用开发快照的工作区需要重新创建这些未发布资源。
- Beta breaking: the final 0.2.0 format directly uses three Director documents and final module references. It no longer reads or migrates development-only combined Director documents, Event Systems, Opening Selectors, Teller orchestration, embedded Director resources, the `auto` adaptation mode, or the combined submission contract. Workspaces created from development snapshots must recreate those unreleased resources.
- 游戏模式移除独立 Story Memory / Memory Structure 可写真源；旧文件不会自动删除或覆盖，但新运行时只使用 Turn、Actor State、Lore 和 Director 文档。
- Game Mode removes Story Memory / Memory Structure as a separate writable source of truth. Existing files are not automatically deleted or overwritten, while new runtime behavior uses Turns, Actor State, Lore, and Director documents.
- 工具结果不再支持真正无上限：默认安全上限提高到 1024 KB，旧配置中的 `agent_tool_result_limit_kb = 0` 会归一到该值；仍可配置更高的正数。
- Tool results are no longer truly unbounded: the default safety limit is now 1024 KB, legacy `agent_tool_result_limit_kb = 0` values normalize to that limit, and larger positive limits remain configurable.
- 状态结构审查中的当前 Actor 快照在 1 MiB 内完整保留，整份动态提示上限为 2 MiB；超限会返回明确的中英文错误，不会静默截断或提交半份上下文。
- State-schema review preserves the complete current Actor snapshot up to 1 MiB and caps the full dynamic prompt at 2 MiB. Oversize input returns an explicit bilingual error instead of silent truncation or a partial prompt.
- 默认 `triggered` Director 只在首轮、资料 revision 变化或 Game Agent 报告实质规划变化时运行；`every_turn` 与 `off` 仍可显式选择。
- The default `triggered` Director now runs for the opening, lore-revision changes, or material planning changes reported by the Game Agent; `every_turn` and `off` remain explicit options.
- 根目录开发脚本迁移到 `scripts/`；GitHub Release 构建现在校验 tag、包版本、CHANGELOG 与双语 README，并运行完整 Go/前端校验后生成五个平台包、双语说明与 SHA-256 校验。
- Development scripts moved under `scripts/`. GitHub Release builds now verify the tag, package versions, CHANGELOG, and both READMEs, run complete Go/frontend checks, and produce five platform archives, bilingual notes, and SHA-256 checksums.

### Fixed

- 方案预设检测到无效 JSON 时会区分普通配置与旧版内置覆盖：后者提供明确说明和手动“恢复内置”操作，但不会自动修改或回退用户数据。
- Presets with invalid JSON now distinguish ordinary edits from older built-in overrides. Overrides offer an explicit manual restore action without automatically changing or rolling back user data.
- 资料库所有读改写操作现在在同一工作区跨 `LoreStore` 实例串行化，并使用同目录临时文件、`fsync` 与原子 rename 保存，修复并发创建、分类或更新时丢条目和留下半写 JSON 的问题。
- Lore read-modify-write operations are serialized across `LoreStore` instances for the same workspace and saved through same-directory temp files, `fsync`, and atomic rename, preventing lost entries and partially written JSON during concurrent creates, classification, or updates.
- 修复世界状态长期为空、基础 `story_context` Actor 缺失、状态字段重复初始化、`set(null)`、错误覆盖现值及状态结构审查未初始化已确认值等问题，并保留迁移与规则状态来源审计。
- Fixed empty World State, missing foundational `story_context` Actors, duplicate initialization, `set(null)`, current-value overwrites, and confirmed values not being initialized during schema review, while retaining migration and rule-state provenance.
- 修复 Game Agent 正文与隐藏提交时序、漏调工具后的同运行重试、重复提交覆盖、错误 JSON 牵连已接受模块，以及流式 thinking/工具卡抖动和失败回合无法复制/重试。
- Fixed Game Agent prose/submission ordering, same-run correction after omitted tools, duplicate submission replacement, malformed JSON invalidating accepted sibling modules, streaming trace jitter, and missing copy/retry actions for failed turns.
- 修复方案预设自动保存竞态、迟到响应覆盖、内置资源恢复、窄屏溢出、共享弹窗与导航行为；同时修复写作编辑器跨文件保存、光标恢复、Diff、单文件恢复和缺失设定入口。
- Fixed preset autosave races, stale-response overwrites, built-in restore, narrow-screen overflow, shared dialogs/navigation, plus cross-file writing saves, cursor restoration, diffs, single-file restore, and missing-setting shortcuts.
- 修复启动端口与前端代理、Gemini/OpenAI 兼容参数、SSE 心跳、上下文缓存边界、工具结果元数据和 Trace 关联中的稳定性问题。
- Fixed startup ports and frontend proxying, Gemini/OpenAI-compatible parameters, SSE heartbeats, context-cache boundaries, tool-result metadata, and trace correlation.
- 默认测试不再访问实时搜索引擎；只有显式设置 `DENOVA_LIVE_WEB_SEARCH_TEST=1` 才运行联网聚合检查，保持常规测试快速且可复现。
- The default test suite no longer accesses live search engines; network aggregation runs only with `DENOVA_LIVE_WEB_SEARCH_TEST=1`, keeping normal tests fast and reproducible.

### Removed

- 删除所有仅服务于未发布 0.2.0 中间格式的后端迁移器、前端字段、API 适配器、模型提示和回归测试，包括导演三文档迁移、Event System 转 Event Package、Opening Selector 转 Actor State、旧 Actor State 模块版本、`TurnBrief`、旧 `StateDelta`/合并 Turn 提交、单 proposal 状态审查、纯文本 PlanDecision，以及基于 path、d100/别名和 v1 StateOps 的 TRPG 适配。
- Removed backend migrators, frontend fields, API adapters, model guidance, and regression tests used only by unreleased 0.2.0 intermediate formats, including three-document migration, Event System-to-Package conversion, Opening Selector-to-Actor State conversion, old Actor State module versions, `TurnBrief`, legacy `StateDelta`/combined Turn submission, single-proposal schema review, plain-text PlanDecision, and path-, d100/alias-, or v1 StateOps-based TRPG adapters.
- 删除独立 Hot Choices Agent、Opening Selector API/UI、旧聊天并行入口和不再使用的配置字段；最终协议由通用 Agent、Actor State、事件包和统一消息时间线承担。
- Removed the standalone Hot Choices Agent, Opening Selector APIs/UI, parallel legacy chat endpoints, and unused settings; final protocols are handled by common Agent, Actor State, Event Package, and unified timeline abstractions.

### Data protection

- v0.1.18 已发布的 `.nova`/`.denova` 工作区发现、故事 JSONL、通用 StateOps、会话消息、资料库和配置读取路径继续保留；自动迁移或状态重写仍使用 revision 校验、备份或可回放事件。
- Read paths remain for v0.1.18 `.nova`/`.denova` workspace discovery, story JSONL, generic StateOps, session messages, lore, and settings. Automatic migration or state rewrites continue to use revision checks, backups, or replayable events.

## [v0.1.18] - 2026-07-01

### Brief / 简要说明

#### 中文

- 完成 Denova 品牌与分发命名切换；兼容性提示：Release 包不再提供 `nova` / `nova.exe` / `nova-updater` 别名，新安装请直接运行 `denova` / `denova.exe`。
- 新增新用户引导、消息中心、PWA/移动端主屏体验，以及可内嵌前端的单文件自托管能力。
- 大幅补齐移动端写作与游戏模式的输入、弹窗、文件操作、故事记忆和分支导航体验。
- 图像方案、书籍封面生成、互动图像回写、Plan Mode 展示、章节正文隐藏输出和资源保存冲突保护更稳定。

#### English

- Completed the Denova branding and distribution rename; compatibility note: release packages no longer include `nova`, `nova.exe`, or `nova-updater` aliases, and new installs should run `denova` / `denova.exe` directly.
- Added onboarding, a message center, PWA/mobile home-screen support, and a self-hosting path where the backend can embed the web app.
- Filled in more mobile Writing and Game Mode input, dialog, file action, story memory, and branch navigation workflows.
- Made image presets, cover generation, interactive image writes, Plan Mode rendering, hidden chapter-body streaming, and resource conflict protection more reliable.

### Added

- 游戏模式：剧情页新增宽屏轮次导航，左侧横杠可快速定位每个对话轮次，悬停/聚焦时展示用户输入与 Agent 剧情正文预览；窄屏或舞台空间不足时自动隐藏。
- WebUI：新增新用户引导，按“配置语言模型 API Key → 新建书籍 → 创作 Agent 预填第一章开头 → 一级模块导览”串联主流程；支持一键跳过、设置页重新打开，状态仅保存在浏览器本地，不写入用户或工作区配置。
- WebUI：新增全局消息中心，顶部栏铃铛入口可查看 Denova 更新日志；打开某条消息会自动标记为已读，也可一键全部已读。已读状态保存到用户级 Denova 数据目录，不写入作品 workspace。
- 游戏模式：行动选项默认会在故事输出结束后后台自动生成，用户点击输入框右侧“选择”后再展开；输入框左侧菜单保留“自动生成 / 手动生成”切换。
- WebUI：新增 PWA manifest、应用图标（apple-touch-icon / 192 / 512 / maskable）与移动端 viewport meta（`viewport-fit=cover`、`theme-color`、`apple-mobile-web-app-capable` 等）。自托管后可在手机主屏“添加到桌面”以独立应用形态打开，并正确延伸到刘海安全区；图标由 `pnpm generate-icons`（sharp）从 `favicon.svg` 复现式生成。
- 后端：静态资源服务对未知前端路径做 SPA 回退（返回 `index.html`）。手机刷新任意页面或深链打开不再返回 Hertz 默认 404；`/api/*` 路由不受影响。
- 后端：Denova 二进制现在可内嵌前端（构建标签 `embedweb`），裸二进制无需磁盘 `web/` 目录即可提供前端服务，适合 `go install` / 单文件分发 / 最小化自托管。默认构建行为不变；release 仍附带 `web/` 作为磁盘快速路径与 updater 兼容，内嵌为独立运行的兜底。
- 文档：README（中/英）新增「自托管与远程访问（手机访问）」章节，覆盖构建前端、开启远程访问、手机使用与 HTTPS 反向代理。

### Fixed

- 文档/更新：修正 README 徽章、Release 下载、源码克隆、Star History 与应用内更新检查使用的 GitHub 仓库标识，改为 `alfredxw/denova`，避免用户跳转或检查到旧 Release 页。
- 游戏模式：互动图像生成完成后允许把展示事件写回当前分支父链上的继承回合，避免从旧分支接出的剧情线在生成祖先回合图像时误报“展示事件回合不属于当前分支”；图像生成上下文也改用当前快照分支读取故事记忆。
- Agent：内置 `novel-lite` / `novel-standard` / `novel-heavy` 写作 Skill 明确要求按场景使用 `read_file`、`write_file`、`edit_file`、`task` 等工具，并在写入后检查工具结果与读回关键片段，避免工具失败时误向用户宣称文件已修改。
- 对话渲染：游戏模式改为用后端落盘增量事件原地合并新回合，并把完整快照刷新降级为静默校准；同时移除通用对话和游戏剧情页在 `done` 事件上的临时“完成 / Done”活动行，并把流式正文改为 `streaming_target_content` 隐藏占位、下一帧再提升为可见 `content` 的两阶段提交，避免输出完成或换行瞬间因消息列表高度变化、live 消息切换到持久化快照而抖动或重新入场。本次为内部渲染行为优化，无用户数据迁移。
- WebUI：桌面布局根与运行时错误边界由 `h-screen` 改为 `h-dvh`，与移动 shell 一致，避免 iOS Safari 地址栏导致的底部跳动。
- WebUI：远程访问登录框用户名/密码输入框使用 16px 字号，避免 iOS Safari 聚焦时自动缩放页面（该登录浮层渲染在 app shell 之外，原先不享受 16px 字号覆盖规则）。
- WebUI：章节版本对比的紧凑模式改用项目统一的 `useIsMobile()` 断点（767px），与移动 shell 及版本对比弹窗的 `max-md` 行为一致，消除 760–767px 区间的断点错位。
- 移动端：聊天 Agent 与互动故事的浮动输入框现在会随软键盘上移，不再被键盘遮挡（此前在 iOS 上输入框会被键盘盖住）。新增 `useKeyboardInset` hook 基于 `visualViewport` 计算键盘高度，仅在输入聚焦时生效；桌面端与 Android（`dvh` 已自动收缩）不受影响。顺带为输入框加上 `enterKeyHint="send"` 等移动键盘提示。
- 移动端：文件树每行的操作菜单（新建 / 重命名 / 复制 / 移动 / 删除 / 引用）按钮在触摸下常显（原先仅 hover 可见，手机上无法触达文件操作），并加大行与内联输入框的触摸区、补上 `enterKeyHint`。
- 移动端：标签页关闭按钮在触摸下常显、当前标签常显（原先仅 hover 可见，手机上无法关闭标签）。
- 移动端：移动顶栏新增「命令」按钮，没有实体键盘的手机也能打开命令面板（原先仅 ⌘K / Ctrl+K）。
- 移动端：写作编辑器阅读区在手机上改用更紧凑的横向留白（`px-4`，桌面仍为 `px-10`），避免窄屏正文被两侧大留白挤压。
- 移动端：聊天消息的助手操作（重新生成 / 切换版本等）与消息元信息在触摸下常显（原先仅 hover 可见，手机上无法触达）。
- 修复：命令面板（⌘K / Ctrl+K）打开即崩溃（`Cannot read properties of undefined (reading 'subscribe')`，触发前端错误边界白屏）。根因是 `CommandDialog` 未用 cmdk 的 `<Command>` 根包裹内容，导致 `CommandInput`/`CommandList`/`CommandItem` 拿不到 cmdk store；补上 `<Command>` 包裹后面板在桌面与移动端均可正常打开。
- 移动端（共享原语）：`Dialog` / `AlertDialog` 内容默认限制在视口高度内并可滚动（`max-h-[calc(100dvh-2rem)] overflow-y-auto`），长内容弹窗在手机上不再溢出屏幕；自带 `max-h` / `overflow` 的弹窗不受影响（tailwind-merge 优先消费者值）。
- 移动端（共享原语）：`Popover` 内容新增 `max-w-[calc(100vw-1rem)]`，窄屏下不再溢出到屏幕外。
- 移动端（互动模式）：故事记忆（StoryMemory）记录列表在窄屏改用卡片渲染（原先的 `table-fixed` 列表在手机上列宽被挤压到几个字符、内容不可读，且 `overflow-x-hidden` 无法横向滚动）；桌面端仍保持表格。复用 `AdaptiveSurface` 提供的 `isMobile` 与既有字段渲染逻辑。
- 移动端（互动模式）：剧情分支时间线（BranchTimeline）工作台视图在手机上新增「回到当前节点」按钮（桌面端用缩略导航 MiniMap 定位，移动端 MiniMap 隐藏，故补充此按钮以便手动平移后重新定位到当前剧情线）；分支切换 pills 触摸区在移动端加大。该视图的图本身已可触摸拖拽平移、切换分支自动居中、选中节点后可创建分支，本次补齐移动端导航缺口。
- 移动端：修复创作 Agent / 互动故事输入框在预填长 prompt（如「和创作 Agent 聊灵感」自动注入的启动 prompt）时 textarea 无限增高、composer 撑满大半屏挤压对话区的问题；移动端将 composer textarea 的最大行数限制为 5（桌面仍为 10），长内容在框内滚动而非顶高整个输入区。

### Changed

- 项目改名：应用名、Go module、命令目录、前端标题、PWA manifest、README、配置模板、内置 Agent 提示、npm 包名和 GitHub Release 产物统一从 Nova/nova 切换为 Denova/denova。新工作区与新配置默认使用 `.denova` / `DENOVA_*`；已有 `.nova` 工作区与 `NOVA_*` 环境变量继续兼容读取。GitHub Release 包不再附带 `nova`、`nova.exe` 或 `nova-updater` 别名，用户新下载后直接运行 `denova` / `denova.exe`。
- WebUI：Chat 输入框默认以双行展开显示，Plan Mode 不再占用独立按钮，改为放入输入动作菜单；开启 Plan Mode 时在输入区底部工具行显示轻量 `Plan` 状态提示，并保留 `Shift+Tab` 快捷切换。游戏模式输入框保持单行。
- 移动端：Agent 面板从右侧抽屉改为**底部常驻面板**（与编辑器竖向分割），恢复桌面端「编辑器 + Agent 同屏可见」的核心操作逻辑。使用 `react-resizable-panels` 做竖向分割，可拖拽分隔条调节编辑器/Agent 比例。Agent 不再需要点导航打开；快捷创作按钮始终可达。桌面端不受影响。
- 设置：新增 `hide_novel_chapter_body_in_live_output` 配置，开启后隐藏章节正文在 Agent 流中的输出，并保留目标路径和已生成字符数；默认关闭以保持原有实时输出行为。
- Agent 调试：完整 LLM 输入日志默认关闭，即使 `--dev-mode` 启动也需要在开发模式设置页的「调试」分区手动开启；日志写入改为后台异步队列，`provider_request_id` 以独立关联事件追加到 `log/llm-inputs.jsonl`，避免模型请求热路径同步重写大文件。

### Changed

- 方案预设：图像方案升级为可配置注入位置的规则列表，支持分别注入图像 Agent system prompt 和最终图像请求 prompt；旧单段 prompt 会兼容迁移为图像请求规则。
- WebUI：书籍管理里的“编辑信息”改为独立弹窗，扩大书名、作者、简介和封面生成区域，避免在书架卡片内编辑过于拥挤。
- WebUI：书籍管理手机端书架改为以封面为主的紧凑自适应网格，iPhone 15 Pro 等窄屏宽度下书卡只展示封面和书名，减少纵向占用。
- WebUI：图像放大查看器改用 `react-zoom-pan-pinch` 管理缩放、拖拽/触控板滚动平移和触控板 pinch；工具栏按钮保持 25% 步进，手势缩放改为按比例变化。

### Fixed

- Agent：写作模式生成小说章节时，开启 `hide_novel_chapter_body_in_live_output` 后，SSE 推流前 middleware 会在 `write_file` 写入 `chapters/` 或 `drafts/` 时只向前端发送目标文件路径、隐藏提示和已生成字符数，不再输出大量章节正文或省略号占位；字符进度会按增量轻量节流，并在工具结束前用完整参数解码校准最终值，口径与 `wc -m` 保持一致，前端工具卡片会提示章节正文仅在 Agent 流中隐藏、文件仍会正常写入。
- 书籍管理：编辑书籍信息时可直接选择图像方案并生成书籍封面，生成结果立即写入固定展示路径 `assets/image/cover.png`，旧封面会自动备份到 `assets/image/covers/backups/`。
- WebUI：书架卡片和当前书籍区域会展示同一固定封面；没有封面时保持简洁书本占位，酒馆角色卡导入的 `assets/image/cover.png` 也会正常展示。
- WebUI：书架封面即使暂时没有 `cover_updated_at` 版本号，也会尝试读取固定路径 `assets/image/cover.png`，避免本地已有封面却显示占位图。
- WebUI：设置页、Agents 页和游戏设置页保存时带上资源版本，后端检测到 Agent 或其他页面已更新同一配置/资源时返回冲突错误，避免旧自动保存覆盖新内容。
- 游戏模式：互动图像重新生成完成并追加新版本后，回合内联预览会自动切到最新图片，不再停留在用户之前手动查看的旧版本。
- WebUI：将 `react-virtuoso` 锁定到满足 pnpm minimum-release-age 策略的版本，避免 `pnpm --dir web test` 在执行测试前被供应链校验拦截。
- WebUI：滚动消息列表时同步记录实际 Virtuoso 滚动容器，避免“回到底部”按钮在测试或 ref 变化后无法恢复底部锁定。
- 写作 Agent：优化 Plan Mode 卡片交互与输出展示。问题卡限制高度并固定操作区，卡片生成、内容增长、题目切换和布局变化会将卡片底部对齐到对话输入框顶部，且不打断后续工具数据的自动跟随；连续多轮 Plan 按当前 run 原地更新；生成中只展示 running 后新变化的 root thinking 预览，停滞后自动隐藏；提交问题答案或选择最终计划操作后隐藏按钮并显示完成态，内部回答/批准协议、卡片前后说明和误触发的 `plan_questions`/`proposed_plan` 协议工具卡不再重复展示；最终计划改用轻量 Markdown 模板并复用聊天 Markdown 样式。

## [v0.1.17] - 2026-06-27

### Added

- 游戏模式：新增“互动图像”，默认手动生成；输入框左侧菜单提供侧边配置，可切换为手动或每 X 轮生成，每个剧情回合操作区提供手动生成/重新生成按钮。
- Agent：新增通用 `image` Agent，默认仅启用 Skills 和图像生成工具；互动图像通过 `interactive-image` Skill、`purpose=interactive_image` 和专用 System Prompt 复用该通用 Agent。
- 后端：新增 `POST /api/interactive/stories/:id/images/generate`，互动图像保存到 `assets/interactive/images/<story>/<branch>/<turn>/<timestamp>/`，结果以 `interactive_image.v1` display event 挂到对应回合，不移动分支 head、不写入叙事正文、不进入下一轮模型上下文。
- 方案预设：新增独立“图像方案”资源和 `GET/POST/PATCH/DELETE /api/image-presets`，内置 `游戏CG`、`写实`、`2D插画` 三种方案，写作 Agent 与游戏互动图像默认使用 `游戏CG`。
- 写作模式：新增内置 `chapter-illustration` Skill 和通用 `generate_image` Agent 工具，创作 Agent 可基于当前或指定章节生成一张非剧透插画，结果保存到 `assets/illustrations/` 并在工具卡片中预览，用户可手动插入为 Markdown 图像。
- 后端：新增受保护的 workspace asset 图像读取接口，仅允许读取 `assets/` 下的图像文件，供章节插画和 Markdown 渲染使用。

### Changed

- WebUI：中文界面中 Automation Agent 统一改称“自动化Agent”，包括 Agents 页、自动化模型继承提示和自动化 Agent 内置中文提示。
- WebUI：顶层“互动模式 / Interactive Mode”更名为“游戏模式 / Game Mode”，强调其定位是互动文字冒险游戏工作台；内部 `interactive` API、配置键和存储目录保持不迁移。
- WebUI：顶层“叙事编排 / Narrative Direction”更名为“方案预设 / Presets”，内部 `teller` 路由、一级菜单行为和模式切换规则保持不迁移；该页现在并列管理叙事方案和图像方案。
- Breaking：旧 `Teller.image_prompt` 已下线，不迁移、不读取、不展示、不兜底；图像生成风格改由独立图像方案预设保存到 `image-presets/*.json`。
- 游戏模式：互动叙事 Agent 不再要求用 XML 标签包裹正文，默认直接输出故事正文；历史或异常输出里的旧正文包装仍会兼容清洗。
- Agent：通用 General SubAgent 的内置默认范围收窄为仅写作 Agent 和 Automation Agent 启用；互动叙事 Agent 和配置管理 Agent 默认继承关闭，仍可在 Agents 页单独开启。
- Agent：自定义 SubAgent 的 `parents` 改为显式父 Agent 归属列表，空列表不再表示所有父 Agent 共享；Agents 页新增“仅从当前父 Agent 移除”和“全部删除”两种删除范围。
- Agent：工具结果默认不再截断，设置页 Agent 分区新增按 KB 配置的工具结果截断上限；设置为 `0` 或留空时不截断。
- Agent：`read_file` 默认读取窗口固化为从第 1 行开始最多 2000 行，只有显式指定更大的 `limit` 时才读取更多，并同步更新工具描述避免默认使用过小扫描窗口。
- 游戏模式：`read_interactive_memories` 不再限制最多 6 条、每条 4KB 或总计 12KB；互动记忆入库不再按 12KB 裁剪文本，Agent 显式读取时返回所有可见请求项的完整正文。
- WebUI：设置页将原“模型”分区改名为“语言模型”，将原“图像 API”分区改名为“图像模型”，并从设置页移除后端/前端端口输入和访问地址端口展示；端口仍可通过环境变量或配置文件在启动时设置。
- Agent：图像生成工具改为通用 `generate_image`，章节插画 Skill 改用中文流程调用该工具；生成尺寸改为调用时在 2K/3K/4K 预设中选择，设置页不再配置默认图像尺寸，输出格式限制为 `png` 或 `jpeg`。

### Fixed

- WebUI：Agents 页操作 General SubAgent 开关时先按本地草稿即时刷新开关与状态标记，保存继续异步执行，避免点击后等待配置保存才反馈。
- WebUI：修复设置页语言模型配置点击“添加语言模型”后，新建空模型配置被立即过滤掉、看起来没有反应的问题。
- 模型配置：修复多语言模型配置中 API Key 留空时不再继承默认模型 API Key 的问题；设置页将 `default` 配置直接标记为“默认模型”。

## [v0.1.16] - 2026-06-27

### Added

- 后端新增统一图像生成 API：支持配置多个 OpenAI 标准 Images API profile，`POST /api/images/generate` 会调用所选图像模型并将结果保存到当前工作区 `assets/image/generated/`。
- 设置页新增图像 API 配置区，可用 shadcn 表单组件配置默认图像 API、多个 OpenAI 图像 profile、默认尺寸、质量和输出格式。

### Fixed

- WebUI：修复默认模型配置未填写别名时仍继承模板里的 “DeepSeek 写作” 并在输入区出现无效模型选项的问题；默认模型现在始终使用稳定 `default` 配置 ID，未填别名时显示模型名。
- WebUI：一级菜单默认展开并把默认宽度从 152px 调整到 180px，同时迁移旧默认宽度，让常用菜单文字默认完整展示；用户手动拖拽后的宽度仍会保留。

## [v0.1.15] - 2026-06-27

### Added

- Agent 开发模式 LLM 输入日志会在响应返回后回写 `provider_request_id`，`log/llm-inputs.jsonl` 可直接关联完整请求输入和供应商请求 ID。
- Agent 模型响应日志新增 `provider_request_id`：当 OpenAI 兼容供应商返回请求 ID 时，后端会打印该 ID，便于向模型 API 供应商提供 debug 信息。
- 应用内更新新增独立 `nova-updater`：Release 包会携带同平台 updater，设置页先下载暂存更新，再通过“重启并安装”退出当前 Nova、替换主程序和资源目录，并自动启动新版本。

### Changed

- WebUI：一级菜单展开态支持拖拽调整宽度，默认宽度可容纳五字菜单名，最小宽度保留至少两个中文字的可读空间。
- WebUI：模型配置新增可选别名，模型选择器优先显示别名、未填写时显示模型名；设置页 Temperature 输入框改为 0-1 的紧凑数字框，上下文长度选项改用统一组件样式。
- WebUI：默认模型改为与其他模型一致的 `model_profiles` 列表配置，默认项使用 `id = "default"`，也支持设置别名。
- WebUI：底部状态栏右侧不再显示空闲状态和当前模型名，仅在生成中保留运行状态提示。
- Agent：创作 Agent 不再直接注入默认 Writing Skill 的 SKILL.md 正文，也不再用后端正则判断写作意图；本轮动态提示只说明当前选择的 Writing Skill，涉及正文写作/续写时由模型通过 `skill` 工具自行加载对应 Skill。
- Agent：默认写作 Skill 从 `novel-standard` 改为 `novel-lite`；用户仍可在创作 Agent 输入菜单或设置页自行切换默认 Skill。
- Agent：`config.toml` 模板预置 `writer`、`reviewer`、`fixer` 等写作 SubAgent，它们不再由 Go 默认值或内置 Writing Skill 运行时策略控制；用户可在 Agents 页像管理自定义 SubAgent 一样覆盖或关闭。
- Agent：系统提示词明确限制 SubAgent 委派时机，除非用户主动要求或已加载 Skill 流程要求，否则父 Agent 不应主动拉起 SubAgent。
- Agent：创作 Agent 的本轮动态上下文会注入前端 IDE 当前聚焦文件和打开文件路径；该状态只包含有界路径信息，不注入文件正文，需要正文时仍必须显式通过工具读取。
- Agent：默认不限制空闲等待时间；设置页和 `NOVA_AGENT_IDLE_TIMEOUT_SECONDS` 仍可配置正数秒数启用空闲超时，配置为 `0` 表示不限制。
- Agent：移除独立章节初稿目录和对应设置开关；章节初稿统一写入 `chapters/`，通过章节状态从初稿确认成章。

### Fixed

- Agent：修复真实模型用量明细刷新后只在互动 Agent 可用的问题；创作 Agent、配置管理 Agent 和固定 Agent 会话 API 现在会保留 `agent_kind`、token 统计和 `usage_calls`，并在持久化层按每种 Agent 只保留最近 10 条用量记录，避免历史无限膨胀。
- Agent：修复运行中配置刷新没有合入根 `config.toml` global 层的问题，避免 Agents 页和实际写作 Agent 只看到用户级/工作区级残留的部分 SubAgent。
- WebUI：修复编辑 SubAgent 可用父 Agent 时立即写入列表导致弹窗消失的问题；弹窗内改动现在会先保存在本地未提交内容，点击完成后再写回配置。
- WebUI：Agents 页将工具、Skills、上下文压缩、General SubAgent 和自定义 SubAgent 的启停控件统一为 Switch；自定义 SubAgent 可直接在列表启停，删除继承来的 SubAgent 不再变成关闭/恢复的循环。
- WebUI：优化创作 Agent 面板标题栏布局，新建会话入口移到视图切换器右侧并简化为加号按钮，同时移除空闲状态和当前会话摘要文字。
- WebUI：对话消息悬浮元信息改为截图式的消息下方操作行，只出现在用户消息气泡和根 Agent 正文下方，并新增仅图标的一键复制按钮；复制成功后按钮会短暂切换为勾号反馈，历史普通消息会补齐展示时间，SubAgent 小窗和工具卡片不再显示消息时间。
- WebUI：创作 Agent 的 SubAgent 详情栏支持拖拽调整宽度，关闭后会恢复右侧面板原宽度。
- WebUI：Agents 页的 SubAgent 列表按当前父 Agent 过滤，预置写作 SubAgent 只显示在写作 Agent 下。
- WebUI：修复创作 Agent 右侧面板读取旧持久化布局时可能因面板顺序错配导致拖拽宽度方向异常的问题。
- WebUI：创作 Agent、配置 Agent 和自动化运行对话的消息列表会随底部浮动输入区高度自动预留空间，长输入不再遮住最后一行消息。
- WebUI：移除写作模式左侧目录顶部的作品名与字数统计摘要，并将“其他设定”折叠入口合并到“书籍设定”标题行，减少重复行占用。
- WebUI：简化写作模式“章节组细纲”的空状态提示，不再显示内部目录路径。

## [v0.1.14] - 2026-06-26

### Added

- 写作 Skill Preset：内置 `novel-lite`、`novel-standard`、`novel-heavy` 三种 IDE 写作 Skill，默认使用 `novel-standard`；创作 Agent 输入区可选择当前写作 Skill，也可选择用户/工作区自建的 IDE Skill，运行时按工作区覆盖 > 用户覆盖 > 内置预设解析并注入有效 SKILL.md。
- 配置管理 Agent：新增 `list_agent_configs` / `write_agent_configs` 专用工具，可在 Agents 页通过对话管理 Agent 模型覆盖、Prompt、工具权限、Skills 可用性、上下文压缩、General SubAgent 和自定义 `sub_agents`；新增 `agent_config_read` / `agent_config_write` 工具权限，默认仅配置管理 Agent 启用。
- Added SubAgent delegation support with configurable General SubAgent availability, custom `sub_agents`, real-time subagent stream metadata, and compact Agents page management UI.
- WebUI / Agent：新增会话级 Plan Mode，写作 Agent / IDE Chat 支持 Chat / Plan 状态展示和 `Shift+Tab` 切换；Plan Mode 可一次接收结构化问题集、逐题向用户确认并在全部确认后统一提交答案，也可渲染拟定计划卡；计划卡展示和确认执行上下文都有长度上限，确认计划后再带有界批准计划切回执行模式。

### Changed

- Agent：运行时 system prompt 现在会按界面/请求语言引导模型使用对应语言输出 thinking 内容；该约束只影响思考过程，不覆盖输出协议、JSON 字段、文件内容或故事正文语言。
- Agent：默认不再为所有 Agent 设置 `max_iteration` 轮数上限；只有用户显式配置正数时才限制迭代次数。
- Agent：Review 自动化不再强制把 `max_iteration` 提升到 100，避免 task 委派继续被隐藏上限截断。
- Agent：自定义 SubAgent 现在继承父 Agent 稳定 system prompt、workspace/mode/tool 边界，并要求父 Agent 委派 task 时传递目标、约束和路径/资源 ID；若旧 SubAgent prompt 试图覆盖父 Agent 工具权限或模式边界，会以父级契约为准。
- Skills：内置预制 Skill 支持在界面中创建同名覆盖，默认写入用户级 `<nova_dir>/skills/<skill-name>/SKILL.md`，只有用户级目录不可写时才退回工作区覆盖；Skill 配置页现在支持修改 Skill 名称，并可在用户级与工作区级保存位置之间迁移。
- WebUI：Agents 页面默认编辑用户配置，Skills 页面默认在用户级目录新建 Skill；需要工作区级覆盖时仍可手动切换到工作区配置。
- WebUI：创作 Agent 面板移除独立 Review tab，Review 任务配置与运行过程统一回到自动化页；SubAgent 正文输出改为主会话高亮进度卡，点击后可在右侧打开独立子会话详情栏，避免混入父 Agent 正文。
- WebUI：写作模式作品目录上方的灵感、大纲和状态文件入口合并为可折叠的“书籍设定”，并新增创作规则、写作进度和角色当前状态快捷入口。

### Fixed

- Agent：写作模式生成小说章节时，开启 `hide_novel_chapter_body_in_live_output` 后，SSE 推流前 middleware 会在 `write_file` 写入 `chapters/` 或 `drafts/` 时只向前端发送目标文件路径、隐藏提示和已生成字符数，不再输出大量章节正文或省略号占位；字符进度会按增量轻量节流，并在工具结束前用完整参数解码校准最终值，口径与 `wc -m` 保持一致，前端工具卡片会提示章节正文仅在实时输出中隐藏、文件仍会正常写入。
- WebUI：允许 pnpm 在安装时执行 `msw` 的构建脚本，避免高版本 pnpm 首次安装后因 `ERR_PNPM_IGNORED_BUILDS` 导致前端启动失败。
- WebUI：修复 Agent 对话、SubAgent 详情栏和工具流式预览在输出增长时不会稳定锁定到底部的问题；现在默认跟随到底部，用户主动上滑后停止自动滚动，重新滚到底部后再恢复跟随。
- WebUI：修复创作 Agent 输入动作菜单里的写作 Skill 列表需要鼠标悬停后才开始加载、首次展开慢一拍的问题；现在创作 Agent 面板打开时就会预加载写作 Skill 列表和默认选择。
- Agent：修复自定义 SubAgent 在互动故事父 Agent 下可能绕过写文件拦截的问题，并让配置管理 SubAgent 的专属读写工具遵守自身工具权限限制。
- WebUI：修复浅色主题下 SubAgent 删除确认弹窗危险按钮对比度不足的问题，并将基础弹窗宽度改为随视口自适应，避免自定义 SubAgent 编辑等弹窗过窄。
- Agent 模型：所有 Agent 请求不再主动设置 `max_tokens` 输出上限，避免长章节通过 `write_file` 写入时工具参数在正文中途被截断并报 JSON EOF。
- WebUI：修复对话区思考过程和工具调用卡片 hover 时也显示消息时间、并导致列表高度变化的问题；现在仅用户消息和 Agent 正文消息显示悬浮时间，时间戳使用绝对定位不再撑开页面。
- WebUI：修复 `execute` 等工具执行完成后，对话页工具调用卡片可能仍停留在 Loading 状态的问题；工具结果现在会按调用 ID 或工具名回填到原卡片，正常结束时也会收敛未完成卡片。
- Agent 工具：Windows 运行时现在通过 PowerShell 支持 `execute` 命令执行工具，不再强制关闭 `shell_execute`，Agents 设置页也允许正常配置该开关。
- Agent 运行：修复写作 Agent 连续调用多个工具后，如果模型或工具流长时间不再返回事件，后端任务会永久保持 running、前端一直显示回复中的问题；现在主循环、助手流和工具结果流都有可配置空闲超时，默认 180 秒，超时会结束任务并返回错误。
- Agent 会话：修复 `write_file` 等工具流式参数每帧都重写 `.nova/sessions` 导致 Windows 文件写入容易出现 open 超时和重复错误日志的问题；工具参数展示改为内存实时累积、磁盘节流持久化，并对超长参数只保存有界预览。
- WebUI 编辑区 Tab：修复点击标题文字之外的 Tab 区域不会切换文件、容易感觉需要点两次的问题；现在整个 Tab 条目都可点击，关闭按钮仍独立关闭。
- Windows Release：修复设置页和文档将局域网访问地址误指向开发前端端口的问题；release 现在展示实际 Nova 入口端口，避免手机访问到未监听的 `5173`。
- 修复应用内安装更新缺少下载进度且下载包只保存在临时目录的问题；安装现在使用 `grab` 下载 Release 安装包到本地 `.nova-updates/downloads/`，通过前端进度条展示下载阶段，完成后再解压并替换本地文件，同时修复 Windows 安装路径含空格时更新脚本可能无法启动的问题。
- 后端：修复写作 Agent 启动日志仍引用已移除的 `style_references` 请求字段导致后端编译失败的问题，日志现在记录当前场景风格选择数量。

## [v0.1.13] - 2026-06-24

### Fixed

- WebUI：修复所有 Agent 输入框长文本可能被右侧按钮遮挡的问题；输入区和按钮现在由 composer 组件分槽布局，内容换行后会保持多行输入，清空后恢复单行。
- 修复手机宽度下版本管理、书籍管理、Agents、Skills、设置、自动化和故事记忆等页面的适配问题；共享滚动容器现在会按手机宽度收缩，配置卡片会换行，长 Skill 名称不再撑宽，书架手机端改为单列卡片。
- WebUI：修复叙事编排场景风格内容编辑弹窗在长文本输入时内容区撑开导致无法滚动的问题，并将弹窗保存按钮调整为 Nova 主题色。
- WebUI：删除超过 1 秒或接近 1 秒的前端长链测试文件和用例，避免 CI 因低效测试超时失败。
- WebUI：修复互动故事“故事线”选择面板与“叙事”选择面板相同的首次打开只显示部分选项问题；现在故事线选择也改为打开即全量渲染，长列表由面板整体滚动。
- WebUI：修复互动故事“叙事”选择面板首次打开只显示部分选项、滚动后才补齐的问题；现在改为打开即渲染全部叙事方案，长列表由面板整体滚动。
- WebUI：修复互动故事底部输入区在多行输入或展开“可选择”行动建议时遮挡最新故事文字、且消息列表无法继续滚到最后一行的问题；现在故事消息区会按底部浮层实际高度预留滚动空间，行动建议列表也改为纵向可滚动。
- WebUI：互动故事输入框输入 `/` 展开 Skills 选择浮层时，现在和写作 Agent 一样支持按 Tab 选中当前 Skill。
- WebUI：修复写作 Agent 与互动故事对话输入框单行状态下文字垂直偏上的问题，输入内容和 placeholder 现在在紧凑 composer 内垂直居中。
- WebUI：修复输入框输入 `/` 时 Skills/命令选择浮层会被组件库默认选中态颜色覆盖、与 Nova 灰黑主题不一致的问题，浮层背景、选中态和图标颜色统一使用中性主题变量。
- 修复写作页章节列表点击“确认成章”时按钮保存中显示不可操作光标的问题；现在保存期间仅显示旋转 Loading，鼠标保持常规按钮反馈，空章节仍保持禁用态。
- 修复互动快捷选择和互动记忆 Agent 在本地 LM（如 LM Studio）下生成失败且错误信息为空的问题：这两个 Agent 此前强制使用 `response_format=json_object`，部分本地 LM 服务器不支持该参数会返回空错误；现在先尝试 JSON mode，失败后自动降级为普通文本模式重试，与小说导入工具 Agent 的降级策略一致。
- 修复本地 LM 返回空错误时日志只显示前缀（如"生成互动快捷选择失败: "）的问题：现在会记录错误类型并补充可读描述，便于诊断本地 LM 兼容性问题。

### Added

- 配置管理 Agent 新增复杂配置资源的自动 Skill 注入：自动化、故事记忆和叙事编排写入前会按模块加载对应内置配置 Skill，帮助 Agent 使用正确 JSON 结构、枚举和写入流程。
- 新增 `CONTRIBUTING.md`，整理本地开发、代码风格、前端验证、测试、提交信息、文档和发布贡献约定。
- 开发模式下新增完整 LLM 输入 JSONL 日志：通过 `bootstrap.sh` 启动时会向后端传入 `--dev-mode`，每次模型请求都会把未截断的 messages、工具 schema 和非密钥模型参数写入 `log/llm-inputs.jsonl`，最多保留最近 10 条记录，便于排查前缀缓存命中率；直接运行 binary 默认不写该文件。
- 真实模型用量新增未命中缓存 Token 统计，按整次 Agent 请求和单次模型调用同时展示 `prompt - cached` 的输入 Token 数。
- 检测到 Nova 新版本时，一级菜单会显示可关闭的小提示；关闭后同一版本不再重复提示。
- 上下文压缩 Agent 改为流式输出摘要增量，IDE 与互动故事的对话区会以小窗卡片展示压缩阶段、token 进度和摘要预览，用户可直接看到自动压缩进展。
- 写作 Agent 与互动故事输入菜单新增真实模型用量明细，按每次模型请求列表记录 prompt、cached prompt、completion、reasoning、total tokens、模型调用次数和缓存命中率。
- 所有 Agent 对话输入框左侧选项菜单新增模型配置快速切换，可直接为写作、互动故事、配置管理和自动化 Agent 保存当前工作区的模型配置。
- `bootstrap.sh fe` 新增 `--lan` 和 `--host <host>` 选项，可将 Vite 前端开发服务绑定到局域网可访问地址，并输出可在手机等设备打开的局域网地址。
- 设置页新增局域网访问控制：用户可开启同一局域网设备访问 Nova，查看其他设备访问地址，并配置远程访问用户名和密码；非本机访问会通过 HTTP Basic Auth 校验，密码仅以哈希形式保存。
- WebUI 内部页面新增移动端自适应面板：设置、Agents、Skills、自动化、故事记忆和互动设置等带左右侧栏的页面在手机宽度下改为左右滑出抽屉，主内容保持优先展示。
- 局域网访问的登录入口改为前端页面：设置页展示 `5173` 前端访问地址，Vite 代理会转发真实客户端地址，后端会拒绝未登录的远端请求。

### Changed

- 写作 Agent 与互动故事的场景风格规则改为注入 system prompt，不再追加到本轮动态用户消息；上下文分析会把已选规则显示为 SystemPrompt 来源，保留 32k 字符上限。
- 叙事编排的场景风格规则改为直接保存文字内容，不再保存或读取风格文件路径；`/api/styles` 与输入框 `#` 文件引用能力已移除，`#` 现在只用于选择当前叙事编排中的场景风格。旧 `style_rules.styles` 配置不再兼容，需要在叙事编排中重新上传或填写 `style_contents`。
- WebUI：优化写作 Agent 与互动故事输入框 UI，改为悬浮在对话上的紧凑单行圆角矩形 composer；模型配置保留在输入动作菜单内，互动快捷选择移到右侧发送区，移动端互动输入也保留明确发送入口。消息项鼠标悬停时会在下方显示发送时间，当天仅显示 24 小时时间，历史消息显示完整日期时间。本次不新增 permission 配置或语音输入入口。
- 简化多模型配置：设置页新增模型配置时不再要求单独填写配置 ID 和显示名称，用户只需填写 Base URL、API Key 和模型名；后端会用模型名作为默认内部配置 ID，旧配置的 `id` / `name` 字段仍可继续读取。
- 写作 Agent 的模型可见作品上下文改为 stable/dynamic 两段：创作灵感、大纲和资料库作为 stable context 放在对话历史与压缩摘要之前，章节组细纲、章节目录概览、进度和角色状态作为 dynamic context 放在本轮用户请求之前，以提升前缀缓存稳定性。
- 写作 Agent 的大纲、进度、角色状态、章节目录概览、资料库摘要和章节组细纲等作品状态不再注入 system prompt；会话历史仍只保存用户原始请求，运行时作品快照只在模型调用前临时组装。
- 写作页面顶部文件 Tab 切换改为即时状态更新，移除无实际帮助的切换动画。
- 写作模式章节状态从字数阈值自动判定改为作者手动确认：非空章节默认保持初稿，只有在章节列表中确认后才标记为成章。
- Nova 后端默认改为只监听本机地址；开启局域网访问后才监听 `0.0.0.0`，关闭后远端请求会被拒绝。
- 移动端互动剧情页改为阅读优先布局：故事舞台顶部操作、工作台状态提示与底部一级导航默认收起，按需展开，减少手机上对正文区域的占用。
- 更新中英文 README 的能力介绍，补充自定义故事记忆、Memory Compact、缓存命中率优化和 token 成本说明，并同步欢迎交流、开发启动和赞助信息。
- 移除压缩前的上下文回合窗口裁剪；未触发压缩时模型上下文保持当前有效对话链，压缩后保留的原文尾部回合数改由 `context_compaction` Agent 配置，默认 1 回合。

### Fixed

- 修复豆包等输入法/语音输入仍在组合或后处理文字时，Agent 和互动输入框按 Enter 会误发送未定稿文本的问题。
- 修复配置管理 Agent 在自动化、资料库、故事记忆、叙事编排和 Skills 等不同配置入口之间共用同一段对话历史的问题；现在会按入口和目标资源隔离历史与 `/clear`。
- 修复写作 Agent 上下文分析器会按作品状态 Markdown 小标题误拆来源的问题；现在作品状态按创作灵感、状态文件、章节目录、资料库和章节组细纲等真实来源展示。
- 修复上下文压缩运行时同时出现压缩卡片和 activity 卡片的问题；现在 IDE 与互动故事只保留一个简洁压缩卡片，并用旋转 Loading 表示进行中状态。
- 修复互动故事移除上下文压缩时写入的 `context_compaction_removed` 事件被故事 schema 误判为未知类型，导致移除压缩失败的问题。
- 修复真实模型用量明细缺少数据来源说明、模型调用未按 Agent 请求分组、工具调用后的下一次模型请求缺少工具归属、单次调用时间不准确且窄屏必须横向滚动才能看到关键 token 信息的问题；互动模式的模型用量改为写入独立 usage 文件，不再混入 story 事件。
- 修复真实模型用量与上下文分析弹窗打开时默认聚焦右上角关闭按钮，导致关闭按钮一开始就高亮的问题。
- 修复应用内安装更新下载较慢时可能因请求超时失败的问题；安装现在使用 GitHub Release 直连下载地址，下载完成后解压替换本地文件，并在完成后提示重启生效。
- 修复互动故事流式输出期间系统 prompt 组成日志可能按 chunk 高频重复打印的问题；现在每次 Agent 请求只记录一次 system composition/source 摘要。
- 修复互动模式仍会持续轮询工作区目录、作品统计和风格参考的问题；后台自动刷新现在只在 IDE 写作页启用，互动页保留首次加载和显式刷新。
- 修复互动模式刷新页面后可能回到其他故事线或主线的问题：前端会记住最近选择的故事线，并按故事恢复最近选择的分支。
- 修复互动故事切换较早回合版本后会截断后续正本路径的问题；同一回合的多个版本现在只作为该回合候选，选中后后续对话继续沿当前正本保存与刷新。
- 修复互动故事后台状态/记忆任务晚于下一轮完成时可能把分支头回退到旧回合的问题，避免刷新后最新回合从故事舞台消失。
- 修复互动记忆和快捷选项未沿用互动叙事 Agent 压缩后模型可见历史的问题；现在三者都会使用同一份压缩摘要、保留尾部和压缩后新增回合。
- 修复压缩后保留原文尾部回合数被固定为 8 且 Agents 设置项不可见的问题；现在可在 `context_compaction` Agent 中配置，默认保留最近 1 回合。

## [v0.1.12] - 2026-06-20

### Added

- Agent 主流程新增基于 token Context Usage 的自动上下文压缩：模型配置可设置上下文上限（默认 400K，预设 200K / 1M，并支持自定义），Agents 页可配置自动压缩开关、触发阈值（默认 90%）和摘要目标比例。
- 上下文分析器新增模型可见压缩摘要、估算 token 使用量、上下文窗口、使用率、压缩 epoch 和是否将触发压缩等信息，用户对话历史仍保持未压缩原文展示。
- 新增独立 `context_compaction` Agent，可在 Agents 页单独配置模型、thinking、reasoning effort、压缩提示词和目标压缩比例范围（默认 5%-20%）；默认不启用工具和 Skills。
- IDE 与互动故事新增 `/compact` 主动压缩命令，并新增上下文压缩 API；上下文分析器可查看 active 压缩摘要并软移除压缩，让模型上下文恢复原始消息链后可重新压缩。
- Agents 配置页新增每个 Agent 独立的自动压缩阈值配置，并由运行时统一按 Agent kind 生效。
- 网页搜索工具（`web_search`）由单一 DuckDuckGo 升级为 DuckDuckGo、Bing、百度、Google 四引擎并发聚合：四引擎多线程并行搜索，失败的引擎结果会被直接丢弃，仅合并成功引擎的结果。
- 写作 Agent 和互动故事输入框新增上下文分析入口，可模拟当前输入发送并展示真实 SystemPrompt、上下文来源明细和实际消息列表，不调用 LLM、不写入会话或故事回合。

### Fixed

- 修复写作编辑器自动保存未使用用户配置且语义容易被理解为定时保存的问题；现在只会在用户修改内容后按配置延迟触发一次保存，外部文件同步不会触发自动保存。
- 修复 Agent 修改当前文件后，前端旧自动保存请求可能覆盖 Agent 新内容的问题；工作区文件保存现在会基于读取时的文件 revision 做冲突保护。
- 修复后端未启动或 Vite 代理返回 502/503/504 时前端点击无反馈的问题，现在本地 API 连接失败会用去重 Toast 提示“后端未启动”。
- 修复互动模式中“版本管理”一级菜单与其他共享菜单行为不一致的问题：版本管理纳入互动菜单默认顺序，切换写作/互动模式时会退出版本页并保持单一一级菜单高亮。
- 互动记忆 Agent 生成的故事记忆 patch 兼容数值和布尔值字段，并在生成、解析或写入失败后携带错误原因最多重试 3 次。
- 互动记忆 Agent 生成故事记忆 patch 时强制按目标表输出完整字段，未变化字段沿用既有记忆，无法确认的字段也要写明待确认原因，避免表格字段缺失。
- 故事记忆表格改为自适应列宽和展开详情网格，减少展开前后的横向滚动。
- 互动右边栏故事记忆拆分为“记忆内容”和“整理过程”两个页签，整理日志不再挤占记忆列表空间。
- 自动触发的故事记忆整理改为由右边栏消费 pending 回合并流式展示整理过程，避免后台整理无过程输出。
- 自动触发故事记忆整理时不再自动切到“整理过程”页签，用户当前停留在“记忆内容”时保持不被打断；手动整理仍会打开整理过程。
- 修复设置保存后一级菜单可能被排序拖拽状态标记为不可用的问题；设置页作为覆盖页打开时，其他一级菜单保持原样式并可直接点击切换。
- 修复互动故事启用上下文压缩后只保留压缩之后新增回合、且压缩摘要同时出现在 `Nova Context Compaction` 和“较早剧情压缩记忆”里的问题；现在模型上下文只保留一个压缩摘要，并追加固定原文尾部。
- 修复 `context_compaction` Agent 摘要策略配置不生效的问题；摘要目标比例现在统一读取独立上下文压缩 Agent 配置。

### Changed

- Agent 模型上下文不再只依赖固定回合数滑动窗口；启用上下文压缩且模型配置有上下文上限时，会先保留完整有效上下文直到达到阈值，再写入 append-only 压缩 epoch 并用“压缩摘要 + 保留尾部”继续后续回合，以提升前缀缓存稳定性并降低长上下文遗忘风险。
- 上下文压缩源改为原始有效对话链：IDE 自动压缩排除当前最新用户消息、旧压缩摘要和 reasoning/thinking 展示内容；互动故事压缩使用当前分支 user+narrative 原始回合链，并额外注入有硬上限的 Story Memory reference，提示词强调优先保留剧情纪要、用户意图、重要事件、角色关系和状态变化。
- 上下文分析器不再把 active 压缩摘要作为顶部独立卡片展示；压缩元信息和“移除压缩”按钮改为跟随最终模型消息里的 `Nova Context Compaction` 消息展示。
- 上下文压缩 Agent 默认提示词改为“事件时间线记忆”结构，按事件时间线、长期影响账本、当前阶段快照和已合并/舍弃信息输出，强化互动小说长期状态、用户行动、角色关系、物品资源、线索伏笔和当前停顿点保留。
- 叙事编排不再配置上下文回合数，互动故事、互动记忆、快捷选项、写作、配置管理和自动化等 Agent 改为读取各自的 Agents 上下文策略配置；旧叙事编排里的回合窗口字段运行时不再使用。
- 叙事编排中的 `state_memory` 可见名称改为“记忆沉淀规则 / Memory Rules”，内置叙事编排版本刷新到新版命名；内部 target 仍保持 `state_memory` 以兼容已有自定义配置。
- 互动故事 Agent 的动态上下文从消息列表开头移动到本轮用户消息末尾，历史消息保持在动态上下文前，减少故事状态、字数目标和故事记忆变动对 LLM 前缀缓存命中的影响。
- 互动记忆 Agent 内置提示词改为按故事记忆表结构逐字段填表，明确使用历史回合上下文、资料库人物设定和既有故事记忆作为生成来源，并在 Agents 配置页展示新版 `story_memory_patches` 输出协议。
- 新增统一 `config_manager` Agent，替代旧资料库 Agent 和叙事编排 Agent；资料库、叙事编排、Skills、自动化和故事记忆模块改为内嵌同一个配置管理 Agent，并通过各自 list/read/write 工具直接更新资源。
- 移除旧 `lore_editor` / `teller_editor` Agent kind、专属前端聊天组件、专属会话入口和 `/api/lore/agent*`、`/api/interactive/tellers/agent*` API；Beta 阶段不做旧会话和旧接口兼容迁移。
- 配置管理 Agent 不暴露设置修改工具；模型、提示词、Skills 和工具权限仍在 Agents 配置页针对 `config_manager` 自身配置。
- 优化故事记忆表格展示，增加固定记录列、字段说明和长文本展开细节；记忆结构新增结构级和字段级生成要求配置，并注入互动记忆 Agent 的故事记忆结构上下文。
- 故事记忆内置预设表替换为 6 张默认开启表：当前状态、主角信息、重要角色、世界上下文、进行中事项和剧情纪要；新增恋爱关系档案、恋爱日记和成人向关系档案 3 张默认关闭的可选表，结构和字段支持启用/关闭，关闭内容不会进入 Agent 上下文或自动整理写入。本次不兼容旧内置预设表结构，自定义结构保留。
- 故事记忆记录的“隐藏/恢复”改为“归档/恢复”：API 字段改为 `archived`，操作路由改为 `/archive`，归档记录默认不进入右侧面板、Agent 召回、故事记忆上下文和自动整理依据；右侧面板改为浏览入口，编辑与归档统一到故事记忆管理页处理。

## [v0.1.11] - 2026-06-18

### Added

- 互动剧情新增故事级开场白配置：创建故事或进入舞台后可选择 AI 自动生成、预设开场或自定义开局；空故事可一键生成开场，生成后的首轮继续支持刷新和版本切换。
- 互动模式新增独立一级模块“故事记忆 / Story Memory”，支持按结构管理当前状态、主角信息、重要角色、任务事件和剧情纪要；用户可新增自定义结构和纯文本字段，按当前故事线和分支查看、编辑、隐藏或恢复记忆内容。
- 故事记忆存储升级为 `interactive/memory/story-<storyID>.json` v2，新增 `settings`、`structures` 和 `records`；新分支会继承分叉点前的可见记忆，分叉后的编辑和隐藏会在当前分支 copy-on-write，不污染父分支。
- 故事记忆新增自动整理配置，默认每 3 回合触发一次后台整理，也支持在故事记忆模块中手动触发整理。
- 互动模式新增按故事和分支隔离的故事记忆召回工具，互动故事 Agent 可通过只读工具按当前分支主动召回相关记忆。
- 互动故事新增兼容记忆 API 和右侧故事记忆预览面板，支持搜索、手动新增、编辑、软隐藏和恢复记忆。
- 设置页新增应用更新检查与安装：后端通过 GitHub latest Release 匹配当前平台安装包，前端支持自动检查、手动检查、一键安装并提示重启生效。
- WebUI 新增移动端工作台布局：窄屏下使用底部一级菜单、项目目录抽屉、创作 Agent 抽屉和互动场景记忆抽屉，避免桌面可拖拽面板在手机宽度下挤出主编辑/剧情区域。
- WebUI 左侧一级菜单支持拖拽排序，写作模式与互动模式分别保存顺序，避免两种工作台入口互相影响。
- 书籍管理页新增从书架移除和拖拽自定义排序；移除书籍只会从书架隐藏并保留磁盘目录，删除当前书籍后会自动切换到下一个可用书籍。
- Agent loop 新增 `LoopPolicy`、`ContextLedger` 和 `.nova/runs` 运行账本，按轮记录上下文来源、大小上限、事件摘要和完成状态，为后续工具筛选、恢复和验证阶段提供稳定工程边界。
- Agent loop 新增中心化 tool manifest 与模型可见工具结果筛选，统一标注工具来源、是否变更 workspace、输出上限、幂等键和 post-check 要求，并对 invokable/streamable 工具返回做有界回填。
- 创作 Agent 新增写入后轻量验证阶段，会根据工具 mutation metadata 检查写入路径、章节目录约束、资料库 `brief_description` 和删除结果，并写入 `.nova/runs` trace。
- WebUI 创作 Agent 面板新增 Agent Trace 视图，可查看最近运行的上下文账本、工具事件序列、验证结果和截断状态。
- WebUI 接入 Motion for React，新增全局动效强度配置（跟随系统、完整、减少、关闭），并为工作台切换、一级菜单、Tab、面板和聊天消息提供更克制流畅的过渡。
- 设置页新增浅色、深色和跟随系统主题切换；主题配置支持用户级和工作区级继承，并即时应用到主工作台。
- 浅色/深色主题主文字分别使用纯黑/纯白，写作编辑器主题会跟随全局浅色/深色切换；默认界面字体改为 Apple 字体栈，界面字号改为 14px。
- 默认主题改为深色模式，首次启动和未配置主题时会进入 dark theme。
- 自动化新增自定义触发条件与 Trigger Inbox：支持定时触发器和由 LLM 基于有界章节上下文判断的语义触发器；触发后可按任务级行为配置为确认后执行、自动执行或仅通知，定时任务可选择静默执行或写入收件箱。
- 自动化任务支持从现有多模型配置中选择任务级模型配置；未选择时继承 Automation Agent 默认模型。
- 自动化新增写作模式章节批次触发器，可按每 N 个非空章节触发 review、续写或自定义任务，并在 Trigger evidence 中记录本批次章节路径、标题、字数和更新时间。
- 工作区自动化会预置“续写章节”和“自动 Review”两个默认关闭任务，用户可直接启用并调整触发和写入配置。
- 自动化运行会把触发 evidence 作为有界触发范围传给 Agent；默认“自动 Review”聚焦本次新增章节，并对照用户任务、`CREATOR.md`、大纲、角色和必要前文检查质量与一致性。
- 预置自动化任务会把默认 Prompt 直接写入任务配置，用户可在自动化页自由修改；运行时不再根据内部 template 套用不同 Prompt。
- 酒馆角色卡导入预览新增兼容性检查报告，展示已导入、降级导入和暂不兼容的 Tavern 字段，并提示 PNG 封面、开场预设和 `{{user}}` 玩家角色导入计划。
- 资料库条目新增启用/停用状态；停用条目会保留在编辑页但不会进入资料库索引、读取工具或模型上下文。

### Changed

- 酒馆 PNG 角色卡导入会把 PNG 本体写入书籍目录 `assets/image/cover.png` 作为封面图；`first_mes` 和 `alternate_greetings` 不再写进资料库角色条目，而是同步到书籍级预设开场白。
- 酒馆角色卡导入会同步世界书条目的 `enabled` 状态，并在检测到 `{{user}}` 占位符时允许用户自定义玩家角色资料名称。
- 互动模式每回合默认目标字数从 1200 调整为 2000，并统一为前后端默认值常量。
- 互动剧情每轮目标字数现在作为 story 级最高篇幅约束注入，覆盖 CREATOR.md 章节篇幅、导演规则和 Nova 内置提示中的其他篇幅倾向；后端兜底默认值同步为 2000。
- 互动记忆 Agent 输出协议从 `state_ops + memory_entry` 调整为 `story_memory_patches`，旧 `memory_entry` 输出会兼容映射为 `plot_summary` 故事记忆；旧 `/api/interactive/stories/:id/memory` 接口继续保留，并映射到故事记忆记录。
- 互动记忆 Agent 生成故事记忆时会注入有硬上限的资料库上下文，优先提供完整重要资料并为未展开条目保留索引，减少记忆记录与作品设定偏差。
- 互动模式不再把“当前状态”作为独立用户管理入口，当前时间、地点和事件改由故事记忆的默认结构维护；右侧记忆面板改为故事记忆预览，不再展示原始状态 JSON。
- 资料库不再维护独立版本和 `.nova/lore/versions` 自动备份，资料条目跟随工作区整体版本管理统一保存与恢复；对应 `/api/lore/versions` 专用接口和资料库 Agent 面板中的版本入口已移除。
- WebUI 移除独立的“创作者”一级菜单，`CREATOR.md` 改为在资料库页面内作为固定条目统一管理，仍保留 workspace 根目录文件和 Agent 注入契约。
- WebUI 将“版本管理”调整为写作模式和互动模式共享的一级入口，打开时覆盖当前工作区但不自动切换写作/互动模式。
- WebUI 将用户可见的 “IDE 模式 / Novel IDE” 统一改名为“写作模式 / Writing Mode”，内部 `ide` 配置键和存储 key 保持兼容。
- “状态记忆 Agent”用户可见命名合并为“互动记忆 Agent / Interactive Memory Agent”，继续兼容内部 `interactive_state` 配置键，同时负责状态快照和长期纪要生成。
- 互动故事上下文不再由后端默认整段预注入资料库和长期记忆；互动 Agent 默认 system prompt 会引导 Agent 使用 `list_lore_items` / `read_lore_items` 与 `list_interactive_memories` / `read_interactive_memories` 主动召回，且 Agents 页会直接展示可编辑的默认 system prompt。
- Agents 页的 System Prompt 改为按来源折叠展示：运行契约、输出格式、CREATOR.md、作品状态/资料库注入和叙事编排只读，流程规则与用户自定义规则可分别在用户配置或工作区配置中编辑。
- 旧自动化定时任务会自动迁移为任务级 `auto_run` 与 `silent` schedule trigger，保留原有到点自动运行且不额外通知的行为；新建自动化默认触发后先进入确认流程。
- 自动化触发器不再单独配置触发后动作，触发器只负责触发条件和通知方式；任务触发后的运行方式统一由“执行模式”决定。
- 自动化写入配置拆分为统一的 `write_mode` 与 `write_scope`，前端展示为“执行模式”和“写入范围”：支持自动只读执行、自动出方案后确认写入、自动执行并写入；旧 `write_policy` 会按原语义迁移并继续作为兼容字段回填。
- 自动化配置页不再暴露“模板”选择，任务以具体自动化目标和 Prompt 为中心配置。
- 互动故事单轮目标字数改为故事级运行参数，并在互动剧情主舞台顶部直接配置；不再兼容叙事编排 JSON 中的 `reply_target_chars` 旧字段，旧规则包里的该字段不会继续生效，需要在具体互动故事里重新设置。
- 精简互动剧情主舞台顶部和消息区抬头，移除“互动创作”、回合数以及“指令流 / 记录数”状态栏，降低控制区拥挤感。
- 中英文 README 补充写作模式与互动模式的职责边界，明确互动模式是独立互动娱乐工作台，写作大纲、章节进展和 `progress.md` 不会自动进入互动模式。
- 优化中英文 README 首屏定位与能力说明，补充写作模式作品管理、创作 Agent、互动故事、结构化资料库、版本管理、Skills/Agents、自动化和导入能力介绍。
- 互动故事 Agent 上下文改为按叙事编排回合窗口配置保留原文回合尾部，并将更早剧情压缩为有界摘要，避免长线互动把完整历史无限注入模型。
- Nova favicon 去掉右下角 `I` 标记，并改为三色清爽的 iOS 风格图标。
- 重新设计 Nova 极简 SVG 品牌图标，并在中英文 README 首屏顶部展示品牌图标。
- 新增与图标同风格的 Nova wordmark SVG，并重组中英文 README 首屏介绍，强化 AI-native fiction workspace 的高级创作工作室定位。

### Fixed

- 修复各处保存按钮在“保存中”状态下因文案变宽导致按钮抖动的问题；保存按钮现在保持“保存”文案不变，仅将前置图标切换为加载中转圈，宽度始终稳定。
- 修复一级菜单栏被隐藏侧栏的可调整宽度命中区覆盖，导致鼠标显示为宽度调整形状且项目侧栏拖拽调整失效的问题。
- 修复设置页新增或修改模型配置后，Agents 页模型配置下拉不会立即刷新、必须整页刷新才出现的问题。
- 修复设置页每次打开都会自动请求 GitHub 更新检查的问题；自动检查现在会在浏览器本地记录时间，1 小时内不重复检查，手动检查不受影响。
- 修复资料库 Agent 固定会话可能出现在创作 Agent 会话列表并被切换使用，导致创作对话和资料库 Agent 对话串在一起的问题；普通创作会话现在会过滤并拒绝操作固定 Agent 会话。
- 修复设置页“重启服务”只让后端进程退出而没有重新启动的问题；后端现在会用当前可执行文件、启动参数和环境变量替换当前进程，并在无法安排重启时返回明确错误。
- Agent 追踪不再把正文流、thinking 增量、工具参数增量和完成状态等 SSE 传输事件逐条写入 `.nova/runs`，只保留工具调用、工具结果和异常等语义事件，降低运行追踪噪音和空间占用。
- 修复自动化章节批次触发器会因章节字数、更新时间变化或 `trigger_state` 丢失而重复触发同一批章节的问题；同一批次现在按章节路径和历史 Inbox evidence 去重。
- 修复自动化 auto-run 触发启动失败时会把 Inbox 标记为已自动执行且无可重试入口的问题；失败通知现在会转为待确认并保留错误摘要。
- 修复语义触发器使用滚动最近上下文导致适用范围不明确的问题；语义触发现在按每 N 个非空章节批次检查，并只把本批章节作为 LLM 判断范围。
- 修复 Agent `edit_file` 在 `old_string` 仅因行尾空格或 Tab 与文件内容不一致时直接失败的问题；现在仅在归一化后仍能唯一定位片段时才会执行替换，避免模糊匹配误改。
- 修复设置页多模型配置编辑配置 ID 时输入框随 ID 变化反复重建，导致只能逐字输入的问题。
- 优化互动剧情页和工作台侧栏的数据加载稳定性：切换故事、分支或刷新目录时保留上一份有效内容并显示轻量刷新状态，减少后端响应较慢时的页面抖动。
- 修复互动模式分支路线节点在紧凑字号下标题、摘要或 HEAD 标记挤出卡片的问题。
- 修复浅色主题下创作 Agent 对话、互动剧情命令菜单、一级菜单、文件树、全局命令面板、Tooltip、版本差异弹窗和错误提示仍使用暗色硬编码导致文字或图标对比度不足的问题。
- 调整浅色主题的工作台层级色，统一一级菜单、上下栏、侧栏、对话区和编辑器 IDE 背景，去掉浅色模式下割裂的纯白栏和内容区渐变。

## [v0.1.10] - 2026-06-12

### Fixed

- 工作区文件删除不再依赖系统回收站，改为删除前保存 Nova 版本快照后直接删除，并同步更新中英文确认文案。

## [v0.1.9] - 2026-06-12

### Changed

- Skills 新建体验改为主编辑区引导式流程：左侧专注浏览现有 Skills，新建时集中填写保存位置、名称、触发说明和可用 Agent，并在创建后直接打开生成的 `SKILL.md`。
- Skills 支持按 Agent 分工：内置 Skills 新增默认可用 Agent 范围，互动叙事 Agent 默认启用 Skills；Agents 页可按 Agent 覆盖单个 Skill 的启用/禁用，Skills 创建表单可选择新 Skill 可用的 Agents。
- 项目文件树改为始终显示真实文件/目录名，不再把 `ideas.md`、隐藏排序前缀章节等映射成展示名；作品目录新增 `ideas.md` 灵感入口用于快速打开创作灵感文件。
- 自动化页面右侧改为“任务配置 / 运行过程”双页签布局；运行过程复用创作 Agent 消息流和输入框，支持在单次自动化运行会话中继续追问，新运行会清空并创建独立运行过程。
- P1 复杂度治理：新增 Agent kind/tool capability registry，模型、工具、prompt 配置解析和后台 Agent 会话 ID 统一从 registry 获取；deep agent 构建参数收敛为运行时 spec，降低新增 Agent 时的分支同步成本。
- 互动故事 JSONL 存储新增 typed event envelope 与 state op schema 校验，读取/写入/快照构建统一经过事件类型、schema version、ID、branch 和状态操作校验。
- 前端 API client 拆分为 `api-client` 领域模块，`@/lib/api` 保留兼容 barrel；互动和设置 API 复用共享 JSON/SSE 客户端，避免重复 fetch/parser 逻辑。
- WebUI i18n locale 按 key namespace 拆分为独立资源文件，新增 `npm --prefix web run check:i18n` 校验中英文 key 对齐、重复 key 和 namespace 前缀。
- P0 工程治理：拆分 Agent 聊天主流程、互动故事存储/快照/状态逻辑，以及互动设置面板的 Agent 对话、目录/编辑器和叙事编排编辑器组件，降低核心文件体量和职责耦合。
- Agent 运行和后台任务关键路径改用 `slog` 结构化日志，沿用现有日志输出目标，并为任务生命周期、事件广播、上下文组装和中断恢复输出稳定字段。
- Agent 上下文审计新增结构化来源明细，记录每个注入片段的来源、标题、字节数、字符数、预览和备注，方便排查模型实际可见上下文。
- README 新增微信交流图与“快速迭代中，欢迎交流”说明。
- README 合并“为什么选择 Nova”、核心能力和推荐创作流程，简化 Nova 与普通 AI 小说工具的差异说明。

### Added

- 自动化任务新增运行流式过程视图：手动和定时触发都会生成独立运行会话，前端可像新聊天一样查看 thinking、工具调用、输出过程，并可从最近运行回看完整历史。
- 新增 GitHub Actions CI，在 push/PR 上执行 whitespace 检查、`go test ./...`、前端测试、前端构建和完整 `./build.sh`。

### Fixed

- 修复支持 Skills 的输入框提示不明确的问题；在当前 Agent 有可用 Skills 时，输入 placeholder 会提示可输入 `/` 选择 Skills，互动剧情输入框也支持 `/` Skills 候选与键盘滚动跟随。
- 修复支持 Skills 的 Agent 输入框没有统一展示 `/<skill-name>` 候选的问题；资料库 Agent 和自动化运行对话现在会按工具权限展示 Skills 候选，并修复 `/` 候选列表用上下箭头切换时高亮项不跟随滚动的问题。
- 修复自动化任务流式输出把每个 thinking 片段拆成独立思考过程的问题；自动化运行复用创作 Agent 的共享 SSE 消费逻辑，统一 thinking、正文、工具调用和参数增量展示。
- 修复作品目录和项目文件定时刷新时短暂进入 loading 状态导致侧栏内容抖动的问题；后台刷新失败时也会保留当前目录和作品进度。
- 修复 Skills 管理中单独打开被工作区覆盖的用户级 `SKILL.md` 时仍显示为可用的问题；创建/保存后也会按完整搜索路径返回真实 Active 状态。
- 修复首次启动 `.nova` 下没有书籍或未选工作区时，前端仍请求目录、统计、styles、chat session 和 active chat 等工作区 API 导致后端报错的问题；空书架会先引导用户创建或导入书籍。
- 修复新建 Skill 默认 `SKILL.md` 在描述包含换行、冒号或列表符号时可能生成非法 YAML frontmatter 的问题。
- 修复创作 Agent 输入框在 IDE Agent 关闭 Skills 工具后仍展示 `/<skill-name>` 命令的问题。
- 修复 Agents 页 Automation Agent 工具权限前端兜底值与后端默认配置不一致的问题。
- 修复内置叙事编排缺少 `screenwriter` 预设导致回归测试失败的问题，并让内置刷新测试跟随当前预设名称。

## [v0.1.8] - 2026-06-11

### Added

- Agent 工具权限新增 `web_search`，使用 Eino Ext 预制 DuckDuckGo V2 搜索工具注册为模型可调用的网页搜索能力；Agents 页同步提供中英双语开关，IDE、资料库和自动化 Agent 默认开启，互动叙事 Agent 默认关闭但可手动启用。
- 新增一级菜单 `Skills`，支持查看内置、用户级 `<nova_dir>/skills` 和工作区级 `<workspace>/.nova/skills` 的 `SKILL.md`，可在界面中新建/编辑用户自定义 Skill；内置 `skills-creator` Skill 可通过创作 Agent 辅助创建，支持在创作 Agent 及其他启用 Skills 的 Agent 中用 `/<skill-name>` 命令触发。

## [v0.1.7] - 2026-06-10

### Added

- README 新增中英语言切换入口，并补充英文版 `README.en.md`。
- 新增 `lore-init` 资料库初始化 Skill：资料库为空时引导作者先讨论题材、角色、核心冲突、世界规则、创作风格、禁忌和互动开局，用户确认后再写入资料库与 `CREATOR.md`。
- IDE 写作主页面和互动剧情主页面在资料库为空时提供轻量引导；IDE 会打开创作 Agent 并预填新书构思 prompt，互动模式继续跳转资料库 Agent 并预填初始化指令。
- Agents 页新增每个 Agent 的自定义 system prompt 配置，支持用户级/工作区级分层继承；运行时按「Nova 运行时契约（不可覆盖）→ 用户自定义提示 → Nova 内置提示」拼装，确保自定义提示能覆盖行为偏好但不能覆盖工具权限、输出协议、互动禁写、结构化 JSON 和后端校验边界。
- 内置叙事编排新增 `直白情色` 和 `编剧风格` 两个预设，分别面向成人自愿情欲张力和编剧式场景节拍。
- WebUI 新增 i18n 多语言基础设施，接入 `i18next` / `react-i18next`，首版提供简体中文与 English 资源，并为后续语言扩展预留统一 locale 目录。
- 设置页新增“界面语言”配置，支持跟随浏览器、简体中文和 English；语言配置进入现有分层设置体系，保存后可热切换。
- 后端 API 支持 `X-Nova-Locale` 请求头，workspace、books、settings、versions、session、chat、interactive、lore、style 和角色卡导入等短错误/成功提示会按中英文返回。
- 设置页新增全局外观字号配置，支持分别设置界面字号与阅读字号；阅读字号统一作用于 IDE 主编辑器和互动模式故事阅读区。
- 书籍管理新增 txt/md 现有小说导入：上传后自动解析章节、创建新书并写入 `chapters/`；导入后回到 IDE 主页，由已有空资料库引导跳转资料库 Agent 生成设定资料。
- 小说导入升级为确认式智能分割流程：上传后工具 Agent 基于前 `20000` 字样本推断章节标题 Go regexp，用户可调整 `2000-100000` 字样本范围、编辑正则并重新预览，确认后再创建书籍和写入章节；工具 Agent 默认无工具且关闭 thinking，可在 Agents 页配置模型和 system prompt。
- 小说导入预览新增流式进度：前端会展示文件读取、章节解析、工具 Agent 正则识别、回退和预览完成等阶段，避免长时间智能识别时界面无反馈。
- 新增一级菜单“自动化 / Automations”，作为 Books、Agents 同级共享工作台页面；点击只打开自动化页面，不自动切换 IDE/互动模式，并保持一级菜单单 active。
- 新增 Automations 后端服务与 REST API，支持用户级任务和当前工作区任务的 JSON 存储、CRUD、手动运行、最近运行记录、结构化定时规则、调度器加载和 panic recover。
- 新增 Automation Agent kind，接入 `agent_models`、`agent_tools`、`agent_prompts` 分层配置，并在 Agents 页展示；默认允许文件/资料库读写和 Skills，命令执行默认关闭，写文件/写资料库仍必须同时满足任务写入权限和 Agent 工具权限。
- 自动化任务支持记忆整合、Review、续写章节和自定义 Prompt 四类模板；不再要求用户配置上下文来源，Agent 会按任务目标自行使用允许的工具读取所需章节、设定、资料库和状态。

### Changed

- 书籍管理不再以“最近书籍”记录作为列表来源，改为展示当前 Nova 数据目录下实际存在的书籍目录，并将前端列表优化为书架式网格布局；旧最近打开记录仅保留用于启动恢复当前书籍。
- 章节和分卷默认命名改为隐藏排序前缀模板：章节使用 `ch{order:05}-{chapter}-{title}.md`，分卷目录使用 `v{order:05}-{volume}`，作品目录隐藏前缀展示自然章节名；该变更只影响新章节和新导入内容，旧章节不会自动重命名。
- 版本管理底层从原生文件快照切换为 go-git 驱动的 workspace 根目录 `.git` 本地仓库；Nova 会自动初始化并提交版本，像 Git 一样保存正文、设置和 `.nova/lore`、`.nova/sessions` 等本地创作状态，历史直接来自 Git commit，恢复通过移动 HEAD 生效，不再创建 `.nova/versions` 索引、内部版本目录或裁剪 Git 历史；旧原生快照不再读取或迁移。
- 顶层定调文件改为 `ideas.md`（作品目录展示为「灵感」/ Ideas）；新建作品会创建该文件，旧工作区仅存在 `brainstorm.md` 时会在初始化时迁移为 `ideas.md`，并同步更新 Agent 提示词、技能、前端初始化文案和 README。
- 整体优化中英文 README：重写项目首屏定位、核心价值、能力矩阵、推荐创作流程、快速开始、配置和开发说明，提升公开项目页的专业度与可读性。
- 资料库 Agent 从单次结构化 JSON 编辑方案升级为工具型 Agent，支持 Skills、资料库读写和文件读写工具；初始化流程要求多轮确认，最终只写资料库和 `CREATOR.md`，不写 `ideas.md`、大纲、章节、progress、character-states，也不自动创建互动 story。
- 资料库条目简介改为多行编辑，并统一要求 `brief_description` 使用“类型 名称 + 3-5 句触发说明 + 必须参考详情”的索引结构，提升 Agent 自动匹配并读取资料正文的准确性。
- 指令类多行输入框改为随输入内容自动扩展，最多显示 10 行后进入内部滚动，覆盖创作 Agent、资料库 Agent、叙事编排 Agent、互动剧情输入和短表单简介。
- 用户可见“讲述者 / Teller / 导演 / Director”统一改名为“叙事编排 / Narrative Direction”，IDE 和互动模式内的紧凑选择提示使用“叙事 / Narrative”；内部 `Teller`、`story_teller_id`、`story-tellers/` 和 API 路径保持兼容不迁移。
- 强化内置叙事编排规则内容，旧版内置 JSON 会随 `tellerVersion` 自动刷新，规则会更明确影响剧情裁定、角色主动性、代价、节奏、伏笔和状态沉淀。
- 设置页 General Appearance 调整到顶部，语言选项固定展示为 `Follow Browser`、`简体中文` 和 `English`，并支持设置页与 Agents 页修改后自动保存。
- 创作 Agent 的新书构思前置流程现在会同时读取 `ideas.md` 和 `CREATOR.md`，并在初始化沟通中把阶段性结论、待确认点和取舍理由持续整理到 `ideas.md`；`ideas.md` 不再是一次性归档文件，而是后续生成大纲或重大方向调整时优先参考的有界指引文件。
- IDE 作品目录中的章节组细纲默认只展示最新一组，历史章节组可折叠展开；章节组生成规则同步收紧为短小可维护，方便作者阅读、评论和后续更新。
- 扩大 WebUI i18n 覆盖面，补齐会话管理、工具卡片、Agent 配置、互动故事舞台、分支路线、场景记忆、字体设置和编辑区浮层等模块内的硬编码界面文案。
- WebUI 字号改为按层级从界面字号派生，默认保持 `text-xs`、`text-sm`、`text-[11px]` 和 `text-[10px]` 原有视觉大小，并覆盖创作 Agent 输出、用户消息、菜单、侧栏和子模块小字。
- 资料库 Agent 和叙事编排 Agent 的消息展示复用创作 Agent 的通用消息列表与工具卡片样式，统一 thinking、工具调用和历史消息呈现。
- 新建资料库条目的默认 ID 改为基于条目名的可读格式，如 `林川_ab12`；后端继续校验显式 ID 重复并阻止写入。
- Agent 资料库读取工具从 `search_lore_items` 收敛为 `list_lore_items` + `read_lore_items`：先返回全量轻量索引，再按 ID 读取完整正文。

### Fixed

- 修复创作 Agent 和互动模式流式输出完成并刷新为持久化历史后，Markdown 段落、列表和行距重新排版导致会话区域抖动的问题。
- 修复作品目录树和章节摘要对中文自然章节名排序不准确的问题，`序章`、`第一章`、`第十章`、`第十一章`、`第一百一十一章` 等会按实际章序排列。
- 修复资料库 Agent 和叙事编排 Agent 复用通用消息列表后，长历史消息撑开整个页面滚动的问题；消息历史改为在 Agent 内部区域滚动。
- 修复 GitHub Release 打包脚本在系统缺少 `zip` 命令时无法生成 Windows 压缩包的问题；现在会回退使用 `python3 -m zipfile`。
- 小说导入智能章节识别失败时增加后端排查日志，记录工具 Agent 调用、模型输出摘要、正则命中数量和回退原因，方便定位为何回退内置规则。
- 小说导入工具 Agent 正则识别超时时间从 25 秒提升到 90 秒，降低大样本或慢模型导致 `context deadline exceeded` 后直接回退内置规则的概率。
- 小说导入工具 Agent 在 JSON mode 返回空内容或解析失败时，会自动降级为普通文本模式重试一次，兼容 OpenAI 协议平台对 `response_format=json_object` 支持不稳定的情况。
- 小说导入章节分割优先使用本地规则识别常见标题，新增对 `序章`、`楔子`、`尾声`、`番外`、`卷一`、`一卷`、`上卷` 等序章/卷标题的内置支持，减少简单 txt 依赖工具 Agent 后回退的问题。
- 小说导入预览新增“AI 识别”入口，可在本地规则已命中时强制跳过预置正则并重新调用工具 Agent 推断章节标题正则。
- 小说导入工具 Agent 正则识别的输出上限提升到 `8192` tokens，并在解析失败时记录有界原始返回内容、reasoning 内容和提取后的 JSON 内容，便于排查输出截断或非 JSON 响应。
- 小说导入支持识别分卷边界：`第一卷`、`卷一`、`Part I`、`Volume 1` 等标题会作为分卷目录，后续章节写入带隐藏排序前缀的 `chapters/v00001-<分卷名>/`，预览中同步展示章节所属分卷。
- txt 小说导入写入 `.md` 章节时会把原文非空单行转换为 Markdown 段落，避免源文件没有空行时 Markdown 渲染把换行折叠成一行。
- txt 小说导入会清理行首 ASCII 缩进并转义 `#`、`>`、列表符号和代码围栏等 Markdown 块语法，避免普通小说正文被渲染成代码块、标题、引用或列表。
- 小说导入按阅读顺序生成 `ch00001-序章.md`、`ch00002-第一章-缘起.md` 等稳定文件名；新工作区会同步写入 `chapter_filename_format` 和 `volume_dir_format`，目录汇总兼容 `ch0001`、数字编号、中文章回和英文 Chapter 等旧格式。
- 默认章节文件名模板改为隐藏排序前缀格式 `ch{order:05}-{chapter}-{title}.md`，Agent 提示词中的章节路径示例同步改为 `chapters/v00001-第一卷/ch00002-第一章-废材开局.md`。

## [v0.1.6] - 2026-06-05

### Changed

- 后端 HTTP 层按职责拆分：将具体 handler 迁移到 `internal/api/handlers`，将任务 SSE 输出迁移到 `internal/api/sse`，`internal/api` 保留服务启动、路由注册和静态资源托管职责。
- 后端应用运行时构建逻辑从 `internal/app/runtime_manager.go` 拆到 `internal/app/runtime_builder.go`，降低 workspace manager 文件职责密度。
- 版本管理从本地 Git 仓库替换为 Nova 原生快照系统，版本库存放在每本书的 `.nova/versions/`，无需初始化 Git 即可创建版本、查看历史、对比和恢复。
- 内部重构版本管理实现：后端快照逻辑拆分到 `internal/book/versions`，前端版本面板拆分为状态头、自动策略、变更列表、历史容器和工具函数，降低版本管理模块耦合。
- WebUI 版本管理面板改为全中文快照工作流，第一屏展示保护状态、手动保存、定时保存和 Agent 自动保存状态，并在历史中标注手动、定时、Agent 与回滚前备份版本。
- 版本管理手动保存支持由 LLM 根据当前文件变更自动推理中文版本说明，前端不再要求用户手动填写说明；模型失败时会降级为本地变更摘要。
- 设置页 Agent 模型分配支持按 Agent 单独配置思考开关和 OpenAI `reasoning_effort`；快捷选项 Agent 和版本说明 Agent 默认关闭思考，其他 Agent 未配置时不向模型请求传递相关参数。
- WebUI 报错提示调整为贴近 IDE 面板风格的紧凑卡片，统一版本管理和设置页错误展示。
- 右下角 Toast 弹窗关闭 Sonner 默认高饱和错误色，改为使用 Nova IDE 面板变量和低干扰边框样式，并将关闭按钮改为右侧常显的小图标。
- 设置页新增工作区级版本管理配置，支持定时自动保存、Agent 大量输出自动保存、Agent 字数阈值和自动版本保留数量。
- 创作 Agent 新增用户可见的 `setting/character-states.md` 角色状态层，章节定稿后主要同步 `progress.md` 与角色当前状态；资料库改为只承载角色身份、人设、长期关系、能力体系和世界规则等稳定设定，避免每章状态抖动频繁写入资料库。
- 创作 Agent 调整 `write_lore_items` 批量写资料库工具语义，用于在大纲定稿或长期设定变化时一次性创建/更新多个资料条目，并在 WebUI 自动刷新资料库索引；写入条目缺少简介时会按资料类型、名称、标签和正文自动生成 `brief_description`。
- `scripts/npm-release.sh` 发布到 npm registry 时默认使用 `--auth-type web`，可通过浏览器完成 npm 2FA/认证流程；提供 `--auth-type` 参数并保留 `--otp` 覆盖方式。
- 整理 `ideas.md` 规划记录，补充“续写下一章没自动分卷”待修复项并移除空的 NEED FIX 段落。

### Fixed

- 互动模式：修复状态变化解析白名单遗漏 `action_space`，导致包含可行动选项的状态更新整组被丢弃的问题。
- 创作 Agent：修复“按细纲写下一章”未按大纲分卷的问题，系统提示会结合大纲卷章安排、章节组细纲、进度和最近章节路径选择 `chapters/<分卷名>/` 目标目录，并在快捷创作提示中同步强调分卷写入。
- Windows Release：修复默认 8080 端口被占用时双击启动后服务监听失败并退出的问题；未显式指定端口时会自动顺延选择可用端口，并保留 `NOVA_BACKEND_PORT` / `--port` 的显式配置语义。

## [v0.1.5] - 2026-06-02

### Added

- 新增 npm 分发包骨架，提供 `nova` CLI 入口和跨平台预编译二进制打包脚本，支持通过 npm/npx 一键安装运行。
- 新增 `scripts/npm-release.sh`，串联 npm 发布目录构建、包内容预览、本地 tgz 生成和 registry 发布流程，并默认以 dry run 防止误发布。
- 新增 GitHub Actions Release 流水线和 `scripts/build-github-release.sh`，推送 `v*` tag 后自动构建 macOS/Linux/Windows 下载包、生成 checksums 并上传 GitHub Release。
- 后端/设置页支持多个 OpenAI 协议兼容模型配置，可为 IDE 创作、互动叙事、资料库编辑、讲述者编辑、互动状态和快捷选项等 Agent 分配不同模型与 Temperature；未配置 Temperature 时不再写死默认值，交由平台/模型默认策略处理。
- 互动模式新增按需快捷行动建议生成接口，故事舞台可继续生成更多选择，并在设置页支持关闭“输入框快捷选择”。
- 互动模式故事舞台支持像 IDE 模式一样通过 `#` 引用用户级 `<nova_dir>/styles/` 下的风格参考，本轮会随互动 Agent 请求注入。
- 互动模式支持复用场景化风格规则；每个具体讲述者编辑页可分别维护场景风格规则和互动单轮目标字数。
- 讲述者编辑支持自动保存，修改名称、规则、场景风格规则等内容后会防抖写入当前讲述者。
- IDE 模式新增左侧全局搜索：可在当前书籍 workspace 内搜索 Markdown/TXT 等文本文件内容和路径，结果按文件分组展示，点击后打开文件并联动编辑器高亮关键词。
- 互动模式故事舞台支持编辑历史输入并从该回合重新生成，也可直接对指定回合重新生成内容，当前分支会回退到被编辑回合前继续推进。
- 互动模式分支路线支持直接切换故事线，每条故事线展示各自独立的分支路线。
- 互动模式故事舞台支持展示并持久化 Agent 工具调用卡片，刷新后保留卡片状态但不保存工具输入输出参数。
- 风格参考文件移动到用户级 `<nova_dir>/styles/`，不同书籍可复用同一批 `.md` / `.txt` 文风样本。
- IDE 模式新增章节组细纲工作流：新建书籍会准备 `setting/chapter-groups/`，Agent 可生成下一组细纲，快捷创作增加“下一组细纲 / 按细纲写下一章 / 定稿并同步状态”入口。
- IDE 模式作品目录支持以轻量导航列表展示大纲、细纲，并按章节目录自动分卷折叠；项目文件支持多选批量移动、复制、删除和拖拽整理。
- 设置页新增章节创作配置，支持章节组建议规模范围，默认建议 3-8 章。

### Changed

- 生产态 Web 静态资源托管支持 `NOVA_WEB_DIR` 和可执行文件相对路径探测，npm 包安装后不再依赖启动时的当前工作目录；npm CLI 未显式配置 `NOVA_DIR` 时默认使用执行命令目录下的 `./.nova`，`NOVA_BACKEND_PORT` 也会作为后端默认端口生效。
- Agent 资料库读取工具从单条 `read_lore_item` 升级为批量 `read_lore_items`，可一次按多个资料 ID 读取完整正文，减少连续工具调用。
- 资料库支持渐进式加载：条目新增常驻、简介自动匹配和手动引用三种加载策略；IDE/互动 Agent 会常驻注入核心资料、展示含简介的非常驻资料索引，并可通过只读工具按需读取资料正文。
- IDE 创作提示词改为以结构化资料库承载角色、世界观、地点、势力、规则和物品等长期设定，不再引导读写 `setting/characters.md` 或 `setting/world-building.md`；作品状态注入也停止回退读取这两个旧文件。
- 后端 Agent 构建接入 `max_iteration` 与 `model_max_retries` 运行时设置，不再使用构建时硬编码值。
- 互动故事 Agent 不再随正文输出内联快捷选择，也不再对缺失选择做兜底生成；快捷选择改为用户点击“选择”时由独立 LLM 调用按当前上下文生成。
- 互动模式快捷行动建议生成后会按当前剧情节点持久化到故事 JSONL，刷新后优先复用已生成结果；状态 Agent 不再维护可选择入口。
- 互动模式快捷行动建议不再自动展示，改为输入区显式按钮触发，面板可手动收起并保留生成结果。
- 互动模式底部输入区改为更紧凑的高度和独立行高，减少对故事阅读空间的占用。
- 设置页不再展示场景化风格规则和互动单轮目标字数，这两项集成到每个具体讲述者编辑页，并保存到对应讲述者 JSON。
- 手动保存讲述者时不再重新跳回第一个讲述者，会保持当前讲述者和当前规则选中状态。
- 章节文件名默认模板调整为 `ch{NNNN}-{title}.md`，创作 Agent 会读取配置中的章节文件名模板，文件树按章节数字排序以支持千章作品。
- 更新 README，按当前书籍管理、小说 IDE、创作 Agent、互动工作台、资料库、角色卡导入和版本管理能力重写使用指南，并将新增界面截图改为可折叠展示。
- 讲述者规则配置页优化交互：规则启用开关移到左侧规则列表，注入位置改为紧凑下拉选择，减少详情区占用并提升操作效率。
- 创作 Agent 工具卡片统一为暗色面板风格，优化执行中、结果、详情和待办列表的边距、状态图标与展开区域质感。
- Agent 写作工作流调整为“创作灵感 -> 大纲 -> 下一组细纲 -> 章节初稿/成章”，细纲只规划接下来一组章节，章节定稿后才同步 progress 与角色状态。
- Agent 注入场景化风格规则前会把相对风格名解析为用户级 `<nova_dir>/styles/` 下的绝对路径，IDE 和互动模式都按当前讲述者选择规则。
- IDE 模式适配结构化资料库和讲述者：写作工作台新增资料库/讲述者入口，创作 Agent 支持引用资料条目，并会按工作区默认讲述者注入写作规则。
- IDE 模式下资料库和讲述者入口改为覆盖项目目录、编辑区和右侧面板的全工作区管理页。
- WebUI 导航调整：IDE/互动模式切换移到顶部 Nova 标识旁的分段切换，左侧一级菜单按当前模式切换；设置页改为覆盖工作区页面，不再使用弹窗。
- WebUI 细化工作台层级：书籍管理会返回打开前的 IDE/互动模式，版本管理改为全工作区页面，互动模式的场景记忆开关移入剧情页右侧按钮。
- 讲述者 Agent 不再强制只能修改当前选中的讲述者，可根据用户本轮意图新建讲述者、自由选择已有讲述者，或通过输入框 `@` 引用讲述者来限定修改对象。
- 互动故事舞台的下一步行动候选改为在底部输入框聚焦时柔和展开，减少浏览历史时的界面跳动。
- 酒馆角色卡导入入口并入书籍管理，左侧活动栏不再保留独立上传图标。

### Fixed

- WebUI：修复 IDE 写作页打开 AI 右侧栏时，切到资料库/讲述者/版本管理等全工作区页面再返回写作会丢失右侧栏开合状态的问题。
- 后端：互动快捷选择模型输出解析失败时会记录原始模型输出，便于定位 JSON 格式问题。
- WebUI：修复互动故事消息切换到最早版本后因版本索引为 0 被省略，导致版本切换按钮消失、无法切回后续版本的问题。
- 后端设置保存：修复首次没有本地配置文件时，在界面保存 API Key 后当前运行时仍使用旧空配置，导致新建配置无法立即连上模型的问题；保存用户/工作区配置后会同步刷新运行时模型配置。
- WebUI：修复切换书籍后互动工作台资料库、资料库版本、资料库 Agent 历史和相邻设置面板状态仍显示旧书数据的问题，workspace 变化时会先清空旧状态再重新拉取当前书籍数据。
- 角色卡导入：修复批量创建世界书资料时资料 ID 基于时间戳生成可能碰撞，导致导入失败并提示 `资料 ID 已存在: world-*` 的问题。

## [v0.1.4] - 2026-05-29

### Added

- 互动故事工作台新增默认故事线、下一步行动候选、可中断生成、对白高亮和可配置的单轮字数/Token 上限，让互动写作从开局到推进更顺。
- 互动模式新增场景记忆、可行动空间、物品资源、世界规则和未解决线索展示，并用剧情分支图呈现故事线继承关系。
- 资料库升级为结构化 Lore Item 系统，支持角色、世界观、地点、势力、规则和物品等条目管理。
- 新增资料库 Agent，可通过中文指令批量整理资料，支持流式过程、`@` 引用条目、会话持久化、手动版本和历史恢复。
- 支持导入 SillyTavern 酒馆 v2 PNG/JSON 角色卡，可导入当前书籍或用角色卡创建新书。
- 新增故事讲述者配置页和讲述者 Agent，可通过自然语言创建或修改讲述者规则。
- 写作工作台新增作品统计接口和章节概览，显示章节数、全书字数、章节状态和更新时间。

### Fixed

- 文件删除支持 macOS、Linux 和 Windows 回收站，不再只依赖 macOS。
- 书籍管理在纯 Web 形态下收敛为 Nova 数据目录内创建和切换书籍，避免浏览器尝试访问任意本机目录。
- 互动故事的流式输出、分支切换、页面切换和刷新恢复更稳定，生成中的正文和思考过程不会轻易丢失。
- 场景记忆同步、剧情分支图、节点创建和长篇 JSONL 读取更加可靠。
- 全局快捷键不再抢占输入框、弹窗和富文本编辑器的原生文本操作。
- 创作者指令和作品状态在每轮对话前重新读取，修改 `CREATOR.md` 后下一轮即可生效。
- 作品统计接口对空章节列表做了兼容，避免编辑区 Tab 标题异常。

### Changed

- 工作台视觉和导航收敛为更紧凑的双层侧栏结构，写作、互动、书籍管理、角色卡导入和设置入口更清晰。
- 互动模式将资料库、创作者指令、讲述者、剧情舞台、场景记忆和分支路线重新组织为更稳定的工作流。
- 分支路线改为左侧导航中的主区视图，支持横向浏览、节点选中、剧情线切换和从节点创建新剧情线。
- 互动故事生成改为正文生成与状态整理分阶段处理，正文先流式落盘，场景记忆随后同步。
- 书籍管理和设置改为全局弹窗，IDE 与互动模式下都能打开。
- 编辑器与互动故事舞台新增字体、字号和行高配置，长文阅读体验更可控。
- 代码结构按领域拆分后端应用层和前端工作台主入口，降低后续维护成本。

## [v0.1.3] - 2026-05-24

### Fixed

- WebUI 编辑区 Tab：修复 Tab 列表出现重复 React key 的报错（`Encountered two children with the same key, file:skills/test/SKILL.md`）——`handleRenameItem` / `handleMoveItem` 通过 `map` 把 `from → to` 时若 `to` 已在打开列表中会产生重复条目，`readTabsFor` 兼容旧版字符串与新版对象持久化时也可能出现同 key 多份；提取 `dedupeTabs` 工具函数并在 `enforceTabLimit`、`readTabsFor`、rename/move 三个出口统一去重
- WebUI 目录树：修复在空目录（如初始 `skills/` 子目录）右键「新建文件 / 新建目录」时内联输入框不出现的问题——空目录被后端 JSON `omitempty` 序列化后 `children` 为 `undefined`，前端 `expanded && node.children &&` 短路掉了承载输入框的子层 `FileTreeList`，改为展开时始终渲染（缺省视为空数组）

### Changed

- 后端 `internal/prompts`：新增独立 prompts 包，集中管理后端所有写死的长段提示词（系统指令 / 计划模式 / 上下文边界 / 异常中断恢复 / 场景化风格规则 / 引用·选区文案 / 未知工具反馈 / `brainstorm.md` 与 `CREATOR.md` 模板）。`internal/agent` 与 `internal/book` 改为从 `internal/prompts` 读取，agent 仅保留 IO/上下文拼装薄壳；移除 `agent/prompt.go` 内联指令大字符串与 `book/state.go` `book/creator.go` 的模板常量，提示词文案变更不再需要改动业务包
- 后端 `book` / `app`：重构自动 Commit 触发时机——由「写章节前在 `safeToolMiddleware` 中创建快照」改为「每次新对话 `App.StartTask` 入口自动 commit」；新增 `book.GitService.AutoCommit(ctx, threshold)`，仅当工作区脏且累计 add+del 行数（含 untracked 文件整文件行数）≥ 阈值时才执行 `add -A` + `commit`，默认阈值 `book.DefaultAutoCommitLineThreshold = 50`，未达阈值/工作区干净/仓库未初始化均跳过；自动 commit 失败不阻断对话，仅写日志
- 后端 `agent`：移除 `safeToolMiddleware` 中的 `shouldSnapshotBeforeChapterWrite` / `autoCommitBeforeChapterWrite` 路径及对 `internal/book` 的耦合，中间件回归纯错误兜底；`prompt.go` 与 `skills/continue/SKILL.md` 中关于「写章节前自动 Git 快照」的说明同步删除

### Added

- 后端 `session` / `agent`：新增异常中断恢复标识持久化；Runner/流式读取异常或 Agent panic 时记录待恢复中断，用户后续明确输入“继续/继续刚才/从中断的地方继续”等请求时，会从上一轮异常中断上下文续跑，成功完成后标记该中断已恢复；前端/SSE 断线但后端任务仍运行时仍沿用现有 active task 重连，不写入异常标识
- 后端 `interactive`：讲述者 JSON 新增 `reply_target_chars` 和 `style_rules`，场景化风格规则按当前讲述者独立生效。
- 后端 `agent`：当用户本轮未通过 `#` 指定风格参考时，由 IDE 默认讲述者或互动故事当前讲述者注入 `ChatRequest.StyleRules`，`ChatService` 追加「场景化默认风格规则 + 触发规则」提示。
- WebUI：具体讲述者编辑页新增「单轮目标字数」和「场景风格规则」编辑能力，支持新增/删除规则、选择用户级风格文件和手动添加 `.md` / `.txt` 路径。

- 后端 `config`：新增 `Settings.MaxOpenTabs`（默认 5），通过用户/工作区分层覆盖；JSON/TOML 字段为 `max_open_tabs`
- WebUI：编辑区 Tab 数量上限化，超过 `max_open_tabs` 时按 LRU（最久未激活优先）自动关闭旧 Tab，当前激活 Tab 永远受保护；workspace 切换恢复时也会按上限裁剪
- WebUI：设置页「编辑器」分组新增「最大同时打开 Tab 数」配置项；设置保存后通过 `nova:settings-updated` 事件触发主界面立即重新拉取生效配置
- 后端 `book`：工作区初始化 `InitWorkspace` 在缺失时自动写入 `brainstorm.md` 顶层定调模板（题材、核心卖点、目标读者、整体风格、金手指、故事尺度、剧情走向、参考作品等），引导作者在生成大纲前先完成顶层设定讨论；新增 `BrainstormFileName`、`BrainstormPath()` 与 `CreatorFileName`，CREATOR.md 模板生成时机一并迁移到 `InitWorkspace`
- 后端 `agent`：在系统提示中加入 `brainstorm.md` 路径说明与「生成大纲时」前置工作流——先与作者讨论补全 `brainstorm.md` 顶层定调，作者确认定稿后才生成 setting/outline.md / characters.md / world-building.md / progress.md；空作品的状态文案改为引导作者优先填写 `brainstorm.md`
- 后端 `agent`：在每轮 Agent 输入前注入「上下文边界」提示，明确「当前请求 = 这次做什么 / 已确认小说状态 = 背景是什么 / 历史对话只能辅助理解」，要求 Agent 在新请求与历史无关或冲突时只依据本轮请求、@ 引用、# 风格参考和编辑器选区行动，避免跨对话的上一轮工具意图被误执行；新增 `appendContextBoundaryInstruction` 纯函数及对应单测
- 后端 `app`：当启动时既未指定 `--workspace` 又无最近书籍记录时，App 进入「无 workspace」状态，仅初始化 `chatService` / `bookRegistry` / `bookMetaStore`，等待用户在前端书籍管理页选择或新建书籍后再构建 runtime；新增 `App.HasWorkspace()` 与 `ErrNoWorkspace` 用于守卫
- 后端 API：新增 `Server.requireWorkspace` 守卫；写操作（`/api/workspace/*` 写、`/api/chat`、`/api/git/*`、`/api/command` 中的 clear/status、`/api/sessions` 的 create/switch/rename/delete）在无 workspace 时返回 409 并提示「尚未选择书籍工作区」；只读拉取（`tree`、`styles`、`sessions`、`session messages`）在无 workspace 时返回空数组，避免前端启动报错
- WebUI：`workspace` 为空时 `App.tsx` 默认打开「书籍管理」Tab 并激活，引导用户选书
- 后端 `config`：引入 `Settings` + `LoadLayered`，合并语义为 默认 < 全局 (`config.toml`) < 用户 (`<nova_dir>/config.toml`) < 工作区 (`<workspace>/.nova/config.toml`) < 环境变量；指针类型字段（`*bool`/`*int`）用于区分「未设置」与「显式置零」
- 后端 API：新增 `GET /api/settings`（返回三层快照 + effective）、`PUT /api/settings/user`、`PUT /api/settings/workspace`
- WebUI：编辑区支持多 Tab，文件树打开文件时复用已存在的 Tab 或新建 Tab；Hover Tab 显示关闭按钮，关闭当前 Tab 自动切到相邻 Tab；Tab 列表与激活项按 workspace 分桶持久化到 localStorage，刷新后恢复
- WebUI：Tab 不仅承载文件，也承载「书籍管理」（Home）页面；Activity Bar 主页按钮改为打开/聚焦 Home Tab，可与文件 Tab 自由切换
- WebUI：Agentic Loop `write_todos` 工具卡片渲染为可读的待办列表，支持 pending/in_progress/completed 三态、显示进度（completed/total），并对流式不完整 JSON 容错

### Changed

- 设置配置：`nova_dir` 改为全局启动级参数，仅由全局 `config.toml` 或 `NOVA_DIR` 决定；用户级/工作区级配置会忽略并过滤该字段，设置页改为只读展示 Nova 数据目录、用户配置文件和工作区配置文件路径
- WebUI：删除/重命名/移动文件时同步更新打开的 Tab 列表
- WebUI：主区域统一由 Tab 栏驱动渲染，根据激活 Tab 切换显示编辑器或 Home 视图，移除原 `view` 单一视图状态

### Removed

- 后端 API/命令：移除 `/init` 命令（CREATOR.md 与 `brainstorm.md` 模板改由 `InitWorkspace` 在工作区创建时自动生成），`/help` 输出同步去除该项
- WebUI：聊天输入区命令菜单移除 `/init`，`useChat` 命令分发列表同步删除
- WebUI：移除顶部工作区栏的「切换」按钮（功能不实用），切换工作区改由「书籍」Popover 底部「添加/打开其他书籍目录...」入口完成
- WebUI：移除编辑区 Tab 栏右侧未接线的左右翻页占位图标

### Fixed

- 后端 Agent：当 LLM 幻觉调用不存在的工具（如 `write_todo`）时，不再以 `NodeRunError` 中断任务；通过配置 `ToolsNodeConfig.UnknownToolsHandler` 把可读错误作为 ToolMessage 回喂给模型，引导 Agent 自我分析并改用正确工具名继续执行

### Added

- 后端测试：新增 `TestHandleUnknownTool`，覆盖未知工具调用时的回退提示生成

## [v0.1.2] - 2026-05-18

### Added

- 后端 API：新增会话列表、创建、切换、重命名、删除接口，并支持按 `session_id` 读取会话历史
- 后端测试：覆盖多会话隔离、clear 标记、有效上下文读取、旧会话文件兼容和 App 会话切换/删除
- WebUI：创作Agent 面板新增会话列表、创建、切换、重命名和删除入口
- 测试：新增后端会话 API CRUD/切换/消息读取测试，以及前端会话切换和 `/clear` 分界展示测试
- WebUI：新增 React Query、Zustand、Resizable Panels、Monaco Diff、Sonner Toast 和工作台快捷键基础设施
- WebUI：新增章节 Diff View、版本时间线、版本 Diff 弹窗和回滚确认弹窗 UI 骨架
- 测试：新增 ChapterDiffView、RollbackDialog 和 Workspace Store 前端单测
- 测试：新增命令面板、书籍 Popover、编辑器设置 Popover 的前端测试

### Changed

- WebUI：底部状态栏版本号改为读取前端包版本
- 后端会话：支持 workspace 内多会话管理、最近激活会话恢复和 `/clear` 上下文清理标记
- 后端 Agent：构建上下文时只读取当前激活会话最后一个 clear 标记之后的有效消息
- WebUI：执行 `/clear` 后保留旧消息并展示“上下文已清理”分界，切换会话时同步刷新消息和活跃任务状态
- WebUI：将会话切换控件移动到创作Agent 标题栏，避免占用对话内容区域
- WebUI：会话切换控件改为下拉列表选择，替代横向滚动会话标签
- WebUI：工作区布局改为 `react-resizable-panels` 管理，右侧/底部面板状态迁移到 Zustand
- WebUI：版本管理面板改为 React Query 管理 Git 状态和历史查询，并用 shadcn AlertDialog 替代原生回滚确认
- WebUI：命令面板改为 shadcn `CommandDialog`，书籍列表与编辑器设置浮层改为 Radix `Popover`
- WebUI：图标按钮统一接入 Tooltip，部分滚动区域接入 `ScrollArea`

### Fixed

- WebUI：修复 Tooltip 提示背景对比不足导致按钮悬浮提示看不清的问题

## [v0.1.1] - 2026-05-17

### Added

- WebUI：基于 React + Vite + TypeScript + Tailwind CSS + TipTap 构建小说 IDE 前端
- 后端服务：基于 Hertz 提供 REST API 与 SSE 流式聊天接口
- 工作区 API：支持目录树、文件读取、文件保存、当前 workspace 查询和 workspace 切换
- 三栏写作界面：左侧项目结构、中间 TipTap 章节编辑器、右侧 AI 输出
- 编辑器设置：支持字号、行间距、背景主题调整，并持久化到 localStorage
- 自动保存：编辑停止后自动保存章节内容，同时保留 Ctrl/Cmd+S 手动保存
- CREATOR.md：支持 workspace 根目录自定义最高优先级创作者指令
- bootstrap.sh：开发环境一键启动前后端并输出前端 localhost 地址
- WebUI 布局：项目结构、AI 输出、任务面板支持拖拽调整大小和显示/隐藏，并持久化用户偏好
- 编辑区：基于 TipTap 官方 Markdown 扩展渲染和保存 Markdown 内容
- 项目结构：支持目录树自动刷新和窗口聚焦刷新，及时展示 AI 写入的新文件
- 风格参考：新增 `setting/styles/` 目录，支持在 AI 对话中通过 `#` 选择本轮风格参考
- 项目结构：目录树同级节点按目录优先、文件其次排序展示
- AI 对话区：Agent 输出改为无气泡正文流样式，仅用户输入保留右侧气泡
- AI 对话区：实时思考内容默认自动下滑，用户上滑阅读时暂停跟随
- 编辑区：基于 TipTap 字数统计扩展展示当前文件总字数和选中文字数
- AI 对话区：支持中断正在执行的 Agent，并保留中断前已生成内容
- 书籍管理：记录最近打开的 workspace，后端重启后自动恢复上次书籍，并支持基础书籍列表/移除记录
- AI 对话区：打开面板时消息列表直接定位到底部，避免先显示顶部再跳转
- 编辑区：支持 Cmd/Ctrl+F 在当前文章内搜索关键词，并高亮匹配结果
- AI 对话区：Agent 写入或创建文件后自动刷新目录结构，并同步刷新当前打开文本
- 版本管理：底部面板新增受限 Git 命令行，支持本地 init/status/add/commit/diff/history/reset --soft/--mixed
- 版本管理：受限 Git 命令行支持使用分号串联白名单命令，例如 `git add -A; git commit -m "说明"`
- 版本管理：新增按钮式初始化、创建版本、查看历史和整本书回滚能力
- 版本管理：新增右侧 Source Control 风格面板，支持通过活动栏图标 toggle
- 版本管理：新增暂存当前未提交内容和恢复最近暂存内容能力
- 风格参考：支持在 `setting/styles/` 中维护 `.txt` 文风样本，并通过 `#` 引用注入 Agent
- 后端 Agent：新增任务、SSE、Runner、工具调用和 panic recover 运行日志，便于排查输出中断与工具失败

### Changed

- 入口程序从 bubbletea TUI 改为启动 Hertz Web 服务
- build.sh 增加前端构建流程，并复制 Web 产物到 output/web
- 会话存储迁移到 workspace 内部 `.nova/sessions/`
- 作品设定文件迁移到用户可编辑的 `setting/` 目录
- 编辑器默认视觉调整为贴合 IDE 的深色阅读主题
- 后端能力拆分为 `internal/agent`、`internal/book`、`internal/api`、`internal/app`，明确 AI Agent、书籍管理、HTTP API 和运行时装配边界
- Chat 执行不再使用固定 ADK checkpoint，用户本轮引用的大段文件和风格参考只作为当轮上下文注入
- Agent 创建章节文件时遵循 `chXX-章节名.md` 命名规范，便于目录整体浏览
- bootstrap.sh 启动开发服务时不再自动打开浏览器
- AI 对话区工具输出改为单张结构化卡片，聚合工具名、参数摘要、执行状态和结果展开查看
- AI 对话区工具卡片改为单行状态展示，调用开始即显示，并按 tool id 更新乱序完成的结果
- 版本管理：底部面板从命令行输入改为按钮式操作，减少误操作风险
- 版本管理：从底部任务面板迁移到右侧面板，并优化变更列表、提交历史和操作结果展示
- AI 对话区：流式输出阶段改为纯文本渲染，结束后按历史消息渲染 Markdown，降低长输出崩溃风险
- AI 对话区：流式输出改为统一时间线展示思考内容、工具卡片和正文，并在流式阶段节流渲染 Markdown
- AI 对话区：合并流式文本增量和自动滚动更新，提升长回复输出流畅度
- AI 对话区：当前思考过程在流式阶段默认展开，思考结束后自动折叠
- 后端 Agent：强化章节重写规则，重写时以创作者要求和前后章节衔接为准，避免被旧状态摘要约束
- 后端 Agent：强化续写规则，续写需衔接前面至少两章且不改大纲，仅更新进度和角色状态
- 后端 Agent：明确 outline、progress、characters 职责边界，写作推进主要更新进度和角色状态，避免状态文件职责混写
- AI 对话区：流式 Markdown 改为轻量即时渲染，减少长回复输出卡顿
- AI 对话区：将后端大段 chunk 拆成逐帧小片段输出，让文字呈现更接近常规 LLM 流式吐字
- 前端运行时：记录 React 崩溃、全局 JS 异常、Promise 未处理异常和白屏原因，便于排查前端故障
- 后端 Agent：补充 Chat 上下文拼装和流式工具调用合并单测，防止引用、风格参考和选中文本注入逻辑回归
- 前端测试：引入 Vitest、React Testing Library 和 MSW，补充 API 与 Chat 消息组件测试
- 后端 Agent：写入 `chapters/` 前自动提交原工作区 Git 快照，快照失败时阻止覆盖章节正文

### Fixed

- 修复创作 Agent 流式输出阶段退化为纯文本导致 Markdown 标题、表格等不渲染的问题
- 修复打开版本管理面板时，后端返回空变更列表为 `null` 导致前端崩溃的问题
- 版本管理：保存文件、Agent 写入、文件树操作、窗口聚焦和 workspace 切换后自动刷新 Git 状态
- 修复 Agent 输出异常中断或前端断流时已生成内容可能被清空的问题
- 修复流式 Recv 异常后仍可能继续发送 `done` 状态的问题
- 修复流式 thinking、重复 tool_call 和重复正文片段被拆成多张卡片导致对话展示混乱的问题
- 修复前端因初始化恢复对话 effect 依赖变化而反复请求 `/api/chat/active` 和 `/api/session/messages` 的问题
- 切换 workspace 时同步重建 Agent Runner，避免 Agent 指令和作品状态继续指向旧 workspace
- 修复右侧 AI 输出对 SSE `tool_result` / `error` 字段解析错误，并实时展示思考内容和工具执行状态
- 修复编辑区自动保存会移除 Markdown 空行，导致段落换行渲染异常的问题
- 修复编辑区 Markdown 单换行不展示的问题，兼容逐行小说文本和风格参考文件
- 修复编辑区自动保存后重置 TipTap 内容导致光标跳动的问题

### Removed

- 移除 bubbletea TUI 相关实现与依赖
