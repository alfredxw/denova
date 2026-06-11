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
  Current version: <strong>v0.1.8</strong> (2026-06-11) · Beta
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

Nova is closer to an AI creative workspace for fiction than a one-off writing assistant. Many AI novel tools focus on "enter a prompt, generate a passage"; Nova focuses on continuity for long-form work: book files, lore, chapter state, interactive rehearsal, Agent tool calls, and local versions all stay in the same workspace so the author can keep iterating.

If you want AI to do more than complete the next paragraph, and instead collaborate around the same book, read bounded context, accumulate lore, maintain chapter progress, and leave restorable versions at important moments, Nova is the better fit.

| What you need | How Nova handles it |
| --- | --- |
| Long-form book management | An IDE-like workspace with file tree, Markdown editor, multiple tabs, chapter statistics, global search, and AI side panel |
| Deeper AI collaboration | Agents can read selections and files, reference lore, call tools, track todos, and write drafts |
| Continuous creation workflow | Ideas, settings, outlines, chapter-group plans, drafts, final prose, and state sync share clear entry points |
| Plot validation | IDE mode is for writing; interactive mode rehearses branches, character actions, scene memory, and storylines |
| Durable lore | Characters, worlds, locations, factions, rules, and items live in structured lore, while current character state is tracked separately |
| Personalized workflow | Built-in and custom Skills, narrative direction, layered settings, and per-book model configuration |
| Version confidence | go-git maintains a local `.git` in the book folder with history, diffs, restore, timed saves, and Agent-output auto saves |

The recommended path is to start with ideas, settle top-level settings and creative rules, then build the outline and chapter-group plan. For each chapter, use the Agent to draft or write prose, finalize it, and sync progress plus character state. When a plot needs testing, switch to interactive mode to rehearse branches, then fold stable decisions back into lore and keep saving local versions.

```text
Ideas
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

You can also configure models, Agent parameters, editor options, interactive-mode behavior, version management, and interface language in `config.toml`. `NOVA_SKILLS_DIR` / `skills_dir` is the built-in read-only Skills root; custom Skills can be written from the UI to `<nova_dir>/skills` or `<workspace>/.nova/skills`. Configuration precedence:

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
scripts/build-github-release.sh v0.1.8
```

After pushing the tag, GitHub Actions will create or update the Release automatically:

```bash
git tag v0.1.8
git push origin v0.1.8
```

## License

[Apache-2.0](./LICENSE)
