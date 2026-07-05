<p align="center">
  <img src="./web/public/favicon.svg" alt="Denova 图标" width="76" height="76">
</p>

<p align="center">
  <strong>Denova 一个面向小说创作与 AI 角色扮演游戏的 AI 创作平台，内置支持 AI Agents、Skills、Subagent Workflows、自动化、图像自动生成与项目版本管理等核心能力</strong>
</p>

<p align="center">
  <a href="README.en.md">English</a> | 中文
</p>

<p align="center">
  <a href="https://github.com/alfredxw/denova/releases"><img alt="Release" src="https://img.shields.io/github/v/release/alfredxw/denova?style=flat-square"></a>
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/github/license/alfredxw/denova?style=flat-square"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat-square&logo=go&logoColor=white">
  <img alt="Node.js" src="https://img.shields.io/badge/Node.js-20%2B-5FA04E?style=flat-square&logo=nodedotjs&logoColor=white">
</p>

<p align="center">
  当前版本：<strong>v0.1.18</strong>（2026-07-01） · Beta
</p>

![Denova 写作模式](./img/ide.png)

<details>
<summary>查看更多界面截图</summary>

### 游戏模式

![Denova 游戏模式](./img/interactive.png)

### 剧情分支

![剧情分支](./img/branch.png)

### 资料库

![Denova 资料库](./img/setting.png)

### 方案预设

![Denova 方案预设](./img/story-teller.png)

</details>

## 为什么选择 Denova

Denova 面向长期创作项目和互动娱乐，把写作 IDE、互动故事、结构化资料库、Agent 工具调用、图像生成、自动化和本地版本管理放在同一个项目工作区里，让创作过程可以反复迭代、回溯和沉淀。

你可以从原创灵感开始，也可以导入已有小说做同人、改编或续写；还可以导入 AI 酒馆角色卡，快速搭建互动文字冒险。模型上下文会按来源、用途和大小上限组织，避免把完整历史、日志或全部设定无界塞进下一轮对话。

## 核心能力

- **写作模式**：面向小说创作，支持 Markdown 编辑、多 Tab、全局搜索、章节统计、大纲、章节组细纲、进度追踪和现有小说导入。
- **创作 Agent**：可读取选区、文件和资料库，调用工具生成或修改章节，并通过 Skills / SubAgents 适配不同写作任务、文风和工作流。
- **游戏模式**：运行互动文字冒险，支持玩家输入、剧情分支、故事线切换、行动建议、场景记忆、长期故事记忆，以及由故事导演驱动的目标、压力、代价、事件卡包和规则检定。
- **资料库与预设**：沉淀角色、世界观、地点、势力、规则、物品等稳定设定；叙事风格负责文风、提示词槽位和场景风格，故事导演可插拔组合叙事风格、事件系统、数值/TRPG 系统、开局选择器和图像方案，且每个模块都可独立关闭。
- **图像创作**：支持章节插画、互动图像和书籍封面生成，复用 OpenAI 兼容图像模型配置，并在界面中预览和管理结果。
- **上下文管理**：渐进式组织模型可见上下文，支持 Memory Compact、缓存优化和有界工具结果，降低长篇创作的上下文噪音与 token 成本。
- **版本与恢复**：基于本地 Git 保存版本、查看 Diff、恢复历史，并支持定时保存和 Agent 大量输出后的自动保存。
- **自动化**：支持定时任务、Review、自动续写和自定义 Prompt 工作流。
- **产品化体验**：中英文界面、浅色/深色主题、OpenAI 兼容模型配置、远程访问、PWA 手机使用，以及 Windows / macOS / Linux 全平台支持。

## 写作模式与游戏模式

Denova 有两个并列工作台。写作模式关注小说生产线：构思、设定、大纲、章节细纲、正文和进度；游戏模式关注可游玩的互动叙事：玩家行动、剧情分支、场景记忆、故事线和选择推进。

游戏模式内置故事导演能力：互动 Agent 负责理解玩家行动并生成本回合 TurnBrief，后端工具只执行字段校验、数值结算、骰子/表达式检定等确定性工作；后台 Director Agent 在每个回合落盘后维护分支级三层 Markdown 规划，包括大方向、当前主线事件和最近分支安排。互动正文只读取这些文档中的可见区，导演私密区只提供给后台导演；热选项只读取最近分支安排的可见区。创作者可以在故事导演中组合或关闭叙事风格、事件系统、规则系统、开局选择器和图像方案，并可配置分支规划回合数与三份规划模板。事件卡包含类型名、Markdown 事件描述、触发场景、回收方式和调度元数据，作为导演规划输入而不是运行时事件队列。配置 Agent 可先读取资料库索引和少量相关条目，再基于具体世界观生成事件卡包。也可以配置开局词条抽取、数值属性和可复用规则检定模板。

两种模式会共享适合长期复用的资产，例如资料库、方案预设、模型与 Agent 配置、Skills、版本管理和基础设置。写作进度、章节细纲等生产状态不会自动进入游戏模式；如果互动故事需要引用某段正文或当前进度，建议先把稳定信息沉淀进资料库，或在输入中明确引用。

## 欢迎交流

Denova 仍在快速迭代中，欢迎反馈问题、分享用法或一起讨论创作工作流。

<p align="center">
  <img src="./img/wechat.png" alt="微信交流" width="240">
</p>

## 快速开始

### 下载 Release

从 [GitHub Releases](https://github.com/alfredxw/denova/releases) 下载对应平台压缩包，解压后运行：

```bash
./denova
```

Windows 用户运行 `denova.exe`。macOS 如果提示安全限制，可以执行：

```bash
xattr -dr com.apple.quarantine denova
```

### 从源码运行

需要 Go 1.26+、Node.js 20+ 和 pnpm。

```bash
git clone https://github.com/alfredxw/denova.git
cd denova
corepack enable
./bootstrap.sh
```

默认地址：

- 前端：`http://localhost:5173`
- 后端：`http://localhost:8080`

## 模型与配置

Denova 使用 OpenAI 兼容接口。推荐先在设置页配置语言模型、图像模型、Agent 参数、默认写作 Skill、编辑器、游戏模式、版本管理、语言、主题和字体。

需要脚本化启动或部署时，也可以用环境变量覆盖模型配置：

```bash
export OPENAI_API_KEY="your-api-key"
export OPENAI_BASE_URL="https://api.deepseek.com"
export OPENAI_MODEL="deepseek-v4-pro"
export OPENAI_IMAGE_API_KEY="your-openai-image-key"
export OPENAI_IMAGE_BASE_URL="https://api.openai.com/v1"
export OPENAI_IMAGE_MODEL="gpt-image-1"
```

可选 Denova 启动环境变量：

```bash
export DENOVA_WORKSPACE="/path/to/your-workspace"
export DENOVA_DIR="./.denova"
export DENOVA_SKILLS_DIR="./skills"
export DENOVA_WEB_DIR="./web"
export DENOVA_BACKEND_PORT="8080"
export DENOVA_FRONTEND_PORT="5173"
```

配置优先级：

```text
内置默认值 < 全局 config.toml < 用户级配置 < 工作区级配置 < 环境变量
```

旧工作区和旧环境变量仍会兼容读取；新配置建议使用 `.denova` / `DENOVA_*`。

## 远程访问与手机使用

Denova 可以在本机、局域网或自托管服务器上使用。Release 包已包含前端资源；从源码部署时可先构建前端：

```bash
pnpm --dir web build
```

在 **设置页 → 远程访问** 开启「允许局域网访问」并设置用户名和密码后，其他设备可以打开设置页展示的访问地址。手机浏览器登录后可添加到主屏幕，以接近独立应用的方式使用。

如果要通过公网或域名访问，建议使用 Caddy / Nginx 等反向代理提供 HTTPS，避免凭据明文传输，并确保浏览器剪贴板、PWA 等能力正常工作。

Caddy 示例：

```text
denova.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

## 开发

启动前后端：

```bash
./bootstrap.sh
```

分开启动前端或后端：

```bash
./bootstrap.sh fe
./bootstrap.sh be
```

允许局域网设备访问前端开发服务：

```bash
./bootstrap.sh fe --lan
```

## 赞助项目

> 给项目冲点token，帮助这个项目持续迭代，持续开源，你的支持真的很重要！非常感谢！

<p align="center">
  <img src="./img/donate.png" alt="捐赠" width="240">
</p>

## Star History

<a href="https://www.star-history.com/#alfredxw/denova&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=alfredxw/denova&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=alfredxw/denova&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=alfredxw/denova&type=date&legend=top-left" />
 </picture>
</a>

## License

[Apache-2.0](./LICENSE)
