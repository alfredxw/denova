import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { OnboardingGuide } from './OnboardingGuide'
import { ONBOARDING_OPEN_EVENT } from './events'
import { ONBOARDING_STORAGE_KEY } from './state'
import { fetchSettings } from '@/features/settings/api'

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(),
}))

describe('OnboardingGuide', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.mocked(fetchSettings).mockReset()
  })

  it('auto-opens at language model setup when no usable model is configured', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(layeredSettings({}))

    renderGuide()

    expect(await screen.findByText('先配置语言模型')).toBeInTheDocument()
    expect(screen.getByText('写作和对话需要可用的语言模型。填写 API Key 和模型名并保存后，引导会自动进入下一步。')).toBeInTheDocument()
  })

  it('persists skip locally and closes the guide', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(layeredSettings({}))

    renderGuide()

    expect(await screen.findByText('先配置语言模型')).toBeInTheDocument()
    const skipButton = screen.getAllByRole('button', { name: '跳过引导' }).at(-1)
    expect(skipButton).toBeTruthy()
    fireEvent.click(skipButton!)

    expect(screen.queryByText('先配置语言模型')).not.toBeInTheDocument()
    expect(window.localStorage.getItem(ONBOARDING_STORAGE_KEY)).toContain('"skipped":true')
  })

  it('prefills the first chapter prompt only after the user clicks the Agent action', async () => {
    const onNavigate = vi.fn()
    vi.mocked(fetchSettings).mockResolvedValue(layeredSettings({
      model_profiles: [{ id: 'default', openai_api_key: 'key', openai_model: 'gpt-4.1' }],
    }))

    renderGuide({
      workspace: '/tmp/book',
      booksCount: 1,
      currentBookName: '测试书',
      onNavigate,
    })

    await waitFor(() => expect(fetchSettings).toHaveBeenCalled())
    window.dispatchEvent(new CustomEvent(ONBOARDING_OPEN_EVENT))

    expect(await screen.findByText('用创作 Agent 写第一章开头')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '填入示例' }))

    expect(onNavigate).toHaveBeenCalledTimes(1)
    expect(onNavigate.mock.calls[0][0]).toBe('writing-agent')
    expect(onNavigate.mock.calls[0][1]).toContain('《测试书》')
  })
})

function renderGuide(overrides: Partial<ComponentProps<typeof OnboardingGuide>> = {}) {
  const props: ComponentProps<typeof OnboardingGuide> = {
    mode: 'ide',
    rightPanel: 'ai',
    settingsOpen: false,
    workspace: '',
    booksCount: 0,
    currentBookName: '未命名',
    messages: [],
    isStreaming: false,
    onNavigate: vi.fn(),
    ...overrides,
  }
  return render(<OnboardingGuide {...props} />)
}

function layeredSettings(effective: Record<string, unknown>) {
  return {
    default: {},
    global: {},
    user: {},
    workspace: {},
    effective,
    paths: {},
    runtime: {},
    revisions: {},
  } as never
}
