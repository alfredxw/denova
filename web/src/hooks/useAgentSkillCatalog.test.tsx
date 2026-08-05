import { act, renderHook, waitFor } from '@testing-library/react'
import { StrictMode, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchProjectSettings } from '@/features/settings/api'
import type { LayeredSettings, ResolvedAgentToolCapability } from '@/features/settings/types'
import { getSkills } from '@/lib/api'
import { queryClient } from '@/lib/query-client'
import { useSkillCommands } from './useSkillCommands'
import { useWritingSkillOptions } from './useWritingSkillOptions'

vi.mock('@/features/settings/api', () => ({ fetchProjectSettings: vi.fn() }))
vi.mock('@/lib/api', () => ({
  getSkills: vi.fn(),
  projectSkillTarget: (projectId: string) => ({ kind: 'project', projectId }),
}))

const PROJECT_ID = 'project-book'

describe('useAgentSkillCatalog', () => {
  beforeEach(() => {
    queryClient.clear()
    vi.mocked(getSkills).mockReset().mockResolvedValue({
      scopes: [],
      skills: [{
        name: 'novel-lite', description: 'Write', agent: 'ide', scope: 'builtin', path: '/skills/novel-lite',
        editable: false, active: true, capabilities: ['writing-workflow'],
      }],
    })
    vi.mocked(fetchProjectSettings).mockReset().mockResolvedValue(settingsWithSkills())
  })

  it('shares one request and one invalidation lane across mounted conversation consumers', async () => {
    const { result } = renderHook(() => ({
      commands: useSkillCommands({ agentKey: 'ide', projectId: PROJECT_ID }),
      writing: useWritingSkillOptions(PROJECT_ID),
      secondConversationCommands: useSkillCommands({ agentKey: 'ide', projectId: PROJECT_ID }),
      secondConversationWriting: useWritingSkillOptions(PROJECT_ID),
    }), { wrapper: strictWrapper })

    await waitFor(() => expect(result.current).toEqual({
      commands: [{ name: 'novel-lite', description: 'Write' }],
      writing: [expect.objectContaining({ name: 'novel-lite' })],
      secondConversationCommands: [{ name: 'novel-lite', description: 'Write' }],
      secondConversationWriting: [expect.objectContaining({ name: 'novel-lite' })],
    }))
    expect(getSkills).toHaveBeenCalledOnce()
    expect(fetchProjectSettings).toHaveBeenCalledOnce()
    expect(fetchProjectSettings).toHaveBeenCalledWith(PROJECT_ID)

    act(() => window.dispatchEvent(new CustomEvent('nova:conversation-config-updated')))
    expect(getSkills).toHaveBeenCalledOnce()
    expect(fetchProjectSettings).toHaveBeenCalledOnce()

    act(() => window.dispatchEvent(new CustomEvent('nova:skills-updated')))
    await waitFor(() => expect(getSkills).toHaveBeenCalledTimes(2))
    expect(fetchProjectSettings).toHaveBeenCalledOnce()

    act(() => window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { projectId: PROJECT_ID } })))
    await waitFor(() => expect(fetchProjectSettings).toHaveBeenCalledTimes(2))
    expect(getSkills).toHaveBeenCalledTimes(2)
  })

  it('does not load a catalog while its owning surface is disabled', async () => {
    const { result } = renderHook(() => ({
      commands: useSkillCommands({ agentKey: 'ide', projectId: '', enabled: false }),
      writing: useWritingSkillOptions('', false),
    }), { wrapper: strictWrapper })

    expect(result.current).toEqual({ commands: [], writing: [] })
    await act(async () => { await Promise.resolve() })
    expect(getSkills).not.toHaveBeenCalled()
    expect(fetchProjectSettings).not.toHaveBeenCalled()
  })
})

function strictWrapper({ children }: { children: ReactNode }) {
  return <StrictMode>{children}</StrictMode>
}

function settingsWithSkills(): LayeredSettings {
  return {
    default: {}, global: {}, user: {}, workspace: {}, effective: {},
    paths: { denova_dir: '', nova_dir: '', user_config: '', workspace_config: '' },
    resolved_agent_tool_manifests: { ide: [skillsCapability()] },
    resolved_agent_contexts: {},
  }
}

function skillsCapability(): ResolvedAgentToolCapability {
  return {
    capability: 'skills', title_key: 'agents.tool.skills.title', description_key: 'agents.tool.skills.subtitle',
    tool_names: ['skill'], descriptor: {
      execution: 'parallel', mutation_scope: 'none', post_check: 'none', recovery: 'retry',
      result_projection: 'summary', result_retention: 'receipt', steering: 'interruptible',
    },
    available_to_subagents: true, allowed: true, availability: 'available',
  }
}
