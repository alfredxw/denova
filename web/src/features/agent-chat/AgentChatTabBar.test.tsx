import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { WorkbenchTabDragContext } from '@/components/workbench/WorkbenchTabDrag'
import { AGENT_CHAT_PAGE_IDS, type AgentChatTab } from './types'
import { AgentChatTabBar } from './AgentChatTabBar'

const tabs: AgentChatTab[] = [
  {
    kind: 'page',
    id: 'reader-tab',
    projectId: 'project-one',
    workspace: '/books/one',
    pageId: 'reader',
  },
  {
    kind: 'page',
    id: 'skills-tab',
    projectId: 'project-one',
    workspace: '/books/one',
    pageId: 'skills',
  },
]

function renderTabBar(ui: ReactNode) {
  return render(
    <TooltipProvider delayDuration={0}>
      <WorkbenchTabDragContext onDragEnd={vi.fn()}>{ui}</WorkbenchTabDragContext>
    </TooltipProvider>,
  )
}

describe('AgentChatTabBar', () => {
  it('shows the active workbench tab with its selected fill and accent rule', () => {
    renderTabBar(
      <AgentChatTabBar
        projectId="project-one"
        group="primary"
        tabs={tabs}
        activeTabId="skills-tab"
        terminalCommands={[]}
        pageIds={AGENT_CHAT_PAGE_IDS}
        tabTitle={(tab) => (tab.id === 'skills-tab' ? 'Skills tab' : 'Reader tab')}
        onActivate={vi.fn()}
        onClose={vi.fn()}
        onCloseOthers={vi.fn()}
        onCloseToRight={vi.fn()}
        onRename={vi.fn()}
        onTogglePin={vi.fn()}
        onMoveTab={vi.fn()}
        onNewAgentTab={vi.fn()}
        onNewTerminalTab={vi.fn()}
        onOpenFiles={vi.fn()}
        onOpenPage={vi.fn()}
      />,
    )

    const activeTab = screen.getByRole('tab', { name: /Skills tab/ })
    expect(screen.getByRole('tablist')).toHaveClass('!h-full')
    expect(activeTab).toHaveAttribute('aria-selected', 'true')
    expect(activeTab.className).toContain('aria-selected:bg-[var(--nova-active)]')
    expect(activeTab.querySelector('[aria-hidden="true"]')?.className).toContain('group-aria-[selected=true]/tab:opacity-100')
    expect(screen.getByRole('tab', { name: /Reader tab/ })).toHaveAttribute('aria-selected', 'false')
    expect(activeTab.parentElement).toHaveClass('min-w-28', 'max-w-40', 'flex-[1_1_10rem]')
    expect(screen.getByRole('tab', { name: /Reader tab/ }).parentElement).toHaveClass('min-w-28', 'max-w-40', 'flex-[1_1_10rem]')
    expect(activeTab).toHaveAttribute('aria-roledescription', '可排序标签页')
    expect(screen.getByRole('tablist')).toHaveClass('overflow-x-auto', '[&::-webkit-scrollbar]:hidden')
    expect(screen.getByRole('tablist')).toHaveStyle({ scrollbarWidth: 'none' })
    expect(screen.getByRole('button', { name: '新建标签页' })).toHaveClass('mx-1', 'h-7', 'w-8', 'self-center', 'rounded-lg')
  })

  it('renders the shared tab preview while keyboard dragging', async () => {
    const user = userEvent.setup()
    renderTabBar(
      <AgentChatTabBar
        projectId="project-one"
        group="primary"
        tabs={tabs}
        activeTabId="reader-tab"
        terminalCommands={[]}
        pageIds={AGENT_CHAT_PAGE_IDS}
        tabTitle={(tab) => (tab.id === 'reader-tab' ? 'Reader tab' : 'Skills tab')}
        onActivate={vi.fn()}
        onClose={vi.fn()}
        onCloseOthers={vi.fn()}
        onCloseToRight={vi.fn()}
        onRename={vi.fn()}
        onTogglePin={vi.fn()}
        onMoveTab={vi.fn()}
        onNewAgentTab={vi.fn()}
        onNewTerminalTab={vi.fn()}
        onOpenFiles={vi.fn()}
        onOpenPage={vi.fn()}
      />,
    )

    const tab = screen.getByRole('tab', { name: /Reader tab/ })
    tab.focus()
    fireEvent.keyDown(tab, { key: ' ', code: 'Space' })

    await waitFor(() => expect(document.querySelector('[data-slot="workbench-tab-drag-overlay"]')).toHaveTextContent('Reader tab'))

    await user.keyboard('{Escape}')
    await waitFor(() => expect(document.querySelector('[data-slot="workbench-tab-drag-overlay"]')).not.toBeInTheDocument())
  })

  it('keeps Review closing on the real workbench tab', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    const reviewTab: AgentChatTab = {
      kind: 'review',
      id: 'review-tab',
      projectId: 'project-one',
      workspace: '/books/one',
      threadID: 'review-thread',
    }
    renderTabBar(
      <AgentChatTabBar
        projectId="project-one"
        group="primary"
        tabs={[reviewTab]}
        activeTabId="review-tab"
        terminalCommands={[]}
        pageIds={AGENT_CHAT_PAGE_IDS}
        tabTitle={() => 'Review'}
        onActivate={vi.fn()}
        onClose={onClose}
        onCloseOthers={vi.fn()}
        onCloseToRight={vi.fn()}
        onRename={vi.fn()}
        onTogglePin={vi.fn()}
        onMoveTab={vi.fn()}
        onNewAgentTab={vi.fn()}
        onNewTerminalTab={vi.fn()}
        onOpenFiles={vi.fn()}
        onOpenPage={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '关闭 Review' }))
    expect(onClose).toHaveBeenCalledWith('review-tab')
  })

  it('reveals a genuinely truncated title below the strip after a deliberate hover delay', async () => {
    const user = userEvent.setup()
    const longTitle = 'A deliberately long conversation title that cannot fit in one tab'
    const clientWidth = vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(100)
    const scrollWidth = vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(240)
    try {
      renderTabBar(
        <AgentChatTabBar
          projectId="project-one"
          group="primary"
          tabs={tabs}
          activeTabId="reader-tab"
          terminalCommands={[]}
          pageIds={AGENT_CHAT_PAGE_IDS}
          tabTitle={(tab) => (tab.id === 'reader-tab' ? longTitle : 'Skills')}
          onActivate={vi.fn()}
          onClose={vi.fn()}
          onCloseOthers={vi.fn()}
          onCloseToRight={vi.fn()}
          onRename={vi.fn()}
          onTogglePin={vi.fn()}
          onMoveTab={vi.fn()}
          onNewAgentTab={vi.fn()}
          onNewTerminalTab={vi.fn()}
          onOpenFiles={vi.fn()}
          onOpenPage={vi.fn()}
        />,
      )

      await user.hover(screen.getByText(longTitle))
      expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
      const tooltip = await screen.findByRole('tooltip')
      expect(tooltip).toHaveTextContent(longTitle)
      expect(document.querySelector('[data-slot="tooltip-content"]')).toHaveAttribute('data-side', 'bottom')
    } finally {
      clientWidth.mockRestore()
      scrollWidth.mockRestore()
    }
  })

  it('does not create a title tooltip when the full label fits', async () => {
    const user = userEvent.setup()
    const clientWidth = vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(240)
    const scrollWidth = vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(120)
    try {
      renderTabBar(
        <AgentChatTabBar
          projectId="project-one"
          group="primary"
          tabs={tabs}
          activeTabId="reader-tab"
          terminalCommands={[]}
          pageIds={AGENT_CHAT_PAGE_IDS}
          tabTitle={(tab) => (tab.id === 'reader-tab' ? 'Reader' : 'Skills')}
          onActivate={vi.fn()}
          onClose={vi.fn()}
          onCloseOthers={vi.fn()}
          onCloseToRight={vi.fn()}
          onRename={vi.fn()}
          onTogglePin={vi.fn()}
          onMoveTab={vi.fn()}
          onNewAgentTab={vi.fn()}
          onNewTerminalTab={vi.fn()}
          onOpenFiles={vi.fn()}
          onOpenPage={vi.fn()}
        />,
      )

      await user.hover(screen.getByText('Reader'))
      await new Promise((resolve) => window.setTimeout(resolve, 600))
      expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
      expect(document.querySelector('[data-slot="tooltip-content"]')).not.toBeInTheDocument()
    } finally {
      clientWidth.mockRestore()
      scrollWidth.mockRestore()
    }
  })

  it('offers Shell and every enabled configured CLI directly in the new-tab menu', async () => {
    const user = userEvent.setup()
    const onNewTerminalTab = vi.fn()
    renderTabBar(
      <AgentChatTabBar
        projectId="project-one"
        group="primary"
        tabs={tabs}
        activeTabId="skills-tab"
        terminalCommands={[
          { id: 'codex', name: 'Codex CLI' },
          { id: 'claude', name: 'Claude Code' },
          { id: 'aider', name: 'Aider' },
        ]}
        pageIds={AGENT_CHAT_PAGE_IDS}
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
        onOpenFiles={vi.fn()}
        onOpenPage={vi.fn()}
      />,
    )

    const openMenu = () => user.click(screen.getByRole('button', { name: '新建标签页' }))
    await openMenu()
    expect(await screen.findByRole('menuitem', { name: '终端' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Codex CLI' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Claude Code' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Shell' })).not.toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Aider' })).toBeInTheDocument()

    await user.click(screen.getByRole('menuitem', { name: '终端' }))
    expect(onNewTerminalTab).toHaveBeenLastCalledWith('primary', 'shell')
    await openMenu()
    await user.click(await screen.findByRole('menuitem', { name: 'Codex CLI' }))
    expect(onNewTerminalTab).toHaveBeenLastCalledWith('primary', 'codex', 'Codex CLI')
    await openMenu()
    await user.click(await screen.findByRole('menuitem', { name: 'Claude Code' }))
    expect(onNewTerminalTab).toHaveBeenLastCalledWith('primary', 'claude', 'Claude Code')

    await openMenu()
    await user.click(await screen.findByRole('menuitem', { name: 'Aider' }))
    expect(onNewTerminalTab).toHaveBeenLastCalledWith('primary', 'aider', 'Aider')
  })
})
