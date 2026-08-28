# Changelog

Denova 仅在此记录用户可感知的重大功能、重要不兼容或数据变更、安全更新，以及影响核心流程的关键修复。内部重构、测试调整、文案修改和细节级 UI 优化不再逐项列出，完整记录可查看 [Git history](https://github.com/alfredxw/denova/commits/master)。

Denova records only major user-visible features, important compatibility or data changes, security updates, and fixes affecting core workflows. Internal refactors, test changes, copy edits, and minor UI polish are omitted; see the [Git history](https://github.com/alfredxw/denova/commits/master) for full details.

`Unreleased` 以最近一个已发布版本（当前为 v0.3.3）为比较基线，只描述升级用户最终可感知的净变化；内部接口、实现重构和 v0.3.3 后从未发布的中间格式不计入。

`Unreleased` compares against the latest release (currently v0.3.3) and describes only the final user-visible delta. Internal APIs, implementation refactors, and intermediate formats never released after v0.3.3 are excluded.

格式参考 [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)。

## [Unreleased]

### Major changes / 重大变更

- 字体设置改为类似 Dynamic Type 的统一离散档位：界面字号按完整层级缩放，阅读字号同步覆盖写作、游戏剧情、Agent 回复、源码编辑器、Diff 与终端。
- Font settings now use unified Dynamic Type-style steps: interface text scales as one hierarchy, while reading size also covers Writing, Game story, Agent replies, source editors, diffs, and terminals.
- 项目、Skill、版本 Diff 与面包屑文件树统一升级，采用一致的默认密度，并支持完整文件图标、空目录折叠、Git 状态、名称搜索与上下文菜单；项目文件树同时支持拖放整理。
- Project, Skill, version Diff, and breadcrumb file trees now share a consistent default density, complete file icons, flattened empty directories, Git status, name search, and context menus; Project files also support drag-and-drop organization.
- 版本管理改为可调整的历史侧栏与常驻 Diff 工作区，支持搜索和加载更早版本、按版本浏览文件，并直接恢复版本或单个文件。
- Version Management now uses a resizable history sidebar and persistent Diff workspace, with history search and loading, per-version file browsing, and direct version or file restore.
- 新增通用 Agent 与基于稳定 Project ID 的工作台；书籍和任意本地目录都可并行打开对话、Files、终端、阅读器与资料库。v0.3.3 Project 数据会保留源文件并自动复制迁移。
- Added a General Agent and a stable Project-ID-based workbench. Books and arbitrary local directories can run chats and open Files, terminals, Reader, and Lore in parallel. v0.3.3 Project data is copy-migrated automatically while retaining the source files.
- 写作页创作 Agent 支持同一本书的多个会话并行运行，并新增可隐藏的快捷会话栏；切换会话不会中断后台任务。
- The Writing Agent now runs multiple conversations in the same Book concurrently, with an optional quick-session rail; switching conversations no longer stops background work.
- 写作、游戏、工作台与配置对话支持运行中连续追加 Follow Up，刷新或重启后恢复任务，并按会话保存模型、思考强度与权限设置。
- Writing, Game, Workspace, and configuration conversations now accept queued follow-ups during a run, recover after refresh or restart, and persist model, thinking, and permission settings per conversation.
- 写作、游戏与通用 Agent 对话支持从现有输入菜单、拖拽或粘贴添加多个通用文件；Denova 保存独立副本供 Agent 使用，并将图片作为原生视觉输入发送给模型。
- Writing, Game, and General Agent chats now accept multiple generic files from the existing input menu, drag-and-drop, or paste. Denova stores independent copies for the Agent and sends images as native vision input.
- Agent 回复现在会在生成过程中实时呈现 Markdown、数学公式与中日韩文本排版，并以平滑动画展示新增内容。
- Agent replies now render Markdown, math, and CJK typography during generation, with smooth animation for newly streamed content.
- 新增 Ask、Write、Full access 安全模式；Project 外部文件读取按模式授权，写入与 Shell 工作目录仍受 Project 边界保护。
- Added Ask, Write, and Full access safety modes. Reads outside a Project are approval-aware, while writes and shell working directories remain Project-bound.
- 新增会话级 Goal、JavaScript `script` 编排和 Project `AGENTS.md` 指令，并将现有 `CREATOR.md` 与多 Skill 按需协作统一到所有 Project Agent，支持更长时间的自主执行。
- Added session Goals, JavaScript `script` orchestration, and Project `AGENTS.md` instructions, while applying the existing `CREATOR.md` and on-demand multi-Skill coordination consistently across all Project Agents for longer autonomous runs.
- 自动化改为 Project 任务，并作为普通 Writing 或 General Agent 会话执行；任务可选择每次新建会话或固定复用会话，运行历史直接在 AgentChat 中查看。
- Automations are now Project tasks executed as ordinary Writing or General Agent conversations. Each task can create a new conversation per run or reuse one conversation, with run history opening directly in AgentChat.
- 语言模型接入拆分为服务商预设与协议适配器，支持 OpenAI Chat Completions、Responses、Anthropic Messages、自定义兼容端点、模型发现和连接测试；思考设置统一为 `thinking_level`，模型输出上限改由模型与协议能力决定。
- Split language-model setup into provider presets and protocol adapters, with OpenAI Chat Completions, Responses, Anthropic Messages, custom-compatible endpoints, model discovery, and connection tests. Thinking settings now use `thinking_level`, while model and protocol capabilities determine output limits.
- 语言与图像模型配置改为“连接 + 模型”两层结构：同一 Endpoint 与 API Key 可复用于多个模型，语言模型支持从服务端列表批量添加，已完成配置默认折叠；v0.3.3 配置会在备份后自动迁移。
- Language and image model settings now separate reusable connections from model profiles. One endpoint and API key can serve multiple models, language models support batch addition from provider discovery, completed items collapse by default, and v0.3.3 settings migrate automatically after backup.
- 图像生成支持 OpenAI Images、xAI/Grok、火山方舟 Seedream、Google Gemini Image 与 ComfyUI Workflow；ComfyUI 可发现已保存工作流并自动关联参数，自定义端点可复用协议，多图任务会保留部分成功结果。v0.3.3 图像配置会自动迁入 Profile，并在改写前备份。
- Image generation now supports OpenAI Images, xAI/Grok, Volcengine Ark Seedream, Google Gemini Image, and ComfyUI Workflow. ComfyUI can discover saved workflows and bind their parameters automatically; custom endpoints can reuse protocols, and multi-image jobs preserve partial successes. v0.3.3 image settings migrate into Profiles automatically and are backed up before rewrite.
- 工作台与创作界面进一步统一：项目页签收敛为写作、Files、资料库，支持可持久化双栏工作区；资料编辑器提供 Rich/Raw 模式、本地图片上传与评论，界面和阅读字体支持本机字体。书籍 Lore 与 Skills 迁至用户可见目录，v0.3.3 原文件保留为备份。
- Unified the workbench and creation surfaces around Writing, Files, and Lore tabs with a persistent split workspace. Lore adds Rich/Raw editing, local image uploads, and comments, while UI and reading settings support local fonts. Book Lore and Skills move to user-visible directories, with the v0.3.3 source files retained as backups.
- 写作页按文件类型选择编辑器：Markdown 可在文档编辑与 Monaco 源码编辑间切换，图片使用预览，JSON、JSONL 等其他文本使用 Monaco，并共享自动保存与冲突保护。
- Writing now selects editors by file type: Markdown can switch between the document editor and Monaco source editing, images use preview, and other text such as JSON and JSONL uses Monaco with the same autosave and conflict protection.
- 工作台改为统一且可自定义的一级导航：写作与游戏成为并列入口，菜单支持调整顺序和显隐；资料库、方案预设和版本管理不再按创作入口重复。方案预设统一展示全部类型，并标明通用、游戏专用或写作专用。
- The workbench now uses unified, customizable top-level navigation: Writing and Game are peer destinations, menus can be reordered or hidden, and Lore, Presets, and Versions are no longer duplicated by creative context. Presets show every type with fixed Shared, Game-only, or Writing-only labels.
- 游戏剧情支持从任意已保存 AI 回复就地创建分支，并在导演控制台集中预览、切换和管理故事线。
- Game stories can branch directly from any persisted AI reply, with branch preview, switching, and management consolidated in the Director Console.
- Developer Mode 新增跨 Project 的 Trajectory 与 Harness 工作区，用于查看运行轨迹、导出诊断数据和优化 Agent 行为；Harness 改动先保存为草稿，可按目标 Agent 调试、整体发布，并可随时停用自定义 State 而不删除数据。
- Developer Mode adds cross-Project Trajectory and Harness workspaces for run inspection, diagnostic export, and Agent optimization. Harness changes are saved as a draft, can be debugged per target Agent, published together, and disabled without deleting custom State data.
- 终端启动方式改为用户级可排序注册表，并新增 macOS/Linux 一键安装器和 Windows PowerShell 快速重启脚本。
- Terminal launchers now use a user-level ordered registry, with a macOS/Linux installer and a native Windows PowerShell quick-restart script.

### Incompatible data changes / 用户数据不兼容变更

- v0.3.3 的用户级全局 Automation 任务文件会保留，但不再展示、触发或执行；需要继续使用的任务须在对应 Project 下重新创建。
- User-level global Automation task files from v0.3.3 are retained but are no longer displayed, triggered, or executed. Tasks that remain needed must be recreated under the relevant Project.
- v0.3.3 中的 `enable_thinking`、`reasoning_effort`、`max_output_tokens`、`tool_result_retention_enabled` 和低层 Cleanup 参数不再生效；升级后需重新选择 `thinking_level` 与当前上下文选项，输出上限则改由模型能力决定。
- The v0.3.3 `enable_thinking`, `reasoning_effort`, `max_output_tokens`, `tool_result_retention_enabled`, and low-level Cleanup settings no longer take effect. Select `thinking_level` and the current context options again after upgrading; model capabilities now determine output limits.
- v0.3.3 的 `labs.continual_learning` 开关不再生效；已有 Harness State 与历史仍会保留，需要启用 Developer Mode 才会加载。
- The v0.3.3 `labs.continual_learning` switch no longer takes effect. Existing Harness State and history remain intact but load only when Developer Mode is enabled.

### Major fixes / 重要修复

- 修复跨轮、工具调用边界或重启后丢失推理上下文的问题；写作与游戏现在会安全保留并复用完整的 provider continuation。
- Fixed reasoning context being lost across turns, tool-call boundaries, or restarts; Writing and Game now safely retain and reuse the complete provider continuation.
- 修复 Provider Token 校准可能下调本地保守估算、导致长会话过晚清理上下文的问题；真实用量现在只会向上修正上下文压力。
- Fixed provider token calibration lowering conservative local estimates and delaying context cleanup in long sessions; observed usage now only raises projected pressure.
- 修复部分 v0.3.3 游戏故事升级后因旧上下文事件而无法打开的问题；迁移会先完整备份原始 JSONL，再原子重连有效历史。
- Fixed some v0.3.3 Game stories failing to open after upgrade because of obsolete context events. Migration now backs up the original JSONL before atomically reconnecting valid history.
- 修复写作、游戏和工作台长任务在刷新、断线重连、中断或继续执行后可能丢失或重复消息、工具结果和已提交内容的问题。
- Fixed Writing, Game, and Workspace runs losing or duplicating messages, tool results, or committed content after refresh, reconnect, interruption, or continuation.
- 修复游戏回合重生成失败后再次重试会丢失原回合定位、把原提示误提交为新回合的问题。
- Fixed retrying a failed Game turn regeneration losing its target and submitting the original prompt as a new turn.
- 修复写作与资料编辑中的自动保存、外部修改合并、评论消费和版本恢复问题，避免草稿被覆盖或已提交反馈重复出现。
- Fixed autosave, external-edit merging, comment consumption, and version restoration in Writing and Lore, preventing draft overwrites and submitted feedback from reappearing.
- 修复游戏新故事空回合崩溃、历史回合跳转错位、回合数错误及刷新后重复回放已结算剧情的问题。
- Fixed empty-turn crashes in new Game stories, incorrect historical-turn navigation and counts, and replay of already settled story content after refresh.
- 修复内置终端退出、重连、主题和 PTY 生命周期问题，交互式 CLI 退出后可可靠返回原工作目录的 Shell。
- Fixed embedded-terminal exit, reconnect, theme, and PTY lifecycle issues so interactive tools reliably return to the shell in the original working directory.
- 修复 DeepSeek、MiniMax、Claude 等模型及游戏回合的输出上限被错误压低的问题，已知模型按其能力使用正确上限。
- Fixed incorrectly low output limits for DeepSeek, MiniMax, Claude, and Game turns; known models now use limits derived from their capabilities.
- 修复 DeepSeek 思考模式在工具结果投影后继续运行或启动 SubAgent 时可能因未回传 `reasoning_content` 而失败的问题。
- Fixed DeepSeek thinking-mode runs failing after tool-result projection or SubAgent startup because `reasoning_content` was not replayed.
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
