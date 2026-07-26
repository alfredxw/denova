# Story Director (`story_director`)

Story Directors are Game-mode composition presets. They select reusable narrative, event, rule, state, and image modules and add orchestration policy; they do not duplicate those modules' content.

This resource uses `user` scope and complete editable-resource replacement. Update/delete require the latest `updated_at` returned by `get` as `revision`.

## Field reference

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | create: recommended | Stable lowercase letters/digits/hyphen ID. It cannot change on update. |
| `name` | string | yes | User-visible preset name, up to 256 bytes. |
| `description` | string | no | Composition summary, up to 1024 bytes. |
| `module_refs` | object | yes | IDs and explicit disabled switches for composed modules. |
| `strategy` | object | yes | Orchestration and planning behavior. |

### `module_refs`

| Module | ID field | Disable field |
| --- | --- | --- |
| Narrative style | `narrative_style_id` | `narrative_style_disabled` |
| Event packages | `event_package_ids` | `event_packages_disabled` |
| Rule system | `rule_system_id` | `rule_system_disabled` |
| State system | `actor_state_id` | `actor_state_disabled` |
| Image preset | `image_preset_id` | `image_preset_disabled` |

Resolve every non-disabled ID with `config_read` before applying. To disable a module, set its `*_disabled` flag to `true` and preserve the configured ID so it can be re-enabled later. An empty ID is not a disable signal. A missing module may fall back to the last resolved snapshot and produce a warning; treat that as degraded, not successful resolution.

### `strategy`

| Field | Supported values / limits | Meaning |
| --- | --- | --- |
| `enabled` | boolean | Master orchestration switch. |
| `mainline_strength` | `soft_guidance`, `balanced`, `strong_arc` | How strongly the Director protects a prepared arc. Default `soft_guidance`. |
| `failure_policy` | `reversible`, `consequence`, `fail_forward` | High-level narrative handling of setbacks. Default `reversible`; this is separate from a rule template's dice failure policy. |
| `pacing_curve` | `progressive`, `wave`, `goal-pressure-payoff` | Long-form pacing pattern. Default `progressive`. |
| `event_frequency` | `off`, `sparse`, `balanced`, `frequent` | Frequency of event-package opportunities. Default `balanced`. |
| `director_agent_mode` | `triggered`, `every_turn`, `off` | When the Director Agent replans. Default `triggered`. |
| `rule_state_consumption_mode` | `hybrid_auto`, `director_only` | Whether rule-produced state changes may be applied automatically or only through the Director. |
| `rule_visibility_mode` | `audit_only`, `public_roll` | Whether checks stay audit-only or expose a public roll. |
| `prompt_markdown` | string | Additional Director policy. Keep at most 64 KiB for UI compatibility; never place canon or future prose here. |
| `branch_planning_turns` | integer 1–12 | Near-term branch horizon; default 5. |
| `planning_templates` | object | Optional `plan` and `agent_brief` Markdown templates. Omit to use maintained defaults. |

Custom planning templates should preserve their required headings and `{{branch_planning_turns}}`. The private `plan` may contain hidden orchestration; `agent_brief` must contain only facts safe for the story Agent and player-facing narrative.

`event_packages`, `trpg_system`, `actor_state`, and `resolved_snapshot` are expanded inspection data. `version`, `path`, ownership flags, validation fields, and timestamps are also host-owned. Make composition decisions only through `module_refs` and `strategy`.

## Create example

After listing and reading each referenced module:

```text
config_apply({
  "operation": "create",
  "resource": "story_director",
  "scope": "user",
  "value": {
    "id": "measured-mystery",
    "name": "Measured mystery",
    "description": "Clue-forward investigation with restrained event pressure.",
    "module_refs": {
      "narrative_style_id": "rhythm",
      "narrative_style_disabled": false,
      "event_package_ids": ["default"],
      "event_packages_disabled": false,
      "rule_system_id": "default",
      "rule_system_disabled": false,
      "actor_state_id": "default",
      "actor_state_disabled": false,
      "image_preset_id": "game-cg",
      "image_preset_disabled": false
    },
    "strategy": {
      "enabled": true,
      "mainline_strength": "soft_guidance",
      "failure_policy": "fail_forward",
      "pacing_curve": "wave",
      "event_frequency": "sparse",
      "director_agent_mode": "triggered",
      "rule_state_consumption_mode": "hybrid_auto",
      "rule_visibility_mode": "audit_only",
      "branch_planning_turns": 5
    }
  }
})
```

## Complete update example

This example disables events and changes only the branch horizon. It preserves the event-package ID and every other module and strategy field:

```text
config_apply({
  "operation": "update",
  "resource": "story_director",
  "scope": "user",
  "id": "measured-mystery",
  "revision": "REVISION_FROM_GET",
  "value": {
    "id": "measured-mystery",
    "name": "Measured mystery",
    "description": "Clue-forward investigation with restrained event pressure.",
    "module_refs": {
      "narrative_style_id": "rhythm",
      "narrative_style_disabled": false,
      "event_package_ids": ["default"],
      "event_packages_disabled": true,
      "rule_system_id": "default",
      "rule_system_disabled": false,
      "actor_state_id": "default",
      "actor_state_disabled": false,
      "image_preset_id": "game-cg",
      "image_preset_disabled": false
    },
    "strategy": {
      "enabled": true,
      "mainline_strength": "soft_guidance",
      "failure_policy": "fail_forward",
      "pacing_curve": "wave",
      "event_frequency": "sparse",
      "director_agent_mode": "triggered",
      "rule_state_consumption_mode": "hybrid_auto",
      "rule_visibility_mode": "audit_only",
      "branch_planning_turns": 7
    }
  }
})
```

Read the Director again. Verify the requested values, every preserved reference, `resolved_snapshot.status`, and any warnings. Updating a built-in ID creates an override; deleting that override restores the built-in preset. Future plans belong in per-story Director files, stable canon in Lore, current facts in Actor State, and committed history in Turns.
