# Master 发布核心回归清单 / Master Release Core Regression Checklist

## 1. 目的与发布判定

本清单是 Denova 版本分支合并到 `master`、创建 Git tag 和发布产物前的最小发布门禁。它验证最终用户能否安全完成核心创作流程，不代替单元测试、专项测试或变更审查。

- 所有标记为“每次发布”的项目都必须通过；失败会阻塞合并、打 Tag 或发布。
- “本次候选版本”项目用于 `codex/agent-runtime` 这一轮大规模改造，也必须执行。
- 外部模型、图像、远程访问等条件项如无法执行，必须记录原因、风险、负责人和补测时间，不能静默跳过。
- 测试结论只对被测试的源码树有效。合并冲突修复、依赖变更、版本号修改或其他代码变更后，至少重跑自动门禁和受影响链路。
- 最终 Git tag 必须指向通过自动门禁的 `master` 提交；发布包还需在该 Tag 产物上完成一次安装与启动冒烟。

### 当前候选版本风险快照（2026-08-14）

此快照用于说明本次回归范围，执行时应重新获取远端并更新提交信息。

| 项目 | 当前值 |
| --- | --- |
| 候选分支 | `codex/agent-runtime` |
| 目标分支 | `origin/master` |
| 当前共同基线 | `v0.3.3`（`c8b4b60a`） |
| 当前 `origin/master` | `344c2054` |
| 变更规模 | 约 2,300 个文件；不能按普通小版本只做改动点冒烟 |
| 主要风险 | Agent runtime/session/context/tooling、Writing/Game/AgentChat 共用会话、Project 与工作区数据、Diff Review、设置与模型协议、自动化、终端、Trajectory |

当前候选分支尚未以最新 `origin/master` 为祖先。最终回归前必须先形成包含目标 `master` 的候选提交并解决冲突；不能用合并前的通过结果替代最终提交验证。

## 2. 执行记录

每次发布复制本文件为 `docs/tests/release-<version>-<date>.md`，填写本节并保留失败、重试和豁免记录。不要在长期清单中直接提交一次性勾选状态。

| 字段 | 值 |
| --- | --- |
| 版本 / Version |  |
| 候选提交 / RC commit |  |
| 最终 `master` 提交 |  |
| Git tag |  |
| 测试日期与负责人 |  |
| 操作系统 / 架构 |  |
| 浏览器与视口 |  |
| 语言 / 主题 |  |
| 语言模型 Profile / 协议 |  |
| 图像模型 Profile |  |
| 测试数据副本 |  |
| 自动门禁结果链接 |  |
| 浏览器证据链接 |  |
| 未覆盖项与风险接受人 |  |

## 3. 测试数据与环境

所有迁移、删除、覆盖、版本恢复和安装测试只能在临时目录或完整副本上执行，不能直接使用唯一的用户工作区。

- [ ] 一个全新的临时 `DENOVA_DIR`，用于首次启动、新建书籍和新建普通 Project。
- [ ] 一份 v0.3.3 用户数据与作品目录的完整副本，至少包含已完成的写作会话、长工具调用、游戏故事、剧情分支、资料库、预设和版本历史。
- [ ] 一份长篇作品：大量章节、长正文、深层目录和长文件名，用于目录虚拟化、滚动与搜索替换。
- [ ] 一个普通本地目录 Project，用于 General Agent、Files 和 Terminal；与当前书籍目录不同。
- [ ] 至少一个可真实调用的语言模型；本次协议改造还需分别覆盖 OpenAI Responses、OpenAI Chat Completions、Anthropic Messages，或明确记录缺失协议。
- [ ] 如发布图像能力，准备一个可真实调用的图像 Profile 和允许删除的测试图片目录。
- [ ] 宽屏至少覆盖 `1440×900`，窄屏至少覆盖 `390×844`；中文/英文、light/dark 采用交叉组合，导航和设置页需四种状态都看一次。
- [ ] 大型前端回归使用应用内浏览器覆盖写作和游戏主链路；系统性断言可用 Playwright，依赖真实账号或现有登录态时再用 Chrome。
- [ ] 复用用户已有前后端时不要启动第二套进程；后端变更使用 `scripts/restart-backend.sh` 更新可见进程。

## 4. 自动化发布门禁（每次发布）

先确认候选提交包含最新目标分支：

```bash
git fetch origin
git merge-base --is-ancestor origin/master HEAD
git status --short
git diff --check
git diff --check origin/master...HEAD
```

在根模块和 `agent` 子模块分别执行依赖、测试与静态检查：

```bash
go mod tidy -diff
go test -count=1 ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

(cd agent && go mod tidy -diff)
(cd agent && go test -count=1 ./...)
(cd agent && go vet ./...)
(cd agent && go run golang.org/x/vuln/cmd/govulncheck@latest ./...)
```

本次 Agent runtime、并发会话和文件持久化改造还必须在支持 race detector 的 Unix 主机执行：

```bash
go test -race -count=1 ./...
(cd agent && go test -race -count=1 ./...)
```

执行前端完整门禁与本机分发构建：

```bash
pnpm --dir web install --frozen-lockfile
pnpm --dir web check:i18n
pnpm --dir web test
pnpm --dir web build
./scripts/build.sh
```

验收要求：

- [ ] 所有命令退出码为 0，`go mod tidy -diff` 不产生改动，`git diff --check` 无错误。
- [ ] 没有 race、panic、goroutine 泄漏、未处理 Promise rejection 或新增的构建 warning。
- [ ] 中英文 i18n key 完全对齐；用户可见新增文案没有直接写死单一语言。
- [ ] `./scripts/build.sh` 完整结束，包含前端、主程序、updater、内嵌 Web、Skills 和受校验的 ripgrep；外网下载失败不能记作构建通过，需在网络恢复后重跑。
- [ ] PR title、commit message 使用英文；CI 的 Test and build 与 Windows filesystem durability 两个 Job 均通过。

## 5. 启动、升级与数据边界（每次发布）

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| DATA-01 | 用全新 `DENOVA_DIR` 启动，完成引导、新建书籍、新建普通目录 Project，重启后重新打开 | 页面可用；书籍、Project、当前选择、设置和新会话均能恢复 |
| DATA-02 | 用 v0.3.3 数据副本首次启动候选版本 | 作品文件、已完成写作会话、游戏正文/状态/分支、资料库、预设和版本历史不丢失；需要迁移时有备份或回执 |
| DATA-03 | 在两个书籍和一个普通 Project 间快速切换，同时保留一个未发送草稿和一个待自动保存编辑 | 切换前先正确保存或明确阻止；数据不会写到另一个 Project，返回后草稿和布局状态正确 |
| DATA-04 | 同时打开前台书籍与后台 Project 的 Files、Reader、Lore、Agent 会话并分别修改测试内容 | 所有 API、缓存、事件与 Diff 都按稳定 Project ID 隔离；当前书籍不会被后台操作替换 |
| DATA-05 | 放入一个损坏或退役的会话/故事索引副本后刷新列表 | 单个坏项被隔离或给出明确错误，不阻断其他会话、故事和新建操作；原始数据不被无声覆盖 |
| DATA-06 | 检查失败请求、SSE 错误和服务端日志 | UI 保留双语错误和 Log ID；日志含 request ID 与源码位置，无凭据、完整敏感 URL 或用户正文泄漏 |

任何跨 Project 写入、无备份覆盖、已完成正文丢失、故事回合归零或不可逆迁移都直接阻塞发布。

## 6. 导航与共用界面（每次发布）

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| NAV-01 | 分别显式切换 Writing 与 Game，再刷新页面 | 只有用户点击模式切换器才改变模式；刷新恢复用户最后选择 |
| NAV-02 | 从 Writing、Game 各自依次进入 AgentChat、Books、Lore、Presets、Versions、Skills、Agents、Automations、Settings；启用开发者模式后再进入 Trajectory | 共享一级菜单绝不自动切换 Writing/Game；任意时刻只有一个一级菜单高亮，关闭共享页返回原模式 |
| NAV-03 | 拖动一级菜单、左右侧栏和分栏宽度，折叠后刷新并切换 Project | 只有真实用户调整会被持久化；折叠不会被拖动误触发；不同布局上下文不串值 |
| NAV-04 | 在中英文、light/dark、宽窄屏检查上述菜单、空状态、错误态和长文本 | 无关键文案缺失、不可读配色、横向页面溢出或不可达操作；窄屏使用正确抽屉/下拉 |
| NAV-05 | 用键盘遍历模式切换、一级菜单、列表展开和主要 Dialog | 焦点可见且顺序合理；模式按钮状态互斥；列表父项整行可展开/折叠 |

## 7. Agent runtime 共用主链路（每次发布）

先在下列入口各完成至少一个真实 Run，验证同一 runtime 在各产品 Adapter 上的行为一致：

| 入口 | 最小真实流程 |
| --- | --- |
| Writing Agent | 普通回复 → `read` / `grep` → `write` 或 `edit` → Diff Review |
| Game Agent | 回合准备/规则判定 → 流式正文 → `submit_interactive_turn` 原子落盘 |
| AgentChat / General Agent | `read` → 平台 Shell → `ask` → `task` 子 Agent |
| Config Manager | `config_read` → 对一个测试资源执行 `config_apply` |
| Automation | 手动运行一个 Prompt 工作流，并完成一次需要确认的动作 |
| Image Agent | 在配置了真实图像模型时生成并保存一张测试图 |

所有入口共同检查：

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| AGENT-01 | 观察思考、正文、工具名、参数 delta、执行进度和结果 | SSE 实时到达；一次调用始终复用一张工具卡；顺序、状态和最终正文正确，SSE 不被 gzip 缓冲 |
| AGENT-02 | 在 Ask、Write、Full access 间切换，用无害的只读与测试写入命令验证边界 | 会话独立保存权限；审批只由真实命令与规则决定，展示用 `description` 不影响安全判断 |
| AGENT-03 | Run 进行中刷新页面 | 同一后端进程内能附着活动 Run，继续看到排队、流式输出、工具和交互；不会新建重复 Run 或重复写文件 |
| AGENT-04 | Run 进行中输入追问草稿，再分别测试发送和中断 | 中断和发送入口按设计并列；中断作用于精确 Run，已接受后续输入顺序稳定，不会误杀其他会话 |
| AGENT-05 | 在测试数据副本上让后端于工具前后各重启一次 | 未完成 Run 明确显示 `interrupted`，不会重放命令/工具 effect；用户可发起新 Run 重试，已完成内容与 canonical 数据仍在 |
| AGENT-06 | 生成一个含长 thinking、多工具和长结果的单一用户回合，刷新并向上分页 | 历史按完整用户回合分页，不截断末尾工具或正文，不错误显示“加载更早消息” |
| AGENT-07 | 回答、取消并重复提交一次 `ask`；再冷重启检查待处理 Ask | 有且仅有一个规范结果；重复请求幂等；冷恢复按安全规则取消，不出现错误 404 |
| AGENT-08 | 运行近似 `grep` 写法、canonical `rg`、分页 cursor、长结果读取及 `edit operation=delete` | 搜索规范化和计数单位正确；危险参数/外部路径被拒绝；大结果可分页/恢复；删除进入 Diff 并可完整撤销 |
| AGENT-09 | 在 Writing/AgentChat 测试 `/goal`、Plan 和自动续跑 | Goal 与 Plan 互斥；目标可编辑/暂停/清除并跨刷新恢复；只由根 Agent 结束，游戏入口不出现 Goal |
| AGENT-10 | 运行一个有长输出的 SubAgent，分别在运行中和完成后打开详情 | 运行中跟随最新输出；完成后首次从开头展示且可回到底部；运行路径和一张委派卡保持稳定 |
| AGENT-11 | 短回复、长回复和主动上翻各跑一次 | 短会话从顶部排列且无底部大空白；长会话默认到最新；用户主动上翻后不被强制拉回 |

## 8. Writing 核心回归（每次发布）

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| WRITE-01 | 新建书籍，并分别导入一个 `.txt` 与 `.md` 小说副本 | 预览、章节拆分、目录、正文、统计和初始版本正确，失败不会留下半成品 |
| WRITE-02 | 在长篇作品中展开/折叠卷、定位最新章、切换多章与多个 Tab，再返回原文件 | 大纲可用且无数秒主线程卡死；每个文件恢复自己的阅读位置与撤销历史，Tab 顺序跨刷新保存 |
| WRITE-03 | 编辑正文，测试自动保存、外部磁盘修改、并发冲突与文件删除 | 内容/结构事件不混淆；自身保存不造成刷新回声；冲突可恢复，删除文件刷新后不会恢复为空白 Tab |
| WRITE-04 | 执行全局普通搜索、正则搜索、普通替换和正则替换 | 预览、命中数和实际文件一致；不会扫描 Denova 状态目录；失败/冲突时不产生部分不可恢复写入 |
| WRITE-05 | 让 Writing Agent 新建、修改、删除测试文件，并在运行中查看变更摘要 | 首次写入即预留 Diff 摘要；完成后显示完整增删与文件状态；当前 Project 身份正确 |
| WRITE-06 | 在 Diff Review 中筛选文件、切换文件、加评论、接受/驳回、Undo/Redo | Monaco 与底层编辑状态稳定；新增/修改/删除/冲突状态正确；每个操作可逆且刷新后仍一致 |
| WRITE-07 | 在正文和 Lore 的选区创建评论，提交给 Agent 后刷新/重连 | 锚点和 revision 正确；反馈在首次模型请求前只消费一次，之后从输入区与资料页消失 |
| WRITE-08 | 从非最新章节触发“续写下一段” | 始终追加到实际最新非空章节，不误写当前选中旧章，也不自动创建下一章 |
| WRITE-09 | 新建、切换、重命名、删除写作会话并完成多轮消息 | 列表消息数实时更新；会话绑定不为空且不串 Project；刷新后完整恢复 |

## 9. Game 核心回归（每次发布）

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| GAME-01 | 新建故事，使用默认 Director、动态 Actor State、规则与 5 个行动建议完成开局 | 空回合为 `[]` 而非崩溃；开局结构与初值闭环；正文、建议、状态和回合一次性原子落盘 |
| GAME-02 | 连续完成至少 3 个回合：行动建议、自由输入、需要规则检定和无需检定各一次 | 叙事连续；同一回合重复准备只复用首次判定；每回合正文、状态、建议和真实回合数一致 |
| GAME-03 | 正文已流式输出后由纯工具消息提交回合 | 最终正文不会因最后一条 assistant 消息为空而丢失，回合成功持久化 |
| GAME-04 | 中断一个生成中的回合并重试，再重新生成一个已完成回合 | 中断不留下半回合；重试只新增一个回合；重新生成替换原回合而不是增加回合数 |
| GAME-05 | 编辑一条已保存 AI 回复，刷新后检查正文与历史 | 修改稳定保存，不破坏状态、建议、分支与后续历史 |
| GAME-06 | 从历史 AI 回复创建分支，切换主线/分支并删除测试分支 | 来源预览、自动切换、路线图与真实回合数正确；主线内容不变，剧情一级菜单保持唯一高亮 |
| GAME-07 | 加载更早历史、搜索早期事实、点击左侧旧回合定位 | 分页不丢回合；搜索来源正确；跳转定位真实历史回合而非最新回合 |
| GAME-08 | 检查导演台的变化、角色、世界、事件、规则与分支，并编辑一个测试模块 | Director 计划和 Actor State 与最新回合一致；CAS 冲突不会覆盖其他保存；模块可独立关闭 |
| GAME-09 | 用 v0.3.3 故事副本和含退役 compaction 事件的副本首次打开 | 索引可安全重建；必要迁移先备份再原子重连；已完成正文、状态和分支不变 |
| GAME-10 | 在 `ready` 状态空闲观察 10 秒，再检查长回合流式期间交互 | 空闲时没有持续拉取完整 snapshot/branch 的轮询；无重复 404、long task 风暴或明显逐字符卡顿 |

## 10. 版本与恢复（每次发布）

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| VERSION-01 | 创建手动版本，修改多个文件后查看版本 Diff | 版本说明、时间、文件列表和完整 Diff 正确；后台 Project 不污染当前书籍 |
| VERSION-02 | 先查看整工作区恢复计划，再执行恢复 | UI 明确告知自动备份；恢复到目标内容且可通过备份返回恢复前状态 |
| VERSION-03 | 只恢复新增、修改、删除三类测试路径 | 全部路径先预检；任一路径失败时整体回滚，不出现混合版本；符号链接逃逸被拒绝 |
| VERSION-04 | 在存在未保存编辑或未审阅 Agent Diff 时发起恢复/切换 | 明确阻止或要求确认，不静默覆盖用户内容 |

## 11. 本次候选版本追加回归（`codex/agent-runtime` 必须执行）

### AgentChat、Project、Files 与 Terminal

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| RC-CHAT-01 | 在 AgentChat 添加一本现有书和一个普通目录，测试新建、重命名、排序、Pin、归档、目录重连 | 稳定 Project ID 不变；历史和状态保留；操作不切换当前 Writing/Game 模式 |
| RC-CHAT-02 | 创建多个对话和终端 Tab，拖动排序并跨左右工作区迁移，隐藏右栏后刷新 | Tab、草稿、编辑器和后台 Run 保持挂载；分栏宽度与项目级状态正确恢复 |
| RC-CHAT-03 | 在 Files 中执行创建多级路径、保存、重命名、多选拖动、粘贴和删除，并模拟外部文件变化 | 目标目录、选择恢复和部分成功回执正确；无自刷新回声、错误自动保存或跨 Project 变更 |
| RC-CHAT-04 | 在后台 Book 打开 Reader/Lore 并编辑，同时保持前台书籍不变 | 章节、Lore、评论和自动保存全部绑定后台 Project；前台模式与书籍不被替换 |
| RC-CHAT-05 | 启动 Shell 和一个已配置 CLI，切换主题，退出 CLI，断开后重连 | PTY 可输入输出和 resize；主题即时刷新；CLI 退出回到原目录 Shell；重连从有界 scrollback 恢复 |

### 设置、资源、自动化与开发者工具

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| RC-SHARED-01 | 新建/编辑 Model Profile，反复切换 provider/protocol/Base URL，获取模型建议并用未保存草稿测试连接 | 自定义模型仍可输入；API Key/Header 只在同源继承；协议选项不串线；日志不泄漏凭据或查询参数 |
| RC-SHARED-02 | 在 Writing、Game、AgentChat 与 Config 会话分别修改模型、thinking 和权限后改全局设置 | 旧会话保持自己的冻结配置；新会话继承同类最近配置；Game 新分支复制来源配置 |
| RC-SHARED-03 | 对 Lore、叙事风格、故事导演、事件包、规则、Actor State、图像预设各做一次读取；至少编辑、排序并冲突保存一个测试资源 | Writing/Game 共用资源即时同步；SHA-256 revision CAS 生效；冲突不覆盖远端内容 |
| RC-SHARED-04 | 创建/编辑一个 Skill，通过 `/` 与 `、` 显式调用；再用 `@` 文件/Lore 和 Game `#` 风格选择器 | 候选列表、键盘操作和标准 token 正确；Skill 只按需加载；长说明不破坏输入框布局 |
| RC-SHARED-05 | 在测试 Project 放置 `AGENTS.md` 与 `CREATOR.md`，修改其中一个后各运行一轮 | 两个来源独立、顺序与 revision 可审计；单文件变化只影响对应上下文边界；模型可见内容保持英文 |
| RC-SHARED-06 | 创建自动化，手动运行、确认一次写操作、查看 Inbox/时间线，再测试取消和后端重启恢复 | 定义与 Run 按 Project 隔离；确认幂等；状态不永久卡在 running；不会重复执行 host effect |
| RC-SHARED-07 | 关闭开发者模式运行一轮，再开启后运行一轮并打开 Trajectory | 关闭时不持久化完整敏感内容；开启后 Request/Input/Output、messages、tools、reasoning 和结果完整可审计 |
| RC-SHARED-08 | 在 Trajectory 切换 Conversation/Timeline/Raw、Readable/Debug，展开工具、导出 JSONL，并检查宽窄屏和深浅主题 | 层级、call ID 配对、schema、耗时、token/cache 和原始数据正确；详情 inline/侧栏/抽屉可用，无 metadata-only 崩溃 |
| RC-SHARED-09 | 若 Harness 优化 Lab 随版本发布：选择明确 trajectory，生成草稿、校验、发布并恢复一个测试 State | 空选择不扩大范围；草稿与已发布 State 隔离；校验失败可继续修复；发布/恢复不覆盖未保存内容 |

### 上下文、缓存与持久化

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| RC-CTX-01 | 连续两轮不改变 Project/Context State，再修改一项状态运行第三轮，通过 Trajectory 比较请求 | 稳定前缀不重复旋转；无变化零注入；变化以增量注入；tool schema 与 Skill 目录变化边界符合设计 |
| RC-CTX-02 | 触发手动压缩和一次达到阈值的自动压缩，再继续对话 | 压缩复用原 Agent model/system/tools/options；完成后状态重建；无循环压缩、协议断链或历史正文重写 |
| RC-CTX-03 | 产生超过 128 KiB 的可恢复工具结果并继续两轮 | 完整结果安全外置；模型获得有界预览/receipt 并可分页恢复；UTF-8、Markdown 围栏和来源/用途标记完整 |
| RC-CTX-04 | 构造超过 resident window 的长会话后重启并继续 | 完整 canonical 区间可按 locator 读取；最新工具结果不丢；显示历史与模型上下文保持分离 |
| RC-CTX-05 | 检查 session、trace、ledger、artifact 文件权限与内容 | 敏感目录/文件为受限权限；默认 ledger 不落用户/工具正文、原始错误、thinking 或凭据形态恢复路径 |

## 12. 条件回归

以下功能只要本次仍作为可用能力发布，或相关代码有改动，就应执行；无法执行时按第 1 节记录风险。

| ID | 操作 | 通过标准 |
| --- | --- | --- |
| OPTIONAL-01 | 分别从写作章节、游戏回合和 Image Agent 生成图片；多图中人为制造一项保存失败 | 成功图片仍可见并落盘；逐项失败明确；全部失败才整体失败；Project 路径正确 |
| OPTIONAL-02 | 开启 LAN 访问与账号密码，从另一设备登录；随后关闭 | 未认证请求被拒绝；登录、SSE、WebSocket、资源访问正常；关闭后不再暴露 LAN |
| OPTIONAL-03 | 在窄屏浏览器完成 PWA 加载、模式切换、发送 Writing/Game 输入和恢复会话 | 安全区、键盘、输入框、抽屉与滚动可用，无桌面专属阻断 |
| OPTIONAL-04 | 使用合法公网来源安装一个测试 Skill，并测试非法协议、重定向到私网和符号链接包 | 合法安装成功；SSRF、路径穿越、symlink 和超限包被拒绝且不留下半安装目录 |
| OPTIONAL-05 | 检查更新、下载并应用一个可控测试版本 | 版本比较、下载进度、校验、原子替换、失败回滚和重启提示正确，用户数据目录不变 |

## 13. Release 产物门禁

版本元数据必须在最终候选源码中先完成，再运行发布构建：

- [ ] `web/package.json` 与 `npm/package.json` 都是 `X.Y.Z`。
- [ ] `CHANGELOG.md` 存在 `## [vX.Y.Z]`，包含用户可感知变化、修复和明确的不兼容行为。
- [ ] `README.md` 与 `README.en.md` 的版本、功能和安装说明同步。
- [ ] Release commit message 与 PR title 使用英文，Tag 为 `vX.Y.Z`。

执行：

```bash
scripts/build-github-release.sh "vX.Y.Z"
scripts/npm-release.sh --pack
```

产物验收：

- [ ] 生成 `darwin-arm64`、`darwin-x64`、`linux-arm64`、`linux-x64`、`windows-x64` 五个平台归档及 `checksums.txt`。
- [ ] 每个归档含主程序、updater、`web/`、`skills/`、`tools/`、`licenses/`、配置、双语 README、CHANGELOG 与 LICENSE。
- [ ] 所有归档 SHA-256 校验通过；文件名、内部目录与 Release tag 一致。
- [ ] 在当前原生平台从归档运行 `denova --version` 和 `denova-updater --version`，均输出 `X.Y.Z`。
- [ ] 用临时 `DENOVA_INSTALL_DIR`、`DENOVA_BIN_DIR`、`DENOVA_DIR` 执行一次指定版本安装和重复安装；用户数据目录选择被保留，失败能回滚旧安装。
- [ ] 从原生平台归档启动 Denova，使用全新数据目录完成“打开页面 → 新建书 → 一轮 Writing → 一轮 Game → 重启恢复”的最终冒烟。
- [ ] npm tarball 只包含声明文件，当前平台 wrapper 能启动正确二进制并输出 `X.Y.Z`。
- [ ] Git tag 指向已测试的最终 `master` 提交；GitHub Release notes 只写简要用户变化，不复制完整 Changelog；上传资产与本地校验值一致。

## 14. 最终放行标准

同时满足以下条件才能发布：

- [ ] 自动化门禁、最终 `master` CI、Writing 主链路、Game 三回合、Agent runtime 刷新/中断/重启、数据升级、Project 隔离、Diff Review 和版本恢复全部通过。
- [ ] 本次候选版本追加回归全部通过，或每个未执行项都有明确风险接受记录。
- [ ] 浏览器控制台无未解释的 exception/unhandled rejection，网络面板无重复 4xx/5xx、错误轮询或被压缩缓冲的 SSE。
- [ ] 服务端日志无 panic、race、重复工具 effect、跨 Project 路径、敏感信息泄漏或持续失败重试。
- [ ] 所有临时测试数据已删除；如执行了真实删除或恢复，报告中写明目标、备份和可恢复位置。
- [ ] 最终原生平台安装包冒烟通过，Tag、版本号、README、Changelog、Release notes 和校验文件一致。

以下任一问题直接阻塞发布：用户内容丢失或串 Project、共享菜单自动切换模式、Writing 无法安全写作/审阅、Game 回合无法原子落盘、Agent 重复执行工具或静默重放、恢复流程不可回滚、构建/测试/race/安全扫描失败、发布包无法安装启动。
