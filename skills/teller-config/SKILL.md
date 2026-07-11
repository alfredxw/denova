---
name: teller-config
description: Use when config_manager creates or updates Denova narrative style configurations.
agent: config_manager
---

# Narrative Style Config

Use this skill before calling `write_tellers`.

## Workflow

1. Call `list_tellers` first. For updates, call `read_tellers` for the exact teller IDs.
2. Call `list_style_references` before editing `style_refs` or `style_rules`. If a needed reference does not exist, use `write_style_references` to create a `.denova/styles/*.md` Markdown file first.
3. Use `write_tellers` for create/update/delete. Do not edit teller JSON files directly.
4. Updating a built-in narrative-style ID creates a user-space override of that same ID. Deleting a built-in ID is only for restoring the code-defined default and requires an explicit restore request.
5. For update, preserve slots and policy fields the user did not ask to change.
6. For delete, require an explicit user request.
7. Do not create or update `orchestration` here. Events, State Systems, TRPG checks, and opening trait rolls belong in `story-director-config` and the corresponding story-director tools.
8. Narrative styles are shared modules for Writing Mode and Game Mode. Do not add a per-style mode/scope field.

## Teller Shape

Important fields:

- `id`: stable ID. Required for update/delete; create may generate one if omitted.
- `name`: user visible name.
- `description`: short explanation of the narrative style.
- `context_policy`: controls which context groups the teller expects.
- `slots`: prompt slots used by writing and interactive story prompt assembly.

Do not change `version`, `path`, `custom`, `builtin_overridden`, `invalid`, `error`, `created_at`, or `updated_at` unless preserving an existing complete object from `read_tellers`.

## Context Policy

`context_policy` contains:

- `creator`: how to use CREATOR.md and creator-level rules.
- `lore`: how to use lore/context library.
- `runtime_state`: how to use current story state and turn context.

Keep these as short policy strings. They guide prompt assembly but do not replace runtime safety rules.

## Slots

Each slot contains:

- `id`: stable slot ID.
- `name`: user visible slot name.
- `target`: where the slot applies, such as `system`, `turn_context`, or another existing target read from a teller.
- `enabled`: boolean.
- `content`: prompt text for that target.

When modifying slots:

- Preserve slot IDs so existing UI selection and semantics remain stable.
- Keep slot content focused on narrative behavior, not backend tool permissions.
- Do not put story facts, chapter prose, or temporary scene state into teller slots.
- If a new slot target is needed, mirror the target style already present in existing tellers.

## Style References

Top-level `style_refs` lists shared style reference files that apply to every scene by default.

`style_rules` maps specific scenes to shared style reference files:

- `scene`: scenario label.
- `style_refs`: list of paths returned by `list_style_references`, usually `.denova/styles/<name>.md`.
- `style_contents`: legacy inline snippets. Preserve existing values unless the user asks to migrate them, but do not add new inline content.

Use top-level `style_refs` when the user wants one reference style to affect all scenes. Only add `style_rules` when the user asks for scene-specific style behavior or when an existing teller already uses that pattern.

## Shared Style References

Style references are shared by all narrative styles and live under `.denova/styles/`.

When creating a reference from a user source file:

- Default to extracting a Markdown style reference instead of saving the raw original.
- The Markdown should be dominated by distilled typical reference paragraphs, with short supporting style guidance.
- Do not include real-world author names, work names, source notes, or long raw quotations.
- Keep it reusable across Writing Mode and Game Mode.
- If the user explicitly chooses not to extract, save the provided source content as a Markdown reference with a clear name and description.

When writing the teller back, use `write_tellers` with the complete updated teller object and a concise change message.
