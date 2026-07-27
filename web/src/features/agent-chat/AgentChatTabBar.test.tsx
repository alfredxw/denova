import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { AgentChatTab } from './types'
import { AgentChatTabBar } from './AgentChatTabBar'

const tabs: AgentChatTab[] = [
  { kind: 'page', id: 'reader-tab', workspace: '/books/one', pageId: 'reader' },
  { kind: 'page', id: 'skills-tab', workspace: '/books/one', pageId: 'skills' },
]

describe('AgentChatTabBar', () => {
  it('shows the active workbench tab with its selected fill and accent rule', () => {
    render(
      <AgentChatTabBar
        group="primary"
        tabs={tabs}
        activeTabId="skills-tab"
        tabTitle={(tab) => tab.id === 'skills-tab' ? 'Skills tab' : 'Reader tab'}
        onActivate={vi.fn()}
        onClose={vi.fn()}
        onCloseOthers={vi.fn()}
        onCloseToRight={vi.fn()}
        onRename={vi.fn()}
        onTogglePin={vi.fn()}
        onMoveTab={vi.fn()}
        onNewAgentTab={vi.fn()}
        onNewTerminalTab={vi.fn()}
        onOpenPage={vi.fn()}
      />,
    )

    const activeTab = screen.getByRole('tab', { name: /Skills tab/ })
    expect(screen.getByRole('tablist')).toHaveClass('!h-full')
    expect(activeTab).toHaveAttribute('aria-selected', 'true')
    expect(activeTab.className).toContain('aria-[selected=true]:bg-[var(--nova-active)]')
    expect(activeTab.querySelector('[aria-hidden="true"]')?.className).toContain('group-aria-[selected=true]/tab:opacity-100')
    expect(screen.getByRole('tab', { name: /Reader tab/ })).toHaveAttribute('aria-selected', 'false')
  })

  it('offers Shell, Codex CLI, and Claude Code directly in the new-tab menu', async () => {
    const user = userEvent.setup()
    const onNewTerminalTab = vi.fn()
    render(
      <AgentChatTabBar
        group="primary"
        tabs={tabs}
        activeTabId="skills-tab"
        tabTitle={(tab) => tab.id}
        onActivate={vi.fn()}
        onClose={vi.fn()}
        onCloseOthers={vi.fn()}
        onCloseToRight={vi.fn()}
        onRename={vi.fn()}
        onTogglePin={vi.fn()}
        onMoveTab={vi.fn()}
        onNewAgentTab={vi.fn()}
        onNewTerminalTab={onNewTerminalTab}
        onOpenPage={vi.fn()}
      />,
    )

    const openMenu = () => user.click(screen.getByRole('button', { name: '新建标签页' }))
    await openMenu()
    expect(await screen.findByRole('menuitem', { name: '终端' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Codex CLI' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Claude Code' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Shell' })).not.toBeInTheDocument()
    expect(screen.queryByText('自定义命令…')).not.toBeInTheDocument()

    await user.click(screen.getByRole('menuitem', { name: '终端' }))
    expect(onNewTerminalTab).toHaveBeenLastCalledWith('primary', 'shell')
    await openMenu()
    await user.click(await screen.findByRole('menuitem', { name: 'Codex CLI' }))
    expect(onNewTerminalTab).toHaveBeenLastCalledWith('primary', 'codex')
    await openMenu()
    await user.click(await screen.findByRole('menuitem', { name: 'Claude Code' }))
    expect(onNewTerminalTab).toHaveBeenLastCalledWith('primary', 'claude')
  })
})
