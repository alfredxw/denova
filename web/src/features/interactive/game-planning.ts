import type { TFunction } from 'i18next'
import type { GamePlanningSection, GamePlanningTemplate } from './types'

export const DEFAULT_GAME_PLANNING_TEMPLATE_ID = 'default'

/** Localize built-in templates while preserving custom creator content. */
export function gamePlanningTemplateName(template: GamePlanningTemplate, t: TFunction): string {
  return localizedBuiltinValue(template, t, 'name', template.name || template.id)
}

export function gamePlanningTemplateDescription(template: GamePlanningTemplate, t: TFunction): string {
  return localizedBuiltinValue(template, t, 'description', template.description || '')
}

export function gamePlanningSectionTitle(
  template: GamePlanningTemplate,
  section: GamePlanningSection,
  t: TFunction,
): string {
  return localizedBuiltinSectionValue(template, section, t, 'title', section.title)
}

export function gamePlanningSectionDescription(
  template: GamePlanningTemplate,
  section: GamePlanningSection,
  t: TFunction,
): string {
  return localizedBuiltinSectionValue(template, section, t, 'description', section.description || '')
}

function localizedBuiltinValue(
  template: GamePlanningTemplate,
  t: TFunction,
  field: 'name' | 'description',
  fallback: string,
): string {
  if (template.custom || template.builtin_overridden) return fallback
  return t(`storyPicker.planningTemplates.${template.id}.${field}`, { defaultValue: fallback })
}

function localizedBuiltinSectionValue(
  template: GamePlanningTemplate,
  section: GamePlanningSection,
  t: TFunction,
  field: 'title' | 'description',
  fallback: string,
): string {
  if (template.custom || template.builtin_overridden) return fallback
  return t(`storyPicker.planningTemplates.${template.id}.sections.${section.id}.${field}`, { defaultValue: fallback })
}
