# Story Director (`story_director`)

Story Directors are Game-mode orchestration presets. They compose other reusable modules rather than duplicating their contents.

## Important fields

- `id`, `name`, `description`
- `module_refs`: `narrative_style_id`, `event_package_ids`, `rule_system_id`, `actor_state_id`, `image_preset_id`, plus matching `*_disabled` switches
- `strategy`: pacing, failure, event-frequency, Director-agent, rule-visibility/consumption, prompt, branch-planning, and planning-template policy

Resolve every referenced module with `config_read` before applying the Director. To disable a module, set its explicit `*_disabled` flag and preserve the ID so it can be re-enabled later; do not use an empty ID as a disable signal.

The expanded `event_packages`, `trpg_system`, `actor_state`, and `resolved_snapshot` fields are derived inspection data. Preserve them when basing an update on `get`, but make composition choices through `module_refs`. Future plans belong in per-story Director files, stable canon in Lore, current facts in Actor State, and committed history in Turns.

Use the complete current value as the update base and pass the latest `updated_at` as `revision`.
