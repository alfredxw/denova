import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import i18n, { setConfiguredLocale } from '@/i18n'
import type { StateSchemaInitializationStatus } from '../../types'
import { StateSchemaOverview } from './StateSchemaOverview'

const evidenceInitialization: StateSchemaInitializationStatus = {
  mode: 'adapt_template',
  status: 'ready',
  requirements: [
    { source: { kind: 'opening', id: 'confirmed-source' }, requirement: '明确事实', evidence_kind: 'confirmed', decision: 'covered' },
    { source: { kind: 'trpg', id: 'default-source' }, requirement: '规则初始值', evidence_kind: 'default', decision: 'covered' },
    { source: { kind: 'opening', id: 'future-source' }, requirement: '未来证据类型', evidence_kind: 'future-evidence', decision: 'ignored', reason: '兼容未来值' },
  ],
}

describe('StateSchemaOverview', () => {
  afterEach(async () => {
    setConfiguredLocale('zh-CN')
    await i18n.changeLanguage('zh-CN')
  })

  it('shows the current revision, visible schema, adaptation changes, and warnings', () => {
    render(<StateSchemaOverview
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
        mode: 'adapt_template', status: 'ready', outcome: 'changed', target_revision: 2, lore_revision: 'lore-rev-2',
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

  it('renders a frozen legacy schema without Director review actions', () => {
    render(<StateSchemaOverview initialization={{ mode: 'fixed_template', status: 'ready', outcome: 'fixed' }} />)

    expect(screen.getByText('故事状态结构')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
