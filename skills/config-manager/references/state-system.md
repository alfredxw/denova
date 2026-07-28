# State System (`state_system`)

State Systems define reusable Game-mode Actor schemas, initial Actors, and weighted trait pools. A story freezes a schema snapshot when initialized, so editing a reusable module affects new stories, not historical story state.

This resource uses `user` scope and complete editable-resource replacement. Update/delete require the latest `updated_at` returned by `get` as `revision`.

## Top-level fields

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | create: recommended | Stable lowercase letters/digits/hyphen module ID. |
| `name` | string | yes | User-visible name, up to 256 bytes. |
| `description` | string | no | Purpose and intended story types, up to 1024 bytes. |
| `actor_state.templates` | object[] | yes | At least one Actor template; no arbitrary collection-count limit. |
| `actor_state.initial_actors` | object[] | no | Initial Actor definitions; all valid entries are retained. |
| `actor_state.trait_pools` | object[] | no | Reusable weighted trait libraries; all valid entries are retained. |

Do not send host-owned version, path, ownership/validation fields, or timestamps.

## Template and field reference

A template has stable ASCII `id`, visible `name`, optional `description`, an ordered `fields` collection, and optional `trait_rules`.

Each field uses `name` as both its model-visible identity and user-visible label. It is an exact key, not a dotted path. It may contain localized text and punctuation but cannot be empty, contain `/`, or case-insensitively duplicate another field in the same template.

| Field property | Type / values | Meaning |
| --- | --- | --- |
| `name` | string | Stable field identity. Renaming it creates a different reusable field for new stories. |
| `type` | `number`, `string`, `bool`, `enum`, `object`, `list` | Value contract. Invalid/missing types normalize to `string`. |
| `default` | matching JSON value | Initial/fallback value. Use a number, string, boolean, enum string, object, or array matching `type`. |
| `min`, `max` | number | Number-only clamps. |
| `options` | string[] | Legal values for `enum`; the default must be one of them. |
| `description` | string | What the field means, not when it changes. |
| `update_instruction` | string | Exact conditions for updating and whether changes are replacement or numeric delta. |
| `group` | string | Optional UI ledger section. |
| `display` | `stat`, `inline`, `block`, `list` | Optional rendering hint; empty lets the UI infer it. |

Do not write legacy `id`, `path`, `legacy_path`, `order`, or `display_groups`; reusable modules persist the current field-name contract and array order.

## Initial Actors

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | yes | Stable Actor identity. Control characters and `.` are removed; keep it simple. |
| `name` | yes | User-visible name. |
| `template_id` | yes | Exact template ID in this module. Unknown templates cause the Actor to be dropped. |
| `role` | no | Narrative role; defaults to the template ID. |
| `description` | no | Stable starting description. |
| `state` | no | Field-name-to-value overrides. Unknown keys are discarded and values must match field types. |

## Trait pools and rules

A trait pool has stable ASCII `id`, visible `name`, optional `description`, and an ordered trait collection. Each trait has stable `id`, `name`, optional `summary`, and positive numeric `weight` (non-positive values normalize to 1).

A template's `trait_rules` entries contain exact `pool_id` and positive `draw_count`. The pool must exist and be non-empty; `draw_count` cannot exceed the pool size, and the total traits assigned by one template cannot exceed 24. Assigned traits are story snapshots; ordinary numeric or object effects still belong in typed state fields.

## Complete create example

This example demonstrates all six field types and provides the exact identities used by the Rule System binding example:

```text
config_apply({
  "operation": "create",
  "resource": "state_system",
  "scope": "user",
  "value": {
    "id": "investigation-state",
    "name": "Investigation state",
    "description": "Typed state for clue-driven investigations.",
    "actor_state": {
      "templates": [
        {
          "id": "investigator",
          "name": "Investigator",
          "description": "A player or NPC who actively develops the case.",
          "fields": [
            {
              "name": "focus",
              "type": "number",
              "default": 1,
              "min": 0,
              "max": 4,
              "description": "Current usable concentration.",
              "update_instruction": "Apply numeric deltas only when rest, pressure, injury, or a rule outcome materially changes concentration.",
              "group": "resources",
              "display": "stat"
            },
            {
              "name": "stress",
              "type": "number",
              "default": 0,
              "min": 0,
              "max": 10,
              "description": "Accumulated investigative pressure.",
              "update_instruction": "Apply numeric deltas after explicit relief or meaningful pressure; never infer a reset between scenes.",
              "group": "resources",
              "display": "stat"
            },
            {
              "name": "codename",
              "type": "string",
              "default": "unknown",
              "description": "Current public or operational codename.",
              "update_instruction": "Replace only when the identity used by the story changes.",
              "group": "identity",
              "display": "inline"
            },
            {
              "name": "compromised",
              "type": "bool",
              "default": false,
              "description": "Whether enemies have confirmed this Actor's investigative role.",
              "update_instruction": "Set true only after confirmed exposure; set false only after an explicit recovery event.",
              "group": "risk",
              "display": "inline"
            },
            {
              "name": "case_status",
              "type": "enum",
              "default": "open",
              "options": ["open", "stalled", "resolved"],
              "description": "Current case lifecycle state.",
              "update_instruction": "Replace with one legal option after a durable case-state transition.",
              "group": "case",
              "display": "inline"
            },
            {
              "name": "clue_counts",
              "type": "object",
              "default": {"confirmed": 0, "disputed": 0},
              "description": "Structured clue totals used for display and optional nested bindings.",
              "update_instruction": "Replace the complete object after confirming the new totals.",
              "group": "case",
              "display": "block"
            },
            {
              "name": "active_leads",
              "type": "list",
              "default": [],
              "description": "Short identifiers or summaries of currently actionable leads.",
              "update_instruction": "Replace the complete list when leads are opened, closed, merged, or invalidated.",
              "group": "case",
              "display": "list"
            }
          ],
          "trait_rules": [{"pool_id": "investigator-temperament", "draw_count": 1}]
        }
      ],
      "initial_actors": [
        {
          "id": "protagonist",
          "name": "Protagonist",
          "template_id": "investigator",
          "role": "protagonist",
          "description": "The initial player investigator.",
          "state": {"focus": 2, "codename": "rook", "case_status": "open"}
        }
      ],
      "trait_pools": [
        {
          "id": "investigator-temperament",
          "name": "Investigator temperament",
          "description": "Stable approaches that color investigation without replacing typed state.",
          "traits": [
            {"id": "methodical", "name": "Methodical", "summary": "Prefers corroboration and documented chains of evidence.", "weight": 2},
            {"id": "intuitive", "name": "Intuitive", "summary": "Moves quickly on patterns but still needs verification.", "weight": 1}
          ]
        }
      ]
    }
  }
})
```

## Complete update procedure

Call `get`, use the returned editable module as the base, and retain every template, field, initial Actor, trait pool, trait, and stable identity. Apply the complete value with the current revision. Omitting an array removes its contents for future story initialization.

After update, read the module again and verify:

- every unrequested template/field/pool remains;
- defaults still match their declared types;
- enum defaults remain in `options`;
- initial Actor overrides use exact field names;
- trait rules resolve to non-empty pools; and
- the revision changed.

Before changing a template ID, field name/type, or trait-pool ID, read every Rule System that binds to this State System. Such identity changes can break future module resolution even though existing stories retain their frozen snapshots.
