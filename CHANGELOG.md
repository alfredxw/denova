# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- 互动模式故事舞台支持编辑历史输入并从该回合重新生成，也可直接对指定回合重新生成内容，当前分支会回退到被编辑回合前继续推进。
- 设置页的场景化风格规则扩展到用户配置层，用户配置会保存具体风格文件路径，并允许手动添加 `.md` / `.txt` 风格文件路径。
- IDE 模式新增章节组细纲工作流：新建书籍会准备 `setting/chapter-groups/`，Agent 可生成下一组细纲，快捷创作增加“下一组细纲 / 按细纲写下一章 / 定稿并同步状态”入口。
- IDE 模式作品目录支持以列表方式展示 `setting/outline.md` 大纲、`setting/chapter-groups/` 细纲，并按章节目录自动分卷折叠。
- 设置页新增章节创作流程配置，支持配置是否启用草稿流程，以及章节组建议规模范围，默认关闭草稿流程并建议 3-8 章。

### Changed

- 更新 README，按当前书籍管理、小说 IDE、创作 Agent、互动工作台、资料库、角色卡导入和版本管理能力重写使用指南，并将新增界面截图改为可折叠展示。
- Agent 写作工作流调整为“创作灵感 -> 大纲 -> 下一组细纲 -> 单章草稿/定稿”，细纲只规划接下来一组章节，章节定稿后才同步 progress 与角色状态。
- Agent 注入场景化风格规则前会把历史相对风格名解析为当前工作区 `setting/styles/` 下的绝对路径，兼容工作区配置并支持跨书籍复用用户配置。
- IDE 模式适配结构化资料库和讲述者：写作工作台新增资料库/讲述者入口，创作 Agent 支持引用资料条目，并会按工作区默认讲述者注入写作规则。
- IDE 模式下资料库和讲述者入口改为覆盖项目目录、编辑区和右侧面板的全工作区管理页。
- WebUI 导航调整：IDE/互动模式切换移到顶部 Nova 标识旁的分段切换，左侧一级菜单按当前模式切换；设置页改为覆盖工作区页面，不再使用弹窗。
- WebUI 细化工作台层级：书籍管理会返回打开前的 IDE/互动模式，版本管理改为全工作区页面，互动模式的场景记忆开关移入剧情页右侧按钮。
- 讲述者 Agent 不再强制只能修改当前选中的讲述者，可根据用户本轮意图新建讲述者、自由选择已有讲述者，或通过输入框 `@` 引用讲述者来限定修改对象。
- 互动故事舞台的下一步行动候选改为在底部输入框聚焦时柔和展开，减少浏览历史时的界面跳动。
- 酒馆角色卡导入入口并入书籍管理，左侧活动栏不再保留独立上传图标。

### Fixed

- 后端设置保存：修复首次没有本地配置文件时，在界面保存 API Key 后当前运行时仍使用旧空配置，导致新建配置无法立即连上模型的问题；保存用户/工作区配置后会同步刷新运行时模型配置。
- WebUI：修复切换书籍后互动工作台资料库、资料库版本、资料库 Agent 历史和相邻设置面板状态仍显示旧书数据的问题，workspace 变化时会先清空旧状态再重新拉取当前书籍数据。

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
- 互动模式将资料库、创作者指令、讲述者、剧情舞台、场景记忆和剧情路线图重新组织为更稳定的工作流。
- 剧情路线图改为左侧导航中的主区视图，支持横向浏览、节点选中、剧情线切换和从节点创建新剧情线。
- 互动故事生成改为正文生成与状态整理分阶段处理，正文先流式落盘，场景记忆随后同步。
- 书籍管理和设置改为全局弹窗，IDE 与互动模式下都能打开。
- 编辑器与互动故事舞台新增字体、字号和行高配置，长文阅读体验更可控。
- 代码结构按领域拆分后端应用层和前端工作台主入口，降低后续维护成本。

## [v0.1.3] - 2026-05-24

### Fixed

- WebUI 编辑区 Tab：修复 Tab 列表出现重复 React key 的报错（`Encountered two children with the same key, file:skills/test/SKILL.md`）——`handleRenameItem` / `handleMoveItem` 通过 `map` 把 `from → to` 时若 `to` 已在打开列表中会产生重复条目，`readTabsFor` 兼容旧版字符串与新版对象持久化时也可能出现同 key 多份；提取 `dedupeTabs` 工具函数并在 `enforceTabLimit`、`readTabsFor`、rename/move 三个出口统一去重
- WebUI 目录树：修复在空目录（如初始 `skills/` 子目录）右键「新建文件 / 新建目录」时内联输入框不出现的问题——空目录被后端 JSON `omitempty` 序列化后 `children` 为 `undefined`，前端 `expanded && node.children &&` 短路掉了承载输入框的子层 `FileTreeList`，改为展开时始终渲染（缺省视为空数组）

### Changed

- 后端 `internal/prompts`：新增独立 prompts 包，集中管理后端所有写死的长段提示词（系统指令 / 计划模式 / 上下文边界 / 异常中断恢复 / 场景化风格规则 / 引用·选区文案 / 未知工具反馈 / `脑暴.md` 与 `CREATOR.md` 模板）。`internal/agent` 与 `internal/book` 改为从 `internal/prompts` 读取，agent 仅保留 IO/上下文拼装薄壳；移除 `agent/prompt.go` 内联指令大字符串与 `book/state.go` `book/creator.go` 的模板常量，提示词文案变更不再需要改动业务包
- 后端 `book` / `app`：重构自动 Commit 触发时机——由「写章节前在 `safeToolMiddleware` 中创建快照」改为「每次新对话 `App.StartTask` 入口自动 commit」；新增 `book.GitService.AutoCommit(ctx, threshold)`，仅当工作区脏且累计 add+del 行数（含 untracked 文件整文件行数）≥ 阈值时才执行 `add -A` + `commit`，默认阈值 `book.DefaultAutoCommitLineThreshold = 50`，未达阈值/工作区干净/仓库未初始化均跳过；自动 commit 失败不阻断对话，仅写日志
- 后端 `agent`：移除 `safeToolMiddleware` 中的 `shouldSnapshotBeforeChapterWrite` / `autoCommitBeforeChapterWrite` 路径及对 `internal/book` 的耦合，中间件回归纯错误兜底；`prompt.go` 与 `skills/continue/SKILL.md` 中关于「写章节前自动 Git 快照」的说明同步删除

### Added

- 后端 `session` / `agent`：新增异常中断恢复标识持久化；Runner/流式读取异常或 Agent panic 时记录待恢复中断，用户后续明确输入“继续/继续刚才/从中断的地方继续”等请求时，会从上一轮异常中断上下文续跑，成功完成后标记该中断已恢复；前端/SSE 断线但后端任务仍运行时仍沿用现有 active task 重连，不写入异常标识
- 后端 `config`：新增 `Settings.StyleRules`（工作区级「场景 → 风格文件」规则集，类型为 `[]StyleRule{Scene, Styles}`），分层 Merge 中 `nil` 表示继承、空切片表示显式清空、非空切片整体覆盖；新增 `TestMergeStyleRules` 单测覆盖三种语义
- 后端 `agent`：当用户本轮未通过 `#` 指定风格参考时，由 `App.StartTask` 从工作区配置读取 `StyleRules` 注入 `ChatRequest.StyleRules`，`ChatService` 通过新增 `appendStyleRulesHint` 在用户消息后追加「场景化默认风格规则 + 触发规则」——仅当 Agent 判定本轮属于章节正文创作/续写/重写时，才根据本轮章节内容选出最匹配的场景并 `read_file` 加载对应风格文件，其他场景（脑暴、大纲、设定、问答等）一律忽略；本轮显式 `#` 指定时仍按原"本轮覆盖默认"语义处理，不注入规则建议
- WebUI：设置页「当前工作区」分层新增「场景化风格规则」分组，采用渐进式披露的「场景 + 风格」编辑器——每条规则一个场景描述输入框，风格文件列表默认折叠（仅显示已选概要），点击「风格 (N)」按钮才展开多选；支持新增/删除规则；继承生效时显示只读概要；类型 `Settings.style_rules` 与 `StyleRule` 同步加入

- 后端 `config`：新增 `Settings.MaxOpenTabs`（默认 5），通过用户/工作区分层覆盖；JSON/TOML 字段为 `max_open_tabs`
- WebUI：编辑区 Tab 数量上限化，超过 `max_open_tabs` 时按 LRU（最久未激活优先）自动关闭旧 Tab，当前激活 Tab 永远受保护；workspace 切换恢复时也会按上限裁剪
- WebUI：设置页「编辑器」分组新增「最大同时打开 Tab 数」配置项；设置保存后通过 `nova:settings-updated` 事件触发主界面立即重新拉取生效配置
- 后端 `book`：工作区初始化 `InitWorkspace` 在缺失时自动写入 `脑暴.md` 顶层定调模板（题材、核心卖点、目标读者、整体风格、金手指、故事尺度、剧情走向、参考作品等），引导作者在生成大纲前先完成顶层设定讨论；新增 `BrainstormFileName`、`BrainstormPath()` 与 `CreatorFileName`，CREATOR.md 模板生成时机一并迁移到 `InitWorkspace`
- 后端 `agent`：在系统提示中加入 `脑暴.md` 路径说明与「生成大纲时」前置工作流——先与作者讨论补全 `脑暴.md` 顶层定调，作者确认定稿后才生成 setting/outline.md / characters.md / world-building.md / progress.md；空作品的状态文案改为引导作者优先填写 `脑暴.md`
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

- 后端 API/命令：移除 `/init` 命令（CREATOR.md 与 `脑暴.md` 模板改由 `InitWorkspace` 在工作区创建时自动生成），`/help` 输出同步去除该项
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
