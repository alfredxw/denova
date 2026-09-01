# Event package (`event_package`)

An event package is a reusable Game-mode catalog of flexible event cards. Stories reference package IDs directly; editing a package affects future Game Agent planning without turning cards into a fixed chapter outline.

This resource uses `user` scope and complete editable-resource replacement. Update/delete require the latest content-addressed `revision` returned by `get`; `updated_at` is display metadata and is not a concurrency token.

## Field reference

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | create: recommended | Stable lowercase letters/digits/hyphen ID. |
| `name` | string | yes | User-visible package name, up to 256 bytes. |
| `description` | string | no | Package purpose, up to 1024 bytes. |
| `events` | object[] | no | Complete ordered card list. The backend does not impose an arbitrary card-count limit. |

Each event card contains:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | recommended | Stable letters/digits/`_`/`-` ID, unique within the package. |
| `type_name` | string | yes | Human-readable event type; falls back to `id`. |
| `description_markdown` | string | yes | Reusable situation design, at most 8,000 characters. |
| `enabled` | boolean | yes | Whether the card can be scheduled. |
| `category` | string | recommended | Grouping label; defaults to `type_name`. |
| `tags` | string[] | no | Distinct search/compatibility tags; all supplied tags are retained. |
| `intensity` | string | no | Scheduling hint; defaults to `medium`. Prefer a consistent vocabulary such as `low`, `medium`, `high`. |

`description_markdown` should cover the trigger scene, background fusion, rough beginning/development/turn/payoff, recovery or lasting consequences, reward or cost, and constraints that preserve player choice. Use Lore facts only after reading them. Do not encode a mandatory player decision or fixed future chapter.

Do not send host-owned `version`, `path`, ownership/validation fields, or timestamps when constructing the value.

## Create example

```text
config_apply({
  "operation": "create",
  "resource": "event_package",
  "scope": "user",
  "value": {
    "id": "investigation-pressure",
    "name": "Investigation pressure",
    "description": "Reusable complications that advance clues without forcing one solution.",
    "events": [
      {
        "id": "witness-withdraws",
        "type_name": "Witness withdraws",
        "enabled": true,
        "category": "social-pressure",
        "tags": ["witness", "clue", "relationship"],
        "intensity": "medium",
        "description_markdown": "## Trigger scene\nA previously cooperative witness notices a credible danger and begins to retreat.\n\n## Background fusion\nUse an existing threat, faction, obligation, or relationship actually present in Lore or current state.\n\n## Event logic\nShow the warning sign, let the player respond, reveal what pressure is operating, then convert the response into a changed relationship or clue route.\n\n## Recovery / consequences\nThe witness may require protection, distance themselves, provide incomplete information, or redirect the player.\n\n## Reward / cost\nSuccess preserves trust or gains a sharper clue; failure costs time, exposure, or relationship strength but does not erase all leads.\n\n## Choice constraints\nDo not force intimidation, payment, or combat. Accept any credible protective, social, investigative, or indirect plan."
      },
      {
        "id": "evidence-contaminated",
        "type_name": "Evidence becomes unreliable",
        "enabled": true,
        "category": "clue-pressure",
        "tags": ["evidence", "time-pressure"],
        "intensity": "high",
        "description_markdown": "## Trigger scene\nA known clue source is disturbed before it can be fully secured.\n\n## Background fusion\nChoose a location, object, or record already established in the current investigation.\n\n## Event logic\nReveal the disturbance, preserve at least one usable trace, introduce competing interpretations, and open a route to corroboration.\n\n## Recovery / consequences\nThe player can reconstruct, cross-check, pursue the contaminator, or accept uncertainty.\n\n## Reward / cost\nCareful action yields provenance or a new suspect; delay increases exposure or false confidence.\n\n## Choice constraints\nNever invalidate all prior progress and never declare one interpretation mandatory."
      }
    ]
  }
})
```

## Complete update example

To disable one card, first `get` the package, retain every other card and every card field, change only that card's `enabled`, and submit the complete package:

```text
config_apply({
  "operation": "update",
  "resource": "event_package",
  "scope": "user",
  "id": "investigation-pressure",
  "revision": "REVISION_FROM_GET",
  "value": {
    "id": "investigation-pressure",
    "name": "Investigation pressure",
    "description": "Reusable complications that advance clues without forcing one solution.",
    "events": [
      {
        "id": "witness-withdraws",
        "type_name": "Witness withdraws",
        "enabled": false,
        "category": "social-pressure",
        "tags": ["witness", "clue", "relationship"],
        "intensity": "medium",
        "description_markdown": "COMPLETE_EXISTING_MARKDOWN_FROM_GET"
      },
      {
        "id": "evidence-contaminated",
        "type_name": "Evidence becomes unreliable",
        "enabled": true,
        "category": "clue-pressure",
        "tags": ["evidence", "time-pressure"],
        "intensity": "high",
        "description_markdown": "COMPLETE_EXISTING_MARKDOWN_FROM_GET"
      }
    ]
  }
})
```

The `COMPLETE_EXISTING_MARKDOWN_FROM_GET` markers mean copy the actual unchanged text; never write the marker. Verify card count, stable IDs, the requested enabled state, and a changed revision. Attaching the package is a separate complete `story_director` update.
