# PRD: Game Mode Narrative Orchestration

## 1. 背景与目标

Denova 游戏模式当前已经支持互动叙事、剧情分支、快捷行动建议、故事记忆、资料库召回和互动图像，但核心体验仍偏向“根据用户输入续写下一段”。本 PRD 目标是在“方案预设”模块中引入强大的叙事编排能力，让游戏模式可以像优秀类型小说一样持续管理节奏、爽点、压力、危机、反转、伏笔、代价和长期主线，而不是单纯补全器。

目标体验：

- 每回合都围绕“目标 + 节奏/压力/危机 + 结果/代价 + 状态”推进。
- 系统能维护长期故事方向、潜在角色、伏笔与回收、节奏曲线，并随用户自由选择动态调整。
- 支持爽文成长流常见事件：打脸、扮猪吃虎、奇遇、秘境、天降、意外、世界事件、冲突、学院、比拼、排行、恋爱、英雄救美、误会与消解等。
- 支持数值系统、TRPG 判定、开局词条抽取和表格记忆优化。
- 用户选择错误会有合理后果，必要时可进入 Bad End、主角死亡、主线失败或分支终局。
- 支持逆袭、复仇、种田等长期情节安排，并保持玩家自由选择。

## 2. 核心结论

本需求基于多轮产品问答后的最终决策：

- Director Agent 不在用户输入后同步前置运行。
- Interactive Agent 在本回合内负责理解用户输入、生成结构化 TurnBrief、选择事件推进方向，并通过规则工具完成数值、骰子和词条结算。
- Director Agent 主要在后台运行，负责维护和更新长期计划、事件队列、伏笔、分支补丁和导演记忆。
- 默认质量优先，互动正文必须基于本回合 TurnBrief、规则结算结果和 Director 当前产物生成。
- 方案预设保存可复用默认配置；每个故事保存独立运行态。
- 旧故事采用懒初始化，不回溯改写历史回合。

推荐的最终链路：

1. 用户输入行动。
2. Interactive Agent 读取当前剧情、故事记忆、Director compact state、事件候选、禁用项和规则清单。
3. Interactive Agent 自行理解用户行动，生成本回合 TurnBrief，并决定希望推进的事件、压力、代价和叙事约束。
4. Interactive Agent 调用 `prepare_interactive_turn` 工具，提交 TurnBrief 中需要固定结算的规则检定请求。
5. 后端工具校验请求，运行声明式规则引擎，返回数值、骰子、词条、状态变化和终局候选等 RuleResolution。
6. Interactive Agent 基于 TurnBrief + RuleResolution 输出本回合可展示正文。
7. 正文落盘后，后台 Director Agent 异步更新长期计划、事件队列、伏笔、分支补丁和导演状态。
8. 后台故事记忆 Agent 继续整理表格记忆，但不得把未来计划默认写入普通故事记忆。

## 3. 现有系统落点

当前相关代码边界：

- 互动故事存储：`internal/interactive/story*.go`
- 互动故事数据结构：`internal/interactive/story_types.go`
- 互动故事主任务：`internal/app/interactive_app_service.go`
- 互动对话上下文：`internal/app/interactive_conversation.go`
- 互动提示词：`internal/prompts/interactive.go`
- 故事记忆：`internal/interactive/memory.go`
- 方案预设 Teller：`internal/interactive/tellers.go`
- Agent kind registry：`config/agent_registry.go`
- Agent 工具权限：`config/agent_tools.go`
- 前端游戏模式：`web/src/features/interactive`
- 前端方案预设编辑：`web/src/features/interactive/components/SettingPanelTellerEditor.tsx`

现有能力可复用：

- `Teller` 已有 system / turn_context / state_memory slots、随机事件率、style_rules。
- `Snapshot.State` 已支持 `rules`、`resources`、`characters`、`events`、`threads` 等状态根路径。
- 故事记忆已有 `current_state`、`protagonist`、`important_character`、`world_context`、`open_threads`、`plot_summary`，并预留恋爱相关结构。
- 互动 Agent 已有资料库和长期记忆工具化召回机制。
- 异步故事记忆整理已经提供后台 Agent 模式，可作为 Director 后台任务的实现参考。

## 4. 用户与场景

目标用户：

- 创作者：希望配置可复用叙事方案、事件库、规则和开局模板。
- 玩家型用户：希望互动故事有主线、节奏、惊喜、代价和可理解的后果。
- 高级用户：希望调试和干预后台导演计划、事件队列和规则结算。

典型场景：

- 新建一个“废柴逆袭修仙学院”游戏故事，选择成长流方案、抽取开局词条和初始属性。
- 用户连续做出偏离主线的选择，系统尊重选择，同时通过事件、NPC 行动和代价软引导回高价值冲突。
- 用户选择危险行动，规则引擎判定失败，主角受伤、失去机会或进入分支终局。
- 用户希望强制安排一次“学院比拼打脸”事件，系统把事件加入近期队列，并要求 Director 后台调整伏笔和节奏。

## 5. 范围

### In Scope

- 方案预设中的叙事编排配置。
- 独立 Director Agent kind 和后台任务。
- Interactive Agent 的 TurnBrief 工具协议。
- 事件管理系统和内置全量事件模板库。
- 声明式规则引擎、骰子/随机、词条抽取、状态变更和规则审计。
- 开局构建器。
- 分支终局 / Bad End 状态。
- 导演面板和规则结算卡。
- 中英双语 UI 文案。
- 旧故事懒初始化。
- 集成测试和前端关键测试。

### Out of Scope

- 本期不直接接入写作模式运行链路，但数据结构应保留未来复用空间。
- 不允许用户执行任意脚本或 JS 作为规则公式。
- 不做持续守护进程式 Director；默认每回合后异步更新，支持手动重建。
- 不回溯迁移旧故事历史回合生成导演计划。

## 6. 产品需求

### 6.1 方案预设升级

Teller 方案预设需要升级为叙事编排方案，继续兼容现有 prompt slots，并新增以下配置能力：

- 导演风格：
  - 主线强度：软引导优先，默认不硬拦截用户自由选择。
  - 失败策略：可配置轻后果、可逆失败、硬核后果、允许终局。
  - 节奏曲线：平稳、递进、高压、爽点密集、慢热。
  - 随机扰动：沿用现有 random_event_rate，但语义升级为事件扰动倾向。
- 事件包：
  - 内置事件模板包可开启/关闭。
  - 用户可用自然语言添加自定义事件模板。
  - 系统内部需将自然语言事件模板归一化为结构化调度元数据。
- 规则模板：
  - 属性、资源、关系数值、骰子规则、失败等级、奖励/代价。
  - 可被故事创建时复制为 story 运行态。
- 开局模板：
  - 词条池、初始属性、背景选项、初始资源、初始关系。

验收：

- 旧 Teller 文件可正常加载，并自动使用默认空编排配置。
- 新 Teller 可配置事件包、失败策略、规则模板和开局模板。
- 保存失败需要提示冲突或字段错误，不得静默丢配置。

### 6.2 Director State

每个互动故事保存独立 DirectorState。它是未来计划和后台导演运行态，不等同于普通故事记忆。

DirectorState 必须包含：

- `enabled`: 是否启用叙事编排。
- `spoiler_mode`: UI 剧透分层状态，仅影响展示。
- `main_arc`: 长期主线，支持逆袭、复仇、种田等长期结构。
- `stage_plan`: 中期阶段计划，如当前篇章/阶段目标。
- `beat_queue`: 未来 3-5 回合节拍。
- `event_queue`: 条件事件队列。
- `foreshadowing`: 伏笔、状态、回收条件、隐藏真相。
- `potential_characters`: 潜在角色和登场条件。
- `branch_patches`: 分支继承后的差异补丁。
- `forced_events`: 用户强制尽快发生的事件。
- `disabled_events`: 用户禁用的事件或长期线。
- `last_director_run`: 最近后台更新状态、错误和摘要。

约束：

- DirectorState 中的未来计划默认不得进入普通故事记忆和下一轮模型上下文全文。
- 注入给 Interactive Agent 时必须生成有界摘要，包含来源和大小上限。
- 分支继承祖先 DirectorState，但从分叉点生成 branch patch，避免不同分支互相污染。

验收：

- 创建新故事时生成默认 DirectorState。
- 打开旧故事时懒初始化 DirectorState，不修改历史 turn。
- 从历史回合创建分支时继承祖先计划，并生成可变更的分支补丁。

### 6.3 TurnBrief 与 RuleResolution

Interactive Agent 在本回合中负责全部语义理解和叙事编排，生成 TurnBrief，但不得把 TurnBrief 直接输出到故事正文。TurnBrief 中涉及事件选择、压力设计、代价设计和剧情方向的部分由 Interactive Agent 自行判断；后端工具只负责固定规则结算、字段校验和审计落盘。

新增工具：`prepare_interactive_turn`

输入由 Interactive Agent 生成：

- `user_action`: 用户行动摘要。
- `intent`: 行动意图，如观察、对话、探索、战斗、交易、逃跑、冒险、修炼、经营、恋爱互动。
- `turn_goal`: 本回合叙事目标。
- `pressure`: 压力、危机或紧迫感设计。
- `event_intents`: Interactive Agent 希望触发或推进的事件类型；工具只随 TurnBrief 记录用于审计，不校验、不选择、不锁定事件。
- `cost_policy`: 可能的代价、失败后果和禁忌。
- `rule_checks`: 需要规则引擎结算的检定。
- `state_expectation`: 本回合结束后可能变化的状态。
- `continuity_notes`: 必须遵守的连续性和禁改事实。

后端工具职责：

- 校验 TurnBrief 字段完整性和大小上限。
- 记录 `event_intents`、`turn_goal`、`pressure`、`cost_policy` 等语义字段，供回合审计和后台 Director Agent 后续更新使用；不对这些字段做文学判断或事件调度。
- 执行 `rule_checks` 中声明的固定规则检定，包括属性读取、骰子、词条、公式、资源消耗、关系数值变化和终局候选判定。
- 生成 RuleResolution。
- 将 TurnBrief 和 RuleResolution 作为本回合审计数据暂存，最终随 turn 落盘。

RuleResolution 返回给 Interactive Agent：

- `accepted_brief`: 校验后的 TurnBrief 摘要。
- `rule_results`: 检定结果、骰子、修正、成功等级、失败等级。
- `state_ops_preview`: 建议状态变化。
- `terminal_candidate`: 是否可能进入终局。
- `rule_constraints`: 由数值、骰子、词条、资源和终局候选产生的正文约束。

约束：

- 编排开启时，Interactive Agent 每回合必须先调用 `prepare_interactive_turn`，再输出正文。
- 如果工具失败，正文可以继续生成，但必须降级为“无编排结果”，并记录可见错误状态。
- TurnBrief 不直接展示给用户；导演面板和回合审计卡可折叠查看。
- `prepare_interactive_turn` 不承担语义理解、文学判断或事件编排职责；这些职责必须留在 Interactive Agent。

验收：

- 测试能证明正文上下文包含 RuleResolution，并且故事正文不泄漏 JSON。
- 工具失败时不会污染正文，也不会阻断整个故事写入。
- 回合落盘后可在 UI 查看本回合 brief 和规则审计。

### 6.4 Director Agent 后台更新

Director Agent 主要负责后台更新，不在用户输入后同步阻塞正文生成。

触发时机：

- 每个互动回合正文落盘后异步运行。
- 用户手动点击“重建导演计划”。
- 用户强制/禁用事件后可触发局部更新。
- 分支创建后可触发分支计划补丁生成。

输入：

- 当前 Snapshot 有界摘要。
- 当前 StoryMemory 有界摘要。
- 本回合 TurnBrief。
- 本回合 RuleResolution。
- 本回合最终正文。
- 当前 DirectorState 摘要。
- 用户强制/禁用事件列表。

输出：

- DirectorState patch JSON。
- 可选 StoryMemory patch，但只能写已发生事实，不得把未来计划写入普通故事记忆。
- 运行摘要和错误状态。

约束：

- Director Agent 默认不允许写文件、Shell、Todo。
- 后台任务必须 recover，避免 panic 影响服务。
- 不使用硬编码超时；如需中止策略，必须走现有可配置 Agent 运行策略。

验收：

- 每回合后可看到 DirectorState 的计划/事件队列更新。
- Director Agent 失败时不影响已完成回合，UI 显示后台计划更新失败。
- 手动重建可以基于当前分支重新生成中长期计划。

### 6.5 事件管理系统

事件系统用于安排爽点、压力、意外、伏笔回收和长期线推进。

内置事件类型必须覆盖：

- 打脸
- 扮猪吃虎
- 奇遇
- 秘境
- 天降
- 意外
- 世界事件
- 冲突
- 学院
- 比拼
- 排行
- 恋爱
- 英雄救美
- 误会与消解
- 逆袭
- 复仇
- 种田
- 资源经营
- 势力压迫
- 公开审判/舆论反转
- 隐藏身份暴露
- 伏笔回收

事件模板字段：

- `id`
- `name`
- `category`
- `enabled`
- `spoiler_title`
- `public_summary`
- `hidden_truth`
- `natural_language_template`
- `normalized_trigger`
- `weight`
- `cooldown_turns`
- `intensity`
- `required_foreshadowing`
- `payoff_target`
- `reward`
- `cost`
- `failure_level`
- `compatible_genres`
- `incompatible_states`

用户自定义策略：

- 用户输入自然语言事件模板。
- Director Agent 或配置管理流程将其归一化为结构化调度元数据。
- UI 首版只需要开放自然语言模板、开关、强度和禁用/强制操作；高级字段可作为折叠详情或后续增强。

验收：

- 用户能开启/关闭事件包。
- 用户能强制某事件尽快进入队列。
- 用户能禁用某事件或长期线。
- 事件触发后会记录冷却和回收状态。

### 6.6 规则引擎

规则引擎负责数值、骰子、词条、失败等级和终局判定。它是确定性系统，不依赖模型执行公式。

能力：

- 属性：生命、体力、灵力、声望、财富、境界、好感、信任、敌意等。
- 资源：物品、货币、组织资源、土地、秘境钥匙等。
- 关系：角色维度的好感、信任、怀疑、敌意、暧昧、误会。
- 词条：开局词条、天赋、缺陷、身份、隐藏背景。
- 骰子：d20、d100、NdM、优势/劣势、爆骰可作为后续配置项。
- 安全表达式：加减乘除、比较、布尔、min/max、clamp、属性引用、标签/词条修正、骰子函数。
- 失败等级：成功、大成功、部分成功、失败、大失败、终局候选。
- 状态变更：生成可审计 state ops。

安全约束：

- 不允许任意脚本。
- 不允许访问文件、网络、环境变量或系统 API。
- 表达式只允许读取显式传入的状态和规则上下文。

随机策略：

- 每次规则结算生成 seed，并随 RuleResolution 落盘。
- 同一回合重生成默认复用原结算。
- 用户可手动重抽，重抽需要生成新的 resolution version。
- 新分支继承祖先结算，从分叉点后重新结算。

验收：

- 同一 seed 下骰子和词条结果可复现。
- 表达式非法时返回结构化错误，不 panic。
- Bad End / terminal 条件能被系统识别并落盘。
- 单项测试不得超过 1 秒。

### 6.7 开局构建器

新建故事时支持可选开局构建器。

功能：

- 从 Teller 的开局模板读取词条池、属性模板、背景选项和初始资源。
- 支持抽取词条、手动选择词条、重抽。
- 支持生成初始角色状态、初始关系、初始目标和第一阶段主线倾向。
- 将结果写入故事初始规则状态和 DirectorState。

验收：

- 用户能跳过开局构建器。
- 用户能抽取/选择词条并创建故事。
- 初始词条、属性和背景进入规则状态，并能被第一回合结算引用。

### 6.8 分支终局与 Bad End

终局是分支状态，不是整个故事删除或锁死。

终局类型：

- 主角死亡
- 主线失败
- 关键目标永久失败
- 关系彻底破裂
- 世界线崩坏
- 用户主动结束

终局数据：

- `terminal: true`
- `terminal_type`
- `reason`
- `final_narrative_summary`
- `caused_by_turn_id`
- `rule_resolution_id`
- `restart_suggestions`

交互：

- 当前分支 terminal 后，输入框默认禁用继续推进。
- 用户可以从任意历史回合创建新分支。
- UI 提供“从上一安全节点开新分支”建议，但不自动回滚。

验收：

- terminal 分支不会继续追加普通 turn，除非用户明确新建分支。
- 分支时间线能标识终局节点。
- 终局说明中英双语可显示。

### 6.9 表格记忆优化

故事记忆继续记录已发生事实，DirectorState 记录未来计划。

优化方向：

- 新增或调整默认结构以支持规则和长期线：
  - `rule_state_summary`: 规则状态摘要，只记录对叙事有意义的数值变化。
  - `relationship_state`: 关系数值和误会状态。
  - `foreshadowing_resolved`: 已回收伏笔记录。
  - `long_term_arc_progress`: 长期线已发生进展。
- 故事记忆 Agent 不得输出未来计划、隐藏真相或未发生事件。
- 故事记忆上下文必须保持来源、用途和大小上限。

验收：

- 本回合发生的规则变化能进入记忆摘要。
- 未触发的未来事件不会进入普通故事记忆。
- Context compaction 仍优先参考已发生剧情纪要。

## 7. 前端需求

### 7.1 导演面板

位置：

- 游戏模式设置/方案预设区域新增“叙事编排 / Orchestration”。
- 当前故事右侧或记忆面板附近新增“导演 / Director”面板。

内容：

- 编排开关。
- 当前阶段目标。
- 无剧透节奏摘要。
- 事件包开关。
- 强制/禁用事件入口。
- 后台 Director 状态。
- 分层剧透展开区：
  - 未来节拍
  - 隐藏真相
  - 伏笔回收计划
  - 潜在角色

设计约束：

- 遵循 Denova IDE 化工作台气质。
- 不用弹窗抢主舞台，优先侧栏/面板/折叠区。
- 支持 light/dark。
- 支持窄屏。
- 所有可见文案中英双语。

### 7.2 回合审计卡

每个回合可折叠显示：

- TurnBrief 摘要。
- 本回合事件。
- 骰子/随机结果。
- 属性和关系变化。
- 成功/失败等级。
- 终局候选或终局原因。
- 后台 Director 更新状态。

默认折叠，避免破坏小说阅读节奏。

### 7.3 开局构建器

新建故事时提供可选步骤：

- 选择规则模板。
- 抽取/选择词条。
- 查看初始属性和资源。
- 确认初始目标。
- 创建故事。

## 8. API 需求

新增或扩展 API：

- `GET /api/interactive/stories/:id/director?branch=...`
  - 获取当前故事/分支 DirectorState。
- `PATCH /api/interactive/stories/:id/director`
  - 更新编排开关、事件包、剧透展示偏好、用户强制/禁用项。
- `POST /api/interactive/stories/:id/director/rebuild`
  - 手动重建当前分支 DirectorState。
- `POST /api/interactive/stories/:id/director/events/:event_id/force`
  - 强制某事件尽快发生。
- `POST /api/interactive/stories/:id/director/events/:event_id/disable`
  - 禁用事件或长期线。
- `POST /api/interactive/stories/:id/rules/resolutions/:resolution_id/reroll`
  - 手动重抽规则结算。
- `POST /api/interactive/opening/roll`
  - 基于方案预设和用户选择抽取开局词条。

扩展现有 API：

- `GET /api/interactive/stories/:id/snapshot`
  - 返回当前 turn 的 brief/resolution/terminal/director status 摘要。
- `POST /api/interactive/chat`
  - 编排开启时运行含 `prepare_interactive_turn` 工具的互动 Agent。
- `GET/POST/PATCH /api/interactive/tellers`
  - 支持新 Teller 编排配置字段。

错误要求：

- 所有错误返回用户可理解信息。
- 前端显示需中英双语。
- Director 后台失败不应导致本回合正文失败。

## 9. 数据与存储

建议新增事件类型：

- `director_state`
- `director_patch`
- `turn_brief`
- `rule_resolution`
- `terminal_outcome`

建议扩展结构：

- `StoryMeta`
  - 编排启用状态和规则模板引用。
- `TurnEvent`
  - `turn_brief_id`
  - `rule_resolution_id`
  - `director_update_status`
  - `terminal_outcome`
- `Snapshot`
  - 当前 DirectorState 摘要。
  - 当前 RuleResolution 摘要。

兼容策略：

- Teller v4 缺失编排字段时使用默认配置。
- 旧 story JSONL 缺失 DirectorState 时懒初始化。
- 不批量改写旧 turn。
- 若读取到未知新事件类型，旧逻辑不能 panic，应忽略或保留 raw。

## 10. Agent 与提示词要求

### Interactive Agent

系统提示应补充：

- 编排开启时，必须先调用 `prepare_interactive_turn`。
- 最终输出只允许故事正文，不允许输出 TurnBrief JSON、规则 JSON、工具协议或 Markdown 标题。
- 必须遵守 RuleResolution 中的规则结算、数值变化、资源代价和终局候选。
- 用户选择错误应产生合理后果，不得无代价化解。
- 软引导优先：尊重用户行动，用事件、NPC 主动性和代价引导回高价值冲突。

### Director Agent

系统提示应补充：

- 只负责后台计划和记忆整理，不负责直接续写正文。
- 输出 DirectorState patch JSON。
- 不得把未来计划写入普通故事记忆，除非该信息已在正文中发生。
- 必须考虑用户自由选择和分支差异。
- 必须维护伏笔、事件冷却、长期线进度和节奏曲线。

## 11. 验收标准

### 产品验收

- 互动故事明显不再只是续写：每回合可看到目标、压力/危机、代价和状态变化。
- 用户偏离主线时，系统尊重选择但能软引导到新的高价值冲突。
- 错误选择会有可理解后果，严重时能进入分支终局。
- 爽文成长流能自动安排逆袭、打脸、奇遇、秘境、比拼等事件。
- 用户能配置事件包、强制事件、禁用事件和规则模板。
- 开局词条能影响后续数值和剧情。

### 技术验收

- 编排开启时，每回合都有 TurnBrief 和 RuleResolution 审计数据。
- Interactive Agent 正文不泄漏内部 JSON。
- Director 后台失败不影响正文落盘。
- 规则结算可复现，可重抽，有审计。
- 分支继承和终局状态正确。
- 旧故事懒初始化后可继续游戏。
- 中英双语 key 完整。

## 12. 测试计划

Go 测试：

- Teller v4 到新结构默认值。
- DirectorState 创建、读取、patch、分支继承。
- TurnBrief 工具输入校验、大小上限、错误降级。
- 事件队列选择、强制、禁用、冷却由 DirectorState 与事件系统测试覆盖，不属于 `prepare_interactive_turn` 固定规则工具职责。
- 安全表达式、骰子、seed 复现、重抽。
- terminal outcome 落盘和 snapshot。
- 旧故事懒初始化。
- 后台 Director panic recover。

Agent 集成测试：

- 编排开启后互动 Agent 必须调用 `prepare_interactive_turn`。
- RuleResolution 注入后正文遵守规则结果。
- 工具失败时正文降级但不污染。
- Director 后台基于 turn brief + narrative 更新计划。

前端测试：

- 导演面板空状态、加载态、错误态。
- 事件包开关、强制、禁用。
- 回合审计卡折叠展示。
- 开局构建器抽取/重抽/确认。
- terminal 分支禁用输入和新分支入口。
- i18n key 检查。

手动验证：

- 宽屏、窄屏、light、dark。
- 长事件名、长伏笔、空 DirectorState。
- 旧故事打开后下一回合生效。
- 重生成默认复用规则结算，手动重抽后生成新版本。

## 13. 发布与文档

这是用户明显感知的大功能，发布时需要：

- 更新 `CHANGELOG.md`。
- 更新 `README.md` 和 `README.en.md` 的游戏模式说明。
- 如创建 release，需要在 Release notes 中说明：
  - 新增叙事编排。
  - 新增 Director 后台计划。
  - 新增规则/骰子/词条/终局。
  - 旧故事懒初始化，不回溯迁移历史。

## 14. 风险与约束

- 规则引擎和 Director Agent 同时引入，工程面大；需要以测试保护核心链路。
- 事件模板全量覆盖容易膨胀；首版 UI 对自定义事件保持自然语言入口，内部结构化。
- 未来计划存在剧透风险；默认分层剧透，不主动展开隐藏真相。
- 规则结算和正文可能冲突；必须让 Interactive Agent 在最终正文前拿到 RuleResolution。
- 不允许硬编码模型运行超时；需要使用现有可配置 Agent 运行策略。

## 15. 推荐实施顺序

虽然产品目标是一次性完整交付，工程上建议按可验证顺序提交：

1. 新数据结构、懒初始化和 API 骨架。
2. 规则引擎和 RuleResolution 审计。
3. `prepare_interactive_turn` 工具和 Interactive Agent 提示词。
4. Director Agent kind 和后台 patch 流程。
5. 事件模板库、强制/禁用/冷却。
6. 开局构建器。
7. 导演面板和回合审计卡。
8. 故事记忆优化。
9. README / CHANGELOG / 集成验收。
