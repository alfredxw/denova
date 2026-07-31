import type { ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  enforceTabLimit,
  persistTabsFor,
  readTabsFor,
  setTabPinned,
  TabController,
  type Tab,
} from './TabController'

const tabs: Tab[] = [
  { kind: 'file', path: 'chapters/alpha.md' },
  { kind: 'file', path: 'chapters/beta.md' },
]

function renderTabController(ui: ReactNode) {
  return render(<TooltipProvider>{ui}</TooltipProvider>)
}

describe('TabController', () => {
  it('persists Lore as a singleton workspace tab and renders its dedicated label', () => {
    window.localStorage.clear()
    persistTabsFor('/books/demo', [tabs[0], { kind: 'lore' }, { kind: 'lore' }])
    const restored = readTabsFor('/books/demo')

    expect(restored).toEqual([tabs[0], { kind: 'lore' }])
    renderTabController(
      <TabController
        tabs={restored}
        activeTabKey="lore"
        summary={null}
        onActivateTab={vi.fn()}
        onCloseTab={vi.fn()}
        onTogglePin={vi.fn()}
      />,
    )
    const loreTab = screen.getByRole('tab', { name: /设定/ })
    expect(loreTab).not.toHaveAttribute('title')
    expect(loreTab).toHaveAttribute('aria-selected', 'true')
    expect(loreTab.className).toContain('aria-selected:bg-[var(--nova-active)]')
    expect(loreTab.parentElement).toHaveClass('min-w-28', 'max-w-40', 'flex-[1_1_10rem]')
    expect(screen.getByRole('tablist')).toHaveClass('overflow-x-auto', '[&::-webkit-scrollbar]:hidden')
  })

  it('activates a tab when clicking the tab surface outside the label text', async () => {
    const user = userEvent.setup()
    const onActivateTab = vi.fn()

    renderTabController(
      <TabController
        tabs={tabs}
        activeTabKey="file:chapters/alpha.md"
        summary={null}
        onActivateTab={onActivateTab}
        onCloseTab={vi.fn()}
        onTogglePin={vi.fn()}
      />,
    )

    await user.click(screen.getByText('beta.md').closest('[role="tab"]')!)

    expect(onActivateTab).toHaveBeenCalledWith(tabs[1])
  })

  it('does not activate the tab when clicking its close button', () => {
    const onActivateTab = vi.fn()
    const onCloseTab = vi.fn()

    renderTabController(
      <TabController
        tabs={tabs}
        activeTabKey="file:chapters/alpha.md"
        summary={null}
        onActivateTab={onActivateTab}
        onCloseTab={onCloseTab}
        onTogglePin={vi.fn()}
      />,
    )

    const tab = screen.getByRole('tab', { name: /beta.md/ })
    const closeButton = screen.getByRole('button', { name: '关闭 beta.md' })
    expect(tab).not.toContainElement(closeButton)

    fireEvent.click(closeButton)

    expect(onActivateTab).not.toHaveBeenCalled()
    expect(onCloseTab).toHaveBeenCalledWith(tabs[1])
  })

  it('does not activate the tab from keyboard events inside the close button', () => {
    const onActivateTab = vi.fn()

    renderTabController(
      <TabController
        tabs={tabs}
        activeTabKey="file:chapters/alpha.md"
        summary={null}
        onActivateTab={onActivateTab}
        onCloseTab={vi.fn()}
        onTogglePin={vi.fn()}
      />,
    )

    fireEvent.keyDown(screen.getByRole('button', { name: '关闭 beta.md' }), { key: ' ' })

    expect(onActivateTab).not.toHaveBeenCalled()
  })

  it('pins from the context menu, moves the tab forward, and persists the state', async () => {
    const user = userEvent.setup()
    const onTogglePin = vi.fn()
    renderTabController(
      <TabController
        tabs={tabs}
        activeTabKey="file:chapters/alpha.md"
        summary={null}
        onActivateTab={vi.fn()}
        onCloseTab={vi.fn()}
        onTogglePin={onTogglePin}
      />,
    )

    fireEvent.contextMenu(screen.getByRole('tab', { name: /beta.md/ }))
    await user.click(screen.getByRole('menuitem', { name: '固定标签页' }))
    expect(onTogglePin).toHaveBeenCalledWith(tabs[1])

    const pinned = setTabPinned(tabs, 'file:chapters/beta.md', true)
    expect(pinned).toEqual([{ ...tabs[1], pinned: true }, tabs[0]])
    persistTabsFor('/books/pinned', pinned)
    expect(readTabsFor('/books/pinned')).toEqual(pinned)
  })

  it('never evicts pinned tabs when applying the configured tab limit', () => {
    const pinned: Tab = { kind: 'file', path: 'chapters/pinned.md', pinned: true }
    const next: Tab = { kind: 'file', path: 'chapters/new.md' }
    const result = enforceTabLimit([pinned, tabs[0], next], 'file:chapters/new.md', 2, new Map([
      ['file:chapters/pinned.md', 1],
      ['file:chapters/alpha.md', 2],
      ['file:chapters/new.md', 3],
    ]))

    expect(result).toEqual([pinned, next])
  })
})
