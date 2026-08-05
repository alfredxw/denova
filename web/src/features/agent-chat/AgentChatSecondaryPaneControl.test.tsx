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
    await user.click(await screen.findByRole('menuitem', { name: '阅读器' }))

    expect(onOpenPage).toHaveBeenCalledWith('secondary', 'reader')
  })

  it('hides a populated pane without closing any tab', async () => {
    const user = userEvent.setup()
    const onHide = vi.fn()
    render(<AgentChatSecondaryPaneControl {...baseProps} visible hasTabs onHide={onHide} />)

    await user.click(screen.getByRole('button', { name: '隐藏右侧工作区' }))

    expect(onHide).toHaveBeenCalledTimes(1)
    expect(baseProps.onNewAgentTab).not.toHaveBeenCalled()
  })

  it('keeps reusable pages available for General Projects and hides Book-only pages', async () => {
    const user = userEvent.setup()
    render(
      <AgentChatSecondaryPaneControl
        {...baseProps}
        pageIds={agentChatPageIdsForProjectType('general')}
      />,
    )

    await user.click(screen.getByRole('button', { name: '显示右侧工作区' }))

    expect(await screen.findByRole('menuitem', { name: 'Skills' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: '预设' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: '阅读器' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: '版本管理' })).not.toBeInTheDocument()
  })

  it('announces running work when a populated pane is hidden', () => {
    render(<AgentChatSecondaryPaneControl {...baseProps} hasTabs busy />)

    const button = screen.getByRole('button', { name: '显示右侧工作区，仍有任务运行' })
    expect(button).toHaveAttribute('aria-pressed', 'false')
    expect(button.querySelector('[aria-hidden="true"]')).toBeInTheDocument()
  })
})
