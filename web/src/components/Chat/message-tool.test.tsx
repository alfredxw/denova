import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ToolExecutionBlock } from './message-tool'

describe('ToolExecutionBlock', () => {
  it('progressively formats incomplete JSON tool input without interpreting it as Markdown', async () => {
    const rawInput = '{"path":"chapters/ch01.md","options":{"recursive":tr'
    const { container } = render(<ToolExecutionBlock
      message={{
        id: 'streaming-write',
        role: 'tool_call',
        name: 'write',
        args: rawInput,
        status: 'running',
        streaming: true,
      }}
    />)

    const input = container.querySelector('[data-nova-tool-input-stream]')
    expect(input).toHaveAttribute('aria-busy', 'true')
    await waitFor(() => {
      expect(input).toHaveTextContent('"recursive": true')
    })
    expect(input?.textContent).toBe(`{
  "path": "chapters/ch01.md",
  "options": {
    "recursive": true
  }
}`)
    expect(input?.querySelector('a, em')).not.toBeInTheDocument()
  })

  it('keeps non-JSON streaming input visible as raw text', async () => {
    const rawInput = '*files* [chapter](https://example.com)'
    const { container } = render(<ToolExecutionBlock
      message={{
        id: 'streaming-raw',
        role: 'tool_call',
        name: 'unknown_tool',
        args: rawInput,
        status: 'running',
        streaming: true,
      }}
    />)

    const input = container.querySelector('[data-nova-tool-input-stream]')
    await waitFor(() => {
      expect(input).toHaveTextContent(rawInput)
    })
    expect(input?.textContent).toBe(rawInput)
    expect(input?.querySelector('a, em')).not.toBeInTheDocument()
  })

  it('removes the streaming projection when the tool input completes', async () => {
    const message = {
      id: 'completed-input',
      role: 'tool_call' as const,
      name: 'unknown_tool',
      args: '{"path":"chapters/ch01.md"',
      status: 'running' as const,
      streaming: true,
    }
    const { container, rerender } = render(<ToolExecutionBlock message={message} />)

    await waitFor(() => {
      expect(container.querySelector('[data-nova-tool-input-stream]')).toHaveTextContent('"path": "chapters/ch01.md"')
    })
    rerender(<ToolExecutionBlock message={{ ...message, args: '{"path":"chapters/ch01.md"}', streaming: false }} />)

    expect(container.querySelector('[data-nova-tool-input-stream]')).not.toBeInTheDocument()
  })

  it('presents a bounded directory read as an expected partial result', () => {
    const metadata = JSON.stringify({
      schema: 'resource.read.v1',
      status: 'partial',
      source: { kind: 'directory', path: '.' },
      limits: { returned: 200, truncated: true },
      recovery: { retryable: true, suggestion: 'Narrow the resource path or requested limit.' },
    })

    const { container } = render(<ToolExecutionBlock
      message={{
        role: 'tool_call',
        name: 'read',
        args: JSON.stringify({ path: '.', limit: 200, depth: 2, hidden: false }),
        result: `${metadata}\nAGENTS.md\nCHANGELOG.md`,
        status: 'success',
      }}
    />)

    expect(screen.getByText(/已返回当前批次/)).toBeInTheDocument()
    expect(screen.queryByText(/结果不完整/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Narrow the resource path/)).not.toBeInTheDocument()
    expect(container.querySelector('.lucide-triangle-alert')).not.toBeInTheDocument()
  })
})
