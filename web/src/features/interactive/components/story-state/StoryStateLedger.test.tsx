import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18n, { setConfiguredLocale } from '@/i18n'
import type { Snapshot, TurnEvent } from '../../types'
import { StoryStateLedger } from './StoryStateLedger'

function isVisibleElement(element: HTMLElement) {
  const closestHidden = element.closest('[aria-hidden="true"], .invisible')
  return closestHidden === null
}

function visibleText(text: string) {
  return screen.getAllByText(text).find(isVisibleElement)
}

afterEach(async () => {
  cleanup()
  vi.restoreAllMocks()
  setConfiguredLocale('zh-CN')
  await i18n.changeLanguage('zh-CN')
})

describe('StoryStateLedger', () => {
  it('keeps Actor and World State as peer tabs when the stage panel is open', async () => {
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const tabs = screen.getByRole('tablist', { name: '当前状态对象' })
    expect(tabs).toBeInTheDocument()
    expect(tabs).toHaveClass('story-state-ledger__tabs-list')
    expect(screen.getByRole('tab', { name: '林风' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: '世界状态' })).toBeInTheDocument()
    expect(visibleText('青石镇客栈')).toBeInTheDocument()
    expect(screen.queryByText('本回合变化')).not.toBeInTheDocument()
    expect(within(screen.getByRole('tabpanel', { name: '林风' })).getByText('-3')).toBeInTheDocument()
    expect(within(screen.getByRole('tabpanel', { name: '林风' })).getByText('受了轻伤')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: '世界状态' }))
    expect(screen.getByRole('tab', { name: '世界状态' })).toHaveAttribute('aria-selected', 'true')
    expect(visibleText('暴雨将至')).toBeInTheDocument()
    expect(screen.queryByText('青石镇客栈')).not.toBeInTheDocument()
    expect(screen.getByRole('tablist', { name: '结构化状态' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Scene' })).toHaveAttribute('aria-selected', 'true')
    expect(within(screen.getByRole('tabpanel', { name: 'Scene' })).getByText('暴雨将至')).toBeInTheDocument()
  })

  it('keeps turn changes inside their fields instead of rendering a separate change module', async () => {
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.queryByText('本回合变化')).not.toBeInTheDocument()
    expect(screen.getAllByText('7 / 10').filter(isVisibleElement).length).toBeGreaterThanOrEqual(1)
    expect(visibleText('生命')).toBeInTheDocument()
    const vitalityMetric = visibleText('生命')?.closest('[data-state-metric]')
    expect(vitalityMetric).not.toBeNull()
    expect(within(vitalityMetric as HTMLElement).getByText('-3')).toBeInTheDocument()
    expect(within(vitalityMetric as HTMLElement).getByText('受了轻伤')).toBeInTheDocument()
    expect(screen.queryByText('Weather')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: '世界状态' }))
    const sceneField = screen.getByRole('tabpanel', { name: 'Scene' })
    expect(within(sceneField).getAllByText('Weather').length).toBeGreaterThanOrEqual(1)
    expect(within(sceneField).getByText('天色骤暗')).toBeInTheDocument()
    expect(screen.queryByText('生命')).not.toBeInTheDocument()
  })

  it('groups bounded numeric fields into parallel progress bars while keeping unbounded numbers in the detail grid', () => {
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const metrics = screen.getByRole('group', { name: '数值状态' })
    expect(metrics).toHaveClass('story-state-ledger__metric-grid')
    expect(metrics.querySelectorAll('[data-state-metric]')).toHaveLength(2)

    const vitalityMetric = within(metrics).getByText('生命').closest('[data-state-metric]')
    expect(vitalityMetric).not.toBeNull()
    expect(within(vitalityMetric as HTMLElement).getByText('7 / 10')).toBeInTheDocument()
    expect(within(vitalityMetric as HTMLElement).getByRole('progressbar', {
      name: '生命：当前 7，范围 0 到 10',
    })).toHaveAttribute('aria-valuenow', '70')
    expect(within(metrics).getByText('灵力')).toBeInTheDocument()

    const ageField = visibleText('年龄')?.closest('[data-state-field]')
    expect(ageField).not.toBeNull()
    expect(within(ageField as HTMLElement).queryByRole('progressbar')).not.toBeInTheDocument()
    expect(visibleText('当前处境')?.closest('[data-state-field]')).not.toBeNull()
  })

  it('packs incomplete metric rows and places each structured field in a switchable sub-tab', async () => {
    const snapshot = storyStateSnapshot()
    const template = snapshot.actor_state_schema?.system.templates?.[0]
    const actors = snapshot.state.actors as Record<string, { state?: Record<string, unknown> }> | undefined
    const protagonist = actors?.protagonist
    const fields = template?.fields
    if (!fields || !protagonist?.state) throw new Error('Expected Actor State fixture')

    fields.push(
      { name: '神识', id: 'sense', type: 'number', min: 0, max: 100, order: 21 },
      { name: '精血', id: 'essence', type: 'number', min: 0, max: 100, order: 22 },
      { name: '道心', id: 'resolve', type: 'number', min: 0, max: 100, order: 23 },
      { name: '修为', id: 'cultivation', type: 'number', min: 0, max: 100, order: 24 },
      { name: '宗门贡献', id: 'contribution', type: 'number', min: 0, max: 100, order: 25 },
      { name: '宗门', id: 'sect', type: 'string', order: 31 },
      { name: '储物袋', id: 'inventory', type: 'object', order: 50 },
      { name: '功法', id: 'techniques', type: 'object', order: 60 },
    )
    protagonist.state.sense = 80
    protagonist.state.essence = 95
    protagonist.state.resolve = 45
    protagonist.state.cultivation = 0
    protagonist.state.contribution = 0
    protagonist.state.sect = '散修'
    protagonist.state['当前处境'] = '灵基刻痛已减轻约三成，但运转灵力时仍有明显不适。后天出坊市前，需要恢复至能够抵御寒剑气的程度；在那之前还要继续观察额角外伤是否彻底愈合。'
    protagonist.state.inventory = { 下品灵石: 0, 铜牌: 1 }
    protagonist.state.techniques = { 敛息诀: { 熟练度: 30 } }

    render(
      <StoryStateLedger
        snapshot={snapshot}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const metrics = screen.getByRole('group', { name: '数值状态' })
    expect(metrics).toHaveClass('story-state-ledger__flow-grid')
    expect(metrics.querySelectorAll(':scope > [data-state-metric]')).toHaveLength(7)

    const compactFacts = document.querySelector('[data-state-field-group="compact"]')
    const longDetails = document.querySelector('[data-state-field-group="wide"]')
    expect(compactFacts).not.toBeNull()
    expect(longDetails).not.toBeNull()
    expect(within(compactFacts as HTMLElement).getByText('年龄')).toBeInTheDocument()
    expect(within(compactFacts as HTMLElement).getByText('宗门')).toBeInTheDocument()
    expect(within(longDetails as HTMLElement).getByText('当前处境')).toBeInTheDocument()
    const groupOrder = (compactFacts as HTMLElement).compareDocumentPosition(longDetails as Node)
    expect(groupOrder & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    const structuredTabs = screen.getByRole('tablist', { name: '结构化状态' })
    expect(structuredTabs.parentElement).toHaveClass('overflow-x-auto', 'overflow-y-hidden')
    expect(within(structuredTabs).getByRole('tab', { name: '储物袋' })).toHaveAttribute('aria-selected', 'true')
    expect(within(structuredTabs).getByRole('tab', { name: '功法' })).toBeInTheDocument()
    expect(within(screen.getByRole('tabpanel', { name: '储物袋' })).getByText('下品灵石')).toBeInTheDocument()

    await userEvent.click(within(structuredTabs).getByRole('tab', { name: '功法' }))
    expect(within(structuredTabs).getByRole('tab', { name: '功法' })).toHaveAttribute('aria-selected', 'true')
    expect(within(screen.getByRole('tabpanel', { name: '功法' })).getByText('敛息诀')).toBeInTheDocument()
    expect(screen.queryByRole('tabpanel', { name: '储物袋' })).not.toBeInTheDocument()
  })

  it('renders a positive decrement amount with a minus sign', () => {
    const snapshot = storyStateSnapshot()
    const change = snapshot.current_turn?.state_delta?.actor_ops?.[0]
    if (!change) throw new Error('Expected actor state change fixture')
    change.op = 'decrement'
    change.value = 3

    render(
      <StoryStateLedger
        snapshot={snapshot}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const vitalityMetric = visibleText('生命')?.closest('[data-state-metric]')
    expect(vitalityMetric).not.toBeNull()
    expect(within(vitalityMetric as HTMLElement).getByText('-3')).toBeInTheDocument()
  })

  it('bounds the adaptive preview and lets the user expand or restore it for the current turn', async () => {
    vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(720)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(320)

    const { rerender } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="preview"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const region = screen.getByRole('region', { name: '当前状态' })
    expect(region).toHaveAttribute('data-state-panel-mode', 'preview')
    await userEvent.click(screen.getByRole('button', { name: '展开全部' }))
    expect(region).toHaveAttribute('data-state-panel-mode', 'expanded')

    await userEvent.click(screen.getByRole('button', { name: '收起为预览' }))
    expect(region).toHaveAttribute('data-state-panel-mode', 'preview')

    await userEvent.click(screen.getByRole('button', { name: '展开全部' }))
    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot('turn-2')}
        displayPreference="preview"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(region).toHaveAttribute('data-state-panel-mode', 'preview')
  })

  it('localizes the preview controls in English', async () => {
    setConfiguredLocale('en-US')
    await i18n.changeLanguage('en-US')
    vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(720)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(320)

    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="preview"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    await userEvent.click(screen.getByRole('button', { name: 'Expand all' }))
    expect(screen.getByRole('button', { name: 'Collapse to preview' })).toBeInTheDocument()
  })

  it('uses the collapsed preference as a single-line default and preserves manual expansion during the same turn', async () => {
    const { rerender } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="collapsed"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const region = screen.getByRole('region', { name: '当前状态' })
    const header = region.querySelector('header')
    expect(header).toHaveClass('h-11')
    expect(region).toHaveAttribute('data-state', 'closed')
    expect(screen.queryByRole('tablist', { name: '当前状态对象' })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '展开状态面板' }))
    expect(screen.getByRole('tablist', { name: '当前状态对象' })).toBeInTheDocument()
    expect(region.querySelector('header')).toBe(header)

    const sameTurnSnapshot = storyStateSnapshot()
    if (sameTurnSnapshot.current_turn) sameTurnSnapshot.current_turn.state_status = 'pending'
    rerender(
      <StoryStateLedger
        snapshot={sameTurnSnapshot}
        displayPreference="collapsed"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(screen.getByRole('tablist', { name: '当前状态对象' })).toBeInTheDocument()

    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot('turn-2')}
        displayPreference="collapsed"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(screen.queryByRole('tablist', { name: '当前状态对象' })).not.toBeInTheDocument()
    expect(region.querySelector('header')).toBe(header)
  })

  it('restores the expanded default only when a new turn begins', async () => {
    const { rerender } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.getByRole('tablist', { name: '当前状态对象' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '折叠状态面板' }))
    expect(screen.queryByRole('tablist', { name: '当前状态对象' })).not.toBeInTheDocument()

    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(screen.queryByRole('tablist', { name: '当前状态对象' })).not.toBeInTheDocument()

    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot('turn-2')}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(screen.getByRole('tablist', { name: '当前状态对象' })).toBeInTheDocument()
  })

  it('applies a changed default to the current panel immediately', () => {
    const { rerender } = render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="collapsed"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    expect(screen.queryByRole('tablist', { name: '当前状态对象' })).not.toBeInTheDocument()
    rerender(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="expanded"
        onDisplayPreferenceChange={() => undefined}
      />,
    )
    expect(screen.getByRole('tablist', { name: '当前状态对象' })).toBeInTheDocument()
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

  it('exposes all four display preferences from the stage', async () => {
    const onChange = vi.fn()
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="collapsed"
        onDisplayPreferenceChange={onChange}
      />,
    )

    await userEvent.click(screen.getByRole('button', { name: '状态显示偏好' }))
    expect(screen.getByText('默认预览')).toBeInTheDocument()
    expect(screen.getByText('默认折叠')).toBeInTheDocument()
    expect(screen.getByText('仅导演台')).toBeInTheDocument()
    await userEvent.click(screen.getByText('默认展开'))
    expect(onChange).toHaveBeenCalledWith('expanded')
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
