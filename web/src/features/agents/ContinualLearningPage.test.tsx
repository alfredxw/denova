import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ContinualLearningPage } from './ContinualLearningPage'

const api = vi.hoisted(() => ({
  getHarnessState: vi.fn(),
  getHarnessStateVersions: vi.fn(),
  getContinualLearningSchedule: vi.fn(),
  getHarnessStateVersionDiff: vi.fn(),
  restoreHarnessStateVersion: vi.fn(),
  updateHarnessState: vi.fn(),
}))

vi.mock('@/lib/api', () => api)

describe('ContinualLearningPage', () => {
  beforeEach(() => {
    Object.values(api).forEach((mock) => mock.mockReset())
    api.getHarnessStateVersions.mockResolvedValue([])
    api.getContinualLearningSchedule.mockResolvedValue({ enabled: false, interval_hours: 24 })
    api.updateHarnessState.mockResolvedValue({ changed: true })
  })

  it('saves one State file with optimistic revision control', async () => {
    const user = userEvent.setup()
    api.getHarnessState.mockResolvedValue({
      revision: 'revision-one',
      files: [{ path: 'prompts/general.md', content: 'Lead with the result.\n' }],
    })

    render(<ContinualLearningPage />)

    const editor = await screen.findByRole('textbox', { name: '编辑 prompts/general.md' })
    await user.clear(editor)
    await user.type(editor, 'Prefer concise evidence.')
    await user.click(screen.getByRole('button', { name: '保存改动' }))

    await waitFor(() => {
      expect(api.updateHarnessState).toHaveBeenCalledWith({
        base_revision: 'revision-one',
        summary: 'Update prompts/general.md',
        changes: [{ path: 'prompts/general.md', content: 'Prefer concise evidence.' }],
      })
    })
  })

  it('discards a new unpublished file without leaving a phantom editor', async () => {
    const user = userEvent.setup()
    api.getHarnessState.mockResolvedValue({ revision: 'empty-revision', files: [] })

    render(<ContinualLearningPage />)

    await screen.findByText('还没有 State 文件')
    await user.click(screen.getByRole('button', { name: '新建 State 文件' }))
    await user.click(await screen.findByRole('menuitem', { name: '上下文片段' }))

    expect(screen.getByRole('textbox', { name: '编辑 context/preference-1.md' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '放弃' }))
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.getByText('未选择文件')).toBeInTheDocument()
  })

  it('does not restore a version over an unsaved State edit', async () => {
    const user = userEvent.setup()
    api.getHarnessState.mockResolvedValue({
      revision: 'current-revision',
      files: [{ path: 'prompts/general.md', content: 'Current prompt.\n' }],
    })
    api.getHarnessStateVersions.mockResolvedValue([{
      id: 'harness-state-v1:previous',
      revision: 'previous-revision',
      summary: 'Previous State',
      created_at: '2026-08-12T00:00:00Z',
    }])

    render(<ContinualLearningPage />)

    const editor = await screen.findByRole('textbox', { name: '编辑 prompts/general.md' })
    await user.type(editor, 'Unsaved')
    await user.click(screen.getByRole('tab', { name: '版本记录' }))
    await user.click(screen.getByRole('button', { name: '恢复' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: '恢复' }))

    expect(api.restoreHarnessStateVersion).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })
})
