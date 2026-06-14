# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- WebUI 左侧一级菜单支持拖拽排序，IDE 与互动模式分别保存顺序，避免两种工作台入口互相影响。
- 书籍管理页新增从书架移除和拖拽自定义排序；移除书籍只会从书架隐藏并保留磁盘目录，删除当前书籍后会自动切换到下一个可用书籍。
- Agent loop 新增 `LoopPolicy`、`ContextLedger` 和 `.nova/runs` 运行账本，按轮记录上下文来源、大小上限、事件摘要和完成状态，为后续工具筛选、恢复和验证阶段提供稳定工程边界。
- Agent loop 新增中心化 tool manifest 与模型可见工具结果筛选，统一标注工具来源、是否变更 workspace、输出上限、幂等键和 post-check 要求，并对 invokable/streamable 工具返回做有界回填。
- 创作 Agent 新增写入后轻量验证阶段，会根据工具 mutation metadata 检查写入路径、章节目录约束、资料库 `brief_description` 和删除结果，并写入 `.nova/runs` trace。
- WebUI 创作 Agent 面板新增 Agent Trace 视图，可查看最近运行的上下文账本、工具事件序列、验证结果和截断状态。
- WebUI 接入 Motion for React，新增全局动效强度配置（跟随系统、完整、减少、关闭），并为工作台切换、一级菜单、Tab、面板和聊天消息提供更克制流畅的过渡。
- 设置页新增浅色、深色和跟随系统主题切换；主题配置支持用户级和工作区级继承，并即时应用到主工作台。
- 浅色/深色主题主文字分别使用纯黑/纯白，IDE 编辑器主题会跟随全局浅色/深色切换；默认界面字体改为 Apple 字体栈，界面字号改为 14px。
- 默认主题改为深色模式，首次启动和未配置主题时会进入 dark theme。

### Changed

- 互动故事单轮目标字数改为故事级运行参数，并在互动剧情主舞台顶部直接配置；不再兼容叙事编排 JSON 中的 `reply_target_chars` 旧字段，旧规则包里的该字段不会继续生效，需要在具体互动故事里重新设置。
- 精简互动剧情主舞台顶部和消息区抬头，移除“互动创作”、回合数以及“指令流 / 记录数”状态栏，降低控制区拥挤感。
- 优化中英文 README 首屏定位与能力说明，补充 IDE 式作品管理、创作 Agent、互动故事、结构化资料库、版本管理、Skills/Agents、自动化和导入能力介绍。
- 互动故事 Agent 上下文改为按叙事编排 `recent_turns` 保留最近回合原文，并将更早剧情压缩为有界摘要，避免长线互动把完整历史无限注入模型。
- Nova favicon 去掉右下角 `I` 标记，并改为三色清爽的 iOS 风格图标。
- 重新设计 Nova 极简 SVG 品牌图标，并在中英文 README 首屏顶部展示品牌图标。
- 新增与图标同风格的 Nova wordmark SVG，并重组中英文 README 首屏介绍，强化 AI-native fiction workspace 的高级创作工作室定位。

### Fixed

- 修复设置页多模型配置编辑配置 ID 时输入框随 ID 变化反复重建，导致只能逐字输入的问题。
- 优化互动剧情页和工作台侧栏的数据加载稳定性：切换故事、分支或刷新目录时保留上一份有效内容并显示轻量刷新状态，减少后端响应较慢时的页面抖动。
- 修复互动模式剧情路线图节点在紧凑字号下标题、摘要或 HEAD 标记挤出卡片的问题。
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
- 扩大 WebUI i18n 覆盖面，补齐会话管理、工具卡片、Agent 配置、互动故事舞台、剧情路线图、场景记忆、字体设置和编辑区浮层等模块内的硬编码界面文案。
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
