# Style reference (`style_reference`)

Shared Markdown prose references are available to Writing and Game modes. They are stored in `user` scope and addressed by the exact `display_path` returned by `config_read`, normally `.denova/styles/<file>.md`.

## Mutation semantics

- Create writes one new file and fails if its normalized filename already exists.
- Update replaces the complete Markdown `content`; it cannot rename the file or separately edit its catalog metadata.
- Delete removes the file and requires explicit user intent.
- Update and delete require the latest document `revision` returned by `get`.

## Field reference

### Create value

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `name` | string | yes | Human-readable reference name and fallback heading. |
| `description` | string | no | One-line catalog summary, at most 240 characters; oversized input is rejected before writing. |
| `filename` | string | no | Requested safe filename. Omit to derive it from `name`; use a Markdown filename when choosing one. |
| `content` | string | yes | Complete non-empty Markdown, at most 160 KiB. Oversized input is rejected before writing; the backend ensures a heading and trailing newline. |

### Update value

`content` is the only accepted field. It is a complete replacement, must remain non-empty, and is rejected without changing the file when it exceeds 160 KiB.

### Returned, host-owned fields

`reference.path`, `reference.display_path`, `reference.size`, `reference.updated_at`, `reference.missing`, `reference.error`, and `revision` are inspection data. Use `display_path` as the exact ID and copy `revision` only into the top-level mutation argument.

## Create example

```text
config_apply({
  "operation": "create",
  "resource": "style_reference",
  "scope": "user",
  "value": {
    "name": "Tense close third person",
    "description": "Short, sensory paragraphs for dangerous scenes.",
    "filename": "tense-close-third.md",
    "content": "# Tense close third person\n\nUse concrete sensory details, compressed sentences under pressure, and only information the viewpoint character can perceive."
  }
})
```

The receipt ID may be `.denova/styles/tense-close-third.md`. Read that exact ID before attaching it elsewhere:

```text
config_read({
  "operation": "get",
  "resource": "style_reference",
  "scope": "user",
  "ids": [".denova/styles/tense-close-third.md"]
})
```

## Update example

After `get`, replace the complete content with the requested edit:

```text
config_apply({
  "operation": "update",
  "resource": "style_reference",
  "scope": "user",
  "id": ".denova/styles/tense-close-third.md",
  "revision": "REVISION_FROM_GET",
  "value": {
    "content": "# Tense close third person\n\nUse concrete sensory details and compressed sentences under pressure. Keep every observation inside the viewpoint character's knowledge.\n"
  }
})
```

Read the document again and verify both the new content and a changed revision.

## Content boundaries

Prefer distilled, reusable sample paragraphs and concise style guidance. Do not preserve long copyrighted passages, source/work names, temporary story facts, tool rules, model settings, or secrets. When attaching a reference to a narrative style, use the returned `display_path` in `style_refs` or a scene-specific `style_rules[].style_refs` entry.
