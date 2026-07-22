<p align="center">
  <img src="./web/public/favicon.svg" alt="Denova icon" width="76" height="76">
</p>

<p align="center">
  <strong>Denova is an AI creative platform for novel writing and AI generated RPG, with built-in support for AI agents, Skills, subagent workflows, automations, image generation, and version control.</strong>
</p>

<p align="center">
  English | <a href="README.md">中文</a>
</p>

<p align="center">
  <a href="https://discord.gg/QuHu2aPya"><img src="https://img.shields.io/badge/Discord-5865F2?logo=discord&logoColor=white" alt="Join the Denova Discord" /></a>
  <a href="https://github.com/alfredxw/denova/releases"><img alt="Release" src="https://img.shields.io/github/v/release/alfredxw/denova?style=flat-square"></a>
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/github/license/alfredxw/denova?style=flat-square"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat-square&logo=go&logoColor=white">
  <img alt="Node.js" src="https://img.shields.io/badge/Node.js-20%2B-5FA04E?style=flat-square&logo=nodedotjs&logoColor=white">
</p>

<p align="center">
  Current version: <strong>v0.3.1</strong> (2026-07-23) · Beta
</p>

![Denova Writing Mode](./img/ide.png)

<details>
<summary>View more screenshots</summary>

### Game Mode

![Denova Game Mode](./img/interactive.png)

### Branches

![Branches](./img/branch.png)

### Lore Library

![Denova Lore Library](./img/setting.png)

### Presets

![Denova Presets](./img/story-teller.png)

</details>

## Why Denova

Denova is built for long-running creative projects and interactive entertainment. It brings together a writing IDE, interactive stories, structured lore, Agent tool calls, image generation, automation, and local version management in one project workspace so the creative process can iterate, recover, and accumulate durable context.

You can start from an original idea, import an existing novel for fan fiction, adaptation, or continuation, or import AI tavern character cards to quickly set up an interactive text adventure. Model-visible context is built with explicit sources, purposes, and size limits instead of blindly injecting the whole history, logs, or all settings into every turn.

## Core Features

- **Writing Mode**: fiction-focused Markdown editing, multiple tabs, regex find and replace, recoverable workspace-wide replacement, chapter statistics, outlines, chapter-group plans, progress tracking, document comments, Change Review, and existing novel import.
- **Creative Agents**: read selections, files, lore, and trusted review feedback; call tools to generate or edit chapters; and use Skills / SubAgents for different writing tasks, prose styles, and workflows. Changes can be reviewed, commented on, and undone from a cumulative diff.
- **Game Mode**: run interactive text adventures with player input, story branches, storyline switching, Background Director schedules, action suggestions, saved AI reply corrections, searchable Turn history, replayable Actor archive/restore, customizable state layouts, and a full-screen Director Desk for goals, pressure, costs, event card packs, and rule checks.
- **Lore and presets**: maintain durable settings such as characters, worlds, locations, factions, rules, and items; narrative styles handle prose, prompt slots, and scene style rules, while Story Directors can plug together narrative styles, event packages, TRPG Checks, State Systems, and image presets, with each module independently switchable. State Systems also provide reusable trait libraries whose templates define draw rules for each kind of Actor.
- **Image creation**: generate chapter illustrations, interactive images, and book covers through OpenAI-compatible image model profiles, with previews and result management in the UI.
- **Context management**: progressively assemble model context, inspect and copy its sources, build source-linked history checkpoints, improve cache reuse, and keep tool results bounded to reduce noise and token cost.
- **Versions and restore**: save local versions, inspect diffs, restore history, use restart-safe undo/redo for Agent workspace changes, and enable timed saves or automatic saves after large Agent outputs.
- **Automation**: schedule tasks, reviews, auto-continuation, and custom Prompt workflows.
- **Product experience**: Chinese and English UI, light and dark themes, OpenAI-compatible model setup, remote access, PWA phone usage, and Windows / macOS / Linux support.

## Writing Mode and Game Mode

Denova has two parallel workspaces. Writing Mode focuses on the fiction production line: ideas, settings, outlines, chapter plans, prose, and progress. Game Mode focuses on playable interactive narrative: player actions, story branches, Turn history, Actor State, storylines, and choice-driven progression.

Game Mode includes a built-in Story Director that prepares the opening stage from the story setup and lore library, choosing the key characters, factions, clues, risks, and near-term branches that can make the first scene immediately playable. As the story continues, it responds to player choices while keeping character motives, world rules, relationships, and foreshadowing coherent. Important characters, locations, factions, and rules from the lore library are given priority, so established creative material becomes part of the adventure. Each turn aims to deliver meaningful information, relationship movement, pressure, reward, cost, or a fresh hook, then leaves the player with clear ways to continue.

Creators can freely combine narrative styles, event packages, TRPG Checks, State Systems, and image presets, or disable any module that a story does not need. Event packages give the Director optional story material to seed, develop, resolve, or abandon. State Systems adapt to the actual opening and track lasting changes such as attributes, resources, relationships, injuries, and traits; TRPG Checks add fixed-d20 rules and state-based modifiers. Committed Turns are the source of historical facts, Actor State owns current computable facts, Lore owns stable canon, and `director.md` owns future intent. Agents can recover older facts through bounded Turn-history search without maintaining a second writable source of truth. The full-screen Director Desk centralizes plans, events, and execution audit, while the state-aware sidebar keeps actor and world changes visible. Saved AI replies can also be corrected directly without regenerating a turn.

The two modes share durable creative assets such as lore, presets, model and Agent configuration, Skills, version management, and base settings. Writing progress and chapter plans do not automatically enter Game Mode. If an interactive story should reference a passage or current writing milestone, move stable information into lore first or reference it explicitly in the input.

## Community

Denova is iterating quickly. Feedback, bug reports, usage notes, and workflow discussions are welcome.

Join the [Discord community](https://discord.gg/QuHu2aPya) to connect with other creators.

<p align="center">
  <img src="./img/wechat.png" alt="WeChat group" width="240">
</p>

## Quick Start

### Download a Release

Download the archive for your platform from [GitHub Releases](https://github.com/alfredxw/denova/releases), extract it, and run:

```bash
./denova
```

Windows users should run `denova.exe`. On macOS, if the system blocks the app for security reasons, run:

```bash
xattr -dr com.apple.quarantine denova
```

### Run from Source

Requires Go 1.26.5+, Node.js 20+, pnpm and ripgrep.

```bash
git clone https://github.com/alfredxw/denova.git
cd denova
corepack enable
./scripts/bootstrap.sh
```

Default addresses:

- Frontend: `http://localhost:5173`
- Backend: `http://localhost:8080`

## Models and Configuration

Denova uses an OpenAI-compatible API. The recommended path is to configure language models, image models, Agent parameters, the default Writing Skill, editor options, Game Mode behavior, version management, language, theme, and fonts from Settings.

For scripted startup or deployment, you can also override model configuration with environment variables:

```bash
export OPENAI_API_KEY="your-api-key"
export OPENAI_BASE_URL="https://api.deepseek.com"
export OPENAI_MODEL="deepseek-v4-pro"
export OPENAI_IMAGE_API_KEY="your-openai-image-key"
export OPENAI_IMAGE_BASE_URL="https://api.openai.com/v1"
export OPENAI_IMAGE_MODEL="gpt-image-1"
```

Optional Denova startup environment variables:

```bash
export DENOVA_WORKSPACE="/path/to/your-workspace"
export DENOVA_DIR="./.denova"
export DENOVA_SKILLS_DIR="./skills"
export DENOVA_WEB_DIR="./web"
export DENOVA_BACKEND_PORT="8080"
export DENOVA_FRONTEND_PORT="5173"
```

Configuration precedence:

```text
Built-in defaults < global config.toml < user-level config < environment variables
```

Common, Writing Mode, and Game Mode preferences from Settings are now stored uniformly at the user level. A workspace `.denova/config.toml` only carries workspace customizations explicitly exposed by the Agents page; other legacy fields remain on disk but no longer override user settings. Legacy environment variables are still read for compatibility; new configuration should use `.denova` / `DENOVA_*`.

## Remote Access and Phone Usage

Denova can run locally, on your LAN, or on a self-hosted server. Release archives already include frontend assets; when deploying from source, build the frontend first:

```bash
pnpm --dir web build
```

Enable **Settings → Remote Access → Allow LAN access**, then set a username and password. Other devices can open the access URL shown in Settings. After signing in from a phone browser, you can add Denova to the home screen and use it like a standalone app.

For public or domain access, use a reverse proxy such as Caddy / Nginx to provide HTTPS. This avoids sending credentials in cleartext and keeps browser features such as clipboard access and PWA behavior working reliably.

Caddy example:

```text
denova.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

## Development

Start both frontend and backend:

```bash
./scripts/bootstrap.sh
```

Start frontend or backend separately:

```bash
./scripts/bootstrap.sh fe
./scripts/bootstrap.sh be
```

Stop the Denova backend running from this repository and restart it in the foreground:

```bash
./scripts/restart-backend.sh
```

Allow LAN devices to access the frontend dev server:

```bash
./scripts/bootstrap.sh fe --lan
```

## Donate QR Code

> Buy the author a coffee and help cover the monthly AI iteration cost.

<p align="center">
  <img src="./img/donate.png" alt="Donate" width="240">
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
