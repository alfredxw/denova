---
name: story-memory-config
description: Use when config_manager creates or updates Story Memory structures or records.
agent: config_manager
---

# Story Memory Config

Use this skill before calling `write_story_memory_structures` or `write_story_memory_records`.

## Workflow

1. Use `list_story_memory_structures` for the target `story_id`; it returns the full structure definitions.
2. Use `list_story_memory_records` before changing records. Use `read_story_memory_records` when exact `values` are needed.
3. Keep structure and record changes separate:
   - Structures define tables, fields, modes, and generation instructions.
   - Records store concrete story facts for one branch.
4. Always pass the active `story_id`. Pass `branch_id` for record writes when the user is working on a specific branch.
5. Do not write story memory by editing story JSONL files directly.

## Structure Rules

`write_story_memory_structures` operations use `structure`:

- `id`: stable snake_case ID. Required for update; optional for create only if the backend can generate one.
- `name`: user visible table name.
- `description`: what belongs in this structure.
- `generation_instruction`: how automatic memory generation should maintain it.
- `mode`: one of `singleton`, `keyed`, `append`.
- `key_field_id`: required for `keyed`; empty for `singleton` and usually empty for `append`.
- `enabled`: boolean pointer. Use `false` for optional structures that should not enter automatic generation yet.
- `order`: integer sort order.
- `fields`: ordered field definitions.

Field definitions use:

- `id`: stable snake_case field ID.
- `name`: user visible field name.
- `description`: what the field stores.
- `generation_instruction`: how agents should fill or update the field.
- `required`: boolean.
- `enabled`: optional boolean pointer.
- `order`: integer sort order.

Do not delete built-in structures unless the user explicitly asks and accepts the impact. Prefer disabling optional custom structures.

## Record Rules

`write_story_memory_records` operations use `record`:

- `structure_id`: must match an existing structure.
- `branch_id`: target branch. If omitted, the current branch is used by the backend.
- `key`: required for `keyed` structures. It should equal the value of the structure's `key_field_id`.
- `values`: map from field ID to string value.
- `manual`: true for user-authored corrections or explicit additions.

Mode behavior:

- `singleton`: one active row per branch. Updating without a record ID upserts the singleton row.
- `keyed`: one active row per key per branch. Provide `key` and the key field value.
- `append`: every create adds a new row unless updating by record ID.

For updates, preserve existing values unless the user asked to change them. For archive/restore/delete, provide the target record ID; `delete` archives rather than hard-deleting.

## Branch Safety

Story Memory is branch-aware. If a visible inherited record from another branch is changed on the current branch, the backend may create a copy-on-write record for the branch. Do not assume one record ID is global across all branches.

## Content Boundaries

- Story Memory is evolving branch-local truth, not stable lore.
- Record concrete facts from the story and user corrections.
- Do not invent unseen events, relationships, abilities, or world rules to fill fields.
- If a required field is unknown, write a bounded placeholder such as `待确认` only when the user accepts that ambiguity.
