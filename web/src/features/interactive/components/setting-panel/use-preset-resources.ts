/** 方案预设 6 类资源的数据层：列表/选中 id/草稿 state、加载与外部同步、保存合并与刷新。 */
import { useEffect, useMemo, useRef, useState } from 'react'
import { getActorStates, getEventPackages, getImagePresets, getInteractiveTellers, getRuleSystems, getStoryDirectors } from '../../api'
import type { PresetResourceKind } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, Teller } from '../../types'
import { cloneActorState, cloneEventPackage, cloneImagePreset, cloneRuleSystem, cloneStoryDirector, cloneTeller, EMPTY_ACTOR_STATES, EMPTY_EVENT_PACKAGES, EMPTY_IMAGE_PRESETS, EMPTY_RULE_SYSTEMS, EMPTY_STORY_DIRECTORS, EMPTY_TELLERS, presetResourceDraftSignature, TELLER_CONFIG_AGENT_ENTRY_ID, type PresetDrafts } from './presetResources'

interface AutosavedListEntry {
  id: string
  signature: string
}

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
  const autosavedListEntriesRef = useRef<Partial<Record<PresetResourceKind, AutosavedListEntry>>>({})

  const [tellers, setTellers] = useState<Teller[]>(externalTellers)
  const [activeTellerId, setActiveTellerId] = useState('')
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

  const markAutosavedListEntry = (kind: PresetResourceKind, item: { id: string } & object) => {
    autosavedListEntriesRef.current[kind] = {
      id: item.id,
      signature: presetResourceDraftSignature(item),
    }
  }

  const clearAutosavedListEntry = (kind: PresetResourceKind) => {
    delete autosavedListEntriesRef.current[kind]
  }

  const shouldPreserveAutosavedDraft = (kind: PresetResourceKind, item: ({ id: string } & object) | null, draftId?: string) => {
    const autosavedEntry = autosavedListEntriesRef.current[kind]
    if (!autosavedEntry) return false
    if (
      !item
      || item.id !== autosavedEntry.id
      || draftId !== item.id
      || presetResourceDraftSignature(item) !== autosavedEntry.signature
    ) {
      delete autosavedListEntriesRef.current[kind]
      return false
    }
    return true
  }

  useEffect(() => {
    autosavedListEntriesRef.current = {}
  }, [workspace])

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
    const activeExternalTeller = externalTellers.find((teller) => teller.id === activeTellerId) || null
    if (!shouldPreserveAutosavedDraft('teller', activeExternalTeller, tellerDraft?.id)) {
      setTellerDraft(null)
      setActiveSlotId('')
    }
  }, [externalTellers, workspace])

  useEffect(() => {
    setStoryDirectors(externalStoryDirectors)
    setActiveStoryDirectorId((current) => {
      if (current && externalStoryDirectors.some((director) => director.id === current)) return current
      return externalStoryDirectors[0]?.id || ''
    })
    const activeExternalDirector = externalStoryDirectors.find((director) => director.id === activeStoryDirectorId) || null
    if (!shouldPreserveAutosavedDraft('director', activeExternalDirector, storyDirectorDraft?.id)) {
      setStoryDirectorDraft(null)
    }
  }, [externalStoryDirectors, workspace])

  useEffect(() => {
    setImagePresets(externalImagePresets)
    setActiveImagePresetId((current) => {
      if (current && externalImagePresets.some((preset) => preset.id === current)) return current
      return externalImagePresets[0]?.id || ''
    })
    const activeExternalPreset = externalImagePresets.find((preset) => preset.id === activeImagePresetId) || null
    if (!shouldPreserveAutosavedDraft('image', activeExternalPreset, imagePresetDraft?.id)) {
      setImagePresetDraft(null)
    }
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

  const mergeSavedTeller = (teller: Teller, preserveDraft = false) => {
    if (preserveDraft) markAutosavedListEntry('teller', teller)
    else clearAutosavedListEntry('teller')
    setTellers((current) => {
      const next = current.map((entry) => (entry.id === teller.id ? teller : entry))
      onTellersChange?.(next)
      return next
    })
  }

  const mergeSavedStoryDirector = (director: StoryDirector, preserveDraft = false) => {
    if (preserveDraft) markAutosavedListEntry('director', director)
    else clearAutosavedListEntry('director')
    setStoryDirectors((current) => {
      const next = current.map((entry) => (entry.id === director.id ? director : entry))
      onStoryDirectorsChange?.(next)
      return next
    })
  }

  const mergeSavedImagePreset = (preset: ImagePreset, preserveDraft = false) => {
    if (preserveDraft) markAutosavedListEntry('image', preset)
    else clearAutosavedListEntry('image')
    setImagePresets((current) => {
      const next = current.map((entry) => (entry.id === preset.id ? preset : entry))
      onImagePresetsChange?.(next)
      return next
    })
  }

  const mergeSavedEventPackage = (item: EventPackageModule, preserveDraft = false) => {
    if (preserveDraft) markAutosavedListEntry('event', item)
    else clearAutosavedListEntry('event')
    setEventPackages((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
  }

  const mergeSavedRuleSystem = (item: RuleSystemModule, preserveDraft = false) => {
    if (preserveDraft) markAutosavedListEntry('rule', item)
    else clearAutosavedListEntry('rule')
    setRuleSystems((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
  }

  const mergeSavedActorState = (item: ActorStateModule, preserveDraft = false) => {
    if (preserveDraft) markAutosavedListEntry('actor-state', item)
    else clearAutosavedListEntry('actor-state')
    setActorStates((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
  }

  const refreshTellers = async (nextActiveId?: string) => {
    clearAutosavedListEntry('teller')
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
    clearAutosavedListEntry('director')
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
    clearAutosavedListEntry('image')
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
    clearAutosavedListEntry('event')
    const data = await getEventPackages()
    setEventPackages(data)
    setActiveEventPackageId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((item) => item.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const refreshRuleSystems = async (nextActiveId?: string) => {
    clearAutosavedListEntry('rule')
    const data = await getRuleSystems()
    setRuleSystems(data)
    setActiveRuleSystemId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((item) => item.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const refreshActorStates = async (nextActiveId?: string) => {
    clearAutosavedListEntry('actor-state')
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
    shouldPreserveAutosavedDraft,
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
  teller: { resetBaseline: (draft: Teller | null) => void }
  director: { resetBaseline: (draft: StoryDirector | null) => void }
  image: { resetBaseline: (draft: ImagePreset | null) => void }
  event: { resetBaseline: (draft: EventPackageModule | null) => void }
  rule: { resetBaseline: (draft: RuleSystemModule | null) => void }
  'actor-state': { resetBaseline: (draft: ActorStateModule | null) => void }
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
    shouldPreserveAutosavedDraft,
  } = resources

  useEffect(() => {
    if (activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID) {
      setTellerDraft(null)
      autosaves.teller.resetBaseline(null)
      setActiveSlotId('')
      return
    }
    const teller = tellers.find((entry) => entry.id === activeTellerId) || null
    if (shouldPreserveAutosavedDraft('teller', teller, tellerDraft?.id)) return
    const nextDraft = teller ? cloneTeller(teller) : null
    setTellerDraft(nextDraft)
    setActiveSlotId((current) => {
      if (current && teller?.slots?.some((slot) => slot.id === current)) return current
      return teller?.slots?.[0]?.id || ''
    })
    autosaves.teller.resetBaseline(nextDraft)
  }, [activeTellerId, tellers, autosaves.teller.resetBaseline])

  useEffect(() => {
    const preset = imagePresets.find((entry) => entry.id === activeImagePresetId) || null
    if (shouldPreserveAutosavedDraft('image', preset, imagePresetDraft?.id)) return
    const nextDraft = preset ? cloneImagePreset(preset) : null
    setImagePresetDraft(nextDraft)
    autosaves.image.resetBaseline(nextDraft)
  }, [activeImagePresetId, imagePresets, autosaves.image.resetBaseline])

  useEffect(() => {
    const director = storyDirectors.find((entry) => entry.id === activeStoryDirectorId) || null
    if (shouldPreserveAutosavedDraft('director', director, storyDirectorDraft?.id)) return
    const nextDraft = director ? cloneStoryDirector(director) : null
    setStoryDirectorDraft(nextDraft)
    autosaves.director.resetBaseline(nextDraft)
  }, [activeStoryDirectorId, storyDirectors, autosaves.director.resetBaseline])

  useEffect(() => {
    const item = eventPackages.find((entry) => entry.id === activeEventPackageId) || null
    if (shouldPreserveAutosavedDraft('event', item, eventPackageDraft?.id)) return
    const nextDraft = item ? cloneEventPackage(item) : null
    setEventPackageDraft(nextDraft)
    autosaves.event.resetBaseline(nextDraft)
  }, [activeEventPackageId, eventPackages, autosaves.event.resetBaseline])

  useEffect(() => {
    const item = ruleSystems.find((entry) => entry.id === activeRuleSystemId) || null
    if (shouldPreserveAutosavedDraft('rule', item, ruleSystemDraft?.id)) return
    const nextDraft = item ? cloneRuleSystem(item) : null
    setRuleSystemDraft(nextDraft)
    autosaves.rule.resetBaseline(nextDraft)
  }, [activeRuleSystemId, ruleSystems, autosaves.rule.resetBaseline])

  useEffect(() => {
    const item = actorStates.find((entry) => entry.id === activeActorStateId) || null
    if (shouldPreserveAutosavedDraft('actor-state', item, actorStateDraft?.id)) return
    const nextDraft = item ? cloneActorState(item) : null
    setActorStateDraft(nextDraft)
    autosaves['actor-state'].resetBaseline(nextDraft)
  }, [activeActorStateId, actorStates, autosaves['actor-state'].resetBaseline])
}
