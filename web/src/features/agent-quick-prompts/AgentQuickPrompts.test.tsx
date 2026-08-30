import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AgentQuickPrompts } from './AgentQuickPrompts'

vi.mock('./use-agent-quick-prompts', () => ({
  useAgentQuickPrompts: () => ({
    customized: true,
    loading: false,
    prompts: [
      { id: 'fill', name: '填充操作', prompt: 'Fill this prompt', behavior: 'fill', enabled: true },
      { id: 'send', name: '发送操作', prompt: 'Send this prompt', behavior: 'send', enabled: true },
      { id: 'hidden', name: '隐藏操作', prompt: 'Hidden prompt', behavior: 'fill', enabled: false },
    ],
    save: vi.fn(),
  }),
}))

describe('AgentQuickPrompts', () => {
  it('fills or sends according to each prompt setting', async () => {
    const user = userEvent.setup()
    const onFill = vi.fn()
    const onSend = vi.fn()
    render(
      <AgentQuickPrompts
        scope="skills"
        writingTarget="current work"
        onFill={onFill}
        onSend={onSend}
      />,
    )

    await user.click(screen.getByRole('button', { name: /填充操作/ }))
    expect(onFill).toHaveBeenCalledWith('Fill this prompt')
    expect(onSend).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /发送操作/ }))
    expect(onSend).toHaveBeenCalledWith('Send this prompt')
    expect(screen.queryByText('隐藏操作')).not.toBeInTheDocument()
  })

  it('opens page-scoped settings from the header control', async () => {
    const user = userEvent.setup()
    render(
      <AgentQuickPrompts
        scope="skills"
        writingTarget="current work"
        onFill={vi.fn()}
        onSend={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '管理Skills快捷指令' }))

    expect(screen.getByRole('dialog')).toHaveTextContent('快捷指令 · Skills')
    expect(screen.getByDisplayValue('Fill this prompt')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Send this prompt')).toBeInTheDocument()
  })
})
