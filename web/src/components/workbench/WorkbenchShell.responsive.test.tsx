import { useEffect, useState, type ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { WorkbenchShell } from './WorkbenchShell'

const responsiveState = vi.hoisted(() => ({ mobile: false }))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => responsiveState.mobile,
}))

vi.mock('@/components/layout/workspace-layout', () => ({
  WorkspaceLayout: ({ topBar, activityBar, main }: { topBar: ReactNode; activityBar: ReactNode; main: ReactNode }) => <section data-testid="desktop-shell">{topBar}{activityBar}{main}</section>,
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
  getAutomationInbox: vi.fn().mockResolvedValue([]),
}))

describe('WorkbenchShell responsive main content', () => {
  beforeEach(() => {
    responsiveState.mobile = false
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

  it('keeps Story Memory out of primary navigation and treats its manager as part of Story', () => {
    const props = {
      ...workbenchProps(<div />),
      interactiveSubmode: 'memory' as const,
    }
    const { rerender } = render(<WorkbenchShell {...props} />)

    expect(screen.queryByRole('button', { name: /故事记忆|Story Memory/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /剧情|Story/ })).toHaveClass('is-active')

    responsiveState.mobile = true
    rerender(<WorkbenchShell {...props} />)
    expect(screen.queryByRole('button', { name: /故事记忆|Story Memory/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /剧情|Story/ })).toHaveAttribute('aria-pressed', 'true')
  })
})

function workbenchProps(main: ReactNode) {
  return {
    mode: 'interactive' as const,
    booksReturnMode: 'interactive' as const,
    currentBookName: 'Test book',
    workspace: '/tmp/test-book',
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
  }
}
