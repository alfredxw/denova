# Denova 插件与游戏平台 HTTP API

状态：第一版 A，尚未发布。更新：2026-09-12。当前实现以 [OpenAPI 生成代码](../internal/platform/openapi.go) 和隔离运行实例的 GET /api/platform/v1/openapi.json 为准。

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

动态 context、资源域、图像、面板、编辑、托管 Story flow 与 planner 尚不属于本版发现列表。

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
| GET /game-preferences | { defaultGameId }，不可用时投影为 builtin.story |
| PATCH /game-preferences | { defaultGameId }，只接受内置或当前可用游戏 |
| GET /development | 自动发现根清单，relativePath 为 .；返回只读 projectName、manifest 或逐项 messageKey；保留显式子目录绑定，损坏草稿仍可修复 |
| POST /development | { projectId, relativePath, kind, templateId, id, name }；templateId 仅支持 http-tool（插件）和 npc-game（游戏） |
| GET /development/{id}/build | 当前 Project 目录和可见构建配方 |
| GET /development/{id}/check | 冻结的检查结果 |
| PATCH /instances/{id} | { title }，不改变游戏与存档身份 |

新建普通工作目录使用 POST /api/agent-chat/projects/directory，body 为 { name, parent_directory? }，返回 Project Record。父目录未填写时使用受管 projects 目录。源码初始化与 Project 创建分步执行；失败保留已创建目录，不覆盖已有文件。

GitHub 来源为 `{ url, ref, path, commit }`：URL 规范化为公开 HTTPS 仓库地址，ref 保留跟踪目标，commit 为下载时固定的完整提交；path 使用 `/` 相对路径，`.` 表示仓库根。输入省略 ref 时解析默认分支，省略 path 时使用根目录。来源可选保存在 Candidate、Installed 和导入后的 Development 中，不参与包身份或摘要。构建产物通过摘要区分，来源 commit 仅表示基础源码。传入 commit 会固定此次下载，即使分支已推进也不会改读最新提交。

源码归档与展开大小各不超过 256 MiB，最多 10000 项、单文件 16 MiB；单次网络请求有三分钟超时。只访问 GitHub API 和官方 HTTPS 下载域，不运行 Git hook、依赖安装或构建脚本。拒绝路径逃逸、大小写冲突、链接和特殊文件。GitHub 故障使用独立本地化 `GITHUB_*` 错误；缺少安装产物返回 `GITHUB_BUILD_REQUIRED`，用户可显式导入工作台构建。

同版本号可以安装不同摘要。未指定固定依赖的新增消费者使用当前安装并检查版本范围；已有游戏存档与平台消费者绑定保留准确发行；普通宿主 Agent 的后续执行采用当前插件，更新不改写会话 journal。

停用只禁止新建和重启，不撤销运行中的凭证；卸载及重新授权会先停止受影响实例。已有存档始终保留准确发行，不能随默认游戏改变而改写。

每个扩展的数据聚合于 plugins/{id} 或 games/{id}，包括 installed.json、settings.toml、releases、previews 和 backups。自管存档位于 games/{id}/instances/{instanceId}；插件 data 按范围隔离。扩展设置与游戏 setup 分开，保存设置保留运行快照，预览不读取已安装覆盖。字段、合并、兼容性与保存协议见[扩展设置标准](extension-settings.md)。

重新安装同一发行可以调整授权，变更前停止受影响实例。平台 Agent 会话不会进入普通写作或 Project 会话入口，避免用其他 Agent 定义继续其历史。

当前新增格式只用于新平台数据，不批量迁移既有 Project 或 Story。v0.4.5 不支持新增 platform_session journal 记录，不能继续这些新平台会话；原有写作、Story 和 Project 数据格式不变。降级前应备份整个数据目录。
