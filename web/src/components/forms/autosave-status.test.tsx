import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AutosaveStatusIndicator } from './autosave-status'

describe('AutosaveStatusIndicator', () => {
  it('announces autosave progress without exposing a save action', () => {
    const view = render(<AutosaveStatusIndicator status="pending" />)

    expect(screen.getByRole('status')).toHaveTextContent('等待自动保存')
    expect(screen.queryByRole('button', { name: '保存' })).not.toBeInTheDocument()

    view.rerender(<AutosaveStatusIndicator status="saving" />)
    expect(screen.getByRole('status')).toHaveTextContent('正在自动保存')

    view.rerender(<AutosaveStatusIndicator status="saved" />)
    expect(screen.getByRole('status')).toHaveTextContent('所有更改均已保存')
  })

  it('offers an explicit retry only after autosave fails', () => {
    const onRetry = vi.fn()
    render(<AutosaveStatusIndicator status="error" error="网络不可用" onRetry={onRetry} />)

    expect(screen.getByRole('status')).toHaveTextContent('自动保存失败：网络不可用')
    fireEvent.click(screen.getByRole('button', { name: '重试自动保存' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('explains when invalid configuration pauses autosave', () => {
    render(<AutosaveStatusIndicator status="blocked" />)

    expect(screen.getByRole('status')).toHaveTextContent('修复配置后将自动保存')
  })
})
