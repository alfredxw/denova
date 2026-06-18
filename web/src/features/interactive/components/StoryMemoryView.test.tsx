import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StoryMemoryView } from './StoryMemoryView'
import { getStoryMemory } from '../api'
import type { StoryMemoryState } from '../types'

vi.mock('../api', () => ({
  deleteStoryMemoryStructure: vi.fn(),
  generateStoryMemory: vi.fn(),
  getStoryMemory: vi.fn(),
  saveStoryMemoryRecord: vi.fn(),
  saveStoryMemoryStructure: vi.fn(),
  setStoryMemoryRecordHidden: vi.fn(),
  updateStoryMemorySettings: vi.fn(),
}))

const getStoryMemoryMock = vi.mocked(getStoryMemory)

describe('StoryMemoryView', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('loads the current branch first and can switch the memory branch in place', async () => {
    getStoryMemoryMock.mockImplementation(async (_storyId, branchId) => buildState(branchId || 'main'))

    render(
      <StoryMemoryView
        storyId="story-1"
        branchId="br_current"
        branches={[
          { id: 'br_current', title: '当前线', head: 'turn-current-head', created_at: '2026-06-18T08:00:00Z', current: true },
          { id: 'br_alt', title: '支线', head: 'turn-alt-head', created_at: '2026-06-18T08:10:00Z', current: false },
        ]}
      />,
    )

    await waitFor(() => expect(getStoryMemoryMock).toHaveBeenCalledWith('story-1', 'br_current', false))
    expect(screen.getByRole('columnheader', { name: '目标' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: '状态' })).toBeInTheDocument()
    expect(screen.getByText('当前线 · Head turn-cur')).toBeInTheDocument()

    await userEvent.selectOptions(screen.getByRole('combobox', { name: '剧情线' }), 'br_alt')

    await waitFor(() => expect(getStoryMemoryMock).toHaveBeenLastCalledWith('story-1', 'br_alt', false))
    expect(screen.getByText('支线记录')).toBeInTheDocument()
    expect(screen.getByText('支线 · Head turn-alt')).toBeInTheDocument()

    const row = screen.getByRole('row', { name: /支线记录/ })
    await userEvent.click(within(row).getByRole('button', { name: '展开全文' }))
    expect(screen.getByRole('button', { name: '收起全文' })).toBeInTheDocument()
  })
})

function buildState(branchId: string): StoryMemoryState {
  const alt = branchId === 'br_alt'
  return {
    story_id: 'story-1',
    branch_id: branchId,
    settings: { enabled: true, auto_interval_turns: 3 },
    structures: [
      {
        id: 'plot',
        name: '剧情',
        mode: 'keyed',
        key_field_id: 'goal',
        fields: [
          { id: 'goal', name: '目标', order: 10 },
          { id: 'status', name: '状态', order: 20 },
        ],
        order: 10,
      },
    ],
    records: [
      {
        id: alt ? 'rec-alt' : 'rec-current',
        structure_id: 'plot',
        branch_id: branchId,
        key: alt ? '支线记录' : '当前记录',
        values: {
          goal: alt ? '调查另一条路' : '推进当前路线',
          status: alt ? '等待确认' : '正在推进',
        },
        created_at: '2026-06-18T08:00:00Z',
        updated_at: '2026-06-18T08:30:00Z',
      },
    ],
  }
}
