import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import type { DirectorPlan, RuleResolution } from '../../types'
import { DirectorView, type DirectorViewProps } from './DirectorView'

function renderView(overrides: Partial<DirectorViewProps> = {}) {
  const props: DirectorViewProps = {
    snapshot: null,
    revealed: false,
    onReveal: vi.fn(),
    hasDirectorRun: false,
    directorPlan: null,
    draftDocs: null,
    onDraftDocsChange: vi.fn(),
    loading: false,
    running: false,
    rebuilding: false,
    saving: false,
    directorError: '',
    directorDisplayEvents: [],
    analyzing: false,
    canAnalyze: false,
    onRun: vi.fn(),
    onAnalyze: vi.fn(),
    onEvaluateEvent: vi.fn(),
    onResetEvents: vi.fn(),
    onSave: vi.fn(),
    onRebuild: vi.fn(),
    hasRuleAudit: false,
    ruleError: '',
    rerolling: false,
    onReroll: vi.fn(),
    ...overrides,
  }
  return render(
    <VirtuosoMockContext.Provider value={{ viewportHeight: 320, itemHeight: 48 }}>
      <DirectorView {...props} />
    </VirtuosoMockContext.Provider>,
  )
}

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

describe('DirectorView', () => {
  it('没有任何导演运行记录时不设置防剧透门，直接展示空态', () => {
    renderView({ hasDirectorRun: false, revealed: false })

    expect(screen.getByTestId('director-run-summary')).toBeInTheDocument()
    expect(screen.queryByText('导演编排可能涉及剧透')).not.toBeInTheDocument()
    expect(screen.getByText('当前分支暂无导演编排或规则审计')).toBeInTheDocument()
    expect(screen.getByText('还没有可展示的执行过程。')).toBeInTheDocument()
    expect(screen.getByText('手动触发导演规划')).toBeInTheDocument()
  })

  it('有运行记录但未揭示时只展示概览与门，隐藏剧透内容', () => {
    renderView({
      hasDirectorRun: true,
      revealed: false,
      directorStatus: { status: 'ready', updated_at: '2026-07-16T10:00:00Z' },
      directorPlan: samplePlan(),
      draftDocs: samplePlan().docs,
    })

    expect(screen.getByTestId('director-run-summary')).toBeInTheDocument()
    expect(screen.getByText('导演编排可能涉及剧透')).toBeInTheDocument()
    expect(screen.queryByText('导演节拍表')).not.toBeInTheDocument()
    expect(screen.queryByTestId('director-process-panel')).not.toBeInTheDocument()
    expect(screen.queryByText('事件编排运行态')).not.toBeInTheDocument()
  })

  it('揭示后展示节拍表、事件编排与执行过程，默认展开主计划文档', () => {
    renderView({
      hasDirectorRun: true,
      revealed: true,
      directorStatus: { status: 'ready', updated_at: '2026-07-16T10:00:00Z' },
      directorPlan: samplePlan(),
      draftDocs: samplePlan().docs,
    })

    expect(screen.queryByText('导演编排可能涉及剧透')).not.toBeInTheDocument()
    expect(screen.getByText('导演节拍表')).toBeInTheDocument()
    expect(screen.getByText('事件编排运行态')).toBeInTheDocument()
    expect(screen.getByTestId('director-process-panel')).toBeInTheDocument()
    // plan 文档默认展开，其余文档折叠
    expect(screen.getByText('主角下山。')).toBeInTheDocument()
    expect(screen.queryByText('给 Agent 的简报')).not.toBeInTheDocument()
  })

  it('规则审计卡不受防剧透门影响', () => {
    renderView({
      hasDirectorRun: true,
      revealed: false,
      directorStatus: { status: 'ready' },
      hasRuleAudit: true,
      ruleResolution: { id: 'rule-1', request: { intent: '攻击', difficulty: '困难' } } as unknown as RuleResolution,
    })

    expect(screen.getByText('导演编排可能涉及剧透')).toBeInTheDocument()
    expect(screen.getByText('规则审计')).toBeInTheDocument()
  })

  it('状态结构卡默认折叠，点击后展开结构内容', async () => {
    renderView({
      snapshot: {
        story_id: 'story', branch_id: 'main', turns: [], state: {},
        actor_state_schema: {
          version: 2,
          revision: 1,
          system: { templates: [{ id: 'cultivator', name: '修行者', fields: [{ name: '身体状态', type: 'number', order: 10 }] }] },
        },
      } as DirectorViewProps['snapshot'],
    })

    expect(screen.getByRole('button', { name: '故事状态结构' })).toBeInTheDocument()
    expect(screen.queryByText('当前结构')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '故事状态结构' }))
    expect(screen.getByText('当前结构')).toBeInTheDocument()
    expect(screen.getByText(/修行者/)).toBeInTheDocument()
  })

  it('状态结构初始化失败时自动展开并提示', () => {
    renderView({
      snapshot: {
        story_id: 'story', branch_id: 'main', turns: [], state: {},
        state_schema_initialization: { mode: 'after_opening', status: 'failed', error: '模型不可用' },
      } as DirectorViewProps['snapshot'],
    })

    expect(screen.getByText('模型不可用')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重试适配' })).toBeInTheDocument()
  })
})
