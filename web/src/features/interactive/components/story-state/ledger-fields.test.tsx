import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { LedgerFieldView } from './ledger-fields'

describe('LedgerFieldView', () => {
  it('renders nested object properties as a readable list', () => {
    render(
      <LedgerFieldView
        item={{
          id: 'abilities',
          label: '技能与能力',
          renderer: 'object',
          change: null,
          value: {
            太初阴阳诀: {
              名称: '太初阴阳诀',
              掌握或当前状态: '已传承，炼气期',
              代价与限制: '双修对象必须自愿且符合条件',
            },
          },
        }}
      />,
    )

    expect(screen.getByRole('region', { name: '技能与能力' })).toBeInTheDocument()
    const ability = screen.getByTitle('太初阴阳诀').closest('li')
    expect(ability).not.toBeNull()
    expect(within(ability!).getAllByRole('listitem')).toHaveLength(3)
    expect(within(ability!).getByText('掌握或当前状态:')).toBeInTheDocument()
    expect(ability).toHaveTextContent('已传承，炼气期')
    expect(within(ability!).getByText('代价与限制:')).toBeInTheDocument()
  })

  it('renders signed favorability from a neutral center', () => {
    render(
      <LedgerFieldView
        item={{
          id: 'favorability',
          label: '好感度',
          field: { name: '好感度', type: 'number', min: -100, max: 100, display: 'stat' },
          renderer: 'stat',
          change: null,
          value: -40,
        }}
      />,
    )

    const meter = screen.getByRole('meter', { name: '好感度：当前 -40，范围 -100 到 100' })
    expect(meter).toHaveAttribute('aria-valuenow', '-40')
    expect(meter.querySelector('span[style*="width: 20%"]')).toHaveClass('bg-[var(--story-state-negative)]')
  })

  it('renders an unbounded stat without inventing a maximum', () => {
    render(
      <LedgerFieldView
        item={{
          id: 'level',
          label: '等级',
          field: { name: '等级', type: 'number', min: 1, display: 'stat' },
          renderer: 'stat',
          change: null,
          value: 27,
        }}
      />,
    )

    expect(screen.getByText('27')).toBeInTheDocument()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
    expect(screen.queryByRole('meter')).not.toBeInTheDocument()
  })
})
