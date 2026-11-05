import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStagePreferences } from './use-stage-preferences'

const settingsMock = vi.hoisted(() => ({
  fetchSettings: vi.fn(),
  fetchProjectSettings: vi.fn(),
  subscribeSettingsTarget: vi.fn(() => vi.fn()),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: settingsMock.fetchSettings,
  fetchProjectSettings: settingsMock.fetchProjectSettings,
}))

vi.mock('@/features/settings/query', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/features/settings/query')>(),
  subscribeSettingsTarget: settingsMock.subscribeSettingsTarget,
}))

describe('useStagePreferences', () => {
  beforeEach(() => {
    vi.resetAllMocks()
  })

  it('switches to global settings and subscriptions when a workspace has no Project identity', async () => {
    settingsMock.fetchProjectSettings.mockResolvedValue({
      effective: {
        interactive_stage_line_height: 2,
        interactive_stage_scrim_opacity: 0.6,
        interactive_stage_text_outline: true,
        interactive_stage_theme: 'dark',
        interactive_stage_text_max_width: 1000,
        interactive_stage_character_layout: 'sides',
      },
    })
    settingsMock.fetchSettings.mockResolvedValue({
      effective: {
        interactive_stage_line_height: 1.5,
        interactive_stage_scrim_opacity: 0.4,
        interactive_stage_text_max_width: 800,
        interactive_stage_character_layout: 'left',
      },
    })
    const unsubscribeProject = vi.fn()
    const unsubscribeGlobal = vi.fn()
    settingsMock.subscribeSettingsTarget
      .mockReturnValueOnce(unsubscribeProject)
      .mockReturnValueOnce(unsubscribeGlobal)

    const { result, rerender, unmount } = renderHook(({ projectId }) => useStagePreferences(projectId), {
      initialProps: { projectId: 'project-1' },
    })

    await waitFor(() => expect(result.current).toEqual({
      theme: 'dark', lineHeight: 2, scrimOpacity: 0.6, textOutline: true, textMaxWidth: 1000, characterLayout: 'sides', characterSize: 0.7,
    }))
    expect(settingsMock.fetchProjectSettings).toHaveBeenCalledExactlyOnceWith('project-1')
    expect(settingsMock.fetchSettings).not.toHaveBeenCalled()
    expect(settingsMock.subscribeSettingsTarget).toHaveBeenNthCalledWith(
      1, { kind: 'project', projectId: 'project-1' }, expect.any(Function),
    )

    rerender({ projectId: '' })

    await waitFor(() => expect(result.current).toEqual({
      theme: undefined, lineHeight: 1.5, scrimOpacity: 0.4, textOutline: false, textMaxWidth: 800, characterLayout: 'left', characterSize: 0.7,
    }))
    expect(settingsMock.fetchSettings).toHaveBeenCalledTimes(1)
    expect(settingsMock.fetchProjectSettings).toHaveBeenCalledTimes(1)
    expect(unsubscribeProject).toHaveBeenCalledTimes(1)
    expect(settingsMock.subscribeSettingsTarget).toHaveBeenNthCalledWith(
      2, { kind: 'global' }, expect.any(Function),
    )
    expect(unsubscribeGlobal).not.toHaveBeenCalled()

    unmount()
    expect(unsubscribeGlobal).toHaveBeenCalledTimes(1)
  })
})
