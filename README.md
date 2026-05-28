# Nova

Nova 是一个面向长篇小说创作的 AI 写作工作台，目标是提供类似 IDE 的创作体验，覆盖灵感、设定、大纲、分卷、分章、细纲、正文、版本管理和一致性检查等流程。

当前版本：v0.1.4（2026-05-29）

## 截图

![Nova 截图](./img/screenshot.png)

## 功能特性

- **小说 IDE 工作台**：左侧项目结构、中间 TipTap 编辑器、右侧创作 Agent 和版本管理面板。
- **AI 创作 Agent**：支持 SSE 流式输出、工具调用展示、思考过程折叠、任务中断和活跃任务恢复。
- **章节编辑**：支持 Markdown / TXT 编辑、自动保存、手动保存、字数统计、文章内搜索和选区引用。
- **互动故事工作台**：支持故事线、剧情分支、下一步行动候选、场景记忆和可中断的流式生成。
- **结构化资料库**：支持角色、世界观、地点、势力、规则和物品等 Lore Item 管理，并提供资料库 Agent 辅助整理。
- **角色卡导入**：支持导入 SillyTavern 酒馆 v2 PNG / JSON 角色卡，可导入当前书籍或创建新书。
- **多书籍管理**：记录最近打开书籍，支持切换 workspace，并在后端重启后恢复上次书籍。
- **多会话管理**：每个书籍 workspace 支持多个独立会话，`/clear` 只追加上下文清理标记，不删除历史。
- **风格参考**：支持在 `setting/styles/` 中维护 `.md` / `.txt` 文风样本，并通过 `#` 在本轮对话中引用。
- **版本管理**：每个书籍 workspace 可作为独立 Git 仓库，支持初始化、创建版本、查看历史、回滚、stash 和 pop。
- **前端稳定性**：内置 React 崩溃边界、全局运行时日志、白屏检测、Vitest 测试和 shadcn/Radix 基础组件。

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

## 快速开始

### 环境要求

- Go 1.24+
- Node.js 20+
- pnpm

### 配置密钥

推荐使用环境变量管理敏感信息，不要把真实密钥提交到仓库：

```bash
export OPENAI_API_KEY="your-api-key"
export OPENAI_BASE_URL="https://api.deepseek.com"
export OPENAI_MODEL="deepseek-v4-pro"
```

可选环境变量：

```bash
export NOVA_WORKSPACE="/path/to/your-novel"
export NOVA_DIR="./.nova"
export NOVA_SKILLS_DIR="./skills"
export NOVA_BACKEND_PORT="8080"
export NOVA_FRONTEND_PORT="5173"
```

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

## 构建

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

## 作品 Workspace 约定

一个 workspace 对应一本书，推荐结构：

```text
my-novel/
├── CREATOR.md
├── chapters/
│   ├── ch01-废材开局.md
│   └── ch02-初入宗门.md
├── setting/
│   ├── outline.md
│   ├── progress.md
│   ├── characters.md
│   └── styles/
│       ├── 古龙.md
│       └── 番茄.txt
└── .nova/
    ├── config.toml
    └── sessions/
```

说明：

- `CREATOR.md` 是作品级最高优先级创作者指令。
- `chapters/` 存放正文，章节文件建议遵循 `chXX-章节名.md`。
- `setting/styles/` 存放文风参考，可在 AI 输入框中通过 `#` 引用。
- `.nova/config.toml` 存放当前书籍的工作区级配置，例如互动模式单轮目标字数 `interactive_reply_target_chars`，以及可选最大输出 Token `interactive_max_tokens`。
- `.nova/sessions/` 存放会话历史。

## 常用交互

- `Cmd/Ctrl + S`：保存当前章节
- `Cmd/Ctrl + K`：打开命令面板
- `Cmd/Ctrl + Enter`：触发当前章节续写
- `Cmd/Ctrl + Shift + D`：打开版本管理
- `Esc`：关闭右侧面板或弹窗
- `@文件路径`：本轮引用工作区文件
- `#风格文件`：本轮引用风格参考
- `/plan`：先规划再执行
- `/clear`：追加上下文清理标记
- `/continue`：继续写作
- `/rewrite`：重写章节
- `/outline`：生成或调整大纲

## 版本管理说明

Nova 的版本管理基于书籍 workspace 内的本地 Git 仓库：

- 初始化版本仓库
- 创建版本
- 查看版本历史
- 回滚到指定版本
- 暂存未提交内容
- 恢复最近暂存内容

前端只负责展示和触发操作，实际 Git 逻辑由 Go 后端执行。

## 开发约定

- 前端负责 UI 交互，后端负责 AI Agent 运行、文件管理和版本管理。
- 服务端数据使用 React Query，本地 UI 状态使用 Zustand。
- 基础交互组件优先使用 shadcn/ui 和 Radix UI，避免手写复杂底层组件。
- 修改关键逻辑时补充 Go 单测或 Vitest 测试。
- 提交前更新 `CHANGELOG.md`，遵循 Keep a Changelog 格式。

## 发布记录

最新版本：v0.1.4（2026-05-29）。完整记录见 [CHANGELOG.md](./CHANGELOG.md)。
