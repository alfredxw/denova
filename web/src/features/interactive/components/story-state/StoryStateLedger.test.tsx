import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Snapshot, TurnEvent } from '../../types'
import { StoryStateLedger } from './StoryStateLedger'

function isVisibleElement(element: HTMLElement) {
  const closestHidden = element.closest('[aria-hidden="true"], .invisible')
  return closestHidden === null
}

function visibleText(text: string) {
  return screen.getAllByText(text).find(isVisibleElement)
}

describe('StoryStateLedger', () => {
  it('keeps Actor and World State as peer tabs in the collapsed stage summary', async () => {
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="collapsed"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const tabs = screen.getByRole('tablist', { name: '当前状态对象' })
    expect(tabs).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '林风' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: '世界状态' })).toBeInTheDocument()
    expect(visibleText('青石镇客栈')).toBeInTheDocument()
    expect(screen.queryByText('本回合变化')).not.toBeInTheDocument()
    expect(within(screen.getByRole('tabpanel')).getByText('-3')).toBeInTheDocument()
    expect(within(screen.getByRole('tabpanel')).getByText('受了轻伤')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: '世界状态' }))
    expect(screen.getByRole('tab', { name: '世界状态' })).toHaveAttribute('aria-selected', 'true')
    expect(visibleText('暴雨将至')).toBeInTheDocument()
    expect(screen.queryByText('青石镇客栈')).not.toBeInTheDocument()
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
    const sceneField = visibleText('Scene')?.closest('[data-state-field]')
    expect(sceneField).not.toBeNull()
    expect(within(sceneField as HTMLElement).getAllByText('Weather').length).toBeGreaterThanOrEqual(1)
    expect(within(sceneField as HTMLElement).getByText('天色骤暗')).toBeInTheDocument()
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

  it('keeps one fixed-height header while the Radix Collapsible content opens and closes', async () => {
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="collapsed"
        onDisplayPreferenceChange={() => undefined}
      />,
    )

    const region = screen.getByRole('region', { name: '当前状态' })
    const header = region.querySelector('header')
    expect(header).toHaveClass('h-11')

    await userEvent.click(screen.getByRole('button', { name: '折叠状态面板' }))
    expect(screen.queryByRole('tablist', { name: '当前状态对象' })).not.toBeInTheDocument()
    expect(region.querySelector('header')).toBe(header)
    expect(region.querySelector('header')).toHaveClass('h-11')

    await userEvent.click(screen.getByRole('button', { name: '展开状态面板' }))
    expect(screen.getByRole('tablist', { name: '当前状态对象' })).toBeInTheDocument()
    expect(region.querySelector('header')).toBe(header)
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

  it('exposes all three display preferences from the stage', async () => {
    const onChange = vi.fn()
    render(
      <StoryStateLedger
        snapshot={storyStateSnapshot()}
        displayPreference="collapsed"
        onDisplayPreferenceChange={onChange}
      />,
    )

    await userEvent.click(screen.getByRole('button', { name: '状态显示偏好' }))
    await userEvent.click(screen.getByText('展开'))
    expect(onChange).toHaveBeenCalledWith('expanded')
  })
})

function storyStateSnapshot(): Snapshot {
  const turn: TurnEvent = {
    id: 'turn-1',
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
