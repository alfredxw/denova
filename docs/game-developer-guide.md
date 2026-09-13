# Denova 游戏开发手册

状态：第一版 A 已实现，尚未发布。更新：2026-09-12。

配套：[插件开发手册](plugin-developer-guide.md) · [系统设计](plugin-platform-design.md) · [HTTP API](plugin-platform-api.md)。

游戏是游戏页中独立的可玩作品。内置「互动故事」保留现有 Story 数据与运行方式；第三方游戏拥有独立前端、可选 Node 后端和每局单独的自管存档。

## 开发与游玩

在「扩展 → 创建扩展」选择「开发游戏」，填写名称和可选需求，创建独立 Project 后进入工作台。工作台统一提供普通 Agent 会话、源码清单、文件、版本、构建、检查、试玩、导出和安装。源码编辑不改变已安装发行。扩展页只管理已安装作品，通过「打开源码」继续原项目与会话；未安装的源码草稿只在工作台显示。

游戏页直接展示当前故事线与游戏内容。点击新建故事线时，内置互动故事与已启用且依赖可用的游戏一起出现在「游戏类型」中；只有一种可用游戏时隐藏选择器。用户可在这里设置默认游戏，默认失效时回退内置互动故事。偏好按用户保存在 games/preferences.json，只影响新建故事线。

新故事线绑定当前书籍 Project、所选游戏的准确发行、依赖、模型及独立实例 ID。改变默认游戏或新建时的选择不转换已有故事线。已有存档在同一个故事线选择器中继续，也可重命名、导出、删除和显式切换发行。

游戏占用内容区，宿主保留故事线切换、退出停止、全屏、导航与设置。切换一级菜单不停止游戏；扩展页停用游戏或依赖只阻止新启动，已有实例可继续至正常停止。修改实际授权会先停止受影响的实例，游戏开局参数仍在故事线中配置。开发预览和测试存档统一在工作台管理；重置测试存档会先备份。

## 游戏示例

产品只提供一个游戏示例「月下来信」，默认不安装、不启用。静态游戏和 Node 后端游戏仍是支持的实现方式；存档与 Node 进程测试使用独立测试素材，不作为产品模板分发。

npc-game 模板是独立 HTML 侦探游戏「月下来信」。界面由游戏自己的 HTML/CSS 绘制像素车站，与宿主样式隔离；玩家调查旧钟后才能找到信件，收集三件证物后可送出信件结案。证物和结案状态由游戏程序确定并保存，不依赖模型回复判断。玩家分别与守灯人林灯、档案员许笺交谈，通过转述线索了解一封未寄出的信。两位角色有不同知识和性格，不会直接读取彼此的私聊。游戏使用 characters/lamplighter.json 和 characters/archivist.json，分别通过 local:character、local:archivist 调用，共用由玩家开局选择的 local:writer 模型槽，无需提供模型密钥或安装其他插件。

index.html 与 style.css 决定界面，game.mjs 展示完整 API 调用，locales 中保存独立中英文文案。模板无需构建与 Node 后端。更换角色时同步修改私有定义、game.uses.agents、game.mjs 联系人与两份语言资源；改变已有角色定义应使用新的会话 key。聊天输入上限为 8000 个 UTF-16 代码单元，超出时输入框阻止继续输入；历史按 50 条分页，可继续加载更早记录。模型回复没有总运行时长限制，玩家可以停止回复。

模板使用已有 HTTP/JSON API，不依赖单独发布的 SDK。通用 client.mjs 仅负责凭证握手和 fetch；可以直接用其他 HTTP 客户端实现相同协议。

### 角色聊天如何使用平台

```js
const client = await connect();
const session = await client.request('/agents/sessions', {
  method: 'POST',
  body: JSON.stringify({
    projectId: client.context.scope.projectId,
    definition: 'local:character', key: 'messages-v1:lamplighter',
  }),
});
const history = await client.request(`/agents/sessions/${session.ref.sessionId}/history?limit=50`);
// Persist commandId and exact input with a revision-checked game-data write first.
const result = await client.request(`/agents/sessions/${session.ref.sessionId}/runs`, {
  method: 'POST', body: JSON.stringify({ commandId, input: { text } }),
});
// Subscribe to /agents/runs/{runId}/events with the scoped bearer token.
// POST /agents/runs/{runId}/cancel lets the player stop a reply.
```

平台负责模型配置、调用、角色会话及历史持久化；游戏负责联系人呈现、剧情设计和自身状态。模板通过 stream.mjs 消费 SSE，按 snapshot 替换当前文本、按 delta 追加对白，按 result 完成请求。断流时保留待处理请求并显示恢复入口，重连使用新 snapshot，避免重复追加；无需轮询或限制模型总运行时间。角色聊天只是起点：需要物品、任务或关系数值时，可在 gameData 保存游戏状态；需要可复用业务能力时，可声明工具依赖并调用 tools API。纯提示词角色不会自动更新结构化数值，作者应明确工具或状态更新协议。

## 游戏清单

扩展详情页的「开始游戏」会进入游戏页并预选此游戏的新故事线表单；提交表单前不会创建存档，也不会修改默认游戏。

游戏可在 `game.cover` 声明封面图片的源码相对路径，例如 `"cover": "cover.png"`，同时将该文件加入 `distribution.files`。支持 PNG、JPEG、WebP、GIF，最大 4 MiB，建议 3:4 竖版。宿主从已安装的冻结发行读取封面，不启动游戏、不加载远程图片或 SVG。未声明或加载失败时使用游戏图标；插件不展示封面。源码清单编辑器提供可选路径输入。

「月下来信」示例包含封面及可用的游戏设置：`showHints`（默认 `true`）控制输入提示，`textSize`（默认 `standard`，可选 `large`）控制对话和输入文字。它们通过双语 schema 展示，保存在已有扩展设置文件中，运行时读取 `context.settings`；保存后下次启动游戏生效。

denova.game.json 至少声明 manifestVersion: 1、id、version、apiMajor: 1、中英文 name、permissions、views 和 game。不能包含公共 contributes；私有能力放在 definitions。

```json
{
  "manifestVersion": 1,
  "id": "author.garden",
  "version": "1.0.0",
  "apiMajor": 1,
  "name": { "zh-CN": "花园", "en-US": "Garden" },
  "distribution": { "files": ["index.html", "client.mjs"] },
  "views": [{ "id": "stage", "source": { "kind": "static", "path": "index.html" } }],
  "game": {
    "viewId": "stage",
    "storage": { "kind": "self", "saveFormat": "garden-v1" }
  },
  "permissions": { "required": ["gameData"], "optional": [] }
}
```

静态页面只读取入口所在目录的分发文件。backend 视图由隔离来源代理到声明的后端路径，支持作品自己的 HTTP 业务接口。后端启动协议见插件手册。

game.uses.agents 只能选择 builtin/assistant 或本游戏的 local:id；game.uses.toolsets 可以选择工具插件能力，requires 声明对应插件版本与贡献。私有模型槽在开局配置中使用 local:slotId。游戏 ID 与插件 ID 即使相同也不是同一身份。

## 页面连接

游戏在独立 loopback 来源的 iframe 中运行。宿主根据实际 frame window、origin 与随机 nonce 验证 denova:ready，再回复 denova:bootstrap，内容包含 connection 与 context。页面不能读取宿主 DOM，也不使用宿主内部 API。

connection 包含临时 baseUrl 与 bearer token。context 包含准确 source、绑定 scope、locale、theme、environment、settings 与 setup。凭证重启后失效，不写进存档。宿主在外观变化时发送 denova:appearance，模板更新单语文案和主题。

## 存档与请求恢复

纯前端游戏通过 game-data/file 使用规范相对路径。写入或删除必须提交 expectedRevision：null 只允许创建；修改已有内容必须使用上次读取的 revision。冲突时重新读取，不能盲目覆盖。

Node 后端直接管理自己的 dataDir；这类游戏不能同时用文件 API 覆盖该目录，以免破坏数据库事务。导出或删除前先停止后台。业务文件必须满足跨平台名称规则，不能含符号链接、特殊文件和大小写冲突。

角色聊天示例先将角色 ID、稳定 commandId 和完整玩家输入保存到 messages.json，再提交 Agent 请求；请求结束后从平台读取聊天历史，再清除待处理请求。回答与完成收据留在平台 canonical journal，不在游戏文件另存一份聊天正文。刷新或重启后重用同一个 commandId 找回原请求。状态为 incomplete 时历史仍保留，不自动重做可能产生外部副作用的调用。保存冲突或网络错误会显示重新连接入口，重新读取最新 revision 后恢复请求；同一时间只发送一条消息，但仍可切换联系人查看历史。

一个 Agent 会话只使用其 Product Session JSONL 保存正文、运行记录、配置与请求收据。索引可以删除后重建，runs 目录和浏览器存储不是恢复事实源。自管存档引用完成记录不等于支持任意回档或删除未来 NPC 记忆；这属于后续托管恢复能力。

## 更换发行和备份

游戏与插件共用[GitHub 安装和更新流程](plugin-developer-guide.md#从-github-安装与更新)。同版本号可以安装不同源码快照，兼容性由作者保证；扩展详情只展示当前安装，不提供历史发行管理。

安装更新不会改动已有存档。存档页只提供「使用已安装的更新」，不列出历史发行。显式切换必须保持同一非空 saveFormat、API 主版本、可用依赖及有效配置。切换先停止运行并保存 ZIP 备份，再用存档副本检查目标后台能否启动；失败保留原绑定与原存档。通过后原子写入新绑定。

默认保留存档已固定的插件版本，不随目录中的新版本自动更新。新的依赖范围不再接受这些版本时，切换会明确失败；改变依赖和角色定义需要作者一起处理其逻辑会话 key。

已有 Agent 会话保留原定义，不能用相同会话 key 偷换定义。需要改变角色定义或模型绑定时，作者应使用新的逻辑 key；存档格式转换与跨定义迁移不属于本版。

备份位于 games/backups，可手动保留或导出。卸载游戏不级联卸载插件，受引用发行与存档不会自动清理。本版不提供任意数据库在线快照、云部署、多人同步或通用 Story flow；这些能力按系统设计后续阶段推进。

## 游戏独有设置

游戏与插件共用[扩展设置标准](extension-settings.md)。扩展页的 settings 应用于该游戏的所有故事线，在下次启动时生效；game.setup 在新建故事线时生成开局表单，并随 instance.json 保存。运行时分别读取 context.settings 和 context.setup。全局设置修改不会改写存档进度或开局参数。
