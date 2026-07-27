import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { InputArea } from './InputArea'

describe('InputArea command menu', () => {
  it('keeps retired writing actions out of the built-in command list', async () => {
    const user = userEvent.setup()
    render(<InputArea onSend={vi.fn()} disabled={false} />)

    await user.type(screen.getByRole('textbox'), '/')

    expect(screen.getByText('/plan')).toBeInTheDocument()
    expect(screen.queryByText('/outline')).not.toBeInTheDocument()
    expect(screen.queryByText('/group-plan')).not.toBeInTheDocument()
    expect(screen.queryByText('/continue')).not.toBeInTheDocument()
    expect(screen.queryByText('/rewrite')).not.toBeInTheDocument()
  })

  it('shows enabled built-in commands before Skills when typing slash', async () => {
    const user = userEvent.setup()
    render(
      <InputArea
        onSend={vi.fn()}
        disabled={false}
        commandScope="all"
        builtinCommands={['/clear']}
        skills={[{ name: 'scene-tone', description: '场景语气' }]}
      />,
    )

    await user.type(screen.getByRole('textbox'), '/')

    const clearCommand = screen.getByText('/clear')
    const skillCommand = screen.getByText('/scene-tone')
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
        skills={[{ name: 'scene-tone', description: '场景语气' }]}
      />,
    )

    await user.type(screen.getByRole('textbox'), '/sce')
    await user.click(screen.getByText('/scene-tone'))

    const textbox = screen.getByRole('textbox')
    expect(within(textbox).getByText('/scene-tone')).toHaveClass('nova-composer-token')

    await user.click(screen.getByRole('button', { name: '发送' }))

    expect(handleSend).toHaveBeenCalledWith('/scene-tone')
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
    const token = await within(textbox).findByText('@ch01.md')
    expect(token).toHaveClass('nova-composer-token')
    expect(token).toHaveAttribute('data-token-value', 'chapters/ch01.md')
    expect(token).toHaveAttribute('title', 'chapters/ch01.md')
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
    const planAction = screen.getByRole('menuitemcheckbox', { name: /Plan/ })
    expect(within(planAction).getByText('Shift+Tab')).toHaveClass('order-3', 'ml-auto', 'shrink-0')
    await user.click(planAction)

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
    const handleOpen = vi.fn()
    const feedback = {
      source: 'document' as const,
      reviewThreadId: 'review-1',
      comments: [{
        id: 'comment-1',
        body: '把这一段写得更克制',
        path: 'chapters/ch01.md',
      }],
    }
    render(
      <InputArea
        onSend={handleSend}
        disabled={false}
        reviewFeedback={[feedback]}
        onReviewFeedbackOpen={handleOpen}
        onReviewFeedbackRemove={handleRemove}
      />,
    )

    expect(screen.getByText(/把这一段写得更克制/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /批注 · chapters\/ch01\.md.*把这一段写得更克制/ }))
    expect(handleOpen).toHaveBeenCalledWith(feedback, feedback.comments[0])

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

describe('InputArea active generation controls', () => {
  it('keeps the composer editable and exposes independent send and stop actions', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn()
    const handleStop = vi.fn()

    render(
      <InputArea
        onSend={handleSend}
        onStop={handleStop}
        disabled={false}
        generationActive
        inputPrefill={{ prompt: 'Add more atmosphere', nonce: 1 }}
      />,
    )

    const textbox = screen.getByRole('textbox')
    await waitFor(() => expect(textbox).toHaveTextContent('Add more atmosphere'))
    expect(textbox).toHaveAttribute('contenteditable', 'true')

    const sendButton = screen.getByRole('button', { name: '发送' })
    const stopButton = screen.getByRole('button', { name: '中断 AI 执行' })
    expect(sendButton).toBeEnabled()
    expect(stopButton).toBeEnabled()

    await user.click(sendButton)
    expect(handleSend).toHaveBeenCalledWith('Add more atmosphere')
    expect(handleStop).not.toHaveBeenCalled()

    await user.click(stopButton)
    expect(handleStop).toHaveBeenCalledTimes(1)
  })

  it('shows an accepted instruction above the composer with steer, delete, and return-to-edit actions', async () => {
    const user = userEvent.setup()
    const queued = {
      command_id: 'queued-1',
      operation_id: 'operation-1',
      delivery: 'follow_up' as const,
      message: 'Change the ending to a cliffhanger',
    }
    const handleSteer = vi.fn()
    const handleDelete = vi.fn()
    const handleEdit = vi.fn()

    render(
      <InputArea
        onSend={vi.fn()}
        onStop={vi.fn()}
        disabled={false}
        generationActive
        queuedCommands={[queued]}
        onQueuedCommandSteer={handleSteer}
        onQueuedCommandDelete={handleDelete}
        onQueuedCommandEdit={handleEdit}
      />,
    )

    expect(screen.queryByRole('button', { name: /发送方式/ })).not.toBeInTheDocument()
    expect(screen.getByText('Change the ending to a cliffhanger')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '立即转向' }))
    expect(handleSteer).toHaveBeenCalledWith(queued)

    await user.click(screen.getByRole('button', { name: '删除排队指令' }))
    expect(handleDelete).toHaveBeenCalledWith(queued)

    await user.click(screen.getByRole('button', { name: '更多排队指令操作' }))
    await user.click(screen.getByRole('menuitem', { name: '返回编辑' }))
    expect(handleEdit).toHaveBeenCalledWith(queued)
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
