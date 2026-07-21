---
name: story-director-config
description: Use when config_manager creates or updates Denova Story Director resources.
agent: config_manager
---

# Story Director Config

Use this skill before calling `write_story_directors`, `write_event_packages`, or `write_actor_states`.

## Workflow

1. Call `list_story_directors` first. For updates, call `read_story_directors` for the exact director IDs.
2. Call `list_event_packages` before changing a director's event package references. For event-card content updates, call `read_event_packages` for exact package IDs.
3. Call `list_actor_states` before changing a director's `actor_state_id`; call `read_actor_states` before editing state templates, fields, trait pools, or trait rules.
4. Use `write_story_directors` for director create/update/delete, `write_event_packages` for event package create/update/delete, and `write_actor_states` for State System schema changes. Do not edit JSON files directly.
5. Built-in story directors, event packages, and State Systems can be read and copied as examples. Deleting built-ins restores their built-in version.
6. For update, preserve sections the user did not ask to change.
7. For delete, require an explicit user request.
8. When grounding event cards in the current work, call `list_lore_items` first, then `read_lore_items` for only the small relevant set. Do not claim concrete world, faction, character, or relationship facts unless they came from read lore, read director/package data, or explicit user input.
9. Story Directors, event packages, TRPG Checks, and State Systems are Game Mode-only module types. Reusable Actor traits belong inside State Systems; do not add per-resource mode/scope fields.

## Shape

Story Directors are Game Mode modules independent from shared narrative styles and shared image presets. They combine reusable modules through `module_refs` and keep expanded resolved sections for inspection.

- `module_refs`: referenced module IDs plus switches. Use `narrative_style_id`, `event_package_ids`, `rule_system_id`, `actor_state_id`, and `image_preset_id`; set `narrative_style_disabled`, `event_packages_disabled`, `rule_system_disabled`, `actor_state_disabled`, or `image_preset_disabled` to `true` to turn a module off. When disabling, preserve IDs so the user can re-enable without reselecting. Actor traits live in the referenced State System.
- `strategy`: `enabled`, `mainline_strength`, `failure_policy`, `pacing_curve`, `random_event_rate`. Prefer the standard enum IDs used by the UI: `mainline_strength` is `soft_guidance`, `balanced`, or `strong_arc`; `failure_policy` is `reversible`, `consequence`, or `fail_forward`; `pacing_curve` is `progressive`, `wave`, or `goal-pressure-payoff`; `random_event_rate` is usually `0`, `0.08`, `0.15`, or `0.3`.
- `event_packages`: resolved event packages; used only by the background director planner and empty when event packages are disabled.
- `actor_state`: resolved State System schema with `templates`, `trait_pools`, and `initial_actors`. `templates[].fields[]` define `path`, `name`, `type`, `default`, optional `min`/`max`, `options`, and `update_instruction`; optional `group` and `display` are presentation-only hints for the stage state ledger (`display` is `stat`, `inline`, `block`, or `list`; empty falls back to shape-based layout) and never affect state updates. Object fields are generic JSON maps: use stable, readable story-language keys as record IDs, and do not duplicate or require an inner name unless the user explicitly defines that shape. `templates[].trait_rules[]` bind a `pool_id` and positive `draw_count`. `trait_pools[].traits[]` contain only `id`, `name`, `summary`, and `weight`. Initial and runtime-created Actors use the same backend creation flow: template defaults, instance overrides, then automatic trait draws. The assigned definitions are persisted as snapshots under `actors.<actor_id>.traits`. Every state field and assigned trait is available to the Agent and state panel; private future planning belongs in the Director's private files rather than Actor State.
- `trpg_system`: resolved fixed-d20 rule templates for checks only. Each rule uses `label`, `dice`, `modifier`, `failure_policy`, `difficulty_guidance`, `state_effect_guidance`, `trigger`, `success_hint`, and `failure_hint`.
Historical facts come from committed Turns and can be retrieved with `search_story_history`; current computable facts belong in Actor State, stable canon belongs in Lore, and future intent belongs in `director.md`. Do not create another writeable continuity store inside a Story Director preset.

Traits are state snapshots only; ordinary numeric or field effects must remain typed Actor state patches. StateOps are an internal replay mechanism and are not part of the Story Director resource contract.

Do not change `version`, `path`, `custom`, `invalid`, `error`, `created_at`, or `updated_at` unless preserving an existing complete object from `read_story_directors`.
Do not use empty IDs to mean disabled; use the explicit `*_disabled` switches. Event content is referenced through `event_package_ids`.

## Event Cards

Event packages are standalone resources made of rich event cards. Do not generate keyword-only category packages.

Each `events[]` item in an event package should use this schema:

- `id`: stable ASCII ID, unique inside the director.
- `type_name`: user-visible event type name, for example `外门考核打脸`.
- `description_markdown`: Markdown event card, up to 8000 characters.
- `enabled`: boolean.
- `category`: broad category such as `打脸`, `奇遇`, `学院`, `恋爱`.
- `tags`: short searchable labels.
- `weight`: positive number, usually `1`.
- `cooldown_turns`: non-negative integer, usually `2`.
- `intensity`: short value such as `low`, `medium`, `high`.

`description_markdown` should contain these sections:

```markdown
## 触发场景

## 背景融合方式

## 大致事件逻辑（起承转合）

## 事件回收 / 后果

## 奖励 / 代价

## 避免生硬的约束
```

Every card must bind to at least one concrete source from the work: a world rule, faction, place, item, character relationship, current conflict source, or user-provided premise. Do not generate generic "any protagonist anywhere" cards unless the user explicitly asks for a generic template package.

Default generation strategy:

- Generate 12-24 event cards in one package when the user asks for an event pack. Write the package with `write_event_packages`, then add its ID to `story_director.module_refs.event_package_ids` only if the user asked to attach it to a director.
- Cover a mix of 打脸, 扮猪吃虎, 奇遇, 秘境, 天降, 意外, 世界事件, 冲突, 学院, 比拼, 排行, 恋爱, 英雄救美, 误会与消解 where suitable for the actual work.
- Each card should describe a flexible reusable situation, not a fixed future chapter outline.
- The event must integrate with user action and current background; do not force the protagonist into a single choice.
- Include payoff/recovery hooks so the Director Agent can close the event later without leaving dangling pressure.
- If lore was not read, write cards using only user-provided facts and clearly keep them generic.

## Rule Checks

Use the fixed-d20 rule-template schema. `dice` must be `1d20`. `modifier` is a numeric difficulty adjustment where positive values are harder and negative values are easier. `failure_policy` must be `fail_forward`, `success_at_cost`, `blocked`, or `hard_failure`. Write `difficulty_guidance` as natural-language criteria for how the Interactive Agent should choose runtime `difficulty` and `bonuses`; write `state_effect_guidance` as natural-language guidance for choosing concrete `outcomes.state_changes`.

Rules are guidance for the Interactive Agent when it decides whether to call `prepare_interactive_turn`; the actual tool performs one d20 check per turn. Do not store advantage/disadvantage in the template; the Agent chooses runtime `roll_mode` from current character state. `modifier` is tool-side fixed difficulty correction, not prose guidance. Put reusable state-mutation principles in `state_effect_guidance`; concrete numeric changes still belong in the turn outcome or state-system tools.

When writing the director back, use `write_story_directors` with the complete updated director object, preserve unrelated `module_refs`, and include a concise change message. When writing event cards, use `write_event_packages` with the complete updated package object.
