import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { LedgerFieldView } from './ledger-fields'

describe('LedgerFieldView object fields', () => {
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
})
