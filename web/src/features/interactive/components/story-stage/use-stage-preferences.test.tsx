import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStagePreferences } from './use-stage-preferences'

const settingsMock = vi.hoisted(() => ({
  fetchProjectSettings: vi.fn(),
  projectSettingsTarget: vi.fn((projectId: string) => {
    if (!projectId.trim()) throw new Error('Project ID is required')
    return { kind: 'project' as const, projectId }
  }),
  subscribeSettingsTarget: vi.fn(() => vi.fn()),
}))

vi.mock('@/features/settings/api', () => ({
  fetchProjectSettings: settingsMock.fetchProjectSettings,
}))

vi.mock('@/features/settings/query', () => ({
  projectSettingsTarget: settingsMock.projectSettingsTarget,
  subscribeSettingsTarget: settingsMock.subscribeSettingsTarget,
}))

describe('useStagePreferences', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps defaults while a workspace switch has no Project identity', () => {
    const { result } = renderHook(() => useStagePreferences(''))

    expect(result.current).toEqual({ lineHeight: 1.78 })
    expect(settingsMock.fetchProjectSettings).not.toHaveBeenCalled()
    expect(settingsMock.projectSettingsTarget).not.toHaveBeenCalled()
    expect(settingsMock.subscribeSettingsTarget).not.toHaveBeenCalled()
  })
})
