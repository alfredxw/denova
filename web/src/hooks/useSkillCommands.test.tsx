import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchSettings } from '@/features/settings/api'
import type { LayeredSettings, ResolvedAgentToolCapability } from '@/features/settings/types'
import { getSkills } from '@/lib/api'
import { useSkillCommands } from './useSkillCommands'

vi.mock('@/features/settings/api', () => ({ fetchSettings: vi.fn() }))
vi.mock('@/lib/api', () => ({ getSkills: vi.fn() }))

describe('useSkillCommands', () => {
  beforeEach(() => {
    vi.mocked(fetchSettings).mockReset()
    vi.mocked(getSkills).mockReset()
    vi.mocked(getSkills).mockResolvedValue({
      scopes: [],
      skills: [
        {
          name: 'novel-outline',
          description: 'Build an outline',
          agent: 'ide',
          scope: 'builtin',
          path: '/skills/novel-outline',
          editable: false,
          active: true,
        },
      ],
    })
  })

  it('fails closed when the selected agent manifest has no Skills capability', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsWithManifest())

    const { result } = renderHook(() => useSkillCommands({ agentKey: 'ide' }))

    await waitFor(() => expect(fetchSettings).toHaveBeenCalledOnce())
    expect(result.current).toEqual([])
  })

  it('hides commands when the backend manifest disables Skills', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsWithManifest(skillsCapability(false)))

    const { result } = renderHook(() => useSkillCommands({ agentKey: 'ide' }))

    await waitFor(() => expect(fetchSettings).toHaveBeenCalledOnce())
    expect(result.current).toEqual([])
  })

  it('returns active commands only when the backend manifest enables Skills', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsWithManifest(skillsCapability(true)))

    const { result } = renderHook(() => useSkillCommands({ agentKey: 'ide' }))

    await waitFor(() => expect(result.current).toEqual([
      { name: 'novel-outline', description: 'Build an outline' },
    ]))
  })
})

function settingsWithManifest(capability?: ResolvedAgentToolCapability): LayeredSettings {
  return {
    default: {},
    global: {},
    user: {},
    workspace: {},
    effective: {},
    paths: { denova_dir: '', nova_dir: '', user_config: '', workspace_config: '' },
    resolved_agent_tool_manifests: capability ? { ide: [capability] } : {},
    resolved_agent_contexts: {},
  }
}

function skillsCapability(allowed: boolean): ResolvedAgentToolCapability {
  return {
    capability: 'skills',
    title_key: 'agents.tool.skills.title',
    description_key: 'agents.tool.skills.subtitle',
    tool_names: ['skill'],
    descriptor: {
      execution: 'parallel',
      mutation_scope: 'none',
      post_check: 'none',
      recovery: 'retry',
      result_projection: 'summary',
      result_retention: 'receipt',
      steering: 'interruptible',
    },
    available_to_subagents: true,
    allowed,
    availability: 'available',
  }
}
