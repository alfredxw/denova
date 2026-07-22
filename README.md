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
  <a href="https://discord.gg/QuHu2aPya"><img src="https://img.shields.io/badge/Discord-5865F2?logo=discord&logoColor=white" alt="加入 Denova Discord" /></a>
  <a href="https://github.com/alfredxw/denova/releases"><img alt="Release" src="https://img.shields.io/github/v/release/alfredxw/denova?style=flat-square"></a>
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/github/license/alfredxw/denova?style=flat-square"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat-square&logo=go&logoColor=white">
  <img alt="Node.js" src="https://img.shields.io/badge/Node.js-20%2B-5FA04E?style=flat-square&logo=nodedotjs&logoColor=white">
</p>

<p align="center">
  当前版本：<strong>v0.3.0</strong>（2026-07-22） · Beta
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

- **写作模式**：面向小说创作，支持 Markdown 编辑、多 Tab、正则查找与替换、带自动备份的工作区全局替换、章节统计、大纲、章节组细纲、进度追踪、正文评论、Change Review 和现有小说导入。
- **创作 Agent**：可读取选区、文件、资料库和可信审阅意见，调用工具生成或修改章节，并通过 Skills / SubAgents 适配不同写作任务、文风和工作流；修改可在累计 Diff 中审阅、评论与撤销。
- **游戏模式**：运行互动文字冒险，支持玩家输入、剧情分支、故事线切换、后台导演运行策略、行动建议、已保存 AI 回复修正、可检索回合历史、可归档恢复的 Actor State 与自定义状态布局，以及由全屏导演台管理的目标、压力、代价、事件卡包和规则检定。
- **资料库与预设**：沉淀角色、世界观、地点、势力、规则、物品等稳定设定；叙事风格负责文风、提示词槽位和场景风格，故事导演可插拔组合叙事风格、事件包、TRPG 检定、状态系统和图像方案，且每个模块都可独立关闭；状态系统同时提供可复用词条库，模板决定各类 Actor 创建时的抽取规则。
- **图像创作**：支持章节插画、互动图像和书籍封面生成，复用 OpenAI 兼容图像模型配置，并在界面中预览和管理结果。
- **上下文管理**：渐进式组织模型可见上下文，支持上下文来源展开与一键复制、带回合来源的历史检查点、缓存优化和有界工具结果，降低长篇创作的上下文噪音与 token 成本。
- **版本与恢复**：基于本地 Git 保存版本、查看 Diff、恢复历史，并通过工作区变更账本提供跨重启的 Agent 修改 Undo/Redo，以及定时和 Agent 大量输出后的自动保存。
- **自动化**：支持定时任务、Review、自动续写和自定义 Prompt 工作流。
- **产品化体验**：中英文界面、浅色/深色主题、OpenAI 兼容模型配置、远程访问、PWA 手机使用，以及 Windows / macOS / Linux 全平台支持。

## 写作模式与游戏模式

Denova 有两个并列工作台。写作模式关注小说生产线：构思、设定、大纲、章节细纲、正文和进度；游戏模式关注可游玩的互动叙事：玩家行动、剧情分支、回合历史、Actor State、故事线和选择推进。

游戏模式内置故事导演，会在首个场景开始前结合开局设定和资料库，安排当前舞台、关键角色与势力、线索、风险和近期分支；游戏进行中，它会持续根据玩家的选择调整剧情方向，在尊重自由行动的同时维持人物动机、世界规则和伏笔的连贯。资料库中的重要角色、地点、势力与规则会优先进入剧情，让创作者沉淀的设定真正参与冒险。每个回合都会努力带来新的信息、关系变化、压力、收益、代价或悬念，并在合适的位置提供可继续行动的建议。

创作者可以自由组合叙事风格、事件包、TRPG 检定、状态系统和图像方案，也可以按故事需要关闭其中任一模块。事件包为导演提供可选的剧情素材，支持埋设、推进、回收和放弃；状态系统会从真实开局中适配故事需要长期追踪的属性、资源、关系、伤势与词条；TRPG 检定支持固定 d20 规则和状态加成。历史事实以已提交 Turn 为真源，当前可计算事实归 Actor State，稳定设定归资料库，未来意图归 `director.md`；Agent 可通过有界的回合历史检索找回早期事实，不再维护第二套可写真源。全屏导演台集中展示与编辑当前规划、事件和执行审计，右侧状态栏持续呈现角色与世界变化；已保存的 AI 回复也可以直接修正，而无需重新生成回合。

两种模式会共享适合长期复用的资产，例如资料库、方案预设、模型与 Agent 配置、Skills、版本管理和基础设置。写作进度、章节细纲等生产状态不会自动进入游戏模式；如果互动故事需要引用某段正文或当前进度，建议先把稳定信息沉淀进资料库，或在输入中明确引用。

## 欢迎交流

Denova 仍在快速迭代中，欢迎反馈问题、分享用法或一起讨论创作工作流。

加入 [Discord 社区](https://discord.gg/QuHu2aPya) 一起交流。

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

需要 Go 1.26.5+、Node.js 20+、pnpm 和 ripgrep。

```bash
git clone https://github.com/alfredxw/denova.git
cd denova
corepack enable
./scripts/bootstrap.sh
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
内置默认值 < 全局 config.toml < 用户级配置 < 环境变量
```

设置页中的通用、写作与游戏偏好统一保存为用户配置。工作区 `.denova/config.toml` 只承载 Agent 页明确提供的工作区定制；旧文件中的其他字段会保留，但不再覆盖用户设置。旧环境变量仍会兼容读取；新配置建议使用 `.denova` / `DENOVA_*`。

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
./scripts/bootstrap.sh
```

分开启动前端或后端：

```bash
./scripts/bootstrap.sh fe
./scripts/bootstrap.sh be
```

停止当前仓库中运行的 Denova 后端并以前台方式重启：

```bash
./scripts/restart-backend.sh
```

允许局域网设备访问前端开发服务：

```bash
./scripts/bootstrap.sh fe --lan
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
