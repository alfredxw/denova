<p align="center">
  <img src="./web/public/favicon.svg" alt="Nova icon" width="76" height="76">
</p>

<p align="center">
  <strong>Nova is an AI-native creative workspace for storytellers: manage a book like an IDE, collaborate with Agents on planning, drafting, revision, and interactive rehearsal, and keep lore, bounded context, versions, and automation in one durable workspace.</strong>
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
  Current version: <strong>v0.1.10</strong> (2026-06-12) · Beta
</p>

![Nova Novel IDE](./img/ide.png)

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

Nova is not a one-off "prompt in, passage out" generator. It is a full workspace for long-running fiction projects. Book files, Markdown editing, multiple tabs, global search, chapter statistics, structured lore, interactive story rehearsal, Agent tool calls, and local version management live in the same workspace, so ideas, settings, outlines, chapter drafts, revisions, branch testing, and final prose stay on one continuous production line.

Beyond writing original stories, Nova can import existing novels as a starting point for fan fiction, adaptation, or continuation, and it can import AI tavern character cards to quickly create interactive presets. Model-visible context is built progressively with explicit sources and limits, keeping lore, file excerpts, tool results, and display history separate instead of blindly injecting the entire project into every turn.

- **Book file management**: file tree, Markdown editor, multiple tabs, global search, chapter statistics, and an IDE-like writing workspace.
- **Creative Agents**: read selections, read files, reference lore, call tools, and write into drafts or chapters.
- **Structured lore**: characters, worlds, locations, factions, rules, items, and other durable settings become searchable long-term lore.
- **Progressive context**: model context is organized by source, purpose, and hard size limits instead of unbounded history, logs, or full settings.
- **Interactive story mode**: rehearse branches, character actions, scene memory, and storyline changes with the same underlying lore.
- **Version management**: go-git powered saves, diffs, restore, timed saves, and automatic saves for large Agent outputs.
- **Skills and Agents**: configure creative skills, prompts, tool permissions, and custom prose styles for different Agents.
- **Automation**: schedule tasks, reviews, auto-continuation, and custom Prompt workflows.
- **Imports and presets**: import AI tavern character cards or existing novels for fan fiction, adaptation, or continuation.
- **Product experience**: Chinese and English UI, light and dark themes, OpenAI-compatible model configuration, and Windows, macOS, and Linux support.

The recommended path is to start from an idea or an import: settle top-level settings and creative rules, build the outline and chapter-group plan, then use Agents to draft or write chapter prose. When a plot needs testing, switch to interactive mode to rehearse branches, then fold stable decisions back into lore and keep saving local versions.

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

You can also configure models, Agent parameters, editor options, interactive-mode behavior, version management, backend/frontend ports, and interface appearance (language, theme, fonts) in `config.toml`. Set the backend port with `backend_port = 8080` and the frontend dev port with `frontend_port = 5173`, or edit them from the user settings page and restart Nova; for one-off launches, `--port` / `--frontend-port` take precedence over `NOVA_BACKEND_PORT` / `NOVA_FRONTEND_PORT` and config files. `theme` supports `dark` (default), `light`, and `system`, and can be saved at the user or workspace level. `NOVA_SKILLS_DIR` / `skills_dir` is the built-in read-only Skills root; custom Skills can be written from the UI to `<nova_dir>/skills` or `<workspace>/.nova/skills`. Configuration precedence:

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

- **Writing**: edit chapters, browse the file tree, search project files, and collaborate with the Writing Agent.
- **Import Existing Novel**: upload a txt/md file from Book Management, preview the Tool Agent's chapter-splitting regex and chapter list, adjust sample size or the Go regexp when needed, then confirm before Nova creates the new book and writes `chapters/`.
- **Interactive**: rehearse plots, explore branches, switch storylines, and maintain scene memory.
- **Lore Library**: maintain characters, worlds, locations, factions, rules, and items. Current character location, injuries, mental state, goals, and similar state live in `setting/character-states.md`.
- **Narrative Direction**: configure point of view, pacing, style rules, and interactive generation preferences.
- **Version Management**: manually save versions, view history and diffs, restore previous versions, and enable timed or large-Agent-output automatic versions. Local creative state such as `.nova/lore` and `.nova/sessions` is versioned, and history comes directly from the workspace `.git`.
- **Settings**: adjust models, editor behavior, Agent behavior, interactive-mode parameters, appearance, and language.

## Development

Start both frontend and backend:

```bash
./bootstrap.sh
```

Start frontend only:

```bash
./bootstrap.sh fe
```

Start backend only:

```bash
./bootstrap.sh be
```

Production build:

```bash
./build.sh
```

Run the build output:

```bash
cd output
./nova --workspace /path/to/your-novel
```

## Tech Stack

- Backend: Go, Hertz, Eino, SSE
- Frontend: React, TypeScript, Vite, Tailwind CSS, TipTap
- State: TanStack Query, Zustand
- Packaging: GitHub Actions, cross-platform Go binaries

## Project Structure

```text
.
├── cmd/nova/        # Service entry point
├── config/          # Configuration loading
├── internal/        # Backend business modules
├── scripts/         # Build and release scripts
├── skills/          # Creative skill prompts
└── web/             # React Web UI
```

## Release

Build a local GitHub Release package:

```bash
scripts/build-github-release.sh v0.1.10
```

After pushing the tag, GitHub Actions will create or update the Release automatically:

```bash
git tag v0.1.10
git push origin v0.1.10
```

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
