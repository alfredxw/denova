import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchProjectSettings } from '@/features/settings/api'
import type { LayeredSettings, ResolvedAgentToolCapability } from '@/features/settings/types'
import { getSkills } from '@/lib/api'
import { queryClient } from '@/lib/query-client'
import { useSkillCommands } from './useSkillCommands'

vi.mock('@/features/settings/api', () => ({ fetchProjectSettings: vi.fn() }))
vi.mock('@/lib/api', () => ({
  getSkills: vi.fn(),
  projectSkillTarget: (projectId: string) => ({ kind: 'project', projectId }),
}))

const PROJECT_ID = 'project-book'

describe('useSkillCommands', () => {
  beforeEach(() => {
    queryClient.clear()
    vi.mocked(fetchProjectSettings).mockReset()
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
    vi.mocked(fetchProjectSettings).mockResolvedValue(settingsWithManifest())

    const { result } = renderHook(() => useSkillCommands({ agentKey: 'ide', projectId: PROJECT_ID }))

    await waitFor(() => expect(fetchProjectSettings).toHaveBeenCalledWith(PROJECT_ID))
    expect(result.current).toEqual([])
  })

  it('hides commands when the backend manifest disables Skills', async () => {
    vi.mocked(fetchProjectSettings).mockResolvedValue(settingsWithManifest(skillsCapability(false)))

    const { result } = renderHook(() => useSkillCommands({ agentKey: 'ide', projectId: PROJECT_ID }))

    await waitFor(() => expect(fetchProjectSettings).toHaveBeenCalledWith(PROJECT_ID))
    expect(result.current).toEqual([])
  })

  it('returns active commands only when the backend manifest enables Skills', async () => {
    vi.mocked(fetchProjectSettings).mockResolvedValue(settingsWithManifest(skillsCapability(true)))

    const { result } = renderHook(() => useSkillCommands({ agentKey: 'ide', projectId: PROJECT_ID }))

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
  const descriptor: ResolvedAgentToolCapability['descriptor'] = {
    source: 'other',
    execution: 'parallel',
    mutation_scope: 'none',
    post_check: 'none',
    recovery: 'retry',
    result_projection: 'summary',
    result_retention: 'receipt',
    steering: 'interruptible',
    max_result_bytes: 128 << 10,
    call_presentation: 'generic',
    result_presentation: 'generic',
  }
  return {
    capability: 'skills',
    title_key: 'agents.tool.skills.title',
    description_key: 'agents.tool.skills.subtitle',
    tool_names: ['skill'],
    descriptor,
    tool_descriptors: { skill: descriptor },
    available_to_subagents: true,
    allowed,
    availability: 'available',
  }
}
