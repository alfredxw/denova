# Agent profile (`agent_profile`)

Agent profiles manage layered model, capability, prompt, Skill, context, General SubAgent, and custom SubAgent settings in `user` or `workspace` scope.

This resource is a singleton registry snapshot. Call `config_read(operation=get, resource=agent_profile, ids=["registry"])` before a mutation; reads do not accept `scope`. The snapshot lists valid mutation IDs, parent kinds, capability keys, effective values, layer overrides, and both scope revisions. Use the revision for the exact target scope selected by `config_apply`.

## Operations

Set `value.kind` to:

- `agent` (default for create/update): `id` is an Agent kind; value may include `model`, `tools`, `prompt`, `skills`, or `context`. Model selection is user-scoped only.
- `general_sub_agent`: `id` is a supported parent Agent and `enabled` is true, false, or omitted/null to inherit.
- `sub_agent`: create/update with a complete `sub_agent` containing stable `id`, `description`, and `system_prompt`; delete by exact ID. This is the only kind that supports create.

Every delete must include `value.kind` with exactly `agent`, `general_sub_agent`, or `sub_agent`; delete never infers the kind from `id`. This prevents overlapping Agent and General SubAgent IDs from being routed to the wrong configuration section.

Use `workspace` for book-specific behavior and `user` for personal defaults. Capability settings are upper bounds: a SubAgent cannot gain a capability disabled on its parent. Prefer a focused capability/Skill override or narrow SubAgent over rewriting a broad system prompt. Custom prompts cannot override runtime contracts, output protocols, permissions, or backend validation.

Preserve every unrequested section in the selected layer. Delete removes that layer's override; it does not necessarily remove an inherited effective value. Update/delete and SubAgent create require the latest revision for the exact target scope returned by `config_read`.
