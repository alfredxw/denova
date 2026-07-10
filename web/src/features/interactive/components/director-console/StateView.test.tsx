import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StateView } from './StateView'

describe('StateView', () => {
  it('renders Actor templates, fields, and visible trait snapshots without raw Actor JSON', () => {
    render(
      <StateView
        snapshot={null}
        stateFacts={[
          ['actors', {
            protagonist: {
              name: '林风',
              role: 'protagonist',
              template_id: 'cultivator',
              state: { health: 8, situation: '青石镇客栈' },
              traits: [
                {
                  pool_id: 'origin',
                  trait_id: 'ancient-bloodline',
                  name: '来自失落纪元且尚未完全觉醒的古老血脉',
                  summary: '一条足够长、用于验证窄状态卡截断展示的词条说明。',
                  visibility: 'visible',
                  source: 'template',
                  source_turn_id: 'story_create',
                },
                {
                  pool_id: 'secret',
                  trait_id: 'director-secret',
                  name: '导演隐藏词条',
                  visibility: 'hidden',
                },
              ],
            },
          }],
        ]}
      />,
    )

    const actorCard = screen.getByText('林风').closest('article')
    expect(actorCard).not.toBeNull()
    const card = within(actorCard as HTMLElement)
    expect(card.getByText('protagonist')).toBeInTheDocument()
    expect(card.getByText(/cultivator/)).toBeInTheDocument()
    expect(card.getByText('来自失落纪元且尚未完全觉醒的古老血脉')).toHaveAttribute('title', '一条足够长、用于验证窄状态卡截断展示的词条说明。')
    expect(card.queryByText('导演隐藏词条')).not.toBeInTheDocument()
    expect(card.getByText(/青石镇客栈/)).toBeInTheDocument()
    expect(screen.queryByText('actors')).not.toBeInTheDocument()
  })
})
