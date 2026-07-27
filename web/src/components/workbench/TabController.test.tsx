import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { persistTabsFor, readTabsFor, TabController, type Tab } from './TabController'

const tabs: Tab[] = [
  { kind: 'file', path: 'chapters/alpha.md' },
  { kind: 'file', path: 'chapters/beta.md' },
]

describe('TabController', () => {
  it('persists Lore as a singleton workspace tab and renders its dedicated label', () => {
    window.localStorage.clear()
    persistTabsFor('/books/demo', [tabs[0], { kind: 'lore' }, { kind: 'lore' }])
    const restored = readTabsFor('/books/demo')

    expect(restored).toEqual([tabs[0], { kind: 'lore' }])
    render(
      <TabController
        tabs={restored}
        activeTabKey="lore"
        summary={null}
        onActivateTab={vi.fn()}
        onCloseTab={vi.fn()}
      />,
    )
    expect(screen.getByRole('tab', { name: /设定/ })).toHaveAttribute('title', '作品资料设定')
  })

  it('activates a tab when clicking the tab surface outside the label text', () => {
    const onActivateTab = vi.fn()

    render(
      <TabController
        tabs={tabs}
        activeTabKey="file:chapters/alpha.md"
        summary={null}
        onActivateTab={onActivateTab}
        onCloseTab={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('beta.md').closest('[role="tab"]')!)

    expect(onActivateTab).toHaveBeenCalledWith(tabs[1])
  })

  it('does not activate the tab when clicking its close button', () => {
    const onActivateTab = vi.fn()
    const onCloseTab = vi.fn()

    render(
      <TabController
        tabs={tabs}
        activeTabKey="file:chapters/alpha.md"
        summary={null}
        onActivateTab={onActivateTab}
        onCloseTab={onCloseTab}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '关闭 beta.md' }))

    expect(onActivateTab).not.toHaveBeenCalled()
    expect(onCloseTab).toHaveBeenCalledWith(tabs[1])
  })

  it('does not activate the tab from keyboard events inside the close button', () => {
    const onActivateTab = vi.fn()

    render(
      <TabController
        tabs={tabs}
        activeTabKey="file:chapters/alpha.md"
        summary={null}
        onActivateTab={onActivateTab}
        onCloseTab={vi.fn()}
      />,
    )

    fireEvent.keyDown(screen.getByRole('button', { name: '关闭 beta.md' }), { key: ' ' })

    expect(onActivateTab).not.toHaveBeenCalled()
  })
})
