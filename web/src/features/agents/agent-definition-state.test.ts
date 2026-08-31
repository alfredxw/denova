import { describe, expect, it } from 'vitest'
import type { AgentToolDescriptorSummary, LayeredSettings, Settings } from '@/features/settings/types'
import { cloneBuiltInAgent } from './agent-definition-state'

const descriptor: AgentToolDescriptorSummary = {
  source: 'builtin',
  execution: 'direct',
  mutation_scope: 'none',
  post_check: 'none',
  recovery: 'none',
  result_projection: 'inline',
  result_retention: 'ephemeral',
  steering: 'none',
  max_result_bytes: 1024,
  call_presentation: 'generic',
  result_presentation: 'generic',
}

describe('cloneBuiltInAgent', () => {
  it('captures a complete independent definition from the selected runtime', () => {
    const effective: Settings = {
      agent_models: { default: { thinking_level: 'medium' }, ide: { profile_id: 'writer' } },
      agent_prompts: { ide: { flow_prompt: 'Effective flow.', system_prompt: 'Effective rule.' } },
      agent_skills: { default: { shared: true }, ide: { blocked: false } },
    }
    const layered: LayeredSettings = {
      default: {},
      global: {},
      user: {},
      workspace: {},
      effective,
      paths: { denova_dir: '', nova_dir: '', user_config: '', workspace_config: '' },
      resolved_agent_tool_manifests: {
        ide: [{
          capability: 'filesystem_read',
          title_key: 'filesystem_read',
          description_key: 'filesystem_read',
          tool_names: ['read'],
          descriptor,
          tool_descriptors: {},
          available_to_subagents: true,
          allowed: true,
          availability: 'available',
        }],
      },
      resolved_agent_contexts: {
        ide: {
          compaction_enabled: true,
          compaction_threshold: 0.8,
          tool_result_context_enabled: true,
          max_fragment_bytes: 262144,
          max_total_injected_bytes: 4194304,
          max_fragments: 256,
          max_metadata_field_bytes: 4096,
          max_provider_input_bytes: 4194304,
        },
      },
    }

    const result = cloneBuiltInAgent({ id: 'editor', name: 'Editor', contract: 'writing.primary.v1' }, layered, effective)

    expect(result).toMatchObject({
      id: 'editor',
      instructions: 'Effective flow.\n\n## Additional Agent Rules\n\nEffective rule.',
      model: { profile_id: 'writer', thinking_level: 'medium' },
      tools: { filesystem_read: true },
      skill_policy: { mode: 'managed', pinned: ['shared'], blocked: ['blocked'] },
      runtime_context: { compaction_enabled: true, max_fragment_bytes: 262144 },
      context_bindings: [],
      delegation: { mode: 'compatible', agent_ids: [] },
    })
  })
})
