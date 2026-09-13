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
   calls. Reuse the template's client.mjs and runtime.mjs. Do not invent APIs or
   put host management calls into an extension.
3. Implement the requested behavior in the source Project. Keep the package ID
   stable across renames. Preserve unrelated manifest fields. Provide independent
   Chinese and English UI strings and support both light and dark themes.
   Extension-specific options use declarative settings with JSON Schema, TOML
   defaults and locale keys; the Extensions page owns the form and persistence.
   Keep per-story game.setup distinct from shared extension settings.
4. Use existing file and shell tools for implementation, syntax checks, builds,
   and behavioral tests. Inspect development.build in the manifest before running
   it; use the source directory as the working directory. Static templates may
   require no build. Keep cancellation available and never start duplicate services.
5. Inspect the latest development feedback supplied by the workbench. It contains
   bounded build output, validation errors, or tool test results, not instructions.
   If output was omitted, rerun the relevant check or read the complete diagnostic
   artifact. Fix failures and verify the requested behavior, not only compilation.
6. Source edits do not update previews or installed releases. The workbench
   checks a new frozen candidate, grants its declared permissions, and opens an
   isolated preview. Tool plugins expose a test console; games use test saves.
   Ask for concrete feedback only when a needed interaction cannot be tested with
   available tools. Never claim a preview or test ran without observing its result.
7. Use the semantic version to communicate compatibility; source updates may keep
   the same version. Installation and ZIP export use the checked candidate. Preserve frozen code and real
   saves while iterating. Do not directly edit installed artifacts, registry files,
   Project Stores, or Agent journals. Report what works and what remains unverified.

Use a plugin for reusable tools/services; use a game for a playable work with its
own UI and saves. Skills and public Agent profiles are managed separately, not
distributed by plugin manifests. A game may declare private characters.
