# Game Preset (`story_director`)

Game Presets select reusable narrative, event, rule, state, and image modules and optionally add free-form planning guidance. They do not duplicate module content. Planning itself belongs to the Game Agent and is enabled or disabled per story, not by this resource.

This resource uses `user` scope and complete editable-resource replacement. Update/delete require the latest content-addressed `revision` returned by `get`; `updated_at` is display metadata and is not a concurrency token.

## Field reference

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | create: recommended | Stable lowercase letters/digits/hyphen ID. It cannot change on update. |
| `name` | string | yes | User-visible preset name, up to 256 bytes. |
| `description` | string | no | Composition summary, up to 1024 bytes. |
| `module_refs` | object | yes | IDs and explicit disabled switches for composed modules. |
| `strategy` | object | yes | Rule handling and optional creator-authored planning guidance. |

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
| `rule_state_consumption_mode` | `hybrid_auto`, `suggestions_only` | Whether valid rule-produced state changes are applied automatically or retained only as suggestions. |
| `rule_visibility_mode` | `audit_only`, `public_roll` | Whether checks stay audit-only or expose a public roll. |
| `prompt_markdown` | string | Free-form creator guidance for how the Game Agent should plan this kind of story, at most 256 KiB. It may express preferred freedom, momentum, tone, clue handling, or taboos without a fixed template. Never place canon or future prose here. |

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
      "rule_state_consumption_mode": "hybrid_auto",
      "rule_visibility_mode": "audit_only",
      "prompt_markdown": "Keep clues legible, respect player-led detours, and let consequences create new options instead of forcing a fixed route."
    }
  }
})
```

## Complete update example

This example disables events and changes only the planning style. It preserves the event-package ID and every other module and strategy field:

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
      "rule_state_consumption_mode": "hybrid_auto",
      "rule_visibility_mode": "audit_only",
      "prompt_markdown": "Favor a slow-burn mystery, but let the player abandon prepared clues and pursue their own priorities."
    }
  }
})
```

Read the Game Preset again. Verify the requested values, every preserved reference, `resolved_snapshot.status`, and any warnings. Updating a built-in ID creates an override; deleting that override restores the built-in preset. Future plans are maintained by the Game Agent in each branch, stable canon belongs in Lore, current facts in Actor State, and committed history in Turns.
