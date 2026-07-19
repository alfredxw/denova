import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18n, { setConfiguredLocale } from '@/i18n'
import type { Snapshot, TurnEvent } from '../../types'
import { StoryStateLedger } from './StoryStateLedger'

const LONG_DETAIL_TEXT = '左臂骨裂虽然已经开始愈合，但运转灵力时仍有明显刺痛，短时间内无法再与人动手。'

function expectVitalityVisible() {
  expect(screen.getAllByText('生命').length).toBeGreaterThan(0)
}

function expectVitalityHidden() {
  expect(screen.queryAllByText('生命')).toHaveLength(0)
}

afterEach(async () => {
  cleanup()
  vi.restoreAllMocks()
  setConfiguredLocale('zh-CN')
  await i18n.changeLanguage('zh-CN')
})

describe('StoryStateLedger', () => {
  it('groups fields by shape and switches groups with tabs showing field counts', async () => {
    render(
      <StoryStateLedger
        snapshot={richStoryStateSnapshot()}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const groupTabs = screen.getByRole('tablist', { name: '状态字段分组' })
    const tabs = within(groupTabs).getAllByRole('tab')
    expect(tabs.map((tab) => tab.textContent)).toEqual(['概览4', '详情1', '持有与资源2', '隐藏信息1'])

    expect(screen.getByRole('tab', { name: /概览/ })).toHaveAttribute('aria-selected', 'true')
    expect(within(screen.getByRole('tabpanel', { name: /概览/ })).getByText('生命')).toBeInTheDocument()
    expect(screen.queryByText(LONG_DETAIL_TEXT)).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: /详情/ }))
    const detailsPanel = screen.getByRole('tabpanel', { name: /详情/ })
    expect(within(detailsPanel).getByText(LONG_DETAIL_TEXT)).toBeInTheDocument()
    expect(within(detailsPanel).queryByText('生命')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: /持有与资源/ }))
    const holdingsPanel = screen.getByRole('tabpanel', { name: /持有与资源/ })
    expect(within(holdingsPanel).getByText('敛息诀')).toBeInTheDocument()
    expect(within(holdingsPanel).getByText('下品灵石')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: /隐藏信息/ }))
    expect(within(screen.getByRole('tabpanel', { name: /隐藏信息/ })).getByText('被赵师兄盯上')).toBeInTheDocument()
  })

  it('renders a declared custom group with its template-given name', async () => {
    const snapshot = richStoryStateSnapshot()
    const template = snapshot.actor_state_schema?.system.templates?.[0]
    const actors = snapshot.state.actors as Record<string, { state?: Record<string, unknown> }>
    template?.fields?.push({ name: '称号', type: 'string', order: 36, group: '身份' })
    actors.protagonist.state!['称号'] = '外门弟子'

    render(
      <StoryStateLedger
        snapshot={snapshot}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    await userEvent.click(screen.getByRole('tab', { name: /身份/ }))
    expect(within(screen.getByRole('tabpanel', { name: /身份/ })).getByText('外门弟子')).toBeInTheDocument()
  })

  it('skips the group tab bar when all fields land in a single group', () => {
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.queryByRole('tablist', { name: '状态字段分组' })).not.toBeInTheDocument()
    const actorPanel = screen.getByRole('tabpanel', { name: /林风/ })
    expect(within(actorPanel).getByText('生命')).toBeInTheDocument()
    expect(within(actorPanel).getByText('7 / 10')).toBeInTheDocument()
    expect(within(actorPanel).getByRole('progressbar', { name: '生命：当前 7，范围 0 到 10' })).toHaveAttribute('aria-valuenow', '70')
    expect(within(actorPanel).getByText('青石镇客栈')).toBeInTheDocument()
  })

  it('keeps Actor and World State as peer tabs and hides the world tab without facts', async () => {
    const { rerender } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.getByRole('tab', { name: '林风' })).toHaveAttribute('aria-selected', 'true')
    await userEvent.click(screen.getByRole('tab', { name: '世界状态' }))
    const worldPanel = screen.getByRole('tabpanel', { name: /世界状态/ })
    expect(within(worldPanel).getByText('暴雨将至')).toBeInTheDocument()
    expect(within(worldPanel).getByText('Weather')).toBeInTheDocument()

    const withoutWorld = storyStateSnapshot()
    delete withoutWorld.state.scene
    rerender(
      <StoryStateLedger
        snapshot={withoutWorld}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(screen.queryByRole('tab', { name: '世界状态' })).not.toBeInTheDocument()
    expectVitalityVisible()
  })

  it('shows the turn delta once in the summary row plus per-field chips, not per-field notes', () => {
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.getByText('本回合 2 项变化')).toBeInTheDocument()
    expect(screen.getAllByText('-3').length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText('本回合已更新')).not.toBeInTheDocument()

    const vitalityField = screen.getAllByLabelText('生命').find((element) => element.tagName === 'SECTION')
    expect(vitalityField).toBeDefined()
    expect(within(vitalityField as HTMLElement).getByText('-3')).toBeInTheDocument()
    expect(vitalityField).toHaveAttribute('data-change-tone', 'negative')
    expect(vitalityField).toHaveAttribute('title', '受了轻伤')
  })

  it('uses the collapsed preference as a single-line default and preserves manual expansion during the same turn', async () => {
    const { rerender } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="collapsed"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expectVitalityHidden()

    await userEvent.click(screen.getByRole('button', { name: '展开状态面板' }))
    expectVitalityVisible()

    const sameTurnSnapshot = storyStateSnapshot()
    if (sameTurnSnapshot.current_turn) sameTurnSnapshot.current_turn.state_status = 'pending'
    rerender(
      <StoryStateLedger
        snapshot={sameTurnSnapshot}
        displayPreference="collapsed"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expectVitalityVisible()

    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot('turn-2')}
        displayPreference="collapsed"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expectVitalityHidden()
  })

  it('restores the visible default only when a new turn begins', async () => {
    const { rerender } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expectVitalityVisible()
    await userEvent.click(screen.getByRole('button', { name: '折叠状态面板' }))
    expectVitalityHidden()

    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expectVitalityHidden()

    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot('turn-2')}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expectVitalityVisible()
  })

  it('can hide the stage ledger while keeping the same snapshot available to the Director Console', () => {
    const { container } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="director-only"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(container).toBeEmptyDOMElement()
  })

  it('exposes the three display preferences from the stage menu', async () => {
    const onChange = vi.fn()
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="visible"
        onDisplayPreferenceChange={onChange}
      />,
    )

    await userEvent.click(screen.getByRole('button', { name: '状态显示偏好' }))
    expect(screen.getByText('默认显示')).toBeInTheDocument()
    expect(screen.getByText('默认折叠')).toBeInTheDocument()
    expect(screen.getByText('仅导演台')).toBeInTheDocument()
    expect(screen.queryByText('默认预览')).not.toBeInTheDocument()
    await userEvent.click(screen.getByText('默认折叠'))
    expect(onChange).toHaveBeenCalledWith('collapsed')
  })

  it('localizes the summary and groups in English', async () => {
    setConfiguredLocale('en-US')
    await i18n.changeLanguage('en-US')

    render(
      <StoryStateLedger
        snapshot={richStoryStateSnapshot()}
        displayPreference="visible"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.getByText('2 changes this turn')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /Overview/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /Hidden Info/ })).toBeInTheDocument()
  })
})

function storyStateSnapshot(turnId = 'turn-1'): Snapshot {
  const turn: TurnEvent = {
    id: turnId,
    parent_id: null,
    branch_id: 'main',
    ts: '2026-07-13T00:00:00Z',
    user: '推门',
    narrative: '风雨压城。',
    state_status: 'ready',
    state_delta: {
      actor_ops: [{ op: 'inc', actor_id: 'protagonist', field_id: 'vitality', value: -3, reason: '受了轻伤' }],
      ops: [{ op: 'set', path: 'scene.weather', value: '暴雨将至', reason: '天色骤暗' }],
    },
  }
  return {
    story_id: 'story',
    branch_id: 'main',
    turns: [turn],
    current_turn: turn,
    actor_state_schema: {
      version: 2,
      revision: 1,
      system: {
        templates: [{
          id: 'cultivator',
          name: '修行者',
          fields: [
            { name: '生命', id: 'vitality', type: 'number', min: 0, max: 10, order: 10 },
            { name: '灵力', id: 'spirit', type: 'number', min: 0, max: 10, order: 20 },
            { name: '年龄', id: 'age', type: 'number', order: 30 },
            { name: '当前处境', type: 'string', order: 40 },
          ],
        }],
      },
    },
    state: {
      actors: {
        protagonist: {
          name: '林风',
          role: 'protagonist',
          template_id: 'cultivator',
          state: { vitality: 7, spirit: 4, age: 23, 当前处境: '青石镇客栈' },
          traits: [{ pool_id: 'origin', trait_id: 'calm', name: '冷静', visibility: 'visible' }],
        },
        supporting: { name: '沈凝', role: 'supporting', state: { stance: '观望' } },
      },
      scene: { weather: '暴雨将至', location: '青石镇' },
    },
  }
}

function richStoryStateSnapshot(turnId = 'turn-1'): Snapshot {
  const snapshot = storyStateSnapshot(turnId)
  const template = snapshot.actor_state_schema?.system.templates?.[0]
  const actors = snapshot.state.actors as Record<string, { state?: Record<string, unknown> }>
  const protagonist = actors.protagonist
  if (!template?.fields || !protagonist.state) throw new Error('Expected Actor State fixture')
  template.fields.push(
    { name: '伤势详情', type: 'string', order: 50 },
    { name: '储物袋', type: 'object', order: 60 },
    { name: '功法', type: 'list', order: 70 },
    { name: '隐藏风险', type: 'list', visibility: 'spoiler', order: 80 },
  )
  protagonist.state['伤势详情'] = LONG_DETAIL_TEXT
  protagonist.state['储物袋'] = { 下品灵石: 9 }
  protagonist.state['功法'] = ['敛息诀']
  protagonist.state['隐藏风险'] = ['被赵师兄盯上']
  return snapshot
}
