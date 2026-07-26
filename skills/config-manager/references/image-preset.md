# Image preset (`image_preset`)

Image presets are shared by Writing and Game modes. They define reusable visual policy injected into the Image Agent and image-generation request; they do not select an API profile or model.

This resource uses `user` scope and complete editable-resource replacement. Update/delete require the latest `updated_at` returned by `get` as `revision`.

## Field reference

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `id` | string | create: recommended | Stable letters/digits/`_`/`-` ID. |
| `name` | string | yes | User-visible preset name. |
| `description` | string | no | Intended visual use. |
| `slots` | object[] | yes | Ordered injection rules; at least one must be enabled with non-empty content. |
| `prompt` | string | compatibility only | Derived from enabled `tool_request` slots. If no slots are supplied, a non-empty legacy prompt becomes one default `tool_request` slot. Prefer explicit slots. |

Each slot contains:

| Field | Type / values | Meaning |
| --- | --- | --- |
| `id` | stable string | Letters/digits/`_`/`-`, unique in the preset. |
| `name` | string | User-visible label; defaults to `id`. |
| `target` | `agent_system` or `tool_request` | Stable Image Agent policy or constraints appended to the generation request. |
| `enabled` | boolean | Whether the slot contributes. |
| `content` | string | Reusable visual instruction, at most 4,000 characters per slot. |

Keep slots about medium, composition, lighting, color, texture, mood, and negative visual constraints. Do not include story facts, chapter prose, temporary state, API keys, model selection, tool rules, text/logo requests, watermarks, or future-story spoilers.

Do not send host-owned version, path, ownership/validation fields, or timestamps.

## Create example

```text
config_apply({
  "operation": "create",
  "resource": "image_preset",
  "scope": "user",
  "value": {
    "id": "ink-noir",
    "name": "Ink noir",
    "description": "High-contrast monochrome illustrations with restrained red accents.",
    "slots": [
      {
        "id": "visual-identity",
        "name": "Visual identity",
        "target": "agent_system",
        "enabled": true,
        "content": "Design scenes as cinematic ink illustrations: strong silhouettes, readable spatial staging, controlled negative space, and one restrained dark-red accent."
      },
      {
        "id": "request-constraints",
        "name": "Generation constraints",
        "target": "tool_request",
        "enabled": true,
        "content": "Monochrome ink wash, high contrast, subtle paper texture, dramatic practical lighting. No text, subtitles, logos, signatures, watermarks, UI, or spoilers beyond supplied scene facts."
      }
    ]
  }
})
```

## Complete update example

Read the exact preset and preserve every unrequested slot. This example changes one slot while retaining the other:

```text
config_apply({
  "operation": "update",
  "resource": "image_preset",
  "scope": "user",
  "id": "ink-noir",
  "revision": "REVISION_FROM_GET",
  "value": {
    "id": "ink-noir",
    "name": "Ink noir",
    "description": "High-contrast monochrome illustrations with restrained red accents.",
    "slots": [
      {
        "id": "visual-identity",
        "name": "Visual identity",
        "target": "agent_system",
        "enabled": true,
        "content": "Design scenes as cinematic ink illustrations: strong silhouettes, readable spatial staging, controlled negative space, and one restrained dark-red accent."
      },
      {
        "id": "request-constraints",
        "name": "Generation constraints",
        "target": "tool_request",
        "enabled": true,
        "content": "Monochrome ink wash, high contrast, restrained grain, natural lens perspective, dramatic practical lighting. No text, subtitles, logos, signatures, watermarks, UI, or spoilers beyond supplied scene facts."
      }
    ]
  }
})
```

Read it again and verify slot IDs, order, targets, enabled states, content, and revision. Updating a built-in ID creates an override; deleting that override explicitly restores the built-in preset.
