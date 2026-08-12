import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ContinualLearningPage } from './ContinualLearningPage'

const api = vi.hoisted(() => ({
  getHarnessState: vi.fn(),
  getHarnessStateVersions: vi.fn(),
  getContinualLearningSchedule: vi.fn(),
  getHarnessTrajectories: vi.fn(),
  getHarnessTrajectory: vi.fn(),
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
    api.getHarnessTrajectories.mockResolvedValue({ since: '2026-08-11T00:00:00Z', items: [] })
    api.getHarnessTrajectory.mockResolvedValue({ uri: '', kind: 'trajectory_session', content: '{}' })
    api.updateHarnessState.mockResolvedValue({ changed: true })
  })

  it('saves one State file with optimistic revision control', async () => {
    const user = userEvent.setup()
    api.getHarnessState.mockResolvedValue({
      revision: 'revision-one',
      source: 'user',
      files: [{ path: 'prompts/general.md', content: 'Lead with the result.\n' }],
    })

    render(<ContinualLearningPage scheduleSettings={scheduleSettings()} onEvidenceChange={vi.fn()} />)

    await user.click(screen.getByRole('tab', { name: 'State' }))

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
    api.getHarnessState.mockResolvedValue({ revision: 'empty-revision', files: [], source: 'builtin' })

    render(<ContinualLearningPage scheduleSettings={scheduleSettings()} onEvidenceChange={vi.fn()} />)

    await user.click(screen.getByRole('tab', { name: 'State' }))
    await screen.findByText('系统内置 Harness State')
    await user.click(screen.getByRole('button', { name: '新建 State 文件' }))

    expect(screen.getByRole('textbox', { name: '编辑 prompts/general.md' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '放弃' }))
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.getByText('系统内置 Harness State')).toBeInTheDocument()
  })

  it('does not restore a version over an unsaved State edit', async () => {
    const user = userEvent.setup()
    api.getHarnessState.mockResolvedValue({
      revision: 'current-revision',
      source: 'user',
      files: [{ path: 'prompts/general.md', content: 'Current prompt.\n' }],
    })
    api.getHarnessStateVersions.mockResolvedValue([{
      id: 'harness-state-v1:previous',
      revision: 'previous-revision',
      summary: 'Previous State',
      created_at: '2026-08-12T00:00:00Z',
    }])

    render(<ContinualLearningPage scheduleSettings={scheduleSettings()} onEvidenceChange={vi.fn()} />)

    await user.click(screen.getByRole('tab', { name: 'State' }))
    const editor = await screen.findByRole('textbox', { name: '编辑 prompts/general.md' })
    await user.type(editor, 'Unsaved')
    await user.click(screen.getByRole('tab', { name: 'Git 历史' }))
    await user.click(screen.getByRole('button', { name: '回滚' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: '恢复' }))

    expect(api.restoreHarnessStateVersion).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  it('edits scheduled optimization from the Harness optimization page', async () => {
    const user = userEvent.setup()
    const onEnabledChange = vi.fn()
    const onIntervalHoursChange = vi.fn()
    api.getHarnessState.mockResolvedValue({ revision: 'empty-revision', files: [], source: 'builtin' })

    const { rerender } = render(
      <ContinualLearningPage
        scheduleSettings={scheduleSettings({ onEnabledChange, onIntervalHoursChange })}
        onEvidenceChange={vi.fn()}
      />,
    )

    const interval = await screen.findByRole('spinbutton', { name: '优化间隔（小时）' })
    expect(interval).toBeDisabled()
    await user.click(screen.getByRole('switch', { name: '启用' }))
    expect(onEnabledChange).toHaveBeenCalledWith(true)

    rerender(
      <ContinualLearningPage
        scheduleSettings={scheduleSettings({ enabled: true, onEnabledChange, onIntervalHoursChange })}
        onEvidenceChange={vi.fn()}
      />,
    )
    const enabledInterval = screen.getByRole('spinbutton', { name: '优化间隔（小时）' })
    fireEvent.change(enabledInterval, { target: { value: '48' } })
    expect(onIntervalHoursChange).toHaveBeenLastCalledWith(48)
  })

  it('defaults trajectory analysis to the past day and selects every result', async () => {
    const onEvidenceChange = vi.fn()
    api.getHarnessState.mockResolvedValue({ revision: 'empty-revision', files: [], source: 'builtin' })
    api.getHarnessTrajectories.mockResolvedValue({
      since: '2026-08-11T00:00:00Z',
      items: [{
        uri: 'trajectory://projects/project-1/sessions/session-1',
        kind: 'session',
        project_id: 'project-1',
        project_name: 'First Book',
        id: 'session-1',
        title: 'Opening revision',
        created_at: '2026-08-12T00:00:00Z',
        updated_at: '2026-08-12T00:00:00Z',
        message_count: 4,
      }],
    })
    api.getHarnessTrajectory.mockResolvedValue({
      uri: 'trajectory://projects/project-1/sessions/session-1',
      kind: 'trajectory_session',
      content: '{"schema":"denova.trajectory.session.v1"}',
    })

    render(<ContinualLearningPage scheduleSettings={scheduleSettings()} onEvidenceChange={onEvidenceChange} />)

    expect(await screen.findByText('Opening revision')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Trajectory 时间范围' })).toHaveTextContent('过去 24 小时')
    await waitFor(() => expect(onEvidenceChange).toHaveBeenLastCalledWith(
      ['trajectory://projects/project-1/sessions/session-1'],
      true,
    ))
  })

  it('keeps optimization blocked when the trajectory catalog cannot load', async () => {
    const onEvidenceChange = vi.fn()
    api.getHarnessState.mockResolvedValue({ revision: 'empty-revision', files: [], source: 'builtin' })
    api.getHarnessTrajectories.mockRejectedValue(new Error('catalog unavailable'))

    render(<ContinualLearningPage scheduleSettings={scheduleSettings()} onEvidenceChange={onEvidenceChange} />)

    expect(await screen.findByText('catalog unavailable')).toBeInTheDocument()
    expect(onEvidenceChange).toHaveBeenLastCalledWith([], false)
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
