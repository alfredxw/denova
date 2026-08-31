# Agent profile (`agent_profile`)

Agent profiles manage layered fixed Agent, model, capability, prompt, Skill, context, General SubAgent, and custom SubAgent settings. They also manage complete user-owned custom Agent definitions in `user` scope.

This resource is a singleton registry snapshot. Reads do not accept `scope`: use `config_read(operation=get, resource=agent_profile, ids=["registry"])`. The singleton snapshot is returned as `items[0]`; it contains valid Agent kinds, SubAgent parents, capability names, safe model-profile IDs, user/workspace/effective layers, and separate revisions for both scopes.

Use the revision for the exact target scope selected by `config_apply`. Model selection is user-scoped only. API keys and other secrets are never returned and cannot be changed through this resource.

## Kinds and operations

Set `value.kind` to one of:

| Kind | ID | Create | Update | Delete effect |
| --- | --- | --- | --- | --- |
| `agent` | `default` or an Agent kind from `snapshot.agents` | no | Replaces only supplied configuration sections in the selected layer | Clears all model/tool/prompt/Skill/context overrides for that Agent in the selected layer |
| `custom_agent` | Stable custom Agent ID | yes, user scope only | Replaces the complete independent Agent definition | Deletes the reusable definition; existing conversations keep their captured snapshot |
| `general_sub_agent` | `default` or a parent from `snapshot.subagent_parents` | no | Sets `enabled` true/false for the selected layer | Removes that layer's switch so it inherits |
| `sub_agent` | Stable custom SubAgent ID | yes | Replaces the selected layer's complete SubAgent entry | Removes that layer's entry; an inherited same-ID SubAgent may remain effective |

`kind` defaults to `agent` for non-delete mutations. Every delete must include `value.kind` with exactly `agent`, `custom_agent`, `general_sub_agent`, or `sub_agent`; delete never infers it from `id`. SubAgent create requires the latest revision for the exact target scope. Custom Agent create, update, and delete always require the latest user revision; every other update and delete requires the revision for its selected scope.

## Layering and update semantics

- `user` supplies personal defaults; `workspace` overrides book-specific fixed-Agent and SubAgent behavior.
- Fixed Agent updates are sectional: omitted `model`, `tools`, `prompt`, `skills`, and `context` sections stay unchanged.
- Each supplied section replaces that complete section in the target layer. Start from `snapshot.layers.<scope>`, preserve existing explicit keys in that section, and then make the requested change.
- Missing map keys inherit from parent/default layers; explicit `false` disables a capability or Skill at that layer.
- Deleting an override changes the selected layer only. Always inspect `layers.effective` afterward.
- Capability settings are upper bounds. A SubAgent cannot gain a tool disabled on its parent, and fixed Agents cannot enable capabilities outside their registered ceilings.
- A custom Agent is a complete independent definition owned by the user. Its immutable `contract` selects only the stable runtime input/output boundary and capability ceiling from `snapshot.agent_contracts`; it is not a live inheritance link to a built-in Agent.

## Custom Agent fields

Custom Agents are named user-owned definitions inside stable runtime contracts. Creation clones a built-in definition as a starting point, but later built-in changes do not alter the custom Agent. Custom Agent mutations always use `scope=user`; put the complete definition in `value.custom_agent`.

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | yes | Stable lowercase identifier normalized from letters/digits/`-`/`_`. Conversation and branch state persist this identity. |
| `name` | yes on create | User-visible name. |
| `description` | no | Short user-visible purpose. |
| `contract` | yes on create | One ID from `snapshot.agent_contracts`; immutable after creation. It defines runtime input/output and the maximum capability set, not behavior. |
| `enabled` | no | Defaults true. False archives the instance without changing existing history. |
| `model` | no | Same model fields as a fixed Agent; user scope only. |
| `tools` | no | Capability booleans bounded by the contract ceiling. |
| `instructions` | no | Complete user-owned behavior and workflow instructions placed after protected runtime/output contracts. |
| `tool_guidance` | no | Map of enabled concrete tool names to extra English guidance. Canonical names, schemas, implementations, permissions, and recovery behavior stay locked. |
| `skill_policy` | no | `{mode, pinned, blocked}`. `managed` follows Skill audience metadata; `explicit` admits only pinned Skills; blocked always wins. |
| `runtime_context` | no | Same compaction and context budget policy fields as a fixed Agent. |
| `context_bindings` | no | User context fragments with stable IDs, `stable`/`session`/`turn` slots, purpose, content, and a byte hard limit. |
| `delegation` | no | `{mode, agent_ids}` where mode is `compatible`, `selected`, or `disabled`; selected children come from the shared SubAgent registry. |
| `image_api_profile_id` | no | Image profile for a custom Agent using the image contract. |

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
      "contract": "writing.primary.v1",
      "instructions": "Edit prose while preserving voice and intent. Prefer small, reviewable edits and explain any change in meaning.",
      "tools": {"filesystem_read": true, "workspace_write": true, "web_search": false, "web_fetch": false},
      "tool_guidance": {"edit": "Prefer the smallest replacement that preserves surrounding prose."},
      "skill_policy": {"mode": "managed", "pinned": ["continuity-review"], "blocked": []},
      "context_bindings": [{
        "id": "house-style",
        "name": "House style",
        "purpose": "apply the user's stable prose conventions",
        "slot": "stable",
        "content": "Use restrained prose and preserve point of view.",
        "hard_limit_bytes": 262144
      }],
      "delegation": {"mode": "compatible"}
    }
  }
})
```

For update, start from the exact user definition in the registry snapshot, preserve every unrequested field, and submit the complete Agent definition. Never mutate `contract`; create a new Agent if the runtime boundary must change. Set `enabled` to false to archive it. Conversations and game branches capture the complete definition when created, so later edits or deletion do not rewrite existing history. Selecting another Agent still creates a new conversation or branch so one durable history keeps one stable identity and revision.

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

Enable the built-in General SubAgent for the Writing Agent in workspace scope:

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
