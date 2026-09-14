# Denova 插件与游戏平台 HTTP API

状态：API v1，当前工作区实现，尚未发布。更新：2026-09-13。当前实现以 [OpenAPI 生成代码](../internal/platform/openapi.go) 和隔离运行实例的 GET /api/platform/v1/openapi.json 为准。

[插件开发手册](plugin-developer-guide.md) · [游戏开发手册](game-developer-guide.md) · [系统设计](plugin-platform-design.md)。

## 边界与连接

Consumer API 使用普通 HTTP + JSON，事件使用 SSE。每个游戏开局或显式插件目标拥有独立运行来源；baseUrl 和 bearer token 通过受验证页面握手或后台 stdin 提供。SDK 是可选封装。

请求使用 Authorization: Bearer <token>。凭证绑定包身份、准确发行、Project/Session/游戏实例范围与已授予权限；body 不能选择其他范围。可信 App 的 /api/platform/manage/* 仅允许本机宿主管理，第三方页面不能借用该路由或历史内部 API。第一版第三方作品在本机桌面使用，不承诺 LAN 远程运行。

包引用为 { kind: "plugin" | "game", id }，发行引用为 { package, releaseId }。公共贡献引用 pluginId/localId，游戏私有定义 local:id。运行时路径仅作当前系统投影；所有受管来源与文件引用按规范相对路径持久化。

错误统一为 { code, messageKey, diagnostic }。diagnostic 用英文供开发日志定位；用户界面根据 messageKey 本地化。常见错误：

| HTTP | code |
| --- | --- |
| 400 | INVALID_ARGUMENT、INVALID_CONFIGURATION、INVALID_TOML |
| 403 | PERMISSION_DENIED |
| 404 | NOT_FOUND |
| 409 | DOCUMENT_CONFLICT、IDEMPOTENCY_CONFLICT、SESSION_BUSY、API_INCOMPATIBLE、DEPENDENCY_UNAVAILABLE、SAVE_INCOMPATIBLE、CONFIGURATION_CONFLICT、CURSOR_EXPIRED |
| 413 | LIMIT_EXCEEDED |
| 503 | RUNTIME_UNAVAILABLE、RUNTIME_FAILED |

apiMajor 必须为 1，不兼容的清单在检查阶段即拒绝。JSON Schema 使用 2020-12，不下载外部引用。清单与定义上限 1 MiB，单个分发文件 16 MiB，包最多 10000 文件、256 MiB；当前管理服务上传请求还受宿主 HTTP body 限制。消费者 JSON 请求上限 1 MiB，文本文件读取上限 16 MiB，Agent 输入和 instructions 分别最多 256 KiB，超限完整拒绝。GET /capabilities 返回当前权限及容量。

## 发现

| 方法与路径（相对 baseUrl） | 结果 |
| --- | --- |
| GET /context | source、scope、locale、theme、environment、settings、可选 setup |
| GET /capabilities | apiMajor、permissions、limits、schemaDialect |
| GET /contributions | 已固定且当前范围可见的贡献与发行列表 |
| GET /openapi.json | 本版完整路径与请求/响应 schema |
| GET /agents/definitions | 当前可使用定义的 items 列表 |

扩展可以组合资料库、图像生成、Agent、工具、可选的现有 Story 引擎与自有前端。宿主不定义角色立绘、演出稿、CG 或台词播放协议；完整应用界面由包的 view 提供。通用 DOM 修改、宿主组件覆盖和内部 planner 接口不属于公开协议。

## 资料库与图像资源

这些接口使用凭证绑定的 Project；请求不能传入另一个 Project。未绑定 Project 返回 `NOT_CONFIGURED`。

| 方法与路径 | 权限 | 输入或结果 |
| --- | --- | --- |
| GET /library/items | library.read | `offset=0&limit=50&query=...`；返回 `{ items, total, nextOffset? }` |
| GET /library/items/{id} | library.read | 返回一条完整资料 |
| GET /assets/content | assets.read | `kind=project或generated&path=规范相对路径`；返回图片字节 |

资料字段为 `id、type、name、tags、briefDescription、updatedAt、enabled、keywords?、content?、image?`。列表省略正文，`limit` 为 1–100，`offset` 非负；query 不区分大小写，在名称、类型、简介、标签和关键词中匹配。`nextOffset` 缺失表示没有下一页。详细读取正文最多 1 MiB，超限返回 `LIMIT_EXCEEDED`，不截断；扩展应按用户选择加载正文，再遵循自身模型上下文预算。

`image` 是 `{ "kind": "project", "path": "assets/lore/images/.../image.png" }`。资产引用仅使用两种来源：`project` 表示已绑定 Project 的 `assets/` 目录，`generated` 表示当前扩展、当前范围生成的图片。路径必须是规范 `/` 相对路径，不允许绝对路径、链接、大小写冲突或目录逃逸。图片类型限 PNG、JPEG、WebP、GIF，单张最多 32 MiB，SVG 和其他活动内容不提供。

浏览器读取图片需要 bearer header，不能将 token 拼入 URL。可使用 `fetch` 获取 Blob，再用 `URL.createObjectURL(blob)` 显示；切换页面或替换图片时调用 `URL.revokeObjectURL`。**只保存资产引用，不保存运行端口、baseUrl、blob URL 或宿主路径。** 相同 `generated` 引用在其他实例或其他包的凭证下不可见。

## 图像生成

需要 `images.generate`，展示结果另需 `assets.read`。清单声明 `kind: "image"` 的 modelSlot，用户为其选择图像模型。消费者只指定本包 slot ID；API 不接受模型凭证、provider URL 或任意 profile ID。

```json
{
  "commandId": "scene-12-background",
  "modelSlot": "illustrator",
  "prompt": "An empty railway platform at sunset, no lettering.",
  "aspectRatio": "16:9"
}
```

| 方法与路径 | 作用 |
| --- | --- |
| POST /images/generations | 提交以上请求；可选 `size、aspectRatio、quality`，每次生成一张图片 |
| GET /images/generations/{commandId} | 查询运行状态或恢复持久结果 |
| POST /images/generations/{commandId}/cancel | 请求取消该操作；继续查询直至终态 |

`commandId` 为 1–128 字节，不含 `/`、`\` 或首尾空白；prompt 为非空文本，最多 64 KiB。运行时返回 202，已完成的重放返回 200；结果结构为：

```json
{
  "commandId": "scene-12-background",
  "status": "completed",
  "images": [{
    "asset": { "kind": "generated", "path": "opaque-image-name.png" },
    "mimeType": "image/png",
    "sizeBytes": 123456,
    "revisedPrompt": "A quiet railway platform at sunset."
  }]
}
```

状态为 `running、completed、failed、cancelled、interrupted`，失败可能附带标准 `error`。调用共享图像服务直接执行 provider 请求，不额外创建 Agent 会话，也不改写资料条目的图片绑定。扩展自行决定将生成结果关联到什么业务对象。

请求执行前持久化去重记录。同一范围内重用相同 commandId 和输入只返回原操作；输入或选定模型不同返回 `IDEMPOTENCY_CONFLICT`。HTTP 断开不会取消已受理操作，停止运行实例会取消并等待资源操作结束。进程中断后无法确认完成的请求返回 `interrupted`，不自动重发可能已经付费的 provider 请求；用户明确重试时使用新的 commandId。

直接图像请求记录与生成文件保存在 scope 的 `data-resources/` 或等价相邻资源目录，由平台管理，并随所在实例或 Project 范围备份。它们不替代 Agent 或 Story 的 canonical journal。托管 Story 扩展采纳图片后，应将资产引用写入对应 Story record，保持演出与正文版本关联。

## 复用现有 Story 引擎

游戏通过 `game.storage.kind: "story"` 声明复用内置 Story。可信管理界面负责新建 Story 或让用户选择已有 Story，再将 `ProjectID、StoryID` 绑定到运行凭证。此接口要求有 instanceId 的托管 game-instance 范围；其依赖插件可以在同一范围和各自授权下使用，普通宿主 Agent 工具的分支范围不会因此扩大。第三方不能在请求中更换绑定的 Project 或 Story；分支操作仅限同一 Story。`storage.kind: "self"` 的既有游戏继续自管数据与玩法。

| 方法与路径 | 权限 | 输入或结果 |
| --- | --- | --- |
| GET /story | stories.read | 玩家可见快照 |
| GET /story/events?operationId=... | stories.read | 当前执行的临时正文 SSE；reset/delta/settled |
| GET /story/history | stories.read | 可选 `branchId、beforeCursor、limit`；返回 `{ turns, beforeCursor?, hasMore }`，每页最多 200 回合 |
| POST /story/commands | stories.write | 提交下表操作，返回最新快照 |
| GET /story/records | stories.read | 按 `key、branchId?、turnId?、sourceRevision?` 读取扩展 JSON |
| PUT /story/records | stories.write | 按准确地址和 expectedRevision 写入扩展 JSON |

快照字段为 `storyId、branchId、title、turns、branches、beforeCursor?、hasMore、status、operationId?、interruptionId?`。turn 字段为 `id、revision、user、narrative、choices、versions`，versions 为可选历史版本的 turn ID 列表；branch 字段为 `id、title、current`。快照不包含隐藏规划、内部模型消息、工具参数或 provider 信息。流式显示订阅 `/story/events?operationId=...`，事件只包含 `kind` 和 delta 的 `text`；reset 撤回临时正文，settled 后重新读取快照。断线重读快照并订阅原 operationId，不重新发起生成。思考、工具参数和子 Agent 事件不进入该流；快照仍是已提交剧情的权威。

命令都有 `kind、commandId`，并可携带目标 `branchId`。`commandId` 为 1–256 字节，模型推进类操作复用既有 Story 去重和恢复机制。

| kind | 其他字段 | 行为 |
| --- | --- | --- |
| advance | message、locale? | 将玩家行动提交给现有 Story Agent |
| resume | interruptionId、message? | 恢复观察到的中断；省略 message 使用默认继续指令 |
| regenerate | turnId、message?、locale? | 重新生成最新回合；省略 message 复用该回合输入，历史回合须先创建分支 |
| stop | operationId | 暂停准确观察到的运行，防止停止后来启动的操作 |
| fork | turnId、title? | 从指定回合创建分支 |
| switchBranch | branchId | 切换同一 Story 的分支 |
| switchVersion | turnId、versionTurnId、branchId? | 切换回合版本 |

上述命令调用现有 Story 应用层，运行时必须已打开绑定的 Project；读取快照、历史和扩展记录不依赖前台 Project。逐句播放、快进、动画或图片显示是扩展自身的阅读行为，只有真正推进故事时才调用 advance。停止扩展只暂停由该实例发起的 Story 运行；卸载或移除绑定保留原有 Story，可回到内置界面继续。

### 扩展 JSON 的版本与恢复

记录由凭证自动确定包身份，扩展不能读取或覆盖其他包的记录。宿主只认识归属、来源版本、格式版本、容量和并发修订；`value` 的业务结构完全由扩展定义，不自动注入模型上下文。

```json
{
  "key": "presentation",
  "branchId": "main",
  "turnId": "turn-id",
  "sourceRevision": "revision-from-story-snapshot",
  "expectedRevision": 0,
  "schemaVersion": 1,
  "value": { "segments": [], "assets": [] }
}
```

读取和写入结果均为 `{ revision, schemaVersion, value }`。不存在时返回 `revision: 0、schemaVersion: 0、value: null`；首次写入 expectedRevision 使用 0，后续使用读取所得 revision，冲突返回 `DOCUMENT_CONFLICT`。key 为 1–256 字节，schemaVersion 为正整数，value 为最多 1 MiB 的有效 JSON；整条 HTTP 请求仍受 1 MiB 上限约束，应预留封装字段空间。

不传 turnId/sourceRevision 表示 Story 或分支级记录；省略 branchId 使用固定 `main`，适合应用绑定和偏好，不能把它理解为“当前分支”。回合记录必须同时指定 turnId 和从快照得到的准确 sourceRevision；明确传 branchId，避免异步结果归入错误分支。写入时宿主检查该回合仍存在于该分支且正文修订未变；变化后写入冲突，不能静默复用旧演出数据。切换或改写正文后，用新快照的地址读取或重新生成演出。

所有扩展 JSON 与应用绑定均写入该 Story 的 canonical JSONL，索引可重建。扩展需要自行处理 schemaVersion 升级；宿主不解释或迁移其中的玩法结构。图片等二进制文件使用独立资产引用，不能把播放进度误写成故事推进。

## 宿主 Agent 工具

宿主 Agent 自动使用已启用且获得授权的插件公开工具。扩展管理 API 的启停、当前安装版本和 settings.toml 是唯一配置来源；会话配置 API 不接受插件选择或设置副本。

适用范围为写作、工作台会话和内置 Story 分支。每次新执行读取当前已安装发行及全局设置；运行中的执行使用已经装载的工具。暂停期间修改相关配置后，原任务可能无法继续，历史保留且不自动重放旧工具调用。准备工具定义不会启动插件后端，实际调用才启动。

宿主 Agent 工具遵循所在会话的审批策略。Consumer API 和游戏私有 NPC 调用 `effect: write/propose` 的工具必须同时具有 `tools.invoke` 与 `tools.write`；未授权返回 `PERMISSION_DENIED`，不自动放行写操作。

## Agent 会话和运行

需要 agents.run。模型通过宿主选择 profile，不向消费者开放模型凭证。定义仅支持 builtin/assistant 和游戏私有的 local:id；不存在公开插件 Agent 定义。

POST /agents/sessions：

```json
{ "projectId": "project-id", "definition": "local:character", "key": "lamplighter" }
```

projectId 必须匹配已绑定范围。相同 key 和相同配置返回原会话（200）；首次创建返回 201。相同 key 不能替换定义或模型配置。响应为 { ref, definition, key }；ref 包含 owner（kind、projectId、sessionId）与 sessionId。

GET /agents/sessions/{sessionId} 返回绑定；GET /agents/sessions/{sessionId}/history?cursor=...&limit=50 返回 { items, cursor }，items 包含 recordId、role、text、createdAt。空 cursor 表示没有更早一页，limit 最大 100。

POST /agents/sessions/{sessionId}/runs：

```json
{ "commandId": "stable-request-id", "input": { "text": "Describe the quiet station." } }
```

202 表示持久接收或找回已有请求。响应 RunResult 包含 run（agent 与 runId）、status、text、可选 error 和 completion（agent、recordId）。状态为 accepted、running、waiting、completed、failed、aborted、incomplete。

commandId 按会话去重，收据写入同一 canonical JSONL 后才允许运行。相同 ID 携带不同输入报冲突。一个会话只允许一个活动运行。模型任务没有固定总时长或迭代上限，可以取消。

| 方法与路径 | 作用 |
| --- | --- |
| GET /agents/sessions/{sessionId}/runs?commandId=... | 刷新／重启后查回 |
| GET /agents/runs/{runId} | 当前或持久结果 |
| GET /agents/runs/{runId}/events | SSE |
| POST /agents/runs/{runId}/cancel | 请求取消，保留历史 |
| POST /agents/runs/{runId}/interactions/{interactionId}/responses | 仅普通问题回答，不能批准权限 |

终态及定义保存在 Product Session JSONL，索引可重建。进程重启时未持久完成的请求返回 incomplete，不自动重放工具副作用；已完成的相同请求不再调用模型。completion.recordId 指向本会话已提交的 assistant 记录，不承诺其他会话或游戏数据库处于同一事务。

SSE 首帧 snapshot 含当前 RunResult 与 cursor，其后发送 delta、state、interaction、result。Last-Event-ID 仅在当前进程保留窗口内有效；窗口最多 256 项，过期或重启返回 CURSOR_EXPIRED，调用方应不带旧游标重新获取 snapshot。连接断开不会取消任务。

## 工具

需要 tools.invoke。POST /tools/{providerId}/{toolId}/invoke：

```json
{ "input": { "text": "A🌷中" } }
```

成功为 { content, data? }，content 是英文模型反馈，data 遵循工具声明的输出 schema。游戏只能调用其私有工具或 game.uses.toolsets 选定的公共工具；依赖贡献列表限制可发现范围。游戏私有 Agent 定义只加载其声明的工具与工具集。插件不声明 Agent 或 Skill。

提供器端接收清单 endpoint 声明的相对 HTTP POST，body 是 input 本身。宿主使用独立 hostToken 调用后台，避免消费者直接调用未公开后端端点。详情见插件手册的启动协议。

## 自管文件

| 方法与路径 | body / 查询 |
| --- | --- |
| GET /game-data/files?directory=... | 列出当前实例文件，返回 { items } |
| GET /game-data/file?path=... | 返回 { content, revision } |
| PUT /game-data/file | { path, content, expectedRevision }，返回 { revision } |
| DELETE /game-data/file | { path, expectedRevision }，返回 204 |

插件自身数据使用对应的 /plugin-data/files、/plugin-data/file。权限分别为 gameData、pluginData，不开放其他实例目录。expectedRevision 为 null 时仅创建；已有文件使用读取所得 revision。

路径必须为不含宿主绝对路径的规范 / 相对路径。拒绝保留名称、目录逃逸、大小写冲突、符号链接与特殊文件。带后端的包直接管理自己的 dataDir，不能同时经此 API 覆盖后端拥有的数据。

## 管理与持久化

可信宿主通过 `GET /api/platform/manage/packages/game/{id}/cover?releaseId={installedReleaseId}` 读取该安装发行声明的 `game.cover`。响应为经类型与大小校验的图片字节，携带 `nosniff` 和同源资源策略；不启动扩展代码。封面为可选字段，无封面的现有安装包无需迁移。

可信管理接口负责统一扩展目录、自动识别类型的检查与安装、授权与启停、Project 源码开发操作以及故事线存档。以下路径相对 /api/platform/manage：

| 方法与路径 | 输入或结果 |
| --- | --- |
| GET /catalog | 带 kind、启用状态、发行及 unavailableReason 的统一已安装列表 |
| GET /packages/{kind}/{id}/settings | 当前设置表单、默认值、覆盖、TOML 和 revision，支持 locale |
| POST /packages/{kind}/{id}/settings/validate | 校验并转换表单/TOML，不持久化 |
| PUT /packages/{kind}/{id}/settings | { releaseId, expectedRevision, format, overrides 或 toml }；校验在用发行，备份并原子保存 |
| PUT /packages/{kind}/{id}/permissions | { releaseId, grants }；更新授权并停止受影响实例 |
| GET /packages/game/{id}/setup | 指定 releaseId 的开局表单，支持 locale |
| GET /candidates/{id}/settings 或 /setup | 独立预览表单，支持 locale |
| POST /packages/preview | JSON { directory } 或 application/zip；根目录必须恰有一种清单 |
| POST /packages/github/preview | { url, ref?, path?, commit? }；下载固定提交并返回候选包，不执行构建 |
| POST /packages/github/import | 同上；创建受管源码 Project，返回 DevelopmentSource，不安装、不执行代码 |
| GET /packages/{kind}/{id}/update | { status: current 或 available, source }，只查询上游提交 |
| POST /packages/{kind}/{id}/update | { commit }；校验安装身份并生成该提交的更新候选包 |
| POST /packages/install | { candidateId, grants }；消费冻结字节，更新候选须仍对应原安装 |
| GET /packages/{kind}/{id}/archive?releaseId={releaseId} | 导出指定已安装发行的准确冻结文件并校验摘要；不需要源码，不包含本机设置、模型绑定或存档 |
| GET /game-preferences | { defaultGameId }，不可用时投影为 builtin.story |
| PATCH /game-preferences | { defaultGameId }，只接受内置或当前可用游戏 |
| GET /development | 自动发现根清单，relativePath 为 .；返回只读 projectName、manifest 或逐项 messageKey；保留显式子目录绑定，损坏草稿仍可修复 |
| POST /development | { projectId, relativePath, kind, id, name }；kind 为 plugin 或 game，各生成一个通用开发骨架，不接受模板选择 |
| GET /development/{id}/build | 当前 Project 目录和可见构建配方 |
| GET /development/{id}/check | 冻结的检查结果 |
| PATCH /instances/{id} | { title }，不改变游戏与存档身份 |

新建普通工作目录使用 POST /api/agent-chat/projects/directory，body 为 { name, parent_directory? }，返回 Project Record。父目录未填写时使用受管 projects 目录。源码初始化与 Project 创建分步执行；失败保留已创建目录，不覆盖已有文件。

GitHub 来源为 `{ url, ref, path, commit }`：URL 规范化为公开 HTTPS 仓库地址，ref 保留跟踪目标，commit 为下载时固定的完整提交；path 使用 `/` 相对路径，`.` 表示仓库根。输入省略 ref 时解析默认分支，省略 path 时使用根目录。来源可选保存在 Candidate、Installed 和导入后的 Development 中，不参与包身份或摘要。构建产物通过摘要区分，来源 commit 仅表示基础源码。传入 commit 会固定此次下载，即使分支已推进也不会改读最新提交。

源码归档与展开大小各不超过 256 MiB，最多 10000 项、单文件 16 MiB；单次网络请求有三分钟超时。只访问 GitHub API 和官方 HTTPS 下载域，不运行 Git hook、依赖安装或构建脚本。拒绝路径逃逸、大小写冲突、链接和特殊文件。GitHub 故障使用独立本地化 `GITHUB_*` 错误；缺少安装产物返回 `GITHUB_BUILD_REQUIRED`，用户可显式导入工作台构建。

同版本号可以安装不同摘要。未指定固定依赖的新增消费者使用当前安装并检查版本范围；已有游戏存档与平台消费者绑定保留准确发行；普通宿主 Agent 的后续执行采用当前插件，更新不改写会话 journal。

停用只禁止新建和重启，不撤销运行中的凭证；卸载及重新授权会先停止受影响实例。已有存档始终保留准确发行，不能随默认游戏改变而改写。

每个扩展的安装与发行聚合于 plugins/{id} 或 games/{id}，包括 installed.json、settings.toml、releases、previews 和 backups。自管存档位于 games/{id}/instances/{instanceId}；插件 data 按范围隔离。托管 Story 的应用绑定和扩展 JSON 位于该 Story journal，生成资产位于同一 Project Store 的扩展范围，不能在安装目录维护另一份 Story 恢复事实。扩展设置与游戏 setup 分开，保存设置保留运行快照，预览不读取已安装覆盖。字段、合并、兼容性与保存协议见[扩展设置标准](extension-settings.md)。

重新安装同一发行可以调整授权，变更前停止受影响实例。平台 Agent 会话不会进入普通写作或 Project 会话入口，避免用其他 Agent 定义继续其历史。

未使用新扩展功能的数据不需批量迁移。平台 Agent 会话使用 platform_session 记录；托管 Story 使用版本化 extension_record 保存应用绑定及自定义 JSON。旧版本不理解这些新记录，不能假设旧版能够继续已使用新扩展功能的会话或 Story；降级前应备份整个数据目录。
