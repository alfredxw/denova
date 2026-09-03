# Changelog

Denova 仅在此记录用户可感知的重大功能、重要不兼容或数据变更、安全更新，以及影响核心流程的关键修复。内部重构、测试调整、文案修改和细节级 UI 优化不再逐项列出，完整记录可查看 [Git history](https://github.com/alfredxw/denova/commits/master)。

Denova records only major user-visible features, important compatibility or data changes, security updates, and fixes affecting core workflows. Internal refactors, test changes, copy edits, and minor UI polish are omitted; see the [Git history](https://github.com/alfredxw/denova/commits/master) for full details.

`Unreleased` 以最近一个已发布版本（当前为 v0.3.3）为比较基线，只描述升级用户最终可感知的净变化；内部接口、实现重构和 v0.3.3 后从未发布的中间格式不计入。

`Unreleased` compares against the latest release (currently v0.3.3) and describes only the final user-visible delta. Internal APIs, implementation refactors, and intermediate formats never released after v0.3.3 are excluded.

格式参考 [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)。

## [Unreleased]

### Brief / 简要说明

#### 中文

- 升级提示：v0.3.3 的游戏组合预设、独立 Config Manager、用户级全局 Automation 及部分旧模型与上下文设置不再参与运行；原数据会保留或先备份，可迁移的数据会自动迁移，全局 Automation 任务需在对应 Project 重新创建。
- `.denova` 数据目录可在 Windows、WSL、Linux 与 macOS 之间整体移动；统一 Project 工作台支持通用、写作与游戏会话并行运行，并共享 Files、资料库、终端和版本历史。
- Agent 新增自定义主 Agent、Goal、脚本编排、Project 指令、安全模式及更可靠的多 Agent 协作；语言与图像模型改为可复用连接和模型配置，并扩展常用协议与图像服务商支持。
- 游戏新增规划模板、单页开局、故事控制台和任意已保存回复分支；写作、游戏与资料编辑同时增强了任务恢复、自动保存、版本恢复和长会话可靠性。
- 导航、文件树、编辑器、版本 Diff 与字体缩放统一到同一套创作工作台，支持可持久化双栏布局、本机字体和更完整的文件操作。

#### English

- Upgrade note: v0.3.3 Game composition presets, standalone Config Manager, user-level global Automations, and several legacy model or context settings no longer participate in runtime behavior. Existing data is retained or backed up before supported migrations, and global Automations must be recreated in their Projects.
- The complete `.denova` data directory can move across Windows, WSL, Linux, and macOS. The unified Project workbench runs General, Writing, and Game conversations concurrently while sharing Files, Lore, terminals, and version history.
- Agents gain custom main Agents, Goals, script orchestration, Project instructions, safety modes, and more reliable multi-Agent work. Language and image models now use reusable connections and model profiles with broader protocol and image-provider support.
- Game adds Planning Templates, a single-page start flow, the Story Console, and branching from any saved reply. Writing, Game, and Lore also gain stronger task recovery, autosave, version restoration, and long-session reliability.
- Navigation, file trees, editors, version diffs, and typography now share one creation workbench with persistent split layouts, local fonts, and more complete file operations.

### Major changes / 重大变更

- `.denova` 数据目录现在可在 Denova 完全退出后整体移动或复制到 Windows、WSL、Linux 与 macOS 的任意合法可写位置；受管 Project 的会话、游戏、版本、附件、工具产物和自动化会按稳定 Project ID 与相对路径继续使用，v0.3.3 索引在首次切换前自动备份并迁移。
- After Denova fully exits, the complete `.denova` data directory can be moved or copied to any valid writable location on Windows, WSL, Linux, or macOS. Managed Project sessions, Game state, versions, attachments, tool artifacts, and automations continue through stable Project IDs and relative paths, with v0.3.3 indexes backed up before the first migration.
- 新增通用 Agent 与基于稳定 Project ID 的统一工作台；书籍和任意本地目录都可并行运行通用或写作会话，并共享 Files、资料库、终端、阅读器和版本历史。会话支持运行中追加 Follow Up、刷新或重启后恢复、按会话保存模型与权限，以及通过选择、拖拽或粘贴添加通用文件和原生视觉输入。
- Added a General Agent and a unified stable Project-ID workbench where Books and arbitrary local directories can run General or Writing conversations concurrently while sharing Files, Lore, terminals, Reader, and version history. Conversations support queued follow-ups, refresh or restart recovery, per-conversation model and permission settings, generic file attachments, and native image input.
- 配置与自动化统一进入 Project Agent 会话：通用和写作 Agent 通过 `/configuration` Skill 管理配置，各页面共享会话历史与恢复；Automation 作为普通 Project 会话运行，写作和配置页面可分别维护快捷指令组。
- Configuration and automation now use Project Agent conversations. General and Writing Agents manage configuration through `/configuration` with shared history and recovery, Automations run as ordinary Project conversations, and Writing or configuration pages can maintain separate quick-prompt groups.
- Agents 页面支持独立自定义主 Agent、会话 Goal、JavaScript `script` 编排、Project `AGENTS.md`、稀疏 Skill、上下文与压缩策略、委派策略以及 Ask、Write、Full access 安全模式；主 Agent 默认仍自行处理任务，仅在用户或 Skill 明确要求时并行委派，等待可被新指令安全打断。
- The Agents page supports independent custom main Agents, session Goals, JavaScript `script` orchestration, Project `AGENTS.md`, sparse Skills, context and compaction policies, delegation policies, and Ask, Write, or Full access safety modes. Main Agents still delegate only when requested by the user or a Skill, with parallel work and interruptible waits.
- 语言模型接入拆分为服务商预设与协议适配器，支持 OpenAI Chat Completions、Responses、Anthropic Messages、自定义兼容端点、模型发现和连接测试；思考设置统一为 `thinking_level`，模型输出上限改由模型与协议能力决定。
- Split language-model setup into provider presets and protocol adapters, with OpenAI Chat Completions, Responses, Anthropic Messages, custom-compatible endpoints, model discovery, and connection tests. Thinking settings now use `thinking_level`, while model and protocol capabilities determine output limits.
- 语言与图像模型配置改为可复用的“连接 + 模型”结构；语言模型支持服务端发现和批量添加，图像生成支持 OpenAI Images、xAI/Grok、火山方舟 Seedream、Google Gemini Image 与 ComfyUI Workflow，多图任务会保留部分成功结果，v0.3.3 配置会在备份后迁移。
- Language and image settings now use reusable connections plus model profiles. Language models support provider discovery and batch addition, while image generation supports OpenAI Images, xAI/Grok, Volcengine Ark Seedream, Google Gemini Image, and ComfyUI Workflow with partial success preserved for multi-image jobs. v0.3.3 settings migrate after backup.
- 工作台统一为可排序、可隐藏的一级导航与可持久化双栏布局；文件树、面包屑、Skill 与版本 Diff 共享搜索、Git 状态、文件图标和上下文操作，版本管理支持历史搜索、按版本浏览及恢复整个版本或单个文件，界面和阅读字体按统一档位缩放并支持本机字体。
- The workbench now uses reorderable top-level navigation and persistent split layouts. File, breadcrumb, Skill, and version trees share search, Git status, icons, and context actions; Version Management supports history search, per-version browsing, and full-version or single-file restore; UI and reading typography scale through unified steps with local-font support.
- 写作、Files 与资料库按文件类型提供文档、Monaco、图片预览及 Rich/Raw 编辑，并共享自动保存与冲突保护；Book Lore、Skills 与用户方案迁至用户可见目录或 `.denova/presets`，v0.3.3 原文件在迁移时保留。
- Writing, Files, and Lore now select document, Monaco, image preview, or Rich/Raw editors by file type with shared autosave and conflict protection. Book Lore, Skills, and user presets move into visible directories or `.denova/presets`, while v0.3.3 source files are retained during migration.
- 游戏新增五种内置规划方式和自定义规划模板、单页开局、集中式故事控制台，以及从任意已保存 AI 回复创建和管理分支；故事直接保存叙事、事件、检定、状态与图像选择，不再继承组合预设。
- Game adds five built-in planning approaches and custom Planning Templates, a single-page start flow, a consolidated Story Console, and branch creation from any saved AI reply. Stories now store narrative, event, check, state, and image selections directly instead of inheriting a composition preset.
- Agent 配置迁入受管 Agents Project 并复用普通 Project 的会话、Files 与版本历史；General Agent 可按需读取跨 Project 的只读 Trajectory 证据维护 Profile，Developer Mode 继续提供 Trajectory 查看与诊断导出。
- Agent configuration now lives in a managed Agents Project with ordinary Project conversations, Files, and version history. The General Agent can read cross-Project, read-only Trajectory evidence on demand when maintaining Profiles, while Developer Mode retains Trajectory inspection and diagnostic export.
- 终端启动方式改为用户级可排序注册表，并新增 macOS/Linux 一键安装器和 Windows PowerShell 快速重启脚本。
- Terminal launchers now use a user-level ordered registry, with a macOS/Linux installer and a native Windows PowerShell quick-restart script.

### Incompatible data changes / 用户数据不兼容变更

- v0.3.3 的游戏预设不再作为运行时组合配置。故事首次打开时会先备份原故事日志和引用的旧预设，再保留当时实际生效的叙事、事件、检定、状态、图像及规则展示配置，并切换到默认规划模板；旧预设中的自定义规划 Markdown 不自动转换，原内容保留在迁移备份中。
- v0.3.3 Game Presets no longer act as runtime composition settings. When a story is first opened, Denova backs up its journal and referenced legacy preset, preserves the effective narrative, event, check, state, image, and rule-visibility selections on the story, and selects the default Planning Template. Custom planning Markdown is not converted automatically and remains available in the migration backup.
- v0.3.3 的独立 Config Manager 会话、专属设置和用户级全局 Automation 任务文件会保留，但不再展示、触发或参与运行；后续配置对话使用对应 Project 的统一会话列表，需要继续使用的 Automation 须在对应 Project 下重新创建。
- Standalone v0.3.3 Config Manager sessions, dedicated settings, and user-level global Automation files are retained but no longer displayed, triggered, or used at runtime. Future configuration conversations use the Project's shared conversation list, and Automations that remain needed must be recreated in that Project.
- v0.3.3 的旧思考、输出上限、工具结果保留、低层 Cleanup 与 `labs.continual_learning` 设置不再生效；升级后需重新选择当前选项。有效用户级 Agent 设置会在备份后迁入 `.denova/agents` Profile，Project 覆盖继续保留。
- Legacy v0.3.3 thinking, output-limit, tool-result retention, low-level Cleanup, and `labs.continual_learning` settings no longer take effect and must be reselected where applicable. Active user-level Agent settings are backed up before migration into `.denova/agents` Profiles, while Project overrides remain intact.

### Major fixes / 重要修复

- 修复数据目录绝对路径参与 Project 与会话身份，以及部分 v0.3.3 游戏故事含旧上下文事件的问题；搬迁或升级后不再出现历史消失、重复 Project 或故事无法打开，迁移会先备份原始数据再原子更新。
- Fixed absolute data-directory paths participating in Project and session identity, along with obsolete context events in some v0.3.3 Game stories. Relocation or upgrade no longer causes missing history, duplicate Projects, or stories that fail to open, and migrations back up source data before atomic updates.
- 修复跨轮、工具调用、刷新、重连、中断、继续或重启后推理上下文、消息、工具结果和已提交内容丢失或重复的问题；主动中断现在进入可恢复暂停，Token 校准不会再下调保守上下文估算。
- Fixed reasoning context, messages, tool results, or committed content being lost or duplicated across turns, tool calls, refresh, reconnect, interruption, continuation, or restart. User interruption now creates a resumable pause, and token calibration no longer lowers conservative context estimates.
- 修复写作与资料编辑中的自动保存、外部修改合并、评论消费和版本恢复问题，避免草稿被覆盖或已提交反馈重复出现。
- Fixed autosave, external-edit merging, comment consumption, and version restoration in Writing and Lore, preventing draft overwrites and submitted feedback from reappearing.
- 修复游戏检定重复应用故事修正、重生成重试丢失原回合、生成中调校丢弃正文，以及空回合崩溃、历史跳转和刷新后重复回放等问题；规则模板修正由后端解析，大成功与大失败仅由自然 20/1 触发。
- Fixed Game checks applying story tuning twice, regeneration retries losing their target, tuning during generation discarding prose, empty-turn crashes, incorrect history navigation, and settled content replaying after refresh. Rule-template modifiers are resolved by the backend, and critical success or failure requires a natural 20 or 1.
- 修复内置终端退出、重连、主题和 PTY 生命周期问题，交互式 CLI 退出后可可靠返回原工作目录的 Shell。
- Fixed embedded-terminal exit, reconnect, theme, and PTY lifecycle issues so interactive tools reliably return to the shell in the original working directory.
- 修复 DeepSeek、MiniMax、Claude 等模型及游戏回合输出上限过低，以及 DeepSeek 思考模式在工具结果投影或启动 SubAgent 后未回传 `reasoning_content` 而失败的问题。
- Fixed incorrectly low output limits for DeepSeek, MiniMax, Claude, and Game turns, plus DeepSeek thinking-mode failures when `reasoning_content` was not replayed after tool-result projection or SubAgent startup.
- 升级 Go、`go-git` 与 `x/image` 安全基线，修复标准库、Git 路径/符号链接和 WebP 解码相关漏洞。
- Upgraded Go, `go-git`, and `x/image` security baselines to address standard-library, Git path/symlink, and WebP decoding vulnerabilities.

## [v0.3.3] - 2026-07-25

- 修复写作编辑器与对话框切换时输入法被重置的问题。
- Fixed IME resets when switching between the Writing editor and chat.
- 每个 Markdown 文件使用独立撤销历史，跨标签页撤销不再串改内容。
- Each Markdown file now has an isolated undo history, preventing cross-tab edits during undo.

## [v0.3.2] - 2026-07-23

- Agent 对话按 Run 聚合为可折叠的执行过程；工作台支持跨 Project、多会话真正并行运行。
- Agent conversations are grouped into collapsible run histories, while Workspace supports true concurrent conversations across Projects.
- Skills 新增分类与能力声明，工具参数支持安全规范化，网页搜索支持可替换 Provider 与抓取回退。
- Skills gained categories and capability declarations, tool arguments gained safe normalization, and web access gained replaceable search providers and fetch fallbacks.
- 编辑器自动保存和 Git 自动版本改为修改后延迟执行，并用 revision/CAS 与三方合并保护并发编辑。
- Editor autosave and Git versions now run after edit idle periods, with revision/CAS checks and three-way merging for concurrent changes.
- 游戏历史、模型上下文和 UI 展示拆分为独立投影，并强化最新回合编辑、重生成与恢复的一致性。
- Game history, model context, and UI rendering now use separate projections with safer latest-turn editing, regeneration, and recovery.

## [v0.3.1] - 2026-07-23

- Agent Trace 支持复制 Run ID 和导出完整 JSONL；前端独立启动可显式指定后端端口。
- Agent Trace can copy Run IDs and export complete JSONL traces; standalone frontend startup can target an explicit backend port.
- 修复端口冲突、Windows 设置自动保存和游戏正文候选重复等关键问题。
- Fixed important port-conflict, Windows settings-autosave, and duplicate Game narrative issues.

## [v0.3.0] - 2026-07-22

- 写作模式新增持久化 Change Review、正文评论、跨重启 Undo/Redo、正则替换和带备份的全局替换。
- Writing added durable Change Review, document comments, restart-safe undo/redo, regex replacement, and backed-up workspace-wide replacement.
- 游戏模式新增导演运行策略、故事级状态结构、Actor 归档/恢复、状态布局、回复修正和全屏导演台。
- Game added Director policies, story-scoped state schemas, Actor archive/restore, state layouts, response correction, and a full-screen Director Desk.
- 自动保存、三方合并、工作区变更账本、崩溃恢复与活动任务重连统一为更可靠的数据保护链路。
- Unified autosave, three-way merging, workspace change journals, crash recovery, and active-task reconnection into a safer data pipeline.
- 新增上下文检查、完整 Trace、更平滑的流式展示和 Unicode 规范化安全升级。
- Added context inspection, complete traces, smoother streaming, and a Unicode normalization security update.

## [v0.2.0] - 2026-07-15

- 写作工作台与 AI 互动叙事统一到长期创作架构，明确 Story Director、Actor State、事件、TRPG、资料与历史边界。
- Unified the Writing workbench and interactive narrative around clear Story Director, Actor State, event, TRPG, Lore, and history boundaries.
- Agent/Subagent 新增可配置 Skills、上下文压缩、工具结果策略、运行计划、自动化和本地 Trace。
- Agent and Subagent workflows gained configurable Skills, context compaction, tool-result policies, plans, automation, and local traces.
- 游戏当前状态改为摘要优先的自适应布局，并支持安全恢复无效的旧预设覆盖。
- Game Current State moved to an adaptive summary-first layout with safe recovery for invalid legacy preset overrides.
- 资料库新增按需加载、批量读取、revision 保护和 Tavern 角色卡/世界书导入。
- Lore gained on-demand loading, batch reads, revision protection, and Tavern character-card/world-book imports.

## [v0.1.18] - 2026-07-01

- 完成 Denova 品牌与分发命名切换，Release 包只提供 `denova` / `denova.exe`。
- Completed the Denova branding and distribution rename; release packages now use only `denova` / `denova.exe`.
- 新增新用户引导、消息中心、PWA/移动端主屏和前端内嵌自托管能力。
- Added onboarding, a message center, PWA/mobile home-screen support, and embedded-web self-hosting.
- 完善移动端写作与游戏的输入、弹窗、文件操作、故事记忆与分支导航。
- Expanded mobile Writing and Game input, dialogs, file actions, story memory, and branch navigation.
- 改进书籍封面、互动图像、Plan Mode 和资源保存冲突保护。
- Improved book covers, interactive images, Plan Mode, and resource-save conflict protection.

## [v0.1.17] - 2026-06-27

- 新增章节插图与互动图像生成工作流，并完善模型设置和故事开局交互。
- Added chapter-illustration and interactive-image workflows, with improved model settings and story-opening controls.

## [v0.1.16] - 2026-06-27

- 新增图像生成配置和默认模型别名支持。
- Added image-generation settings and default model aliases.

## [v0.1.15] - 2026-06-27

- 完善 Writing Agent 配置、运行控制和应用内更新体验。
- Improved Writing Agent configuration, run controls, and in-app updates.

## [v0.1.14] - 2026-06-26

- 新增写作 Skill 预设、配置工具和 Subagent 会话详情，并强化 Agent 启动与更新稳定性。
- Added Writing Skill presets, configuration tools, and Subagent session details, while improving Agent startup and updater reliability.

## [v0.1.13] - 2026-06-24

- 改进 Agent 上下文缓存、配置资源管理和移动端适配，并隔离配置管理会话。
- Improved Agent context caching, configuration resources, mobile layouts, and Config Manager session isolation.

## [v0.1.12] - 2026-06-20

- 重构互动故事记忆与状态 Schema，并补充上下文控制和失败重试。
- Reworked interactive-story memory and state schemas with stronger context controls and retry behavior.

## [v0.1.11] - 2026-06-18

- 新增故事开局、资料启用状态、Tavern 卡导入与记忆召回。
- Added story openings, Lore enablement, Tavern-card import, and memory recall.
- 新增自动化触发、消息收件箱、GitHub Release 更新器和深浅主题。
- Added automation triggers, an inbox, a GitHub Release updater, and light/dark themes.

## [v0.1.10] - 2026-06-12

- 改进工作区删除与恢复安全性。
- Improved workspace deletion and recovery safety.

## [v0.1.9] - 2026-06-12

- 修复空工作区启动、Skills 与互动叙事回归，完成核心架构稳定性清理。
- Fixed empty-workspace startup, Skills, and interactive-story regressions, with core architecture stabilization.

## [v0.1.8] - 2026-06-11

- 新增网页搜索和自定义 Skills。
- Added web search and custom Skills.

## [v0.1.7] - 2026-06-10

- 新增现有小说导入、自动化、工作区版本历史、中英文本地化和全局字号设置。
- Added existing-novel import, automation, workspace version history, Chinese/English localization, and global font sizing.

## [v0.1.6] - 2026-06-05

- 新增角色状态跟踪，增强资料管理，并将版本能力收敛为本地快照。
- Added character-state tracking, improved Lore management, and consolidated versioning around local snapshots.

## [v0.1.5] - 2026-06-02

- 资料库迁移为结构化数据并支持渐进加载与批量操作；资料、导演和写作工作区升级为完整 IDE 页面。
- Migrated Lore to structured storage with progressive loading and batch operations, and promoted Lore, Director, and Writing surfaces to full IDE pages.
- 互动故事新增回合版本切换、行动候选和按 Agent 配置的模型 Profile。
- Interactive stories gained turn versioning, action choices, and per-Agent model Profiles.

## [v0.1.4] - 2026-05-29

- 建立可玩的互动故事工作台，支持流式回合、状态、分支路线、行动候选和可中断生成。
- Introduced the playable interactive-story workbench with streaming turns, state, branch routes, action choices, and interruptible generation.
- 资料库升级为结构化 Lore Item，并新增资料 Agent、导演配置和 Tavern v2 角色卡导入。
- Upgraded Lore to structured items and added a Lore Agent, Director configuration, and Tavern v2 character-card import.
- 写作工作台新增作品统计，并统一写作与游戏的紧凑导航、字体和布局设置。
- Added manuscript statistics and unified compact navigation, typography, and layout controls across Writing and Game.

## [v0.1.3] - 2026-05-24

- 工作区新增多 Tab、分层设置、无工作区引导和 `brainstorm.md` 创作流程。
- Added multi-tab workspaces, layered settings, empty-workspace onboarding, and the `brainstorm.md` creation flow.
- Agent 新增异常中断恢复、上下文边界和场景化风格规则。
- Added Agent interruption recovery, context boundaries, and scene-specific style rules.

## [v0.1.2] - 2026-05-18

- 新增多会话管理、`/clear` 上下文分界、可调工作区布局和章节版本 Diff/回滚界面。
- Added multi-session management, `/clear` context boundaries, resizable workspace layouts, and chapter version diff/rollback views.

## [v0.1.1] - 2026-05-17

- 发布基于 React、TipTap 与 Go/Hertz 的首个 Web 小说 IDE，包含文件树、Markdown 编辑、自动保存和三栏工作区。
- Released the first web novel IDE based on React, TipTap, and Go/Hertz, with a file tree, Markdown editing, autosave, and a three-pane workspace.
- 创作 Agent 支持 SSE 流式输出、思考与工具时间线、中断、风格引用和 `CREATOR.md` 指令。
- The Writing Agent supports SSE streaming, reasoning/tool timelines, interruption, style references, and `CREATOR.md` instructions.
- 新增本地版本管理、书籍切换和基础会话持久化，并移除旧 TUI。
- Added local version management, book switching, and basic session persistence, and removed the former TUI.
