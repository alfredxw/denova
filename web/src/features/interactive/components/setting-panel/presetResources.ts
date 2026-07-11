import type { PresetResourceKind } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, StoryMemoryStructureModule, Teller } from '../../types'
import { defaultRuleTemplates, normalizeTRPGSystem } from '../preset-config/ruleTemplates'

export const TELLER_CONFIG_AGENT_ENTRY_ID = '__config_manager_teller__'

export const EMPTY_TELLERS: Teller[] = []
export const EMPTY_STORY_DIRECTORS: StoryDirector[] = []
export const EMPTY_IMAGE_PRESETS: ImagePreset[] = []
export const EMPTY_EVENT_PACKAGES: EventPackageModule[] = []
export const EMPTY_RULE_SYSTEMS: RuleSystemModule[] = []
export const EMPTY_ACTOR_STATES: ActorStateModule[] = []
export const EMPTY_MEMORY_STRUCTURES: StoryMemoryStructureModule[] = []

export const PRESET_DELETE_COPY: Record<PresetResourceKind, { titleKey: string; descriptionKey: string }> = {
  teller: { titleKey: 'settingPanel.deleteTeller', descriptionKey: 'settingPanel.confirmDeleteTeller' },
  image: { titleKey: 'settingPanel.deleteImagePreset', descriptionKey: 'settingPanel.confirmDeleteImagePreset' },
  director: { titleKey: 'settingPanel.deleteStoryDirector', descriptionKey: 'settingPanel.confirmDeleteStoryDirector' },
  event: { titleKey: 'settingPanel.deleteEventPackage', descriptionKey: 'settingPanel.confirmDeleteEventPackage' },
  rule: { titleKey: 'settingPanel.deleteRuleSystem', descriptionKey: 'settingPanel.confirmDeleteRuleSystem' },
  'actor-state': { titleKey: 'settingPanel.deleteActorState', descriptionKey: 'settingPanel.confirmDeleteActorState' },
  'memory-structure': { titleKey: 'settingPanel.deleteMemoryStructure', descriptionKey: 'settingPanel.confirmDeleteMemoryStructure' },
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
  director: StoryDirector | null
  image: ImagePreset | null
  event: EventPackageModule | null
  rule: RuleSystemModule | null
  actorState: ActorStateModule | null
  memoryStructure: StoryMemoryStructureModule | null
}

export function presetResourceDraftSignature(item: object) {
  return JSON.stringify(item)
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

export function cloneStoryDirector(director: StoryDirector): StoryDirector {
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

export function cloneMemoryStructure(item: StoryMemoryStructureModule): StoryMemoryStructureModule {
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

export function makeStoryDirectorPayload(draft: StoryDirector): Partial<StoryDirector> {
  const payload = cloneStoryDirector({
    ...draft,
    id: draft.id,
  })
  delete (payload as unknown as Record<string, unknown>).event_system
  delete (payload as unknown as Record<string, unknown>).opening_selector
  const refs = payload.module_refs
  if (refs) {
    if (!refs.event_package_ids?.length && refs.event_system_id) {
      refs.event_package_ids = [refs.event_system_id]
    }
    if (refs.event_packages_disabled === undefined && refs.event_system_disabled === true) {
      refs.event_packages_disabled = true
    }
    delete (refs as Record<string, unknown>).event_system_id
    delete (refs as Record<string, unknown>).event_system_disabled
    delete (refs as Record<string, unknown>).opening_selector_id
    delete (refs as Record<string, unknown>).opening_selector_disabled
  }
  return payload
}

export function makeEventPackagePayload(draft: EventPackageModule): Partial<EventPackageModule> {
  const payload = cloneEventPackage({
    ...draft,
    id: draft.id,
  })
  delete (payload as unknown as Record<string, unknown>).event_system
  delete (payload as unknown as Record<string, unknown>).custom_events
  return payload
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

export function makeMemoryStructurePayload(draft: StoryMemoryStructureModule): Partial<StoryMemoryStructureModule> {
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
    name: presetDraftText(t, 'settingPanel.presetDraft.teller.name', '自定义叙事风格'),
    description: presetDraftText(t, 'settingPanel.presetDraft.teller.description', '新的叙事风格'),
    random_event_rate: 0.15,
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
        name: presetDraftText(t, 'settingPanel.presetDraft.teller.systemName', '系统提示'),
        target: 'system',
        enabled: true,
        content: presetDraftText(t, 'settingPanel.presetDraft.teller.systemContent', '你是一套自定义叙事风格。你要明确影响故事的文风倾向、角色反应、剧情裁定、节奏推进和长期叙事原则。'),
      },
      {
        id: 'turn_context',
        name: presetDraftText(t, 'settingPanel.presetDraft.teller.turnName', '本轮上下文'),
        target: 'turn_context',
        enabled: true,
        content: presetDraftText(t, 'settingPanel.presetDraft.teller.turnContent', '每轮都要让用户行动带来具体后果，并主动制造符合叙事风格的反馈、阻碍、发现、NPC 反应、代价、暗线推进或新的行动入口。'),
      },
      {
        id: 'state_memory',
        name: presetDraftText(t, 'settingPanel.presetDraft.teller.memoryName', '记忆沉淀规则'),
        target: 'state_memory',
        enabled: true,
        content: presetDraftText(t, 'settingPanel.presetDraft.teller.memoryContent', '记录本回合已经成立的关系变化、风险、线索、资源、暗线和可继续行动的入口。'),
      },
    ],
  }
}

export function newStoryDirectorDraft(t?: PresetDraftTranslator): Partial<StoryDirector> {
  return {
    id: `custom-director-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.director.name', '自定义故事导演'),
    description: presetDraftText(t, 'settingPanel.presetDraft.director.description', '新的故事导演，组合叙事风格、事件包、TRPG 检定、状态系统、记忆结构和图像方案。'),
    module_refs: {
      narrative_style_id: 'classic',
      event_package_ids: ['default'],
      rule_system_id: 'default',
      actor_state_id: 'default',
      memory_structure_id: 'default',
      image_preset_id: 'game-cg',
    },
    strategy: {
      enabled: true,
      mainline_strength: 'balanced',
      failure_policy: 'consequence',
      pacing_curve: 'goal-pressure-payoff',
      random_event_rate: 0.15,
      director_agent_mode: 'triggered',
      rule_state_consumption_mode: 'hybrid_auto',
      rule_visibility_mode: 'audit_only',
      branch_planning_turns: 5,
    },
    event_packages: [],
    trpg_system: {
      rule_templates: [],
    },
    actor_state: {
      templates: [],
      initial_actors: [],
    },
    version: 2,
    custom: true,
  }
}

export function newEventPackageDraft(t?: PresetDraftTranslator): Partial<EventPackageModule> {
  return {
    id: `custom-event-package-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.event.name', '自定义事件包'),
    description: presetDraftText(t, 'settingPanel.presetDraft.event.description', '新的事件包，配置事件卡、强度、冷却和事件描述。'),
    events: [],
    version: 1,
    custom: true,
  }
}

export function newRuleSystemDraft(t?: PresetDraftTranslator): Partial<RuleSystemModule> {
  return {
    id: `custom-rule-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.rule.name', '自定义 TRPG 检定'),
    description: presetDraftText(t, 'settingPanel.presetDraft.rule.description', '新的 TRPG 检定，代表一种 DM 检定风格，并配置固定 d20、难度修正、失败处理、难度判断、状态影响指引和可选 State Binding。'),
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
    name: presetDraftText(t, 'settingPanel.presetDraft.actor.name', '自定义状态系统'),
    description: presetDraftText(t, 'settingPanel.presetDraft.actor.description', '新的状态系统，配置状态表模板、字段 schema 和初始状态对象。'),
    actor_state: {
      templates: [
        {
          id: 'protagonist',
          name: presetDraftText(t, 'settingPanel.presetDraft.actor.templateName', '默认主角状态表'),
          description: presetDraftText(t, 'settingPanel.presetDraft.actor.templateDescription', '示例主角状态表，可替换或新增世界、故事、势力、基地、特定角色等状态表。'),
          fields: [
            { id: 'current_status', path: 'current.status', name: presetDraftText(t, 'settingPanel.presetDraft.actor.fieldName', '当前状态'), type: 'string', default: presetDraftText(t, 'settingPanel.presetDraft.actor.fieldDefault', '状态稳定，等待剧情确定。'), visibility: 'visible' },
          ],
        },
      ],
      trait_pools: [],
      initial_actors: [{ id: 'protagonist', name: presetDraftText(t, 'settingPanel.presetDraft.actor.initialName', '主角'), template_id: 'protagonist', role: 'protagonist' }],
    },
    version: 5,
    custom: true,
  }
}

export function newMemoryStructureDraft(t?: PresetDraftTranslator): Partial<StoryMemoryStructureModule> {
  return {
    id: `custom-memory-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.memory.name', '自定义记忆结构'),
    description: presetDraftText(t, 'settingPanel.presetDraft.memory.description', '新的故事记忆结构，配置长期记忆分组、字段和整理要求。'),
    structures: [],
    version: 1,
    custom: true,
  }
}

export function newImagePresetDraft(t?: PresetDraftTranslator): Partial<ImagePreset> {
  return {
    id: `custom-image-${Date.now()}`,
    name: presetDraftText(t, 'settingPanel.presetDraft.image.name', '自定义图像方案'),
    description: presetDraftText(t, 'settingPanel.presetDraft.image.description', '新的图像风格方案'),
    prompt: presetDraftText(t, 'settingPanel.presetDraft.image.prompt', '描述画面风格、媒介、构图、镜头语言、光影、色彩、角色与环境呈现限制，以及需要避免的内容。'),
    version: 1,
    custom: true,
  }
}

export function isPresetConfigResourceKind(kind: PresetResourceKind) {
  return kind === 'director' || kind === 'event' || kind === 'rule' || kind === 'actor-state' || kind === 'memory-structure'
}

export function currentPresetBuiltinOverridden(kind: PresetResourceKind, drafts: PresetDrafts) {
  if (kind === 'director') return Boolean(drafts.director?.builtin_overridden)
  if (kind === 'image') return Boolean(drafts.image?.builtin_overridden)
  if (kind === 'event') return Boolean(drafts.event?.builtin_overridden)
  if (kind === 'rule') return Boolean(drafts.rule?.builtin_overridden)
  if (kind === 'actor-state') return Boolean(drafts.actorState?.builtin_overridden)
  if (kind === 'memory-structure') return Boolean(drafts.memoryStructure?.builtin_overridden)
  return Boolean(drafts.teller?.builtin_overridden)
}

export function presetEditorTitle(kind: PresetResourceKind, drafts: PresetDrafts, t: (key: string) => string) {
  if (kind === 'image') return drafts.image?.name || t('settingPanel.editor.defaultImagePreset')
  if (kind === 'director') return drafts.director?.name || t('settingPanel.editor.defaultStoryDirector')
  if (kind === 'event') return drafts.event?.name || t('settingPanel.editor.defaultEventPackage')
  if (kind === 'rule') return drafts.rule?.name || t('settingPanel.editor.defaultRuleSystem')
  if (kind === 'actor-state') return drafts.actorState?.name || t('settingPanel.editor.defaultActorState')
  if (kind === 'memory-structure') return drafts.memoryStructure?.name || t('settingPanel.editor.defaultMemoryStructure')
  return drafts.teller?.name || t('settingPanel.editor.defaultTeller')
}

export function presetEditorSubtitle(kind: PresetResourceKind, drafts: PresetDrafts, t: (key: string) => string) {
  if (kind === 'image') return drafts.image?.description || t('settingPanel.editor.imagePresetSubtitle')
  if (kind === 'director') return drafts.director?.description || t('settingPanel.editor.storyDirectorSubtitle')
  if (kind === 'event') return drafts.event?.description || t('settingPanel.editor.eventPackageSubtitle')
  if (kind === 'rule') return drafts.rule?.description || t('settingPanel.editor.ruleSystemSubtitle')
  if (kind === 'actor-state') return drafts.actorState?.description || t('settingPanel.editor.actorStateSubtitle')
  if (kind === 'memory-structure') return drafts.memoryStructure?.description || t('settingPanel.editor.memoryStructureSubtitle')
  return drafts.teller?.description || t('settingPanel.editor.tellerSubtitle')
}

function cloneJSON<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function presetDraftText(t: PresetDraftTranslator | undefined, key: string, fallback: string) {
  return t?.(key) || fallback
}
