import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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

    render(<ContinualLearningPage scheduleSettings={scheduleSettings()} />)

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

  it('discards a new unsaved file without leaving a phantom editor', async () => {
    const user = userEvent.setup()
    api.getHarnessState.mockResolvedValue({ revision: 'empty-revision', files: [] })

    render(<ContinualLearningPage scheduleSettings={scheduleSettings()} />)

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

    render(<ContinualLearningPage scheduleSettings={scheduleSettings()} />)

    const editor = await screen.findByRole('textbox', { name: '编辑 prompts/general.md' })
    await user.type(editor, 'Unsaved')
    await user.click(screen.getByRole('tab', { name: '版本记录' }))
    await user.click(screen.getByRole('button', { name: '恢复' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: '恢复' }))

    expect(api.restoreHarnessStateVersion).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  it('edits scheduled learning from the Continual Learning page', async () => {
    const user = userEvent.setup()
    const onEnabledChange = vi.fn()
    const onIntervalHoursChange = vi.fn()
    api.getHarnessState.mockResolvedValue({ revision: 'empty-revision', files: [] })

    const { rerender } = render(
      <ContinualLearningPage
        scheduleSettings={scheduleSettings({ onEnabledChange, onIntervalHoursChange })}
      />,
    )

    const interval = await screen.findByRole('spinbutton', { name: '学习间隔（小时）' })
    expect(interval).toBeDisabled()
    await user.click(screen.getByRole('switch', { name: '自动运行' }))
    expect(onEnabledChange).toHaveBeenCalledWith(true)

    rerender(
      <ContinualLearningPage
        scheduleSettings={scheduleSettings({ enabled: true, onEnabledChange, onIntervalHoursChange })}
      />,
    )
    const enabledInterval = screen.getByRole('spinbutton', { name: '学习间隔（小时）' })
    fireEvent.change(enabledInterval, { target: { value: '48' } })
    expect(onIntervalHoursChange).toHaveBeenLastCalledWith(48)
  })
})

function scheduleSettings(overrides: Partial<React.ComponentProps<typeof ContinualLearningPage>['scheduleSettings']> = {}) {
  return {
    enabled: null,
    inheritedEnabled: false,
    intervalHours: null,
    inheritedIntervalHours: 24,
    onEnabledChange: vi.fn(),
    onIntervalHoursChange: vi.fn(),
    ...overrides,
  }
}
