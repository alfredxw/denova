import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ToolExecutionBlock } from './message-tool'

describe('ToolExecutionBlock', () => {
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
