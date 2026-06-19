import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StoryMemoryView } from './StoryMemoryView'
import { getStoryMemory, saveStoryMemoryStructure } from '../api'
import type { StoryMemoryState } from '../types'

vi.mock('../api', () => ({
  deleteStoryMemoryStructure: vi.fn(),
  generateStoryMemory: vi.fn(),
  getStoryMemory: vi.fn(),
  saveStoryMemoryRecord: vi.fn(),
  saveStoryMemoryStructure: vi.fn(),
  setStoryMemoryRecordArchived: vi.fn(),
  updateStoryMemorySettings: vi.fn(),
}))

const getStoryMemoryMock = vi.mocked(getStoryMemory)
const saveStoryMemoryStructureMock = vi.mocked(saveStoryMemoryStructure)

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
    expect(screen.getByRole('button', { name: '配置 Agent' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新增结构' })).toBeInTheDocument()
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

  it('saves structure and field generation instructions', async () => {
    getStoryMemoryMock.mockResolvedValue(buildState('main'))
    saveStoryMemoryStructureMock.mockImplementation(async (_storyId, input) => ({
      id: input.id || 'plot',
      name: input.name || '',
      description: input.description,
      generation_instruction: input.generation_instruction,
      mode: input.mode || 'append',
      key_field_id: input.key_field_id,
      fields: input.fields || [],
      order: input.order || 10,
    }))

    render(<StoryMemoryView storyId="story-1" branchId="main" />)

    await waitFor(() => expect(getStoryMemoryMock).toHaveBeenCalledWith('story-1', 'main', false))
    await userEvent.click(screen.getByRole('button', { name: '编辑结构' }))
    await userEvent.type(screen.getByPlaceholderText('生成要求，例如：只记录剧情已证实的信息；每条记录需要包含动机、变化原因和后续影响'), '只记录已确认事实')
    await userEvent.type(screen.getAllByPlaceholderText('字段生成要求，例如：不少于 300 字 / 不多于 300 字 / 必须包含触发事件和当前状态')[0], '不少于 300 字')
    await userEvent.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(saveStoryMemoryStructureMock).toHaveBeenCalledWith('story-1', expect.objectContaining({
        generation_instruction: '只记录已确认事实',
        fields: expect.arrayContaining([expect.objectContaining({ id: 'goal', generation_instruction: '不少于 300 字' })]),
      }))
    })
  })

  it('keeps story memory columns inside the available width before and after expansion', async () => {
    getStoryMemoryMock.mockResolvedValue(buildState('main'))

    render(<StoryMemoryView storyId="story-1" branchId="main" />)

    await waitFor(() => expect(getStoryMemoryMock).toHaveBeenCalledWith('story-1', 'main', false))
    const shell = screen.getByTestId('story-memory-table-shell')
    expect(shell).toHaveClass('overflow-x-hidden')
    expect(shell).not.toHaveClass('overflow-x-auto')
    expect(screen.getByTestId('story-memory-table')).not.toHaveClass('min-w-[980px]')

    const row = screen.getByRole('row', { name: /当前记录/ })
    await userEvent.click(within(row).getByRole('button', { name: '展开全文' }))

    const expandedGrid = screen.getByTestId('story-memory-expanded-grid')
    expect(expandedGrid).toHaveClass('grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))]')
    expect(expandedGrid).toHaveClass('overflow-hidden')
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
