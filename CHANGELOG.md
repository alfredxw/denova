# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
- 创作 Agent 新增 `write_lore_items` 批量写资料库工具，可在大纲定稿、章节定稿或重写后一次性创建/更新多个资料条目，并在 WebUI 自动刷新资料库索引；写入条目缺少简介时会按资料类型、名称、标签和正文自动生成 `brief_description`。
- `scripts/npm-release.sh` 发布到 npm registry 时默认使用 `--auth-type web`，可通过浏览器完成 npm 2FA/认证流程；提供 `--auth-type` 参数并保留 `--otp` 覆盖方式。
- 整理 `ideas.md` 规划记录，补充“续写下一章没自动分卷”待修复项并移除空的 NEED FIX 段落。

### Fixed

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
- 互动模式剧情路线图支持直接切换故事线，每条故事线展示各自独立的剧情路线图。
- 互动模式故事舞台支持展示并持久化 Agent 工具调用卡片，刷新后保留卡片状态但不保存工具输入输出参数。
- 风格参考文件移动到用户级 `<nova_dir>/styles/`，不同书籍可复用同一批 `.md` / `.txt` 文风样本。
- IDE 模式新增章节组细纲工作流：新建书籍会准备 `setting/chapter-groups/`，Agent 可生成下一组细纲，快捷创作增加“下一组细纲 / 按细纲写下一章 / 定稿并同步状态”入口。
- IDE 模式作品目录支持以轻量导航列表展示大纲、细纲，并按章节目录自动分卷折叠；项目文件支持多选批量移动、复制、删除和拖拽整理。
- 设置页新增章节创作流程配置，支持配置是否启用草稿流程，以及章节组建议规模范围，默认关闭草稿流程并建议 3-8 章。

### Changed

- 生产态 Web 静态资源托管支持 `NOVA_WEB_DIR` 和可执行文件相对路径探测，npm 包安装后不再依赖启动时的当前工作目录；npm CLI 未显式配置 `NOVA_DIR` 时默认使用执行命令目录下的 `./.nova`，`NOVA_BACKEND_PORT` 也会作为后端默认端口生效。
- Agent 资料库读取工具从单条 `read_lore_item` 升级为批量 `read_lore_items`，可一次按多个资料 ID 读取完整正文，减少连续工具调用。
- 资料库支持渐进式加载：条目新增常驻、简介自动匹配和手动引用三种加载策略；IDE/互动 Agent 会常驻注入核心资料、展示含简介的非常驻资料索引，并可通过只读工具按需读取资料正文。
- IDE 创作提示词改为以结构化资料库承载角色、世界观、地点、势力、规则和物品等长期设定，不再引导读写 `setting/characters.md` 或 `setting/world-building.md`；作品状态注入也停止回退读取这两个旧文件。
- 后端 Agent 构建接入 `max_iteration` 与 `model_max_retries` 运行时设置，不再使用构建时硬编码值。
- 互动故事 Agent 不再随正文输出 `<HOT_STATE>` 快捷选择，也不再对缺失选择做兜底生成；快捷选择改为用户点击“选择”时由独立 LLM 调用按当前上下文生成。
- 互动模式快捷行动建议生成后会按当前剧情节点持久化到故事 JSONL，刷新后优先复用已生成结果；状态 Agent 不再维护可选择入口。
- 互动模式快捷行动建议不再自动展示，改为输入区显式按钮触发，面板可手动收起并保留生成结果。
- 互动模式底部输入区改为更紧凑的高度和独立行高，减少对故事阅读空间的占用。
- 设置页不再展示场景化风格规则和互动单轮目标字数，这两项集成到每个具体讲述者编辑页，并保存到对应讲述者 JSON。
- 手动保存讲述者时不再重新跳回第一个讲述者，会保持当前讲述者和当前规则选中状态。
- 章节文件名默认模板调整为 `ch{NNNN}-{title}.md`，创作 Agent 会读取配置中的章节文件名模板，文件树按章节数字排序以支持千章作品。
- 更新 README，按当前书籍管理、小说 IDE、创作 Agent、互动工作台、资料库、角色卡导入和版本管理能力重写使用指南，并将新增界面截图改为可折叠展示。
- 讲述者规则配置页优化交互：规则启用开关移到左侧规则列表，注入位置改为紧凑下拉选择，减少详情区占用并提升操作效率。
- 创作 Agent 工具卡片统一为暗色面板风格，优化执行中、结果、详情和待办列表的边距、状态图标与展开区域质感。
- Agent 写作工作流调整为“创作灵感 -> 大纲 -> 下一组细纲 -> 单章草稿/定稿”，细纲只规划接下来一组章节，章节定稿后才同步 progress 与角色状态。
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
- 后端 `interactive`：讲述者 JSON 新增 `reply_target_chars` 和 `style_rules`，场景化风格规则按当前讲述者独立生效。
- 后端 `agent`：当用户本轮未通过 `#` 指定风格参考时，由 IDE 默认讲述者或互动故事当前讲述者注入 `ChatRequest.StyleRules`，`ChatService` 追加「场景化默认风格规则 + 触发规则」提示。
- WebUI：具体讲述者编辑页新增「单轮目标字数」和「场景风格规则」编辑能力，支持新增/删除规则、选择用户级风格文件和手动添加 `.md` / `.txt` 路径。

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
