import type { PresetResourceKind } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, ImagePreset, OpeningSelectorModule, RuleSystemModule, StoryDirector, StoryMemoryStructureModule, Teller } from '../../types'

export const TELLER_CONFIG_AGENT_ENTRY_ID = '__config_manager_teller__'

export const EMPTY_TELLERS: Teller[] = []
export const EMPTY_STORY_DIRECTORS: StoryDirector[] = []
export const EMPTY_IMAGE_PRESETS: ImagePreset[] = []
export const EMPTY_EVENT_PACKAGES: EventPackageModule[] = []
export const EMPTY_RULE_SYSTEMS: RuleSystemModule[] = []
export const EMPTY_ACTOR_STATES: ActorStateModule[] = []
export const EMPTY_MEMORY_STRUCTURES: StoryMemoryStructureModule[] = []
export const EMPTY_OPENING_SELECTORS: OpeningSelectorModule[] = []

export const PRESET_DELETE_COPY: Record<PresetResourceKind, { titleKey: string; descriptionKey: string }> = {
  teller: { titleKey: 'settingPanel.deleteTeller', descriptionKey: 'settingPanel.confirmDeleteTeller' },
  image: { titleKey: 'settingPanel.deleteImagePreset', descriptionKey: 'settingPanel.confirmDeleteImagePreset' },
  director: { titleKey: 'settingPanel.deleteStoryDirector', descriptionKey: 'settingPanel.confirmDeleteStoryDirector' },
  event: { titleKey: 'settingPanel.deleteEventPackage', descriptionKey: 'settingPanel.confirmDeleteEventPackage' },
  rule: { titleKey: 'settingPanel.deleteRuleSystem', descriptionKey: 'settingPanel.confirmDeleteRuleSystem' },
  'actor-state': { titleKey: 'settingPanel.deleteActorState', descriptionKey: 'settingPanel.confirmDeleteActorState' },
  'memory-structure': { titleKey: 'settingPanel.deleteMemoryStructure', descriptionKey: 'settingPanel.confirmDeleteMemoryStructure' },
  opening: { titleKey: 'settingPanel.deleteOpeningSelector', descriptionKey: 'settingPanel.confirmDeleteOpeningSelector' },
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
  opening: OpeningSelectorModule | null
}

export function splitTags(value: string) {
  return value
    .split(/[，,]/)
    .map((tag) => tag.trim())
    .filter(Boolean)
}

export function presetResourceDraftSignature(item: object, tagDraft: string) {
  return JSON.stringify({
    ...item,
    tags: splitTags(tagDraft),
  })
}

export function cloneTeller(teller: Teller): Teller {
  return {
    ...teller,
    tags: [...(teller.tags || [])],
    slots: [...(teller.slots || [])],
    context_policy: { ...teller.context_policy },
    style_refs: [...(teller.style_refs || [])],
    style_rules: [...(teller.style_rules || [])],
  }
}

export function cloneImagePreset(preset: ImagePreset): ImagePreset {
  return { ...preset, tags: [...(preset.tags || [])] }
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

export function cloneOpeningSelector(item: OpeningSelectorModule): OpeningSelectorModule {
  return cloneJSON(item)
}

export function makeTellerPayload(draft: Teller, tagDraft: string): Partial<Teller> {
  return {
    ...draft,
    id: draft.id,
    tags: splitTags(tagDraft),
  }
}

export function makeImagePresetPayload(draft: ImagePreset, tagDraft: string): Partial<ImagePreset> {
  return {
    ...draft,
    id: draft.id,
    tags: splitTags(tagDraft),
  }
}

export function makeStoryDirectorPayload(draft: StoryDirector, tagDraft: string): Partial<StoryDirector> {
  const payload = cloneStoryDirector({
    ...draft,
    id: draft.id,
    tags: splitTags(tagDraft),
  })
  delete (payload as unknown as Record<string, unknown>).event_system
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
  }
  return payload
}

export function makeEventPackagePayload(draft: EventPackageModule, tagDraft: string): Partial<EventPackageModule> {
  const payload = cloneEventPackage({
    ...draft,
    id: draft.id,
    tags: splitTags(tagDraft),
  })
  delete (payload as unknown as Record<string, unknown>).event_system
  delete (payload as unknown as Record<string, unknown>).custom_events
  return payload
}

export function makeRuleSystemPayload(draft: RuleSystemModule, tagDraft: string): Partial<RuleSystemModule> {
  return {
    ...draft,
    id: draft.id,
    tags: splitTags(tagDraft),
  }
}

export function makeActorStatePayload(draft: ActorStateModule, tagDraft: string): Partial<ActorStateModule> {
  return {
    ...draft,
    id: draft.id,
    tags: splitTags(tagDraft),
  }
}

export function makeMemoryStructurePayload(draft: StoryMemoryStructureModule, tagDraft: string): Partial<StoryMemoryStructureModule> {
  return {
    ...draft,
    id: draft.id,
    tags: splitTags(tagDraft),
  }
}

export function makeOpeningSelectorPayload(draft: OpeningSelectorModule, tagDraft: string): Partial<OpeningSelectorModule> {
  return {
    ...draft,
    id: draft.id,
    tags: splitTags(tagDraft),
  }
}

export function newTellerDraft(): Partial<Teller> {
  const id = `custom-${Date.now()}`
  return {
    id,
    name: '自定义叙事风格',
    description: '新的叙事风格',
    random_event_rate: 0.15,
    style_refs: [],
    style_rules: [],
    tags: ['自定义'],
    context_policy: {
      creator: 'always',
      lore: 'relevant',
      runtime_state: 'always',
    },
    slots: [
      {
        id: 'identity',
        name: '系统提示',
        target: 'system',
        enabled: true,
        content: '你是一套自定义叙事风格。你要明确影响故事的文风倾向、角色反应、剧情裁定、节奏推进和长期叙事原则。',
      },
      {
        id: 'turn_context',
        name: '本轮上下文',
        target: 'turn_context',
        enabled: true,
        content: '每轮都要让用户行动带来具体后果，并主动制造符合叙事风格的反馈、阻碍、发现、NPC 反应、代价、暗线推进或新的行动入口。',
      },
      {
        id: 'state_memory',
        name: '记忆沉淀规则',
        target: 'state_memory',
        enabled: true,
        content: '记录本回合已经成立的关系变化、风险、线索、资源、暗线和可继续行动的入口。',
      },
    ],
  }
}

export function newStoryDirectorDraft(): Partial<StoryDirector> {
  return {
    id: `custom-director-${Date.now()}`,
    name: '自定义故事导演',
    description: '新的故事导演，组合叙事风格、事件包、TRPG 检定、状态系统、开局选择器和图像方案。',
    module_refs: {
      narrative_style_id: 'classic',
      event_package_ids: ['default'],
      rule_system_id: 'default',
      actor_state_id: 'default',
      memory_structure_id: 'default',
      opening_selector_id: 'default',
      image_preset_id: 'game-cg',
    },
    strategy: {
      enabled: true,
      mainline_strength: 'balanced',
      failure_policy: 'consequence',
      pacing_curve: 'goal-pressure-payoff',
      random_event_rate: 0.15,
      director_agent_mode: 'triggered',
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
    opening_selector: {
      enabled: true,
      trait_pools: [],
      initial_state_ops: [],
    },
    tags: ['自定义'],
    version: 2,
    custom: true,
  }
}

export function newEventPackageDraft(): Partial<EventPackageModule> {
  return {
    id: `custom-event-package-${Date.now()}`,
    name: '自定义事件包',
    description: '新的事件包，配置事件卡、强度、冷却和事件描述。',
    events: [],
    tags: ['自定义'],
    version: 1,
    custom: true,
  }
}

export function newRuleSystemDraft(): Partial<RuleSystemModule> {
  return {
    id: `custom-rule-${Date.now()}`,
    name: '自定义 TRPG 检定',
    description: '新的 TRPG 检定，配置检定模板、骰子、难度、资源消耗和结果状态变更。',
    trpg_system: {
      rule_templates: [],
    },
    tags: ['自定义'],
    version: 1,
    custom: true,
  }
}

export function newActorStateDraft(): Partial<ActorStateModule> {
  return {
    id: `custom-actor-state-${Date.now()}`,
    name: '自定义状态系统',
    description: '新的状态系统，配置关键角色模板、字段 schema 和初始 Actor。',
    actor_state: {
      templates: [
        {
          id: 'protagonist',
          name: '主角',
          description: '主角可计算状态模板。',
          fields: [
            { id: 'hp', path: 'resources.hp', name: '生命', type: 'number', default: 10, min: 0, max: 10, visibility: 'visible' },
          ],
        },
      ],
      initial_actors: [{ id: 'protagonist', name: '主角', template_id: 'protagonist', role: 'protagonist' }],
    },
    tags: ['自定义'],
    version: 1,
    custom: true,
  }
}

export function newMemoryStructureDraft(): Partial<StoryMemoryStructureModule> {
  return {
    id: `custom-memory-${Date.now()}`,
    name: '自定义记忆结构',
    description: '新的故事记忆结构，配置长期记忆分组、字段和整理要求。',
    structures: [],
    tags: ['自定义'],
    version: 1,
    custom: true,
  }
}

export function newOpeningSelectorDraft(): Partial<OpeningSelectorModule> {
  return {
    id: `custom-opening-${Date.now()}`,
    name: '自定义开局选择器',
    description: '新的开局选择器，配置词条池、初始状态变更和抽取规则。',
    opening_selector: {
      enabled: true,
      trait_pools: [],
      initial_state_ops: [],
    },
    tags: ['自定义'],
    version: 1,
    custom: true,
  }
}

export function newImagePresetDraft(): Partial<ImagePreset> {
  return {
    id: `custom-image-${Date.now()}`,
    name: '自定义图像方案',
    description: '新的图像风格方案',
    prompt: '描述画面风格、媒介、构图、镜头语言、光影、色彩、角色与环境呈现限制，以及需要避免的内容。',
    tags: ['自定义'],
    version: 1,
    custom: true,
  }
}

export function isPresetConfigResourceKind(kind: PresetResourceKind) {
  return kind === 'director' || kind === 'event' || kind === 'rule' || kind === 'actor-state' || kind === 'memory-structure' || kind === 'opening'
}

export function currentPresetBuiltinOverridden(kind: PresetResourceKind, drafts: PresetDrafts) {
  if (kind === 'director') return Boolean(drafts.director?.builtin_overridden)
  if (kind === 'image') return Boolean(drafts.image?.builtin_overridden)
  if (kind === 'event') return Boolean(drafts.event?.builtin_overridden)
  if (kind === 'rule') return Boolean(drafts.rule?.builtin_overridden)
  if (kind === 'actor-state') return Boolean(drafts.actorState?.builtin_overridden)
  if (kind === 'memory-structure') return Boolean(drafts.memoryStructure?.builtin_overridden)
  if (kind === 'opening') return Boolean(drafts.opening?.builtin_overridden)
  return Boolean(drafts.teller?.builtin_overridden)
}

export function presetEditorTitle(kind: PresetResourceKind, drafts: PresetDrafts, t: (key: string) => string) {
  if (kind === 'image') return drafts.image?.name || t('settingPanel.editor.defaultImagePreset')
  if (kind === 'director') return drafts.director?.name || t('settingPanel.editor.defaultStoryDirector')
  if (kind === 'event') return drafts.event?.name || t('settingPanel.editor.defaultEventPackage')
  if (kind === 'rule') return drafts.rule?.name || t('settingPanel.editor.defaultRuleSystem')
  if (kind === 'actor-state') return drafts.actorState?.name || t('settingPanel.editor.defaultActorState')
  if (kind === 'memory-structure') return drafts.memoryStructure?.name || t('settingPanel.editor.defaultMemoryStructure')
  if (kind === 'opening') return drafts.opening?.name || t('settingPanel.editor.defaultOpeningSelector')
  return drafts.teller?.name || t('settingPanel.editor.defaultTeller')
}

export function presetEditorSubtitle(kind: PresetResourceKind, drafts: PresetDrafts, t: (key: string) => string) {
  if (kind === 'image') return drafts.image?.description || t('settingPanel.editor.imagePresetSubtitle')
  if (kind === 'director') return drafts.director?.description || t('settingPanel.editor.storyDirectorSubtitle')
  if (kind === 'event') return drafts.event?.description || t('settingPanel.editor.eventPackageSubtitle')
  if (kind === 'rule') return drafts.rule?.description || t('settingPanel.editor.ruleSystemSubtitle')
  if (kind === 'actor-state') return drafts.actorState?.description || t('settingPanel.editor.actorStateSubtitle')
  if (kind === 'memory-structure') return drafts.memoryStructure?.description || t('settingPanel.editor.memoryStructureSubtitle')
  if (kind === 'opening') return drafts.opening?.description || t('settingPanel.editor.openingSelectorSubtitle')
  return drafts.teller?.description || t('settingPanel.editor.tellerSubtitle')
}

function cloneJSON<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}
