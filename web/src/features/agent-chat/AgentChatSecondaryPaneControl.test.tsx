import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AgentChatSecondaryPaneControl } from './AgentChatSecondaryPaneControl'
import { AGENT_CHAT_PAGE_IDS, agentChatPageIdsForProjectType } from './types'

const baseProps = {
  visible: false,
  hasTabs: false,
  busy: false,
  terminalCommands: [],
  pageIds: AGENT_CHAT_PAGE_IDS,
  onShow: vi.fn(),
  onHide: vi.fn(),
  onNewAgentTab: vi.fn(),
  onNewTerminalTab: vi.fn(),
  onOpenFiles: vi.fn(),
  onOpenPage: vi.fn(),
}

describe('AgentChatSecondaryPaneControl', () => {
  it('opens the standard tab creation choices when the secondary pane is empty', async () => {
    const user = userEvent.setup()
    const onOpenPage = vi.fn()
    render(<AgentChatSecondaryPaneControl {...baseProps} onOpenPage={onOpenPage} />)

    await user.click(screen.getByRole('button', { name: '显示右侧工作区' }))
    await user.click(await screen.findByRole('menuitem', { name: '写作' }))

    expect(onOpenPage).toHaveBeenCalledWith('secondary', 'reader')
  })

  it('hides a populated pane without closing any tab', async () => {
    const user = userEvent.setup()
    const onHide = vi.fn()
    render(<AgentChatSecondaryPaneControl {...baseProps} visible hasTabs onHide={onHide} />)

    const button = screen.getByRole('button', { name: '隐藏右侧工作区' })
    expect(button).not.toHaveAttribute('title')
    await user.click(button)

    expect(onHide).toHaveBeenCalledTimes(1)
    expect(baseProps.onNewAgentTab).not.toHaveBeenCalled()
  })

  it('marks a populated pane while it is folded away', () => {
    const { rerender } = render(<AgentChatSecondaryPaneControl {...baseProps} hasTabs />)

    expect(screen.getByRole('button', { name: '显示右侧工作区' }))
      .toContainElement(document.querySelector('[data-slot="secondary-pane-presence-indicator"]'))

    rerender(<AgentChatSecondaryPaneControl {...baseProps} visible hasTabs />)

    expect(document.querySelector('[data-slot="secondary-pane-presence-indicator"]')).not.toBeInTheDocument()
  })

  it('does not mark an empty pane as populated', () => {
    render(<AgentChatSecondaryPaneControl {...baseProps} />)

    expect(screen.getByRole('button', { name: '显示右侧工作区' })).not.toHaveAttribute('title')
    expect(document.querySelector('[data-slot="secondary-pane-presence-indicator"]')).not.toBeInTheDocument()
  })

  it('offers only Files for General Projects', async () => {
    const user = userEvent.setup()
    render(
      <AgentChatSecondaryPaneControl
        {...baseProps}
        pageIds={agentChatPageIdsForProjectType('general')}
      />,
    )

    await user.click(screen.getByRole('button', { name: '显示右侧工作区' }))

    expect(await screen.findByRole('menuitem', { name: '文件' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: '写作' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: '资料库' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Skills' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: '版本管理' })).not.toBeInTheDocument()
  })

  it('announces running work when a populated pane is hidden', () => {
    render(<AgentChatSecondaryPaneControl {...baseProps} hasTabs busy />)

    const button = screen.getByRole('button', { name: '显示右侧工作区，仍有任务运行' })
    expect(button).toHaveAttribute('aria-pressed', 'false')
    expect(document.querySelector('[data-slot="secondary-pane-running-indicator"]')).toBeInTheDocument()
    expect(document.querySelector('[data-slot="secondary-pane-presence-indicator"]')).not.toBeInTheDocument()
  })
})
