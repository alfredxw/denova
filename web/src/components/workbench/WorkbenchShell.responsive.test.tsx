import { useEffect, useState, type ReactNode } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatDateTime, setConfiguredLocale } from '@/i18n'
import { WorkbenchShell } from './WorkbenchShell'
import { registerExecutableDraft, unregisterExecutableDraft, useExecutableDraftGuard } from '@/features/config-guard/executable-draft-guard'

const responsiveState = vi.hoisted(() => ({ mobile: false }))
const automationActivityApi = vi.hoisted(() => ({
  getTasks: vi.fn(),
}))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => responsiveState.mobile,
}))

vi.mock('@/components/layout/workspace-layout', () => ({
  WorkspaceLayout: ({ topBar, activityBar, main, statusBar }: { topBar: ReactNode; activityBar: ReactNode; main: ReactNode; statusBar: ReactNode }) => <section data-testid="desktop-shell">{topBar}{activityBar}{main}{statusBar}</section>,
}))

vi.mock('@/features/messages/MessageCenter', () => ({
  MessageCenterButton: () => null,
}))

vi.mock('@/lib/api', () => ({
  getTasks: automationActivityApi.getTasks,
}))

describe('WorkbenchShell responsive main content', () => {
  beforeEach(() => {
    useExecutableDraftGuard.setState({ entries: {} })
    responsiveState.mobile = false
    localStorage.clear()
    setConfiguredLocale('zh-CN')
    automationActivityApi.getTasks.mockReset().mockResolvedValue({ tasks: [], action_required_count: 0 })
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
    expect(automationActivityApi.getTasks).toHaveBeenCalledTimes(1)

    const inbox = deferred<{ tasks: unknown[]; action_required_count: number }>()
    automationActivityApi.getTasks.mockReturnValue(inbox.promise)

    act(() => {
      vi.advanceTimersByTime(30000)
    })
    await act(async () => { await Promise.resolve() })
    expect(automationActivityApi.getTasks).toHaveBeenCalledTimes(2)

    act(() => {
      vi.advanceTimersByTime(90000)
    })
    await act(async () => { await Promise.resolve() })
    expect(automationActivityApi.getTasks).toHaveBeenCalledTimes(2)

    await act(async () => {
      Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
      document.dispatchEvent(new Event('visibilitychange'))
      inbox.resolve({ tasks: [], action_required_count: 0 })
      await inbox.promise
      vi.advanceTimersByTime(30000)
    })
    expect(automationActivityApi.getTasks).toHaveBeenCalledTimes(2)
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

    expect(document.querySelector('[data-nova-mobile-shell="true"]')).toBeInTheDocument()
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

    expect(screen.queryByRole('group', { name: /模式切换|Mode Switch/ })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /更多|More/ }))
    expect(screen.getByRole('button', { name: /写作模式|Writing Mode/ })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: /游戏模式|Game Mode/ })).toHaveAttribute('aria-pressed', 'true')
  })

  it('shows editor updated time and line in the global bottom status bar', () => {
    const updatedAt = '2026-07-11 22:00'
    render(<WorkbenchShell {...workbenchProps(<div />)}
      mode="ide"
      currentChapter={{
        path: 'chapters/ch01.md', file_name: 'ch01.md', display_title: '第一章', index: 1,
        words: 100, status: 'draft', confirmed: false, updated_at: updatedAt,
        volume: '', volume_path: '',
      }}
      editorLine={54}
    />)

    expect(screen.getByText(`更新：${formatDateTime(updatedAt)} · 行 54`)).toBeInTheDocument()
  })

  it('guards closing the IDE teller panel while a preset draft is pending', async () => {
    const discard = vi.fn()
    const onSetRightPanel = vi.fn()
    registerExecutableDraft('setting-panel', { hasPending: true, discard })

    render(
      <WorkbenchShell
        {...workbenchProps(<div />)}
        mode="ide"
        rightPanel="teller"
        onSetRightPanel={onSetRightPanel}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '方案预设' }))

    expect(await screen.findByRole('alertdialog', { name: '放弃未保存的配置？' })).toBeInTheDocument()
    expect(onSetRightPanel).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '放弃修改' }))

    expect(discard).toHaveBeenCalled()
    expect(onSetRightPanel).toHaveBeenCalledWith(null)
    unregisterExecutableDraft('setting-panel')
  })
})

function workbenchProps(main: ReactNode) {
  return {
    mode: 'interactive' as const,
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
