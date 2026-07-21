import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { ActorStateField, Snapshot } from '../../types'
import { StateView } from './StateView'

describe('StateView', () => {
  it('renders Actor templates, fields, and historical trait snapshots without raw Actor JSON', () => {
    render(
      <StateView
        section="actors"
				snapshot={{
					story_id: 'story', branch_id: 'main', turns: [], state: {},
					actor_state_schema: {
						version: 2,
						revision: 1,
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
                  name: '旧隐藏词条',
                  visibility: 'hidden',
                },
                {
                  pool_id: 'secret',
                  trait_id: 'old-spoiler',
                  name: '旧剧透词条',
                  visibility: 'spoiler',
                },
              ],
            },
          }],
        ]}
      />,
    )

    const actorCard = screen.getByRole('article', { name: '林风' })
    const card = within(actorCard)
    expect(card.queryByText('主角')).not.toBeInTheDocument()
    expect(card.queryByText(/修行者/)).not.toBeInTheDocument()
    expect(card.getByText('来自失落纪元且尚未完全觉醒的古老血脉')).toHaveAttribute('title', '一条足够长、用于验证窄状态卡截断展示的词条说明。')
    expect(card.getByText('旧隐藏词条')).toBeInTheDocument()
    expect(card.getByText('旧剧透词条')).toBeInTheDocument()
    expect(card.getByText(/青石镇客栈/)).toBeInTheDocument()
		expect(card.getByText('身体状态')).toBeInTheDocument()
		expect(card.getByText('旧玉佩')).toBeInTheDocument()
		expect(actorCard.querySelector('pre')).not.toBeInTheDocument()
		expect(card.queryByText('raw_internal_key')).not.toBeInTheDocument()
		expect(card.queryByText('不得展示')).not.toBeInTheDocument()
    expect(screen.queryByText('actors')).not.toBeInTheDocument()
  })

  it('expands the protagonist by default and toggles actor rows inline', async () => {
    render(
      <StateView
        section="actors"
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

    // 所有角色都渲染为行，主角默认展开，其余折叠
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    expect(screen.getByRole('article', { name: '林风' })).toBeInTheDocument()
    expect(screen.getByRole('article', { name: '沈凝' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '林风' })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('迎战')).toBeInTheDocument()
    expect(screen.queryByText('观望')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '沈凝' }))
    expect(screen.getByRole('button', { name: '沈凝' })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('观望')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '沈凝' }))
    expect(screen.queryByText('观望')).not.toBeInTheDocument()
  })

  it('renders every actor as a compact row without tabs', () => {
    render(
      <StateView
        section="actors"
        snapshot={{ story_id: 'story', branch_id: 'main', turns: [], state: {} }}
        stateFacts={[['actors', {
          protagonist: { name: '林风', role: 'protagonist', state: { stance: '迎战' } },
          supporting: { name: '沈凝', role: 'supporting', state: { stance: '观望' } },
          opponent: { name: '顾临渊', role: 'opponent', state: { stance: '敌对' } },
        }]]}
      />,
    )

    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    expect(screen.getByRole('article', { name: '林风' })).toBeInTheDocument()
    expect(screen.getByRole('article', { name: '沈凝' })).toBeInTheDocument()
    expect(screen.getByRole('article', { name: '顾临渊' })).toBeInTheDocument()
    expect(screen.getByText('迎战')).toBeInTheDocument()
    expect(screen.queryByText('观望')).not.toBeInTheDocument()
    expect(screen.queryByText('敌对')).not.toBeInTheDocument()
  })

  it('shows inline meters for numeric ranged fields on collapsed actor rows', () => {
    render(
      <StateView
        section="actors"
        snapshot={{
          story_id: 'story', branch_id: 'main', turns: [], state: {},
          actor_state_schema: {
            version: 2,
            revision: 1,
            system: { templates: [{ id: 'cultivator', name: '修行者', fields: [
              { name: '生命', type: 'number', min: 0, max: 100, order: 10 },
              { name: '灵力', type: 'number', min: 0, max: 50, order: 20 },
            ] }] },
          },
        }}
        stateFacts={[['actors', {
          supporting: { name: '沈凝', role: 'supporting', template_id: 'cultivator', state: { 生命: 80, 灵力: 10 } },
          protagonist: { name: '林风', role: 'protagonist', template_id: 'cultivator', state: { 生命: 35, 灵力: 40 } },
        }]]}
      />,
    )

    // 主角行展开但 meter 也在行头；配角行折叠，行头仍可见关键数值
    const supportingRow = screen.getByRole('article', { name: '沈凝' })
    expect(supportingRow).toHaveTextContent('生命')
    expect(supportingRow).toHaveTextContent('80')
    expect(supportingRow).toHaveTextContent('灵力')
    expect(supportingRow).toHaveTextContent('10')
    expect(within(supportingRow).queryByText('状态字段')).not.toBeInTheDocument()
  })

  it('renders fields from historical schemas regardless of legacy visibility metadata', () => {
    render(
      <StateView
        section="actors"
        snapshot={{
          story_id: 'story',
          branch_id: 'main',
          turns: [],
          state: {},
          actor_state_schema: {
            version: 2,
            revision: 1,
            system: { templates: [{ id: 'secret_actor', name: '秘密角色', fields: [
              { name: '旧隐藏字段', type: 'string', visibility: 'hidden' },
              { name: '旧剧透字段', type: 'string', visibility: 'spoiler' },
            ] as unknown as ActorStateField[] }] },
          },
        }}
        stateFacts={[['actors', { secret: { name: '无名', role: 'supporting', template_id: 'secret_actor', state: { 旧隐藏字段: '仍然展示', 旧剧透字段: '普通字段' } } }]]}
      />,
    )

    expect(screen.getByText('旧隐藏字段')).toBeInTheDocument()
    expect(screen.getByText('仍然展示')).toBeInTheDocument()
    expect(screen.getByText('旧剧透字段')).toBeInTheDocument()
    expect(screen.getByText('普通字段')).toBeInTheDocument()
  })

  it('shows an empty hint instead of raw structure when there are no actors', () => {
    render(<StateView section="actors" snapshot={{ story_id: 'story', branch_id: 'main', turns: [], state: {} }} stateFacts={[]} />)

    expect(screen.getByText('当前分支暂无结构化状态')).toBeInTheDocument()
  })

  it('truncates a long turn change list and expands it on demand', async () => {
    render(
      <StateView
        section="changes"
        snapshot={{
          story_id: 'story', branch_id: 'main', turns: [], state: {},
          current_turn: {
            id: 'turn-1',
            state_delta: {
              ops: Array.from({ length: 7 }, (_, index) => ({ path: `world_flags.flag_${index}`, op: 'set', value: index })),
            },
          } as Snapshot['current_turn'],
        }}
        stateFacts={[['world_flags', { flag_0: 0 }]]}
      />,
    )

    expect(screen.getByText('World flags / Flag 0')).toBeInTheDocument()
    expect(screen.queryByText('World flags / Flag 6')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '展开全部（7）' }))
    expect(screen.getByText('World flags / Flag 6')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '收起' }))
    expect(screen.queryByText('World flags / Flag 6')).not.toBeInTheDocument()
  })

  it('shows an empty hint when the current turn has no changes', () => {
    render(<StateView section="changes" snapshot={{ story_id: 'story', branch_id: 'main', turns: [], state: {} }} stateFacts={[]} />)

    expect(screen.getByText('本回合还没有提交状态变化。')).toBeInTheDocument()
  })

  it('renders world facts in the world section and an empty hint when none exist', () => {
    const { rerender } = render(
      <StateView
        section="world"
        snapshot={{ story_id: 'story', branch_id: 'main', turns: [], state: {} }}
        stateFacts={[['world_flags', { 灵田法阵: '嗡鸣' }]]}
      />,
    )

    expect(screen.getByText('World flags')).toBeInTheDocument()
    expect(screen.getByText('嗡鸣')).toBeInTheDocument()

    rerender(<StateView section="world" snapshot={{ story_id: 'story', branch_id: 'main', turns: [], state: {} }} stateFacts={[]} />)
    expect(screen.getByText('世界与场景还没有可展示的事实。')).toBeInTheDocument()
  })
})
