<p align="center">
  <img src="./web/public/favicon.svg" alt="Nova icon" width="76" height="76">
</p>

<p align="center">
  <strong>Nova is an AI-native creative workspace for storytellers: use Writing Mode for fiction creation, use Interactive Mode for interactive entertainment, and keep lore, narrative direction, bounded context, versions, and automation in one durable workspace.</strong>
</p>

<p align="center">
  English | <a href="README.md">中文</a>
</p>

<p align="center">
  <a href="https://github.com/alfredxw/nova/releases"><img alt="Release" src="https://img.shields.io/github/v/release/alfredxw/nova?style=flat-square"></a>
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/github/license/alfredxw/nova?style=flat-square"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat-square&logo=go&logoColor=white">
  <img alt="Node.js" src="https://img.shields.io/badge/Node.js-20%2B-5FA04E?style=flat-square&logo=nodedotjs&logoColor=white">
</p>

<p align="center">
  Current version: <strong>v0.1.13</strong> (2026-06-24) · Beta
</p>

![Nova Writing Mode](./img/ide.png)

<details>
<summary>View more screenshots</summary>

### Interactive Story Workspace

![Nova Interactive Story Workspace](./img/interactive.png)

### Branch

![Branch](./img/branch.png)

### Lore Library

![Nova Lore Library](./img/setting.png)

### Narrative Direction Configuration

![Nova Narrative Direction](./img/story-teller.png)

</details>

## Why Nova

Nova is not a one-off "prompt in, passage out" generator. It is a full workspace for long-running fiction projects and interactive entertainment. Book files, Markdown editing, multiple tabs, global search, chapter statistics, structured lore, interactive stories, Agent tool calls, and local version management live in the same workspace, so fiction creation and interactive play can keep iterating on the same durable assets.

Beyond writing original stories, Nova can import existing novels as a starting point for fan fiction, adaptation, or continuation, and it can import AI tavern character cards to quickly create interactive presets. Model-visible context is built progressively with explicit sources and limits, keeping lore, file excerpts, tool results, and display history separate instead of blindly injecting the entire project into every turn.

- **Writing Mode**: organize book files, outlines, chapter-group plans, progress, Markdown editing, multiple tabs, global search, and chapter statistics for fiction creation.
- **Creative Agents**: read selections, read files, reference lore, call tools, and write into drafts or chapters.
- **Structured lore**: characters, worlds, locations, factions, rules, items, and other durable settings become searchable long-term lore.
- **Progressive context**: model context is organized by source, purpose, and hard size limits instead of unbounded history, logs, or full settings.
- **Interactive Mode**: run playable story branches, character actions, scene memory, and storyline changes for interactive entertainment.
- **Custom story memory**: maintain scenes, storylines, and custom memory fields for interactive stories so long-running play keeps durable context.
- **Memory Compact and cache optimization**: compact long histories and reuse stable context to improve cache hits and reduce token cost during ongoing creation.
- **Version management**: go-git powered saves, diffs, restore, timed saves, and automatic saves for large Agent outputs.
- **Writing Skills and Agents**: built-in Lite / Standard / Heavy Writing Skill presets with Standard as the default, plus custom skills, prompts, tool permissions, and prose styles for different Agents.
- **Automation**: schedule tasks, reviews, auto-continuation, and custom Prompt workflows.
- **Imports and presets**: import AI tavern character cards or existing novels for fan fiction, adaptation, or continuation.
- **Product experience**: Chinese and English UI, light and dark themes, OpenAI-compatible model configuration, and Windows, macOS, and Linux support.

### Writing Mode and Interactive Mode

Nova has two parallel workspaces. Writing Mode is for fiction creation: outlines, chapter-group plans, chapter prose, writing progress, and state sync after finalizing chapters. Interactive Mode is for interactive entertainment: player input, story branches, scene memory, storyline switching, and a playable experience that can keep moving forward.

The two modes only share durable creative assets such as lore, narrative direction, model and Agent configuration, Skills, version management, and base workspace settings. Writing Mode state such as outlines, chapter-group plans, chapter progress, and `progress.md` does not automatically enter Interactive Mode; Interactive Mode also does not implicitly know where the novel is currently written to. If an interactive story should reference a passage or a writing milestone, first move stable setting into lore or explicitly reference it in the interactive input.

The recommended path is to start from an idea or an import: settle top-level settings and creative rules, then build the outline and chapter-group plan in Writing Mode. During chapter work, use Agents to draft or write prose and sync progress plus character state after finalizing. For interactive entertainment, switch to Interactive Mode and create playable branches from shared lore and narrative direction, then fold only truly stable setting back into lore and keep saving local versions.

## Community

Nova is iterating quickly. Feedback and discussion are welcome; joining the group usually gets a faster response.
<p align="center">
  <img src="./img/wechat.png" alt="WeChat group" width="240">
</p>

## Quick Start

### Option 1: Download a Release

Download the archive for your platform from [GitHub Releases](https://github.com/alfredxw/nova/releases), extract it, and run:

```bash
./nova
```

Start with a specific book workspace:

```bash
./nova --workspace /path/to/your-novel
```

Windows users should run `nova.exe`. On macOS, if the system blocks the app for security reasons, run:

```bash
xattr -dr com.apple.quarantine nova
```

### Option 2: Run from Source

Requires Go 1.26+, Node.js 20+, and pnpm.

```bash
git clone https://github.com/alfredxw/nova.git
cd nova
corepack enable
./bootstrap.sh
```

Default addresses:

- Frontend: `http://localhost:5173`
- Backend: `http://localhost:8080`

## Models and Configuration

Nova uses an OpenAI-compatible API. You can configure it quickly with environment variables:

```bash
export OPENAI_API_KEY="your-api-key"
export OPENAI_BASE_URL="https://api.deepseek.com"
export OPENAI_MODEL="deepseek-v4-pro"
```

Common environment variables:

```bash
export NOVA_WORKSPACE="/path/to/your-novel"
export NOVA_DIR="./.nova"
export NOVA_SKILLS_DIR="./skills"
export NOVA_WEB_DIR="./web"
export NOVA_BACKEND_PORT="8080"
export NOVA_FRONTEND_PORT="5173"
```

You can also configure models, Agent parameters, the default Writing Skill (`writing_skill_default`, default `novel-standard`), editor options, interactive-mode behavior, version management, backend/frontend ports, and interface appearance (language, theme, fonts) from the UI settings page, which maps to `config.toml`. `theme` supports `dark` (default), `light`, and `system`, and can be saved at the user or workspace level. `NOVA_SKILLS_DIR` / `skills_dir` is the built-in read-only Skills root; custom Skills can be written from the UI to `<nova_dir>/skills` or `<workspace>/.nova/skills`. To customize a built-in preset Skill, do not edit the built-in directory; Nova creates a same-name user-level override at `<nova_dir>/skills/<skill-name>/SKILL.md` by default, falling back to a workspace override only when the user-level directory is not writable. The Skills page can also rename a Skill, move it between user/workspace storage, or delete an override to restore the built-in version. The Writing Agent infers each turn's workflow from the selected Skill, while the actual writing range always comes from the user's instruction and does not use a separate `writing_scope` field. Configuration precedence:

```text
Built-in defaults < global config.toml < user-level config < workspace-level config < environment variables
```

## Book Workspace

After startup, if no book is specified or restored, the Web UI opens Book Management. One workspace maps to one book. Recommended structure:

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

Common entry points:

- **Writing**: edit chapters, maintain outlines and chapter-group plans, browse the file tree, search project files, and collaborate with the Writing Agent. Writing progress is tracked in `setting/progress.md`, while current character location, injuries, mental state, goals, and similar state live in `setting/character-states.md`.
- **Import Existing Novel**: upload a txt/md file from Book Management, preview the Tool Agent's chapter-splitting regex and chapter list, adjust sample size or the Go regexp when needed, then confirm before Nova creates the new book and writes `chapters/`.
- **Interactive**: play through story branches, explore choices, switch storylines, and maintain scene memory.
- **Lore Library**: maintain durable settings such as characters, worlds, locations, factions, rules, and items for both Writing Mode and Interactive Mode to reuse when needed.
- **Narrative Direction**: configure point of view, pacing, style rules, and interactive generation preferences.
- **Version Management**: manually save versions, view history and diffs, restore previous versions, and enable timed or large-Agent-output automatic versions. Local creative state such as `.nova/lore` and `.nova/sessions` is versioned, and history comes directly from the workspace `.git`.
- **Settings**: adjust models, editor behavior, Agent behavior, interactive-mode parameters, appearance, and language.

## Development

Start both frontend and backend:

```bash
./bootstrap.sh
```

Start frontend/backend separately:

```bash
./bootstrap.sh fe/be
```

Allow LAN devices to access the frontend dev server:

```bash
./bootstrap.sh fe --lan
```

You can also enable "Allow LAN Access" in Settings and restart Nova. Other devices should open the access URL shown in Settings; release builds default to `http://<LAN-IP>:8080`, while frontend dev mode usually uses `http://<LAN-IP>:5173`. Sign in on the page with the remote access username and password; the backend rejects unauthenticated remote requests.

## Donate QR Codes

> Buy the author a coffee and help cover the monthly AI iteration cost.
<p align="center">
  <img src="./img/donate.png" alt="Donate" width="240">
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
