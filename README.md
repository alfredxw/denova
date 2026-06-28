<p align="center">
  <img src="./web/public/favicon.svg" alt="Nova 图标" width="76" height="76">
</p>

<p align="center">
  <strong>Nova 是面向创作者的 AI-native 创作工作台：用写作模式管理小说创作，用游戏模式运行互动文字冒险，并把资料库、方案预设、上下文、版本与自动化留在同一个可持续迭代的 workspace 里</strong>
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
  当前版本：<strong>v0.1.17</strong>（2026-06-27） · Beta
</p>

![Nova 写作模式](./img/ide.png)

<details>
<summary>查看更多界面截图</summary>

### 游戏模式工作台

![Nova 游戏模式工作台](./img/interactive.png)

### 剧情分支

![剧情分支](./img/branch.png)

### 资料库

![Nova 资料库](./img/setting.png)

### 方案预设配置

![Nova 方案预设](./img/story-teller.png)

</details>

## 为什么选择 Nova

Nova 不是“输入提示词，生成一段正文”的一次性工具，而是为长期创作和互动娱乐准备的完整工作台。它把作品文件、Markdown 编辑、多 Tab、全局搜索、章节统计、结构化资料库、互动故事、Agent 工具调用和本地版本管理放在同一个 workspace 里，让小说创作和互动推进都能基于同一套长期资产持续迭代。

除了写自己的原创故事，Nova 也支持导入既有小说作为同人或改编起点，支持导入 AI 酒馆角色卡来快速建立互动预设。模型可见上下文会按来源和上限渐进式组织，资料库、文件片段、工具结果和历史展示彼此分离，避免把完整历史或全部设定无脑塞进下一轮模型输入。

- **写作模式**：围绕小说创作组织作品文件、大纲、章节组细纲、进度、Markdown 编辑、多 Tab、全局搜索和章节统计。
- **创作 Agent**：读取选区、读取文件、引用资料库、调用工具，并在 `chapters/` 下写入章节初稿。
- **章节插画**：创作 Agent 可通过内置 `chapter-illustration` Skill 为当前或指定章节生成一张非剧透插画，保存到 `assets/illustrations/` 后由作者手动插入 Markdown 正文。
- **结构化资料库**：角色、世界观、地点、势力、规则、物品等长期设定可沉淀为可检索资料。
- **渐进式上下文**：按来源、用途和大小上限组织模型上下文，避免无界注入历史、日志或完整设定。
- **游戏模式**：围绕互动文字冒险游戏推进可游玩的故事分支、角色行动、场景记忆和故事线变化。
- **自定义故事记忆**：支持为互动故事维护场景、故事线和自定义记忆字段，让长期游玩持续沉淀上下文。
- **Memory Compact 与缓存优化**：压缩长历史并稳定复用上下文，提高缓存命中率，降低持续创作的 token 成本。
- **版本管理**：基于 go-git 保存、Diff、恢复、定时保存，并在 Agent 大量输出时自动保存。
- **写作 Skills 与 SubAgents**：内置 Lite / Standard / Heavy 写作 Skill Preset，默认 Lite；也可给不同 Agent 配置自定义技能、提示词、可用工具和文风。
- **自动化**：支持定时任务、review、自动续写和自定义 Prompt 工作流。
- **导入与预设**：可导入 AI 酒馆角色卡，也可导入既有小说用于同人、改编或续写。
- **产品化体验**：中英文界面、浅色/深色主题、OpenAI 兼容模型配置和 Windows/macOS/Linux 全平台。

### 写作模式与游戏模式

Nova 有两个并列工作台。写作模式面向小说创作，重点是大纲、章节组细纲、章节正文、创作进度和定稿后的状态同步；游戏模式面向互动文字冒险，重点是玩家输入、剧情分支、场景记忆、故事线切换和可继续推进的游玩体验。

两种模式只共享适合长期复用的创作资产，例如资料库、方案预设（叙事方案和图像方案）、模型与 Agent 配置、Skills、版本管理和基础工作区设置。写作模式里的大纲、章节组细纲、章节进展、`progress.md` 等创作生产线状态不会自动进入游戏模式；游戏模式也不会默认感知写作模式当前写到哪里。需要让互动故事参考某段正文或某个进度时，应先把稳定设定沉淀进资料库，或在游戏输入中明确引用。

推荐从灵感或导入开始：先整理顶层设定与创作规则，再在写作模式生成大纲和章节组细纲；进入单章写作后，用 Agent 在 `chapters/` 下生成章节初稿，作者确认成章后再同步进度与角色状态；需要进行互动文字冒险时，切到游戏模式基于共享资料库和方案预设创建可玩的分支，最后把真正稳定的设定再沉淀回资料库并持续保存版本。

## 欢迎交流
快速迭代中，欢迎交流反馈，加下群响应更及时
<p align="center">
  <img src="./img/wechat.png" alt="微信交流" width="240">
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
export OPENAI_IMAGE_API_KEY="your-openai-image-key"
export OPENAI_IMAGE_BASE_URL="https://api.openai.com/v1"
export OPENAI_IMAGE_MODEL="gpt-image-1"
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

也可以在UI设置页（对应 `config.toml`）中配置语言模型、图像模型、Agent 参数、默认写作 Skill（`writing_skill_default`，默认 `novel-lite`）、编辑器、游戏模式、版本管理和界面外观（语言、主题、字体）。图像生成首版接入 OpenAI 标准 Images API，支持多个 `image_api_profiles`，生成结果会保存到当前工作区 `assets/image/generated/`；图像尺寸不在设置页配置，由 Agent 在调用 `generate_image` 时从支持的 2K/3K/4K 尺寸中选择，输出格式仅支持 `png` 和 `jpeg`。章节插画复用同一套图像模型配置，创作 Agent 调用 `generate_image` 后会将图像和 `meta.json` 保存到 `assets/illustrations/`，聊天工具卡片展示预览，作者确认后再手动插入为 Markdown 图像。方案预设页可管理图像方案，内置 `游戏CG`、`写实`、`2D插画`，写作 Agent 输入菜单和游戏输入菜单都可选择当前图像方案，默认使用 `游戏CG`；这些方案只约束图像生成工具调用，不进入普通正文。游戏模式支持“互动图像”：默认手动生成，也可在输入框左侧菜单的侧边配置中切换为每 X 轮生成（默认 3 轮）；结果保存到 `assets/interactive/images/`，只作为回合 display event 展示，不写入叙事正文，也不进入下一轮模型上下文。`theme` 支持 `dark`（默认）、`light` 和 `system`，可保存到用户级或工作区级配置。`NOVA_SKILLS_DIR` / `skills_dir` 用于内置只读 Skills；自定义 Skills 可通过界面写入 `<nova_dir>/skills` 或 `<workspace>/.nova/skills`。需要修改内置预制 Skill 时，不编辑内置目录，默认在 `<nova_dir>/skills/<skill-name>/SKILL.md` 创建同名用户级覆盖；只有用户级目录不可写时才退回工作区覆盖。Skills 页也可修改 Skill 名称和保存位置，或删除覆盖版本以恢复内置版本。创作 Agent 不会把预设 SKILL.md 直接注入模型上下文，只会在本轮动态提示中说明当前选择的 Writing Skill；当模型判断本轮涉及正文写作/续写时，应通过 `skill` 工具自行加载对应 Skill。写作范围始终从用户指令判断，不使用单独的 `writing_scope` 字段。配置优先级：

```text
内置默认值 < 全局 config.toml < 用户级配置 < 工作区级配置 < 环境变量
```

写作模式生成章节时，如果不希望 `write_file` 的实时工具调用在前端刷出大量小说正文，可以在设置页「写作模式 / 实时输出」开启「隐藏章节正文实时输出」，或在 `config.toml` 中配置：

```toml
hide_novel_chapter_body_in_live_output = true
```

该配置默认关闭。开启后仅影响写作模式实时 SSE 展示：前端会显示目标路径和已生成字符数，统计口径与 `wc -m` 一致；章节正文仍会正常写入文件，Agent 内部事件、工具执行和会话历史不因此丢失。

## 作品目录

启动后，如果没有指定或恢复到作品，Web UI 会进入「书籍管理」。一个 workspace 对应一本书，推荐结构：

```text
my-novel/
├── CREATOR.md
├── ideas.md
├── assets/
│   └── illustrations/
├── chapters/
├── setting/
│   ├── progress.md
│   ├── character-states.md
│   └── chapter-groups/
└── .nova/
    ├── image-presets/
    ├── lore/
    └── sessions/
```

常用入口：

- **写作**：编辑章节、维护大纲与章节组细纲、查看目录树、搜索项目文件，并与创作 Agent 协作；Markdown 章节可一键请求生成本章插画，生成后在 Agent 工具卡片中预览并手动插入正文；写作进度由 `setting/progress.md` 追踪，角色当前位置、伤势、心理和目标等当前状态由 `setting/character-states.md` 追踪。
- **导入现有小说**：在书籍管理上传 txt/md，先预览工具 Agent 识别出的章节分割正则和章节效果；需要时可调整样本字数或手动编辑 Go regexp，确认后再创建新书并写入 `chapters/`。
- **游戏**：推进剧情、探索选择、切换故事线，并维护场景记忆。
- **资料库**：维护角色、世界观、地点、势力、规则和物品等长期稳定设定，供写作模式和游戏模式按需复用。
- **方案预设**：并列管理叙事方案和图像方案；叙事方案配置叙述视角、节奏和风格规则，图像方案配置写作插画和互动图像的视觉风格。
- **版本管理**：手动保存版本、查看历史和差异、恢复旧版本，并支持定时与 Agent 大量输出自动保存；`.nova/lore`、`.nova/sessions` 等本地创作状态会进入版本，历史直接来自 workspace `.git`。
- **设置**：调整模型、编辑器、Agent、游戏模式、外观和语言。

## 开发

启动前后端：

```bash
./bootstrap.sh
```

分开启动前端：

```bash
./bootstrap.sh fe/be
```

允许局域网设备访问前端开发服务：

```bash
./bootstrap.sh fe --lan
```

也可以在设置页开启“允许局域网访问”并重启 Nova。其他设备应打开设置页展示的访问地址；release 默认是 `http://<本机局域网IP>:8080`，开发前端模式通常是 `http://<本机局域网IP>:5173`。首次访问时在页面内输入远程访问用户名和密码；后端会拒绝未登录的远端请求。

## 赞助项目
> 给作者充点token，帮助这个项目持续迭代，持续开源，你的支持真的很重要！非常感谢！
<p align="center">
  <img src="./img/donate.png" alt="捐赠" width="240">
</p>

## Star History

<a href="https://www.star-history.com/#alfredxw/nova&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=alfredxw/nova&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=alfredxw/nova&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=alfredxw/nova&type=date&legend=top-left" />
 </picture>
</a>


## License

[Apache-2.0](./LICENSE)
