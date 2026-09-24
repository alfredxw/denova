import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ConversationConfigController } from '@/features/conversation-config/types'
import { AgentApprovalModeMenu } from './AgentApprovalModeMenu'

describe('AgentApprovalModeMenu', () => {
  for (const active of [false, true]) {
    it(`shows Codex permissions and ${active ? 'locks the running selection' : 'saves the complete engine selection'}`, async () => {
      const user = userEvent.setup()
      const patch = vi.fn().mockResolvedValue(true)
      const controller: ConversationConfigController = {
        snapshot: { agent_kind: 'ide', profile_id: 'unused', thinking_level: 'medium', approval_mode: 'ask', revision: 2,
          runtime: { kind: 'codex', codex: { model: 'test-model', effort: 'high' } } },
        initialized: true, loading: false, saving: false, error: null, patch, reload: vi.fn(),
      }
      render(<AgentApprovalModeMenu runActive={active} conversationConfig={controller} />)
      await user.click(screen.getByRole('button', { name: /工作区写入/ }))
      const readOnly = screen.getByRole('menuitem', { name: /^只读/ })
      if (active) {
        expect(readOnly).toHaveAttribute('data-disabled')
        expect(screen.getByText('请先停止当前运行，再修改权限。')).toBeVisible()
      } else {
        await user.click(readOnly)
        expect(patch).toHaveBeenCalledWith({ codex: { model: 'test-model', effort: 'high', sandbox: 'read-only' } })
      }
    })
  }
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
