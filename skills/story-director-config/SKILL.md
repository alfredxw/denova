---
name: story-director-config
description: 配置管理 Agent 创建或更新 Story Director 资源时使用。Use when config_manager creates or updates Denova Story Director resources.
agent: config_manager
---

# Story Director Config

Use this skill before calling `write_story_directors` or `write_actor_states`. For event package and event card creation/update, load `event-package-config` instead.

## Workflow

1. Call `list_story_directors` first. For updates, call `read_story_directors` for the exact director IDs.
2. Call `list_event_packages` before changing a director's event package references. For event-card content changes, load `event-package-config`.
3. Call `list_actor_states` before changing a director's `actor_state_id`; call `read_actor_states` before editing state templates, fields, trait pools, or trait rules.
4. Use `write_story_directors` for director create/update/delete and `write_actor_states` for State System schema changes. Do not edit JSON files directly.
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
- `actor_state`: resolved State System schema with `templates`, `trait_pools`, and `initial_actors`. `templates[].fields[]` define `path`, `name`, `type`, `default`, optional `min`/`max`, `options`, `visibility` (`visible`, `hidden`, or `spoiler`), and `update_instruction`. `templates[].trait_rules[]` bind a `pool_id` and positive `draw_count`. `trait_pools[].traits[]` contain only `id`, `name`, `summary`, `weight`, and `visibility`. Initial and runtime-created Actors use the same backend creation flow: template defaults, instance overrides, then automatic trait draws. The assigned definitions are persisted as snapshots under `actors.<actor_id>.traits`.
- `trpg_system`: resolved fixed-d20 rule templates for checks only. Each rule uses `label`, `dice`, `modifier`, `failure_policy`, `difficulty_guidance`, `state_effect_guidance`, `trigger`, `success_hint`, and `failure_hint`.
Historical facts come from committed Turns and can be retrieved with `search_story_history`; current computable facts belong in Actor State, stable canon belongs in Lore, and future intent belongs in `director.md`. Do not create another writeable continuity store inside a Story Director preset.

Traits are state snapshots only; ordinary numeric or field effects must remain typed Actor state patches. StateOps are an internal replay mechanism and are not part of the Story Director resource contract.

Do not change `version`, `path`, `custom`, `invalid`, `error`, `created_at`, or `updated_at` unless preserving an existing complete object from `read_story_directors`.
Do not use empty IDs to mean disabled; use the explicit `*_disabled` switches. Event content is referenced through `event_package_ids`.

## Event Cards

Event package and event card creation/update is handled by the `event-package-config` skill. Load that skill when the user asks to create or modify event cards.

To attach an existing event package to a director, add its ID to `module_refs.event_package_ids` and write the director back with `write_story_directors`.

## Rule Checks

Use the fixed-d20 rule-template schema. `dice` must be `1d20`. `modifier` is a numeric difficulty adjustment where positive values are harder and negative values are easier. `failure_policy` must be `fail_forward`, `success_at_cost`, `blocked`, or `hard_failure`. Write `difficulty_guidance` as natural-language criteria for how the Interactive Agent should choose runtime `difficulty` and `bonuses`; write `state_effect_guidance` as natural-language guidance for choosing concrete `outcomes.state_changes`.

Rules are guidance for the Interactive Agent when it decides whether to call `prepare_interactive_turn`; the actual tool performs one d20 check per turn. Do not store advantage/disadvantage in the template; the Agent chooses runtime `roll_mode` from current character state. `modifier` is tool-side fixed difficulty correction, not prose guidance. Put reusable state-mutation principles in `state_effect_guidance`; concrete numeric changes still belong in the turn outcome or state-system tools.

When writing the director back, use `write_story_directors` with the complete updated director object, preserve unrelated `module_refs`, and include a concise change message.
