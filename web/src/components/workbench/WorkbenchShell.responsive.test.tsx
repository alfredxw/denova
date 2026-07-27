import { useEffect, useState, type ReactNode } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatDateTime, setConfiguredLocale } from '@/i18n'
import { WorkbenchShell } from './WorkbenchShell'

const responsiveState = vi.hoisted(() => ({ mobile: false }))
const automationActivityApi = vi.hoisted(() => ({
  getAutomationInbox: vi.fn(),
  getActiveAutomationRuns: vi.fn(),
}))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => responsiveState.mobile,
}))

vi.mock('@/components/layout/workspace-layout', () => ({
  WorkspaceLayout: ({
    topBar,
    activityBar,
    main,
    statusBar,
    sidebarVisible,
    rightPanelVisible,
    rightPanelWide,
    centerFocus,
  }: {
    topBar: ReactNode
    activityBar: ReactNode
    main: ReactNode
    statusBar: ReactNode
    sidebarVisible: boolean
    rightPanelVisible: boolean
    rightPanelWide: boolean
    centerFocus: boolean
  }) => (
    <section
      data-testid="desktop-shell"
      data-sidebar-visible={sidebarVisible}
      data-right-panel-visible={rightPanelVisible}
      data-right-panel-wide={rightPanelWide}
      data-center-focus={centerFocus}
    >
      {topBar}{activityBar}{main}{statusBar}
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
  MessageCenterButton: () => null,
}))

vi.mock('@/lib/api', () => ({
  getAutomationInbox: automationActivityApi.getAutomationInbox,
  getActiveAutomationRuns: automationActivityApi.getActiveAutomationRuns,
}))

describe('WorkbenchShell responsive main content', () => {
  beforeEach(() => {
    responsiveState.mobile = false
    window.localStorage.clear()
    setConfiguredLocale('zh-CN')
    automationActivityApi.getAutomationInbox.mockReset().mockResolvedValue([])
    automationActivityApi.getActiveAutomationRuns.mockReset().mockResolvedValue([])
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
  })

  afterEach(() => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
  })

  it('keeps automation badge polling single-flight and pauses it while hidden', async () => {
    vi.useFakeTimers()
    render(<WorkbenchShell {...workbenchProps(<div />)} />)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(automationActivityApi.getAutomationInbox).toHaveBeenCalledTimes(1)

    const inbox = deferred<unknown[]>()
    const runs = deferred<unknown[]>()
    automationActivityApi.getAutomationInbox.mockReturnValue(inbox.promise)
    automationActivityApi.getActiveAutomationRuns.mockReturnValue(runs.promise)

    act(() => {
      vi.advanceTimersByTime(30000)
    })
    await act(async () => { await Promise.resolve() })
    expect(automationActivityApi.getAutomationInbox).toHaveBeenCalledTimes(2)

    act(() => {
      vi.advanceTimersByTime(90000)
    })
    await act(async () => { await Promise.resolve() })
    expect(automationActivityApi.getAutomationInbox).toHaveBeenCalledTimes(2)

    await act(async () => {
      Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
      document.dispatchEvent(new Event('visibilitychange'))
      inbox.resolve([])
      runs.resolve([])
      await Promise.all([inbox.promise, runs.promise])
      vi.advanceTimersByTime(30000)
    })
    expect(automationActivityApi.getAutomationInbox).toHaveBeenCalledTimes(2)
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

  it('uses the whole active menu button as the sortable drag target', () => {
    render(<WorkbenchShell {...workbenchProps(<div />)} />)

    const storyButton = screen.getByRole('button', { name: /剧情|Story/ })
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
    expect(screen.getByRole('button', { name: /剧情|Story/ })).not.toHaveAttribute('aria-current')
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
  })

  it('keeps the Workspace menu label in sync with the current language', async () => {
    render(<WorkbenchShell {...workbenchProps(<div />)} />)
    expect(screen.getByRole('button', { name: '工作台' })).toBeInTheDocument()

    await act(async () => {
      setConfiguredLocale('en-US')
      await Promise.resolve()
    })

    expect(screen.getByRole('button', { name: 'Workspace' })).toBeInTheDocument()
  })

  it.each([
    ['writing', 'nova.activity.order.ide.v2', 'ide', 'writing', ['writing', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']],
    ['game', 'nova.activity.order.interactive.v2', 'interactive', 'story', ['story', 'timeline', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']],
  ] as const)('places a newly introduced Workspace item after the %s entry', (_label, storageKey, mode, primaryID, storedOrder) => {
    window.localStorage.setItem(storageKey, JSON.stringify(storedOrder))
    const { container } = render(<WorkbenchShell {...workbenchProps(<div />)} mode={mode} booksReturnMode={mode} />)

    const activityIDs = Array.from(container.querySelectorAll('[data-activity-id]'))
      .map((element) => element.getAttribute('data-activity-id'))
    expect(activityIDs.slice(0, 2)).toEqual([primaryID, 'agentchat'])
  })

  it('shows editor updated time and line in the global bottom status bar', () => {
    const updatedAt = '2026-07-11 22:00'
    render(<WorkbenchShell {...workbenchProps(<div />)}
      mode="ide"
      presentedLayout="writing"
      currentChapter={{
        path: 'chapters/ch01.md', file_name: 'ch01.md', display_title: '第一章', index: 1,
        words: 100, status: 'draft', confirmed: false, updated_at: updatedAt,
        volume: '', volume_path: '',
      }}
      editorLine={54}
    />)

    expect(screen.getByText(`更新：${formatDateTime(updatedAt)} · 行 54`)).toBeInTheDocument()
  })
})

function workbenchProps(main: ReactNode) {
  return {
    mode: 'interactive' as const,
    presentedLayout: 'interactive' as const,
    booksReturnMode: 'interactive' as const,
    currentBookName: 'Test book',
    workspace: '/tmp/test-book',
    books: [{ name: 'Test book', path: '/tmp/test-book', author: '', last_opened_at: '' }],
    appVersion: 'test',
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

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((innerResolve) => {
    resolve = innerResolve
  })
  return { promise, resolve }
}
