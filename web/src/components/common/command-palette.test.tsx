import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { CommandPalette } from './command-palette'

function renderCommandPalette(overrides: Partial<React.ComponentProps<typeof CommandPalette>> = {}) {
  const props: React.ComponentProps<typeof CommandPalette> = {
    open: true,
    onOpenChange: vi.fn(),
    onSave: vi.fn(),
    onOpenAgent: vi.fn(),
    onOpenVersions: vi.fn(),
    onOpenSearch: vi.fn(),
    onContinueWriting: vi.fn(),
    onClosePanels: vi.fn(),
    ...overrides,
  }
  render(<CommandPalette {...props} />)
  return props
}

describe('CommandPalette', () => {
  it('打开后展示工作台命令', () => {
    renderCommandPalette()

    expect(screen.getByText('保存当前章节')).toBeInTheDocument()
    expect(screen.getByText('打开创作Agent')).toBeInTheDocument()
    expect(screen.getByText('打开版本管理')).toBeInTheDocument()
    expect(screen.queryByText('打开任务面板')).not.toBeInTheDocument()
    expect(screen.queryByText('关闭任务面板')).not.toBeInTheDocument()
  })

  it('点击打开版本管理时调用 handler', async () => {
    const user = userEvent.setup()
    const props = renderCommandPalette()

    await user.click(screen.getByText('打开版本管理'))

    expect(props.onOpenVersions).toHaveBeenCalledTimes(1)
    expect(props.onOpenChange).toHaveBeenCalledWith(false)
  })

  it('流式中禁用继续写作命令', () => {
    renderCommandPalette({ isStreaming: true })

    expect(screen.getByText('继续写作').closest('[cmdk-item]')).toHaveAttribute('data-disabled', 'true')
  })
})
