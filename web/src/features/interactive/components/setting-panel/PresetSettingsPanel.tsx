import { useEffect, useMemo, useRef, useState } from 'react'
import { Loader2, PanelLeft, RotateCcw, Save, SlidersHorizontal, Sparkles, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { createActorState, createEventPackage, createImagePreset, createInteractiveTeller, createOpeningSelector, createRuleSystem, createStoryDirector, createStoryMemoryStructure, deleteActorState, deleteEventPackage, deleteImagePreset, deleteInteractiveTeller, deleteOpeningSelector, deleteRuleSystem, deleteStoryDirector, deleteStoryMemoryStructurePreset, getActorStates, getEventPackages, getImagePresets, getInteractiveTellers, getOpeningSelectors, getRuleSystems, getStoryDirectors, getStoryMemoryStructures, updateActorState, updateEventPackage, updateImagePreset, updateInteractiveTeller, updateOpeningSelector, updateRuleSystem, updateStoryDirector, updateStoryMemoryStructure } from '../../api'
import type { PresetResourceKind, PresetUsageMode } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, ImagePreset, OpeningSelectorModule, RuleSystemModule, StoryDirector, StoryMemoryStructureModule, Teller } from '../../types'
import { ActorStateEditor, EventPackageEditor, ImagePresetEditor, OpeningSelectorEditor, RuleSystemEditor, StoryMemoryStructureEditor, TellerDirectory } from '../SettingPanelSections'
import { TellerEditor } from '../SettingPanelTellerEditor'
import { StoryDirectorEditor } from '../story-director/StoryDirectorEditor'
import { usePresetResourceAutosave } from './usePresetResourceAutosave'
import { cloneActorState, cloneEventPackage, cloneImagePreset, cloneMemoryStructure, cloneOpeningSelector, cloneRuleSystem, cloneStoryDirector, cloneTeller, currentPresetBuiltinOverridden, EMPTY_ACTOR_STATES, EMPTY_EVENT_PACKAGES, EMPTY_IMAGE_PRESETS, EMPTY_MEMORY_STRUCTURES, EMPTY_OPENING_SELECTORS, EMPTY_RULE_SYSTEMS, EMPTY_STORY_DIRECTORS, EMPTY_TELLERS, isPresetConfigResourceKind, makeActorStatePayload, makeEventPackagePayload, makeImagePresetPayload, makeMemoryStructurePayload, makeOpeningSelectorPayload, makeRuleSystemPayload, makeStoryDirectorPayload, makeTellerPayload, newActorStateDraft, newEventPackageDraft, newImagePresetDraft, newMemoryStructureDraft, newOpeningSelectorDraft, newRuleSystemDraft, newStoryDirectorDraft, newTellerDraft, presetEditorSubtitle, presetEditorTitle, presetResourceDraftSignature, PRESET_DELETE_COPY, TELLER_CONFIG_AGENT_ENTRY_ID, type PresetDeleteTarget, type PresetDrafts } from './presetResources'

interface PresetSettingsPanelProps {
  workspace: string
  tellers?: Teller[]
  storyDirectors?: StoryDirector[]
  imagePresets?: ImagePreset[]
  presetFocus?: { nonce: number; kind: PresetResourceKind; id?: string }
  presetUsageMode?: PresetUsageMode
  onTellersChange?: (tellers: Teller[]) => void
  onStoryDirectorsChange?: (directors: StoryDirector[]) => void
  onImagePresetsChange?: (presets: ImagePreset[]) => void
  embedded?: boolean
}

interface AutosaveController {
  cancelPending: () => void
  flushPending: () => Promise<unknown> | null
  saveNow: (mode: 'manual' | 'auto') => Promise<unknown>
}

const actionButtonClassName = 'nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

export function PresetSettingsPanel({
  workspace,
  tellers: externalTellers = EMPTY_TELLERS,
  storyDirectors: externalStoryDirectors = EMPTY_STORY_DIRECTORS,
  imagePresets: externalImagePresets = EMPTY_IMAGE_PRESETS,
  presetFocus,
  presetUsageMode = 'game',
  onTellersChange,
  onStoryDirectorsChange,
  onImagePresetsChange,
  embedded = false,
}: PresetSettingsPanelProps) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const [deletePresetTarget, setDeletePresetTarget] = useState<PresetDeleteTarget | null>(null)
  const [presetConfigValid, setPresetConfigValid] = useState(true)
  const presetConfigValidRef = useRef(true)
  const presetFocusNonceRef = useRef<number | null>(null)

  const [presetResourceKind, setPresetResourceKind] = useState<PresetResourceKind>('teller')
  const [tellers, setTellers] = useState<Teller[]>(externalTellers)
  const [activeTellerId, setActiveTellerId] = useState('')
  const [tellerDraft, setTellerDraft] = useState<Teller | null>(null)
  const [tellerTagDraft, setTellerTagDraft] = useState('')
  const [activeSlotId, setActiveSlotId] = useState('')
  const [storyDirectors, setStoryDirectors] = useState<StoryDirector[]>(externalStoryDirectors)
  const [activeStoryDirectorId, setActiveStoryDirectorId] = useState('')
  const [storyDirectorDraft, setStoryDirectorDraft] = useState<StoryDirector | null>(null)
  const [storyDirectorTagDraft, setStoryDirectorTagDraft] = useState('')
  const [imagePresets, setImagePresets] = useState<ImagePreset[]>(externalImagePresets)
  const [activeImagePresetId, setActiveImagePresetId] = useState('')
  const [imagePresetDraft, setImagePresetDraft] = useState<ImagePreset | null>(null)
  const [imagePresetTagDraft, setImagePresetTagDraft] = useState('')
  const [eventPackages, setEventPackages] = useState<EventPackageModule[]>(EMPTY_EVENT_PACKAGES)
  const [activeEventPackageId, setActiveEventPackageId] = useState('')
  const [eventPackageDraft, setEventPackageDraft] = useState<EventPackageModule | null>(null)
  const [eventPackageTagDraft, setEventPackageTagDraft] = useState('')
  const [ruleSystems, setRuleSystems] = useState<RuleSystemModule[]>(EMPTY_RULE_SYSTEMS)
  const [activeRuleSystemId, setActiveRuleSystemId] = useState('')
  const [ruleSystemDraft, setRuleSystemDraft] = useState<RuleSystemModule | null>(null)
  const [ruleSystemTagDraft, setRuleSystemTagDraft] = useState('')
  const [actorStates, setActorStates] = useState<ActorStateModule[]>(EMPTY_ACTOR_STATES)
  const [activeActorStateId, setActiveActorStateId] = useState('')
  const [actorStateDraft, setActorStateDraft] = useState<ActorStateModule | null>(null)
  const [actorStateTagDraft, setActorStateTagDraft] = useState('')
  const [memoryStructures, setMemoryStructures] = useState<StoryMemoryStructureModule[]>(EMPTY_MEMORY_STRUCTURES)
  const [activeMemoryStructureId, setActiveMemoryStructureId] = useState('')
  const [memoryStructureDraft, setMemoryStructureDraft] = useState<StoryMemoryStructureModule | null>(null)
  const [memoryStructureTagDraft, setMemoryStructureTagDraft] = useState('')
  const [openingSelectors, setOpeningSelectors] = useState<OpeningSelectorModule[]>(EMPTY_OPENING_SELECTORS)
  const [activeOpeningSelectorId, setActiveOpeningSelectorId] = useState('')
  const [openingSelectorDraft, setOpeningSelectorDraft] = useState<OpeningSelectorModule | null>(null)
  const [openingSelectorTagDraft, setOpeningSelectorTagDraft] = useState('')

  useEffect(() => {
    presetConfigValidRef.current = presetConfigValid
  }, [presetConfigValid])

  useEffect(() => {
    setPresetConfigValid(true)
  }, [activeActorStateId, activeEventPackageId, activeMemoryStructureId, activeOpeningSelectorId, activeRuleSystemId, activeStoryDirectorId, presetResourceKind])

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
    if (!workspace) return
    let cancelled = false
    getStoryMemoryStructures()
      .then((data) => {
        if (cancelled) return
        setMemoryStructures(data)
        setActiveMemoryStructureId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setMemoryStructures([])
      })
    return () => {
      cancelled = true
    }
  }, [workspace])

  useEffect(() => {
    if (!workspace) return
    let cancelled = false
    getOpeningSelectors()
      .then((data) => {
        if (cancelled) return
        setOpeningSelectors(data)
        setActiveOpeningSelectorId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setOpeningSelectors([])
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
    setTellerDraft(null)
    setTellerTagDraft('')
    setActiveSlotId('')
  }, [externalTellers, workspace])

  useEffect(() => {
    setStoryDirectors(externalStoryDirectors)
    setActiveStoryDirectorId((current) => {
      if (current && externalStoryDirectors.some((director) => director.id === current)) return current
      return externalStoryDirectors[0]?.id || ''
    })
    setStoryDirectorDraft(null)
    setStoryDirectorTagDraft('')
  }, [externalStoryDirectors, workspace])

  useEffect(() => {
    setImagePresets(externalImagePresets)
    setActiveImagePresetId((current) => {
      if (current && externalImagePresets.some((preset) => preset.id === current)) return current
      return externalImagePresets[0]?.id || ''
    })
    setImagePresetDraft(null)
    setImagePresetTagDraft('')
  }, [externalImagePresets, workspace])

  useEffect(() => {
    setActiveEventPackageId((current) => {
      if (current && eventPackages.some((item) => item.id === current)) return current
      return eventPackages[0]?.id || ''
    })
    setEventPackageDraft(null)
    setEventPackageTagDraft('')
  }, [workspace])

  useEffect(() => {
    setActiveRuleSystemId((current) => {
      if (current && ruleSystems.some((item) => item.id === current)) return current
      return ruleSystems[0]?.id || ''
    })
    setRuleSystemDraft(null)
    setRuleSystemTagDraft('')
  }, [workspace])

  useEffect(() => {
    setActiveActorStateId((current) => {
      if (current && actorStates.some((item) => item.id === current)) return current
      return actorStates[0]?.id || ''
    })
    setActorStateDraft(null)
    setActorStateTagDraft('')
  }, [workspace])

  useEffect(() => {
    setActiveMemoryStructureId((current) => {
      if (current && memoryStructures.some((item) => item.id === current)) return current
      return memoryStructures[0]?.id || ''
    })
    setMemoryStructureDraft(null)
    setMemoryStructureTagDraft('')
  }, [workspace])

  useEffect(() => {
    setActiveOpeningSelectorId((current) => {
      if (current && openingSelectors.some((item) => item.id === current)) return current
      return openingSelectors[0]?.id || ''
    })
    setOpeningSelectorDraft(null)
    setOpeningSelectorTagDraft('')
  }, [workspace])

  const presetDrafts: PresetDrafts = useMemo(() => ({
    teller: tellerDraft,
    director: storyDirectorDraft,
    image: imagePresetDraft,
    event: eventPackageDraft,
    rule: ruleSystemDraft,
    actorState: actorStateDraft,
    memoryStructure: memoryStructureDraft,
    opening: openingSelectorDraft,
  }), [actorStateDraft, eventPackageDraft, imagePresetDraft, memoryStructureDraft, openingSelectorDraft, ruleSystemDraft, storyDirectorDraft, tellerDraft])

  const mergeSavedTeller = (teller: Teller) => {
    setTellers((current) => {
      const next = current.map((entry) => (entry.id === teller.id ? teller : entry))
      onTellersChange?.(next)
      return next
    })
    setActiveTellerId(teller.id)
  }

  const mergeSavedStoryDirector = (director: StoryDirector) => {
    setStoryDirectors((current) => {
      const next = current.map((entry) => (entry.id === director.id ? director : entry))
      onStoryDirectorsChange?.(next)
      return next
    })
    setActiveStoryDirectorId(director.id)
  }

  const mergeSavedImagePreset = (preset: ImagePreset) => {
    setImagePresets((current) => {
      const next = current.map((entry) => (entry.id === preset.id ? preset : entry))
      onImagePresetsChange?.(next)
      return next
    })
    setActiveImagePresetId(preset.id)
  }

  const mergeSavedEventPackage = (item: EventPackageModule) => {
    setEventPackages((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
    setActiveEventPackageId(item.id)
  }

  const mergeSavedRuleSystem = (item: RuleSystemModule) => {
    setRuleSystems((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
    setActiveRuleSystemId(item.id)
  }

  const mergeSavedActorState = (item: ActorStateModule) => {
    setActorStates((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
    setActiveActorStateId(item.id)
  }

  const mergeSavedMemoryStructure = (item: StoryMemoryStructureModule) => {
    setMemoryStructures((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
    setActiveMemoryStructureId(item.id)
  }

  const mergeSavedOpeningSelector = (item: OpeningSelectorModule) => {
    setOpeningSelectors((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
    setActiveOpeningSelectorId(item.id)
  }

  const tellerAutosave = usePresetResourceAutosave<Teller, Partial<Teller>, Teller>({
    draft: tellerDraft,
    tagDraft: tellerTagDraft,
    active: presetResourceKind === 'teller' && activeTellerId !== TELLER_CONFIG_AGENT_ENTRY_ID,
    makePayload: makeTellerPayload,
    signature: presetResourceDraftSignature,
    save: updateInteractiveTeller,
    onSaved: (teller, mode, previousDraft) => {
      if (!previousDraft.custom || mode === 'manual') mergeSavedTeller(teller)
    },
    onAutoSaveError: (err) => {
      console.warn('[teller-editor] 自动保存叙事风格失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[teller-editor] 切换条目前自动保存叙事风格失败', err),
  })

  const storyDirectorAutosave = usePresetResourceAutosave<StoryDirector, Partial<StoryDirector>, StoryDirector>({
    draft: storyDirectorDraft,
    tagDraft: storyDirectorTagDraft,
    active: presetResourceKind === 'director',
    valid: presetConfigValid,
    makePayload: makeStoryDirectorPayload,
    signature: presetResourceDraftSignature,
    save: updateStoryDirector,
    onSaved: (director, mode, previousDraft) => {
      if (!previousDraft.custom || mode === 'manual') mergeSavedStoryDirector(director)
    },
    onAutoSaveError: (err) => {
      console.warn('[story-director-editor] 自动保存故事导演失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[story-director-editor] 切换条目前自动保存故事导演失败', err),
  })

  const imagePresetAutosave = usePresetResourceAutosave<ImagePreset, Partial<ImagePreset>, ImagePreset>({
    draft: imagePresetDraft,
    tagDraft: imagePresetTagDraft,
    active: presetResourceKind === 'image',
    makePayload: makeImagePresetPayload,
    signature: presetResourceDraftSignature,
    save: updateImagePreset,
    onSaved: (preset, mode, previousDraft) => {
      if (!previousDraft.custom || mode === 'manual') mergeSavedImagePreset(preset)
    },
    onAutoSaveError: (err) => {
      console.warn('[image-preset-editor] 自动保存图像方案失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[image-preset-editor] 切换条目前自动保存图像方案失败', err),
  })

  const eventPackageAutosave = usePresetResourceAutosave<EventPackageModule, Partial<EventPackageModule>, EventPackageModule>({
    draft: eventPackageDraft,
    tagDraft: eventPackageTagDraft,
    active: presetResourceKind === 'event',
    valid: presetConfigValid,
    makePayload: makeEventPackagePayload,
    signature: presetResourceDraftSignature,
    save: updateEventPackage,
    onSaved: (item, mode, previousDraft) => {
      if (!previousDraft.custom || mode === 'manual') mergeSavedEventPackage(item)
    },
    onAutoSaveError: (err) => {
      console.warn('[event-package-editor] 自动保存事件包失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[event-package-editor] 切换条目前自动保存事件包失败', err),
  })

  const ruleSystemAutosave = usePresetResourceAutosave<RuleSystemModule, Partial<RuleSystemModule>, RuleSystemModule>({
    draft: ruleSystemDraft,
    tagDraft: ruleSystemTagDraft,
    active: presetResourceKind === 'rule',
    valid: presetConfigValid,
    makePayload: makeRuleSystemPayload,
    signature: presetResourceDraftSignature,
    save: updateRuleSystem,
    onSaved: (item, mode, previousDraft) => {
      if (!previousDraft.custom || mode === 'manual') mergeSavedRuleSystem(item)
    },
    onAutoSaveError: (err) => {
      console.warn('[rule-system-editor] 自动保存 TRPG 检定失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[rule-system-editor] 切换条目前自动保存 TRPG 检定失败', err),
  })

  const actorStateAutosave = usePresetResourceAutosave<ActorStateModule, Partial<ActorStateModule>, ActorStateModule>({
    draft: actorStateDraft,
    tagDraft: actorStateTagDraft,
    active: presetResourceKind === 'actor-state',
    valid: presetConfigValid,
    makePayload: makeActorStatePayload,
    signature: presetResourceDraftSignature,
    save: updateActorState,
    onSaved: (item, mode, previousDraft) => {
      if (!previousDraft.custom || mode === 'manual') mergeSavedActorState(item)
    },
    onAutoSaveError: (err) => {
      console.warn('[actor-state-editor] 自动保存状态系统失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[actor-state-editor] 切换条目前自动保存状态系统失败', err),
  })

  const memoryStructureAutosave = usePresetResourceAutosave<StoryMemoryStructureModule, Partial<StoryMemoryStructureModule>, StoryMemoryStructureModule>({
    draft: memoryStructureDraft,
    tagDraft: memoryStructureTagDraft,
    active: presetResourceKind === 'memory-structure',
    valid: presetConfigValid,
    makePayload: makeMemoryStructurePayload,
    signature: presetResourceDraftSignature,
    save: updateStoryMemoryStructure,
    onSaved: (item, mode, previousDraft) => {
      if (!previousDraft.custom || mode === 'manual') mergeSavedMemoryStructure(item)
    },
    onAutoSaveError: (err) => {
      console.warn('[story-memory-structure-editor] 自动保存记忆结构失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[story-memory-structure-editor] 切换条目前自动保存记忆结构失败', err),
  })

  const openingSelectorAutosave = usePresetResourceAutosave<OpeningSelectorModule, Partial<OpeningSelectorModule>, OpeningSelectorModule>({
    draft: openingSelectorDraft,
    tagDraft: openingSelectorTagDraft,
    active: presetResourceKind === 'opening',
    valid: presetConfigValid,
    makePayload: makeOpeningSelectorPayload,
    signature: presetResourceDraftSignature,
    save: updateOpeningSelector,
    onSaved: (item, mode, previousDraft) => {
      if (!previousDraft.custom || mode === 'manual') mergeSavedOpeningSelector(item)
    },
    onAutoSaveError: (err) => {
      console.warn('[opening-selector-editor] 自动保存开局选择器失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[opening-selector-editor] 切换条目前自动保存开局选择器失败', err),
  })

  useEffect(() => {
    if (activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID) {
      setTellerDraft(null)
      setTellerTagDraft('')
      tellerAutosave.resetBaseline(null)
      setActiveSlotId('')
      return
    }
    const teller = tellers.find((entry) => entry.id === activeTellerId) || null
    const nextDraft = teller ? cloneTeller(teller) : null
    const nextTagDraft = (teller?.tags || []).join('，')
    setTellerDraft(nextDraft)
    setTellerTagDraft(nextTagDraft)
    setActiveSlotId((current) => {
      if (current && teller?.slots?.some((slot) => slot.id === current)) return current
      return teller?.slots?.[0]?.id || ''
    })
    tellerAutosave.resetBaseline(nextDraft, nextTagDraft)
  }, [activeTellerId, tellers, tellerAutosave.resetBaseline])

  useEffect(() => {
    const preset = imagePresets.find((entry) => entry.id === activeImagePresetId) || null
    const nextDraft = preset ? cloneImagePreset(preset) : null
    const nextTagDraft = (preset?.tags || []).join('，')
    setImagePresetDraft(nextDraft)
    setImagePresetTagDraft(nextTagDraft)
    imagePresetAutosave.resetBaseline(nextDraft, nextTagDraft)
  }, [activeImagePresetId, imagePresets, imagePresetAutosave.resetBaseline])

  useEffect(() => {
    const director = storyDirectors.find((entry) => entry.id === activeStoryDirectorId) || null
    const nextDraft = director ? cloneStoryDirector(director) : null
    const nextTagDraft = (director?.tags || []).join('，')
    setStoryDirectorDraft(nextDraft)
    setStoryDirectorTagDraft(nextTagDraft)
    storyDirectorAutosave.resetBaseline(nextDraft, nextTagDraft)
  }, [activeStoryDirectorId, storyDirectors, storyDirectorAutosave.resetBaseline])

  useEffect(() => {
    const item = eventPackages.find((entry) => entry.id === activeEventPackageId) || null
    const nextDraft = item ? cloneEventPackage(item) : null
    const nextTagDraft = (item?.tags || []).join('，')
    setEventPackageDraft(nextDraft)
    setEventPackageTagDraft(nextTagDraft)
    eventPackageAutosave.resetBaseline(nextDraft, nextTagDraft)
  }, [activeEventPackageId, eventPackages, eventPackageAutosave.resetBaseline])

  useEffect(() => {
    const item = ruleSystems.find((entry) => entry.id === activeRuleSystemId) || null
    const nextDraft = item ? cloneRuleSystem(item) : null
    const nextTagDraft = (item?.tags || []).join('，')
    setRuleSystemDraft(nextDraft)
    setRuleSystemTagDraft(nextTagDraft)
    ruleSystemAutosave.resetBaseline(nextDraft, nextTagDraft)
  }, [activeRuleSystemId, ruleSystems, ruleSystemAutosave.resetBaseline])

  useEffect(() => {
    const item = actorStates.find((entry) => entry.id === activeActorStateId) || null
    const nextDraft = item ? cloneActorState(item) : null
    const nextTagDraft = (item?.tags || []).join('，')
    setActorStateDraft(nextDraft)
    setActorStateTagDraft(nextTagDraft)
    actorStateAutosave.resetBaseline(nextDraft, nextTagDraft)
  }, [activeActorStateId, actorStates, actorStateAutosave.resetBaseline])

  useEffect(() => {
    const item = memoryStructures.find((entry) => entry.id === activeMemoryStructureId) || null
    const nextDraft = item ? cloneMemoryStructure(item) : null
    const nextTagDraft = (item?.tags || []).join('，')
    setMemoryStructureDraft(nextDraft)
    setMemoryStructureTagDraft(nextTagDraft)
    memoryStructureAutosave.resetBaseline(nextDraft, nextTagDraft)
  }, [activeMemoryStructureId, memoryStructures, memoryStructureAutosave.resetBaseline])

  useEffect(() => {
    const item = openingSelectors.find((entry) => entry.id === activeOpeningSelectorId) || null
    const nextDraft = item ? cloneOpeningSelector(item) : null
    const nextTagDraft = (item?.tags || []).join('，')
    setOpeningSelectorDraft(nextDraft)
    setOpeningSelectorTagDraft(nextTagDraft)
    openingSelectorAutosave.resetBaseline(nextDraft, nextTagDraft)
  }, [activeOpeningSelectorId, openingSelectors, openingSelectorAutosave.resetBaseline])

  useEffect(() => {
    if (!presetFocus || presetFocusNonceRef.current === presetFocus.nonce) return
    if (presetFocus.kind === 'memory-structure') {
      const targetId = presetFocus.id || memoryStructures[0]?.id || ''
      if (!targetId) return
      if (presetFocus.id && !memoryStructures.some((item) => item.id === presetFocus.id)) return
      if (!flushPresetResourceAutoSave()) return
      setPresetResourceKind('memory-structure')
      setActiveTellerId((current) => current === TELLER_CONFIG_AGENT_ENTRY_ID ? '' : current)
      setActiveMemoryStructureId(targetId)
      presetFocusNonceRef.current = presetFocus.nonce
    }
  }, [memoryStructures, presetFocus])

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

  const refreshMemoryStructures = async (nextActiveId?: string) => {
    const data = await getStoryMemoryStructures()
    setMemoryStructures(data)
    setActiveMemoryStructureId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((item) => item.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const refreshOpeningSelectors = async (nextActiveId?: string) => {
    const data = await getOpeningSelectors()
    setOpeningSelectors(data)
    setActiveOpeningSelectorId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((item) => item.id === current)) return current
      return data[0]?.id || ''
    })
  }

  const autosaveForKind = (kind: PresetResourceKind): AutosaveController => {
    if (kind === 'director') return storyDirectorAutosave
    if (kind === 'image') return imagePresetAutosave
    if (kind === 'event') return eventPackageAutosave
    if (kind === 'rule') return ruleSystemAutosave
    if (kind === 'actor-state') return actorStateAutosave
    if (kind === 'memory-structure') return memoryStructureAutosave
    if (kind === 'opening') return openingSelectorAutosave
    return tellerAutosave
  }

  const canLeavePresetResource = () => {
    if (isPresetConfigResourceKind(presetResourceKind) && !presetConfigValidRef.current) {
      toast.error(t('settingPanel.presetConfig.invalidBlock'))
      return false
    }
    return true
  }

  function flushPresetResourceAutoSave() {
    if (!canLeavePresetResource()) return false
    void autosaveForKind(presetResourceKind).flushPending()
    return true
  }

  const handleSelectTeller = (id: string) => {
    if (presetResourceKind === 'teller' && activeTellerId === id) return
    if (!flushPresetResourceAutoSave()) return
    if (id !== TELLER_CONFIG_AGENT_ENTRY_ID) setPresetResourceKind('teller')
    setActiveTellerId(id)
  }

  const selectPresetResource = (kind: Exclude<PresetResourceKind, 'teller'>, id: string) => {
    const activeId = currentActivePresetId(kind)
    if (presetResourceKind === kind && activeId === id && activeTellerId !== TELLER_CONFIG_AGENT_ENTRY_ID) return
    if (!flushPresetResourceAutoSave()) return
    setPresetResourceKind(kind)
    setActiveTellerId((current) => current === TELLER_CONFIG_AGENT_ENTRY_ID ? '' : current)
    setActivePresetId(kind, id)
  }

  const currentActivePresetId = (kind: PresetResourceKind) => {
    if (kind === 'director') return activeStoryDirectorId
    if (kind === 'image') return activeImagePresetId
    if (kind === 'event') return activeEventPackageId
    if (kind === 'rule') return activeRuleSystemId
    if (kind === 'actor-state') return activeActorStateId
    if (kind === 'memory-structure') return activeMemoryStructureId
    if (kind === 'opening') return activeOpeningSelectorId
    return activeTellerId
  }

  const setActivePresetId = (kind: Exclude<PresetResourceKind, 'teller'>, id: string) => {
    if (kind === 'director') setActiveStoryDirectorId(id)
    if (kind === 'image') setActiveImagePresetId(id)
    if (kind === 'event') setActiveEventPackageId(id)
    if (kind === 'rule') setActiveRuleSystemId(id)
    if (kind === 'actor-state') setActiveActorStateId(id)
    if (kind === 'memory-structure') setActiveMemoryStructureId(id)
    if (kind === 'opening') setActiveOpeningSelectorId(id)
  }

  const handleCreateTeller = async () => {
    setSaving(true)
    try {
      const teller = await createInteractiveTeller(newTellerDraft())
      setPresetResourceKind('teller')
      await refreshTellers(teller.id)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateStoryDirector = async () => {
    setSaving(true)
    try {
      const director = await createStoryDirector(newStoryDirectorDraft())
      setPresetResourceKind('director')
      await refreshStoryDirectors(director.id)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateEventPackage = async () => {
    setSaving(true)
    try {
      const item = await createEventPackage(newEventPackageDraft())
      setPresetResourceKind('event')
      await refreshEventPackages(item.id)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateRuleSystem = async () => {
    setSaving(true)
    try {
      const item = await createRuleSystem(newRuleSystemDraft())
      setPresetResourceKind('rule')
      await refreshRuleSystems(item.id)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateActorState = async () => {
    setSaving(true)
    try {
      const item = await createActorState(newActorStateDraft())
      setPresetResourceKind('actor-state')
      await refreshActorStates(item.id)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateMemoryStructure = async () => {
    setSaving(true)
    try {
      const item = await createStoryMemoryStructure(newMemoryStructureDraft())
      setPresetResourceKind('memory-structure')
      await refreshMemoryStructures(item.id)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateOpeningSelector = async () => {
    setSaving(true)
    try {
      const item = await createOpeningSelector(newOpeningSelectorDraft())
      setPresetResourceKind('opening')
      await refreshOpeningSelectors(item.id)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateImagePreset = async () => {
    setSaving(true)
    try {
      const preset = await createImagePreset(newImagePresetDraft())
      setPresetResourceKind('image')
      await refreshImagePresets(preset.id)
    } finally {
      setSaving(false)
    }
  }

  const requestDeletePreset = (kind: PresetResourceKind, target: { id: string; name: string; custom?: boolean } | null) => {
    if (!target?.custom) return
    setDeletePresetTarget({
      kind,
      id: target.id,
      name: target.name,
      ...PRESET_DELETE_COPY[kind],
    })
  }

  const handleDelete = () => {
    if (activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID) return
    const target = currentPresetDraft()
    requestDeletePreset(presetResourceKind, target)
  }

  const confirmDeletePresetTarget = async () => {
    if (!deletePresetTarget) return
    setSaving(true)
    try {
      autosaveForKind(deletePresetTarget.kind).cancelPending()
      if (deletePresetTarget.kind === 'image') {
        await deleteImagePreset(deletePresetTarget.id)
        await refreshImagePresets()
      } else if (deletePresetTarget.kind === 'event') {
        await deleteEventPackage(deletePresetTarget.id)
        await refreshEventPackages()
      } else if (deletePresetTarget.kind === 'rule') {
        await deleteRuleSystem(deletePresetTarget.id)
        await refreshRuleSystems()
      } else if (deletePresetTarget.kind === 'actor-state') {
        await deleteActorState(deletePresetTarget.id)
        await refreshActorStates()
      } else if (deletePresetTarget.kind === 'memory-structure') {
        await deleteStoryMemoryStructurePreset(deletePresetTarget.id)
        await refreshMemoryStructures()
      } else if (deletePresetTarget.kind === 'opening') {
        await deleteOpeningSelector(deletePresetTarget.id)
        await refreshOpeningSelectors()
      } else if (deletePresetTarget.kind === 'director') {
        await deleteStoryDirector(deletePresetTarget.id)
        await refreshStoryDirectors()
      } else {
        await deleteInteractiveTeller(deletePresetTarget.id)
        await refreshTellers()
      }
      setDeletePresetTarget(null)
    } finally {
      setSaving(false)
    }
  }

  const handleRestoreBuiltinPreset = async () => {
    if (!currentPresetBuiltinOverridden(presetResourceKind, presetDrafts)) return
    setSaving(true)
    try {
      autosaveForKind(presetResourceKind).cancelPending()
      if (presetResourceKind === 'image' && imagePresetDraft) {
        await deleteImagePreset(imagePresetDraft.id)
        await refreshImagePresets(imagePresetDraft.id)
      } else if (presetResourceKind === 'event' && eventPackageDraft) {
        await deleteEventPackage(eventPackageDraft.id)
        await refreshEventPackages(eventPackageDraft.id)
      } else if (presetResourceKind === 'rule' && ruleSystemDraft) {
        await deleteRuleSystem(ruleSystemDraft.id)
        await refreshRuleSystems(ruleSystemDraft.id)
      } else if (presetResourceKind === 'actor-state' && actorStateDraft) {
        await deleteActorState(actorStateDraft.id)
        await refreshActorStates(actorStateDraft.id)
      } else if (presetResourceKind === 'memory-structure' && memoryStructureDraft) {
        await deleteStoryMemoryStructurePreset(memoryStructureDraft.id)
        await refreshMemoryStructures(memoryStructureDraft.id)
      } else if (presetResourceKind === 'opening' && openingSelectorDraft) {
        await deleteOpeningSelector(openingSelectorDraft.id)
        await refreshOpeningSelectors(openingSelectorDraft.id)
      } else if (presetResourceKind === 'director' && storyDirectorDraft) {
        await deleteStoryDirector(storyDirectorDraft.id)
        await refreshStoryDirectors(storyDirectorDraft.id)
      } else if (tellerDraft) {
        await deleteInteractiveTeller(tellerDraft.id)
        await refreshTellers(tellerDraft.id)
      }
      toast.success(t('settingPanel.restoreBuiltinDone'))
    } catch (err) {
      toast.error((err as Error).message || t('settingPanel.restoreBuiltinFailed'))
    } finally {
      setSaving(false)
    }
  }

  const handleSave = async () => {
    if (isPresetConfigResourceKind(presetResourceKind) && !presetConfigValidRef.current) {
      toast.error(t('settingPanel.presetConfig.invalidBlock'))
      return
    }
    setSaving(true)
    try {
      await autosaveForKind(presetResourceKind).saveNow('manual')
    } catch (err) {
      toast.error((err as Error).message || t('editor.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const currentPresetDraft = () => {
    if (presetResourceKind === 'director') return storyDirectorDraft
    if (presetResourceKind === 'image') return imagePresetDraft
    if (presetResourceKind === 'event') return eventPackageDraft
    if (presetResourceKind === 'rule') return ruleSystemDraft
    if (presetResourceKind === 'actor-state') return actorStateDraft
    if (presetResourceKind === 'memory-structure') return memoryStructureDraft
    if (presetResourceKind === 'opening') return openingSelectorDraft
    return tellerDraft
  }

  const isTellerConfigAgentActive = activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID
  const activeDraft = currentPresetDraft()
  const canRestoreBuiltinPreset = !isTellerConfigAgentActive && currentPresetBuiltinOverridden(presetResourceKind, presetDrafts)
  const presetConfigInvalid = isPresetConfigResourceKind(presetResourceKind) && !presetConfigValid
  const saveDisabled = saving || presetConfigInvalid || !activeDraft
  const titleIcon = presetResourceKind === 'image' ? <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" /> : <SlidersHorizontal className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
  const directoryPanel = (
    <div className="nova-sidebar flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
      <TellerDirectory
        resourceKind={presetResourceKind}
        usageMode={presetUsageMode}
        tellers={tellers}
        storyDirectors={storyDirectors}
        imagePresets={imagePresets}
        eventPackages={eventPackages}
        ruleSystems={ruleSystems}
        actorStates={actorStates}
        memoryStructures={memoryStructures}
        openingSelectors={openingSelectors}
        activeTellerId={activeTellerId}
        activeStoryDirectorId={activeStoryDirectorId}
        activeImagePresetId={activeImagePresetId}
        activeEventPackageId={activeEventPackageId}
        activeRuleSystemId={activeRuleSystemId}
        activeActorStateId={activeActorStateId}
        activeMemoryStructureId={activeMemoryStructureId}
        activeOpeningSelectorId={activeOpeningSelectorId}
        saving={saving}
        onSelectTeller={handleSelectTeller}
        onSelectStoryDirector={(id) => selectPresetResource('director', id)}
        onSelectImagePreset={(id) => selectPresetResource('image', id)}
        onSelectEventPackage={(id) => selectPresetResource('event', id)}
        onSelectRuleSystem={(id) => selectPresetResource('rule', id)}
        onSelectActorState={(id) => selectPresetResource('actor-state', id)}
        onSelectMemoryStructure={(id) => selectPresetResource('memory-structure', id)}
        onSelectOpeningSelector={(id) => selectPresetResource('opening', id)}
        onCreateTeller={() => void handleCreateTeller()}
        onCreateStoryDirector={() => void handleCreateStoryDirector()}
        onCreateImagePreset={() => void handleCreateImagePreset()}
        onCreateEventPackage={() => void handleCreateEventPackage()}
        onCreateRuleSystem={() => void handleCreateRuleSystem()}
        onCreateActorState={() => void handleCreateActorState()}
        onCreateMemoryStructure={() => void handleCreateMemoryStructure()}
        onCreateOpeningSelector={() => void handleCreateOpeningSelector()}
      />
    </div>
  )

  return (
    <section className="h-full min-h-0 bg-[var(--nova-surface-2)] text-[var(--nova-text)]">
      <AdaptiveSurface
        left={{
          id: 'setting-directory',
          title: t('settingPanel.mode.teller'),
          side: 'left',
          icon: <SlidersHorizontal className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />,
          content: directoryPanel,
          desktopClassName: `min-h-0 border-r border-[var(--nova-border)] ${embedded ? 'w-56' : 'w-[320px]'}`,
          mobileClassName: embedded ? 'w-[min(86vw,320px)]' : 'w-[min(90vw,360px)]',
        }}
        className="h-full"
        mainClassName="min-h-0 min-w-0"
        desktopGridClassName={embedded ? 'grid-cols-[14rem_minmax(0,1fr)]' : 'grid-cols-[320px_minmax(0,1fr)]'}
      >
        {({ isMobile, openLeft }) => (
          <main className="flex h-full min-h-0 min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            <div className="nova-topbar flex min-h-12 shrink-0 items-center justify-between gap-3 border-b px-4">
              <div className="flex min-w-0 items-center gap-2">
                {isMobile && (
                  <button type="button" className="nova-icon-button flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('workbench.mobile.openSidePanel', { label: t('settingPanel.mode.teller') })} onClick={openLeft}>
                    <PanelLeft className="h-4 w-4" />
                  </button>
                )}
                <div className="min-w-0">
                  <div className="flex min-w-0 items-center gap-2">
                    {titleIcon}
                    <h2 className="truncate text-sm font-semibold text-[var(--nova-text)]">{isTellerConfigAgentActive ? t('settingPanel.tellerAgent.title') : presetEditorTitle(presetResourceKind, presetDrafts, t)}</h2>
                  </div>
                  <p className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">{isTellerConfigAgentActive ? t('settingPanel.tellerAgent.subtitle') : presetEditorSubtitle(presetResourceKind, presetDrafts, t)}</p>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                {canRestoreBuiltinPreset && (
                  <Button className={actionButtonClassName} variant="outline" size="sm" disabled={saving} onClick={() => void handleRestoreBuiltinPreset()} aria-label={t('settingPanel.restoreBuiltin')} title={t('settingPanel.restoreBuiltin')}>
                    <RotateCcw className="h-4 w-4" />
                    <span className="hidden sm:inline">{t('settingPanel.restoreBuiltin')}</span>
                  </Button>
                )}
                {!isTellerConfigAgentActive && (
                  <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || !activeDraft?.custom} onClick={handleDelete} aria-label={t(PRESET_DELETE_COPY[presetResourceKind].titleKey)}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                )}
                {!isTellerConfigAgentActive && (
                  <Button className={actionButtonClassName} variant="outline" size="sm" disabled={saveDisabled} onClick={handleSave}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                    {t('common.save')}
                  </Button>
                )}
              </div>
            </div>

            {isTellerConfigAgentActive ? (
              <ConfigManagerChat
                workspace={workspace}
                origin="teller"
                resourceId={TELLER_CONFIG_AGENT_ENTRY_ID}
                context={{
                  teller_count: String(tellers.length),
                  event_package_count: String(eventPackages.length),
                  rule_system_count: String(ruleSystems.length),
                  actor_state_count: String(actorStates.length),
                  memory_structure_count: String(memoryStructures.length),
                  opening_selector_count: String(openingSelectors.length),
                  story_director_count: String(storyDirectors.length),
                  image_preset_count: String(imagePresets.length),
                }}
                onMutated={() => {
                  void refreshTellers()
                  void refreshEventPackages()
                  void refreshRuleSystems()
                  void refreshActorStates()
                  void refreshMemoryStructures()
                  void refreshOpeningSelectors()
                  void refreshStoryDirectors()
                  void refreshImagePresets()
                }}
              />
            ) : (
              <PresetResourcePane
                kind={presetResourceKind}
                workspace={workspace}
                tellers={tellers}
                storyDirectors={storyDirectors}
                imagePresets={imagePresets}
                eventPackages={eventPackages}
                ruleSystems={ruleSystems}
                actorStates={actorStates}
                memoryStructures={memoryStructures}
                openingSelectors={openingSelectors}
                tellerDraft={tellerDraft}
                setTellerDraft={setTellerDraft}
                tellerTagDraft={tellerTagDraft}
                setTellerTagDraft={setTellerTagDraft}
                activeSlotId={activeSlotId}
                setActiveSlotId={setActiveSlotId}
                storyDirectorDraft={storyDirectorDraft}
                setStoryDirectorDraft={setStoryDirectorDraft}
                storyDirectorTagDraft={storyDirectorTagDraft}
                setStoryDirectorTagDraft={setStoryDirectorTagDraft}
                imagePresetDraft={imagePresetDraft}
                setImagePresetDraft={setImagePresetDraft}
                imagePresetTagDraft={imagePresetTagDraft}
                setImagePresetTagDraft={setImagePresetTagDraft}
                eventPackageDraft={eventPackageDraft}
                setEventPackageDraft={setEventPackageDraft}
                eventPackageTagDraft={eventPackageTagDraft}
                setEventPackageTagDraft={setEventPackageTagDraft}
                ruleSystemDraft={ruleSystemDraft}
                setRuleSystemDraft={setRuleSystemDraft}
                ruleSystemTagDraft={ruleSystemTagDraft}
                setRuleSystemTagDraft={setRuleSystemTagDraft}
                actorStateDraft={actorStateDraft}
                setActorStateDraft={setActorStateDraft}
                actorStateTagDraft={actorStateTagDraft}
                setActorStateTagDraft={setActorStateTagDraft}
                memoryStructureDraft={memoryStructureDraft}
                setMemoryStructureDraft={setMemoryStructureDraft}
                memoryStructureTagDraft={memoryStructureTagDraft}
                setMemoryStructureTagDraft={setMemoryStructureTagDraft}
                openingSelectorDraft={openingSelectorDraft}
                setOpeningSelectorDraft={setOpeningSelectorDraft}
                openingSelectorTagDraft={openingSelectorTagDraft}
                setOpeningSelectorTagDraft={setOpeningSelectorTagDraft}
                onSave={handleSave}
                onValidityChange={setPresetConfigValid}
              />
            )}
          </main>
        )}
      </AdaptiveSurface>
      <AlertDialog open={Boolean(deletePresetTarget)} onOpenChange={(open) => {
        if (!open && !saving) setDeletePresetTarget(null)
      }}>
        <AlertDialogContent className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
          <AlertDialogHeader>
            <AlertDialogTitle>{deletePresetTarget ? t(deletePresetTarget.titleKey) : t('common.delete')}</AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--nova-text-muted)]">
              {deletePresetTarget ? t(deletePresetTarget.descriptionKey, { name: deletePresetTarget.name }) : ''}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={saving}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-[var(--nova-danger-bg)] text-[var(--nova-danger)] hover:bg-[var(--nova-danger-bg)]"
              disabled={saving || !deletePresetTarget}
              onClick={(event) => {
                event.preventDefault()
                void confirmDeletePresetTarget()
              }}
            >
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}

interface PresetResourcePaneProps {
  kind: PresetResourceKind
  workspace: string
  tellers: Teller[]
  storyDirectors: StoryDirector[]
  imagePresets: ImagePreset[]
  eventPackages: EventPackageModule[]
  ruleSystems: RuleSystemModule[]
  actorStates: ActorStateModule[]
  memoryStructures: StoryMemoryStructureModule[]
  openingSelectors: OpeningSelectorModule[]
  tellerDraft: Teller | null
  setTellerDraft: (draft: Teller | null) => void
  tellerTagDraft: string
  setTellerTagDraft: (value: string) => void
  activeSlotId: string
  setActiveSlotId: (value: string) => void
  storyDirectorDraft: StoryDirector | null
  setStoryDirectorDraft: (draft: StoryDirector | null) => void
  storyDirectorTagDraft: string
  setStoryDirectorTagDraft: (value: string) => void
  imagePresetDraft: ImagePreset | null
  setImagePresetDraft: (draft: ImagePreset | null) => void
  imagePresetTagDraft: string
  setImagePresetTagDraft: (value: string) => void
  eventPackageDraft: EventPackageModule | null
  setEventPackageDraft: (draft: EventPackageModule | null) => void
  eventPackageTagDraft: string
  setEventPackageTagDraft: (value: string) => void
  ruleSystemDraft: RuleSystemModule | null
  setRuleSystemDraft: (draft: RuleSystemModule | null) => void
  ruleSystemTagDraft: string
  setRuleSystemTagDraft: (value: string) => void
  actorStateDraft: ActorStateModule | null
  setActorStateDraft: (draft: ActorStateModule | null) => void
  actorStateTagDraft: string
  setActorStateTagDraft: (value: string) => void
  memoryStructureDraft: StoryMemoryStructureModule | null
  setMemoryStructureDraft: (draft: StoryMemoryStructureModule | null) => void
  memoryStructureTagDraft: string
  setMemoryStructureTagDraft: (value: string) => void
  openingSelectorDraft: OpeningSelectorModule | null
  setOpeningSelectorDraft: (draft: OpeningSelectorModule | null) => void
  openingSelectorTagDraft: string
  setOpeningSelectorTagDraft: (value: string) => void
  onSave: () => void
  onValidityChange: (valid: boolean) => void
}

function PresetResourcePane(props: PresetResourcePaneProps) {
  if (props.kind === 'image') return <ImagePresetPane draft={props.imagePresetDraft} setDraft={props.setImagePresetDraft} tagDraft={props.imagePresetTagDraft} setTagDraft={props.setImagePresetTagDraft} onSave={props.onSave} />
  if (props.kind === 'event') return <EventPackagePane draft={props.eventPackageDraft} setDraft={props.setEventPackageDraft} tagDraft={props.eventPackageTagDraft} setTagDraft={props.setEventPackageTagDraft} onSave={props.onSave} onValidityChange={props.onValidityChange} />
  if (props.kind === 'rule') return <RuleSystemPane draft={props.ruleSystemDraft} setDraft={props.setRuleSystemDraft} tagDraft={props.ruleSystemTagDraft} setTagDraft={props.setRuleSystemTagDraft} onSave={props.onSave} onValidityChange={props.onValidityChange} />
  if (props.kind === 'actor-state') return <ActorStatePane draft={props.actorStateDraft} setDraft={props.setActorStateDraft} tagDraft={props.actorStateTagDraft} setTagDraft={props.setActorStateTagDraft} onSave={props.onSave} onValidityChange={props.onValidityChange} />
  if (props.kind === 'memory-structure') return <MemoryStructurePane draft={props.memoryStructureDraft} setDraft={props.setMemoryStructureDraft} tagDraft={props.memoryStructureTagDraft} setTagDraft={props.setMemoryStructureTagDraft} onSave={props.onSave} onValidityChange={props.onValidityChange} />
  if (props.kind === 'opening') return <OpeningSelectorPane draft={props.openingSelectorDraft} setDraft={props.setOpeningSelectorDraft} tagDraft={props.openingSelectorTagDraft} setTagDraft={props.setOpeningSelectorTagDraft} onSave={props.onSave} onValidityChange={props.onValidityChange} />
  if (props.kind === 'director') {
    return (
      <StoryDirectorPane
        draft={props.storyDirectorDraft}
        tellers={props.tellers}
        eventPackages={props.eventPackages}
        ruleSystems={props.ruleSystems}
        actorStates={props.actorStates}
        memoryStructures={props.memoryStructures}
        openingSelectors={props.openingSelectors}
        imagePresets={props.imagePresets}
        setDraft={props.setStoryDirectorDraft}
        tagDraft={props.storyDirectorTagDraft}
        setTagDraft={props.setStoryDirectorTagDraft}
        onSave={props.onSave}
        onValidityChange={props.onValidityChange}
      />
    )
  }
  return <TellerPane workspace={props.workspace} draft={props.tellerDraft} setDraft={props.setTellerDraft} tagDraft={props.tellerTagDraft} setTagDraft={props.setTellerTagDraft} activeSlotId={props.activeSlotId} setActiveSlotId={props.setActiveSlotId} onSave={props.onSave} />
}

function TellerPane(props: { workspace: string; draft: Teller | null; setDraft: (draft: Teller | null) => void; tagDraft: string; setTagDraft: (value: string) => void; activeSlotId: string; setActiveSlotId: (value: string) => void; onSave: () => void }) {
  return <TellerEditor {...props} />
}

function ImagePresetPane(props: { draft: ImagePreset | null; setDraft: (draft: ImagePreset | null) => void; tagDraft: string; setTagDraft: (value: string) => void; onSave: () => void }) {
  return <ImagePresetEditor {...props} />
}

function EventPackagePane(props: { draft: EventPackageModule | null; setDraft: (draft: EventPackageModule | null) => void; tagDraft: string; setTagDraft: (value: string) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <EventPackageEditor {...props} />
}

function RuleSystemPane(props: { draft: RuleSystemModule | null; setDraft: (draft: RuleSystemModule | null) => void; tagDraft: string; setTagDraft: (value: string) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <RuleSystemEditor {...props} />
}

function ActorStatePane(props: { draft: ActorStateModule | null; setDraft: (draft: ActorStateModule | null) => void; tagDraft: string; setTagDraft: (value: string) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <ActorStateEditor {...props} />
}

function MemoryStructurePane(props: { draft: StoryMemoryStructureModule | null; setDraft: (draft: StoryMemoryStructureModule | null) => void; tagDraft: string; setTagDraft: (value: string) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <StoryMemoryStructureEditor {...props} />
}

function OpeningSelectorPane(props: { draft: OpeningSelectorModule | null; setDraft: (draft: OpeningSelectorModule | null) => void; tagDraft: string; setTagDraft: (value: string) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <OpeningSelectorEditor {...props} />
}

function StoryDirectorPane(props: { draft: StoryDirector | null; tellers: Teller[]; eventPackages: EventPackageModule[]; ruleSystems: RuleSystemModule[]; actorStates: ActorStateModule[]; memoryStructures: StoryMemoryStructureModule[]; openingSelectors: OpeningSelectorModule[]; imagePresets: ImagePreset[]; setDraft: (draft: StoryDirector | null) => void; tagDraft: string; setTagDraft: (value: string) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <StoryDirectorEditor {...props} />
}
