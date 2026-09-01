import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { setConfiguredLocale } from '@/i18n'
import { APIError } from '@/lib/api-client'
import { PresetSettingsPanel } from './PresetSettingsPanel'

const apiMocks = vi.hoisted(() => ({
  createInteractiveTeller: vi.fn(),
  getActorStates: vi.fn(),
  getEventPackages: vi.fn(),
  getGamePlanningTemplates: vi.fn(),
  getImagePresets: vi.fn(),
  getInteractiveTellers: vi.fn(),
  getRuleSystems: vi.fn(),
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
    ]) {
      load.mockReset()
      load.mockResolvedValue([])
    }
    apiMocks.createInteractiveTeller.mockReset()
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
})
