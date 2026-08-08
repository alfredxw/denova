import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentChatRenameDialog } from './AgentChatRenameDialog'

function renderRenameDialog(overrides: Partial<Parameters<typeof AgentChatRenameDialog>[0]> = {}) {
  const props: Parameters<typeof AgentChatRenameDialog>[0] = {
    open: true,
    initialValue: 'Old title',
    title: 'Rename conversation',
    description: 'Change the visible title.',
    label: 'Conversation title',
    requiredMessage: 'Enter a title.',
    inputId: 'conversation-title',
    onOpenChange: vi.fn(),
    onRename: vi.fn(),
    ...overrides,
  }
  return { ...render(<AgentChatRenameDialog {...props} />), props }
}

describe('AgentChatRenameDialog', () => {
  it('trims and submits the new name before closing', async () => {
    const { props } = renderRenameDialog()
    const input = screen.getByLabelText('Conversation title')
    fireEvent.change(input, { target: { value: '  New title  ' } })
    fireEvent.submit(input.closest('form')!)

    await waitFor(() => expect(props.onRename).toHaveBeenCalledWith('New title'))
    expect(props.onOpenChange).toHaveBeenCalledWith(false)
  })

  it('keeps the dialog open when the name is empty', () => {
    const { props } = renderRenameDialog()
    const input = screen.getByLabelText('Conversation title')
    fireEvent.change(input, { target: { value: '   ' } })
    fireEvent.submit(input.closest('form')!)

    expect(screen.getByRole('alert')).toHaveTextContent('Enter a title.')
    expect(props.onRename).not.toHaveBeenCalled()
    expect(props.onOpenChange).not.toHaveBeenCalled()
  })
})
