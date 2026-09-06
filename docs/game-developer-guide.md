# Denova 游戏开发手册

状态：目标体验与契约草案；游戏开发模板、公开 SDK 和第三方游戏运行宿主尚未实现。更新：2026-09-06。

配套：[插件开发手册](plugin-developer-guide.md) · [系统设计](plugin-platform-design.md) · [API 参考](plugin-platform-api.md)。

## 1. 游戏是可以开始和继续游玩的作品

**一个游戏是 Denova 游戏页中的一部具体作品**，拥有自己的名称、介绍、入口、玩法、内容和开局方式。玩家安装或添加游戏后，可以开始新游戏、继续已有进度、退出和管理存档。

游戏作者拥有完整前端和可选后端，可使用 HTML、React、Canvas、WebGL／Three.js 等实现界面，也可运行 Node、Python、Go 等后端。Denova 提供平台外壳、运行管理与开放能力；游戏选择需要的能力，并决定如何组织体验。

| 游戏实现 | 作者负责 | Denova 与插件可提供 |
| --- | --- | --- |
| 纯前端 + 平台能力 | 舞台、交互和玩法代码 | Agent、图像、资产、实例文件、可选托管状态 |
| 纯前端 + 平台插件 | 舞台、具体角色与内容、能力组合 | NPC、规划、回合规则等可复用能力 |
| 自定义前端 + 自定义后端 | 游戏服务、规则、数据库和页面 | 按需调用平台能力；无须把业务路由改成 Denova RPC |

插件提供通用能力，游戏负责具体作品。同一个 NPC 插件可以支撑多部游戏；游戏可以完全不依赖第三方插件。游戏私有的 Agent 定义、工具或剧情算法可直接随游戏交付，只有需要作为平台扩展独立复用时才拆成插件。

## 2. 创建、开发与预览（G1）

目标路径为“**游戏 → 开发中 → 创建游戏**”。插件管理位于共通能力，两者入口分开。

1. 选择“纯前端游戏”或“带后端游戏”模板，也可从现有游戏模板派生。
2. 在普通 Project 的空目录中创建源码，填写稳定 gameId、名称、介绍和游戏入口。
3. 使用已有文件、终端和 Agent 工作台开发前后端；游戏开发不新增 Project 身份或第二套 IDE。
4. 检查游戏清单、产物、内容、插件依赖和权限；需要构建或准备环境时，在可见终端执行已展示命令。
5. 创建独立测试开局，配置模型与插件。需要项目能力时选择测试 Project，不能默认读取真实书籍或正式存档。
6. 预览与正式游戏使用相同容器和 SDK，预览显示开发标识。前端可以热更新，后端或能力变化先结束旧激活再重载。

“停止预览”保留测试进度，“重置测试开局”单独展示删除范围并提供备份。源码位置、测试目标和正式存档互相独立。

## 3. 一个纯前端 NPC 城镇（G2）

假设目标平台的示例库已提供 `alice.npc` 插件，它贡献 `character` Agent；下面的游戏复用它，同时提供自己的场景与交互。这个插件及 SDK 都是拟建示例，目前不可直接安装运行。

```text
moonlit-town/
  denova.game.json
  package.json
  src/main.ts
  locales/zh-CN.json
  locales/en-US.json
  dist/index.html
  dist/assets/...
```

### 3.1 声明一部游戏

```json
{
  "manifestVersion": 1,
  "id": "alice.moonlit-town",
  "version": "0.1.0",
  "apiMajor": 1,
  "name": { "zh-CN": "月下小镇", "en-US": "Moonlit Town" },
  "description": {
    "zh-CN": "探索小镇，与居民交谈。",
    "en-US": "Explore a small town and talk to its residents."
  },
  "development": { "build": { "command": "pnpm", "args": ["build"] } },
  "distribution": { "files": ["dist", "locales"] },
  "views": [{ "id": "stage", "source": { "kind": "static", "path": "dist/index.html" } }],
  "game": {
    "viewId": "stage",
    "storage": { "kind": "self" },
    "uses": { "agents": ["alice.npc/character"] }
  },
  "requires": [{ "pluginId": "alice.npc", "versionRange": "^1.0.0", "contributions": ["character"] }],
  "permissions": { "required": ["agents.run", "gameData"], "optional": ["images.generate"] },
  "locales": { "zh-CN": "locales/zh-CN.json", "en-US": "locales/en-US.json" }
}
```

`denova.game.json` 声明游戏，`denova.plugin.json` 声明扩展。一个产物根目录只能选择其中一种。同一仓库可以有 `games/moonlit-town` 与 `plugins/npc` 两个目录，分别检查、打包和版本化。

`game.uses` 选择会用到的公共能力，`requires` 声明提供这些能力的插件版本。安装时固定准确发行，创建开局时记录实际依赖和模型配置。游戏不自动获得插件的全部权限，也不能随加载顺序替换别的游戏的 Agent。

模型用途来自所选 Agent 和游戏自己的 `modelSlots`（如有）。宿主列出完整的“提供方／用途”，由用户映射已有连接；不把所有插件的同名模型槽合并成一个隐含全局设置。

此例 `storage.kind: self` 表示地图等业务状态由游戏保存。是否有后端与这个选择无关：纯前端也能用 `gameData` 保存自己的 JSON 存档。

### 3.2 对话逻辑

模板处理宿主握手，提供 `PlatformClient` 与当前游戏范围。作者逻辑只需选择明确项目、定义和稳定 NPC key：

```ts
async function talkToNPC(
  api: PlatformClient,
  projectId: string,
  intent: { npcId: string; commandId: string; text: string },
) {
  const session = await api.agents.sessions.ensure({
    projectId,
    definition: "alice.npc/character",
    key: intent.npcId,
  });
  const run = await api.agents.runs.start({
    agent: session.ref,
    commandId: intent.commandId,
    input: { text: intent.text },
  });
  return api.agents.runs.wait({ run });
}
```

当前游戏开局来自授权通道。同一开局内稳定 key 重用同一 NPC 会话；另一次新游戏不会共享上一局记忆。每个会话一次一个主动 Run，不同 NPC 可以并行。

完整页面需要处理：

1. `platform.describe` 查询可用能力。宿主完成模型配置；未绑定项目时，由玩家操作触发项目选择并建立授权。
2. 玩家提交动作时生成 commandId，先保存输入与这个 ID，再调用。重复提交和重连沿用同一 ID，新的玩家意图才创建新 ID。
3. `events.subscribe` 展示流式内容和状态；waiting 转到宿主问答，取消调用 `agents.runs.abort`。平台权限不能由游戏替玩家批准。
4. 完成后保存结果。断线或重启先 `agents.runs.find`，不要盲目重复生成；incomplete 显示给玩家决定下一步。
5. 游戏保存结果时记录 commandId，防止“模型已完成、游戏保存响应丢失”导致重复采纳。

对话历史由 Denova 保存，游戏只记录会话引用和业务结果。平台事件 cursor 不能当作永久存档位置。

## 4. 自定义后端（G3）

给游戏增加以下 runtime，并将 stage 的来源改为 backend，即可由自己的服务提供整个页面：

```json
{
  "runtime": {
    "backend": {
      "launch": { "kind": "runtime", "runtime": "node", "entry": "dist/server.mjs", "args": [] },
      "protocol": "denova-runtime-v1"
    }
  },
  "views": [{ "id": "stage", "source": { "kind": "backend", "path": "/" } }]
}
```

也可以保留静态 stage，仅用后端处理业务。模板启动器接收宿主分配的监听信息和临时连接，服务报告 readiness 后才打开页面。HTTP／WebSocket 路由保持自己的协议，Denova API 只承担平台能力调用。

自管存储时，宿主给出可写 dataDir；数据库、事务和迁移由游戏实现。发行目录不可写，临时依赖与缓存放设备本地，不把本机端口、绝对路径、解释器路径和临时 URL 写进存档。数据库打开时不能再让文件 API 覆盖它。

纯前端游戏调用一个插件提供的后台能力，仍然是纯前端游戏；该插件进程归插件激活，游戏自己的后端归游戏运行。二者独立版本化，不用为了共用一套进程管理代码合并身份。

退出默认停止游戏及仅由它使用的插件激活。需要离开页面后继续工作的游戏明确声明后台需求，经用户选择后留在可见运行列表，并始终可停止。只关闭面板不等于关闭整局游戏。

本地程序按宿主账户权限运行，进程隔离不是 OS 沙箱。缺少 Node／Python、平台产物不匹配或启动失败时展示准确原因，不暗中安装完整开发环境。

## 5. 选择存储、剧情与恢复能力（G4）

| 方式 | 状态事实源 | 能力边界 |
| --- | --- | --- |
| 游戏自管 | 该开局的游戏文件或数据库 | 作者负责 schema、事务、保存与加载；平台负责运行、目录和停止后的导出 |
| Denova 托管 | 既有 Story journal | 使用状态 schema、动作提交、规划和包含 NPC 记忆的分支恢复 |

**两种方式都是游戏实现方式**，在游戏页使用相同的“开始／继续”入口。它们不划分成“应用”和“游戏模式”两个产品类别。

托管方式在 `game.storage` 中声明 schema、初始状态、选定 flow 和可选 plan document。GameInstance 直接绑定既有 Story／分支，发行与依赖等恢复配置写入该 Story journal；不另存一份 authoritative instance.json 或游戏进度。

玩法 flow 可以来自内置能力、插件，也可以由游戏后端实现私有逻辑。游戏自己的 Agent、工具和 flow 定义放在 `definitions`，只在本游戏的运行范围可见；它们不会自动成为其他游戏可选的插件贡献。

托管动作依次读取状态、调用角色／规划、暂存候选变化，最后由宿主一次提交。NPC 的完成位置需被明确采纳，未成功采纳的记忆不能进入下一轮。默认小说 RPG 的导演协议只是一个内置实现，不是所有游戏都必须调用的入口。

自管地图加 Denova NPC 历史是合法组合，但不承诺跨数据库与 journal 的原子恢复。作者只有在世界和 NPC 会话前缀均已协调恢复后才能提供完整回档；未支持时明确展示恢复范围。纯前端照样可以选择托管方式，不能用“没有后端”推导出“不能保存”。

## 6. 内容、表现与游戏身份

角色卡、世界书、团本和素材是游戏可以使用的内容。它们可以随游戏提供或从资料库选择，通过 format 和 formatVersion 声明语义。角色卡本身不授予工具权限，内容导入不执行其中脚本。

通用规则或渲染引擎可作为插件；具体人物、目标、开局与表现组合出可游玩的作品时，就形成游戏。只装一个引擎不在游戏页自动创建作品或存档。

游戏前端控制布局、演出、CG、3D 场景、音效和交互，宿主保留返回、全屏、设置、权限和停止入口。使用平台生图时保存资产引用，游戏决定如何演出；语音也可以由游戏或插件接外部服务，未实现的内置语音接口不能被当成可用。

导航、设置、错误和空状态维护中英文资源，跟随宿主语言；作品正文保留原语言。游戏美术由作者决定，仍需适配主题信号、窄屏、宽屏及长文本。

## 7. 交付并在游戏页运行（G5）

1. 构建并从发行白名单生成不可变游戏候选包，排除密钥、测试存档和私人历史。
2. 在干净环境安装同一产物，验证依赖、开局、交互、取消、退出、继续、导出和错误反馈。
3. 导出游戏包，或提供 GitHub 固定来源。游戏安装器只接受游戏产物；插件依赖单独列出，安装和授权可在同一向导完成但记录仍分开。
4. 玩家在游戏页打开作品详情，开始新游戏或继续某个已有存档；一个游戏发行可对应多个独立开局。
5. 更新安装提供新的游戏发行；旧存档继续使用原游戏发行与准确插件依赖。升级已有存档需停止、备份并显式迁移，失败回到原代码和数据。

卸载游戏不卸载其他作品仍在使用的插件；卸载插件前显示受影响的游戏。存档默认保留，依赖不可用时说明原因，不静默切换算法。复制或导出必须匹配存档所需的代码和内容版本，在线数据库一致备份需要专门适配。

开发工作台和游戏运行容器复用平台基础设施。插件的验收是扩展能力可用，游戏的验收是作品可以开始、游玩、保存和继续；两条链路都必须独立成立。
