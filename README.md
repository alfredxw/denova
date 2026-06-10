<h1 align="center">Nova</h1>

<p align="center">
  <strong>面向长篇小说与互动叙事创作者的 AI 创作工作台</strong>
</p>

<p align="center">
  从灵感、设定、大纲、章节写作，到互动试演、资料库沉淀和本地版本管理，Nova 让完整创作流程集中在一个 IDE 化工作台里。
</p>

<p align="center">
  <a href="README.en.md">English</a> | 中文
</p>

<p align="center">
  <a href="https://github.com/alfredxw/nova/releases"><img alt="Release" src="https://img.shields.io/github/v/release/alfredxw/nova?style=flat-square"></a>
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/github/license/alfredxw/nova?style=flat-square"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat-square&logo=go&logoColor=white">
  <img alt="Node.js" src="https://img.shields.io/badge/Node.js-20%2B-5FA04E?style=flat-square&logo=nodedotjs&logoColor=white">
</p>

<p align="center">
  当前版本：<strong>v0.1.6</strong>（2026-06-05） · Beta
</p>

![Nova 小说 IDE](./img/ide.png)

<details>
<summary>查看更多界面截图</summary>

### 互动故事工作台

![Nova 互动故事工作台](./img/interactive.png)

### 资料库

![Nova 资料库](./img/setting.png)

### 叙事编排配置

![Nova 叙事编排](./img/story-teller.png)

</details>

## 为什么选择 Nova

Nova 不是一个简单的聊天框，也不是只负责补全文字的编辑器。它把小说创作拆成可持续推进的工作流，并让 AI Agent 在有边界的上下文里协作：它能读选区、读文件、引用资料库、调用工具、写入草稿、维护状态，并在重要节点留下本地版本。

- **像 IDE 一样管理作品**：文件树、Markdown 编辑器、多 Tab、章节统计、全局搜索和 AI 侧栏共同组成稳定的写作桌面。
- **让 Agent 参与完整流程**：脑暴、顶层设定、大纲、章节组细纲、草稿、正文、定稿和状态同步都有明确入口。
- **同时支持写作与试演**：IDE 模式负责生产内容，互动模式负责试跑剧情、探索分支和验证角色行动。
- **把设定沉淀成结构化资产**：角色、世界观、地点、势力、规则和物品进入资料库，章节后的角色当前状态独立追踪。
- **默认保护创作过程**：Nova 使用 go-git 在作品根目录维护本地 `.git` 仓库，支持手动保存、历史查看、差异对比、恢复和自动保存；正文、设置和 `.nova` 内的 lore/session 等创作状态都会保存，无需安装系统 Git、手动初始化仓库或额外的 `.nova/versions` 元数据目录。

## 核心能力

| 模块 | 能力 |
| --- | --- |
| 小说 IDE | 文件树、Markdown 编辑器、多 Tab、章节统计、全局搜索、AI 对话区 |
| 创作 Agent | 流式输出、工具调用、选区引用、`@` 文件引用、`#` 风格参考、待办追踪 |
| 章节工作流 | 脑暴、设定、大纲、章节组细纲、草稿、正文、定稿状态同步 |
| 互动故事 | 剧情分支、下一步行动候选、场景记忆、故事线切换、路线图 |
| 资料库 | 结构化维护角色、世界观、地点、势力、规则、物品等长期设定 |
| 叙事编排 | 按作品或场景配置叙述规则、风格约束、节奏偏好和互动生成策略 |
| 现有小说导入 | txt / md 上传后由工具 Agent 智能识别章节标题正则，可预览、调整样本字数和正则后再确认创建新书 |
| 角色卡导入 | 支持 SillyTavern v2 PNG / JSON 导入当前作品或创建新作品 |
| 版本管理 | go-git 管理的 workspace `.git`、包含 `.nova` 创作状态的历史记录、Diff 对比、恢复、定时与 Agent 输出自动保存 |
| 分层配置 | 支持全局、用户级和工作区级配置，适配不同模型与不同作品 |

## 推荐创作流程

```text
灵感脑暴
  ↓
顶层设定与创作规则
  ↓
大纲与章节组细纲
  ↓
单章草稿 / 正文生成
  ↓
定稿并同步进度与角色状态
  ↓
互动模式试演剧情分支
  ↓
资料库与版本持续沉淀
```

Nova 会尽量把展示用历史、模型上下文、资料库正文、工具结果和工作区状态分开处理，让 Agent 只拿到本轮任务真正需要的、有来源和上限的上下文。

## 快速开始

### 方式一：下载 Release

从 [GitHub Releases](https://github.com/alfredxw/nova/releases) 下载对应平台压缩包，解压后运行：

```bash
./nova
```

指定作品目录启动：

```bash
./nova --workspace /path/to/your-novel
```

Windows 用户运行 `nova.exe`。macOS 如果提示安全限制，可以执行：

```bash
xattr -dr com.apple.quarantine nova
```

### 方式二：从源码运行

需要 Go 1.26+、Node.js 20+ 和 pnpm。

```bash
git clone https://github.com/alfredxw/nova.git
cd nova
corepack enable
./bootstrap.sh
```

默认地址：

- 前端：`http://localhost:5173`
- 后端：`http://localhost:8080`

## 模型与配置

Nova 使用 OpenAI 兼容接口，可通过环境变量快速配置：

```bash
export OPENAI_API_KEY="your-api-key"
export OPENAI_BASE_URL="https://api.deepseek.com"
export OPENAI_MODEL="deepseek-v4-pro"
```

常用环境变量：

```bash
export NOVA_WORKSPACE="/path/to/your-novel"
export NOVA_DIR="./.nova"
export NOVA_SKILLS_DIR="./skills"
export NOVA_WEB_DIR="./web"
export NOVA_BACKEND_PORT="8080"
export NOVA_FRONTEND_PORT="5173"
```

也可以在 `config.toml` 中配置模型、Agent 参数、编辑器、互动模式、版本管理和界面语言。配置优先级：

```text
内置默认值 < 全局 config.toml < 用户级配置 < 工作区级配置 < 环境变量
```

## 作品目录

启动后，如果没有指定或恢复到作品，Web UI 会进入「书籍管理」。一个 workspace 对应一本书，推荐结构：

```text
my-novel/
├── CREATOR.md
├── brainstorm.md
├── chapters/
├── setting/
│   ├── progress.md
│   ├── character-states.md
│   └── chapter-groups/
├── drafts/
└── .nova/
    ├── lore/
    └── sessions/
```

常用入口：

- **写作**：编辑章节、查看目录树、搜索项目文件，并与创作 Agent 协作。
- **导入现有小说**：在书籍管理上传 txt/md，先预览工具 Agent 识别出的章节分割正则和章节效果；需要时可调整样本字数或手动编辑 Go regexp，确认后再创建新书并写入 `chapters/`。
- **互动**：试跑剧情、探索分支、切换故事线，并维护场景记忆。
- **资料库**：维护角色、世界观、地点、势力、规则和物品；角色当前位置、伤势、心理和目标等当前状态由 `setting/character-states.md` 追踪。
- **叙事编排**：配置叙述视角、节奏、风格规则和互动生成偏好。
- **版本管理**：手动保存版本、查看历史和差异、恢复旧版本，并支持定时与 Agent 大量输出自动保存；`.nova/lore`、`.nova/sessions` 等本地创作状态会进入版本，历史直接来自 workspace `.git`。
- **设置**：调整模型、编辑器、Agent、互动模式、外观和语言。

## 开发

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

生产构建：

```bash
./build.sh
```

运行构建产物：

```bash
cd output
./nova --workspace /path/to/your-novel
```

## 技术栈

- Backend：Go、Hertz、Eino、SSE
- Frontend：React、TypeScript、Vite、Tailwind CSS、TipTap
- State：TanStack Query、Zustand
- Packaging：GitHub Actions、跨平台 Go binaries

## 项目结构

```text
.
├── cmd/nova/        # 服务入口
├── config/          # 配置加载
├── internal/        # 后端业务模块
├── scripts/         # 构建和发布脚本
├── skills/          # 创作技能提示词
└── web/             # React Web UI
```

## 发布

本地打包 GitHub Release：

```bash
scripts/build-github-release.sh v0.1.6
```

推送 tag 后，GitHub Actions 会自动创建或更新 Release：

```bash
git tag v0.1.6
git push origin v0.1.6
```

## License

[Apache-2.0](./LICENSE)
