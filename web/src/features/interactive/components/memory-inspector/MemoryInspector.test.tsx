import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { MemoryLibraryView, MemorySearchResult } from '@/lib/api-client/interactive-memory'
import { browseInteractiveMemory, searchInteractiveMemory } from '@/lib/api-client/interactive-memory'
import { MemoryInspector } from './MemoryInspector'

vi.mock('@/lib/api-client/interactive-memory', () => ({
  browseInteractiveMemory: vi.fn(),
  searchInteractiveMemory: vi.fn(),
}))

const STORY = 'story-1'
const BRANCH = 'main'

function libraryView(overrides: Partial<MemoryLibraryView> = {}): MemoryLibraryView {
  return {
    story_id: STORY,
    branch_id: BRANCH,
    entries: [
      {
        id: 'rec-1',
        kind: 'object_state',
        subject: '蚀骨剑',
        object: '林舟',
        text: '蚀骨剑在林舟手中。',
        evidence: '得到蚀骨剑',
        valid_from: 'turn-1',
        event_id: 'ev-1',
        epoch: 1,
        source_turn_id: 'turn-1',
        ts: '2026-08-01T00:00:00Z',
      },
    ],
    stats: {
      total_turns: 4,
      turns_with_memory: 2,
      coverage: 0.5,
      events: 2,
      records: 1,
      kind_counts: { object_state: 1 },
      open_promises: 0,
      paid_promises: 0,
      expired_records: 0,
    },
    ...overrides,
  }
}

function searchResult(overrides: Partial<MemorySearchResult> = {}): MemorySearchResult {
  return {
    story_id: STORY,
    branch_id: BRANCH,
    match: 'any',
    limit: 8,
    truncated: false,
    vector_enabled: false,
    hits: [
      {
        record_id: 'rec-1',
        kind: 'object_state',
        subject: '蚀骨剑',
        text: '蚀骨剑在林舟手中。',
        evidence: '得到蚀骨剑',
        valid_from: 'turn-1',
        score: 110,
      },
    ],
    explain: {
      pipeline: {
        projected_events: 3,
        deduped_records: 3,
        stale_records: 0,
        valid_records: 3,
        expired_records: 0,
        candidates: 3,
        keyword_matched: 1,
        vector_candidates: 0,
        fused_ranked: 0,
        anchors: 0,
        expanded_records: 0,
        expanded_hops: 1,
        final_after_budget: 1,
      },
    },
    ...overrides,
  }
}

async function renderDebugTab() {
  render(<MemoryInspector storyId={STORY} branchId={BRANCH} />)
  await screen.findByTestId('memory-library')
  fireEvent.click(screen.getByTestId('memory-tab-debug'))
  fireEvent.click(screen.getByRole('button', { name: '检索' }))
  return screen.findByTestId('memory-pipeline')
}

describe('MemoryInspector', () => {
  beforeEach(() => {
    vi.mocked(browseInteractiveMemory).mockReset()
    vi.mocked(searchInteractiveMemory).mockReset()
    vi.mocked(browseInteractiveMemory).mockResolvedValue(libraryView())
    vi.mocked(searchInteractiveMemory).mockResolvedValue(searchResult())
  })

  it('挂载后拉取记忆库并渲染条目', async () => {
    render(<MemoryInspector storyId={STORY} branchId={BRANCH} />)

    expect(await screen.findByTestId('memory-library')).toBeInTheDocument()
    expect(screen.getAllByTestId('memory-entry')).toHaveLength(1)
    expect(screen.getByText('蚀骨剑在林舟手中。')).toBeInTheDocument()
    expect(browseInteractiveMemory).toHaveBeenCalledWith(STORY, { branch: BRANCH, kind: undefined })
  })

  it('把加载失败显示为错误而不是静默空列表', async () => {
    vi.mocked(browseInteractiveMemory).mockRejectedValueOnce(new Error('网络挂了'))
    render(<MemoryInspector storyId={STORY} branchId={BRANCH} />)
    expect(await screen.findByText('网络挂了')).toBeInTheDocument()
  })

  // 向量未启用时不该出现向量/融合两行,否则用户会以为混合召回在工作。
  it('向量未启用时隐藏向量与融合的流水线行', async () => {
    await renderDebugTab()

    expect(screen.queryByText('向量召回')).not.toBeInTheDocument()
    expect(screen.queryByText('RRF 融合')).not.toBeInTheDocument()
    expect(screen.getByText('关键词命中')).toBeInTheDocument()
  })

  it('向量启用时渲染融合明细与两路名次', async () => {
    vi.mocked(searchInteractiveMemory).mockResolvedValue(
      searchResult({
        vector_enabled: true,
        explain: {
          pipeline: {
            ...searchResult().explain!.pipeline,
            vector_candidates: 2,
            fused_ranked: 3,
          },
          hit_details: [
            {
              record_id: 'rec-1',
              keyword_rank: 2,
              vector_rank: 1,
              vector_score: 0.8123,
              fused_score: 0.0325,
            },
          ],
        },
      }),
    )

    await renderDebugTab()

    expect(screen.getByText('向量召回')).toBeInTheDocument()
    expect(screen.getByText('RRF 融合')).toBeInTheDocument()
    // 命中徽标带名次与相似度,融合分单独一行。
    expect(screen.getByText(/向量 #1 · 0\.812/)).toBeInTheDocument()
    expect(screen.getByText(/融合分 0\.0325/)).toBeInTheDocument()
    expect(screen.getByText(/关键词 #2/)).toBeInTheDocument()
  })

  // 多跳展开的价值全在"为什么这条会在结果里",所以路径必须展示出来。
  it('多跳展开的命中显示实体路径与跳数', async () => {
    vi.mocked(searchInteractiveMemory).mockResolvedValue(
      searchResult({
        hits: [
          {
            record_id: 'rec-far',
            kind: 'relationship',
            subject: '丙',
            object: '丁',
            text: '丙受丁差遣。',
            evidence: '差遣',
            valid_from: 'turn-1',
            score: 20,
            expanded_from: '丙',
            expanded_hop: 2,
            expanded_path: ['乙', '丙'],
          },
        ],
      }),
    )

    await renderDebugTab()

    expect(screen.getByText(/展开自 乙 → 丙/)).toBeInTheDocument()
    expect(screen.getByText(/2 跳/)).toBeInTheDocument()
  })

  it('一跳展开只显示锚点实体,不显示跳数', async () => {
    vi.mocked(searchInteractiveMemory).mockResolvedValue(
      searchResult({
        hits: [
          {
            record_id: 'rec-near',
            kind: 'beat',
            subject: '岚',
            text: '岚的戏剧功能。',
            evidence: '守护',
            valid_from: 'turn-1',
            score: 40,
            expanded_from: '岚',
            expanded_hop: 1,
            expanded_path: ['岚'],
          },
        ],
      }),
    )

    await renderDebugTab()

    expect(screen.getByText(/展开自 岚/)).toBeInTheDocument()
    expect(screen.queryByText(/1 跳/)).not.toBeInTheDocument()
  })

  // 对齐层会静默改写模型输出的实体写法,面板必须让这件事可见。
  it('健康页展示实体对齐的逐条改写', async () => {
    vi.mocked(browseInteractiveMemory).mockResolvedValue(
      libraryView({
        stats: {
          ...libraryView().stats,
          last_publish: {
            model: 'test-model',
            duration_ms: 1200,
            aligned_entities: [
              { record_id: 'rec-1', field: 'subject', from: '蚀 骨 剑', to: '蚀骨剑' },
            ],
          },
        },
      }),
    )

    render(<MemoryInspector storyId={STORY} branchId={BRANCH} />)
    await screen.findByTestId('memory-library')
    fireEvent.click(screen.getByTestId('memory-tab-health'))

    expect(await screen.findByTestId('memory-health')).toBeInTheDocument()
    expect(screen.getByText(/实体对齐: 1/)).toBeInTheDocument()
    expect(screen.getByText(/subject · 蚀 骨 剑 → 蚀骨剑/)).toBeInTheDocument()
  })

  it('没有对齐改写时不显示该段', async () => {
    render(<MemoryInspector storyId={STORY} branchId={BRANCH} />)
    await screen.findByTestId('memory-library')
    fireEvent.click(screen.getByTestId('memory-tab-health'))

    await screen.findByTestId('memory-health')
    expect(screen.queryByText(/实体对齐/)).not.toBeInTheDocument()
  })

  it('刷新按钮重新拉取记忆库', async () => {
    render(<MemoryInspector storyId={STORY} branchId={BRANCH} />)
    await screen.findByTestId('memory-library')

    fireEvent.click(screen.getByRole('button', { name: '刷新' }))
    await waitFor(() => expect(browseInteractiveMemory).toHaveBeenCalledTimes(2))
  })
})
