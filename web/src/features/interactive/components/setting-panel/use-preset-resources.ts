/** 方案预设 6 类资源的数据层：列表/选中 id/草稿 state、加载与外部同步、保存合并与刷新。 */
import { useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { rebaseJSONValue } from '@/lib/three-way-rebase'
import { getActorStates, getEventPackages, getGamePlanningTemplates, getImagePresets, getInteractiveTellers, getRuleSystems } from '../../api'
import { PRESET_RESOURCE_SCOPE } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, GamePlanningTemplate, ImagePreset, RuleSystemModule, Teller } from '../../types'
import { cloneActorState, cloneEventPackage, cloneImagePreset, cloneRuleSystem, cloneStoryDirector, cloneTeller, EMPTY_ACTOR_STATES, EMPTY_EVENT_PACKAGES, EMPTY_IMAGE_PRESETS, EMPTY_RULE_SYSTEMS, EMPTY_STORY_DIRECTORS, EMPTY_TELLERS, type PresetDrafts } from './presetResources'
import { presetResourceRevision } from './usePresetResourceAutosave'

/** 外部传入列表优先；未传入时加载用户级全局预设目录。 */
export function usePresetResources({
  externalTellers = EMPTY_TELLERS,
  externalStoryDirectors = EMPTY_STORY_DIRECTORS,
  externalImagePresets = EMPTY_IMAGE_PRESETS,
  onTellersChange,
  onStoryDirectorsChange,
  onImagePresetsChange,
}: {
  externalTellers?: Teller[]
  externalStoryDirectors?: GamePlanningTemplate[]
  externalImagePresets?: ImagePreset[]
  onTellersChange?: (tellers: Teller[]) => void
  onStoryDirectorsChange?: (directors: GamePlanningTemplate[]) => void
  onImagePresetsChange?: (presets: ImagePreset[]) => void
}) {
  const [tellers, setTellers] = useState<Teller[]>(externalTellers)
  const [activeTellerId, setActiveTellerId] = useState(externalTellers[0]?.id || '')
  const [tellerDraft, setTellerDraft] = useState<Teller | null>(null)
  const [activeSlotId, setActiveSlotId] = useState('')
  const [storyDirectors, setStoryDirectors] = useState<GamePlanningTemplate[]>(externalStoryDirectors)
  const [activeStoryDirectorId, setActiveStoryDirectorId] = useState('')
  const [storyDirectorDraft, setStoryDirectorDraft] = useState<GamePlanningTemplate | null>(null)
  const [imagePresets, setImagePresets] = useState<ImagePreset[]>(externalImagePresets)
  const [activeImagePresetId, setActiveImagePresetId] = useState('')
  const [imagePresetDraft, setImagePresetDraft] = useState<ImagePreset | null>(null)
  const [eventPackages, setEventPackages] = useState<EventPackageModule[]>(EMPTY_EVENT_PACKAGES)
  const [activeEventPackageId, setActiveEventPackageId] = useState('')
  const [eventPackageDraft, setEventPackageDraft] = useState<EventPackageModule | null>(null)
  const [ruleSystems, setRuleSystems] = useState<RuleSystemModule[]>(EMPTY_RULE_SYSTEMS)
  const [activeRuleSystemId, setActiveRuleSystemId] = useState('')
  const [ruleSystemDraft, setRuleSystemDraft] = useState<RuleSystemModule | null>(null)
  const [actorStates, setActorStates] = useState<ActorStateModule[]>(EMPTY_ACTOR_STATES)
  const [activeActorStateId, setActiveActorStateId] = useState('')
  const [actorStateDraft, setActorStateDraft] = useState<ActorStateModule | null>(null)

  useEffect(() => {
    if (onTellersChange || externalTellers.length > 0) return
    let cancelled = false
    getInteractiveTellers()
      .then((data) => {
        if (cancelled) return
        setTellers(data)
        setActiveTellerId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setTellers([])
      })
    return () => {
      cancelled = true
    }
  }, [externalTellers.length, onTellersChange])

  useEffect(() => {
    if (onStoryDirectorsChange || externalStoryDirectors.length > 0) return
    let cancelled = false
    getGamePlanningTemplates()
      .then((data) => {
        if (cancelled) return
        setStoryDirectors(data)
        setActiveStoryDirectorId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setStoryDirectors([])
      })
    return () => {
      cancelled = true
    }
  }, [externalStoryDirectors.length, onStoryDirectorsChange])

  useEffect(() => {
    if (onImagePresetsChange || externalImagePresets.length > 0) return
    let cancelled = false
    getImagePresets()
      .then((data) => {
        if (cancelled) return
        setImagePresets(data)
        setActiveImagePresetId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setImagePresets([])
      })
    return () => {
      cancelled = true
    }
  }, [externalImagePresets.length, onImagePresetsChange])

  useEffect(() => {
    let cancelled = false
    getEventPackages()
      .then((data) => {
        if (cancelled) return
        setEventPackages(data)
        setActiveEventPackageId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setEventPackages([])
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    getRuleSystems()
      .then((data) => {
        if (cancelled) return
        setRuleSystems(data)
        setActiveRuleSystemId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setRuleSystems([])
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    getActorStates()
      .then((data) => {
        if (cancelled) return
        setActorStates(data)
        setActiveActorStateId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setActorStates([])
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    setTellers(externalTellers)
    setActiveTellerId((current) => {
      if (current && externalTellers.some((teller) => teller.id === current)) return current
      return externalTellers[0]?.id || ''
    })
  }, [externalTellers])

  useEffect(() => {
    setStoryDirectors(externalStoryDirectors)
    setActiveStoryDirectorId((current) => {
      if (current && externalStoryDirectors.some((director) => director.id === current)) return current
      return externalStoryDirectors[0]?.id || ''
    })
  }, [externalStoryDirectors])

  useEffect(() => {
    setImagePresets(externalImagePresets)
    setActiveImagePresetId((current) => {
      if (current && externalImagePresets.some((preset) => preset.id === current)) return current
      return externalImagePresets[0]?.id || ''
    })
  }, [externalImagePresets])

  const presetDrafts: PresetDrafts = useMemo(() => ({
    teller: tellerDraft,
    director: storyDirectorDraft,
    image: imagePresetDraft,
    event: eventPackageDraft,
    rule: ruleSystemDraft,
    actorState: actorStateDraft,
  }), [actorStateDraft, eventPackageDraft, imagePresetDraft, ruleSystemDraft, storyDirectorDraft, tellerDraft])

  const mergeSavedTeller = (teller: Teller) => {
    const next = tellers.map((entry) => (entry.id === teller.id ? teller : entry))
    setTellers(next)
    onTellersChange?.(next)
  }

  const mergeSavedStoryDirector = (director: GamePlanningTemplate) => {
    const next = storyDirectors.map((entry) => (entry.id === director.id ? director : entry))
    setStoryDirectors(next)
    onStoryDirectorsChange?.(next)
  }

  const mergeSavedImagePreset = (preset: ImagePreset) => {
    const next = imagePresets.map((entry) => (entry.id === preset.id ? preset : entry))
    setImagePresets(next)
    onImagePresetsChange?.(next)
  }

  const mergeSavedEventPackage = (item: EventPackageModule) => {
    setEventPackages((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
  }

  const mergeSavedRuleSystem = (item: RuleSystemModule) => {
    setRuleSystems((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
  }

  const mergeSavedActorState = (item: ActorStateModule) => {
    setActorStates((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
  }

  const refreshTellers = async (nextActiveId?: string) => {
    const data = await getInteractiveTellers()
    setTellers(data)
    onTellersChange?.(data)
    setActiveTellerId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((teller) => teller.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const refreshStoryDirectors = async (nextActiveId?: string) => {
    const data = await getGamePlanningTemplates()
    setStoryDirectors(data)
    onStoryDirectorsChange?.(data)
    setActiveStoryDirectorId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((director) => director.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const refreshImagePresets = async (nextActiveId?: string) => {
    const data = await getImagePresets()
    setImagePresets(data)
    onImagePresetsChange?.(data)
    setActiveImagePresetId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((preset) => preset.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const refreshEventPackages = async (nextActiveId?: string) => {
    const data = await getEventPackages()
    setEventPackages(data)
    setActiveEventPackageId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((item) => item.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const refreshRuleSystems = async (nextActiveId?: string) => {
    const data = await getRuleSystems()
    setRuleSystems(data)
    setActiveRuleSystemId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((item) => item.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const refreshActorStates = async (nextActiveId?: string) => {
    const data = await getActorStates()
    setActorStates(data)
    setActiveActorStateId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((item) => item.id === current)) return current
      return data[0]?.id || ''
    })
  }

  return {
    tellers,
    activeTellerId,
    setActiveTellerId,
    tellerDraft,
    setTellerDraft,
    activeSlotId,
    setActiveSlotId,
    storyDirectors,
    activeStoryDirectorId,
    setActiveStoryDirectorId,
    storyDirectorDraft,
    setStoryDirectorDraft,
    imagePresets,
    activeImagePresetId,
    setActiveImagePresetId,
    imagePresetDraft,
    setImagePresetDraft,
    eventPackages,
    activeEventPackageId,
    setActiveEventPackageId,
    eventPackageDraft,
    setEventPackageDraft,
    ruleSystems,
    activeRuleSystemId,
    setActiveRuleSystemId,
    ruleSystemDraft,
    setRuleSystemDraft,
    actorStates,
    activeActorStateId,
    setActiveActorStateId,
    actorStateDraft,
    setActorStateDraft,
    presetDrafts,
    mergeSavedTeller,
    mergeSavedStoryDirector,
    mergeSavedImagePreset,
    mergeSavedEventPackage,
    mergeSavedRuleSystem,
    mergeSavedActorState,
    refreshTellers,
    refreshStoryDirectors,
    refreshImagePresets,
    refreshEventPackages,
    refreshRuleSystems,
    refreshActorStates,
  }
}

export type PresetResources = ReturnType<typeof usePresetResources>

interface DraftSyncAutosaves {
  teller: DraftSyncAutosave<Teller>
  director: DraftSyncAutosave<GamePlanningTemplate>
  image: DraftSyncAutosave<ImagePreset>
  event: DraftSyncAutosave<EventPackageModule>
  rule: DraftSyncAutosave<RuleSystemModule>
  'actor-state': DraftSyncAutosave<ActorStateModule>
}

interface DraftSyncAutosave<T> {
  isBaselineAcknowledged: (draft: T | null) => boolean
  resetBaseline: (draft: T | null) => void
}

/** 草稿同步：activeId/列表变化时克隆草稿并对齐 autosave 基线（config agent 伪条目清空草稿）。 */
export function usePresetDraftSync(resources: PresetResources, autosaves: DraftSyncAutosaves) {
  const {
    tellers,
    activeTellerId,
    tellerDraft,
    setTellerDraft,
    setActiveSlotId,
    storyDirectors,
    activeStoryDirectorId,
    storyDirectorDraft,
    setStoryDirectorDraft,
    imagePresets,
    activeImagePresetId,
    imagePresetDraft,
    setImagePresetDraft,
    eventPackages,
    activeEventPackageId,
    eventPackageDraft,
    setEventPackageDraft,
    ruleSystems,
    activeRuleSystemId,
    ruleSystemDraft,
    setRuleSystemDraft,
    actorStates,
    activeActorStateId,
    actorStateDraft,
    setActorStateDraft,
  } = resources

  const teller = tellers.find((entry) => entry.id === activeTellerId) || null
  const director = storyDirectors.find((entry) => entry.id === activeStoryDirectorId) || null
  const imagePreset = imagePresets.find((entry) => entry.id === activeImagePresetId) || null
  const eventPackage = eventPackages.find((entry) => entry.id === activeEventPackageId) || null
  const ruleSystem = ruleSystems.find((entry) => entry.id === activeRuleSystemId) || null
  const actorState = actorStates.find((entry) => entry.id === activeActorStateId) || null

  useRebasedPresetDraft({ resource: 'teller', scopeKey: PRESET_RESOURCE_SCOPE, baseline: teller, draft: tellerDraft, setDraft: setTellerDraft, clone: cloneTeller, ...autosaves.teller })
  useRebasedPresetDraft({
    resource: 'game_planning',
    scopeKey: PRESET_RESOURCE_SCOPE,
    baseline: director,
    draft: storyDirectorDraft,
    setDraft: setStoryDirectorDraft,
    clone: cloneStoryDirector,
    ...autosaves.director,
  })
  useRebasedPresetDraft({ resource: 'image_preset', scopeKey: PRESET_RESOURCE_SCOPE, baseline: imagePreset, draft: imagePresetDraft, setDraft: setImagePresetDraft, clone: cloneImagePreset, ...autosaves.image })
  useRebasedPresetDraft({ resource: 'event_package', scopeKey: PRESET_RESOURCE_SCOPE, baseline: eventPackage, draft: eventPackageDraft, setDraft: setEventPackageDraft, clone: cloneEventPackage, ...autosaves.event })
  useRebasedPresetDraft({ resource: 'rule_system', scopeKey: PRESET_RESOURCE_SCOPE, baseline: ruleSystem, draft: ruleSystemDraft, setDraft: setRuleSystemDraft, clone: cloneRuleSystem, ...autosaves.rule })
  useRebasedPresetDraft({ resource: 'actor_state', scopeKey: PRESET_RESOURCE_SCOPE, baseline: actorState, draft: actorStateDraft, setDraft: setActorStateDraft, clone: cloneActorState, ...autosaves['actor-state'] })

  useEffect(() => {
    setActiveSlotId((current) => {
      if (current && tellerDraft?.slots?.some((slot) => slot.id === current)) return current
      return tellerDraft?.slots?.[0]?.id || ''
    })
  }, [setActiveSlotId, tellerDraft])
}

function useRebasedPresetDraft<T extends { id: string; revision?: string; updated_at?: string }>({
  resource,
  scopeKey,
  baseline,
  draft,
  setDraft,
  clone,
  isBaselineAcknowledged,
  resetBaseline,
}: {
  resource: string
  scopeKey: string
  baseline: T | null
  draft: T | null
  setDraft: Dispatch<SetStateAction<T | null>>
  clone: (value: T) => T
  isBaselineAcknowledged: (draft: T | null) => boolean
  resetBaseline: (draft: T | null) => void
}) {
  const previousBaselineRef = useRef<T | null>(null)
  const scopeKeyRef = useRef(scopeKey)
  const draftRef = useRef(draft)
  draftRef.current = draft

  useEffect(() => {
    let cancelled = false
    if (scopeKeyRef.current !== scopeKey) {
      scopeKeyRef.current = scopeKey
      previousBaselineRef.current = null
    }
    const nextBaseline = baseline ? clone(baseline) : null
    const previousBaseline = previousBaselineRef.current
    const currentDraft = draftRef.current
    const baselineAcknowledged = isBaselineAcknowledged(nextBaseline)
    void (async () => {
      let rebasedFromDraft = currentDraft
      let nextDraft = nextBaseline
        ? previousBaseline?.id === nextBaseline.id && currentDraft?.id === nextBaseline.id
          ? baselineAcknowledged
            ? rebaseJSONValue(previousBaseline, currentDraft, nextBaseline)
            : await rebaseJSONWithRecovery({
                resource,
                scope: scopeKey,
                id: nextBaseline.id,
                baseline: { revision: presetResourceRevision(previousBaseline), value: previousBaseline },
                local: { revision: presetResourceRevision(previousBaseline), value: currentDraft },
                external: { revision: presetResourceRevision(nextBaseline), value: nextBaseline },
              })
          : nextBaseline
        : null
      // Conflict preservation can await storage. Fold every edit made during that
      // wait over the reconciled result before committing it, and repeat if a
      // second overlap also had to be archived.
      while (!cancelled && nextDraft && rebasedFromDraft?.id === nextDraft.id) {
        const latestDraft = draftRef.current
        if (!latestDraft || latestDraft.id !== nextDraft.id || Object.is(latestDraft, rebasedFromDraft)) break
        nextDraft = await rebaseJSONWithRecovery({
          resource,
          scope: scopeKey,
          id: nextDraft.id,
          baseline: { revision: presetResourceRevision(rebasedFromDraft), value: rebasedFromDraft },
          local: { revision: presetResourceRevision(rebasedFromDraft), value: latestDraft },
          external: { revision: nextBaseline ? presetResourceRevision(nextBaseline) : undefined, value: nextDraft },
        })
        rebasedFromDraft = latestDraft
      }
      if (cancelled) return
      previousBaselineRef.current = nextBaseline
      resetBaseline(nextBaseline)
      setDraft(nextDraft)
    })().catch((error) => console.error(`[preset-autosave] failed to reconcile external ${resource} update`, error))
    return () => {
      cancelled = true
    }
  }, [baseline, clone, isBaselineAcknowledged, resetBaseline, resource, scopeKey, setDraft])
}
