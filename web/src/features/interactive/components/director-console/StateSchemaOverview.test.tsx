import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import i18n, { setConfiguredLocale } from '@/i18n'
import type { ActorStateField } from '../../types'
import { StateSchemaOverview } from './StateSchemaOverview'

describe('StateSchemaOverview', () => {
  afterEach(async () => {
    setConfiguredLocale('zh-CN')
    await i18n.changeLanguage('zh-CN')
  })

  it('shows every historical schema field without legacy visibility labels', () => {
    const historicalFields = [
      { name: '危机压力', type: 'number', default: 1 },
      { name: '幕后真相', type: 'string', visibility: 'hidden' },
      { name: '未来线索', type: 'string', visibility: 'spoiler' },
    ] as unknown as ActorStateField[]

    render(<StateSchemaOverview
      schema={{
        version: 3,
        revision: 2,
        system: {
          templates: [{ id: 'protagonist', name: '主角', fields: historicalFields }],
          initial_actors: [{ id: 'protagonist', name: '林川', template_id: 'protagonist' }],
        },
      }}
      initialization={{
        mode: 'adapt_template', status: 'ready', outcome: 'changed', target_revision: 2, lore_revision: 'lore-rev-2',
        reviewed_lore_ids: ['numeric-rule'],
        requirements: [{
          source: { kind: 'lore', id: 'numeric-rule' },
          requirement: '生命值必须保持在 0 到 100', expected_type: 'number', min: 0, max: 100,
          decision: 'add', template_id: 'protagonist', field_id: '生命', reason: '常驻规则要求可计算生命值',
        }],
        changes: [{ kind: 'field', op: 'add', template_id: 'protagonist', target_id: '危机压力', reason: '首轮出现追捕' }],
        warnings: ['旧压力值无法转换，已使用默认值'],
      }}
    />)

    expect(screen.getByText('rev 2')).toBeInTheDocument()
    expect(screen.getByText('危机压力')).toBeInTheDocument()
    expect(screen.getByText('幕后真相')).toBeInTheDocument()
    expect(screen.getByText('未来线索')).toBeInTheDocument()
    expect(screen.queryByText('string · hidden')).not.toBeInTheDocument()
    expect(screen.queryByText('string · spoiler')).not.toBeInTheDocument()
    expect(screen.getByText(/首轮出现追捕/)).toBeInTheDocument()
    expect(screen.getByText('旧压力值无法转换，已使用默认值')).toBeInTheDocument()
    expect(screen.getByText('覆盖审查')).toBeInTheDocument()
    expect(screen.getByText(/生命值必须保持在 0 到 100/)).toBeInTheDocument()
    expect(screen.getByText(/protagonist\.生命/)).toBeInTheDocument()
    expect(screen.getAllByText(/numeric-rule/).length).toBeGreaterThan(0)
    expect(screen.getByText('常驻规则要求可计算生命值')).toBeInTheDocument()
  })

  it('renders a frozen legacy schema without Director review actions', () => {
    render(<StateSchemaOverview initialization={{ mode: 'fixed_template', status: 'ready', outcome: 'fixed' }} />)

    expect(screen.getByText('故事状态结构')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
