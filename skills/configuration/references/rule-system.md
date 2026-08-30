# Rule system (`rule_system`)

Rule systems define one reusable Game-mode fixed-d20 adjudication style and optional typed State System bindings.

This resource uses `user` scope and complete editable-resource replacement. Update/delete require the latest content-addressed `revision` returned by `get`; `updated_at` is display metadata and is not a concurrency token.

## Top-level fields

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | create: recommended | Stable lowercase letters/digits/hyphen ID. |
| `name` | string | yes | User-visible name, up to 256 bytes. |
| `description` | string | no | Adjudication philosophy, up to 1024 bytes. |
| `actor_state_id` | string | when bindings exist | Exact `state_system` ID whose template and field identities the bindings use. |
| `trpg_system.rule_templates` | object[] | yes | Complete ordered rule-template collection; every valid non-blank template is retained. |

Do not send host-owned version, path, ownership/validation fields, or timestamps.

## Rule template fields

| Field | Type / values | Meaning |
| --- | --- | --- |
| `id` | stable string | Audit identity. |
| `label` | string | User-visible label. |
| `dice` | `1d20` | Only supported dice expression. |
| `modifier` | number | Fixed target modifier: positive is harder, negative is easier. |
| `failure_policy` | `fail_forward`, `success_at_cost`, `blocked`, `hard_failure` | Default failure handling. |
| `trigger` | string | When a risky action should use this check. |
| `must_check_examples` | string[] | Positive boundary examples; every distinct non-empty example is retained. |
| `skip_check_examples` | string[] | Cases that should be resolved without a roll; every distinct non-empty example is retained. |
| `difficulty_guidance` | string | How runtime facts move difficulty between supported bands. Do not hard-code a single scene's difficulty. |
| `state_effect_guidance` | string | General consequence guidance; structured recurring effects belong in bindings. |
| `success_hint`, `failure_hint` | string | Narrative outcome guidance. |
| `state_bindings` | object[] | Optional reusable state-driven modifier and outcome rules; every valid binding is retained. |

Runtime advantage/disadvantage, concrete difficulty, and current actor IDs belong to the story Agent's check request, not this preset.

## State Binding fields

Each binding is a named situation under the rule template:

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | stable string | Unique binding ID. |
| `label` | string | Audit/user label. |
| `trigger` | string | Situation in which the story Agent should select this binding. |
| `actor_template_id` | string | Required exact template ID from the linked State System. |
| `target_template_id` | string | Required when any nested item uses `source: target`. |
| `modifiers` | object[] | Number fields converted into check bonus (`advantage`) or target resistance (`resistance`). |
| `narrative_state_refs` | object[] | State facts the Agent should consult without computing a number. |
| `outcome_state_changes` | object[] | Typed numeric deltas produced for selected outcomes. |

### Modifier

| Field | Values / default | Meaning |
| --- | --- | --- |
| `source` | `actor` or `target` | Which runtime Actor supplies the field. |
| `field_id` | exact State System field name | Must resolve to a number, or to an object containing a number selected by `value_path`. |
| `value_path` | string[] | Optional nested object keys below `field_id`. |
| `effect` | `advantage` or `resistance` | Adds to the check result or target resistance. |
| `scale` | number, default 1 | Multiplies the field value. Zero normalizes to 1. |
| `offset` | number, default 0 | Added after scaling. |
| `min`, `max` | optional numbers | Clamp the computed contribution. |
| `rounding` | `none`, `floor`, `ceil`, `nearest`; default `nearest` | Applied before clamping. |
| `required` | boolean, default true | Missing/non-number state fails the check request when true; otherwise it produces a warning and skips this modifier. |

### Narrative state reference

`source` is `actor`, `target`, or `scene`; non-scene sources require `field_id`. `usage` is `check_decision`, `difficulty`, `outcome_design`, or `prose` and defaults to `outcome_design`. `guidance` explains how to interpret the fact. A `scene` source does not name a State System field.

### Outcome state change

`outcome` is `critical_success`, `success`, `failure`, or `critical_failure`. Each `state_changes` item has `source` (`actor`/`target`), an exact numeric `field_id`, a `reason`, and `change_formula`:

```text
change = base + sum(source.field[value_path] * scale + offset)
```

The formula accepts `terms`, optional `min`/`max`, and `rounding` (`none`, `floor`, `ceil`, `nearest`). Its result is a delta applied to the selected numeric field, not an absolute replacement.

## Basic create example

```text
config_apply({
  "operation": "create",
  "resource": "rule_system",
  "scope": "user",
  "value": {
    "id": "clue-forward-d20",
    "name": "Clue-forward d20",
    "description": "Checks determine cost and clarity without deleting the investigation path.",
    "trpg_system": {
      "rule_templates": [
        {
          "id": "clue-forward-check",
          "label": "Clue-forward check",
          "dice": "1d20",
          "modifier": 0,
          "failure_policy": "fail_forward",
          "trigger": "Use when an investigation action has uncertainty and a meaningful cost; do not roll merely to notice an available core clue.",
          "must_check_examples": ["Recovering fragile evidence while guards approach.", "Testing a dangerous inference before acting on it."],
          "skip_check_examples": ["Reading an already obtained document.", "Noticing the central clue in a carefully searched scene."],
          "difficulty_guidance": "Default normal. Reduce difficulty for specific methods, relevant expertise, corroboration, or proper tools; increase it for time pressure, contamination, or active opposition.",
          "state_effect_guidance": "Failures should add time pressure, exposure, uncertainty, or resource cost while preserving another route to progress.",
          "success_hint": "Give a reliable conclusion or a strong new route.",
          "failure_hint": "Give incomplete or costly progress, not a dead end."
        }
      ]
    }
  }
})
```

## State Binding create example

First `get` the linked `state_system` and verify template `investigator` has numeric fields `focus` and `stress`:

```text
config_apply({
  "operation": "create",
  "resource": "rule_system",
  "scope": "user",
  "value": {
    "id": "focused-investigation-d20",
    "name": "Focused investigation d20",
    "description": "Focus improves investigation checks; failed pressure checks increase stress.",
    "actor_state_id": "investigation-state",
    "trpg_system": {
      "rule_templates": [
        {
          "id": "focused-investigation-check",
          "label": "Focused investigation check",
          "dice": "1d20",
          "modifier": 0,
          "failure_policy": "fail_forward",
          "trigger": "Use for uncertain investigative actions under pressure.",
          "must_check_examples": ["Reconstructing evidence before the scene is sealed."],
          "skip_check_examples": ["Reviewing a clue already secured."],
          "difficulty_guidance": "Use current opposition, time pressure, tools, and corroboration.",
          "state_effect_guidance": "Prefer typed stress changes for recurring pressure.",
          "success_hint": "Advance the conclusion and expose a concrete route.",
          "failure_hint": "Advance with uncertainty or cost.",
          "state_bindings": [
            {
              "id": "focus-under-pressure",
              "label": "Focus under pressure",
              "trigger": "The investigator relies on concentration while time or opposition creates pressure.",
              "actor_template_id": "investigator",
              "modifiers": [
                {
                  "source": "actor",
                  "field_id": "focus",
                  "effect": "advantage",
                  "scale": 1,
                  "offset": 0,
                  "min": 0,
                  "max": 4,
                  "rounding": "nearest",
                  "required": true
                }
              ],
              "narrative_state_refs": [
                {
                  "source": "actor",
                  "field_id": "stress",
                  "usage": "outcome_design",
                  "guidance": "High stress should make costs more immediate and visible."
                }
              ],
              "outcome_state_changes": [
                {
                  "outcome": "failure",
                  "state_changes": [
                    {
                      "source": "actor",
                      "field_id": "stress",
                      "change_formula": {"base": 1, "rounding": "nearest", "min": 0, "max": 3},
                      "reason": "Failed investigation under pressure increases stress."
                    }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  }
})
```

## Update and verification

Update is a complete replacement. Start from `get`, retain the top-level identity, `actor_state_id`, the complete rule template, every example, and every nested binding. This valid example updates the basic rule's difficulty guidance while preserving all of its other fields:

```text
config_apply({
  "operation": "update",
  "resource": "rule_system",
  "scope": "user",
  "id": "clue-forward-d20",
  "revision": "REVISION_FROM_GET",
  "value": {
    "id": "clue-forward-d20",
    "name": "Clue-forward d20",
    "description": "Checks determine cost and clarity without deleting the investigation path.",
    "trpg_system": {
      "rule_templates": [
        {
          "id": "clue-forward-check",
          "label": "Clue-forward check",
          "dice": "1d20",
          "modifier": 0,
          "failure_policy": "fail_forward",
          "trigger": "Use when an investigation action has uncertainty and a meaningful cost; do not roll merely to notice an available core clue.",
          "must_check_examples": ["Recovering fragile evidence while guards approach.", "Testing a dangerous inference before acting on it."],
          "skip_check_examples": ["Reading an already obtained document.", "Noticing the central clue in a carefully searched scene."],
          "difficulty_guidance": "Default normal. Reduce difficulty for a specific method, relevant expertise, corroboration, proper tools, or a safely prepared position; increase it for time pressure, contamination, active opposition, or acting on an untested assumption.",
          "state_effect_guidance": "Failures should add time pressure, exposure, uncertainty, or resource cost while preserving another route to progress.",
          "success_hint": "Give a reliable conclusion or a strong new route.",
          "failure_hint": "Give incomplete or costly progress, not a dead end."
        }
      ]
    }
  }
})
```

Read the Rule System again and verify the requested change, the linked `actor_state_id` when present, rule count, binding IDs, field identities, and revision.
