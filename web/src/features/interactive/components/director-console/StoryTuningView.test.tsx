import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StoryDirector, StorySummary, Teller } from '../../types'
import { StoryTuningView } from './StoryTuningView'

vi.mock('../../api', () => ({
  getActorStates: vi.fn().mockResolvedValue([{ id: 'state-basic', name: '基础状态' }]),
  getEventPackages: vi.fn().mockResolvedValue([]),
  getRuleSystems: vi.fn().mockResolvedValue([{ id: 'd20', name: 'D20' }]),
}))

const director: StoryDirector = {
  version: 1,
  id: 'adventure',
  name: '冒险',
  description: '',
  module_refs: {
    narrative_style_id: 'cinematic',
    rule_system_id: 'd20',
    actor_state_id: 'state-basic',
    image_preset_id: 'game-cg',
  },
  strategy: {},
  trpg_system: {},
  custom: false,
}

const teller: Teller = {
  version: 1,
  id: 'cinematic',
  name: '电影化',
  description: '',
  context_policy: {} as Teller['context_policy'],
  slots: [],
  custom: false,
}

function story(turnCount = 0): StorySummary {
  return {
    id: 'story-1',
    title: '测试故事',
    origin: '',
    story_teller_id: teller.id,
    story_director_id: director.id,
    planning_mode: 'disabled',
    module_refs: { ...director.module_refs },
    reply_target_chars: 2000,
    choice_count: 5,
    image_settings: { mode: 'manual', interval_turns: 3, preset_id: 'game-cg' },
    check_settings: { difficulty_shift: 0, roll_modifier: 0 },
    opening: { mode: 'ai' },
    created_at: '',
    updated_at: '',
    branches: 1,
    events: 0,
    turn_count: turnCount,
  }
}

describe('StoryTuningView', () => {
  const onUpdate = vi.fn().mockResolvedValue(undefined)

  beforeEach(() => onUpdate.mockClear())

  it('shows every control group by default and saves story-level agent tuning', async () => {
    render(<StoryTuningView story={story()} directors={[director]} tellers={[teller]} imagePresets={[{ version: 1, id: 'game-cg', name: 'Game CG', description: '', custom: false }]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onDirectorChange={vi.fn()} onUpdate={onUpdate} />)

    expect(screen.getByRole('heading', { name: 'Game Agent' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '回合判定' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '互动图像' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '状态面板' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('switch', { name: '导演规划' }))
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ planning_mode: 'enabled' }))
  })

  it('locks structural rule selection after the first turn but leaves checks configurable', async () => {
    render(<StoryTuningView story={story(1)} directors={[director]} tellers={[teller]} imagePresets={[]} stateDisplayPreference="preview" onStateDisplayPreferenceChange={vi.fn()} onUpdate={onUpdate} />)

    expect(await screen.findByLabelText('规则系统')).toBeDisabled()
    expect(screen.getByLabelText('全局难度')).not.toBeDisabled()
  })
})
