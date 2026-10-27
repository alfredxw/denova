import type { TFunction } from 'i18next'
import type { PresetResourceKind } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, GamePlanningTemplate, ImagePreset, RuleSystemModule, Teller } from '../../types'
import { gamePlanningTemplateDescription, gamePlanningTemplateName } from '../../game-planning'
import { defaultRuleTemplates, normalizeTRPGSystem } from '../preset-config/ruleTemplates'

export const EMPTY_TELLERS: Teller[] = []
export const EMPTY_STORY_DIRECTORS: GamePlanningTemplate[] = []
export const EMPTY_IMAGE_PRESETS: ImagePreset[] = []
export const EMPTY_EVENT_PACKAGES: EventPackageModule[] = []
export const EMPTY_RULE_SYSTEMS: RuleSystemModule[] = []
export const EMPTY_ACTOR_STATES: ActorStateModule[] = []

export const PRESET_DELETE_COPY: Record<PresetResourceKind, { titleKey: string; descriptionKey: string }> = {
  teller: { titleKey: 'settingPanel.deleteTeller', descriptionKey: 'settingPanel.confirmDeleteTeller' },
  image: { titleKey: 'settingPanel.deleteImagePreset', descriptionKey: 'settingPanel.confirmDeleteImagePreset' },
  director: { titleKey: 'settingPanel.deleteStoryDirector', descriptionKey: 'settingPanel.confirmDeleteStoryDirector' },
  event: { titleKey: 'settingPanel.deleteEventPackage', descriptionKey: 'settingPanel.confirmDeleteEventPackage' },
  rule: { titleKey: 'settingPanel.deleteRuleSystem', descriptionKey: 'settingPanel.confirmDeleteRuleSystem' },
  'actor-state': { titleKey: 'settingPanel.deleteActorState', descriptionKey: 'settingPanel.confirmDeleteActorState' },
}

export interface PresetDeleteTarget {
  kind: PresetResourceKind
  id: string
  name: string
  titleKey: string
  descriptionKey: string
}

export interface PresetDrafts {
  teller: Teller | null
  director: GamePlanningTemplate | null
  image: ImagePreset | null
  event: EventPackageModule | null
  rule: RuleSystemModule | null
  actorState: ActorStateModule | null
}

export function presetResourceDraftSignature(item: object) {
  const {
    created_at: _createdAt,
    updated_at: _updatedAt,
    revision: _revision,
    ...editable
  } = item as Record<string, unknown>
  return JSON.stringify(editable)
}

export function cloneTeller(teller: Teller): Teller {
  return {
    ...teller,
    slots: [...(teller.slots || [])],
    context_policy: { ...teller.context_policy },
    style_refs: [...(teller.style_refs || [])],
    style_rules: [...(teller.style_rules || [])],
  }
}

export function cloneImagePreset(preset: ImagePreset): ImagePreset {
  return { ...preset }
}

export function cloneStoryDirector(director: GamePlanningTemplate): GamePlanningTemplate {
  return cloneJSON(director)
}

export function cloneEventPackage(item: EventPackageModule): EventPackageModule {
  return cloneJSON(item)
}

export function cloneRuleSystem(item: RuleSystemModule): RuleSystemModule {
  return cloneJSON(item)
}

export function cloneActorState(item: ActorStateModule): ActorStateModule {
  return cloneJSON(item)
}

export function makeTellerPayload(draft: Teller): Partial<Teller> {
  return {
    ...draft,
    id: draft.id,
  }
}

export function makeImagePresetPayload(draft: ImagePreset): Partial<ImagePreset> {
  return {
    ...draft,
    id: draft.id,
  }
}

export function makeStoryDirectorPayload(draft: GamePlanningTemplate): Partial<GamePlanningTemplate> {
  return cloneStoryDirector({
    ...draft,
    id: draft.id,
  })
}

export function makeEventPackagePayload(draft: EventPackageModule): Partial<EventPackageModule> {
  return cloneEventPackage({
    ...draft,
    id: draft.id,
  })
}

export function makeRuleSystemPayload(draft: RuleSystemModule): Partial<RuleSystemModule> {
  return {
    ...draft,
    id: draft.id,
    trpg_system: normalizeTRPGSystem(draft.trpg_system),
  }
}

export function makeActorStatePayload(draft: ActorStateModule): Partial<ActorStateModule> {
  return {
    ...draft,
    id: draft.id,
  }
}

type PresetDraftTranslator = (key: string) => string

export function newTellerDraft(t?: PresetDraftTranslator): Partial<Teller> {
  const id = `custom-${Date.now()}`
  return {
    id,
    name: presetDraftText(t, 'settingPanel.presetDraft.teller.name', 'Пользовательский стиль повествования'),
    description: presetDraftText(t, 'settingPanel.presetDraft.teller.description', 'Новый стиль повествования'),
    style_refs: [],
    style_rules: [],
    context_policy: {
      creator: 'always',
      lore: 'relevant',
      runtime_state: 'always',
    },
    slots: [
      {
        id: 'identity',
        name: presetDraftText(t, 'settingPanel.presetDraft.teller.systemName', 'Системный промпт'),
        target: 'system',
        enabled: true,
        content: presetDraftText(t, 'settingPanel.presetDraft.teller.systemContent', 'Вы — пользовательский стиль повествования. Задавайте тон текста, реакции персонажей, темп и принципы повествования.'),
      },
      {
        id: 'turn_context',
        name: presetDraftText(t, 'settingPanel.presetDraft.teller.turnName', 'Контекст хода'),
        target: 'turn_context',
        enabled: true,
        content: presetDraftText(t, 'settingPanel.presetDraft.teller.turnContent', 'Каждое действие пользователя должно приводить к осязаемым последствиям, препятствиям или новым возможностям.'),
      },
    ],
  }
}

export function newStoryDirectorDraft(t?: PresetDraftTranslator): Partial<GamePlanningTemplate> {
  return {
    id: `custom-planning-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.director.name', 'Пользовательский шаблон планирования'),
    description: presetDraftText(
      t,
      'settingPanel.presetDraft.director.description',
      'Используйте разделы для определения направлений сюжета.',
    ),
    sections: [
      {
        id: 'long-term-direction',
        title: presetDraftText(t, 'settingPanel.presetDraft.director.sectionTitle', 'Долгосрочное направление'),
        description: presetDraftText(
          t,
          'settingPanel.presetDraft.director.sectionDescription',
          'Отслеживайте долгосрочные возможности и ключевые повороты сюжета.',
        ),
      },
    ],
    version: 1,
    custom: true,
  }
}

export function newEventPackageDraft(t?: PresetDraftTranslator): Partial<EventPackageModule> {
  return {
    id: `custom-event-package-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.event.name', 'Пользовательский пакет событий'),
    description: presetDraftText(t, 'settingPanel.presetDraft.event.description', 'Новый пакет событий для настройки карточек и триггеров.'),
    events: [],
    version: 1,
    custom: true,
  }
}

export function newRuleSystemDraft(t?: PresetDraftTranslator): Partial<RuleSystemModule> {
  return {
    id: `custom-rule-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.rule.name', 'Пользовательская TRPG-проверка'),
    description: presetDraftText(t, 'settingPanel.presetDraft.rule.description', 'Новый стиль проверок мастера с бросками d20 и модификаторами сложности.'),
    trpg_system: {
      rule_templates: defaultRuleTemplates(),
    },
    version: 1,
    custom: true,
  }
}

export function newActorStateDraft(t?: PresetDraftTranslator): Partial<ActorStateModule> {
  return {
    id: `custom-actor-state-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.actor.name', 'Пользовательская система состояний'),
    description: presetDraftText(t, 'settingPanel.presetDraft.actor.description', 'Новая система состояний персонажей и мира.'),
    actor_state: {
      templates: [
        {
          id: 'protagonist',
          name: presetDraftText(t, 'settingPanel.presetDraft.actor.templateName', 'Состояние протагониста'),
          description: presetDraftText(t, 'settingPanel.presetDraft.actor.templateDescription', 'Таблица состояния главного героя.'),
          fields: [
            { id: 'current_status', path: 'current.status', name: presetDraftText(t, 'settingPanel.presetDraft.actor.fieldName', 'Текущий статус'), type: 'string', default: presetDraftText(t, 'settingPanel.presetDraft.actor.fieldDefault', 'Стабилен, ожидает развития сюжета.') },
          ],
        },
      ],
      trait_pools: [],
      initial_actors: [{ id: 'protagonist', name: presetDraftText(t, 'settingPanel.presetDraft.actor.initialName', 'Главный герой'), template_id: 'protagonist', role: 'protagonist' }],
    },
    version: 5,
    custom: true,
  }
}

export function newImagePresetDraft(t?: PresetDraftTranslator): Partial<ImagePreset> {
  return {
    id: `custom-image-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.image.name', 'Пользовательский стиль изображений'),
    description: presetDraftText(t, 'settingPanel.presetDraft.image.description', 'Новый стиль иллюстраций'),
    prompt: presetDraftText(t, 'settingPanel.presetDraft.image.prompt', 'Опишите визуальный стиль, технику, освещение и ограничения.'),
    version: 1,
    custom: true,
  }
}

export function isPresetConfigResourceKind(kind: PresetResourceKind) {
  return kind === 'director' || kind === 'event' || kind === 'rule' || kind === 'actor-state'
}

export function currentPresetBuiltinOverridden(kind: PresetResourceKind, drafts: PresetDrafts) {
  if (kind === 'director') return Boolean(drafts.director?.builtin_overridden)
  if (kind === 'image') return Boolean(drafts.image?.builtin_overridden)
  if (kind === 'event') return Boolean(drafts.event?.builtin_overridden)
  if (kind === 'rule') return Boolean(drafts.rule?.builtin_overridden)
  if (kind === 'actor-state') return Boolean(drafts.actorState?.builtin_overridden)
  return Boolean(drafts.teller?.builtin_overridden)
}

export function presetEditorTitle(kind: PresetResourceKind, drafts: PresetDrafts, t: TFunction) {
  if (kind === 'image') return drafts.image?.name || t('settingPanel.editor.defaultImagePreset')
  if (kind === 'director') {
    return drafts.director
      ? gamePlanningTemplateName(drafts.director, t)
      : t('settingPanel.editor.defaultStoryDirector')
  }
  if (kind === 'event') return drafts.event?.name || t('settingPanel.editor.defaultEventPackage')
  if (kind === 'rule') return drafts.rule?.name || t('settingPanel.editor.defaultRuleSystem')
  if (kind === 'actor-state') return drafts.actorState?.name || t('settingPanel.editor.defaultActorState')
  return drafts.teller?.name || t('settingPanel.editor.defaultTeller')
}

export function presetEditorSubtitle(kind: PresetResourceKind, drafts: PresetDrafts, t: TFunction) {
  if (kind === 'image') return drafts.image?.description || t('settingPanel.editor.imagePresetSubtitle')
  if (kind === 'director') {
    return drafts.director
      ? gamePlanningTemplateDescription(drafts.director, t)
      : t('settingPanel.editor.storyDirectorSubtitle')
  }
  if (kind === 'event') return drafts.event?.description || t('settingPanel.editor.eventPackageSubtitle')
  if (kind === 'rule') return drafts.rule?.description || t('settingPanel.editor.ruleSystemSubtitle')
  if (kind === 'actor-state') return drafts.actorState?.description || t('settingPanel.editor.actorStateSubtitle')
  return drafts.teller?.description || t('settingPanel.editor.tellerSubtitle')
}

function cloneJSON<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function presetDraftText(t: PresetDraftTranslator | undefined, key: string, fallback: string) {
  return t?.(key) || fallback
}
