<p align="center">
  <img src="./web/public/favicon.svg" alt="Nova 图标" width="76" height="76">
</p>

<p align="center">
  <strong>Nova 是一个 AI-native 的小说创作 IDE，由强大的 Agent 协助你完成规划、写作、修订与叙事编排，覆盖长篇小说和互动故事的完整创作链路</strong>
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
  当前版本：<strong>v0.1.10</strong>（2026-06-12） · Beta
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

Nova 的核心不是生成一段文本，而是让创作者持续掌控一部长篇作品。作品文件、资料库、章节状态、互动试演、Agent 工具调用和本地版本都会留在同一个 workspace 里，让规划、写作、修订和剧情编排保持在同一条清晰的生产线上。

如果你希望 AI 不只是帮你补一段正文，而是能围绕同一本书反复协作、读取有边界的上下文、沉淀设定、维护章节进度，并在重要节点留下可恢复版本，Nova 会更适合你。

| 你关心的事 | Nova 的做法 |
| --- | --- |
| 长篇作品管理 | 像 IDE 一样提供文件树、Markdown 编辑器、多 Tab、章节统计、全局搜索和 AI 侧栏 |
| AI 协作深度 | Agent 可读选区、读文件、引用资料库、调用工具、追踪待办并写入草稿 |
| 创作流程连续性 | 覆盖灵感、设定、大纲、章节组细纲、草稿、正文、定稿和状态同步 |
| 剧情验证 | IDE 模式写作，互动模式试跑剧情分支、角色行动、场景记忆和故事线 |
| 设定沉淀 | 角色、世界观、地点、势力、规则和物品进入结构化资料库，角色当前状态单独追踪 |
| 个性化工作流 | 支持内置与自定义 Skills、叙事编排、分层配置和不同作品的模型设置 |
| 版本安全感 | 使用 go-git 在作品目录维护本地 `.git`，支持历史、Diff、恢复、定时保存和 Agent 输出自动保存 |

推荐从灵感开始，先整理顶层设定与创作规则，再生成大纲和章节组细纲；进入单章写作后，用 Agent 生成草稿或正文，定稿时同步进度与角色状态；需要检验剧情时切到互动模式试演分支，最后把稳定设定沉淀进资料库并持续保存版本。

Nova 会尽量把展示用历史、模型上下文、资料库正文、工具结果和工作区状态分开处理，让 Agent 只拿到本轮任务真正需要的、有来源和上限的上下文。

## 欢迎交流
快速迭代中，欢迎交流反馈，加下群响应更及时
<p align="center">
  <img src="./img/wechat.jpg" alt="微信交流" width="240">
</p>

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

也可以在 `config.toml` 中配置模型、Agent 参数、编辑器、互动模式、版本管理和界面外观（语言、主题、字体）。`theme` 支持 `dark`（默认）、`light` 和 `system`，可保存到用户级或工作区级配置。`NOVA_SKILLS_DIR` / `skills_dir` 用于内置只读 Skills；自定义 Skills 可通过界面写入 `<nova_dir>/skills` 或 `<workspace>/.nova/skills`。配置优先级：

```text
内置默认值 < 全局 config.toml < 用户级配置 < 工作区级配置 < 环境变量
```

## 作品目录

启动后，如果没有指定或恢复到作品，Web UI 会进入「书籍管理」。一个 workspace 对应一本书，推荐结构：

```text
my-novel/
├── CREATOR.md
├── ideas.md
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
scripts/build-github-release.sh v0.1.10
```

推送 tag 后，GitHub Actions 会自动创建或更新 Release：

```bash
git tag v0.1.10
git push origin v0.1.10
```

## License

[Apache-2.0](./LICENSE)
