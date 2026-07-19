import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VirtuosoMockContext } from 'react-virtuoso'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DirectorPlan, RuleResolution } from '../../types'
import { DirectorBackstage } from './DirectorBackstage'

const getDirectorMock = vi.fn()

vi.mock('../../api', () => ({
  getInteractiveDirector: (...args: unknown[]) => getDirectorMock(...args),
  rebuildInteractiveDirector: vi.fn(),
  runInteractiveDirector: vi.fn(),
  updateInteractiveDirector: vi.fn(),
  analyzeInteractiveDirectorContext: vi.fn(),
  rerollInteractiveRuleResolution: vi.fn(),
}))

function samplePlan(): DirectorPlan {
  return {
    story_id: 'story',
    branch_id: 'main',
    docs: { plan: '# 第一幕\n\n主角下山。', agent_brief: '给 Agent 的简报', lore_context: '' },
    metadata: {
      version: 1,
      story_id: 'story',
      branch_id: 'main',
      revision: 'rev-1',
      branch_planning_turns: 5,
      updated_at: '2026-07-16T10:00:00Z',
      docs: { plan: { path: 'director.md', bytes: 128, hash: 'h1' } },
    },
  }
}

function renderBackstage({ storyId = 'story', snapshot = null, revealed = false }: { storyId?: string; snapshot?: Parameters<typeof DirectorBackstage>[0]['snapshot']; revealed?: boolean } = {}) {
  if (revealed) window.localStorage.setItem(`nova.directorConsole.revealed.${storyId}`, '1')
  return render(
    <VirtuosoMockContext.Provider value={{ viewportHeight: 320, itemHeight: 48 }}>
      <DirectorBackstage storyId={storyId} branchId="main" snapshot={snapshot} />
    </VirtuosoMockContext.Provider>,
  )
}

describe('DirectorBackstage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    getDirectorMock.mockReset().mockResolvedValue(samplePlan())
  })

  it('没有任何导演运行记录时不设置防剧透门，直接展示空态', () => {
    renderBackstage()

    expect(screen.getByTestId('director-run-summary')).toBeInTheDocument()
    expect(screen.queryByText('导演编排可能涉及剧透')).not.toBeInTheDocument()
    expect(screen.getByText('当前分支暂无导演编排或规则审计')).toBeInTheDocument()
    expect(screen.getByText('还没有可展示的执行过程。')).toBeInTheDocument()
    expect(screen.getByText('手动触发导演规划')).toBeInTheDocument()
  })

  it('有运行记录但未揭示时只展示概览与门，隐藏剧透内容', () => {
    renderBackstage({
      snapshot: {
        story_id: 'story', branch_id: 'main', turns: [], state: {},
        director_plan_status: { story_id: 'story', branch_id: 'main', status: 'ready', planned_docs: 3, completed_docs: 3, doc_bytes: 128, visible_bytes: 128, start_ready: true, blocking: false, updated_at: '2026-07-16T10:00:00Z' },
        director_plan: samplePlan(),
      } as never,
    })

    expect(screen.getByTestId('director-run-summary')).toBeInTheDocument()
    expect(screen.getByText('导演编排可能涉及剧透')).toBeInTheDocument()
    expect(screen.queryByText('导演节拍表')).not.toBeInTheDocument()
    expect(screen.queryByTestId('director-process-panel')).not.toBeInTheDocument()
    expect(screen.queryByText('事件编排运行态')).not.toBeInTheDocument()
  })

  it('规则审计卡不受防剧透门影响', () => {
    renderBackstage({
      snapshot: {
        story_id: 'story', branch_id: 'main', turns: [], state: {},
        director_plan: samplePlan(),
        current_turn: { id: 'turn-1', rule_resolution: { id: 'rule-1', request: { intent: '攻击', difficulty: '困难' } } as unknown as RuleResolution },
      } as never,
    })

    expect(screen.getByText('导演编排可能涉及剧透')).toBeInTheDocument()
    expect(screen.getByText('规则审计')).toBeInTheDocument()
  })

  it('揭示后展示节拍表、事件编排与执行过程，默认展开主计划文档', async () => {
    renderBackstage({
      revealed: true,
      snapshot: {
        story_id: 'story', branch_id: 'main', turns: [], state: {},
        director_plan_status: { story_id: 'story', branch_id: 'main', status: 'ready', planned_docs: 3, completed_docs: 3, doc_bytes: 128, visible_bytes: 128, start_ready: true, blocking: false, updated_at: '2026-07-16T10:00:00Z' },
        director_plan: samplePlan(),
      } as never,
    })

    expect(await screen.findByText('导演节拍表')).toBeInTheDocument()
    expect(screen.queryByText('导演编排可能涉及剧透')).not.toBeInTheDocument()
    expect(screen.getByText('事件编排运行态')).toBeInTheDocument()
    expect(screen.getByTestId('director-process-panel')).toBeInTheDocument()
    // plan 文档默认展开，其余文档折叠
    expect(screen.getByText('主角下山。')).toBeInTheDocument()
    expect(screen.queryByText('给 Agent 的简报')).not.toBeInTheDocument()
  })

  it('状态结构卡默认折叠，点击后展开结构内容', async () => {
    renderBackstage({
      snapshot: {
        story_id: 'story', branch_id: 'main', turns: [], state: {},
        actor_state_schema: {
          version: 2,
          revision: 1,
          system: { templates: [{ id: 'cultivator', name: '修行者', fields: [{ name: '身体状态', type: 'number', order: 10 }] }] },
        },
      } as never,
    })

    expect(screen.getByRole('button', { name: '故事状态结构' })).toBeInTheDocument()
    expect(screen.queryByText('当前结构')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '故事状态结构' }))
    expect(screen.getByText('当前结构')).toBeInTheDocument()
    expect(screen.getByText(/修行者/)).toBeInTheDocument()
  })

  it('旧故事的固定状态结构保持只读', async () => {
    renderBackstage({
      snapshot: {
        story_id: 'story', branch_id: 'main', turns: [], state: {},
        state_schema_initialization: { mode: 'fixed_template', status: 'ready', outcome: 'fixed' },
      } as never,
    })

    expect(screen.queryByText('固定结构')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '故事状态结构' }))
    expect(screen.getByText('本故事的状态结构已经冻结。')).toBeInTheDocument()
    expect(screen.queryByText('重试适配')).not.toBeInTheDocument()
  })
})
