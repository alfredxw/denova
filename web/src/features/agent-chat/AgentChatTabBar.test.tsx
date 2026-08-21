import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { WorkbenchTabDragContext } from '@/components/workbench/WorkbenchTabDrag'
import { AGENT_CHAT_PAGE_IDS, type AgentChatTab } from './types'
import { AgentChatTabBar } from './AgentChatTabBar'

const projectId = 'project-one'

const tabs: AgentChatTab[] = [
  {
    kind: 'page',
    id: 'reader-tab',
    projectId: 'project-one',
    workspace: '/books/one',
    group: 'primary',
    pageId: 'reader',
  },
  {
    kind: 'page',
    id: 'lore-tab',
    projectId: 'project-one',
    workspace: '/books/one',
    group: 'primary',
    pageId: 'lore',
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
  it('shows the active workbench tab as a rounded filled surface', () => {
    renderTabBar(
      <AgentChatTabBar
        projectId={projectId}
        group="primary"
        tabs={tabs}
        activeTabId="lore-tab"
        terminalCommands={[]}
        pageIds={AGENT_CHAT_PAGE_IDS}
        tabTitle={(tab) => (tab.id === 'lore-tab' ? 'Lore tab' : 'Writing tab')}
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

    const activeTab = screen.getByRole('tab', { name: /Lore tab/ })
    expect(screen.getByRole('tablist')).toHaveClass('!h-full')
    expect(activeTab).toHaveAttribute('aria-selected', 'true')
    expect(activeTab.className).toContain('aria-selected:bg-[var(--nova-active)]')
    expect(screen.getByRole('tab', { name: /Writing tab/ })).toHaveAttribute('aria-selected', 'false')
    expect(activeTab).toHaveClass('rounded-[var(--nova-radius)]')
    expect(activeTab.parentElement).toHaveClass('h-7', 'min-w-24', 'max-w-40', 'flex-[1_1_10rem]', 'self-center', 'after:h-3')
    expect(screen.getByRole('tab', { name: /Writing tab/ }).parentElement).toHaveClass('h-7', 'min-w-24', 'max-w-40', 'flex-[1_1_10rem]', 'self-center')
    expect(screen.getByRole('button', { name: '关闭 Lore tab' })).toHaveClass(
      'w-0',
      '-ml-1.5',
      'group-hover/tab:w-4',
      'group-aria-[selected=true]/tab:w-4',
      'group-aria-[selected=true]/tab:opacity-100',
    )
    expect(activeTab).toHaveAttribute('aria-roledescription', '可排序标签页')
    expect(screen.getByRole('tablist')).toHaveClass('overflow-x-auto', '[&::-webkit-scrollbar]:hidden')
    expect(screen.getByRole('tablist')).toHaveStyle({ scrollbarWidth: 'none' })
    expect(screen.getByRole('button', { name: '新建标签页' })).toHaveClass('h-7', 'w-7', 'self-center', 'rounded-[var(--nova-radius)]', 'border-transparent', 'bg-transparent')
    expect(document.querySelector('[data-slot="agent-chat-project-context"]')).not.toBeInTheDocument()
  })

  it('keeps a trailing pane toggle visually inline with the tabs', () => {
    renderTabBar(
      <AgentChatTabBar
        projectId={projectId}
        group="primary"
        tabs={tabs}
        activeTabId="reader-tab"
        terminalCommands={[]}
        pageIds={AGENT_CHAT_PAGE_IDS}
        tabTitle={(tab) => tab.id}
        endActions={<button type="button">Pane toggle</button>}
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

    expect(screen.getByRole('button', { name: 'Pane toggle' }).parentElement)
      .toHaveClass('px-1')
    expect(screen.getByRole('button', { name: 'Pane toggle' }).parentElement)
      .not.toHaveClass('border-l')
  })

  it('renders the shared tab preview while keyboard dragging', async () => {
    const user = userEvent.setup()
    renderTabBar(
      <AgentChatTabBar
        projectId={projectId}
        group="primary"
        tabs={tabs}
        activeTabId="reader-tab"
        terminalCommands={[]}
        pageIds={AGENT_CHAT_PAGE_IDS}
        tabTitle={(tab) => (tab.id === 'reader-tab' ? 'Writing tab' : 'Lore tab')}
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

    const tab = screen.getByRole('tab', { name: /Writing tab/ })
    tab.focus()
    fireEvent.keyDown(tab, { key: ' ', code: 'Space' })

    await waitFor(() => expect(document.querySelector('[data-slot="workbench-tab-drag-overlay"]')).toHaveTextContent('Writing tab'))

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
      group: 'primary',
      threadID: 'review-thread',
    }
    renderTabBar(
      <AgentChatTabBar
        projectId={projectId}
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
          projectId={projectId}
          group="primary"
          tabs={tabs}
          activeTabId="reader-tab"
          terminalCommands={[]}
          pageIds={AGENT_CHAT_PAGE_IDS}
          tabTitle={(tab) => (tab.id === 'reader-tab' ? longTitle : 'Lore')}
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
      const title = document.querySelector('[data-slot="workbench-tab-title"]')
      expect(title).toHaveClass('nova-workbench-tab-title-fade', 'overflow-hidden', 'whitespace-nowrap')
      expect(title).not.toHaveClass('truncate')
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
          projectId={projectId}
          group="primary"
          tabs={tabs}
          activeTabId="reader-tab"
          terminalCommands={[]}
          pageIds={AGENT_CHAT_PAGE_IDS}
          tabTitle={(tab) => (tab.id === 'reader-tab' ? 'Writing' : 'Lore')}
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

      await user.hover(screen.getByText('Writing'))
      await new Promise((resolve) => window.setTimeout(resolve, 600))
      expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
      expect(document.querySelector('[data-slot="tooltip-content"]')).not.toBeInTheDocument()
      expect(screen.getByText('Writing')).not.toHaveClass('nova-workbench-tab-title-fade')
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
        projectId={projectId}
        group="primary"
        tabs={tabs}
        activeTabId="lore-tab"
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
    expect(screen.getAllByRole('menuitem').map((item) => item.textContent)).toEqual([
      '新建对话',
      '终端',
      'Codex CLI',
      'Claude Code',
      'Aider',
      '写作',
      '文件',
      '资料库',
    ])

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
