# Agent profile (`agent_profile`)

Agent profiles manage layered fixed Agent, custom Agent, model, capability, prompt, Skill, context, General SubAgent, and custom SubAgent settings in `user` or `workspace` scope.

This resource is a singleton registry snapshot. Reads do not accept `scope`: use `config_read(operation=get, resource=agent_profile, ids=["registry"])`. The singleton snapshot is returned as `items[0]`; it contains valid Agent kinds, SubAgent parents, capability names, safe model-profile IDs, user/workspace/effective layers, and separate revisions for both scopes.

Use the revision for the exact target scope selected by `config_apply`. Model selection is user-scoped only. API keys and other secrets are never returned and cannot be changed through this resource.

## Kinds and operations

Set `value.kind` to one of:

| Kind | ID | Create | Update | Delete effect |
| --- | --- | --- | --- | --- |
| `agent` | `default` or an Agent kind from `snapshot.agents` | no | Replaces only supplied configuration sections in the selected layer | Clears all model/tool/prompt/Skill/context overrides for that Agent in the selected layer |
| `custom_agent` | Stable custom Agent ID | yes | Replaces the selected layer's complete sparse custom Agent entry | Removes that layer's entry; an inherited same-ID custom Agent may remain effective |
| `general_sub_agent` | `default` or a parent from `snapshot.subagent_parents` | no | Sets `enabled` true/false for the selected layer | Removes that layer's switch so it inherits |
| `sub_agent` | Stable custom SubAgent ID | yes | Replaces the selected layer's complete SubAgent entry | Removes that layer's entry; an inherited same-ID SubAgent may remain effective |

`kind` defaults to `agent` for non-delete mutations. Every delete must include `value.kind` with exactly `agent`, `custom_agent`, `general_sub_agent`, or `sub_agent`; delete never infers it from `id`. SubAgent create require the latest revision for the exact target scope, as do custom Agent create, every update, and every delete.

## Layering and update semantics

- `user` supplies personal defaults; `workspace` overrides book-specific behavior.
- Fixed Agent updates are sectional: omitted `model`, `tools`, `prompt`, `skills`, and `context` sections stay unchanged.
- Each supplied section replaces that complete section in the target layer. Start from `snapshot.layers.<scope>`, preserve existing explicit keys in that section, and then make the requested change.
- Missing map keys inherit from parent/default layers; explicit `false` disables a capability or Skill at that layer.
- Deleting an override changes the selected layer only. Always inspect `layers.effective` afterward.
- Capability settings are upper bounds. A SubAgent cannot gain a tool disabled on its parent, and fixed Agents cannot enable capabilities outside their registered ceilings.
- A custom Agent always inherits one immutable base kind from `snapshot.custom_agent_bases`. It may narrow or tune that base, but cannot replace its protected runtime protocol or gain tools outside the base ceiling.

## Custom Agent fields

Custom Agents are named user-owned instances of fixed runtime kinds. Denova ships no preset custom Agent; create one only when the user asks for a reusable configuration. Put the instance in `value.custom_agent`.

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | yes | Stable lowercase identifier normalized from letters/digits/`-`/`_`. Conversation and branch state persist this identity. |
| `name` | yes on create | User-visible name. |
| `description` | no | Short user-visible purpose. |
| `base_kind` | yes on create | One value from `snapshot.custom_agent_bases`; immutable after creation. |
| `enabled` | no | Defaults true. False archives the instance without changing existing history. |
| `model` | no | Same model fields as a fixed Agent; user scope only. |
| `tools` | no | Sparse capability booleans bounded by the base kind's ceiling. |
| `prompt` | no | Additional system and flow guidance; protected runtime prompts remain in place. |
| `skills` | no | Sparse Skill availability overrides. |
| `context` | no | Same context policy overrides as a fixed Agent. |
| `image_api_profile_id` | no | Image profile for an `image`-based custom Agent; user scope only. |

Create example:

```text
config_apply({
  "operation": "create",
  "resource": "agent_profile",
  "scope": "user",
  "id": "focused-editor",
  "revision": "USER_REVISION_FROM_REGISTRY_GET",
  "value": {
    "kind": "custom_agent",
    "custom_agent": {
      "id": "focused-editor",
      "name": "Focused editor",
      "description": "Edit prose while preserving voice and intent.",
      "base_kind": "ide",
      "prompt": {"system_prompt": "Prefer small, reviewable edits and explain any change in meaning."},
      "tools": {"web_search": false, "web_fetch": false}
    }
  }
})
```

For update, preserve unrequested fields from the exact selected layer entry in the registry snapshot and submit the complete sparse entry. Set `enabled` to false to archive an instance while keeping existing conversations runnable. Deleting the final effective definition makes histories that reference it unavailable, so delete only a layer override unless that is intentional. To remove only the workspace override, delete with `value: {"kind": "custom_agent"}`. Selecting a different custom Agent for an existing conversation or game branch is intentionally not an update operation: create a new conversation or branch so durable history keeps one stable runtime identity.

## Fixed Agent sections

### `model` (user scope only)

| Field | Type | Meaning |
| --- | --- | --- |
| `profile_id` | string | Exact profile ID from the safe snapshot; missing/unknown values resolve to `default`. |
| `temperature` | number or null | Optional Agent-specific override. |
| `thinking_level` | string | Unified thinking level: `default`, `off`, `low`, `medium`, `high`, `xhigh`, or `max`. `default` omits provider thinking parameters; model support for explicit levels is model-dependent. |

### `tools`

A map from exact `snapshot.tool_capabilities[].source` to boolean. Current capability vocabulary includes `filesystem_read`, `workspace_write`, `shell`, `web_search`, `web_fetch`, `browser`, `ask`, `todo`, `skills`, `delegation`, `config_read`, `config_apply`, `event_read`, `lore_read`, `lore_write`, and `image_generation`. Use the runtime snapshot as authority because the catalog may evolve.

### `prompt`

| Field | Meaning |
| --- | --- |
| `system_prompt` | Additional editable system-level behavior. It cannot override runtime contracts, permissions, tool schemas, or output protocols. |
| `flow_prompt` | Additional process guidance for the Agent's normal flow. |

Prefer focused tools/Skills/SubAgents over a broad prompt rewrite. Supplying an empty prompt object clears this layer's prompt section while preserving other sections.

### `skills`

A map from exact Skill name to boolean. `true` explicitly enables an otherwise available Skill, `false` disables it, and an omitted key inherits. This controls availability only; it does not create or edit the Skill document.

### `context`

| Field | Values / bounds |
| --- | --- |
| `compaction_enabled` | boolean |
| `compaction_threshold` | ratio clamped to 0.50–0.98 |
| `tool_result_context_enabled` | boolean; allows recoverable tool results to remain in model context until backend-managed cleanup |
| `max_fragment_bytes` | positive, max 16 MiB; default 256 KiB |
| `max_total_injected_bytes` | positive, max 64 MiB; default 4 MiB |
| `max_fragments` | positive, max 4096; default 256 |
| `max_metadata_field_bytes` | positive, max 64 KiB; default 4 KiB |
| `max_provider_input_bytes` | positive, max 64 MiB; default 4 MiB |

Compaction shape, recovery headroom, cleanup watermarks, protected recent tool
results, and failure handling are one backend-managed policy and are not
individually configurable. These fields control model-context intent and
injection boundaries; they never delete the canonical transcript.

## Fixed Agent update example

Assume the registry snapshot shows no existing workspace `ide` Skill overrides. Enable one Skill without changing model, tools, prompt, or context:

```text
config_apply({
  "operation": "update",
  "resource": "agent_profile",
  "scope": "workspace",
  "id": "ide",
  "revision": "WORKSPACE_REVISION_FROM_REGISTRY_GET",
  "value": {
    "kind": "agent",
    "skills": {"web-research": true}
  }
})
```

If the snapshot already contains workspace `agent_skills.ide` keys, include all of those explicit keys in `skills` and then add/change only the requested one.

## General SubAgent switch examples

Enable the built-in General SubAgent for the IDE Agent in workspace scope:

```text
config_apply({
  "operation": "update",
  "resource": "agent_profile",
  "scope": "workspace",
  "id": "ide",
  "revision": "WORKSPACE_REVISION_FROM_REGISTRY_GET",
  "value": {"kind": "general_sub_agent", "enabled": true}
})
```

To restore inheritance, explicitly delete that layer override:

```text
config_apply({
  "operation": "delete",
  "resource": "agent_profile",
  "scope": "workspace",
  "id": "ide",
  "revision": "WORKSPACE_REVISION_FROM_REGISTRY_GET",
  "value": {"kind": "general_sub_agent"}
})
```

## Custom SubAgent fields

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | yes | Stable lowercase identifier normalized from letters/digits/`-`/`_`; also used by delegation calls. |
| `name` | no | Display name; defaults to `id`. |
| `description` | yes | Trigger-oriented delegation description. |
| `system_prompt` | yes | Focused role, method, and completion boundary. |
| `enabled` | no | Defaults true. |
| `parents` | yes for use | Any subset returned by `snapshot.subagent_parents`. Empty means no parent can delegate to it. |
| `model` | no | Same fields as the Agent model section; inherits the parent when omitted. |
| `tools` | no | Sparse capability booleans; can only narrow the parent ceiling. |

### Create example

Read `registry`, select the workspace revision, and submit a complete new SubAgent:

```text
config_apply({
  "operation": "create",
  "resource": "agent_profile",
  "scope": "workspace",
  "id": "continuity-auditor",
  "revision": "WORKSPACE_REVISION_FROM_REGISTRY_GET",
  "value": {
    "kind": "sub_agent",
    "sub_agent": {
      "id": "continuity-auditor",
      "name": "Continuity auditor",
      "description": "Use for a bounded review of timeline, names, character knowledge, location, inventory, and unresolved setup.",
      "system_prompt": "Review only the supplied files and Lore. Separate confirmed contradictions from author-choice questions. Cite both conflicting sources for every confirmed issue and finish with a concise correction list.",
      "enabled": true,
      "parents": ["ide"],
      "tools": {
        "filesystem_read": true,
        "workspace_write": false,
        "shell": false,
        "web_search": false,
        "web_fetch": false,
        "browser": false,
        "delegation": false,
        "lore_read": true,
        "lore_write": false
      }
    }
  }
})
```

### Complete update and delete

For update, find the exact layer entry in the registry snapshot, preserve every unrequested SubAgent field and capability key, and submit a complete `sub_agent` object with the latest same-scope revision. To remove the workspace entry:

```text
config_apply({
  "operation": "delete",
  "resource": "agent_profile",
  "scope": "workspace",
  "id": "continuity-auditor",
  "revision": "WORKSPACE_REVISION_FROM_REGISTRY_GET",
  "value": {"kind": "sub_agent"}
})
```

## Verification

After mutation, read `registry` again and verify all three views:

1. the selected `layers.user` or `layers.workspace` section contains exactly the intended override;
2. unrelated sections and custom SubAgents in that layer remain unchanged; and
3. `layers.effective` plus `sub_agent_index` reflect the expected inherited result.

Never report success from the mutation receipt alone. A valid layer write may still be shadowed by workspace scope, constrained by a parent capability ceiling, or inactive because a custom SubAgent has no eligible parent.
