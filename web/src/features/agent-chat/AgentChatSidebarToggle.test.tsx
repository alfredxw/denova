import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { AgentChatSidebarToggle } from './AgentChatSidebarToggle'
import type { AgentChatActivitySidebarProps } from './AgentChatActivitySidebar'

const tree: AgentChatActivitySidebarProps = {
  projects: [], activitiesByProject: new Map(), loading: false, error: '', activeProjectId: '',
  onSelectProject: vi.fn(), onOpenActivity: vi.fn(), onOpenSession: vi.fn(),
  onRenameSession: vi.fn(), onCreateSession: vi.fn(), onOpenHistory: vi.fn(),
  onAddProject: vi.fn(), projectDirectoryBusy: false, onRenameProject: vi.fn(),
  onRelinkProject: vi.fn(), onArchiveProject: vi.fn(),
}

afterEach(() => { vi.useRealTimers() })

it('previews without pinning, allows entering the panel, and dismisses on leave or Escape', () => {
  vi.useFakeTimers()
  const onToggle = vi.fn()
  const { container } = render(<AgentChatSidebarToggle visible={false} onToggle={onToggle} tree={tree} />)
  const button = screen.getByRole('button', { name: '显示项目导航' })
  expect(screen.getAllByRole('button')).toHaveLength(1)
  fireEvent.mouseEnter(button)
  act(() => { vi.advanceTimersByTime(120) })
  const panel = container.querySelector('[data-agent-chat-sidebar-peek]')!
  expect(panel).toBeInTheDocument()
  expect(onToggle).not.toHaveBeenCalled()
  fireEvent.mouseLeave(button)
  act(() => { vi.advanceTimersByTime(80) })
  fireEvent.mouseEnter(panel)
  act(() => { vi.advanceTimersByTime(160) })
  expect(panel).toBeInTheDocument()
  fireEvent.keyDown(button, { key: 'Escape' })
  expect(panel).toHaveAttribute('inert')
  fireEvent.mouseEnter(button)
  act(() => { vi.advanceTimersByTime(120) })
  fireEvent.mouseLeave(button)
  act(() => { vi.advanceTimersByTime(160) })
  expect(container.querySelector('[data-agent-chat-sidebar-peek]:not([inert])')).toBeNull()
})

it('pins using the same button and cancels pending previews when clicked', () => {
  vi.useFakeTimers()
  const onToggle = vi.fn()
  const { container, rerender } = render(<AgentChatSidebarToggle visible={false} onToggle={onToggle} tree={tree} />)
  const button = screen.getByRole('button', { name: '显示项目导航' })
  fireEvent.mouseEnter(button)
  fireEvent.click(button)
  expect(onToggle).toHaveBeenCalledTimes(1)
  rerender(<AgentChatSidebarToggle visible onToggle={onToggle} tree={tree} />)
  act(() => { vi.advanceTimersByTime(200) })
  expect(screen.getByRole('button', { name: '隐藏项目导航' })).toBe(button)
  expect(container.querySelector('[data-agent-chat-sidebar-peek]:not([inert])')).toBeNull()
})
