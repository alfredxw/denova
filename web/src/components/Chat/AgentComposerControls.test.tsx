import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AgentComposerControls } from './AgentComposerControls'

describe('AgentComposerControls', () => {
  it('turns the empty idle send action into Resume when an interruption is pending', async () => {
    const onSend = vi.fn()
    render(
      <AgentComposerControls
        generationActive={false}
        hasSendableContent={false}
        resumeAvailable
        onSend={onSend}
        sendDisabled={false}
        disabled={false}
      />,
    )

    const resume = screen.getByRole('button', { name: '继续生成' })
    expect(resume).toBeEnabled()
    expect(resume).toHaveAttribute('data-action', 'resume')
    await userEvent.click(resume)
    expect(onSend).toHaveBeenCalledTimes(1)
  })

  it('keeps Resume disabled while command admission is blocked', () => {
    render(
      <AgentComposerControls
        generationActive={false}
        hasSendableContent={false}
        resumeAvailable
        onSend={vi.fn()}
        sendDisabled
        disabled={false}
      />,
    )

    expect(screen.getByRole('button', { name: '继续生成' })).toBeDisabled()
  })
})
