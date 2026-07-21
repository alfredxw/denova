/** 方案预设 6 类资源的数据层：列表/选中 id/草稿 state、加载与外部同步、保存合并与刷新。 */
import { useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { rebaseJSONValue } from '@/lib/three-way-rebase'
import { getActorStates, getEventPackages, getImagePresets, getInteractiveTellers, getRuleSystems, getStoryDirectors } from '../../api'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, Teller } from '../../types'
import { cloneActorState, cloneEventPackage, cloneImagePreset, cloneRuleSystem, cloneStoryDirector, cloneTeller, EMPTY_ACTOR_STATES, EMPTY_EVENT_PACKAGES, EMPTY_IMAGE_PRESETS, EMPTY_RULE_SYSTEMS, EMPTY_STORY_DIRECTORS, EMPTY_TELLERS, TELLER_CONFIG_AGENT_ENTRY_ID, type PresetDrafts } from './presetResources'

/** 外部传入列表优先；未传入时按 workspace 自行加载。 */
export function usePresetResources({
  workspace,
  externalTellers = EMPTY_TELLERS,
  externalStoryDirectors = EMPTY_STORY_DIRECTORS,
  externalImagePresets = EMPTY_IMAGE_PRESETS,
  onTellersChange,
  onStoryDirectorsChange,
  onImagePresetsChange,
}: {
  workspace: string
  externalTellers?: Teller[]
  externalStoryDirectors?: StoryDirector[]
  externalImagePresets?: ImagePreset[]
  onTellersChange?: (tellers: Teller[]) => void
  onStoryDirectorsChange?: (directors: StoryDirector[]) => void
  onImagePresetsChange?: (presets: ImagePreset[]) => void
}) {
  const [tellers, setTellers] = useState<Teller[]>(externalTellers)
  const [activeTellerId, setActiveTellerId] = useState(TELLER_CONFIG_AGENT_ENTRY_ID)
  const [tellerDraft, setTellerDraft] = useState<Teller | null>(null)
  const [activeSlotId, setActiveSlotId] = useState('')
  const [storyDirectors, setStoryDirectors] = useState<StoryDirector[]>(externalStoryDirectors)
  const [activeStoryDirectorId, setActiveStoryDirectorId] = useState('')
  const [storyDirectorDraft, setStoryDirectorDraft] = useState<StoryDirector | null>(null)
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
    if (onTellersChange || externalTellers.length > 0 || !workspace) return
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
  }, [externalTellers.length, onTellersChange, workspace])

  useEffect(() => {
    if (onStoryDirectorsChange || externalStoryDirectors.length > 0 || !workspace) return
    let cancelled = false
    getStoryDirectors()
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
  }, [externalStoryDirectors.length, onStoryDirectorsChange, workspace])

  useEffect(() => {
    if (onImagePresetsChange || externalImagePresets.length > 0 || !workspace) return
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
  }, [externalImagePresets.length, onImagePresetsChange, workspace])

  useEffect(() => {
    if (!workspace) return
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
  }, [workspace])

  useEffect(() => {
    if (!workspace) return
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
  }, [workspace])

  useEffect(() => {
    if (!workspace) return
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
  }, [workspace])

  useEffect(() => {
    setTellers(externalTellers)
    setActiveTellerId((current) => {
      if (current === TELLER_CONFIG_AGENT_ENTRY_ID) return current
      if (current && externalTellers.some((teller) => teller.id === current)) return current
      return externalTellers[0]?.id || ''
    })
  }, [externalTellers, workspace])

  useEffect(() => {
    setStoryDirectors(externalStoryDirectors)
    setActiveStoryDirectorId((current) => {
      if (current && externalStoryDirectors.some((director) => director.id === current)) return current
      return externalStoryDirectors[0]?.id || ''
    })
  }, [externalStoryDirectors, workspace])

  useEffect(() => {
    setImagePresets(externalImagePresets)
    setActiveImagePresetId((current) => {
      if (current && externalImagePresets.some((preset) => preset.id === current)) return current
      return externalImagePresets[0]?.id || ''
    })
  }, [externalImagePresets, workspace])

  useEffect(() => {
    setActiveEventPackageId((current) => {
      if (current && eventPackages.some((item) => item.id === current)) return current
      return eventPackages[0]?.id || ''
    })
    setEventPackageDraft(null)
  }, [workspace])

  useEffect(() => {
    setActiveRuleSystemId((current) => {
      if (current && ruleSystems.some((item) => item.id === current)) return current
      return ruleSystems[0]?.id || ''
    })
    setRuleSystemDraft(null)
  }, [workspace])

  useEffect(() => {
    setActiveActorStateId((current) => {
      if (current && actorStates.some((item) => item.id === current)) return current
      return actorStates[0]?.id || ''
    })
    setActorStateDraft(null)
  }, [workspace])

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

  const mergeSavedStoryDirector = (director: StoryDirector) => {
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
      if (current === TELLER_CONFIG_AGENT_ENTRY_ID) return current
      if (current && data.some((teller) => teller.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const refreshStoryDirectors = async (nextActiveId?: string) => {
    const data = await getStoryDirectors()
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
    workspace,
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
  director: DraftSyncAutosave<StoryDirector>
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
    workspace,
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

  const teller = activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID
    ? null
    : tellers.find((entry) => entry.id === activeTellerId) || null
  const director = storyDirectors.find((entry) => entry.id === activeStoryDirectorId) || null
  const imagePreset = imagePresets.find((entry) => entry.id === activeImagePresetId) || null
  const eventPackage = eventPackages.find((entry) => entry.id === activeEventPackageId) || null
  const ruleSystem = ruleSystems.find((entry) => entry.id === activeRuleSystemId) || null
  const actorState = actorStates.find((entry) => entry.id === activeActorStateId) || null

  useRebasedPresetDraft({ resource: 'teller', scopeKey: workspace, baseline: teller, draft: tellerDraft, setDraft: setTellerDraft, clone: cloneTeller, ...autosaves.teller })
  useRebasedPresetDraft({ resource: 'story_director', scopeKey: workspace, baseline: director, draft: storyDirectorDraft, setDraft: setStoryDirectorDraft, clone: cloneStoryDirector, ...autosaves.director })
  useRebasedPresetDraft({ resource: 'image_preset', scopeKey: workspace, baseline: imagePreset, draft: imagePresetDraft, setDraft: setImagePresetDraft, clone: cloneImagePreset, ...autosaves.image })
  useRebasedPresetDraft({ resource: 'event_package', scopeKey: workspace, baseline: eventPackage, draft: eventPackageDraft, setDraft: setEventPackageDraft, clone: cloneEventPackage, ...autosaves.event })
  useRebasedPresetDraft({ resource: 'rule_system', scopeKey: workspace, baseline: ruleSystem, draft: ruleSystemDraft, setDraft: setRuleSystemDraft, clone: cloneRuleSystem, ...autosaves.rule })
  useRebasedPresetDraft({ resource: 'actor_state', scopeKey: workspace, baseline: actorState, draft: actorStateDraft, setDraft: setActorStateDraft, clone: cloneActorState, ...autosaves['actor-state'] })

  useEffect(() => {
    setActiveSlotId((current) => {
      if (current && tellerDraft?.slots?.some((slot) => slot.id === current)) return current
      return tellerDraft?.slots?.[0]?.id || ''
    })
  }, [setActiveSlotId, tellerDraft])
}

function useRebasedPresetDraft<T extends { id: string; updated_at?: string }>({
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
                baseline: { revision: previousBaseline.updated_at, value: previousBaseline },
                local: { revision: previousBaseline.updated_at, value: currentDraft },
                external: { revision: nextBaseline.updated_at, value: nextBaseline },
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
          baseline: { revision: rebasedFromDraft.updated_at, value: rebasedFromDraft },
          local: { revision: rebasedFromDraft.updated_at, value: latestDraft },
          external: { revision: nextBaseline?.updated_at, value: nextDraft },
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
