import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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

function sectionLabels(container: HTMLElement) {
  const activePanel = container.querySelector<HTMLElement>('[role="tabpanel"]:not([hidden])')
  return Array.from((activePanel || container).querySelectorAll('.story-state-ledger__section'))
    .map((section) => section.getAttribute('aria-label'))
}

afterEach(async () => {
  cleanup()
  vi.restoreAllMocks()
  setConfiguredLocale('zh-CN')
  await i18n.changeLanguage('zh-CN')
})

beforeEach(() => window.localStorage.clear())

describe('StoryStateLedger', () => {
  it('lays fields out as decorated one-page sections with titled headers', () => {
    const { container } = render(
      <StoryStateLedger
        snapshot={richStoryStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(sectionLabels(container)).toEqual(['概览', '持有与资源', '详情', '隐藏信息'])
    const sections = container.querySelectorAll('.story-state-ledger__section[data-decorated]')
    expect(sections).toHaveLength(4)
    const headers = container.querySelectorAll('.story-state-ledger__section-header')
    expect(headers).toHaveLength(4)
    expect(headers[0].textContent).toContain('概览')
    expect(headers[0].textContent).toContain('4')

    // No second tab switch: all groups render their fields on one page.
    expectVitalityVisible()
    expect(screen.getByText(LONG_DETAIL_TEXT)).toBeInTheDocument()
    expect(screen.getByText('敛息诀')).toBeInTheDocument()
    expect(screen.getByText('下品灵石')).toBeInTheDocument()
    expect(screen.getByText('被赵师兄盯上')).toBeInTheDocument()
  })

  it('persists user-defined group order and can restore the schema fallback', async () => {
    const snapshot = richStoryStateSnapshot()
    const template = snapshot.actor_state_schema?.system.templates?.[0]
    const actors = snapshot.state.actors as Record<string, { state?: Record<string, unknown> }>
    template?.fields?.push(
      { name: '称号', type: 'string', order: 36, group: '身份' },
      { name: '阵营声望', type: 'string', order: 37, group: '人际' },
    )
    actors.protagonist.state!['称号'] = '外门弟子'
    actors.protagonist.state!['阵营声望'] = '冷淡'
    const { container, unmount } = render(
      <StoryStateLedger
        snapshot={snapshot}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(sectionLabels(container)).toEqual(['概览', '持有与资源', '身份', '人际', '详情', '隐藏信息'])
    expect(screen.getByText('外门弟子')).toBeInTheDocument()
    expect(screen.getByText('冷淡')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '状态显示偏好' }))
    await userEvent.click(screen.getByText('自定义布局'))
    await userEvent.click(screen.getByRole('button', { name: '下移分区：身份' }))
    await userEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(sectionLabels(container)).toEqual(['概览', '持有与资源', '人际', '身份', '详情', '隐藏信息'])

    unmount()
    const next = render(
      <StoryStateLedger
        snapshot={snapshot}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(sectionLabels(next.container)).toEqual(['概览', '持有与资源', '人际', '身份', '详情', '隐藏信息'])

    await userEvent.click(screen.getByRole('button', { name: '状态显示偏好' }))
    await userEvent.click(screen.getByText('自定义布局'))
    await userEvent.click(screen.getByRole('button', { name: '恢复默认布局' }))
    await userEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(sectionLabels(next.container)).toEqual(['概览', '持有与资源', '身份', '人际', '详情', '隐藏信息'])
  })

  it('shows only the glanceable sections in preview and expands on demand', async () => {
    const { container, rerender } = render(
      <StoryStateLedger
        snapshot={richStoryStateSnapshot()}
        displayPreference="preview"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(sectionLabels(container)).toEqual(['概览', '持有与资源'])
    expectVitalityVisible()
    expect(screen.queryByText(LONG_DETAIL_TEXT)).not.toBeInTheDocument()
    expect(screen.queryByText('被赵师兄盯上')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '展开全部（还有 2 个分区）' }))
    expect(sectionLabels(container)).toEqual(['概览', '持有与资源', '详情', '隐藏信息'])
    expect(screen.getByText(LONG_DETAIL_TEXT)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '收起为预览' }))
    expect(sectionLabels(container)).toEqual(['概览', '持有与资源'])

    // Manual expansion survives same-turn updates but resets on a new turn.
    await userEvent.click(screen.getByRole('button', { name: '展开全部（还有 2 个分区）' }))
    rerender(
      <StoryStateLedger
        snapshot={richStoryStateSnapshot()}
        displayPreference="preview"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(sectionLabels(container)).toEqual(['概览', '持有与资源', '详情', '隐藏信息'])

    rerender(
      <StoryStateLedger
        snapshot={richStoryStateSnapshot('turn-2')}
        displayPreference="preview"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(sectionLabels(container)).toEqual(['概览', '持有与资源'])
  })

  it('previews the first two ordered sections and preserves that order when expanding', async () => {
    const snapshot = storyStateSnapshot()
    const template = snapshot.actor_state_schema?.system.templates?.[0]
    const actors = snapshot.state.actors as Record<string, { state?: Record<string, unknown> }>
    if (!template || !actors.protagonist.state) throw new Error('Expected Actor State fixture')
    template.fields = [
      { name: '身份', type: 'string', group: '人物设定' },
      { name: '战斗面板', type: 'object', group: '面板' },
      { name: '即时状态', type: 'object', group: '状态' },
    ]
    actors.protagonist.state = {
      身份: '青石镇散修',
      战斗面板: { 攻击: 12 },
      即时状态: { 生命: 7 },
    }

    const { container } = render(
      <StoryStateLedger
        snapshot={snapshot}
        displayPreference="preview"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(sectionLabels(container)).toEqual(['人物设定', '面板'])
    await userEvent.click(screen.getByRole('button', { name: '展开全部（还有 1 个分区）' }))
    expect(sectionLabels(container)).toEqual(['人物设定', '面板', '状态'])
  })

  it('previews up to the first two sections when a template has no structured sections', () => {
    const snapshot = richStoryStateSnapshot()
    const template = snapshot.actor_state_schema?.system.templates?.[0]
    // Drop every overview/holdings field so only details + spoiler remain.
    if (!template) throw new Error('Expected template fixture')
    template.fields = (template.fields || []).filter((field) => ['伤势详情', '隐藏风险'].includes(field.name))

    const { container } = render(
      <StoryStateLedger
        snapshot={snapshot}
        displayPreference="preview"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(sectionLabels(container)).toEqual(['详情', '隐藏信息'])
    expect(screen.getByText(LONG_DETAIL_TEXT)).toBeInTheDocument()
    expect(screen.getByText('被赵师兄盯上')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /展开全部/ })).not.toBeInTheDocument()
  })

  it('renders nested dynamic resources, effects, and cooldowns without flattening them to blanks', () => {
    const snapshot = storyStateSnapshot()
    const template = snapshot.actor_state_schema?.system.templates?.[0]
    const actors = snapshot.state.actors as Record<string, { state?: Record<string, unknown> }>
    if (!template || !actors.protagonist.state) throw new Error('Expected Actor State fixture')
    template.fields = [{ name: '状态', type: 'object', group: '状态' }]
    actors.protagonist.state = {
      状态: {
        资源: { 生命: { 当前值: 18, 上限: 30 } },
        效果: { poison_001: { 名称: '中毒', 剩余: '3轮' } },
        冷却: { ability_fireball: { 名称: '火球术', 剩余: 2, 单位: '轮' } },
      },
    }

    render(
      <StoryStateLedger
        snapshot={snapshot}
        displayPreference="preview"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.getByText(/生命（当前值 18 · 上限 30）/)).toBeInTheDocument()
    expect(screen.getByText(/Poison 001（名称 中毒 · 剩余 3轮）/)).toBeInTheDocument()
    expect(screen.getByText(/Ability fireball（名称 火球术 · 剩余 2 · 单位 轮）/)).toBeInTheDocument()
  })

  it('skips section headers when all fields land in a single group', () => {
    const { container } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(container.querySelector('.story-state-ledger__section-header')).not.toBeInTheDocument()
    expect(container.querySelector('.story-state-ledger__section[data-decorated]')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /展开全部/ })).not.toBeInTheDocument()
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
        displayPreference="expanded"
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
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(screen.queryByRole('tab', { name: '世界状态' })).not.toBeInTheDocument()
    expectVitalityVisible()
  })

  it('mounts every entity tab body up front so switching tabs only changes visibility', () => {
    const { container } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const panels = Array.from(container.querySelectorAll<HTMLElement>('[role="tabpanel"]'))
    expect(panels).toHaveLength(3)
    expect(panels.filter((panel) => panel.hidden)).toHaveLength(2)
    expect(panels.some((panel) => panel.hidden && panel.textContent?.includes('观望'))).toBe(true)
    expect(panels.some((panel) => panel.hidden && panel.textContent?.includes('暴雨将至'))).toBe(true)
  })

  it('shows the turn delta once in the summary row plus per-field chips, not per-field notes', () => {
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.getByText('本回合 2 项变化')).toBeInTheDocument()
    expect(screen.getAllByText('-3').length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText('本回合已更新')).not.toBeInTheDocument()

    const vitalityField = screen.getAllByLabelText('生命').find((element) => element.dataset.renderer !== undefined)
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

  it('restores the expanded default only when a new turn begins', async () => {
    const { rerender } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expectVitalityVisible()
    await userEvent.click(screen.getByRole('button', { name: '折叠状态面板' }))
    expectVitalityHidden()

    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expectVitalityHidden()

    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot('turn-2')}
        displayPreference="expanded"
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

  it('exposes the four display preferences from the stage menu', async () => {
    const onChange = vi.fn()
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="preview"
        onDisplayPreferenceChange={onChange}
      />,
    )

    await userEvent.click(screen.getByRole('button', { name: '状态显示偏好' }))
    expect(screen.getByText('默认预览')).toBeInTheDocument()
    expect(screen.getByText('默认展开')).toBeInTheDocument()
    expect(screen.getByText('默认折叠')).toBeInTheDocument()
    expect(screen.getByText('仅导演台')).toBeInTheDocument()
    await userEvent.click(screen.getByText('默认折叠'))
    expect(onChange).toHaveBeenCalledWith('collapsed')
  })

  it('localizes the summary and sections in English', async () => {
    setConfiguredLocale('en-US')
    await i18n.changeLanguage('en-US')

    const { container } = render(
      <StoryStateLedger
        snapshot={richStoryStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.getByText('2 changes this turn')).toBeInTheDocument()
    expect(sectionLabels(container)).toEqual(['Overview', 'Holdings', 'Details', 'Hidden Info'])
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
