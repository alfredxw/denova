# Nova

Nova 是一个面向长篇小说创作的 AI 写作工作台，目标是提供类似 IDE 的创作体验，Agent能力覆盖灵感、设定、大纲、分卷、分章、细纲、正文、互动创作、版本管理和一致性检查等流程。

当前版本：v0.1.4（2026-05-29）

## 截图
![Nova 小说 IDE](./img/ide.png)
截图较多，默认折叠。点击下方标题可展开查看。


<details>
<summary>展开查看 Nova 界面截图（3 张）</summary>

### 互动故事工作台

![Nova 互动故事工作台](./img/interactive.png)

### 资料库

![Nova 资料库](./img/setting.png)

### 故事风格配置（讲述者）

![Nova 故事风格](./img/story-teller.png)

</details>

## 功能特性

- **小说 IDE 工作台**：左侧项目结构、中间 TipTap 编辑器、右侧创作 Agent 和版本管理面板。
- **AI 创作 Agent**：支持 SSE 流式输出、工具调用展示、思考过程折叠、任务中断和活跃任务恢复。
- **章节组细纲与章节编辑**：支持按接下来一组情节生成细纲，再逐章写作；章节编辑支持 Markdown / TXT、自动保存、手动保存、字数统计、文章内搜索、选区引用和多 Tab。
- **互动故事工作台**：支持故事线、剧情分支、下一步行动候选、场景记忆、可行动空间和可中断流式生成。
- **结构化资料库**：支持角色、世界观、地点、势力、规则和物品等 Lore Item 管理，并提供资料库 Agent 辅助整理。
- **角色卡导入**：支持导入 SillyTavern 酒馆 v2 PNG / JSON 角色卡，可写入当前书籍或创建新书。
- **多书籍管理**：记录最近打开书籍，支持切换 workspace，并在后端重启后恢复上次书籍。
- **多会话管理**：每个书籍 workspace 支持多个独立会话，`/clear` 只追加上下文清理标记，不物理删除历史。
- **风格参考**：支持在用户级 `<nova_dir>/styles/` 中维护 `.md` / `.txt` 文风样本，并通过 `#` 在本轮对话中引用。
- **版本管理**：每个书籍 workspace 可作为独立 Git 仓库，支持初始化、创建版本、查看历史、回滚、stash 和 pop。
- **可分层配置**：支持全局、用户级、工作区级配置，常用模型、编辑器、Agent 和互动模式参数可在设置页调整。

## 快速开始

### 环境要求

- Go 1.24+
- Node.js 20+
- pnpm

如果本机还没有启用 pnpm，可以先执行：

```bash
corepack enable
```

### 配置模型

Nova 使用 OpenAI 兼容接口。推荐从模板创建本地配置文件，或者直接使用环境变量。

```bash
cp config.template.toml config.toml
```

最少需要配置：

```toml
openai_api_key = "your-api-key"
openai_base_url = "https://api.deepseek.com"
openai_model = "deepseek-v4-pro"
```

如需让不同 Agent 使用不同平台或模型，可以增加多个 OpenAI 协议兼容配置，并按 Agent 分配：

```toml
[[model_profiles]]
id = "deepseek"
name = "DeepSeek"
openai_api_key = "your-api-key"
openai_base_url = "https://api.deepseek.com"
openai_model = "deepseek-v4-pro"

[[model_profiles]]
id = "openai"
name = "OpenAI"
openai_api_key = "your-api-key"
openai_base_url = "https://api.openai.com/v1"
openai_model = "gpt-4.1"

[agent_models.ide]
profile_id = "deepseek"
temperature = 0.7

[agent_models.lore_editor]
profile_id = "openai"
temperature = 0.2
```

也可以用环境变量覆盖配置文件，环境变量优先级最高：

```bash
export OPENAI_API_KEY="your-api-key"
export OPENAI_BASE_URL="https://api.deepseek.com"
export OPENAI_MODEL="deepseek-v4-pro"
```

常用可选项：

```bash
export NOVA_WORKSPACE="/path/to/your-novel"
export NOVA_DIR="./.nova"
export NOVA_SKILLS_DIR="./skills"
export NOVA_BACKEND_PORT="8080"
export NOVA_FRONTEND_PORT="5173"
```

说明：

- `NOVA_WORKSPACE` 指定启动后直接打开的书籍目录；不指定时会恢复最近打开的书籍，没有最近书籍则进入书籍管理。
- `NOVA_DIR` 是 Nova 数据目录，新建书籍、用户级配置和最近书籍记录会放在这里。
- `NOVA_FRONTEND_PORT` 主要用于 `./bootstrap.sh fe`；默认一键启动时前端开发服务使用 Vite 默认端口 `5173`。

### 启动开发环境

启动前后端：

```bash
./bootstrap.sh
```

仅启动前端：

```bash
./bootstrap.sh fe
```

仅启动后端：

```bash
./bootstrap.sh be
```

默认地址：

- 前端：`http://localhost:5173`
- 后端：`http://localhost:8080`

首次启动时，如果没有指定或恢复到书籍，WebUI 会自动打开「书籍管理」。点击「新建书籍」填写书名即可开始；如需打开已有本地目录，建议用 `NOVA_WORKSPACE=/path/to/book ./bootstrap.sh` 或生产构建后的 `--workspace` 参数启动。

## 使用指南

### 1. 创建或打开一本书

Nova 中一个 workspace 就是一本书。进入 WebUI 后，左侧活动栏的书籍图标会打开「书籍管理」：

- 「新建书籍」会在 `NOVA_DIR` 下创建新书，并自动生成 `CREATOR.md` 和 `脑暴.md` 等基础文件。
- 最近书籍会显示在列表中，点击即可切换。
- 后端重启后默认恢复最近打开的书籍；如果启动时传入 `--workspace` 或 `NOVA_WORKSPACE`，则以启动参数为准。

推荐工作区结构：

```text
my-novel/
├── CREATOR.md
├── 脑暴.md
├── chapters/
│   ├── ch0001-废材开局.md
│   └── ch0002-初入宗门.md
├── setting/
│   ├── outline.md
│   ├── progress.md
│   └── chapter-groups/
│       └── group01-宗门立足.md
├── drafts/
│   └── ch0001-废材开局.md
└── .nova/
    ├── config.toml
    ├── lore/
    └── sessions/
```

关键文件说明：

- `CREATOR.md` 是作品级最高优先级创作者指令，下一轮对话会重新读取。
- `脑暴.md` 用来沉淀题材、卖点、目标读者、金手指、剧情走向和参考作品。
- `setting/outline.md` 是长期大纲，记录主线、卷章安排、阶段目标和章节方向，低频变化。
- `setting/chapter-groups/` 存放章节组细纲，每个文件只规划接下来要写的一组连续章节，例如 `group01-宗门立足.md`。
- WebUI「资料库」维护角色、世界观、地点、势力、规则和物品等长期设定，内部存储在 `.nova/lore/`；`setting/characters.md` 和 `setting/world-building.md` 已不再作为创作上下文。
- `chapters/` 存放正文，章节文件建议遵循 `chNNNN-章节名.md`，例如 `ch0001-废材开局.md`；已有 `ch01-...` 文件仍可识别，千章作品推荐使用四位补零以保持文件树排序稳定。
- `drafts/` 是可选草稿目录；默认草稿流程关闭，只有启用草稿流程或明确要求草稿时使用。
- `<nova_dir>/styles/` 存放用户级风格参考样本，只在本轮通过 `#` 指定或命中当前讲述者配置的场景化风格规则时注入 Agent。
- `.nova/sessions/` 存放当前书籍的会话历史。

### 2. 在小说 IDE 中写章节

活动栏点击「写作」进入 IDE 模式：

- 左侧目录树用于浏览、新建、重命名、移动和删除工作区文件。
- 中间编辑区支持 Markdown / TXT，打开文件会进入 Tab；已打开 Tab 会按配置的上限自动管理。
- 编辑器会自动保存，也可以用 `Cmd/Ctrl + S` 手动保存。
- `Cmd/Ctrl + F` 搜索当前文章；选中文本后可点引用按钮，或用 `Cmd/Ctrl + Shift + L` 把选区送到创作 Agent。
- 顶部和底部状态栏会显示当前章节字数、全书字数、章节状态和生成状态。

推荐流程：

1. 先完善 `脑暴.md` 和 `CREATOR.md`，把题材、风格和硬约束写清楚。
2. 让 Agent 生成或调整 `setting/outline.md`、`setting/progress.md`，并把角色、世界观等长期设定整理到资料库。
3. 让 Agent 基于当前定稿进度生成下一组章节细纲，写入 `setting/chapter-groups/groupXX-情节目标.md`。细纲只规划接下来一组章节，不批量展开很多组。
4. 按细纲逐章写入 `chapters/`；如启用草稿流程，则先写入 `drafts/`，确认后再进入 `chapters/`。
5. 每完成一段稳定内容就创建版本，后续重写和续写更容易回退。

### 3. 和创作 Agent 协作

活动栏点击机器人图标打开右侧「创作Agent」。输入框支持：

- `Enter` 发送，`Shift + Enter` 换行。
- 输入 `/` 打开命令提示。
- 输入 `@` 引用工作区文件，例如 `@setting/outline.md`。
- 输入 `#` 引用风格参考，例如 `#古龙.md`。
- 正在生成时可以中断；如果前端刷新但后端任务还在运行，页面会尝试恢复活跃任务。

常用命令：

| 命令 | 用途 |
| --- | --- |
| `/plan` | 先规划再执行 |
| `/continue` | 续写当前章节 |
| `/rewrite` | 重写章节 |
| `/outline` | 生成或调整大纲 |
| `/group-plan` | 生成下一组章节细纲 |
| `/status` | 查看当前作品状态 |
| `/clear` | 清理后续上下文，不删除历史消息 |
| `/help` | 查看命令帮助 |

多会话在每本书内独立保存。`/clear` 只追加上下文清理标记，历史消息仍保留在界面和 `.nova/sessions/` 中。

### 4. 使用互动故事工作台

活动栏点击「互动」进入互动工作台。这个模式适合跑剧情、试角色反应、探索分支路线：

- 中间故事舞台输入「你要做什么？」后，Agent 会按互动回合生成正文。
- 每回合会更新下一步行动候选、场景记忆、可行动空间、物品资源、世界规则和未解决线索。
- 剧情路线图用于查看故事线继承关系、切换分支和从节点创建新剧情线。
- 左侧互动设置区可维护资料库、创作者指令和讲述者规则；每个具体讲述者页面可维护自己的互动单轮目标字数和场景化风格规则。
- 舞台字号、行高和最大输出 Token 可在设置页的「互动模式」分组调整。

互动模式不会直接修改 `chapters/` 正文章节，它更像一个剧情试演场；确认好分支和桥段后，再回到 IDE 模式整理成正式章节。

### 5. 管理资料库和角色卡

资料库用于维护结构化设定，支持角色、世界观、地点、势力、规则、物品和其他条目：

- 在互动工作台左侧打开「资料库」，可以手动新增和编辑条目。
- 「资料库 Agent」支持用自然语言批量整理、补充和修改资料；输入框中可用 `@条目名` 限定对象。
- 资料库支持手动创建版本和恢复历史版本。
- 在「书籍管理」中点击「导入酒馆角色卡」可导入 SillyTavern 酒馆 v2 PNG / JSON 角色卡，并选择写入当前书籍或创建新书。

建议把长期稳定的人物、势力、规则放进资料库，把阶段性写作进度放在 `setting/progress.md`，避免信息职责混在一起。

### 6. 使用版本管理

活动栏点击分支图标打开「版本管理」。版本管理基于当前书籍目录内的本地 Git 仓库：

- 第一次使用先点击「初始化版本仓库」。
- 有改动后填写版本说明并创建版本。
- 可以查看版本历史、暂存当前改动、恢复最近暂存内容。
- 回滚前需要工作区干净；如果有未提交变更，先创建版本或暂存。

建议在这些节点创建版本：大纲定稿、每章完成、重写前、批量资料库整理前、长篇 Agent 操作前。

### 7. 调整设置

活动栏底部齿轮打开「设置」。设置分为用户级和工作区级：

- 用户级适合模型、默认字体、通用 Agent 参数。
- 工作区级适合当前书籍的章节命名、章节组建议规模、草稿流程、最大打开 Tab、互动模式显示等。
- 配置加载优先级为：内置默认值 < 全局 `config.toml` < 用户级 `<nova_dir>/config.toml` < 工作区级 `<workspace>/.nova/config.toml` < 环境变量。

常见可调项：

- 模型：`openai_api_key`、`openai_base_url`、`openai_model`、`model_profiles`、`agent_models`
- 编辑器：自动保存间隔、章节文件名格式、最大同时打开 Tab 数
- Agent：最大迭代次数、失败重试次数、是否默认计划模式
- 讲述者页面：按讲述者独立配置单轮目标字数、按场景匹配默认风格参考
- 互动：最大输出 Token、舞台字号和行高

## 常用快捷键

| 快捷键 | 作用 |
| --- | --- |
| `Cmd/Ctrl + S` | 保存当前章节 |
| `Cmd/Ctrl + K` | 打开命令面板 |
| `Cmd/Ctrl + Enter` | 触发当前章节续写 |
| `Cmd/Ctrl + Shift + D` | 打开版本管理 |
| `Cmd/Ctrl + F` | 搜索当前文章 |
| `Cmd/Ctrl + Shift + L` | 引用编辑器选区到创作 Agent |
| `Esc` | 关闭右侧面板、弹窗或输入提示 |

## 构建与运行

生产构建：

```bash
./build.sh
```

构建产物位于 `output/`：

```bash
cd output
./nova --workspace /path/to/my-novel
```

常用启动参数：

```bash
./nova --workspace /path/to/my-novel --port 8080
./nova --workspace /path/to/my-novel --no-open
./nova --workspace /path/to/my-novel --dev
```

## 测试

后端测试：

```bash
go test ./...
```

前端测试：

```bash
cd web
pnpm test
```

前端构建：

```bash
cd web
pnpm build
```

## 技术栈

- **后端**：Go、Hertz、Eino、SSE
- **前端**：React、TypeScript、Vite、Tailwind CSS v4、TipTap、shadcn/ui、Radix UI
- **状态管理**：React Query 管理服务端状态，Zustand 管理本地 UI 状态
- **测试**：Go test、Vitest、React Testing Library、MSW

## 目录结构

```text
.
├── cmd/nova/              # Nova 服务入口
├── config/                # 配置加载与环境变量覆盖
├── internal/
│   ├── agent/             # AI Agent 构建、Prompt 注入、Runner 和流式事件编排
│   ├── api/               # Hertz 路由、请求响应 DTO 和 SSE 输出
│   ├── app/               # 应用装配、workspace/session/book/agent 运行时管理
│   ├── book/              # 作品 workspace、文件树、文件读写和本地 Git 能力
│   ├── interactive/       # 互动故事、剧情线和讲述者数据能力
│   ├── prompts/           # 后端提示词模板
│   └── session/           # 多会话持久化
├── skills/                # 创作技能提示词
├── web/                   # React WebUI
├── AGENTS.md              # 项目约定和 Agent 开发规则
├── CHANGELOG.md           # 版本变更记录
├── bootstrap.sh           # 开发环境启动脚本
└── build.sh               # 生产构建脚本
```

## 开发约定

- 前端负责 UI 交互，后端负责 AI Agent 运行、文件管理和版本管理。
- 服务端数据使用 React Query，本地 UI 状态使用 Zustand。
- 基础交互组件优先使用 shadcn/ui 和 Radix UI，避免手写复杂底层组件。
- 修改关键逻辑时补充 Go 单测或 Vitest 测试。
- 提交前更新 `CHANGELOG.md`，遵循 Keep a Changelog 格式。

## 发布记录

最新版本：v0.1.4（2026-05-29）。完整记录见 [CHANGELOG.md](./CHANGELOG.md)。
