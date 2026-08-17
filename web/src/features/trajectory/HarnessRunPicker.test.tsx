import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { HarnessRunPicker } from './HarnessRunPicker'

const runs = [
  runFixture('first', 'Long First Project Name', 'writing', 'success', 'run-alpha'),
  runFixture('second', 'Second Project', 'interactive_story', 'failed', 'run-beta'),
]

describe('HarnessRunPicker', () => {
  it('searches global Runs and keeps selection separate from viewing', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    const onView = vi.fn()
    const { rerender } = render(
      <HarnessRunPicker
        runs={runs}
        selected={new Set()}
        loading={false}
        onToggle={onToggle}
        onClear={vi.fn()}
        onView={onView}
      />,
    )

    await user.click(screen.getByRole('button', { name: '选择 Run 证据' }))
    await user.type(screen.getByPlaceholderText('搜索项目、Agent、状态或 Run ID…'), 'interactive_story failed run-beta')
    expect(screen.getByText('Second Project')).toBeInTheDocument()
    expect(screen.queryByText('Long First Project Name')).not.toBeInTheDocument()

    await user.click(screen.getByText('Second Project'))
    expect(onToggle).toHaveBeenCalledWith(runs[1].trajectory_uri)
    expect(onView).not.toHaveBeenCalled()

    rerender(
      <HarnessRunPicker
        runs={runs}
        selected={new Set([runs[1].trajectory_uri])}
        loading={false}
        onToggle={onToggle}
        onClear={vi.fn()}
        onView={onView}
      />,
    )
    expect(screen.getByText('已选择 1 条 Run')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '查看 Second Project 的轨迹' }))
    expect(onView).toHaveBeenCalledWith(runs[1].trajectory_uri)
    expect(onToggle).toHaveBeenCalledTimes(1)
  })
})

function runFixture(projectID: string, projectName: string, agentKind: string, status: string, runID: string) {
  return {
    id: runID,
    project_id: projectID,
    project_name: projectName,
    trajectory_uri: `trajectory://projects/${projectID}/runs/${runID}`,
    created_at: '2026-08-15T08:00:00Z',
    path: '',
    status,
    events: 1,
    context_parts: 0,
    agent_kind: agentKind,
  }
}
