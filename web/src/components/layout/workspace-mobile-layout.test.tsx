import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MOBILE_NAVIGATION_OPEN_EVENT, WorkspaceMobileLayout, type MobileNavItem } from './workspace-mobile-layout'

describe('WorkspaceMobileLayout', () => {
  it('collapses mobile navigation into a bottom sheet in compact mode', async () => {
    const openLore = vi.fn()
    render(
      <WorkspaceMobileLayout
        topBar={<div>Top</div>}
        main={<div>Main</div>}
        activityItems={navItems([
          { id: 'story', label: 'Story', active: true },
          { id: 'lore', label: 'Lore', onClick: openLore },
        ])}
        settingsItem={navItem({ id: 'settings', label: 'Settings' })}
        closeLabel="Close"
        navigationLabel="Mobile navigation"
        compactNavigation
        compactNavigationLabel="Navigation"
      />,
    )

    expect(screen.queryByRole('button', { name: 'Navigation' })).not.toBeInTheDocument()
    expect(screen.queryByText('Story')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Lore' })).not.toBeInTheDocument()

    fireEvent(window, new Event(MOBILE_NAVIGATION_OPEN_EVENT))

    expect(await screen.findByRole('button', { name: 'Story' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Lore' }))
    expect(openLore).toHaveBeenCalledTimes(1)
  })

  it('keeps the full bottom navigation outside compact mode', () => {
    render(
      <WorkspaceMobileLayout
        topBar={<div>Top</div>}
        main={<div>Main</div>}
        activityItems={navItems([
          { id: 'story', label: 'Story', active: true },
          { id: 'lore', label: 'Lore' },
        ])}
        settingsItem={navItem({ id: 'settings', label: 'Settings' })}
        closeLabel="Close"
        navigationLabel="Mobile navigation"
      />,
    )

    expect(screen.getByRole('button', { name: 'Story' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Lore' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Settings' })).toBeInTheDocument()
  })
})

function navItems(items: Array<{ id: string; label: string; active?: boolean; onClick?: () => void }>): MobileNavItem[] {
  return items.map(navItem)
}

function navItem(item: { id: string; label: string; active?: boolean; onClick?: () => void }): MobileNavItem {
  return {
    ...item,
    icon: <span aria-hidden="true" />,
    onClick: item.onClick || vi.fn(),
  }
}
