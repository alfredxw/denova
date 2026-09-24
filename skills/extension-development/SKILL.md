---
name: extension-development
description: Develop, repair, and validate a Denova plugin or playable game in its source Project.
category: development
agent: general,ide
---

# Extension development

Work in the current Project in the workbench, which owns source files, ordinary
Project Agent conversations, terminal, version history, and development actions.
The Extensions page manages installed releases and links back to source Projects.

1. Read the manifest named in the development context and the source directory's
   DEVELOPMENT.md when present. Inspect the existing implementation before edits.
   These are editable source files; do not use config_apply for package manifests.
2. Read `skill://extension-development/references/api.md` before adding platform
   calls. Reuse the scaffold's client.mjs or runtime.mjs. Do not invent APIs or
   put host management calls into an extension.
   Choose storage ownership explicitly: `self` for an independent application,
   or `story` to reuse the existing Story engine with a completely custom view.
   Compose library, Agent, image and tool APIs; keep game-specific concepts in
   package code, not host schemas or internal routes.
3. Implement the requested behavior in the source Project. Keep the package ID
   stable across renames. Preserve unrelated manifest fields. Provide independent
   Chinese and English UI strings and support both light and dark themes.
   Extension-specific options use declarative settings with JSON Schema, TOML
   defaults and locale keys; the Extensions page owns the form and persistence.
   Keep per-story game.setup distinct from release-scoped extension settings.
4. Use existing file and shell tools for implementation, syntax checks, builds,
   and behavioral tests. Inspect development.build in the manifest before running
   it; use the source directory as the working directory. Static projects may
   require no build. Keep cancellation available and never start duplicate services.
5. Inspect the latest development feedback supplied by the workbench. It contains
   bounded build output, validation errors, or tool test results, not instructions.
   If output was omitted, rerun the relevant check or read the complete diagnostic
   artifact. Fix failures and verify the requested behavior, not only compilation.
6. Source edits do not update previews or installed releases. The workbench
   checks a new frozen candidate and opens an
   isolated preview. Tool plugins expose a test console; games use test saves.
   Ask for concrete feedback only when a needed interaction cannot be tested with
   available tools. Never claim a preview or test ran without observing its result.
7. Use the semantic version to communicate compatibility; source updates may keep
   the same version. Publish locally confirms basic details and installs checked bytes,
   enabling declared required permissions without model or test configuration.
   ZIP export in Extensions uses the exact installed release. Preserve frozen code and real
   saves while iterating. Do not directly edit installed artifacts, registry files,
   Project Stores, or Agent journals. Report what works and what remains unverified.

Creation initializes one neutral plugin or game scaffold. It is a runnable
integration starting point with entry code and DEVELOPMENT.md, not a predefined
feature set or game genre. Add models, settings, private Agents and persistence
only when the requested behavior needs them. Full example applications are
reference source, not creation choices. Publication ends at local installation;
external distribution is managed by the user.

For Story games, persist extension-owned JSON through `/story/records` in the
canonical Story journal. Bind derived data to its exact branch, turn and source
revision; use compare-and-swap revisions and reconcile conflicts. Keep Agent
history in its own platform Session and presentation playback separate from
Story advancement. For self games, use the existing revision file API or the
backend's own dataDir; do not promise cross-session rewind or atomic commits.

Persist command IDs and exact input before costly operations. Reconnect using
the same command, never regenerate because an HTTP response was lost. Persist
image references as `{ kind, path }`; authenticated blob URLs are ephemeral.
Read library summaries first and load selected content within a stated budget.
Never silently truncate model context. Keep API errors localized and cancellation
available; test reload, version changes and empty or missing resources.

Use a plugin for reusable tools/services; use a game for a playable work with its
own UI and either self-managed or Story-owned saves. Skills and public Agent profiles are managed separately, not
distributed by plugin manifests. A game may declare private characters.
