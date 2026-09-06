# Denova 插件开发手册

状态：目标体验与契约草案；文中插件菜单、模板、SDK 和公开接口尚未实现，不是当前版本的操作说明。更新：2026-09-06。

配套：[游戏开发手册](game-developer-guide.md) · [系统设计](plugin-platform-design.md) · [API 参考](plugin-platform-api.md)。

## 1. 插件扩展能力，游戏交付作品

**插件是对 Denova 能力的扩展。** 它给平台增加工具、Agent 定义、Skills、上下文来源、规划／流程能力、编辑辅助或界面扩展，供写作、聊天、游戏以及其他获准插件选择使用。

**游戏是游戏页中实际可游玩的作品。** 游戏可以有完全自定义的前端和后端，也可以只有前端，组合 Denova 内置能力与插件完成运行。游戏的开发、开局和存档见单独的游戏开发手册。

| 交付物 | 例子 | 用户如何使用 |
| --- | --- | --- |
| 插件 | 世界书检索、NPC 对话能力、语音能力、审稿工具、可复用回合规则 | 安装后，在目标 Agent、工作区或游戏中配置使用 |
| 游戏 | 有人物、场景、玩法和开局的 Galgame、城镇模拟器、3D 冒险 | 在游戏页查看、开始新游戏、继续已有存档 |
| 内容资源 | 角色卡、世界观、团本、素材、规划模板 | 导入资料库，或供支持其格式的游戏与能力读取 |

“有前端”“有后端”“能打包”都不是插件的定义。插件可以有自己的设置页和功能面板；一套具体游戏的代码属于该游戏。通用对话引擎是插件，使用它做出的《月下物语》是游戏。

## 2. 先选需要开放的能力

| 你的需求 | 优先使用 |
| --- | --- |
| 改变 Agent 工作方式或提供审稿策略 | Agent 定义、提示词和 Skill |
| 让 Agent 执行一个新操作 | 工具及命名工具集 |
| 按当前任务提供世界书、记忆或状态材料 | 有来源和容量上限的上下文提供器 |
| 增加可复用玩法步骤或规划策略 | flow／planner 提供器 |
| 在写作、聊天或游戏中增加局部功能 | 已声明的面板、命令或结果视图 |
| 只是提供人物与设定 | 内容资源；不必建立可执行插件 |

插件通过公开协议扩展平台，不访问宿主内部 React 对象、Go Store 或持久化文件。工具仍经过 Denova 的调用、授权和记录流程；Agent 定义仍使用原有 Agent 引擎。

## 3. 在 App 中创建插件（P1）

目标路径为“**共通能力 → 插件 → 开发中 → 创建插件**”。

1. 选择“Agent／Skill”“工具／上下文”“界面扩展”或“后端能力”模板。
2. 选择普通 Project 中的空目录，填写插件 ID 和中英文名称；创建前展示文件，不覆盖已有内容。
3. 在现有工作台中使用文件编辑器、终端和 Agent 开发。插件源码不是新的 Project 类型。
4. 点击“检查”校验清单、入口、schema、本地化和依赖；检查不执行包内脚本。
5. 选择测试工作区／会话／游戏，配置模型和权限，启动预览。预览数据与正式使用范围分开。
6. 在目标功能中实际调用，查看前端、提供器与 Agent 诊断；修改贡献或后端后停止旧激活再重载。

源码项目与被插件操作的目标项目分别选择。审稿工具默认在测试书籍验证，不能因为开发项目正在打开就自动读取真实书稿。原生程序仍受操作系统账户权限约束，测试范围并不是 OS 沙箱。

## 4. 第一个工具插件（P2）

例子：给写作和游戏 Agent 提供统计文本 Unicode 码点数量的工具。模板生成构建配置，以下是作者需要维护的文件。

```text
text-tools/
  denova.plugin.json
  package.json
  src/extension.ts
  tools/count-characters.json
  locales/zh-CN.json
  locales/en-US.json
  dist/extension.mjs
```

### 4.1 插件清单

```json
{
  "manifestVersion": 1,
  "id": "alice.text-tools",
  "version": "0.1.0",
  "apiMajor": 1,
  "name": { "zh-CN": "文本工具", "en-US": "Text Tools" },
  "development": { "build": { "command": "pnpm", "args": ["build"] } },
  "distribution": { "files": ["dist", "tools", "locales"] },
  "runtime": { "extension": { "kind": "module", "entry": "dist/extension.mjs" } },
  "permissions": { "required": [], "optional": [] },
  "contributes": {
    "tools": [{ "id": "count-characters", "definition": "tools/count-characters.json" }],
    "toolsets": [{ "id": "text-utils", "tools": ["count-characters"] }]
  },
  "locales": { "zh-CN": "locales/zh-CN.json", "en-US": "locales/en-US.json" }
}
```

此工具只计算输入，无须申请项目文件或模型调用权限。`contributes` 声明对外可选能力，完整名称为 `alice.text-tools/text-utils`。清单不声明游戏、游戏开局或游戏存档；安装此插件不会在游戏列表中增加一部作品。

`development.build` 引用模板 package.json 中的构建命令。`distribution.files` 是发行文件白名单，不包含开发依赖、密钥和测试数据。locale 文件在本例可为空对象；增加用户可见面板、错误和工具标题时维护两份独立资源。

### 4.2 工具定义

`tools/count-characters.json`：

```json
{
  "description": "Count Unicode code points in text, including whitespace.",
  "inputSchema": {
    "type": "object",
    "properties": { "text": { "type": "string" } },
    "required": ["text"],
    "additionalProperties": false
  },
  "outputSchema": {
    "type": "object",
    "properties": { "count": { "type": "integer", "minimum": 0 } },
    "required": ["count"],
    "additionalProperties": false
  },
  "effect": "pure"
}
```

`src/extension.ts` 的目标作者接口片段：

```ts
export function activate(ctx: ExtensionContext) {
  ctx.tools.register("count-characters", async (_call, input) => {
    if (input === null || typeof input !== "object" || Array.isArray(input)
      || typeof input.text !== "string") {
      throw new Error("Expected a text field");
    }
    const count = Array.from(input.text).length;
    return { content: `Character count: ${count}`, data: { count } };
  });
}
```

`ExtensionContext` 是拟定 SDK 类型，目前没有可安装的 SDK。模板在独立提供器进程内调用 activate，宿主校验输入与结果，并将函数绑定为当前激活的调用端点。

### 4.3 在写作和游戏中选择使用

预览成功后，把工具集加入测试 Agent 的工具选择，实际执行一次调用。安装只使工具集可选，不会把它自动添加到所有 Agent。游戏也可以通过声明依赖，选择这个工具集。

注册 ID 必须在清单中，重复 ID 或缺失实现会导致激活失败。禁用时停止工作、撤销注册；自己创建的连接和监听通过 `ctx.onDispose` 清理。`effect: pure` 描述工具效果，不代表原生程序被 OS 沙箱限制。

## 5. 增加其他扩展（P3）

Agent 定义声明指令、模型用途、工具、Skills、上下文和委派选择。模型槽由用户映射到已有连接，插件不读取模型密钥。平台内置提示、工具描述和模型可见反馈使用英文，用户创作内容保留原语言。

上下文提供器声明来源、用途、稳定性和容量；静态设定保护缓存前缀，动态状态按需提供。Skills 接入同一个有效目录，模型看见的条目与工具实际可加载的资源一致。

写作扩展通过 `editor.context` 读取授权文档和选区，通过 `editor.propose` 提出修改，在原生审阅界面采纳。文件 revision 与未保存草稿都要校验，冲突时保留提案。

界面插件可以在声明的位置挂载前端视图；纯前端面板通过受限桥接调用平台即可，不强制启动后端。模型工具、后台上下文等需要独立于页面运行的实现放在提供器中。关闭面板不应意外终止仍被游戏使用的能力。

需要自有 HTTP 服务、Python 或原生程序时使用后端能力模板。宿主按启动协议提供临时通道和路径，服务报告 readiness；前端通过实例专用来源访问业务路由。后端同时提供工具时使用 `runtime.extension: {"kind":"backend"}`，同一插件激活不重复启动 module 进程。

本地程序按当前操作系统账户权限运行，单独进程不等于安全沙箱；插件 API 授权不能阻止原生代码直接访问账户可访问的文件和网络。安装与预览如实展示本地代码执行和环境要求。

## 6. 提供给游戏的能力如何设计

插件暴露具名能力、输入输出 schema、所需权限和适用范围。游戏选择该能力并提供具体角色、场景和参数。插件不要把某一部作品的私人剧情写成所有使用者必须接受的全局配置。

例如，通用 NPC 插件提供角色会话能力，游戏用自己的角色卡实例化 NPC；通用回合插件提供结算流程，具体地图、胜负条件和 UI 属于游戏。只服务一个游戏的后端算法可以直接放在游戏中，出现复用需求后再独立为插件。

插件参与托管游戏动作时只提出候选状态，由宿主正式提交。插件状态按目标范围保存：Agent 恢复状态归该会话 journal，游戏状态归游戏选择的存储边界；插件缓存不能成为另一份游戏存档或 transcript。

## 7. 打包、安装与维护（P4）

1. 检查并执行模板构建，命令和输出在工作台可见。
2. 从发行白名单生成不可变候选包，在干净测试范围验证声明的能力。
3. 导出插件包，或将同一产物放到 GitHub 发行目录供安装；导出不会自动向外发布。
4. 安装时预览准确来源、发行、能力、权限与运行条件，再启用；依赖需要独立展示和授权。
5. 验证写作与游戏中的选择、调用、取消、禁用和故障反馈，以及声明支持的平台、主题与语言。

更新插件不会静默改变已有游戏存档绑定的版本；游戏明确升级依赖时，先停止相关工作并处理数据兼容性。禁用或卸载前展示受影响的游戏与会话，保留数据和仍被引用的发行；依赖不可用时给出原因，不偷偷替换实现。

插件 ID、发行与能力 ID 独立于显示名和源码目录。修改内容使用用户副本，独立 Fork 插件使用新 ID 并遵守许可证。密钥、授权、私人会话和测试数据不随分享导出。

## 8. 验收边界

插件闭环的结果是：作者能在 App 内开发并安装一种扩展，用户能在目标场景选择、配置、运行和停用它。工具、Skill、面板和后台提供器各有样例，至少一项能力可同时用于写作和游戏。

游戏作品的开发与运行是另一条闭环，见[游戏开发手册](game-developer-guide.md)。两者共用基础机制，但不互相冒充产品对象。
