import { useEffect, useState, type ReactNode } from 'react'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
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
    expect(expandedToggle).toHaveAttribute('data-slot', 'sidebar-menu-button')
    expect(expandedToggleIconClass).not.toContain('rotate')
    expect(expandedToggleIconClass).not.toContain('transition-transform')

    rerender(<WorkbenchShell {...props} activityBarExpanded={false} />)

    const collapsedSidebar = container.querySelector('[data-slot="sidebar"]')
    const collapsedToggle = screen.getByRole('button', { name: '展开' })
    expect(collapsedSidebar).toHaveAttribute('data-state', 'collapsed')
    expect(collapsedToggle).toHaveAttribute('data-slot', 'sidebar-menu-button')
    expect(collapsedToggle.querySelector('svg')?.getAttribute('class')).toBe(expandedToggleIconClass)
  })

  it('aligns every expanded sidebar label to one fixed icon column', () => {
    const { container } = render(
      <WorkbenchShell
        {...workbenchProps(<div />)}
        mode="ide"
        presentedLayout="writing"
        booksReturnMode="ide"
        activityBarExpanded
      />,
    )

    const buttons = [
      screen.getByRole('button', { name: '写作' }),
      screen.getByRole('button', { name: /自动化$/ }),
      screen.getByRole('button', { name: '设置' }),
      screen.getByRole('button', { name: '收起' }),
    ]

    for (const button of buttons) {
      const spans = Array.from(button.children).filter((child) => child.tagName === 'SPAN')
      const iconSlot = spans.at(-2)

      expect(button).toHaveClass('gap-2')
      expect(iconSlot).toHaveClass('size-4')
    }
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

  it('exposes the selected writing/game mode to assistive technology on desktop and mobile', () => {
    const props = workbenchProps(<div />)
    const { rerender } = render(<WorkbenchShell {...props} />)

    expect(screen.getByRole('group', { name: /模式切换|Mode Switch/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /写作模式|Writing Mode/ })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: /游戏模式|Game Mode/ })).toHaveAttribute('aria-pressed', 'true')

    responsiveState.mobile = true
    rerender(<WorkbenchShell {...props} />)
    expect(screen.getByRole('button', { name: /写作模式|Writing Mode/ })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: /游戏模式|Game Mode/ })).toHaveAttribute('aria-pressed', 'true')
  })

  it('keeps the global book switcher fixed across first-level menus on desktop and mobile', () => {
    const props = {
      ...workbenchProps(<div />),
      mode: 'agentchat' as const,
      presentedLayout: 'full' as const,
      booksReturnMode: 'ide' as const,
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

  it('opens Lore from Workspace even while the previous Lore panel state is still settling', () => {
    const frames: FrameRequestCallback[] = []
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return frames.length
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const onSetMode = vi.fn()
    const onSetRightPanel = vi.fn()

    render(
      <WorkbenchShell
        {...workbenchProps(<div />)}
        mode="agentchat"
        presentedLayout="full"
        booksReturnMode="ide"
        rightPanel="lore"
        onSetMode={onSetMode}
        onSetRightPanel={onSetRightPanel}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '资料库' }))
    act(() => { frames.shift()?.(0) })
    act(() => { frames.shift()?.(16) })

    expect(onSetMode).toHaveBeenCalledWith('ide')
    expect(onSetRightPanel).toHaveBeenCalledWith('lore')

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
        rightPanel="lore"
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
        rightPanel="ai"
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
    const modeSwitch = screen.getByRole('group', { name: 'Mode Switch' })
    expect(within(modeSwitch).getByRole('button', { name: 'Writing Mode' })).toHaveTextContent('Writing')
    expect(within(modeSwitch).getByRole('button', { name: 'Game Mode' })).toHaveTextContent('Game')
  })

  it.each([
    ['writing', 'nova.activity.order.ide.v2', 'ide', 'writing', ['writing', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']],
    ['game', 'nova.activity.order.interactive.v2', 'interactive', 'story', ['story', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']],
  ] as const)('places a newly introduced Workspace item after the %s entry', (_label, storageKey, mode, primaryID, storedOrder) => {
    window.localStorage.setItem(storageKey, JSON.stringify(storedOrder))
    const { container } = render(<WorkbenchShell {...workbenchProps(<div />)} mode={mode} booksReturnMode={mode} />)

    const activityIDs = Array.from(container.querySelectorAll('[data-activity-id]'))
      .map((element) => element.getAttribute('data-activity-id'))
    expect(activityIDs.slice(0, 2)).toEqual([primaryID, 'agentchat'])
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
        booksReturnMode={mode}
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

  it('keeps the sidebar full-height while rendering document status only in Writing', () => {
    const { container, rerender } = render(
      <WorkbenchShell
        {...workbenchProps(<div />)}
        mode="ide"
        presentedLayout="writing"
        booksReturnMode="ide"
        activityBarExpanded
        summary={{
          title: 'Test book',
          author: '',
          chapter_count: 12,
          total_words: 34567,
          chapters: [],
          chapter_plans: [],
        }}
        currentChapter={{
          path: 'chapters/ch01.md',
          file_name: 'ch01.md',
          display_title: '第一章',
          index: 1,
          words: 2836,
          status: '成章',
          confirmed: true,
          updated_at: '2026-08-21T00:00:00Z',
          volume: '',
          volume_path: '',
        }}
        editorLine={42}
        isStreaming
      />,
    )

    expect(container.querySelector('.nova-topbar')).toBeNull()
    const statusBar = container.querySelector('.nova-writing-statusbar')
    expect(statusBar).toBeInTheDocument()
    expect(screen.getByRole('navigation', { name: '工作台侧边栏' })).toBeInTheDocument()
    expect(statusBar).toHaveTextContent('《Test book》 · 12 章 · 34,567 字')
    expect(statusBar).toHaveTextContent('当前：第一章 · 2,836 字')
    expect(statusBar).toHaveTextContent('更新：')
    expect(statusBar).toHaveTextContent('行 42')
    expect(statusBar).not.toHaveTextContent('成章')
    expect(screen.getByText(/更新：.*行 42/)).not.toHaveClass('ml-auto')
    expect(statusBar?.querySelectorAll('.border-l')).toHaveLength(0)
    expect(screen.getByText('生成中')).toHaveAttribute('role', 'status')
    expect(screen.getByRole('button', { name: '切换书籍，当前：Test book' })).not.toHaveTextContent('12 章')
    expect(screen.queryByText(/Denova v/)).not.toBeInTheDocument()

    rerender(
      <WorkbenchShell
        {...workbenchProps(<div />)}
        mode="agentchat"
        presentedLayout="full"
        booksReturnMode="ide"
        activityBarExpanded
      />,
    )
    expect(container.querySelector('.nova-writing-statusbar')).toBeNull()
  })
})

function workbenchProps(main: ReactNode) {
  return {
    mode: 'interactive' as const,
    presentedLayout: 'interactive' as const,
    booksReturnMode: 'interactive' as const,
    currentBookName: 'Test book',
    workspace: '/tmp/test-book',
    books: [{ project_id: 'book-test', name: 'Test book', path: '/tmp/test-book', author: '', last_opened_at: '' }],
    summary: null,
    isStreaming: false,
    projectVisible: false,
    activityBarExpanded: false,
    rightPanel: null,
    settingsOpen: false,
    interactiveSubmode: 'story' as const,
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
