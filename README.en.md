<p align="center">
  English | <a href="README.md">中文</a>
</p>

# Nova

Nova is an AI writing workspace for long-form fiction creators. It provides an IDE-like creative environment that covers inspiration, worldbuilding, outlining, chapter writing, interactive story rehearsal, lore management, and local versioning.

Current version: v0.1.6 (2026-06-05)

## Screenshots

![Nova Novel IDE](./img/ide.png)

<details>
<summary>More screenshots</summary>

### Interactive Story Workspace

![Nova Interactive Story Workspace](./img/interactive.png)

### Lore Library

![Nova Lore Library](./img/setting.png)

### Story Style Configuration

![Nova Story Style](./img/story-teller.png)

</details>

## Features

- Novel IDE: file tree, Markdown editor, multiple tabs, chapter statistics, and an AI chat panel.
- Writing Agent: streaming output, tool calls, selected-text references, `@` file references, and `#` style references.
- Chapter workflow: brainstorming, settings, outlines, chapter-group plans, drafts, and final chapter text.
- Interactive stories: branching plots, next-action candidates, scene memory, and storyline switching.
- Lore library: structured long-term settings for characters, worlds, locations, factions, rules, items, and more. Per-chapter character state such as current location, injuries, mental state, and goals is tracked in `setting/character-states.md`.
- Character card import: supports SillyTavern v2 PNG / JSON.
- Local version management: each book uses Nova-native snapshots to create versions, view history, compare diffs, and restore previous states without Git.
- Layered configuration: supports global, user-level, and workspace-level configuration.

## Install

### GitHub Release

Download the archive for your platform from [Releases](https://github.com/alfredxw/nova/releases), extract it, and run:

```bash
./nova
```

Specify a book workspace:

```bash
./nova --workspace /path/to/your-novel
```

Windows users should run `nova.exe`. On macOS, if the system blocks the app for security reasons, run:

```bash
xattr -dr com.apple.quarantine nova
```

### From Source

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

## Configuration

Nova uses an OpenAI-compatible API. You can configure it with environment variables:

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

You can also configure models, Agent parameters, editor options, and interactive-mode settings in `config.toml`. Configuration precedence:

```text
Built-in defaults < global config.toml < user-level config < workspace-level config < environment variables
```

## Usage

After startup, if no book is specified or restored, the Web UI opens Book Management. One workspace maps to one book. Recommended structure:

```text
my-novel/
├── CREATOR.md
├── 脑暴.md
├── chapters/
├── setting/
│   ├── progress.md
│   ├── character-states.md
│   └── chapter-groups/
├── drafts/
└── .nova/
```

Common entry points:

- Writing: edit chapters, browse the file tree, and collaborate with the Writing Agent.
- Interactive: rehearse plots, explore branches, and maintain narrative direction.
- Lore library: maintain characters, worlds, locations, factions, rules, and items. Current character state is tracked by `setting/character-states.md`.
- Version management: manually save versions, view history and diffs, restore previous versions, and enable timed or large-Agent-output auto snapshots.
- Settings: adjust models, editor behavior, Agent behavior, and interactive-mode parameters.

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

## License

[Apache-2.0](./LICENSE)
