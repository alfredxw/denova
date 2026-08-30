import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ConversationConfigController } from '@/features/conversation-config/types'
import { ModelProfileSwitcher } from './ModelProfileSwitcher'

const settingsMocks = vi.hoisted(() => ({
  fetchSettings: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: settingsMocks.fetchSettings,
}))

vi.mock('@/features/settings/query', () => ({
  GLOBAL_SETTINGS_TARGET: 'global',
  subscribeSettingsTarget: () => () => {},
}))

describe('ModelProfileSwitcher', () => {
  it('saves a next-turn model selection while a model turn is active', async () => {
    settingsMocks.fetchSettings.mockResolvedValue({
      effective: { openai_model: 'test-model' },
    })
    const user = userEvent.setup()
    const patch = vi.fn().mockResolvedValue(true)
    const controller: ConversationConfigController = {
      snapshot: {
        agent_kind: 'ide',
        profile_id: 'default',
        thinking_level: 'medium',
        approval_mode: 'write',
        revision: 1,
      },
      initialized: true,
      loading: false,
      saving: false,
      error: null,
      patch,
      reload: vi.fn().mockResolvedValue(null),
    }

    render(
      <ModelProfileSwitcher
        agentKey="ide"
        conversationConfig={controller}
        runActive
      />,
    )

    await user.click(await screen.findByRole('button', { name: /切换模型/ }))
    expect(screen.getByRole('note')).toHaveTextContent('已开始的模型轮次保持当前配置；修改从下一模型轮次生效。')
    const highThinking = screen.getByRole('button', { name: '高' })
    expect(highThinking).toBeEnabled()

    await user.click(highThinking)
    expect(patch).toHaveBeenCalledWith({ thinking_level: 'high' })
  })
})
