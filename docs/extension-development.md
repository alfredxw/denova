# Denova 扩展能力开发手册

扩展通过普通 HTTP 组合 Denova 的能力。游戏拥有完整内容区，可以自行实现聊天、视觉小说、地图或其他交互。宿主负责项目身份、授权、模型配置、执行和持久化；具体角色、分镜、立绘表情、规则与资源选择由扩展定义。

本文说明游戏扩展如何组合平台能力。平台负责原生执行、持久化和权限，扩展负责玩法、演出和作品格式。完整端点见 [HTTP API](plugin-platform-api.md)，安装与工具发布见 [插件手册](plugin-developer-guide.md)，游戏交付见 [游戏手册](game-developer-guide.md)。运行实例的 `/openapi.json` 是精确契约，`/capabilities` 返回本次授权与容量限制。

## 1. 选择最小实现

| 需要交付的内容 | 使用方式 |
| --- | --- |
| 写作、工作台和游戏共用的工具或服务 | `denova.plugin.json`，公开工具与工具集 |
| 自己管理世界状态与交互的作品 | `denova.game.json`，`game.storage.kind: "self"` |
| 使用现有互动故事，重做画面与操作 | 同一游戏清单，`game.storage.kind: "story"` |

静态 HTML/CSS/JavaScript 足以调用全部消费者接口。需要已有数据库或服务端程序时，再增加 Node 后端。无需新增一种插件类型、继承宿主组件或修改宿主 DOM。私有 Agent 留在游戏包中；跨作品共享的工具可提取为插件。

在「扩展 → 创建游戏」填写名称后进入工作台，项目自动生成通用开发骨架。共享连接代码位于 `extensionassets/sdk/client.mjs`。修改源码后点击「试运行」，配置模型与测试参数并启动隔离运行。「发布到本机」安装经过检查的冻结候选包；源码变化后重新检查。平台不依赖具体游戏引擎，独立引擎通过普通扩展安装流程接入。

复用 Story 的预览需要先打开一个书籍项目；源码可以来自另一个通用项目。预览在该书籍项目中创建独立 Story，不关联已有 Story，也不改变普通故事列表的当前选择。执行剧情命令时，该书籍项目必须仍在前台；扩展不会代替用户切换项目。

## 2. 声明需要的能力

下面是复用 Story 的最小清单片段；入口及本地化文件也必须加入 `distribution.files`。

```json
{
  "game": {
    "viewId": "stage",
    "storage": { "kind": "story", "saveFormat": "my-presentation-v1" },
    "story": { "modelSlot": "writer" }
  },
  "modelSlots": [
    { "id": "writer", "kind": "text", "titleKey": "writer", "required": true },
    { "id": "illustrator", "kind": "image", "titleKey": "illustrator", "required": false }
  ],
  "permissions": {
    "required": ["stories.read", "stories.write", "assets.read"],
    "optional": ["library.read", "images.generate"]
  }
}
```

`story.modelSlot` 必须引用文本槽，用于新 Story；关联已有 Story 不替换其生成配置。`local:writer` 是宿主保存模型选择时使用的绑定键。图像调用中的 `modelSlot` 使用 `illustrator`，宿主解析已绑定的图像配置；模型密钥始终留在宿主。槽位可选表示作品能在未配置图像模型时继续游玩，图片按钮应说明如何配置。

权限按实际读取或操作划分，不能通过 body 更换 Project、Story 或另一游戏实例。资料库条目是内容，不携带工具授权。Node 后端具有当前系统账户权限；上述范围限制属于平台 HTTP 接口，并非操作系统沙箱。

创作型扩展可按需声明 `library.write`、`assets.write`、`agents.run` 和 `settings.read/write`。私有 Agent 可用于资料与草稿创作；采用原生 Story 的游戏仍由原生 Story 执行剧情。可选权限关闭后，界面应保留不依赖该权限的操作。

可选权限拒绝以公开接口的 `PERMISSION_DENIED` 为准，不从作品数据推断授权。生成内容只保存为可用草稿，用户明确选择采用后才尝试写入原生库；失败保留草稿及已完成命令，重试采用不再次调用 Agent，也不覆盖生成后用户编辑的内容。拒绝图片生成时停止自动补图，手动重试仍使用原稳定命令。用户从宿主「扩展 → 对应扩展 → 设置 → 权限」保存授权后重新启动，不由作品自行扩大权限。

## 3. 连接与完整自定义界面

模板的 `client.mjs` 验证父窗口、来源与 nonce，再取得短期连接。`context` 包含绑定范围、准确发行、语言、主题、设置和开局参数。

```js
import { connect } from './client.mjs';
const client = await connect();
const story = await client.request('/story');
// Render story.turns using your own DOM, canvas, animation or input controls.
```

响应包含正文、选项、回合修订、分支、运行状态、原生状态 schema 和开局配置。状态面板按 schema 与已提交变化展示；作者秘密、尚未揭示的设定和运行诊断不应直接呈现在玩家界面。模型密钥与宿主内部路径不通过接口提供。监听 `denova:appearance` 后更新当前单一语言和主题。自由输入、点选、逐句播放、自动播放和快捷键由游戏界面实现；页面必须保留停止和错误恢复入口。

## 4. Story 是唯一剧情事实源

读取 `/story` 获得近期回合，需要更早内容时按 `/story/history` 游标分页。通过 `/story/commands` 执行推进、停止、恢复、重新生成、分支和版本切换，仍经过原有 Story 执行与提交过程。

```js
// Persist this exact intent before sending; reuse it after a lost response.
const command = { kind: 'advance', commandId: crypto.randomUUID(),
  branchId: story.branchId, message: 'Open the envelope.', locale: client.context.locale };
await savePendingCommand(command);
await client.request('/story/commands', { method: 'POST', body: JSON.stringify(command) });
// Subscribe to /story/events?operationId=... for provisional display.
// Reload /story on settlement or disconnect; never repeat generation on reconnect.
```

`savePendingCommand` 是作品自己的存档操作，可用下一节的扩展记录实现。`stop` 必须携带观察到的 `operationId`，避免误停另一个执行。读取旧回合与切换阅读位置不生成新剧情。重新生成和分支仍由 Story 管理，作品不能自行复制 JSONL 当作存档。

### 保存扩展数据

`/story/records` 保存扩展自有 JSON。宿主自动按包身份隔离，不理解其中的角色、场景或游戏规则。未写入过的记录返回 `revision: 0, value: null`；修改必须携带上次读到的 `expectedRevision`，冲突后重新读取并协调。

```js
const turn = story.turns.at(-1);
const address = { key: 'presentation', branchId: story.branchId,
  turnId: turn.id, sourceRevision: turn.revision };
const previous = await client.request('/story/records?' + new URLSearchParams(address));
await client.request('/story/records', { method: 'PUT', body: JSON.stringify({
  ...address, expectedRevision: previous.revision, schemaVersion: 1,
  value: { line: 0, artwork: {} },
}) });
```

`turnId` 与 `sourceRevision` 将数据绑定到确切正文。重新生成、切换版本或分支后，不得把旧分镜套到新正文。省略回合地址的记录适合角色选择等 Story 级数据，它不会自动随剧情回退；需要按分支/回合恢复的状态应使用精确地址。JSON schema 和迁移规则由作者版本化，记录容量有限，超限报错，不截断内容。

游戏绑定、开局配置与这些记录均写入同一 Story journal；游戏列表只是投影，不新增 `instance.json`。原来的互动故事仍可打开该 Story。解除游戏绑定保留正文。创作 Agent 的对话与执行写入所属原生会话 journal，作品只存请求和完成引用。呈现层不应复制原生聊天历史；平台不承诺多个独立 Agent 会话的原子回档。

`GET /story/state?branchId=...&turnId=...` 读取历史已提交状态，不改变继续点；`GET /story/presets` 发现状态、规则、事件、规划、叙事和图像预设。`PUT /story/configuration` 只允许未开始且没有额外分支的 Story，沿用原生初始化、校验和持久化。角色的演出 ID 与原生 Actor ID 分开，字段按 schema 中的精确名称读取。显示好感数值不会额外建立一份关系账本；CG 条件只选择画面，不结算奖励。

`GET /story/scene?branchId=...&turnId=...&sourceRevision=...` 是 owning Story 视图的只读精确场景契约，返回 `{turn, previousTurn?, state}`；`state.sourceRevision` 与 `turn.revision` 相同。它用于读取该分支已记录、但可能不再处于当前继续路径的生成版本，不执行 `switchBranch` 或 `switchVersion`。正文编辑导致 revision 不匹配时拒绝，不把新正文与旧图混合；不能借子分支读取未继承的父分支未来回合。扩展采用素材仍由精确地址的既有 `/story/records` GET 获取。

作品携带选定预设快照。状态和规则通过 `statePreset`、`rulePreset` 冻结到 Story；其他四种预设通过 `POST /story/presets/import` 采用为内容寻址的原生副本，再绑定返回的本地 ID。导入不覆盖接收方已有同名预设，也不依赖作者的预设 ID。

## 5. 资料库与图像组合

`GET /library/items?query=...&limit=50` 获取当前项目的分页摘要，`GET /library/items/{id}` 获取选中的完整内容。扩展可冻结选定资料的内容和来源 revision，并将采用的基础图片复制到自己的项目内容作用域，使原库的编辑或删除不改写已采用的作品素材。

`POST /library/items` 需要 `library.write`，输入 `{item, baseRevision?, sourceName?, sourceId?, sourceHash?}`。`item.id` 使用原生支持的稳定字母、数字、`-` 或 `_`；重复采用相同内容复用现有条目。修改必须提供读到的 `updatedAt` 作为 `baseRevision`，宿主先备份旧条目，再调用原生 revision 校验保存。图片绑定沿用原生图像入口，本接口不覆盖图片。生成中断后按原条目 ID 逐项采用，不以同名重复条目掩盖失败。

资源使用 `{kind: "project" | "generated" | "shared", path: "relative/path.png"}`。请求 `/assets/content` 返回经授权的字节；`project` 只允许受支持的原项目 `assets/` 图片，`generated` 只读取当前实例生成的图片，`shared` 读取同一 Project、同一扩展共享的图片或音频。`POST /assets/upload` 以原始字节上传并返回内容寻址的 `shared` 引用，需要 `assets.write`。同一内容重复采用不会分配另一个素材身份。扩展自带素材由其冻结发行提供；自定义作品导出应包含所用素材，不依赖作者环境中的临时路径。

```js
const response = await client.request('/assets/content?' + new URLSearchParams(asset),
  { responseType: 'stream' });
const url = URL.createObjectURL(await response.blob());
imageElement.src = url;
// Revoke the object URL when replacing/unmounting the image.
```

只持久化 `asset` 引用，不能保存 bearer token、绝对路径、临时端口或 blob URL。图像生成前持久保存命令，再提交：

```js
const result = await client.request('/images/generations', {
  method: 'POST', body: JSON.stringify({ commandId, modelSlot: 'illustrator',
    prompt: 'A quiet station at dusk, watercolor illustration, no text.',
    size: '1536x1024', outputFormat: 'png' }),
});
// Query /images/generations/{commandId}; save completed images[].asset.
```

每个命令生成一张图片。可选输出参数为 `size`、`aspectRatio`、`resolution`、`quality`、`outputFormat`，均是字符串。`resolution` 使用原生提供商支持的档位（如支持时的 `1K`／`2K`）；`outputFormat` 沿用原生 `png`、`jpeg`／`jpg`、`webp` 格式处理。省略或空字符串使用所选图片配置的默认值；扩展边界原样转发，具体支持、规范化、校验和尺寸优先级仍由原生服务及提供商适配决定，不保证每种协议都会采用每个参数。

请求状态包括 `running/completed/failed/cancelled/interrupted`；停止使用 `/cancel`。相同 ID 和相同输入查回原结果，不再次付费；相同 ID 更改输入（包括分辨率或格式）报冲突。进程中断后无法确认结果时返回 `interrupted`，由用户明确重新生成并创建新 ID，禁止自动重放。旧请求省略新增参数时仍能使用原 receipt；原生服务判定失败时沿用现有失败结果，扩展适配层不添加新的默认值或替代规则。

`GET /assets/documents` 列出共享创作 JSON 文件；`GET /assets/document?path=...` 返回内容与 revision；`PUT /assets/document` 使用 `{path, content, expectedRevision}` 进行 CAS 写入，`expectedRevision: null` 只创建。每份文档最多 512 KiB，文件名必须是规范相对 JSON 文件名。它们是作品草稿和冻结内容产物，不保存第二份 Agent 对话或剧情结算。正在生成的素材、创作 Agent 和新周目创建仅保存原命令／会话引用；跨 Story 打开的草稿不会自动重放原作用域的付费任务。

创作 Agent 的 `accepted/running/waiting` 都不是完成状态，扩展应持续读取同一 commandId；`waiting` 表示原生待交互并保留停止入口。只有 `completed` 才解析结果。格式失败、未知状态和挂起不应清除原请求引用，避免重复付费生成。

## 6. 平台与扩展的职责边界

平台提供包校验、冻结、安装、权限、模型绑定、共享客户端、设置与实例生命周期；应用适配器向扩展开放原生 Story、资料库、预设和资源能力。平台不解释角色卡、演出指令、镜头、阅读位置或作品 ZIP／PNG 协议，这些属于具体扩展。

代码上，`internal/platform` 管理扩展协议与运行容器，`internal/app/platform` 负责原生能力的协议转换和数据投影。根 `app` 的 `platform_host.go` 只连接配置快照、项目租约和原生 Story 服务；工作区锁、任务准入与提交校验仍由原生应用层持有。子 package 不反向导入 `app`，也不依赖具体扩展。

`GET /story/scene` 只查询精确历史正文及原生状态，不返回 CG 或鉴赏进度。作品前置条件、原子成本和一次性奖励不能靠提示词或禁用按钮保证；平台目前没有扩展可配置的原生提交前校验契约。演出指令若写入正文，原生回看与导出也会保留，不应宣称已有独立演出元数据提交能力。

`POST /assets/source` 提供受限的公共 GitHub 下载；仓库来源解析为实际 commit，Release ZIP 需要明确摘要。端点只下载，不安装内容或执行脚本。作品包格式、完整性校验和素材闭包由扩展负责。

`POST /story/instances` 使用稳定 commandId 创建同项目、同游戏发行及模型绑定的新 Story。扩展通过 `client.openInstance(instanceId)` 请求宿主打开已验证的实例。旧 Story 保持原发行和内容绑定，草稿修改不应隐式升级已有进度。

## 7. 发布前验证

检查纯文本模型和图像模型的选择、权限不足、资料库为空、长正文和长角色描述；覆盖双语、两种主题和窄屏。验证刷新、停止、响应丢失、重启、相同命令查回、记录写冲突，以及分支和正文修订后不复用未来数据。先使用本地确定性模型，再按需验证真实提供商。

继续保留原自管游戏与插件工具的调用链。需要更新数据格式时保持明确版本、备份和恢复路径，不自行改写宿主管理目录。发布只包含运行白名单中的文件；源码、私密配置、诊断日志和生成中的临时资源不进入发行包。

### 核心播放器的退出握手

`connect({ prepareExit })` 可声明退出保护。宿主只接受当前 iframe 来源和 origin 的 `denova:exit-ready`，通过唯一 requestId 匹配 `denova:prepare-exit`。回调先确认保存并停止当前 Story，返回 false 或抛错会保留页面。45 秒只限制 iframe 通信等待，不限制模型总执行时间。确认后卸载 iframe，再撤销 runtime，避免晚到的资源请求访问已停止的实例；撤销失败重新加载已确认的保存状态。视图内使用 `client.exit()` 请求同一宿主退出流程。
