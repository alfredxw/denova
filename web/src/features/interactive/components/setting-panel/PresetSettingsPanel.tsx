import { useEffect, useRef, useState } from 'react'
import { Bot, Compass, Database, Dice5, RotateCcw, ScrollText, SlidersHorizontal, Sparkles, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import type { AutosaveStatus } from '@/components/forms/autosave-status'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { Button } from '@/components/ui/button'
import { createActorState, createEventPackage, createImagePreset, createInteractiveTeller, createRuleSystem, createStoryDirector, deleteActorState, deleteEventPackage, deleteImagePreset, deleteInteractiveTeller, deleteRuleSystem, deleteStoryDirector, getActorStates, getEventPackages, getImagePresets, getInteractiveTellers, getRuleSystems, getStoryDirectors, updateActorState, updateEventPackage, updateImagePreset, updateInteractiveTeller, updateRuleSystem, updateStoryDirector } from '../../api'
import { PRESET_RESOURCE_SCOPE, type PresetResourceKind } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, Teller } from '../../types'
import { PresetResourcePane } from './PresetResourcePane'
import { PresetDirectorySidebar } from './PresetDirectorySidebar'
import { buildPresetDirectorySections, presetDirectoryEntryId } from './preset-directory-sections'
import { applyPresetDirectoryOrder, usePresetDirectoryOrder } from './use-preset-directory-order'
import { usePresetDraftSync, usePresetResources } from './use-preset-resources'
import { usePresetSelection } from './use-preset-selection'
import { createPresetConflictResolver, usePresetResourceAutosave } from './usePresetResourceAutosave'
import { currentPresetBuiltinOverridden, EMPTY_IMAGE_PRESETS, EMPTY_STORY_DIRECTORS, EMPTY_TELLERS, isPresetConfigResourceKind, makeActorStatePayload, makeEventPackagePayload, makeImagePresetPayload, makeRuleSystemPayload, makeStoryDirectorPayload, makeTellerPayload, newActorStateDraft, newEventPackageDraft, newImagePresetDraft, newRuleSystemDraft, newStoryDirectorDraft, newTellerDraft, presetEditorSubtitle, presetEditorTitle, presetResourceDraftSignature, PRESET_DELETE_COPY, TELLER_CONFIG_AGENT_ENTRY_ID, type PresetDeleteTarget } from './presetResources'
import type { ToolNavigationIntent } from '@/components/Chat/tool-navigation'

interface PresetSettingsPanelProps {
  projectId: string
  tellers?: Teller[]
  storyDirectors?: StoryDirector[]
  imagePresets?: ImagePreset[]
  onTellersChange?: (tellers: Teller[]) => void
  onStoryDirectorsChange?: (directors: StoryDirector[]) => void
  onImagePresetsChange?: (presets: ImagePreset[]) => void
  embedded?: boolean
  onClose?: () => void
  toolNavigationIntent?: ToolNavigationIntent | null
}

interface AutosaveController {
  cancelPending: () => void
  flushPending: () => Promise<unknown> | null
  saveNow: (mode: 'manual' | 'auto') => Promise<unknown>
  status: AutosaveStatus
  error: string | null
}

const actionButtonClassName = 'gap-1.5 border-[var(--preset-line)] bg-[var(--preset-raised)] text-[var(--nova-text-muted)] shadow-none hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const iconActionClassName = 'border-[var(--preset-line)] bg-transparent text-[var(--nova-text-muted)] shadow-none hover:border-[var(--nova-danger-border)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]'
const PRESET_CONFIG_INVALID_TOAST_ID = 'preset-config-invalid'

export function PresetSettingsPanel({
  projectId,
  tellers: externalTellers = EMPTY_TELLERS,
  storyDirectors: externalStoryDirectors = EMPTY_STORY_DIRECTORS,
  imagePresets: externalImagePresets = EMPTY_IMAGE_PRESETS,
  onTellersChange,
  onStoryDirectorsChange,
  onImagePresetsChange,
  embedded = false,
  onClose,
  toolNavigationIntent,
}: PresetSettingsPanelProps) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const [transitioning, setTransitioning] = useState(false)
  const [deletePresetTarget, setDeletePresetTarget] = useState<PresetDeleteTarget | null>(null)
  const [presetConfigValid, setPresetConfigValid] = useState(true)
  const presetConfigValidRef = useRef(true)
  const closeDirectoryRef = useRef<() => void>(() => {})

  const [presetResourceKind, setPresetResourceKind] = useState<PresetResourceKind>('teller')

  const resources = usePresetResources({
    externalTellers,
    externalStoryDirectors,
    externalImagePresets,
    onTellersChange,
    onStoryDirectorsChange,
    onImagePresetsChange,
  })
  const {
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
  } = resources
  const presetDirectoryOrder = usePresetDirectoryOrder()

  useEffect(() => {
    presetConfigValidRef.current = presetConfigValid
  }, [presetConfigValid])

  useEffect(() => {
    setPresetConfigValid(true)
  }, [activeActorStateId, activeEventPackageId, activeRuleSystemId, activeStoryDirectorId, presetResourceKind])

  const tellerAutosave = usePresetResourceAutosave<Teller, Partial<Teller>, Teller>({
    draft: tellerDraft,
    scopeKey: PRESET_RESOURCE_SCOPE,
    active: presetResourceKind === 'teller' && activeTellerId !== TELLER_CONFIG_AGENT_ENTRY_ID,
    makePayload: makeTellerPayload,
    baselineFromSaved: (saved) => saved,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateInteractiveTeller(id, payload, baseRevision),
    resolveConflict: createPresetConflictResolver(
      getInteractiveTellers,
      makeTellerPayload,
      { resource: 'interactive_teller', scope: PRESET_RESOURCE_SCOPE },
    ),
    onSaved: mergeSavedTeller,
    onAutoSaveError: (err) => {
      console.warn('[teller-editor] Failed to autosave narrative style', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[teller-editor] Failed to autosave narrative style before selection change', err),
  })

  const storyDirectorAutosave = usePresetResourceAutosave<StoryDirector, Partial<StoryDirector>, StoryDirector>({
    draft: storyDirectorDraft,
    scopeKey: PRESET_RESOURCE_SCOPE,
    active: presetResourceKind === 'director',
    valid: presetConfigValid,
    makePayload: makeStoryDirectorPayload,
    baselineFromSaved: (saved) => saved,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateStoryDirector(id, payload, baseRevision),
    resolveConflict: createPresetConflictResolver(
      getStoryDirectors,
      makeStoryDirectorPayload,
      { resource: 'story_director', scope: PRESET_RESOURCE_SCOPE },
    ),
    onSaved: mergeSavedStoryDirector,
    onAutoSaveError: (err) => {
      console.warn('[story-director-editor] Failed to autosave story director', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[story-director-editor] Failed to autosave story director before selection change', err),
  })

  const imagePresetAutosave = usePresetResourceAutosave<ImagePreset, Partial<ImagePreset>, ImagePreset>({
    draft: imagePresetDraft,
    scopeKey: PRESET_RESOURCE_SCOPE,
    active: presetResourceKind === 'image',
    makePayload: makeImagePresetPayload,
    baselineFromSaved: (saved) => saved,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateImagePreset(id, payload, baseRevision),
    resolveConflict: createPresetConflictResolver(
      getImagePresets,
      makeImagePresetPayload,
      { resource: 'image_preset', scope: PRESET_RESOURCE_SCOPE },
    ),
    onSaved: mergeSavedImagePreset,
    onAutoSaveError: (err) => {
      console.warn('[image-preset-editor] Failed to autosave image preset', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[image-preset-editor] Failed to autosave image preset before selection change', err),
  })

  const eventPackageAutosave = usePresetResourceAutosave<EventPackageModule, Partial<EventPackageModule>, EventPackageModule>({
    draft: eventPackageDraft,
    scopeKey: PRESET_RESOURCE_SCOPE,
    active: presetResourceKind === 'event',
    valid: presetConfigValid,
    makePayload: makeEventPackagePayload,
    baselineFromSaved: (saved) => saved,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateEventPackage(id, payload, baseRevision),
    resolveConflict: createPresetConflictResolver(
      getEventPackages,
      makeEventPackagePayload,
      { resource: 'event_package', scope: PRESET_RESOURCE_SCOPE },
    ),
    onSaved: mergeSavedEventPackage,
    onAutoSaveError: (err) => {
      console.warn('[event-package-editor] Failed to autosave event package', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[event-package-editor] Failed to autosave event package before selection change', err),
  })

  const ruleSystemAutosave = usePresetResourceAutosave<RuleSystemModule, Partial<RuleSystemModule>, RuleSystemModule>({
    draft: ruleSystemDraft,
    scopeKey: PRESET_RESOURCE_SCOPE,
    active: presetResourceKind === 'rule',
    valid: presetConfigValid,
    makePayload: makeRuleSystemPayload,
    baselineFromSaved: (saved) => saved,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateRuleSystem(id, payload, baseRevision),
    resolveConflict: createPresetConflictResolver(
      getRuleSystems,
      makeRuleSystemPayload,
      { resource: 'rule_system', scope: PRESET_RESOURCE_SCOPE },
    ),
    onSaved: mergeSavedRuleSystem,
    onAutoSaveError: (err) => {
      console.warn('[rule-system-editor] Failed to autosave TRPG rules', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[rule-system-editor] Failed to autosave TRPG rules before selection change', err),
  })

  const actorStateAutosave = usePresetResourceAutosave<ActorStateModule, Partial<ActorStateModule>, ActorStateModule>({
    draft: actorStateDraft,
    scopeKey: PRESET_RESOURCE_SCOPE,
    active: presetResourceKind === 'actor-state',
    valid: presetConfigValid,
    makePayload: makeActorStatePayload,
    baselineFromSaved: (saved) => saved,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateActorState(id, payload, baseRevision),
    resolveConflict: createPresetConflictResolver(
      getActorStates,
      makeActorStatePayload,
      { resource: 'actor_state', scope: PRESET_RESOURCE_SCOPE },
    ),
    onSaved: mergeSavedActorState,
    onAutoSaveError: (err) => {
      console.warn('[actor-state-editor] Failed to autosave actor state system', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[actor-state-editor] Failed to autosave actor state system before selection change', err),
  })

  usePresetDraftSync(resources, {
    teller: tellerAutosave,
    director: storyDirectorAutosave,
    image: imagePresetAutosave,
    event: eventPackageAutosave,
    rule: ruleSystemAutosave,
    'actor-state': actorStateAutosave,
  })

  const autosaveForKind = (kind: PresetResourceKind): AutosaveController => {
    if (kind === 'director') return storyDirectorAutosave
    if (kind === 'image') return imagePresetAutosave
    if (kind === 'event') return eventPackageAutosave
    if (kind === 'rule') return ruleSystemAutosave
    if (kind === 'actor-state') return actorStateAutosave
    return tellerAutosave
  }

  const showInvalidPresetConfigNotice = () => {
    const canRestoreBuiltin = currentPresetBuiltinOverridden(presetResourceKind, presetDrafts)
    console.warn('[preset-settings] Invalid JSON blocked saving or switching presets', {
      kind: presetResourceKind,
      builtinOverride: canRestoreBuiltin,
    })
    toast.error(t('settingPanel.presetConfig.invalidTitle'), {
      id: PRESET_CONFIG_INVALID_TOAST_ID,
      description: t(canRestoreBuiltin
        ? 'settingPanel.presetConfig.invalidBuiltinDescription'
        : 'settingPanel.presetConfig.invalidDescription'),
      action: canRestoreBuiltin
        ? {
            label: t('settingPanel.restoreBuiltin'),
            onClick: () => void handleRestoreBuiltinPreset(),
          }
        : undefined,
    })
  }

  const canLeavePresetResource = () => {
    if (isPresetConfigResourceKind(presetResourceKind) && !presetConfigValidRef.current) {
      showInvalidPresetConfigNotice()
      return false
    }
    return true
  }

  async function flushPresetResourceAutoSave() {
    if (!canLeavePresetResource()) return false
    const controller = autosaveForKind(presetResourceKind)
    const pendingSave = controller.flushPending()
    const save = pendingSave ?? (controller.status === 'error' ? controller.saveNow('manual') : null)
    if (!save) return true
    setTransitioning(true)
    try {
      await save
      return true
    } catch (err) {
      console.warn('[preset-settings] Save before resource switch failed; preserving the current editor', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
      return false
    } finally {
      setTransitioning(false)
    }
  }

  const currentActivePresetId = (kind: PresetResourceKind) => {
    if (kind === 'director') return activeStoryDirectorId
    if (kind === 'image') return activeImagePresetId
    if (kind === 'event') return activeEventPackageId
    if (kind === 'rule') return activeRuleSystemId
    if (kind === 'actor-state') return activeActorStateId
    return activeTellerId
  }

  const setActivePresetId = (kind: Exclude<PresetResourceKind, 'teller'>, id: string) => {
    if (kind === 'director') setActiveStoryDirectorId(id)
    if (kind === 'image') setActiveImagePresetId(id)
    if (kind === 'event') setActiveEventPackageId(id)
    if (kind === 'rule') setActiveRuleSystemId(id)
    if (kind === 'actor-state') setActiveActorStateId(id)
  }

  const presetItemsForKind = (kind: PresetResourceKind): { id: string }[] => {
    if (kind === 'director') return storyDirectors
    if (kind === 'image') return imagePresets
    if (kind === 'event') return eventPackages
    if (kind === 'rule') return ruleSystems
    if (kind === 'actor-state') return actorStates
    return tellers
  }

  const { handleSelectTeller, selectPresetResource, handleSelectDirectoryEntry } = usePresetSelection({
    presetResourceKind,
    setPresetResourceKind,
    activeTellerId,
    setActiveTellerId,
    currentActivePresetId,
    setActivePresetId,
    flushPresetResourceAutoSave,
    closeDirectory: () => closeDirectoryRef.current(),
  })
  const toolNavigationNonceRef = useRef(0)
  useEffect(() => {
    const intent = toolNavigationIntent
    if (!intent || intent.nonce === toolNavigationNonceRef.current || intent.target.kind !== 'config_resource') return
    const kind = presetKindForConfigResource(intent.target.resource)
    if (!kind) return
    const items = presetItemsForKind(kind)
    const requestedID = intent.target.resource === 'style_reference' ? '' : intent.target.id || ''
    const id = requestedID || items[0]?.id || ''
    if (!id) {
      setPresetResourceKind(kind)
      toolNavigationNonceRef.current = intent.nonce
      return
    }
    if (!items.some((item) => item.id === id)) return
    toolNavigationNonceRef.current = intent.nonce
    if (kind === 'teller') void handleSelectTeller(id)
    else void selectPresetResource(kind, id)
  }, [actorStates, eventPackages, imagePresets, ruleSystems, storyDirectors, tellers, toolNavigationIntent?.nonce])

  const handleCreateTeller = async () => {
    if (!(await flushPresetResourceAutoSave())) return
    setSaving(true)
    try {
      const teller = await createInteractiveTeller(newTellerDraft(t))
      setPresetResourceKind('teller')
      await refreshTellers(teller.id)
      closeDirectoryRef.current()
    } finally {
      setSaving(false)
    }
  }

  const handleCreateStoryDirector = async () => {
    if (!(await flushPresetResourceAutoSave())) return
    setSaving(true)
    try {
      const director = await createStoryDirector(newStoryDirectorDraft(t))
      setPresetResourceKind('director')
      await refreshStoryDirectors(director.id)
      closeDirectoryRef.current()
    } finally {
      setSaving(false)
    }
  }

  const handleCreateEventPackage = async () => {
    if (!(await flushPresetResourceAutoSave())) return
    setSaving(true)
    try {
      const item = await createEventPackage(newEventPackageDraft(t))
      setPresetResourceKind('event')
      await refreshEventPackages(item.id)
      closeDirectoryRef.current()
    } finally {
      setSaving(false)
    }
  }

  const handleCreateRuleSystem = async () => {
    if (!(await flushPresetResourceAutoSave())) return
    setSaving(true)
    try {
      const item = await createRuleSystem(newRuleSystemDraft(t))
      setPresetResourceKind('rule')
      await refreshRuleSystems(item.id)
      closeDirectoryRef.current()
    } finally {
      setSaving(false)
    }
  }

  const handleCreateActorState = async () => {
    if (!(await flushPresetResourceAutoSave())) return
    setSaving(true)
    try {
      const item = await createActorState(newActorStateDraft(t))
      setPresetResourceKind('actor-state')
      await refreshActorStates(item.id)
      closeDirectoryRef.current()
    } finally {
      setSaving(false)
    }
  }

  const handleCreateImagePreset = async () => {
    if (!(await flushPresetResourceAutoSave())) return
    setSaving(true)
    try {
      const preset = await createImagePreset(newImagePresetDraft(t))
      setPresetResourceKind('image')
      await refreshImagePresets(preset.id)
      closeDirectoryRef.current()
    } finally {
      setSaving(false)
    }
  }

  const createPresetResource = (kind: PresetResourceKind) => {
    if (kind === 'director') return handleCreateStoryDirector()
    if (kind === 'image') return handleCreateImagePreset()
    if (kind === 'event') return handleCreateEventPackage()
    if (kind === 'rule') return handleCreateRuleSystem()
    if (kind === 'actor-state') return handleCreateActorState()
    return handleCreateTeller()
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

  async function handleRestoreBuiltinPreset() {
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
      } else if (presetResourceKind === 'director' && storyDirectorDraft) {
        await deleteStoryDirector(storyDirectorDraft.id)
        await refreshStoryDirectors(storyDirectorDraft.id)
      } else if (tellerDraft) {
        await deleteInteractiveTeller(tellerDraft.id)
        await refreshTellers(tellerDraft.id)
      }
      toast.dismiss(PRESET_CONFIG_INVALID_TOAST_ID)
      toast.success(t('settingPanel.restoreBuiltinDone'))
    } catch (err) {
      toast.error((err as Error).message || t('settingPanel.restoreBuiltinFailed'))
    } finally {
      setSaving(false)
    }
  }

  const flushActivePresetAutosave = async () => {
    if (isPresetConfigResourceKind(presetResourceKind) && !presetConfigValidRef.current) {
      showInvalidPresetConfigNotice()
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

  const closePanel = async () => {
    if (!onClose || !(await flushPresetResourceAutoSave())) return
    onClose()
  }

  const currentPresetDraft = () => {
    if (presetResourceKind === 'director') return storyDirectorDraft
    if (presetResourceKind === 'image') return imagePresetDraft
    if (presetResourceKind === 'event') return eventPackageDraft
    if (presetResourceKind === 'rule') return ruleSystemDraft
    if (presetResourceKind === 'actor-state') return actorStateDraft
    return tellerDraft
  }

  const isTellerConfigAgentActive = activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID
  const activeDraft = currentPresetDraft()
  const activeAutosave = autosaveForKind(presetResourceKind)
  const busy = saving || transitioning
  const canRestoreBuiltinPreset = !isTellerConfigAgentActive && currentPresetBuiltinOverridden(presetResourceKind, presetDrafts)
  const titleIcon = isTellerConfigAgentActive ? Bot : presetResourceIcon(presetResourceKind)
  const title = isTellerConfigAgentActive ? t('settingPanel.tellerAgent.title') : presetEditorTitle(presetResourceKind, presetDrafts, t)
  const subtitle = isTellerConfigAgentActive ? t('settingPanel.tellerAgent.subtitle') : presetEditorSubtitle(presetResourceKind, presetDrafts, t)

  const presetDirectorySections = applyPresetDirectoryOrder(buildPresetDirectorySections({
    lists: { tellers, storyDirectors, imagePresets, eventPackages, ruleSystems, actorStates },
    onCreateKind: (kind) => void createPresetResource(kind),
    t,
  }), presetDirectoryOrder.order)

  const directoryItemIdsForKind = (kind: PresetResourceKind) => {
    if (kind === 'director') return storyDirectors.map((item) => presetDirectoryEntryId(kind, item.id))
    if (kind === 'image') return imagePresets.map((item) => presetDirectoryEntryId(kind, item.id))
    if (kind === 'event') return eventPackages.map((item) => presetDirectoryEntryId(kind, item.id))
    if (kind === 'rule') return ruleSystems.map((item) => presetDirectoryEntryId(kind, item.id))
    if (kind === 'actor-state') return actorStates.map((item) => presetDirectoryEntryId(kind, item.id))
    return tellers.map((item) => presetDirectoryEntryId(kind, item.id))
  }

  const activeDirectoryId = isTellerConfigAgentActive
    ? TELLER_CONFIG_AGENT_ENTRY_ID
    : presetDirectoryEntryId(presetResourceKind, currentActivePresetId(presetResourceKind))

  const directoryPanel = (
    <PresetDirectorySidebar
      sections={presetDirectorySections}
      activeId={activeDirectoryId}
      activeSectionId={presetResourceKind}
      agentEntryId={TELLER_CONFIG_AGENT_ENTRY_ID}
      saving={busy}
      onSelect={handleSelectDirectoryEntry}
      onReorderItems={(sectionId, orderedItemIds) => {
        const kind = sectionId as PresetResourceKind
        presetDirectoryOrder.reorderItems(kind, orderedItemIds, directoryItemIdsForKind(kind))
      }}
    />
  )

  return (
    <section className="preset-workspace h-full min-h-0 text-[var(--nova-text)]">
      <AdaptiveSurface
        left={{
          id: 'setting-directory',
          title: t('settingPanel.mode.teller'),
          side: 'left',
          icon: <SlidersHorizontal className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />,
          content: directoryPanel,
          desktopClassName: 'min-h-0 border-r border-[var(--preset-line)]',
          mobileClassName: embedded ? 'w-[min(86vw,320px)]' : 'w-[min(90vw,360px)]',
        }}
        className="h-full"
        mainClassName="min-h-0 min-w-0"
        leftResize={{
          layoutKey: embedded ? 'nova-embedded-preset-directory-layout' : 'nova-preset-directory-layout',
          label: t('layout.resize.sidebar'),
          defaultSize: embedded ? '224px' : '280px',
          minSize: embedded ? '180px' : '220px',
          maxSize: '42%',
        }}
        collapseAt={embedded ? 760 : 820}
      >
        {({ isMobile, openLeft, closePane }) => {
          closeDirectoryRef.current = closePane
          return (
          <main className="preset-workspace-main flex h-full min-h-0 min-w-0 flex-1 flex-col">
            <FeaturePageShell
              icon={titleIcon}
              title={title}
              subtitle={subtitle}
              leadingContent={isMobile ? (
                <MobilePaneTrigger
                  side="left"
                  label={t('workbench.mobile.openSidePanel', { label: t('settingPanel.mode.teller') })}
                  onClick={openLeft}
                />
              ) : undefined}
              onSaveShortcut={isTellerConfigAgentActive ? undefined : flushActivePresetAutosave}
              onClose={onClose ? () => void closePanel() : undefined}
              actions={(
                <>
                  {!isTellerConfigAgentActive && activeDraft ? (
                    <AutosaveStatusIndicator
                      status={activeAutosave.status}
                      error={activeAutosave.error}
                      onRetry={flushActivePresetAutosave}
                    />
                  ) : null}
                  {canRestoreBuiltinPreset && (
                    <Button className={actionButtonClassName} variant="outline" size="sm" disabled={busy} onClick={() => void handleRestoreBuiltinPreset()} aria-label={t('settingPanel.restoreBuiltin')}>
                      <RotateCcw data-icon="inline-start" />
                      <span className="preset-action-label">{t('settingPanel.restoreBuiltin')}</span>
                    </Button>
                  )}
                  {!isTellerConfigAgentActive && activeDraft?.custom ? (
                    <Button className={iconActionClassName} variant="outline" size="icon" disabled={busy} onClick={handleDelete} aria-label={t(PRESET_DELETE_COPY[presetResourceKind].titleKey)}>
                      <Trash2 data-icon="inline-start" />
                    </Button>
                  ) : null}
                </>
              )}
              className="text-[var(--nova-text)]"
              topbarClassName="preset-workspace-toolbar"
            >
              {isTellerConfigAgentActive ? (
                <ConfigManagerChat
                  projectId={projectId}
                  origin="teller"
                  resourceId={TELLER_CONFIG_AGENT_ENTRY_ID}
                  context={{
                    teller_count: String(tellers.length),
                    event_package_count: String(eventPackages.length),
                    rule_system_count: String(ruleSystems.length),
                    actor_state_count: String(actorStates.length),
                    story_director_count: String(storyDirectors.length),
                    image_preset_count: String(imagePresets.length),
                  }}
                  onMutated={() => {
                    void refreshTellers()
                    void refreshEventPackages()
                    void refreshRuleSystems()
                    void refreshActorStates()
                    void refreshStoryDirectors()
                    void refreshImagePresets()
                  }}
                />
              ) : (
                <PresetResourcePane
                  kind={presetResourceKind}
                  projectId={projectId}
                  tellers={tellers}
                  storyDirectors={storyDirectors}
                  imagePresets={imagePresets}
                  eventPackages={eventPackages}
                  ruleSystems={ruleSystems}
                  actorStates={actorStates}
                  tellerDraft={tellerDraft}
                  setTellerDraft={setTellerDraft}
                  activeSlotId={activeSlotId}
                  setActiveSlotId={setActiveSlotId}
                  storyDirectorDraft={storyDirectorDraft}
                  setStoryDirectorDraft={setStoryDirectorDraft}
                  imagePresetDraft={imagePresetDraft}
                  setImagePresetDraft={setImagePresetDraft}
                  eventPackageDraft={eventPackageDraft}
                  setEventPackageDraft={setEventPackageDraft}
                  ruleSystemDraft={ruleSystemDraft}
                  setRuleSystemDraft={setRuleSystemDraft}
                  actorStateDraft={actorStateDraft}
                  setActorStateDraft={setActorStateDraft}
                  onOpenActorState={(id) => selectPresetResource('actor-state', id)}
                  onOpenRuleSystem={(id) => selectPresetResource('rule', id)}
                  onSave={flushActivePresetAutosave}
                  onValidityChange={setPresetConfigValid}
                />
              )}
            </FeaturePageShell>
          </main>
          )
        }}
      </AdaptiveSurface>
      <ConfirmDialog
        open={Boolean(deletePresetTarget)}
        onOpenChange={(open) => {
          if (!open && !saving) setDeletePresetTarget(null)
        }}
        title={deletePresetTarget ? t(deletePresetTarget.titleKey) : t('common.delete')}
        description={deletePresetTarget ? t(deletePresetTarget.descriptionKey, { name: deletePresetTarget.name }) : ''}
        confirmLabel={t('common.delete')}
        tone="danger"
        onConfirm={confirmDeletePresetTarget}
      />
    </section>
  )
}

function presetKindForConfigResource(resource: string): PresetResourceKind | null {
  switch (resource) {
    case 'style_reference':
    case 'narrative_style':
      return 'teller'
    case 'story_director':
      return 'director'
    case 'event_package':
      return 'event'
    case 'rule_system':
      return 'rule'
    case 'state_system':
      return 'actor-state'
    case 'image_preset':
      return 'image'
    default:
      return null
  }
}

function presetResourceIcon(kind: PresetResourceKind) {
  if (kind === 'director') return Compass
  if (kind === 'image') return Sparkles
  if (kind === 'event') return ScrollText
  if (kind === 'rule') return Dice5
  if (kind === 'actor-state') return Database
  return SlidersHorizontal
}
