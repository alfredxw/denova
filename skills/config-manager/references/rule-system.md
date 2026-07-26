# Rule system (`rule_system`)

Rule systems define reusable Game-mode fixed-d20 check guidance and optional State System bindings.

## Shape

- `id`, `name`, `description`
- optional `actor_state_id` referencing an existing `state_system`
- `trpg_system.rule_templates`

Each rule template uses a stable `id`, `label`, `dice`, numeric `modifier`, `failure_policy`, `trigger`, `must_check_examples`, `skip_check_examples`, `difficulty_guidance`, `state_effect_guidance`, success/failure hints, and optional `state_bindings`.

Rules:

- `dice` must be `1d20`.
- Positive `modifier` makes the target harder; negative makes it easier.
- `failure_policy` is `fail_forward`, `success_at_cost`, `blocked`, or `hard_failure`.
- Runtime advantage/disadvantage and concrete difficulty belong to the story Agent's check request, not the preset.
- State bindings must reference exact template/field identities from the linked State System; do not duplicate ordinary state changes in prose guidance.

Read the linked State System and the complete current rule system before changes. Apply a complete-resource update with the latest `updated_at` revision.
