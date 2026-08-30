import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ConversationConfigController } from '@/features/conversation-config/types'
import { AgentApprovalModeMenu } from './AgentApprovalModeMenu'

describe('AgentApprovalModeMenu', () => {
  it('saves a next-turn safety mode while a model turn is active', async () => {
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

    render(<AgentApprovalModeMenu runActive conversationConfig={controller} />)

    await user.click(screen.getByRole('button', { name: /Agent 安全模式/ }))
    expect(screen.getByText('已开始的模型轮次保持当前安全模式；修改从下一模型轮次生效。')).toBeVisible()
    const fullAccess = screen.getByRole('menuitem', { name: /Full access/ })
    expect(fullAccess).toBeEnabled()

    await user.click(fullAccess)
    expect(patch).toHaveBeenCalledWith({ approval_mode: 'full_access' })
  })
})
