# Denova 插件与游戏平台 API 草案

状态：目标契约，尚无可安装 SDK 或可调用公开路由。更新：2026-09-06；代码核对基线 `8dc5b22b`。

入口：[插件开发手册](plugin-developer-guide.md) · [游戏开发手册](game-developer-guide.md) · [系统设计](plugin-platform-design.md)。本草案按系统设计的 A—C 阶段随实际样例冻结，不保留上一版未发布接口的兼容层。

## 1. 产品与接口边界

- 插件提供 Denova 能力扩展；游戏是游戏页中的具体可玩作品。
- 插件和游戏都可以调用 Consumer API；只有插件向公共目录贡献能力，游戏的私有实现只绑定本游戏范围。
- 可信 Management API 分别管理插件、游戏与开发任务，不授予第三方程序安装／审批自身依赖的权限。
- 两类产物共用构建、安装校验、进程和传输机制，不共用产品身份或存档生命周期。

| 调用者 | 范围 |
| --- | --- |
| B：浏览器视图 | 当前插件面板或游戏视图的受信挂载 |
| S：本地后端 | 当前插件或游戏的受管进程连接 |
| E：能力处理器 | 某次已授权调用的目标范围与所选能力 |
| H：可信宿主 | 安装、授权、导航、选择项目、升级和删除 |

S 可以承载 E，但身份来自启动连接，不能通过 payload 自称另一个插件或游戏。

## 2. 身份、范围和资源

```ts
type JsonValue = null | boolean | number | string | JsonValue[] | JsonObject;
type JsonObject = { [key: string]: JsonValue };
type LocalizedText = { "zh-CN": string; "en-US": string };
type PackageRef = { kind: "plugin" | "game"; id: string };
type ReleaseRef = { package: PackageRef; releaseId: string };
type ContributionId = string; // pluginId/localId; builtin/localId is reserved.
type DefinitionId = string; // A public contribution, or local:id within this game.
type StoryRef = { kind: "story"; projectId: string; storyId: string; branchId: string };
type ProductRef = { kind: "session"; projectId: string; sessionId: string };
type StateOwner = StoryRef | ProductRef;
type GameInstanceRef = { kind: "game-instance"; instanceId: string } | StoryRef;
type ActivationScope = GameInstanceRef | ProductRef | { kind: "project"; projectId: string };
type AgentRef = { owner: StateOwner; sessionId: string };
type CompletionRef = { agent: AgentRef; recordId: string };
type ResourceRef =
  | { kind: "package"; release: ReleaseRef; resourceId: string }
  | { kind: "user"; resourceId: string };
type FileRef =
  | { kind: "project"; projectId: string; path: string }
  | { kind: "package"; release: ReleaseRef; path: string }
  | { kind: "game-data"; instanceId: string; path: string };
type Head = string; // Durable domain commit identity.
type Cursor = string; // Process-local observation position.
type PageRequest = { cursor?: string; limit?: number };
type Page<T> = { items: T[]; nextCursor?: string };
type PlatformError = {
  code: string; messageKey: string; diagnostic: string; messageArgs?: JsonObject;
  fields?: { path: string; code: string; diagnostic: string }[];
};
```

GameInstanceRef 是游玩范围的有限联合：自管实例使用 instanceId，托管实例直接引用既有 Story／分支。`StoryRef.kind` 是新 API 的判别字段，不要求修改已发布 journal 中已有字段值。不为托管游戏另造一份实例权威记录。

PackageRef 仅是安装／资源来源的技术引用，kind 参与身份；不是“通用应用”的产品模型。ReleaseRef 与 GameInstanceRef 分别回答运行什么代码、继续哪一局，不能互换。

AgentRef 指向 Denova 管理的逻辑会话，自管游戏实例本身不是 StateOwner。普通 NPC 根会话使用 Product journal，真正委派的子 Agent 使用自包含 journal；游戏引用它们，不复制 transcript。

所有文件 path 使用 `/` 规范相对路径。game-data 相对于自管实例 data，不开放安装目录、其他实例或 canonical journals。授权来自通道和目标交集，不能省略目标后回退到当前工作区。RPC 仅传可序列化 DTO，大文件走独立传输。

## 3. 两种 manifest

### 3.1 共有字段

| 字段 | 目标形状与约束 |
| --- | --- |
| manifestVersion、id、version、apiMajor | 清单格式、产品稳定身份、发行版本、API 主版本分别版本化 |
| name、description、locales | 独立中英文产品资源；用户内容保留原语言 |
| development | 可选 build／dev 命令配方 `{command, args}`，在开发目录执行；不记录宿主解释器绝对路径 |
| distribution | `{files: string[]}` 包内文件／目录白名单，自动包含 manifest，须覆盖运行引用；未声明时只接收已整理的候选包目录 |
| runtime.backend | 可选 `{launch, protocol: denova-runtime-v1}`；launch 使用 runtime 的 entry／args 或按平台选择 binary |
| views | `{id, source}`；source 为 static 的包内 path 或 backend 的相对路由 |
| modelSlots | `{id, titleKey, kind: text/image, required}[]`；声明用途，用户映射连接 |
| configuration | 可选 `{schema, defaults}` 文件引用，只含非敏感配置 |
| permissions | `{required: string[], optional: string[]}`，权限不因依赖或资源引用转移 |
| requires | `{pluginId, versionRange, contributions: string[]}[]`；只引用插件公开能力，安装解析准确发行 |
| prepare | 可选源码发行准备步骤，展示执行内容和环境要求，检查／安装预览不执行 |

ID 使用小写字母、数字、点和连字符，以字母开头、分段非空，禁止路径分隔符，生成目录还要校验跨平台保留名称。解包拒绝路径逃逸、符号链接、特殊文件与大小写冲突。同产品同版本摘要不同报冲突，预览构建不覆盖正式发行。

一个候选包根目录必须且只能有 denova.plugin.json 或 denova.game.json。两类产品分别版本化；同仓库可分别打包，安装不能根据 display name 或 tags 猜测类型。

### 3.2 PluginManifest：公共能力扩展

完整最小示例见[插件手册](plugin-developer-guide.md)。领域字段：

| 字段 | 内容 |
| --- | --- |
| contributes.agents / tools / toolsets / skills / contexts | Agent 定义、工具 schema、工具集合、Skill 资源与有界上下文 |
| contributes.flows / planners | 可复用动作与规划协议及输入输出 schema |
| contributes.panels | `{id, titleKey, placement, viewId}`；writing-panel、chat-panel、game-panel、tool-result、settings |
| contributes.stateSchemas | 为可选择的 Product Session 扩展声明托管 schema 与初值 |
| contributes.connections | 命名外部连接范围和方法，不包含凭证 |
| resources | 随能力附带的默认内容、模板和素材，不单独形成游戏目录项 |
| runtime.extension | `{kind: module, entry}`，或 `{kind: backend}` 共用插件后端 |

插件可以是静态 Agent／Skill、纯前端面板或后端能力。后台工具和上下文不能依赖某个面板持续打开；需要此类 handler 时在独立提供器进程注册。一个插件激活最多一个后台进程，module 不和独立 backend 重复声明。

插件清单不接受 applications、games 或 gameModes 贡献。能力目录中的一个 flow 是可复用算法，不是可玩的游戏作品。

### 3.3 GameManifest：一部作品

完整纯前端示例见[游戏手册](game-developer-guide.md)。

```ts
type GameDeclaration = {
  viewId: string;
  setupSchema?: string;
  uses?: {
    agents?: ContributionId[]; toolsets?: ContributionId[];
    contexts?: ContributionId[]; flows?: ContributionId[]; planners?: ContributionId[];
  };
  storage:
    | { kind: "self" }
    | {
        kind: "denova"; schema: string; initialState: string;
        flow: DefinitionId; planDocument?: string;
      };
};
```

`game` 是一个 GameDeclaration。`definitions` 可声明本游戏私有 agents、tools、toolsets、skills、contexts、flows、planners；字段结构与相应能力描述复用，引用使用 local:id，只在本游戏解析。私有 handler 由游戏后端绑定；纯前端也可以只调用内置／插件能力或直接提交授权状态。

`resources` 描述游戏角色、场景和素材，用 `format + formatVersion` 声明语义。游戏配置不向公共贡献目录写入任何条目，game.uses 对其他插件的引用必须被 requires 覆盖。纯内容导入走资源流程，不因有若干 JSON 文件就注册插件或游戏。

模型槽的有效身份包含提供方与槽 ID。创建使用范围时冻结实际模型连接映射、定义、资源和依赖；游戏自己的 writer 与插件的 writer 不隐式共享配置。配置升级使用明确记录，不随全局偏好漂移。

## 4. 开发、安装与游戏管理（H-only）

### 4.1 共用开发机制

| 方法 | 输入／结果 |
| --- | --- |
| development.create | `{kind: plugin/game, templateId, projectId, relativePath, id, name}` → 开发绑定与创建文件；目录须为空，新 Project 复用现有创建流程 |
| development.link / list | 关联已有项目相对目录／列出对应 kind 的开发项 |
| development.check | `{developmentId}` → 带文件和字段位置的静态检查，不执行代码 |
| development.build | 明确配方 → 可见、可停止的构建任务 |
| development.preview | 开发项及明确测试目标 → 插件测试激活或游戏测试开局；不同 kind 的参数与目标分别校验 |
| development.stop / reset | 停止；或预览范围后备份并重置测试数据，不碰源码和正式存档 |
| development.pack / export | 冻结产物快照 → 准确候选包与文件清单 → 导出文件，不自动对外发布 |

开发绑定记录稳定 ProjectID 和相对目录，测试目标使用既有资源引用。诊断、进程、开发 URL、依赖环境属于设备运行时。需要项目能力时选择测试 Project，不默认授权源码或当前书籍。原生代码不因此成为沙箱代码。

准备、构建、打包、安装是不同动作。prepare 改变候选字节后重新校验，最终权限和产物需重新展示；安装消费最终预览所指同一内容。运行中前端可开发热更新，后端／定义／权限变化先停止旧激活，生成新的预览绑定；正式发行不引用开发 URL。

### 4.2 插件管理

| 方法 | 输入／效果 |
| --- | --- |
| plugins.previewInstall / install | GitHub URL/ref/subdir、本地包或 candidateId → 固定插件来源／权限预览 → 安装相同字节 |
| plugins.list / get | 已安装插件、公开贡献、可用性及使用方 |
| plugins.configure / setEnabled | 配置与授权或启用状态，生效范围明确；禁用影响可见 |
| plugins.previewUpdate / installUpdate | 新发行可选，已有游戏与会话保持旧绑定 |
| plugins.uninstall | 停止激活、撤销公开贡献，保留用户数据及仍被引用的发行 |

### 4.3 游戏目录与游玩空间

| 方法 | 输入／效果 |
| --- | --- |
| games.catalog.previewInstall / install | 游戏来源 → 游戏产物、依赖和权限预览 → 安装同一内容；插件依赖分别授权和记录 |
| games.catalog.list / get | 可玩的游戏作品和准确发行，不返回工具、角色卡或通用引擎 |
| games.catalog.previewUpdate / installUpdate / uninstall | 管理游戏代码，卸载不级联卸载共享插件或默认删除存档 |
| games.instances.create | `{gameId, releaseId, title, setup, projectId?}` → GameInstance；按存储方式创建自管实例或既有 Story，托管方式必须有 projectId |
| games.instances.list / get / open / stop | 列出、读取、继续或停止某个 GameInstanceRef |
| games.instances.upgrade | 目标游戏发行、依赖与配置 → 停止、备份、显式迁移和原子领域绑定更新 |
| games.instances.export / remove | 导出或明确删除游玩数据；数据库先停止或使用可靠快照，删除提供恢复路径 |
| games.instances.fork / rewind | 仅对已验证支持的存储实现开放；托管使用 Story 提交和 NPC 完整前缀恢复 |

```ts
type GameInstance = {
  ref: GameInstanceRef; gameId: string; releaseId: string;
  title: string; projectId?: string;
  dependencies: { pluginId: string; releaseId: string }[];
};
type RuntimeStatus = "stopped" | "starting" | "running" | "stopping" | "failed";
```

这是读模型，不要求一张统一实例表。自管绑定的权威位置为该实例元数据；托管绑定的权威位置为原 Story journal，目录索引可重建。实例升级不得只更新可丢弃索引，也不能一边修改 instance.json、一边修改 Story 作为双份权威事实。

插件配置页、游戏详情和开始／继续是不同 UI。宿主可将游戏及所需插件的安装编排成一个向导，仍逐项显示来源、执行性质和权限，不静默安装或扩大授权。

## 5. 启动与客户端

本地进程通过临时 bootstrap 通道接收准确来源、scope、packageDir、可写 dataDir（如果该运行拥有自有数据）、tempDir、监听信息和授权连接。凭证不在 argv、URL 或日志中；绝对路径只用于当前运行。

游戏后端与插件后端都是进程协议使用方，各归自己的身份和范围。宿主验证 readiness、监听地址和通道后装载视图／登记 handler。游戏的业务 HTTP／WebSocket 路由通过专用来源代理，不强制改写为平台 RPC。

浏览器模板验证来源、挂载身份和一次性握手后接收 MessagePort：

```ts
type RuntimeContext = {
  source: ReleaseRef; scope: ActivationScope; viewId?: string;
  locale: "zh-CN" | "en-US"; theme: "light" | "dark";
  environment: "preview" | "installed"; configuration: JsonObject;
};
type Capability = {
  id: string; status: "available" | "not_configured" | "denied" | "unsupported";
  reasonKey?: string;
};
```

`platform.connect(port)` 返回 `{api: PlatformClient, context: RuntimeContext}`。PlatformClient 是拟定有类型客户端名，不是目前已发布的 SDK。语言／主题变化由同一受信通道通知；权限或目标变化重建连接并撤销旧通道。

| 方法 | 调用方与效果 |
| --- | --- |
| platform.describe | B/S/E → 当前范围内能力状态、请求／文件／状态上限、schema dialect；容量需用真实样例校准，超限不截断 |
| ui.exit / openView | B → 返回相应目的地／打开同一产物已声明授权视图 |
| ui.requestProject | B 的用户操作 → 可信项目选择和授权，返回 projectId 并重建范围连接 |
| runtime.snapshot / stop | H；B/S 可查看或请求停止自己的运行，不能停止别的游戏 |

建议传输为 `/api/platform/rpc` 上的 `{requestId, method, params}`，认证在头部或连接握手，流与大文件另设通道。游戏与插件分别路由到同一授权语义，不直接暴露现有内部 API。

没有 SDK 的网页仍可被运行容器托管；要使用 Denova 或插件能力时接入客户端。远程页面需遵守嵌入策略，远程后端的直接授权和网络可达性另行设计，不因嵌入 URL 自动开放本机 API。

## 6. Agent、工具与内容的调用

### 6.1 定义与运行

AgentDefinition 组合 instructions、modelSlot、tools／toolsets、skills、contexts 和 delegation。公共贡献来自内置或插件；local:id 只能解析为调用游戏自己的定义。角色卡是输入内容，不是任意工具授权。

| 方法 | B/S/E 输入 → 返回 |
| --- | --- |
| agents.definitions | 分页列出当前调用方可选定义；游戏私有定义不泄露到其他使用方 |
| agents.sessions.ensure | `{projectId, definition, key, content?: ResourceRef[], parent?: AgentRef}` → `{ref: AgentRef, definition, key}` |
| agents.sessions.snapshot / history | agent → 当前快照／分页显示消息 |
| agents.runs.start | `{agent, commandId, input: {text, attachments?}}` → RunRef |
| agents.runs.find | `{agent, commandId}` → 既有结果／状态或 null |
| agents.runs.wait / abort | RunRef → 终态等待／停止请求；取消网络等待不等于停止运行 |
| agents.runs.steer / queue / cancelQueued / followUp | 显式当前运行控制；排队句柄仅进程内有效，不承诺重启重放 |
| agents.respond | `{run, interactionId, answer}`；普通问答可代理，平台权限只能由 H 批准 |

ensure 的稳定范围来自认证通道中的调用产品身份、持久目标、projectId 与 key，不使用 activationId、releaseId、路径或端口。创建来源和准确定义写 Session canonical 配置记录，索引可重建；同 key 的定义变化要求显式配置变更。不同开局中的同名 NPC 不共享会话。

一个逻辑 Session 一次只有一个主动 Run；不同 NPC 可并行。调用跟随游戏／插件激活和动作范围取消。只有真实委派填写 parent，普通游戏使用多个独立 NPC 不需要伪造委派树。

```ts
type RunRef = { agent: AgentRef; runId: string };
type WorkStatus = "accepted" | "running" | "waiting" | "completed" | "failed" | "aborted" | "incomplete";
type RunResult = {
  run: RunRef; status: WorkStatus; text: string;
  completion?: CompletionRef; error?: PlatformError;
};
```

公开 start 在所属 canonical journal 写 accepted 后才执行，终态关联同 commandId。当前 Agent 的进程内 CommandReceipt 不等于跨重启去重。相同 ID 和内容返回原执行；不同内容报 IDEMPOTENCY_CONFLICT。accepted 后崩溃未有终态则 incomplete，不自动重跑未知副作用。CompletionRef 是完整持久前缀，不能用流 cursor 替代。

### 6.2 资源与发现

| 方法 | 效果 |
| --- | --- |
| contributions.list | 当前可见公共能力及准确依赖；不列其他游戏私有实现 |
| resources.list / read | format 过滤和读取授权角色、世界、场景、模板、Skill 或资产引用 |
| resources.import / copy / update | H 操作用户资源副本，校验 format、版本与依赖，不执行脚本 |

内容支持情况由实际消费者的格式契约决定，不靠“角色卡”标签猜测。游戏／会话创建时冻结实际资源内容与发行；修改全局默认资源不会无记录地改变既有角色记忆。

## 7. 公共贡献与游戏私有实现

```ts
type ToolResult = { content: string; data?: JsonValue };
type ContextFragment = {
  source: string; purpose: string; resource: string;
  stability: "stable" | "session" | "turn"; content: string; maxBytes: number;
};
type ProviderCall = {
  scope: ActivationScope; signal: AbortSignal; agent?: AgentRef;
  api: PlatformClient;
  action?: { owner: StateOwner; stage(changes: StateChange[]): void };
};
type ImplementationBindings = {
  tools: { register(id: string, handler: (call: ProviderCall, input: JsonValue) => Promise<ToolResult>): void };
  contexts: { register(id: string, handler: (call: ProviderCall, input: JsonValue) => Promise<ContextFragment[]>): void };
  flows: { register(id: string, handler: (call: ActionContext, input: JsonValue) => Promise<FlowOutput>): void };
  planners: { register(id: string, handler: (call: ActionContext, input: JsonValue) => Promise<PlanProposal>): void };
  onDispose(handler: () => void | Promise<void>): void;
};
type ExtensionContext = ImplementationBindings;
type GameImplementationContext = ImplementationBindings;
```

同形类型复用 handler 语义，不表示权限相同。插件入口 activate 的注册上限是 contributes；游戏后端入口 bind 的上限是 definitions，且仅在本游戏解析 local:id。能力身份和可见性由启动通道确定，客户端无法切换两种角色。

函数留在作者进程，SDK 将其绑定为有类型 RPC handler，不跨边界传函数对象。重复 ID、缺失实现或非法依赖导致激活失败；停止撤销注册并清理资源。没有动态 handler 的纯前端游戏只需 Consumer API。

工具定义为 `{description, inputSchema, outputSchema?, effect, titleKey?}`。输入校验给出字段错误，可选 outputSchema 校验 ToolResult.data；effect 为 pure／read／propose／write，表达效果而非授权。英文模型说明与用户本地化标题分开。

工具集显式选择、去重和冻结；静态资源和 Skills 进入同一个有效 catalog。ContextFragment 来源、用途、稳定性与上限必须验证，超限报错，不能任意裁掉作者内容。动态世界状态不反复重写稳定缓存前缀。

有动作上下文时状态工具只能 stage 候选变更；没有动作时不能伪造该上下文。普通工具获得 AgentRef 不等于有权访问其全部数据。自己的网络与数据库副作用不包含在宿主 journal 事务中。

## 8. 游戏数据与托管动作

### 8.1 自管数据

| 方法 | 当前授权范围内的输入／效果 |
| --- | --- |
| gameData.list / read | 自管游戏开局的 path → 文件列表或 `{content, revision}` |
| gameData.write / remove | path、内容及 expectedRevision → 冲突校验后写入或删除 |
| pluginData.list / read / write / remove | 当前插件在明确 scope 的非会话服务数据；同样执行路径、授权和 revision 校验 |

调用通道决定产品和数据范围，不能传另一个 instanceId／pluginId 越权；插件读游戏数据需另有当前游戏的明确授权。expectedRevision 为 null 表示只允许新建，旧 revision 冲突不能静默覆盖。大文件走资产传输。

gameData 仅在自管实例可用；托管方式使用 Story 状态，不能额外开一个 data 镜像作为恢复源。pluginData 可保存词典等插件服务数据，禁止作为 Denova transcript、恢复状态或同一游戏状态的第二权威副本。实现复用文件服务即可，不新增版本链或通用数据库。

原生后端可直接管理自己的 dataDir，并自行负责数据库事务；不能让文件 API 同时覆写正在打开的数据库。自管数据与 Denova NPC 会话默认分别提交。

### 8.2 托管状态与动作（阶段 C）

| 方法 | B/S/E 输入 → 返回 |
| --- | --- |
| state.read | `{owner, documents?}` → StateSnapshot，按授权 namespace 投影 |
| state.commit | `{owner, commandId, expectedHead, changes}` → ActionSnapshot；无模型步骤的直接原子提交 |
| actions.start | `{owner, flow, commandId, expectedHead, input}` → ActionSnapshot |
| actions.get / find / wait / cancel | owner 与 actionId／commandId → 状态、查询、等待或取消 |
| planning.read / propose / commit | 当前计划、按选定 planner 产生建议、显式采纳；沿用 owner 的动作与去重边界 |

```ts
type StateSnapshot = { head: Head; documents: JsonObject };
type StateChange = { op: "put"; document: string; value: JsonValue } | { op: "remove"; document: string };
type ActionSnapshot = {
  owner: StateOwner; actionId: string; commandId: string; status: WorkStatus;
  commitHead?: Head; result?: JsonValue; error?: PlatformError;
};
type FlowOutput = { changes?: StateChange[]; result: JsonValue };
type PlanProposal = { content: JsonValue; schemaVersion: number };
type ActionContext = {
  owner: StateOwner; actionId: string; signal: AbortSignal;
  api: PlatformClient;
  state: { read(): Promise<StateSnapshot> };
  stage(changes: StateChange[]): void;
  agents: {
    ensure(input: { definition: DefinitionId; key: string }): Promise<{ ref: AgentRef }>;
    start(input: { agent: AgentRef; input: { text: string } }): Promise<RunRef>;
    wait(input: { run: RunRef }): Promise<RunResult>;
  };
  adopt(completion: CompletionRef): void;
  progress(value: JsonValue): void;
};
```

owner 必须已经绑定 schema。Game 的 schema 在 Story 创建时由游戏声明冻结；Product 扩展按选定定义冻结。使用成熟 JSON Schema dialect，禁止远程 $ref；先支持 document 替换，删除显式 remove，大素材保存资产引用。

一个 owner 一次一个写动作；同 owner 的嵌套 start 被拒绝，动作中的独立 state.commit 返回 OWNER_BUSY。去重先于 head 校验，accepted 在副作用前持久化，成功 committed 一次保存正式结果、状态、规划及采纳的 NPC 前缀。纯 state.commit 直接用提交结果去重。

ctx.api 的调用继承动作授权与取消范围，不能借此重入同 owner 的公共动作；文件、网络等副作用仍不纳入状态事务。ctx.state.read 读取候选视图，外部 state.read 读取正式结果。并行修改同一 document 明确报冲突，不按返回时序决定胜者。handler 完成前收束所有工作，并 adopt 采纳的会话位置；schema 失败、取消或异常不提交候选状态。

规划模板是资源，planner 是算法，已采纳计划是 owner 数据。默认小说 RPG 仍以 BranchPlan 为唯一计划事实，新游戏使用声明的 planDocument；不另建规划数据库。未配置的 planning 返回 UNSUPPORTED，Agent Goal/Todo 不是游戏剧情规划。

fork／rewind 从提交位置建立自包含 NPC 会话前缀，恢复 Compaction、Goal/Todo 等，不截断旧 journal、不共享可写未来。失败未采纳的会话尾部必须隔离。前缀恢复未通过验收时拒绝完整回档，不仅回退界面数值。跨游戏自有数据库的完整恢复需专门适配，不由这一组 API 自动保证。

## 9. 写作、图像和外部连接

| 方法 | 调用与授权边界 |
| --- | --- |
| files.list / read / propose / write | 显式 ProjectID 与相对路径；提案与直接写入分权，写入验证 revision |
| editor.context / propose / openReview | 受信挂载的文档和选区、草稿标识、baseRevision；复用原生审阅，不暴露编辑器内部对象 |
| connections.status / request | 命名连接的域名、路径与方法范围，宿主注入凭证，重定向重新校验 |
| assets.beginUpload / finishUpload / url | 获准目标上传 → FileRef → 临时 URL，URL 不作为持久引用 |
| images.generate / get / find / cancel | 明确 projectId、modelSlot、commandId、prompt 与引用 → 可查询取消的任务和 FileRef |

图像生成的 commandId 必须在响应丢失和重启后可查询；具体领域适配待实现，不能因为已有生图路由就宣称公共幂等任务可用。磁盘内容与未保存草稿分别校验，冲突保留提案。文件 API 不开放 journals、安装记录或宿主凭证。

语音录制通过用户手势和系统授权启动，关闭视图停止采集。游戏或插件可组合自己的语音服务；原生 STT/TTS／实时双向语音暂不冻结完整签名。远程后台和多人联机也不属于当前 API 默认能力。

## 10. 事件、停止与权限

`events.subscribe({target, after?})` 原子提供 `{snapshot, cursor}` 和后续事件，unsubscribe 释放订阅。目标为当前可访问的 Run、动作、游戏运行或插件激活；不得广播其他 NPC 的私有历史和其他游戏状态。cursor 过期返回 CURSOR_EXPIRED 并重取快照。

默认退出游戏停止该局及仅由它持有的插件激活。后台继续需声明 activity.run、明确选择并在宿主运行列表可见；插件长期活动仍绑定其目标范围。停止顺序为拒绝新调用、传播取消、收束清理、终止本地进程树、撤销通道。远程取消不保证终止远端程序。

Agent 默认无总时长或迭代硬限制；用户可配置实际需要的预算。跨重启不重放未完成的副作用，实时流和日志不是恢复事实。

| 权限 | 有效范围 |
| --- | --- |
| agents.run | 获准角色与目标会话，不扩大工具权限 |
| gameData / pluginData | 当前实例或插件 scope 的自有文件 |
| state.read / state.write | 已绑定 owner、namespace 和 schema；动作内写入为 stage |
| resources.read | 授权资源，私有游戏定义不公开给其他游戏 |
| files.read / propose / write、editor.read / propose | 项目内容或挂载草稿，提案与直接写入分开 |
| connections.request / images.generate / media.capture | 声明连接、模型用途或当前视图的系统授权 |
| activity.run | 可见、可停止的后台活动 |

有效权限为来源申请、用户授权、目标范围、实际角色／能力选择的交集。UI 隔离、旧 API、代理与本地服务都需校验；API facade 不约束原生程序本来具有的 OS 文件和网络权限。

错误至少区分 INVALID_ARGUMENT、LIMIT_EXCEEDED、PERMISSION_DENIED、NOT_CONFIGURED、UNSUPPORTED、RUNTIME_UNAVAILABLE、DEPENDENCY_UNAVAILABLE、SESSION_BUSY、OWNER_BUSY、STATE_CONFLICT、DOCUMENT_CONFLICT、IDEMPOTENCY_CONFLICT、CURSOR_EXPIRED、RUNTIME_FAILED。用户错误使用独立中英文 messageKey，英文 diagnostic 包含产品、发行、范围和 run/action 身份，不含凭证。

## 11. 冻结前的行为验收

1. 插件安装只增加可选扩展；游戏安装只增加可玩作品；纯内容不混入任一代码目录。
2. 同一个插件可供写作和多部游戏选择；游戏私有定义不外泄、不覆盖公共贡献。
3. 纯前端游戏可用平台＋插件运行并保存；自定义前后端游戏保持自己的业务协议，二者均可开始、退出、继续。
4. 自管实例不强制创建空 Session／Story；托管实例复用 Story，所有绑定与恢复状态只有一个事实源。
5. 同一请求跨刷新和重启可查且不重复执行；状态失败不采纳，完整回档含 NPC 前缀恢复。
6. 停一局不停止其他局；卸载游戏不移除共享插件；禁用插件显示影响并停止依赖工作。
7. 游戏发行与插件依赖准确固定，迁移失败恢复代码和数据；被存档引用的发行不静默清理。
8. 两类产物在 App 中分别开发、预览、检查、打包和安装；macOS、WSL、Windows 原生分别验证声明支持的进程与路径。

当前源码复用点与阶段出口见[系统设计](plugin-platform-design.md)。所有接口均需实际样例和领域适配验证，不能直接把现有内部路由改名后宣称平台开放完成。
