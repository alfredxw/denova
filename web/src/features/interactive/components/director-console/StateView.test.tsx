import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { StateView } from './StateView'

describe('StateView', () => {
  it('renders Actor templates, fields, and visible trait snapshots without raw Actor JSON', () => {
    render(
      <StateView
			snapshot={{
				story_id: 'story', branch_id: 'main', turns: [], state: {},
				actor_state_schema: {
					version: 2,
					system: { templates: [{ id: 'cultivator', name: '修行者', fields: [
						{ name: '身体状态', type: 'number', order: 10 },
						{ name: '当前处境', type: 'string', order: 20 },
						{ name: '随身物品', type: 'object', order: 30 },
					] }] },
				},
			}}
        stateFacts={[
          ['actors', {
            protagonist: {
              name: '林风',
              role: 'protagonist',
              template_id: 'cultivator',
							state: { 身体状态: 8, 当前处境: '青石镇客栈', 随身物品: { 信物: '旧玉佩', 标记: ['发光', '不可转交'] }, raw_internal_key: '不得展示' },
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

    const actorCard = screen.getByRole('heading', { name: '林风' }).closest('article')
    expect(actorCard).not.toBeNull()
    const card = within(actorCard as HTMLElement)
    expect(card.getByText('主角')).toBeInTheDocument()
    expect(card.getByText(/修行者/)).toBeInTheDocument()
    expect(card.getByText('来自失落纪元且尚未完全觉醒的古老血脉')).toHaveAttribute('title', '一条足够长、用于验证窄状态卡截断展示的词条说明。')
    expect(card.queryByText('导演隐藏词条')).not.toBeInTheDocument()
    expect(card.getByText(/青石镇客栈/)).toBeInTheDocument()
		expect(card.getByText('身体状态')).toBeInTheDocument()
		expect(card.getByText('旧玉佩')).toBeInTheDocument()
		expect((actorCard as HTMLElement).querySelector('pre')).not.toBeInTheDocument()
		expect(card.queryByText('raw_internal_key')).not.toBeInTheDocument()
		expect(card.queryByText('不得展示')).not.toBeInTheDocument()
    expect(screen.queryByText('actors')).not.toBeInTheDocument()
  })

  it('prioritizes the protagonist and switches the visible Actor sheet through tabs', async () => {
    render(
      <StateView
        snapshot={{ story_id: 'story', branch_id: 'main', turns: [], state: {} }}
        stateFacts={[[
          'actors',
          {
            supporting: { name: '沈凝', role: 'supporting', state: { stance: '观望' } },
            protagonist: { name: '林风', role: 'protagonist', state: { stance: '迎战' } },
          },
        ]]}
      />,
    )

    const actorTabs = screen.getAllByRole('tab')
    expect(actorTabs[0]).toHaveAccessibleName(/林风/)
    expect(screen.getByRole('heading', { name: '林风' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '沈凝' })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: /沈凝/ }))
    expect(screen.getByRole('heading', { name: '沈凝' })).toBeInTheDocument()
    expect(screen.getByText('观望')).toBeInTheDocument()
  })

  it('does not fall back to raw state keys when a frozen template has no visible fields', () => {
    render(
      <StateView
        snapshot={{
          story_id: 'story',
          branch_id: 'main',
          turns: [],
          state: {},
          actor_state_schema: {
            version: 2,
            system: { templates: [{ id: 'secret_actor', name: '秘密角色', fields: [{ name: '导演秘密', type: 'string', visibility: 'hidden' }] }] },
          },
        }}
        stateFacts={[['actors', { secret: { name: '无名', role: 'supporting', template_id: 'secret_actor', state: { 导演秘密: '不得泄露' } } }]]}
      />,
    )

    expect(screen.queryByText('导演秘密')).not.toBeInTheDocument()
    expect(screen.queryByText('不得泄露')).not.toBeInTheDocument()
  })
})
