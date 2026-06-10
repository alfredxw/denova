<h1 align="center">Nova</h1>

<p align="center">
  <strong>An AI creation workspace for long-form fiction and interactive storytelling</strong>
</p>

<p align="center">
  Nova brings ideation, worldbuilding, outlining, chapter writing, interactive rehearsal, lore management, and local versioning into one IDE-like creative workspace.
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
  Current version: <strong>v0.1.6</strong> (2026-06-05) · Beta
</p>

![Nova Novel IDE](./img/ide.png)

<details>
<summary>View more screenshots</summary>

### Interactive Story Workspace

![Nova Interactive Story Workspace](./img/interactive.png)

### Lore Library

![Nova Lore Library](./img/setting.png)

### Narrative Direction Configuration

![Nova Narrative Direction](./img/story-teller.png)

</details>

## Why Nova

Nova is more than a chat box and more than a text completion editor. It turns fiction creation into a sustainable workflow and lets AI Agents collaborate inside bounded, source-aware context: they can read selected text, inspect files, reference lore, call tools, write drafts, update state, and create local versions at important moments.

- **Manage a book like a project**: file tree, Markdown editor, multiple tabs, chapter statistics, global search, and an AI side panel in one stable writing desk.
- **Bring Agents into the full workflow**: brainstorming, top-level settings, outlines, chapter-group plans, drafts, final prose, and state sync all have clear entry points.
- **Write and rehearse in the same workspace**: IDE mode produces content, while interactive mode tests plot branches and character actions.
- **Turn lore into structured assets**: characters, worlds, locations, factions, rules, and items live in the lore library, while per-chapter character state is tracked separately.
- **Protect the creative process by default**: Nova uses go-git to maintain a local `.git` repository in the book workspace, supporting manual saves, history, diffs, restore, and automatic saves. Prose, settings, and creative state under `.nova` such as lore and sessions are versioned. System Git, manual repository setup, and an extra `.nova/versions` metadata directory are not required.

## Core Capabilities

| Area | Capabilities |
| --- | --- |
| Novel IDE | File tree, Markdown editor, multiple tabs, chapter statistics, global search, AI chat panel |
| Writing Agent | Streaming output, tool calls, selected-text references, `@` file references, `#` style references, todo tracking |
| Chapter Workflow | Brainstorming, settings, outlines, chapter-group plans, drafts, final chapter text, state sync |
| Interactive Stories | Plot branches, next-action candidates, scene memory, storyline switching, route map |
| Lore Library | Structured long-term settings for characters, worlds, locations, factions, rules, items, and more |
| Narrative Direction | Per-book or per-scene narrative rules, style constraints, pacing preferences, and interactive generation strategy |
| Existing Novel Import | Upload txt / md files, let the Tool Agent detect the chapter-title regex, preview the split, adjust sample size or regex, then confirm creation as a new book |
| Character Card Import | SillyTavern v2 PNG / JSON import into the current book or a new book |
| Version Management | go-git managed workspace `.git`, history including `.nova` creative state, diff comparison, restore, timed saves, and large-Agent-output auto saves |
| Layered Configuration | Global, user-level, and workspace-level settings for different models and different books |

## Recommended Workflow

```text
Ideation
  ↓
Top-level settings and creative rules
  ↓
Outline and chapter-group plan
  ↓
Single-chapter draft / final prose
  ↓
Finalize and sync progress plus character state
  ↓
Rehearse plot branches in interactive mode
  ↓
Continuously refine lore and local versions
```

Nova separates display history, model context, lore content, tool results, and workspace state as much as possible, so Agents receive only the source-backed and bounded context needed for the current task.

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

You can also configure models, Agent parameters, editor options, interactive-mode behavior, version management, and interface language in `config.toml`. Configuration precedence:

```text
Built-in defaults < global config.toml < user-level config < workspace-level config < environment variables
```

## Book Workspace

After startup, if no book is specified or restored, the Web UI opens Book Management. One workspace maps to one book. Recommended structure:

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
scripts/build-github-release.sh v0.1.6
```

After pushing the tag, GitHub Actions will create or update the Release automatically:

```bash
git tag v0.1.6
git push origin v0.1.6
```

## License

[Apache-2.0](./LICENSE)
