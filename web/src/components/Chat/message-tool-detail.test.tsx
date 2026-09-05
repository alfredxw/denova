import { fireEvent, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ToolExecutionBlock } from './message-tool'
import { workspaceToolDetailAdapters } from './tool-detail/workspace'

afterEach(() => vi.restoreAllMocks())

describe('tool detail rendering', () => {
  it('defers folded results and reuses unchanged open results during streaming updates', () => {
    const adapter = workspaceToolDetailAdapters.read
    if (adapter.layout === 'unified') throw new Error('Expected the split read detail adapter')
    const renderOutput = vi.spyOn(adapter, 'renderOutput')
    const message = {
      role: 'tool_call' as const,
      name: 'read',
      args: JSON.stringify({ path: 'chapter.md' }),
      result: 'Earlier chapter content',
      status: 'success' as const,
    }
    const { container, rerender } = render(<ToolExecutionBlock message={message} />)
    const header = container.querySelector('[data-nova-tool-header]') as HTMLElement

    expect(renderOutput).not.toHaveBeenCalled()
    const latest = { ...message, result: 'Latest chapter content' }
    rerender(<ToolExecutionBlock message={latest} />)
    expect(renderOutput).not.toHaveBeenCalled()

    fireEvent.click(header)
    expect(renderOutput).toHaveBeenCalledTimes(1)
    expect(container.querySelector('[data-nova-tool-detail-output]')).toHaveTextContent(latest.result)

    // Another streaming part replaces the SDK message without changing this result.
    rerender(<ToolExecutionBlock message={{ ...latest }} />)
    expect(renderOutput).toHaveBeenCalledTimes(1)

    const updated = { ...latest, result: 'Updated open result' }
    rerender(<ToolExecutionBlock message={updated} />)
    expect(renderOutput).toHaveBeenCalledTimes(2)
    expect(container.querySelector('[data-nova-tool-detail-output]')).toHaveTextContent(updated.result)

    fireEvent.click(header)
    const callsAfterClose = renderOutput.mock.calls.length
    rerender(<ToolExecutionBlock message={message} />)
    expect(renderOutput).toHaveBeenCalledTimes(callsAfterClose)
    fireEvent.click(header)
    expect(container.querySelector('[data-nova-tool-detail-output]')).toHaveTextContent(message.result)
  })
})
