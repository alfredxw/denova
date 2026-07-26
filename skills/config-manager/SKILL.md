---
name: config-manager
description: Use when the Config Manager Agent reads or changes Denova presets, game modules, automations, Skills, or Agent profiles through config_read and config_apply.
agent: config_manager
---

# Config Manager

Manage every supported configuration resource through the stable `config_read` and `config_apply` tools. Never edit Denova configuration files or internal JSON storage directly.

## Required workflow

1. For an unfamiliar resource, call `config_read` with `operation=describe`. Omit `resource` to discover all registered kinds, or provide one kind for its contract.
2. Call `config_read(operation=list)` to resolve the exact ID and scope. Before an update or delete, call `config_read(operation=get)` for the exact ID and retain its latest revision.
3. Read only the relevant reference below with `read({"path":"skill://config-manager/references/<file>.md"})`.
4. Call `config_apply` for exactly one create, update, or delete. Use only resource names returned by `describe`.
5. For update and delete, copy the latest revision from `config_read`; never guess or reuse a stale revision. Preserve fields the user did not ask to change.
6. Read the changed item again and verify the effective result before reporting success.

Deletion must be explicitly requested by the user. If a stale-revision conflict occurs, read the current item again, reconcile the requested change with that value, and retry once with the new revision. Do not silently overwrite concurrent changes.

## Reference routing

- `style_reference` → `skill://config-manager/references/style-reference.md`
- `narrative_style` → `skill://config-manager/references/narrative-style.md`
- `story_director` → `skill://config-manager/references/story-director.md`
- `event_package` → `skill://config-manager/references/event-package.md`
- `rule_system` → `skill://config-manager/references/rule-system.md`
- `state_system` → `skill://config-manager/references/state-system.md`
- `image_preset` → `skill://config-manager/references/image-preset.md`
- `automation` → `skill://config-manager/references/automation.md`
- `skill` → `skill://config-manager/references/skill.md`
- `agent_profile` → `skill://config-manager/references/agent-profile.md`

These are references, not sub-Skills. Do not pass a `sub_skill` argument or invoke another Skill name.
