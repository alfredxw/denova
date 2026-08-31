import { describe, expect, it } from 'vitest'
import type { StoryDirectorModuleRefs, StorySummary } from './types'
import { rebaseStoryModuleRefsForPresetChange } from './story-module-overrides'

const previous: StoryDirectorModuleRefs = {
  narrative_style_id: 'old-style',
  event_package_ids: ['old-events'],
  rule_system_id: 'old-rules',
  actor_state_id: 'old-state',
  image_preset_id: 'old-image',
}
const next: StoryDirectorModuleRefs = {
  narrative_style_id: 'new-style',
  event_package_ids: ['new-events'],
  rule_system_id: 'new-rules',
  actor_state_id: 'new-state',
  image_preset_id: 'new-image',
}

describe('rebaseStoryModuleRefsForPresetChange', () => {
  it('moves inherited values to the new preset and retains explicit overrides', () => {
    const story = storyWith({ ...previous, narrative_style_id: 'story-style', event_package_ids: ['story-events'] })

    expect(rebaseStoryModuleRefsForPresetChange(story, previous, next, false)).toEqual({
      narrative_style_id: 'story-style',
      narrative_style_disabled: false,
      event_package_ids: ['story-events'],
      event_packages_disabled: false,
      rule_system_id: 'new-rules',
      rule_system_disabled: false,
      actor_state_id: 'new-state',
      actor_state_disabled: false,
      image_preset_id: 'new-image',
      image_preset_disabled: false,
    })
  })

  it('freezes rule and actor structure after turns exist', () => {
    const rebased = rebaseStoryModuleRefsForPresetChange(storyWith(previous), previous, next, true)

    expect(rebased.rule_system_id).toBe('old-rules')
    expect(rebased.actor_state_id).toBe('old-state')
    expect(rebased.narrative_style_id).toBe('new-style')
    expect(rebased.image_preset_id).toBe('new-image')
  })

  it('protects legacy narrative and image overrides stored outside module refs', () => {
    const story = storyWith(previous)
    story.story_teller_id = 'legacy-style'
    story.image_settings = { mode: 'manual', interval_turns: 3, preset_id: 'legacy-image' }

    const rebased = rebaseStoryModuleRefsForPresetChange(story, previous, next, false)
    expect(rebased.narrative_style_id).toBe('legacy-style')
    expect(rebased.image_preset_id).toBe('legacy-image')
  })
})

function storyWith(moduleRefs: StoryDirectorModuleRefs): StorySummary {
  return {
    id: 'story-1', title: '', origin: '', protagonist: { mode: 'default' }, story_teller_id: 'old-style', story_director_id: 'old',
    module_refs: { ...moduleRefs, event_package_ids: [...(moduleRefs.event_package_ids || [])] },
    reply_target_chars: 2000, choice_count: 5, opening: { mode: 'ai' }, created_at: '', updated_at: '',
    branches: 1, events: 0, turn_count: 0,
  }
}
