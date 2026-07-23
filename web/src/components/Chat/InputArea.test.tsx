import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
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

  it('inserts selected Skills as inline tokens and sends compatible text', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn()
    render(
      <InputArea
        onSend={handleSend}
        disabled={false}
        commandScope="skills"
        skills={[{ name: 'skills-creator', description: '创建 Skill' }]}
      />,
    )

    await user.type(screen.getByRole('textbox'), '/ski')
    await user.click(screen.getByText('/skills-creator'))

    const textbox = screen.getByRole('textbox')
    expect(within(textbox).getByText('/skills-creator')).toHaveClass('nova-composer-token')

    await user.click(screen.getByRole('button', { name: '发送' }))

    expect(handleSend).toHaveBeenCalledWith('/skills-creator')
  })

  it('renders external file references inside the input and removes them as tokens', async () => {
    const user = userEvent.setup()
    const handleRemove = vi.fn()
    render(
      <InputArea
        onSend={vi.fn()}
        disabled={false}
        referencedFiles={['chapters/ch01.md']}
        onReferenceRemove={handleRemove}
      />,
    )

    const textbox = screen.getByRole('textbox')
    expect(await within(textbox).findByText('@chapters/ch01.md')).toHaveClass('nova-composer-token')
    expect(document.querySelector('.nova-agent-composer-references')).toBeNull()

    textbox.focus()
    await user.keyboard('{Backspace}{Backspace}')

    await waitFor(() => expect(handleRemove).toHaveBeenCalledWith('chapters/ch01.md'))
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

  it('shows selected inline comments and allows sending them without extra text', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn()
    const handleRemove = vi.fn()
    render(
      <InputArea
        onSend={handleSend}
        disabled={false}
        reviewFeedback={[{
          reviewThreadId: 'review-1',
          comments: [{
            id: 'comment-1',
            group_id: 'group-1',
            body: '把这一段写得更克制',
            review_path: 'chapters/ch01.md',
            review_line: 12,
          }],
        }]}
        onReviewFeedbackRemove={handleRemove}
      />,
    )

    expect(screen.getByText(/把这一段写得更克制/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '发送' }))
    expect(handleSend).toHaveBeenCalledWith('')

    await user.click(screen.getByRole('button', { name: '移出本次提交' }))
    expect(handleRemove).toHaveBeenCalledWith(expect.objectContaining({ reviewThreadId: 'review-1' }), 'comment-1')
  })

  it('restores supplemental instructions when a review-feedback request is rejected', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn().mockResolvedValue(false)
    render(<InputArea onSend={handleSend} disabled={false} />)

    await user.type(screen.getByRole('textbox'), 'keep pace')
    const submittedText = screen.getByRole('textbox').textContent || ''
    expect(submittedText).not.toBe('')
    await user.click(screen.getByRole('button', { name: '发送' }))

    expect(handleSend).toHaveBeenCalledWith(submittedText)
    await waitFor(() => expect(screen.getByRole('textbox')).toHaveTextContent(submittedText))
  })

  it('submits selected review feedback only once while the request is being accepted', async () => {
    let settleRequest: (accepted: boolean) => void = () => undefined
    const handleSend = vi.fn(() => new Promise<boolean>((resolve) => { settleRequest = resolve }))
    render(
      <InputArea
        onSend={handleSend}
        disabled={false}
        reviewFeedback={[{
          reviewThreadId: 'review-1',
          comments: [{ id: 'comment-1', group_id: 'group-1', body: '调整这一行' }],
        }]}
        onReviewFeedbackRemove={vi.fn()}
      />,
    )

    const sendButton = screen.getByRole('button', { name: '发送' })
    fireEvent.click(sendButton)
    fireEvent.click(sendButton)
    fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Enter', shiftKey: false })

    expect(handleSend).toHaveBeenCalledTimes(1)
    expect(sendButton).toBeDisabled()

    settleRequest(true)
    await waitFor(() => expect(sendButton).toBeEnabled())
  })
})

describe('InputArea prefill clearing', () => {
  it('clears prefilled prompt after sending without disabled transition', async () => {
    const user = userEvent.setup()
    const sentMessages: string[] = []

    function Wrapper() {
      const [inputPrefill, setInputPrefill] = useState<{ prompt: string; nonce: number } | null>({ prompt: 'prefilled-init', nonce: 1 })
      return (
        <InputArea
          onSend={(msg) => { sentMessages.push(msg) }}
          disabled={false}
          inputPrefill={inputPrefill}
          onInputPrefillConsumed={() => setInputPrefill(null)}
        />
      )
    }

    render(<Wrapper />)

    await waitFor(() => {
      expect(screen.getByRole('textbox')).toHaveTextContent('prefilled-init')
    })

    await user.click(screen.getByRole('button', { name: '发送' }))

    expect(sentMessages).toEqual(['prefilled-init'])

    await waitFor(() => {
      expect(screen.getByRole('textbox')).not.toHaveTextContent('prefilled-init')
    })
  })

  it('clears prefilled prompt after sending (realistic inputPrefill state)', async () => {
    const user = userEvent.setup()
    const sentMessages: string[] = []

    function Wrapper() {
      const [inputPrefill, setInputPrefill] = useState<{ prompt: string; nonce: number } | null>({ prompt: 'prefilled-init', nonce: 1 })
      const [disabled, setDisabled] = useState(false)
      return (
        <InputArea
          onSend={(msg) => { sentMessages.push(msg); setDisabled(true) }}
          disabled={disabled}
          inputPrefill={inputPrefill}
          onInputPrefillConsumed={() => setInputPrefill(null)}
        />
      )
    }

    render(<Wrapper />)

    await waitFor(() => {
      expect(screen.getByRole('textbox')).toHaveTextContent('prefilled-init')
    })

    await user.click(screen.getByRole('button', { name: '发送' }))

    expect(sentMessages).toEqual(['prefilled-init'])

    await waitFor(() => {
      expect(screen.getByRole('textbox')).not.toHaveTextContent('prefilled-init')
    })
  })
})

describe('InputArea prompt recall', () => {
  it('restores the last submitted prompt when ArrowUp is pressed on an empty input', async () => {
    const user = userEvent.setup()
    render(<InputArea onSend={vi.fn()} disabled={false} />)

    const textbox = screen.getByRole('textbox')
    const prompt = 'restart scene one optimization and explain every change'
    await user.type(textbox, prompt)
    const submittedPrompt = textbox.textContent || ''
    expect(submittedPrompt.length).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => {
      expect(textbox).toHaveTextContent('')
    })

    textbox.focus()
    await user.keyboard('{ArrowUp}')

    expect(textbox).toHaveTextContent(submittedPrompt)
  })

  it('does not override existing input when ArrowUp is pressed with non-empty content', async () => {
    const user = userEvent.setup()
    render(<InputArea onSend={vi.fn()} disabled={false} />)

    const textbox = screen.getByRole('textbox')
    await user.type(textbox, 'previous prompt')
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => {
      expect(textbox).toHaveTextContent('')
    })

    const currentDraft = 'new draft'
    await user.type(textbox, currentDraft)
    const renderedDraft = textbox.textContent || ''
    expect(renderedDraft.length).toBeGreaterThan(0)
    await user.keyboard('{ArrowUp}')

    expect(textbox).toHaveTextContent(renderedDraft)
  })
})
