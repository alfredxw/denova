# Narrative style (`narrative_style`)

Narrative styles are shared by Writing and Game modes. They contain prose policy, prompt slots, and references to shared style-reference files.

## Editable shape

- `id`, `name`, `description`
- `style_refs`: global style-reference display paths
- `style_rules`: scene-specific objects with `scene` and `style_refs`; preserve legacy `style_contents` but do not add new inline content
- `context_policy`: short `creator`, `lore`, and `runtime_state` policies
- `slots`: stable `id`, user-visible `name`, existing `target`, `enabled`, and focused `content`

Call `get` and use the returned object as the update base. Preserve slot IDs, unknown-to-the-user policy choices, and server metadata. Keep slots about narrative behavior only; do not put story facts, chapter prose, temporary state, tool permissions, or credentials in them.

Create/update are complete-resource writes. Built-in IDs may become user overrides; delete is an explicit restore/delete action and still requires the current `updated_at` revision.
