---
name: story-memory-config
description: Use when config_manager creates or updates Story Memory records or legacy story-local structures.
agent: config_manager
---

# Story Memory Config

Use this skill before calling `write_story_memory_records` or the legacy `write_story_memory_structures`.

## Workflow

1. Use `list_story_memory_structures` for the target `story_id`; it returns the currently effective structure definitions and their `memory_structure_id` source.
2. Use `list_story_memory_records` before changing records. Use `read_story_memory_records` when exact `values` are needed.
3. Keep structure and record changes separate:
   - Structure definitions now belong to Story Memory Structure presets under the Story Director module system.
   - Records store concrete narrative continuity facts for one branch.
4. For new schema changes, switch to `story-director-config`: use `write_story_memory_structure_presets`, then update `story_director.module_refs.memory_structure_id` with `write_story_directors`.
5. Always pass the active `story_id`. Pass `branch_id` for record writes when the user is working on a specific branch.
6. Do not write story memory by editing story JSONL files directly.

## Structure Rules

`write_story_memory_structures` is a compatibility tool for old story-local structures only. Do not use it for normal configuration. New structure schema changes should use `write_story_memory_structure_presets` from `story-director-config`.

Legacy `write_story_memory_structures` operations use `structure`:

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
Do not add current time, location, current event, resources, relationship scores, ongoing conditions, rule flags, or other state-management fields to Story Memory structures. Add those to the State System through `story-director-config` and `write_actor_states`.

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
- Record concrete narrative facts from the story and user corrections.
- Use State System, not Story Memory, for current scene state, computable resources, relationship scores, ongoing conditions, and rule flags.
- Do not invent unseen events, relationships, abilities, or world rules to fill fields.
- If a required field is unknown, write a bounded placeholder such as `待确认` only when the user accepts that ambiguity.
