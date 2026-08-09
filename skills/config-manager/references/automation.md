# Automation (`automation`)

Automations are Project-owned task definitions executed by that Project's Agent. Config Manager can edit definitions only; runtime IDs, trigger state, run history, timestamps, durable command identities, and derived action policy are never accepted by `config_apply`.

Automation update is a sparse patch. Omitted fields remain unchanged, but a present `triggers` array replaces the complete trigger list. Update/delete require the exact scope and latest `revision` returned by `get`.

## Project ownership

The only supported scope is `workspace`, with `target.kind: "workspace"`. The host binds this resource to the Config Manager's current Project; a model-supplied workspace path never selects or authorizes another Project.

`target` is required on create and immutable on update. To manage another Project, the user must open Automation's Config Agent for that Project. Never create a user-scoped/global automation.

## Editable fields

| Field | Type / values | Create | Update semantics |
| --- | --- | --- | --- |
| `target` | object with `kind` | required | forbidden |
| `enabled` | boolean | optional, safe default false | replaces the flag even when false |
| `name` | string | optional | replaces when present |
| `template` | `memory_consolidation`, `review`, `continue_writing`, `custom_prompt` | defaults to `custom_prompt` | replaces when present |
| `prompt` | string | recommended | replaces when present; `""` clears it |
| `model_profile_id` | string | optional | replaces when present; `""` clears the override |
| `session_strategy` | `per_run`, `per_task` | defaults to `per_run` | `per_run` creates a conversation for every run; `per_task` serializes runs through one fixed conversation |
| `schedule` | schedule object | optional compatibility field | replaces the fallback/primary schedule when present; prefer trigger-local schedules |
| `triggers` | trigger[] | optional | complete array replacement when present |

There is no task-level execution, write-scope, or output policy. The Project Agent receives its normal configured tools. Put the complete requested behavior in `prompt`, including whether it should inspect, change, or create Project content and where any durable output belongs.

## Schedule fields

| `kind` | Additional fields | Limits |
| --- | --- | --- |
| `manual` | none | Never automatically due. |
| `daily` | `hour`, `minute` | hour 0–23, minute 0–59. |
| `weekly` | `weekday`, `hour`, `minute` | weekday 0–6, Sunday = 0. |
| `monthly` | `day_of_month`, `hour`, `minute` | day 1–31; nonexistent dates in a month do not run. |
| `every_hours` | `every_hours`, optional `minute` | interval 1–168. |

Never send derived `cron`; the backend computes it.

## Trigger fields

Every trigger has stable `id`, `type`, `enabled`, optional `name`, and optional `notify_policy` (`inbox` or `silent`). Do not send trigger-level `action_policy`; current Project automations are admitted automatically when the trigger matches.

| Trigger type | Additional fields | Meaning |
| --- | --- | --- |
| `manual` | none | User-invoked only. |
| `schedule` | `schedule` | Runs when its normalized schedule is due. Defaults to `silent` notification. |
| `semantic` | `semantic_condition` | Evaluates a bounded natural-language condition against new evidence. Prefer `inbox` notification. |
| `chapter_batch` | `chapter_batch_size` | Evaluates after a positive number of new chapters; defaults to 5. Prefer `inbox`. |

Stable trigger IDs preserve deduplication state. Replacing a trigger with a new ID creates a new trigger identity.

## Project automation create example

This disabled manual task tells the Project Agent exactly what to do in its prompt:

```text
config_apply({
  "operation": "create",
  "resource": "automation",
  "scope": "workspace",
  "value": {
    "target": {"kind": "workspace"},
    "enabled": false,
    "name": "Continuity review",
    "template": "review",
    "prompt": "Review newly written chapters for continuity conflicts. Report findings with file references; do not edit files.",
    "session_strategy": "per_run",
    "triggers": [
      {
        "id": "manual-review",
        "type": "manual",
        "enabled": true,
        "name": "Manual review",
        "notify_policy": "inbox"
      }
    ]
  }
})
```

## Sparse update example

Read the exact task and scope first. To change only its prompt, omit `triggers` and all other fields:

```text
config_apply({
  "operation": "update",
  "resource": "automation",
  "scope": "workspace",
  "id": "CATALOG_ID_FROM_GET",
  "revision": "REVISION_FROM_GET",
  "value": {
    "prompt": "Review newly written chapters for continuity, timeline, naming, and unresolved setup. Report findings with file references; do not edit files."
  }
})
```

Verify that the prompt changed while the target, enabled flag, template, conversation strategy, and trigger IDs stayed identical.

## Scheduled trigger replacement example

To replace the complete trigger list with a disabled manual trigger and one enabled weekday schedule:

```text
config_apply({
  "operation": "update",
  "resource": "automation",
  "scope": "workspace",
  "id": "CATALOG_ID_FROM_GET",
  "revision": "REVISION_FROM_GET",
  "value": {
    "triggers": [
      {"id": "manual-review", "type": "manual", "enabled": true, "name": "Manual review", "notify_policy": "inbox"},
      {
        "id": "weekday-review",
        "type": "schedule",
        "enabled": true,
        "name": "Weekday review",
        "notify_policy": "silent",
        "schedule": {"kind": "weekly", "weekday": 1, "hour": 9, "minute": 30}
      }
    ]
  }
})
```

For semantic or chapter batching, use for example:

```text
{"id":"major-character-change","type":"semantic","enabled":true,"notify_policy":"inbox","semantic_condition":"A named major character's allegiance, core goal, or public relationship changed materially."}
{"id":"five-chapter-review","type":"chapter_batch","enabled":true,"notify_policy":"inbox","chapter_batch_size":5}
```

## Read-only and verification fields

Never copy `id`, `catalog_id`, `revision`, `scope`, returned `target`, `default_action_policy`, derived `schedule.cron`, trigger `action_policy`, trigger state, last/recent runs, timestamps, historical `output_path`, or archive/runtime fields into `value`. Top-level mutation `id`, `scope`, and `revision` carry identity and concurrency.

After every change, `get` the exact catalog ID in the same scope and verify the requested fields plus every preservation-sensitive field. Delete only on explicit request; a task with an active run may reject deletion until runtime reconciliation completes.
