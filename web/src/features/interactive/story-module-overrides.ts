import type { StoryDirectorModuleRefs, StorySummary } from './types'

/**
 * Rebase one story's module layer when its Game Preset changes. Explicit story
 * overrides survive; inherited values follow the new preset. Rule and actor
 * structure can be frozen independently once turns exist.
 */
export function rebaseStoryModuleRefsForPresetChange(story: StorySummary | undefined, previousPreset: StoryDirectorModuleRefs | undefined, nextPreset: StoryDirectorModuleRefs | undefined, preserveStructural: boolean): StoryDirectorModuleRefs {
  const current = effectiveStoryModuleRefs(story, previousPreset)
  const previous = previousPreset || {}
  const next = nextPreset || {}
  const narrative = rebaseModule(current.narrative_style_id, current.narrative_style_disabled, previous.narrative_style_id, previous.narrative_style_disabled, next.narrative_style_id, next.narrative_style_disabled, false)
  const rules = rebaseModule(current.rule_system_id, current.rule_system_disabled, previous.rule_system_id, previous.rule_system_disabled, next.rule_system_id, next.rule_system_disabled, preserveStructural)
  const actorState = rebaseModule(current.actor_state_id, current.actor_state_disabled, previous.actor_state_id, previous.actor_state_disabled, next.actor_state_id, next.actor_state_disabled, preserveStructural)
  const images = rebaseModule(current.image_preset_id, current.image_preset_disabled, previous.image_preset_id, previous.image_preset_disabled, next.image_preset_id, next.image_preset_disabled, false)
  const preserveEvents = Boolean(current.event_packages_disabled) !== Boolean(previous.event_packages_disabled)
    || !sameStringList(current.event_package_ids, previous.event_package_ids)
  return {
    narrative_style_id: narrative.id,
    narrative_style_disabled: narrative.disabled,
    event_package_ids: [...(preserveEvents ? current.event_package_ids || [] : next.event_package_ids || [])],
    event_packages_disabled: preserveEvents ? Boolean(current.event_packages_disabled) : Boolean(next.event_packages_disabled),
    rule_system_id: rules.id,
    rule_system_disabled: rules.disabled,
    actor_state_id: actorState.id,
    actor_state_disabled: actorState.disabled,
    image_preset_id: images.id,
    image_preset_disabled: images.disabled,
  }
}

function effectiveStoryModuleRefs(story: StorySummary | undefined, presetRefs: StoryDirectorModuleRefs | undefined): StoryDirectorModuleRefs {
  const refs: StoryDirectorModuleRefs = {
    ...(presetRefs || {}),
    ...(story?.module_refs || {}),
    event_package_ids: [...(story?.module_refs?.event_package_ids ?? presetRefs?.event_package_ids ?? [])],
  }
  // Older stories could override these through dedicated fields before the
  // console synchronized them with module_refs.
  if (story?.story_teller_id && story.story_teller_id !== presetRefs?.narrative_style_id) {
    refs.narrative_style_id = story.story_teller_id
    refs.narrative_style_disabled = false
  }
  if (story?.image_settings?.preset_id && story.image_settings.preset_id !== presetRefs?.image_preset_id) {
    refs.image_preset_id = story.image_settings.preset_id
    refs.image_preset_disabled = false
  }
  return refs
}

function rebaseModule(currentID: string | undefined, currentDisabled: boolean | undefined, previousID: string | undefined, previousDisabled: boolean | undefined, nextID: string | undefined, nextDisabled: boolean | undefined, forcePreserve: boolean) {
  const preserve = forcePreserve || currentID !== previousID || Boolean(currentDisabled) !== Boolean(previousDisabled)
  return preserve
    ? { id: currentID, disabled: Boolean(currentDisabled) }
    : { id: nextID, disabled: Boolean(nextDisabled) }
}

function sameStringList(left: string[] | undefined, right: string[] | undefined) {
  const leftValues = left || []
  const rightValues = right || []
  return leftValues.length === rightValues.length && leftValues.every((value, index) => value === rightValues[index])
}
