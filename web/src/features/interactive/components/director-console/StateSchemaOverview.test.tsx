import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n, { setConfiguredLocale } from '@/i18n'
import { StateSchemaOverview } from './StateSchemaOverview'

const retryMock = vi.fn()
const reviewMock = vi.fn()
const skipMock = vi.fn()

const evidenceInitialization = {
  mode: 'after_opening',
  status: 'ready',
  requirements: [
    { source: { kind: 'opening', id: 'confirmed-source' }, requirement: '明确事实', evidence_kind: 'confirmed', decision: 'covered' },
    { source: { kind: 'trpg', id: 'default-source' }, requirement: '规则初始值', evidence_kind: 'default', decision: 'covered' },
    { source: { kind: 'opening', id: 'future-source' }, requirement: '未来证据类型', evidence_kind: 'future-evidence', decision: 'ignored', reason: '兼容未来值' },
  ],
}

vi.mock('../../api', () => ({
  retryInteractiveStateSchema: (...args: unknown[]) => retryMock(...args),
  reviewInteractiveStateSchema: (...args: unknown[]) => reviewMock(...args),
  skipInteractiveStateSchema: (...args: unknown[]) => skipMock(...args),
}))

describe('StateSchemaOverview', () => {
  beforeEach(() => {
    retryMock.mockReset().mockResolvedValue({ status: 'running' })
    reviewMock.mockReset().mockResolvedValue({ status: 'running' })
    skipMock.mockReset().mockResolvedValue({ status: 'skipped' })
  })

  afterEach(async () => {
    setConfiguredLocale('zh-CN')
    await i18n.changeLanguage('zh-CN')
  })

  it('shows the current revision, visible schema, adaptation changes, and warnings', () => {
    render(<StateSchemaOverview
      storyId="story-1"
      schema={{
        version: 3,
        revision: 2,
        system: {
          templates: [{ id: 'protagonist', name: '主角', fields: [
            { name: '危机压力', type: 'number', default: 1, visibility: 'visible' },
            { name: '幕后真相', type: 'string', visibility: 'hidden' },
          ] }],
          initial_actors: [{ id: 'protagonist', name: '林川', template_id: 'protagonist' }],
        },
      }}
      initialization={{
        mode: 'after_opening', status: 'ready', outcome: 'changed', target_revision: 2, lore_revision: 'lore-rev-2',
        reviewed_lore_ids: ['numeric-rule'],
        requirements: [{
          source: { kind: 'lore', id: 'numeric-rule' },
          requirement: '生命值必须保持在 0 到 100', expected_type: 'number', min: 0, max: 100,
          evidence_kind: 'inferred', decision: 'add', template_id: 'protagonist', field_id: '生命', reason: '常驻规则要求可计算生命值',
        }],
        changes: [{ kind: 'field', op: 'add', template_id: 'protagonist', target_id: '危机压力', reason: '首轮出现追捕' }],
        warnings: ['旧压力值无法转换，已使用默认值'],
      }}
    />)

    expect(screen.getByText('rev 2')).toBeInTheDocument()
    expect(screen.getByText('危机压力')).toBeInTheDocument()
    expect(screen.queryByText('幕后真相')).not.toBeInTheDocument()
    expect(screen.getByText(/首轮出现追捕/)).toBeInTheDocument()
    expect(screen.getByText('旧压力值无法转换，已使用默认值')).toBeInTheDocument()
    expect(screen.getByText('覆盖审查')).toBeInTheDocument()
    expect(screen.getByText(/生命值必须保持在 0 到 100/)).toBeInTheDocument()
    expect(screen.getByText('合理推测')).toBeInTheDocument()
    expect(screen.getByText(/protagonist\.生命/)).toBeInTheDocument()
    expect(screen.getAllByText(/numeric-rule/).length).toBeGreaterThan(0)
  })

  it('localizes known evidence kinds and renders unknown future values', () => {
    render(<StateSchemaOverview initialization={evidenceInitialization} />)

    expect(screen.getByText('已确认')).toBeInTheDocument()
    expect(screen.getByText('规则默认')).toBeInTheDocument()
    expect(screen.getByText('future-evidence')).toBeInTheDocument()
  })

  it('localizes evidence kinds in English', async () => {
    setConfiguredLocale('en-US')
    await i18n.changeLanguage('en-US')
    render(<StateSchemaOverview initialization={evidenceInitialization} />)

    expect(screen.getByText('Confirmed')).toBeInTheDocument()
    expect(screen.getByText('Rule default')).toBeInTheDocument()
  })

  it('retries or locks the preset after a failed adaptation', async () => {
    const onRefresh = vi.fn()
    render(<StateSchemaOverview storyId="story-1" initialization={{ mode: 'after_opening', status: 'failed', error: '模型不可用' }} onRefresh={onRefresh} />)

    fireEvent.click(screen.getByRole('button', { name: '重试适配' }))
    await waitFor(() => expect(retryMock).toHaveBeenCalledWith('story-1'))
    expect(onRefresh).toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '固定使用当前预设' }))
    await waitFor(() => expect(skipMock).toHaveBeenCalledWith('story-1'))
  })

  it('can explicitly review a completed state schema again', async () => {
    const onRefresh = vi.fn()
    render(<StateSchemaOverview storyId="story-1" initialization={{ mode: 'after_opening', status: 'ready', outcome: 'unchanged' }} onRefresh={onRefresh} />)

    fireEvent.click(screen.getByRole('button', { name: '重新审查' }))
    await waitFor(() => expect(reviewMock).toHaveBeenCalledWith('story-1'))
    expect(onRefresh).toHaveBeenCalled()
  })

  it('explains why a multi-branch story cannot be reviewed again', () => {
    render(<StateSchemaOverview storyId="story-1" canReview={false} initialization={{ mode: 'after_opening', status: 'ready' }} />)

    expect(screen.getByRole('button', { name: '重新审查' })).toBeDisabled()
    expect(screen.getByText('当前故事已有多个分支，暂不能安全迁移共享状态结构。')).toBeInTheDocument()
  })
})
