import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatProject } from './api'
import { AgentChatSessionSidebar } from './AgentChatSessionSidebar'

const project: AgentChatProject = {
  path: '/books/alpha',
  name: 'Alpha',
  current: true,
  total: 2,
  sessions: [
    { id: 'new', title: 'New chat', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-03-01T00:00:00Z', message_count: 2, running: false, active: false },
    { id: 'old', title: 'Old chat', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-02-01T00:00:00Z', message_count: 1, running: false, active: false },
  ],
}

function renderSidebar() {
  return render(
    <AgentChatSessionSidebar
      projects={[project]}
      loading={false}
      error=""
      activeProjectPath={project.path}
      isSessionRunning={() => false}
      onSelectProject={vi.fn()}
      onOpenSession={vi.fn()}
      onCreateSession={vi.fn()}
      onRenameSession={vi.fn()}
      onDeleteSession={vi.fn()}
    />,
  )
}

describe('AgentChatSessionSidebar', () => {
  beforeEach(() => window.localStorage.clear())

  it('toggles a project from the full project target instead of only the chevron', async () => {
    const user = userEvent.setup()
    renderSidebar()

    const projectButton = screen.getByRole('button', { name: 'Alpha' })
    expect(projectButton).toHaveAttribute('aria-expanded', 'true')
    expect(projectButton).not.toHaveAttribute('aria-disabled')
    expect(screen.getByRole('button', { name: 'New chat' })).not.toHaveAttribute('aria-disabled')

    await user.click(projectButton)
    expect(projectButton).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('button', { name: 'New chat' })).not.toBeInTheDocument()

    await user.click(projectButton)
    expect(projectButton).toHaveAttribute('aria-expanded', 'true')
  })

  it('pins projects and conversations and makes the row bodies draggable only in manual mode', async () => {
    const user = userEvent.setup()
    renderSidebar()

    await user.click(screen.getByRole('button', { name: 'Alpha 的项目操作' }))
    await user.click(await screen.findByRole('menuitem', { name: '固定项目' }))
    await user.click(screen.getByRole('button', { name: 'Alpha 的项目操作' }))
    expect(await screen.findByRole('menuitem', { name: '取消固定项目' })).toBeInTheDocument()
    await user.keyboard('{Escape}')

    await user.click(screen.getByRole('button', { name: 'New chat 的操作' }))
    await user.click(await screen.findByRole('menuitem', { name: '固定对话' }))
    await user.click(screen.getByRole('button', { name: 'New chat 的操作' }))
    expect(await screen.findByRole('menuitem', { name: '取消固定对话' })).toBeInTheDocument()
    await user.keyboard('{Escape}')

    await user.click(screen.getByRole('button', { name: '排序：最近更新' }))
    const menu = await screen.findByRole('menu')
    await user.click(within(menu).getByRole('menuitem', { name: '手动排序' }))
    expect(screen.getByRole('button', { name: 'Alpha' })).toHaveClass('cursor-grab')
    expect(screen.getByRole('button', { name: 'New chat' })).toHaveClass('cursor-grab')
    expect(screen.queryByRole('button', { name: /拖动.*排序/ })).not.toBeInTheDocument()
  })
})
