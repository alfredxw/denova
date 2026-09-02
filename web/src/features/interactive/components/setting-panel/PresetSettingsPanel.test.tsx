import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { setConfiguredLocale } from '@/i18n'
import { APIError } from '@/lib/api-client'
import type { GamePlanningTemplate } from '../../types'
import { PresetSettingsPanel } from './PresetSettingsPanel'

const apiMocks = vi.hoisted(() => ({
  createInteractiveTeller: vi.fn(),
  deleteGamePlanningTemplate: vi.fn(),
  getActorStates: vi.fn(),
  getEventPackages: vi.fn(),
  getGamePlanningTemplates: vi.fn(),
  getImagePresets: vi.fn(),
  getInteractiveTellers: vi.fn(),
  getRuleSystems: vi.fn(),
  getStyleReferences: vi.fn(),
  updateGamePlanningTemplate: vi.fn(),
}))
const toastMock = vi.hoisted(() => ({ dismiss: vi.fn(), error: vi.fn(), success: vi.fn() }))

vi.mock('sonner', () => ({ toast: toastMock }))
vi.mock('../../api', async importOriginal => ({
  ...await importOriginal<typeof import('../../api')>(),
  ...apiMocks,
}))

describe('PresetSettingsPanel error feedback', () => {
  beforeEach(() => {
    setConfiguredLocale('zh-CN')
    toastMock.error.mockReset()
    for (const load of [
      apiMocks.getActorStates,
      apiMocks.getEventPackages,
      apiMocks.getGamePlanningTemplates,
      apiMocks.getImagePresets,
      apiMocks.getInteractiveTellers,
      apiMocks.getRuleSystems,
      apiMocks.getStyleReferences,
    ]) {
      load.mockReset()
      load.mockResolvedValue([])
    }
    apiMocks.createInteractiveTeller.mockReset()
    apiMocks.deleteGamePlanningTemplate.mockReset()
    apiMocks.deleteGamePlanningTemplate.mockResolvedValue(undefined)
    apiMocks.updateGamePlanningTemplate.mockReset()
  })

  it.each([
    ['zh-CN', '新建叙事风格', '创建方案预设失败 · 日志 ID: request-123'],
    ['en-US', 'New Narrative Style', 'Failed to create preset · Log ID: request-123'],
  ])('keeps a raw backend error out of the %s create failure', async (locale, createLabel, expected) => {
    const user = userEvent.setup()
    setConfiguredLocale(locale)
    apiMocks.createInteractiveTeller.mockRejectedValue(new APIError('导演 ID 已存在', {
      status: 400,
      requestID: 'request-123',
    }))

    render(
      <TooltipProvider>
        <PresetSettingsPanel projectId="project-1" embedded />
      </TooltipProvider>,
    )

    await user.click(await screen.findByRole('button', { name: createLabel }))

    await waitFor(() => expect(toastMock.error).toHaveBeenCalledWith(expected))
    expect(toastMock.error.mock.calls.flat().join(' ')).not.toContain('导演 ID 已存在')
  })

  it('autosaves a built-in planning override and restores it on the same ID', async () => {
    const user = userEvent.setup()
    const builtin: GamePlanningTemplate = {
      version: 1,
      id: 'default',
      name: 'Classic adventure',
      description: 'Built-in description',
      sections: [{ id: 'long-term-direction', title: 'Long-term direction', description: 'Built-in guidance' }],
      custom: false,
      revision: 'revision-1',
    }
    apiMocks.getGamePlanningTemplates.mockResolvedValue([builtin])
    apiMocks.updateGamePlanningTemplate.mockImplementation(async (_id: string, input: Partial<GamePlanningTemplate>) => ({
      ...builtin,
      ...input,
      builtin_overridden: true,
      revision: 'revision-2',
    }))

    render(
      <TooltipProvider>
        <PresetSettingsPanel projectId="project-1" embedded />
      </TooltipProvider>,
    )

    await user.click(await screen.findByRole('button', { name: '展开游戏规划' }))
    await user.click(await screen.findByRole('button', { name: /经典冒险/ }))
    const editor = await screen.findByTestId('game-planning-editor')

    await user.type(within(editor).getByLabelText('名称'), '·调整')
    await waitFor(() => expect(apiMocks.updateGamePlanningTemplate).toHaveBeenCalledWith(
      'default',
      expect.objectContaining({ name: '经典冒险·调整', builtin_overridden: true }),
      'revision-1',
    ), { timeout: 3500 })

    await user.click(screen.getByRole('button', { name: '恢复内置' }))
    await waitFor(() => expect(apiMocks.deleteGamePlanningTemplate).toHaveBeenCalledWith('default'))
    await waitFor(() => expect(within(editor).getByText('内置')).toBeInTheDocument())
  })
})
