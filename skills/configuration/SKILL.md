---
name: configuration
description: Use when a Project Agent reads or changes managed presets, game modules, automations, Skills, or Agent profiles through config_read and config_apply.
category: configuration
agent: general,ide
---

# Configuration

Manage every supported configuration resource through the stable `config_read` and `config_apply` tools. Never edit Denova configuration files or internal JSON storage directly.

## Required workflow

1. For an unfamiliar resource, call `config_read` with `operation=describe`. Omit `resource` to discover all registered kinds, or provide one kind for its contract.
2. Call `config_read(operation=list)` to resolve the exact ID and scope. Catalogs use `items` plus `next_cursor`; continue with the identical request and returned cursor until `truncated=false` when the full catalog matters. Before an update or delete, call `config_read(operation=get)` for the exact ID and retain its latest revision.
3. Read exactly the relevant reference below with `read({"path":"skill://configuration/references/<file>.md"})`. Do not infer a value shape from another resource.
4. Follow that resource's mutation semantics. Call `config_apply` for exactly one create, update, or delete and use only names returned by `describe`.
5. For update and delete, copy the latest revision from `config_read`; never guess, copy an example placeholder, or reuse a stale revision. Preserve every field or layer section the user did not ask to change.

`get` accepts any number of IDs. A mixed batch succeeds with existing `items`, explicit `missing_ids`, and per-ID `failures`; an entirely unsuccessful completed batch fails. Large exact reads also return `next_cursor`. Never treat `truncated=true` as a complete read.
6. `config_apply` returns only a compact persistence receipt (`resource`, `operation`, `id`, `revision`). Read the changed item again and compare the requested fields plus preservation-sensitive fields. Report success only when the effective result matches.

Deletion must be explicitly requested by the user. If a stale-revision conflict occurs, read the current item again, reconcile the requested change with that value, and retry once with the new revision. Do not silently overwrite concurrent changes.

## Mutation semantics

| Resources | Update behavior |
| --- | --- |
| `narrative_style`, `story_director`, `event_package`, `rule_system`, `state_system`, `image_preset` | Complete editable-resource replacement. Start from `get`, change only requested fields, and submit the complete editable value. Omitted arrays and objects are cleared or defaulted. |
| `automation` | Sparse patch. Omitted fields stay unchanged; a present `triggers` array replaces the whole trigger list. `target` is create-only and immutable. |
| `agent_profile` | Sectional layered update. Only supplied `model`, `tools`, `prompt`, `skills`, or `context` sections change, but each supplied map/object replaces that section in the selected layer. |
| `skill` | Complete replacement of one root `SKILL.md` or one supporting reference file. The root revision covers the whole Skill directory. |
| `style_reference` | Create writes one document; update replaces its complete Markdown content. |

Fields returned for inspection such as `path`, `custom`, `builtin_overridden`, `invalid`, `error`, timestamps, resolved snapshots, run history, and secrets are host-owned unless the resource reference explicitly says otherwise. Never add an unknown field: configuration values reject unknown keys.

Examples use `REVISION_FROM_GET` as a visible placeholder. Always replace it with the exact current revision returned for the same resource, ID, and scope.

## Reference routing

- `style_reference` → `skill://configuration/references/style-reference.md`
- `narrative_style` → `skill://configuration/references/narrative-style.md`
- `story_director` → `skill://configuration/references/story-director.md`
- `event_package` → `skill://configuration/references/event-package.md`
- `rule_system` → `skill://configuration/references/rule-system.md`
- `state_system` → `skill://configuration/references/state-system.md`
- `image_preset` → `skill://configuration/references/image-preset.md`
- `automation` → `skill://configuration/references/automation.md`
- `skill` → `skill://configuration/references/skill.md`
- `agent_profile` → `skill://configuration/references/agent-profile.md`

These are references, not sub-Skills. Do not pass a `sub_skill` argument or invoke another Skill name.
