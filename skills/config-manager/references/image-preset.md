# Image preset (`image_preset`)

Image presets are shared by Writing and Game modes and use user scope.

## Editable shape

- `id`, `name`, `description`
- `prompt`: concise reusable visual constraints
- optional `slots`: stable `id`, `name`, existing `target`, `enabled`, and `content`

Keep prompts about medium, composition, lighting, color, texture, mood, and negative visual constraints such as no text, logo, watermark, or future-story spoilers. Do not put story facts, chapter prose, temporary state, API keys, model selection, or tool rules into a preset.

Call `get` before update and preserve unrequested slots and metadata. Create/update write the complete preset; update/delete require the latest `updated_at` revision. A built-in delete/override action must match the user's explicit intent.
