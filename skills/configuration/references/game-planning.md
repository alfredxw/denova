# Game Planning (`game_planning`)

Game Planning templates are reusable ordered outlines for the mutable future blueprint of a game-story branch. The resulting plan should behave like an adventure module or a long-form outline: distant direction stays concise, while the active arc and next candidate scenes carry enough detail to guide character entrances, exits, events, setups, and payoffs. Completed turns do not belong in the plan, and exact Actor State values or recommended action labels must not be duplicated there. A template owns only its name, description, and planning sections. Narrative style, event packages, rule system, state system, image preset, rule handling, and all story facts belong to each story and are not inherited from this resource.

This resource uses `user` scope and complete editable-resource replacement. Update/delete require the latest content-addressed `revision` returned by `get`; `updated_at` is display metadata and is not a concurrency token. Built-in templates are read-only. To customize one, read it and create a new custom template with a new ID.

## Field reference

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | create: recommended | Stable lowercase letters/digits/hyphen ID. It cannot change on update. |
| `name` | string | yes | User-visible template name, up to 256 bytes. |
| `description` | string | no | Purpose summary, up to 1024 bytes. |
| `sections` | array | yes | Ordered list of 1–64 planning sections. Order is meaningful. |

Each section has:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | recommended | Stable section identity. It is normalized or generated when omitted. |
| `title` | string | yes | Unique section heading, up to 256 bytes. |
| `description` | string | no | Guidance for the section body, up to 16 KiB. Markdown is allowed. |

The host renders sections as ordered ATX H2 Markdown headings. Section IDs never enter model context. Descriptions guide future planning but must not contain story-specific canon, promised future prose, completed-event summaries, exact state projections, recommended action labels, or module configuration.

## Create example

```text
config_apply({
  "operation": "create",
  "resource": "game_planning",
  "scope": "user",
  "value": {
    "id": "measured-mystery",
    "name": "Measured mystery",
    "description": "A fair-clue investigation outline.",
    "sections": [
      {"id": "truth", "title": "Underlying truth", "description": "Track the hidden truth, concealment, and stakes."},
      {"id": "evidence", "title": "Evidence map", "description": "Track discovered and undiscovered evidence plus alternative access paths."},
      {"id": "revelations", "title": "Revelation pacing", "description": "Prepare decisive conclusions with perceptible evidence."}
    ]
  }
})
```

For update, send the complete value with the latest revision and preserve every unchanged section in its intended order. Read the template again after mutation and verify the ordered section list.
