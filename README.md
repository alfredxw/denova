# Nova

Nova 是一个面向长篇小说创作的 AI 写作工作台，提供类似 IDE 的创作体验，覆盖灵感、设定、大纲、章节写作、互动试演、资料库和版本管理。

当前版本：v0.1.5（2026-06-02）

## Screenshots

![Nova 小说 IDE](./img/ide.png)

<details>
<summary>更多截图</summary>

### 互动故事工作台

![Nova 互动故事工作台](./img/interactive.png)

### 资料库

![Nova 资料库](./img/setting.png)

### 故事风格配置

![Nova 故事风格](./img/story-teller.png)

</details>

## Features

- 小说 IDE：文件树、Markdown 编辑器、多 Tab、章节统计和 AI 对话区。
- 创作 Agent：支持流式输出、工具调用、选区引用、`@` 文件引用和 `#` 风格参考。
- 章节工作流：支持脑暴、设定、大纲、章节组细纲、草稿和正文。
- 互动故事：支持剧情分支、下一步行动候选、场景记忆和故事线切换。
- 资料库：结构化维护角色、世界观、地点、势力、规则和物品等长期设定。
- 角色卡导入：支持 SillyTavern v2 PNG / JSON。
- 本地版本管理：每本书可作为独立 Git 仓库创建版本、查看历史和回滚。
- 分层配置：支持全局、用户级和工作区级配置。

## Install

### GitHub Release

从 [Releases](https://github.com/alfredxw/nova/releases) 下载对应平台压缩包，解压后运行：

```bash
./nova
```

指定作品目录：

```bash
./nova --workspace /path/to/your-novel
```

Windows 用户运行 `nova.exe`。macOS 如果提示安全限制，可以执行：

```bash
xattr -dr com.apple.quarantine nova
```

### From Source

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

## Configuration

Nova 使用 OpenAI 兼容接口。可以通过环境变量配置：

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

也可以在 `config.toml` 中配置模型、Agent 参数、编辑器和互动模式设置。配置优先级：

```text
内置默认值 < 全局 config.toml < 用户级 < 工作区级 < 环境变量
```

## Usage

启动后，如果没有指定或恢复到书籍，WebUI 会进入「书籍管理」。一个 workspace 对应一本书，推荐结构：

```text
my-novel/
├── CREATOR.md
├── 脑暴.md
├── chapters/
├── setting/
├── drafts/
└── .nova/
```

常用入口：

- 写作：编辑章节、查看目录树、与创作 Agent 协作。
- 互动：试跑剧情、探索分支、维护讲述者规则。
- 资料库：维护角色、世界观、地点、势力、规则和物品。
- 版本管理：初始化 Git 仓库、创建版本、查看历史和回滚。
- 设置：调整模型、编辑器、Agent 和互动模式参数。

## Development

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

## Release

GitHub Release 本地打包：

```bash
scripts/build-github-release.sh v0.1.5
```

推送 tag 后，GitHub Actions 会自动创建或更新 Release：

```bash
git tag v0.1.5
git push origin v0.1.5
```

## Tech Stack

- Backend: Go, Hertz, Eino, SSE
- Frontend: React, TypeScript, Vite, Tailwind CSS, TipTap
- State: TanStack Query, Zustand
- Packaging: GitHub Actions, cross-platform Go binaries

## Project Structure

```text
.
├── cmd/nova/        # 服务入口
├── config/          # 配置加载
├── internal/        # 后端业务模块
├── scripts/         # 构建和发布脚本
├── skills/          # 创作技能提示词
└── web/             # React WebUI
```

## License

[Apache-2.0](./LICENSE)
