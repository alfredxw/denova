# Narrative style (`narrative_style`)

Narrative styles are shared prose-policy presets. `modes` controls whether a style appears in Writing, Game, or both; shared style-reference files can be attached globally or for named scene types.

This resource uses `user` scope. Create and update submit a complete editable resource. Update/delete require the latest content-addressed `revision` returned by `get`; `updated_at` is display metadata and is not a concurrency token.

## Field reference

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | create: recommended | Stable ID containing letters, digits, `_`, or `-`. It cannot change during update. |
| `name` | string | yes | User-visible name. |
| `description` | string | no | Concise catalog description. |
| `modes` | string[] | no | Any non-empty subset of `writing`, `game`. Missing or unusable values mean both modes for legacy compatibility. |
| `style_refs` | string[] | no | Global style-reference `display_path` values; every distinct valid path is retained. Resolve each with `style_reference` first. |
| `style_rules` | object[] | no | Scene-specific mappings. Each item has non-empty `scene` and any number of distinct `style_refs`. Preserve returned legacy `style_contents`, but do not add new inline excerpts. |
| `context_policy` | object | no | Short policy strings `creator`, `lore`, and `runtime_state`. Defaults are `always`, `relevant`, `always`; these are guidance tokens, not arbitrary story content. |
| `slots` | object[] | yes | Ordered prompt blocks. At least one slot is required. |

Each `slots` item contains:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | recommended | Stable letters/digits/`_`/`-` identity. Keep it unchanged across edits. |
| `name` | string | no | User-visible label; defaults to `id`. |
| `target` | enum | yes | `system` for stable identity/policy or `turn_context` for per-turn behavior. |
| `enabled` | boolean | yes | Whether the slot is injected. |
| `content` | string | yes when enabled | Focused narrative instruction for that target. |

Do not send host-owned `version`, `path`, `custom`, `builtin_overridden`, `invalid`, `error`, `created_at`, `updated_at`, or `revision` when constructing a new value. On update, rebuild the complete editable value from `get` and leave those fields untouched; pass the returned top-level `revision` only through `config_apply.revision`.

## Create example

First verify every `style_refs` path exists. Then create the complete style:

```text
config_apply({
  "operation": "create",
  "resource": "narrative_style",
  "scope": "user",
  "value": {
    "id": "close-suspense",
    "name": "Close suspense",
    "description": "Restricted viewpoint and escalating sensory tension.",
    "modes": ["writing", "game"],
    "style_refs": [".denova/styles/tense-close-third.md"],
    "style_rules": [
      {
        "scene": "quiet-investigation",
        "style_refs": [".denova/styles/tense-close-third.md"]
      }
    ],
    "context_policy": {
      "creator": "always",
      "lore": "relevant",
      "runtime_state": "always"
    },
    "slots": [
      {
        "id": "identity",
        "name": "Narrative identity",
        "target": "system",
        "enabled": true,
        "content": "Stay in close third person. Build tension through perception and uncertainty, not unexplained omniscient facts."
      },
      {
        "id": "turn-pressure",
        "name": "Turn pressure",
        "target": "turn_context",
        "enabled": true,
        "content": "Each scene response should change one concrete risk, clue, relationship, or available action."
      }
    ]
  }
})
```

## Complete update example

Read the current item, retain every existing `style_refs`, `style_rules`, policy field, and slot, then submit the full editable value. This example changes only the description and disables one existing slot:

```text
config_apply({
  "operation": "update",
  "resource": "narrative_style",
  "scope": "user",
  "id": "close-suspense",
  "revision": "REVISION_FROM_GET",
  "value": {
    "id": "close-suspense",
    "name": "Close suspense",
    "description": "Restricted viewpoint with restrained, escalating tension.",
    "modes": ["writing", "game"],
    "style_refs": [".denova/styles/tense-close-third.md"],
    "style_rules": [
      {"scene": "quiet-investigation", "style_refs": [".denova/styles/tense-close-third.md"]}
    ],
    "context_policy": {"creator": "always", "lore": "relevant", "runtime_state": "always"},
    "slots": [
      {"id": "identity", "name": "Narrative identity", "target": "system", "enabled": true, "content": "Stay in close third person. Build tension through perception and uncertainty, not unexplained omniscient facts."},
      {"id": "turn-pressure", "name": "Turn pressure", "target": "turn_context", "enabled": false, "content": "Each scene response should change one concrete risk, clue, relationship, or available action."}
    ]
  }
})
```

Verify the requested change and that every unrequested slot and reference remains. Updating a built-in ID creates an override. Deleting a built-in override is an explicit restore action; deleting a custom ID removes it.

Keep slots about narrative behavior only. Story facts belong in Lore, current facts in runtime state, and credentials or tool permissions never belong here.
