import { useEffect, useState, type ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { WorkbenchShell } from './WorkbenchShell'

const responsiveState = vi.hoisted(() => ({ mobile: false }))
const useActivitySummaryMock = vi.hoisted(() => vi.fn())
const messageCenterButtonMock = vi.hoisted(() => vi.fn())

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => responsiveState.mobile,
}))

vi.mock('@/components/layout/workspace-layout', () => ({
  WorkspaceLayout: ({
    appSidebar,
    main,
    footer,
    sidebarVisible,
    rightPanelVisible,
    rightPanelWide,
    centerFocus,
    routeLayoutKey,
  }: {
    appSidebar: ReactNode
    main: ReactNode
    footer?: ReactNode
    sidebarVisible: boolean
    rightPanelVisible: boolean
    rightPanelWide: boolean
    centerFocus: boolean
    routeLayoutKey: string
  }) => (
    <section
      data-testid="desktop-shell"
      data-sidebar-visible={sidebarVisible}
      data-right-panel-visible={rightPanelVisible}
      data-right-panel-wide={rightPanelWide}
      data-center-focus={centerFocus}
      data-route-layout-key={routeLayoutKey}
    >
      {appSidebar}{main}{footer}
    </section>
  ),
}))

vi.mock('@/components/layout/workspace-mobile-layout', () => ({
  WorkspaceMobileLayout: ({ topBar, main, activityItems }: { topBar: ReactNode; main: ReactNode; activityItems: Array<{ id: string; label: string; active: boolean; onClick: () => void }> }) => (
    <section data-testid="mobile-shell">
      {topBar}
      <nav>{activityItems.map((item) => <button key={item.id} type="button" aria-pressed={item.active} onClick={item.onClick}>{item.label}</button>)}</nav>
      {main}
    </section>
  ),
}))

vi.mock('@/features/messages/MessageCenter', () => ({
  MessageCenterButton: (props: { unreadCount: number }) => {
    messageCenterButtonMock(props)
    return <span data-testid="message-center-count">{props.unreadCount}</span>
  },
}))

vi.mock('@/features/activity/use-activity-summary', () => ({
  useActivitySummary: useActivitySummaryMock,
  setActivityMessageUnreadCount: vi.fn(),
}))

describe('WorkbenchShell responsive main content', () => {
  beforeEach(() => {
    responsiveState.mobile = false
    window.localStorage.clear()
    setConfiguredLocale('zh-CN')
    useActivitySummaryMock.mockReset().mockReturnValue({
      data: {
        message_unread_count: 4,
        automation_inbox_unread_count: 3,
        automation_running_count: 1,
      },
    })
    messageCenterButtonMock.mockReset()
  })

  it('uses one compact activity summary for the message and automation badges', () => {
    render(<WorkbenchShell {...workbenchProps(<div />)} />)

    expect(useActivitySummaryMock).toHaveBeenCalled()
    expect(screen.getByTestId('message-center-count')).toHaveTextContent('4')
    expect(messageCenterButtonMock).toHaveBeenCalledWith(expect.objectContaining({ unreadCount: 4 }))
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('keeps primary menu controls geometrically stable while collapsing', () => {
    const props = workbenchProps(<div />)
    const { container, rerender } = render(<WorkbenchShell {...props} activityBarExpanded />)

    const expandedSidebar = container.querySelector('[data-slot="sidebar"]')
    const expandedToggle = screen.getByRole('button', { name: '收起' })
    const expandedToggleIconClass = expandedToggle.querySelector('svg')?.getAttribute('class')
    expect(expandedSidebar).toHaveAttribute('data-state', 'expanded')
    expect(container.querySelector('[data-slot="sidebar-container"]')).toHaveClass('border-r-0!')
    expect(expandedToggle).toHaveAttribute('data-activity-bar-toggle', 'true')
    expect(expandedToggleIconClass).not.toContain('rotate')
    expect(expandedToggleIconClass).not.toContain('transition-transform')
    expect(screen.getByRole('button', { name: '设置' })).toHaveClass('h-9')
    expect(screen.getByRole('button', { name: '写作' })).toHaveClass('h-9')

    rerender(<WorkbenchShell {...props} activityBarExpanded={false} />)

    const collapsedSidebar = container.querySelector('[data-slot="sidebar"]')
    const collapsedToggle = screen.getByRole('button', { name: '展开' })
    expect(collapsedSidebar).toHaveAttribute('data-state', 'collapsed')
    expect(collapsedToggle).toHaveAttribute('data-activity-bar-toggle', 'true')
    expect(collapsedToggle.querySelector('svg')?.getAttribute('class')).toBe(expandedToggleIconClass)
    expect(screen.getByRole('button', { name: '设置' })).toHaveClass('h-9')
    expect(screen.getByRole('button', { name: '写作' })).toHaveClass('h-9')
    expect(collapsedToggle.closest('[data-slot="sidebar-header"]')).not.toBeNull()
    expect(collapsedToggle.closest('[data-slot="sidebar-footer"]')).toBeNull()
    expect(collapsedToggle.parentElement).toHaveClass('opacity-0', 'group-hover/activity-toggle:opacity-100')
    expect(screen.getByAltText('Denova').parentElement).toHaveClass('opacity-100', 'group-hover/activity-toggle:opacity-0')
  })

  it('aligns every expanded sidebar label to one fixed icon column', () => {
    const { container } = render(
      <WorkbenchShell
        {...workbenchProps(<div />)}
        mode="ide"
        presentedLayout="writing"
        activityBarExpanded
      />,
    )

    const buttons = [
      screen.getByRole('button', { name: '写作' }),
      screen.getByRole('button', { name: /自动化$/ }),
      screen.getByRole('button', { name: '设置' }),
    ]

    for (const button of buttons) {
      const spans = Array.from(button.children).filter((child) => child.tagName === 'SPAN')
      const iconSlot = spans.at(-2)

      expect(button).toHaveClass('gap-2')
      expect(iconSlot).toHaveClass('size-4')
    }
    expect(screen.getByRole('button', { name: '收起' })).toHaveAttribute('data-activity-bar-toggle', 'true')
    expect(container.querySelector('[data-slot="sidebar-header"]')).not.toHaveClass('border-b')
    expect(container.querySelector('[data-slot="sidebar-footer"]')).not.toHaveClass('border-t')
  })

  it('keeps the main subtree mounted and preserves local state across the mobile breakpoint', () => {
    let unmountCount = 0

    function StatefulMain() {
      const [selection, setSelection] = useState('classic')
      useEffect(() => () => {
        unmountCount += 1
      }, [])

      return (
        <button type="button" onClick={() => setSelection('default-state')}>
          {selection}
        </button>
      )
    }

    const props = workbenchProps(<StatefulMain />)
    const { rerender } = render(<WorkbenchShell {...props} />)

    fireEvent.click(screen.getByRole('button', { name: 'classic' }))
    expect(screen.getByRole('button', { name: 'default-state' })).toBeInTheDocument()

    responsiveState.mobile = true
    rerender(<WorkbenchShell {...workbenchProps(<StatefulMain />)} />)

    expect(screen.getByTestId('mobile-shell')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'default-state' })).toBeInTheDocument()
    expect(unmountCount).toBe(0)
  })

  it('exposes Writing and Game as peer destinations on desktop and mobile', () => {
    const props = workbenchProps(<div />)
    const { rerender } = render(<WorkbenchShell {...props} />)

    expect(screen.queryByRole('group', { name: /模式切换|Mode Switch/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^(写作|Writing)$/ })).not.toHaveAttribute('aria-current')
    expect(screen.getByRole('button', { name: /^(游戏|Game)$/ })).toHaveAttribute('aria-current', 'page')

    responsiveState.mobile = true
    rerender(<WorkbenchShell {...props} />)
    expect(screen.getByRole('button', { name: /^(写作|Writing)$/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^(游戏|Game)$/ })).toHaveAttribute('aria-pressed', 'true')
  })

  it('keeps the global book switcher fixed across first-level menus on desktop and mobile', () => {
    const props = {
      ...workbenchProps(<div />),
      mode: 'agentchat' as const,
      presentedLayout: 'full' as const,
    }
    const { rerender } = render(<WorkbenchShell {...props} />)

    expect(screen.getByRole('button', { name: '切换书籍，当前：Test book' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /切换项目|Switch project/ })).not.toBeInTheDocument()

    responsiveState.mobile = true
    rerender(<WorkbenchShell {...props} />)
    expect(screen.getByRole('button', { name: '切换书籍，当前：Test book' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /切换项目|Switch project/ })).not.toBeInTheDocument()
  })

  it('uses the whole active menu button as the sortable drag target', () => {
    render(<WorkbenchShell {...workbenchProps(<div />)} />)

    const storyButton = screen.getByRole('button', { name: /^(游戏|Game)$/ })
    expect(storyButton.querySelector('.lucide-gamepad-2')).toBeInTheDocument()
    expect(storyButton).toHaveAttribute('aria-current', 'page')
    expect(storyButton).toHaveAttribute('aria-roledescription', 'sortable')
    expect(storyButton.querySelector('[aria-roledescription="sortable"]')).toBeNull()
  })

  it('opens sidebar customization from the sidebar context menu and persists visibility', async () => {
    const user = userEvent.setup()
    const { container } = render(<WorkbenchShell {...workbenchProps(<div />)} activityBarExpanded />)

    const sidebar = screen.getByRole('navigation', { name: '工作台侧边栏' })
    expect(sidebar).toHaveClass('select-none')
    fireEvent.contextMenu(sidebar)
    await user.click(await screen.findByRole('menuitem', { name: '自定义侧边栏' }))

    const dialog = await screen.findByRole('dialog', { name: '自定义侧边栏' })
    const loreVisibility = within(dialog).getByRole('switch', { name: '显示资料库' })
    expect(loreVisibility).toBeChecked()
    await user.click(loreVisibility)

    await waitFor(() => expect(container.querySelector('[data-activity-id="lore"]')).toBeNull())
    expect(JSON.parse(window.localStorage.getItem('nova.activity.hidden.workspace.v1') || '[]')).toContain('lore')
  })

  it('lets a first-level menu hide itself from its own context menu', async () => {
    const user = userEvent.setup()
    const { container } = render(<WorkbenchShell {...workbenchProps(<div />)} activityBarExpanded />)

    fireEvent.contextMenu(screen.getByRole('button', { name: '写作' }))
    expect(await screen.findByRole('menuitem', { name: '从侧边栏隐藏' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: '自定义侧边栏' })).toBeInTheDocument()
    await user.click(screen.getByRole('menuitem', { name: '从侧边栏隐藏' }))

    await waitFor(() => expect(container.querySelector('[data-activity-id="writing"]')).toBeNull())
    expect(JSON.parse(window.localStorage.getItem('nova.activity.hidden.workspace.v1') || '[]')).toContain('writing')
  })

  it('repairs preferences when the only unhidden menu is unavailable', async () => {
    window.localStorage.setItem('nova.activity.hidden.workspace.v1', JSON.stringify([
      'writing', 'story', 'agentchat', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations',
    ]))

    const { container } = render(<WorkbenchShell {...workbenchProps(<div />)} activityBarExpanded />)

    expect(container.querySelector('[data-activity-id="writing"]')).toBeInTheDocument()
    await waitFor(() => {
      expect(JSON.parse(window.localStorage.getItem('nova.activity.hidden.workspace.v1') || '[]')).not.toContain('writing')
    })
  })

  it('paints one optimistic menu selection before running the heavy route action', () => {
    const frames: FrameRequestCallback[] = []
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return frames.length
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const onSetMode = vi.fn()

    render(<WorkbenchShell {...workbenchProps(<div />)} onSetMode={onSetMode} />)
    fireEvent.click(screen.getByRole('button', { name: 'Skills' }))

    expect(onSetMode).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Skills' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('button', { name: /^(游戏|Game)$/ })).not.toHaveAttribute('aria-current')
    expect(document.querySelectorAll('.nova-activity-bar [aria-current="page"]')).toHaveLength(1)

    act(() => { frames.shift()?.(0) })
    expect(onSetMode).not.toHaveBeenCalled()
    act(() => { frames.shift()?.(16) })
    expect(onSetMode).toHaveBeenCalledWith('skills')

    requestFrame.mockRestore()
    cancelFrame.mockRestore()
  })

  it('opens Lore as an independent destination', () => {
    const frames: FrameRequestCallback[] = []
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return frames.length
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const onSetMode = vi.fn()

    render(
      <WorkbenchShell
        {...workbenchProps(<div />)}
        mode="agentchat"
        presentedLayout="full"
        onSetMode={onSetMode}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '资料库' }))
    act(() => { frames.shift()?.(0) })
    act(() => { frames.shift()?.(16) })

    expect(onSetMode).toHaveBeenCalledWith('lore')

    requestFrame.mockRestore()
    cancelFrame.mockRestore()
  })

  it('keeps panel geometry aligned with the route content being painted', () => {
    const main = <div />
    const { rerender } = render(
      <WorkbenchShell
        {...workbenchProps(main)}
        mode="agentchat"
        presentedLayout="writing"
        projectVisible
        rightPanelContent={<div />}
        rightPanelWide
        centerFocus
      />,
    )

    const desktopShell = screen.getByTestId('desktop-shell')
    expect(desktopShell).toHaveAttribute('data-sidebar-visible', 'true')
    expect(desktopShell).toHaveAttribute('data-right-panel-visible', 'true')
    expect(desktopShell).toHaveAttribute('data-right-panel-wide', 'true')
    expect(desktopShell).toHaveAttribute('data-center-focus', 'true')
    expect(desktopShell).toHaveAttribute('data-route-layout-key', 'writing')

    rerender(
      <WorkbenchShell
        {...workbenchProps(main)}
        mode="ide"
        presentedLayout="full"
        projectVisible
        rightPanelContent={<div />}
        rightPanelWide
        centerFocus
      />,
    )

    expect(desktopShell).toHaveAttribute('data-sidebar-visible', 'false')
    expect(desktopShell).toHaveAttribute('data-right-panel-visible', 'false')
    expect(desktopShell).toHaveAttribute('data-right-panel-wide', 'false')
    expect(desktopShell).toHaveAttribute('data-center-focus', 'false')
    expect(desktopShell).toHaveAttribute('data-route-layout-key', 'full')
  })

  it('keeps the Workspace menu label in sync with the current language', async () => {
    render(<WorkbenchShell {...workbenchProps(<div />)} activityBarExpanded />)
    expect(screen.getByRole('button', { name: '工作台' })).toBeInTheDocument()

    await act(async () => {
      setConfiguredLocale('en-US')
      await Promise.resolve()
    })

    expect(screen.getByRole('button', { name: 'Workspace' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Books' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Writing' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Game' })).toBeInTheDocument()
  })

  it('places newly introduced items using the unified Workspace order', () => {
    window.localStorage.setItem('nova.activity.order.workspace.v1', JSON.stringify(['writing', 'story', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']))
    const { container } = render(<WorkbenchShell {...workbenchProps(<div />)} />)

    const activityIDs = Array.from(container.querySelectorAll('[data-activity-id]'))
      .map((element) => element.getAttribute('data-activity-id'))
    expect(activityIDs.slice(0, 3)).toEqual(['writing', 'story', 'agentchat'])
  })

  it('keeps Trajectory hidden until developer mode is enabled', () => {
    const { rerender } = render(<WorkbenchShell {...workbenchProps(<div />)} />)
    expect(screen.queryByRole('button', { name: '轨迹' })).not.toBeInTheDocument()

    rerender(<WorkbenchShell {...workbenchProps(<div />)} developerMode />)
    expect(screen.getByRole('button', { name: '轨迹' })).toBeInTheDocument()
  })

  it.each([
    ['writing', 'ide', 'writing'],
    ['game', 'interactive', 'interactive'],
  ] as const)('opens Trajectory from %s without changing the explicit content mode', (_label, mode, presentedLayout) => {
    const frames: FrameRequestCallback[] = []
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return frames.length
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const onSetMode = vi.fn()
    const onSetInteractiveSubmode = vi.fn()

    render(
      <WorkbenchShell
        {...workbenchProps(<div />)}
        mode={mode}
        presentedLayout={presentedLayout}
        developerMode
        onSetMode={onSetMode}
        onSetInteractiveSubmode={onSetInteractiveSubmode}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '轨迹' }))
    act(() => { frames.shift()?.(0) })
    act(() => { frames.shift()?.(16) })

    expect(onSetMode).toHaveBeenCalledTimes(1)
    expect(onSetMode).toHaveBeenCalledWith('trajectory')
    expect(onSetInteractiveSubmode).not.toHaveBeenCalled()

    requestFrame.mockRestore()
    cancelFrame.mockRestore()
  })

  it('keeps the writing sidebar full-height without a footer status bar', () => {
    const { container } = render(
      <WorkbenchShell
        {...workbenchProps(<div />)}
        mode="ide"
        presentedLayout="writing"
        activityBarExpanded
        summary={{
          title: 'Test book',
          author: '',
          chapter_count: 12,
          total_words: 34567,
          chapters: [],
          chapter_plans: [],
        }}
      />,
    )

    expect(container.querySelector('.nova-topbar')).toBeNull()
    expect(screen.getByRole('navigation', { name: '工作台侧边栏' })).toBeInTheDocument()
    expect(container.querySelector('.nova-writing-statusbar')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '切换书籍，当前：Test book' })).not.toHaveTextContent('12 章')
    expect(screen.queryByText(/Denova v/)).not.toBeInTheDocument()
  })
})

function workbenchProps(main: ReactNode) {
  return {
    mode: 'interactive' as const,
    presentedLayout: 'interactive' as const,
    currentBookName: 'Test book',
    workspace: '/tmp/test-book',
    books: [{ project_id: 'book-test', name: 'Test book', path: '/tmp/test-book', author: '', last_opened_at: '' }],
    summary: null,
    projectVisible: false,
    activityBarExpanded: false,
    settingsOpen: false,
    sidebar: null,
    main,
    rightPanelContent: null,
    onSetMode: vi.fn(),
    onToggleActivityBarExpanded: vi.fn(),
    onSetInteractiveSubmode: vi.fn(),
    onSetRightPanel: vi.fn(),
    onToggleSettings: vi.fn(),
    onCloseSettings: vi.fn(),
    onQuickSwitchBook: vi.fn().mockResolvedValue(true),
  }
}
