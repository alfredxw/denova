---
name: agent-config
description: Use when config_manager creates or updates Nova Agent page settings through agent config tools.
agent: config_manager
---

# Agent Config

Use this skill before calling `write_agent_configs`.

## Workflow

1. Call `list_agent_configs` first. It returns user, workspace, and effective Agent page settings in one response.
2. Use `write_agent_configs` only for Agent page settings. Do not edit `config.toml` files directly.
3. Always set `scope` explicitly to `user` or `workspace`. Use the scope requested by the user or shown by the current Agents page context.
4. Preserve fields not requested by the user. For updates, copy the existing override from `list_agent_configs`, change only the requested fields, then send the complete replacement override.
5. Delete or disable SubAgents only after the user clearly asked for it.

## Supported Operations

- `set_agent_override`: replace one Agent override in the selected layer.
  - `agent`: `default`, `ide`, `interactive_story`, `config_manager`, `interactive_state`, `interactive_hot_choices`, `version_summary`, `tool_agent`, `automation`, or `context_compaction`.
  - Optional replacement sections: `model`, `tools`, `prompt`, `skills`, `context`.
- `set_general_sub_agent`: set or inherit the built-in General SubAgent switch.
  - `agent`: `default`, `ide`, `interactive_story`, `config_manager`, or `automation`.
  - `enabled`: `true` or `false`; omit/null to inherit.
- `upsert_sub_agent`: create or replace one custom SubAgent in the selected layer.
  - Provide a complete `sub_agent` with `id`, `description`, and `system_prompt`.
  - Use `enabled: false` with the same `id` to shadow an inherited SubAgent.
- `delete_sub_agent`: remove one custom SubAgent from the selected layer only.
  - This does not delete SubAgents inherited from another layer.

## Field Notes

- `agent_config_read` and `agent_config_write` are only meaningful for the Config Manager Agent and its configured SubAgents.
- A SubAgent cannot gain tools disabled on its parent Agent. Tool settings are upper bounds, not grants beyond the parent.
- `model.profile_id` must reference an existing model profile or be empty to inherit. This skill does not create model profiles.
- `prompt.flow_prompt` changes Nova flow rules for an Agent. `prompt.system_prompt` adds user custom rules. Neither can override runtime contracts, output protocols, tool permissions, or backend validation.
- Context compaction fields are ratios, not percentages: use `0.9` for 90%.

## Safe Defaults

- Use `workspace` scope for book-specific Agent behavior and `user` scope for personal defaults across books.
- Prefer disabling one tool or one Skill over rewriting a full Agent prompt when the user asks for a capability boundary.
- Prefer a focused custom SubAgent with narrow tools over broad changes to the parent Agent.
