---
name: teller-config
description: Use when config_manager creates or updates Denova interactive teller/director configurations.
agent: config_manager
---

# Teller Config

Use this skill before calling `write_tellers`.

## Workflow

1. Call `list_tellers` first. For updates, call `read_tellers` for the exact teller IDs.
2. Use `write_tellers` for create/update/delete. Do not edit teller JSON files directly.
3. Built-in tellers can be read and copied as examples. Deleting built-in tellers is rejected.
4. For update, preserve slots and policy fields the user did not ask to change.
5. For delete, require an explicit user request.

## Teller Shape

Important fields:

- `id`: stable ID. Required for update/delete; create may generate one if omitted.
- `name`: user visible name.
- `description`: short explanation of the teller's narrative role.
- `random_event_rate`: number between 0 and 1. Use small values unless the user wants surprise events.
- `tags`: short searchable labels.
- `context_policy`: controls which context groups the teller expects.
- `slots`: prompt slots used by the interactive story agent.

Do not change `version`, `path`, `custom`, `invalid`, `error`, `created_at`, or `updated_at` unless preserving an existing complete object from `read_tellers`.

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

## Style Rules

`style_rules` maps scenes to inline style reference content:

- `scene`: scenario label.
- `style_contents`: list of text snippets used as prose style references. Each item is stored as text, not a file path, and should stay within 8000 characters.

Only add style rules when the user asks for scene-specific style behavior or when an existing teller already uses that pattern.
