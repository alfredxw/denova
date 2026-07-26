# State System (`state_system`)

State Systems define reusable Game-mode Actor schemas, initial Actors, and trait pools. Existing stories freeze their own schema snapshot; changing a reusable module affects newly initialized stories, not historical state.

## Shape

- `id`, `name`, `description`
- `actor_state.templates`: stable ASCII template `id`, visible `name`, description, `fields`, and optional `trait_rules`
- `actor_state.initial_actors`: initial Actor definitions using a template and field overrides
- `actor_state.trait_pools`: stable pools of weighted trait definitions

Field `type` is one of `number`, `string`, `bool`, `enum`, `object`, or `list`. A field uses its stable `name` as the model-visible field identity; set a correctly typed `default`, numeric `min`/`max`, enum `options`, a precise description, and an update instruction. Optional `group` and `display` (`stat`, `inline`, `block`, `list`) are presentation hints only.

Trait rules reference `pool_id` and a positive `draw_count`. Trait definitions contain stable `id`, `name`, short `summary`, and positive `weight`. Traits are assigned as snapshots; normal numeric or object effects remain typed Actor state changes.

Read the complete current module and any Rule System that binds to it before changing identities or types. Preserve unrequested templates, fields, pools, and initial Actors. Apply a complete-resource update with the latest `updated_at` revision.
