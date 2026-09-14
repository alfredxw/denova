# Denova 扩展能力开发手册

扩展通过普通 HTTP 组合 Denova 的能力。游戏拥有完整内容区，可以自行实现聊天、视觉小说、地图或其他交互。宿主负责项目身份、授权、模型配置、执行和持久化；具体角色、分镜、立绘表情、规则与资源选择由扩展定义。

本文以可修改的「余光来信」示例说明如何组合能力。完整端点见 [HTTP API](plugin-platform-api.md)，安装与工具发布见 [插件手册](plugin-developer-guide.md)，游戏交付见 [游戏手册](game-developer-guide.md)。运行实例的 `/openapi.json` 是精确契约，`/capabilities` 返回本次授权与容量限制。

## 1. 选择最小实现

| 需要交付的内容 | 使用方式 |
| --- | --- |
| 写作、工作台和游戏共用的工具或服务 | `denova.plugin.json`，公开工具与工具集 |
| 自己管理世界状态与交互的作品 | `denova.game.json`，`game.storage.kind: "self"` |
| 使用现有互动故事，重做画面与操作 | 同一游戏清单，`game.storage.kind: "story"` |

静态 HTML/CSS/JavaScript 足以调用全部消费者接口。需要已有数据库或服务端程序时，再增加 Node 后端。无需新增一种插件类型、继承宿主组件或修改宿主 DOM。私有 Agent 留在游戏包中；跨作品共享的工具可提取为插件。

在「扩展 → 创建游戏」填写名称后进入工作台，项目自动生成通用开发骨架。如需研究「余光来信」，将 internal/platform/templates/galgame/ 和 common/client.mjs 复制到独立项目后打开源码。修改源码后点击「试运行」，在这里配置模型与测试参数并启动隔离运行。「发布到本机」只确认基本信息后安装，安装包从扩展详情页导出。检查后的候选包是冻结快照；源码变化后重新检查。安装后，在「游戏 → 新建故事线」选择作品，可以新建 Story 或关联当前项目已有的互动故事。关联前保留格式备份，原 Story 正文与模型配置沿用原有设置。

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
    "required": ["stories.read", "stories.write", "library.read", "assets.read", "images.generate"],
    "optional": []
  }
}
```

`story.modelSlot` 必须引用文本槽，用于新 Story；关联已有 Story 不替换其生成配置。`local:writer` 是宿主保存模型选择时使用的绑定键。图像调用中的 `modelSlot` 使用 `illustrator`，宿主解析已绑定的图像配置；模型密钥始终留在宿主。槽位可选表示作品能在未配置图像模型时继续游玩，图片按钮应说明如何配置。

权限按实际读取或操作划分，不能通过 body 更换 Project、Story 或另一游戏实例。资料库条目是内容，不携带工具授权。Node 后端具有当前系统账户权限；上述范围限制属于平台 HTTP 接口，并非操作系统沙箱。

## 3. 连接与完整自定义界面

模板的 `client.mjs` 验证父窗口、来源与 nonce，再取得短期连接。`context` 包含绑定范围、准确发行、语言、主题、设置和开局参数。

```js
import { connect } from './client.mjs';
const client = await connect();
const story = await client.request('/story');
// Render story.turns using your own DOM, canvas, animation or input controls.
```

响应只提供玩家可见的正文、选项、回合修订、分支与运行状态。规划、模型上下文、工具参数、密钥与内部路径不通过该接口泄漏。监听 `denova:appearance` 后更新当前单一语言和主题。自由输入、点选、逐句播放、自动播放和快捷键均由作品定义；页面必须保留停止和错误恢复入口。

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

游戏绑定、开局配置与这些记录均写入同一 Story journal；游戏列表只是投影，不新增 `instance.json`。原来的互动故事仍可打开该 Story。解除游戏绑定保留正文。独立的呈现 Agent 使用自己的会话 journal，作品只存请求和完成引用，不再保存一份聊天历史。平台不承诺多个独立 Agent 会话的原子回档。

## 5. 资料库与图像组合

`GET /library/items?query=...&limit=50` 获取当前项目的分页摘要，`GET /library/items/{id}` 获取选中的完整内容。样例复制选中角色的名称、描述与可选图片引用到自己的记录中，使游玩中的角色不会随资料库编辑悄悄改变。其他作品可以选择实时读取，但应向用户说明行为。

资源使用 `{kind: "project" | "generated", path: "relative/path.png"}`。请求 `/assets/content` 返回经授权的图片字节；`project` 只允许受支持的 `assets/` 图片，`generated` 只读取当前扩展实例生成的图片。

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
    prompt: 'A quiet station at dusk, watercolor illustration, no text.', size: '1536x1024' }),
});
// Query /images/generations/{commandId}; save completed images[].asset.
```

每个命令生成一张图片。请求状态包括 `running/completed/failed/cancelled/interrupted`；停止使用 `/cancel`。相同 ID 和相同输入查回原结果，不再次付费；相同 ID 更改输入报冲突。进程中断后无法确认结果时返回 `interrupted`，由用户明确重新生成并创建新 ID，禁止自动重放。具体尺寸和质量支持由所选提供商决定。

## 6. 「余光来信」如何工作

1. 选择默认或资料库角色与故事前提，原 Story Agent 直接生成简短的对话与叙述。首次请求定义作品的逐行标记协议，后续玩家自由输入或选择行动。
2. `/story/events?operationId=...` 以 SSE 提供 reset、delta、settled。仅公开当前根任务的临时正文；推理、工具和子 Agent 事件不传出。reset 撤回临时文字，settled 后重新读取 canonical Story。断线重新订阅原执行，绝不触发第二次生成。
3. 同一解析器处理流式和已提交正文。角色、表情和场景指令不显示为对白；未闭合指令等待完整后再解释，已有纯文本故事仍可阅读。每次交流通常二到六句，玩家可以直接回应，不必走完章节。
4. 舞台自带原创车站背景与两位成年角色的透明立绘。玩家可以在画室生成背景、当前表情和事件 CG，或使用资料库图片。图像命令持久保存后才提交，响应丢失沿用原 ID 查回。
5. 阅读位置与图片引用按 Story 分支、回合和正文修订保存。没有额外的呈现 Agent 或第二份剧情历史；刷新、回看不生成剧情。脚本上限 128 Ki 字符、120 行对白，每行 6000 字符，超限明确提示并允许查看原文。

示例文件位于 [`internal/platform/templates/galgame`](../internal/platform/templates/galgame)。`scene.mjs` 定义作品协议与校验，`host.mjs` 封装范围接口，`stage.mjs` 绘制形象，`game.mjs` 组织交互，`locales/` 与 `style.css` 控制双语和外观。替换这些文件即可开发另一种作品；不需要向 Denova 增加立绘、好感度或 CG 类型。

## 7. 发布前验证

检查纯文本模型和图像模型的选择、权限不足、资料库为空、长正文和长角色描述；覆盖双语、两种主题和窄屏。验证刷新、停止、响应丢失、重启、相同命令查回、记录写冲突，以及分支和正文修订后不复用未来数据。先使用本地确定性模型，再按需验证真实提供商。

继续保留原自管游戏与插件工具的调用链。需要更新数据格式时保持明确版本、备份和恢复路径，不自行改写宿主管理目录。发布只包含运行白名单中的文件；源码、私密配置、诊断日志和生成中的临时资源不进入发行包。
