import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { InputArea } from './InputArea'

describe('InputArea command menu', () => {
  it('shows enabled built-in commands before Skills when typing slash', async () => {
    const user = userEvent.setup()
    render(
      <InputArea
        onSend={vi.fn()}
        disabled={false}
        commandScope="all"
        builtinCommands={['/clear']}
        skills={[{ name: 'skills-creator', description: '创建 Skill' }]}
      />,
    )

    await user.type(screen.getByRole('textbox'), '/')

    const clearCommand = screen.getByText('/clear')
    const skillCommand = screen.getByText('/skills-creator')
    expect(clearCommand).toBeInTheDocument()
    expect(skillCommand).toBeInTheDocument()
    expect(screen.queryByText('/plan')).not.toBeInTheDocument()
    expect(clearCommand.compareDocumentPosition(skillCommand) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('moves Plan Mode into input actions instead of rendering a standalone button', async () => {
    const user = userEvent.setup()
    const handleTogglePlanMode = vi.fn()
    render(
      <InputArea
        onSend={vi.fn()}
        disabled={false}
        planMode={false}
        onTogglePlanMode={handleTogglePlanMode}
      />,
    )

    expect(screen.getByRole('textbox')).toHaveAttribute('rows', '1')
    expect(screen.queryByRole('button', { name: 'Chat' })).not.toBeInTheDocument()
    expect(screen.queryByText('Plan')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '输入动作' }))
    await user.click(screen.getByRole('menuitemcheckbox', { name: /Plan/ }))

    expect(handleTogglePlanMode).toHaveBeenCalledTimes(1)
  })

  it('shows a read-only Plan indicator only while Plan Mode is active', () => {
    const { rerender } = render(
      <InputArea
        onSend={vi.fn()}
        disabled={false}
        planMode
        onTogglePlanMode={vi.fn()}
      />,
    )

    const indicator = screen.getByLabelText('Plan Mode 已开启')
    expect(indicator).toHaveTextContent('Plan')
    expect(indicator.closest('button')).toBeNull()

    rerender(
      <InputArea
        onSend={vi.fn()}
        disabled={false}
        planMode={false}
        onTogglePlanMode={vi.fn()}
      />,
    )

    expect(screen.queryByLabelText('Plan Mode 已开启')).not.toBeInTheDocument()
  })
})
