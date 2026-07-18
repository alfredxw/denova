import { useEffect, useRef, useState } from 'react'
import { Bot, Compass, Database, Dice5, Loader2, PanelLeft, RotateCcw, Save, ScrollText, SlidersHorizontal, Sparkles, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { ResourceDirectory } from '@/components/resource-directory/ResourceDirectory'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { createActorState, createEventPackage, createImagePreset, createInteractiveTeller, createRuleSystem, createStoryDirector, deleteActorState, deleteEventPackage, deleteImagePreset, deleteInteractiveTeller, deleteRuleSystem, deleteStoryDirector, updateActorState, updateEventPackage, updateImagePreset, updateInteractiveTeller, updateRuleSystem, updateStoryDirector } from '../../api'
import type { PresetResourceKind, PresetUsageMode } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, Teller } from '../../types'
import { PresetResourcePane } from './PresetResourcePane'
import { buildPresetDirectorySections, presetDirectoryEntryId } from './preset-directory-sections'
import { usePresetDraftSync, usePresetResources } from './use-preset-resources'
import { usePresetSelection } from './use-preset-selection'
import { usePresetResourceAutosave } from './usePresetResourceAutosave'
import { currentPresetBuiltinOverridden, EMPTY_IMAGE_PRESETS, EMPTY_STORY_DIRECTORS, EMPTY_TELLERS, isPresetConfigResourceKind, makeActorStatePayload, makeEventPackagePayload, makeImagePresetPayload, makeRuleSystemPayload, makeStoryDirectorPayload, makeTellerPayload, newActorStateDraft, newEventPackageDraft, newImagePresetDraft, newRuleSystemDraft, newStoryDirectorDraft, newTellerDraft, presetEditorSubtitle, presetEditorTitle, presetResourceDraftSignature, PRESET_DELETE_COPY, TELLER_CONFIG_AGENT_ENTRY_ID, type PresetDeleteTarget } from './presetResources'

interface PresetSettingsPanelProps {
  workspace: string
  tellers?: Teller[]
  storyDirectors?: StoryDirector[]
  imagePresets?: ImagePreset[]
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

const actionButtonClassName = 'gap-1.5 border-[var(--preset-line)] bg-[var(--preset-raised)] text-[var(--nova-text-muted)] shadow-none hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const iconActionClassName = 'border-[var(--preset-line)] bg-transparent text-[var(--nova-text-muted)] shadow-none hover:border-[var(--nova-danger-border)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]'
const PRESET_CONFIG_INVALID_TOAST_ID = 'preset-config-invalid'

export function PresetSettingsPanel({
  workspace,
  tellers: externalTellers = EMPTY_TELLERS,
  storyDirectors: externalStoryDirectors = EMPTY_STORY_DIRECTORS,
  imagePresets: externalImagePresets = EMPTY_IMAGE_PRESETS,
  presetUsageMode = 'game',
  onTellersChange,
  onStoryDirectorsChange,
  onImagePresetsChange,
  embedded = false,
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
    workspace,
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

  useEffect(() => {
    presetConfigValidRef.current = presetConfigValid
  }, [presetConfigValid])

  useEffect(() => {
    setPresetConfigValid(true)
  }, [activeActorStateId, activeEventPackageId, activeRuleSystemId, activeStoryDirectorId, presetResourceKind])

  const tellerAutosave = usePresetResourceAutosave<Teller, Partial<Teller>, Teller>({
    draft: tellerDraft,
    scopeKey: workspace,
    active: presetResourceKind === 'teller' && activeTellerId !== TELLER_CONFIG_AGENT_ENTRY_ID,
    makePayload: makeTellerPayload,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateInteractiveTeller(id, payload, baseRevision, workspace),
    onSaved: (teller, mode) => {
      mergeSavedTeller(teller, mode === 'auto')
    },
    onAutoSaveError: (err) => {
      console.warn('[teller-editor] 自动保存叙事风格失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[teller-editor] 切换条目前自动保存叙事风格失败', err),
  })

  const storyDirectorAutosave = usePresetResourceAutosave<StoryDirector, Partial<StoryDirector>, StoryDirector>({
    draft: storyDirectorDraft,
    scopeKey: workspace,
    active: presetResourceKind === 'director',
    valid: presetConfigValid,
    makePayload: makeStoryDirectorPayload,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateStoryDirector(id, payload, baseRevision, workspace),
    onSaved: (director, mode) => {
      mergeSavedStoryDirector(director, mode === 'auto')
    },
    onAutoSaveError: (err) => {
      console.warn('[story-director-editor] 自动保存故事导演失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[story-director-editor] 切换条目前自动保存故事导演失败', err),
  })

  const imagePresetAutosave = usePresetResourceAutosave<ImagePreset, Partial<ImagePreset>, ImagePreset>({
    draft: imagePresetDraft,
    scopeKey: workspace,
    active: presetResourceKind === 'image',
    makePayload: makeImagePresetPayload,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateImagePreset(id, payload, baseRevision, workspace),
    onSaved: (preset, mode) => {
      mergeSavedImagePreset(preset, mode === 'auto')
    },
    onAutoSaveError: (err) => {
      console.warn('[image-preset-editor] 自动保存图像方案失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[image-preset-editor] 切换条目前自动保存图像方案失败', err),
  })

  const eventPackageAutosave = usePresetResourceAutosave<EventPackageModule, Partial<EventPackageModule>, EventPackageModule>({
    draft: eventPackageDraft,
    scopeKey: workspace,
    active: presetResourceKind === 'event',
    valid: presetConfigValid,
    makePayload: makeEventPackagePayload,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateEventPackage(id, payload, baseRevision, workspace),
    onSaved: (item, mode) => {
      mergeSavedEventPackage(item, mode === 'auto')
    },
    onAutoSaveError: (err) => {
      console.warn('[event-package-editor] 自动保存事件包失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[event-package-editor] 切换条目前自动保存事件包失败', err),
  })

  const ruleSystemAutosave = usePresetResourceAutosave<RuleSystemModule, Partial<RuleSystemModule>, RuleSystemModule>({
    draft: ruleSystemDraft,
    scopeKey: workspace,
    active: presetResourceKind === 'rule',
    valid: presetConfigValid,
    makePayload: makeRuleSystemPayload,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateRuleSystem(id, payload, baseRevision, workspace),
    onSaved: (item, mode) => {
      mergeSavedRuleSystem(item, mode === 'auto')
    },
    onAutoSaveError: (err) => {
      console.warn('[rule-system-editor] 自动保存 TRPG 检定失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[rule-system-editor] 切换条目前自动保存 TRPG 检定失败', err),
  })

  const actorStateAutosave = usePresetResourceAutosave<ActorStateModule, Partial<ActorStateModule>, ActorStateModule>({
    draft: actorStateDraft,
    scopeKey: workspace,
    active: presetResourceKind === 'actor-state',
    valid: presetConfigValid,
    makePayload: makeActorStatePayload,
    signature: presetResourceDraftSignature,
    save: (id, payload, baseRevision) => updateActorState(id, payload, baseRevision, workspace),
    onSaved: (item, mode) => {
      mergeSavedActorState(item, mode === 'auto')
    },
    onAutoSaveError: (err) => {
      console.warn('[actor-state-editor] 自动保存状态系统失败', err)
      toast.error((err as Error).message || t('editor.saveFailed'))
    },
    onFlushError: (err) => console.warn('[actor-state-editor] 切换条目前自动保存状态系统失败', err),
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
    console.warn('[preset-settings] 无效 JSON 阻止保存或切换配置', {
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
    const pendingSave = autosaveForKind(presetResourceKind).flushPending()
    if (!pendingSave) return true
    setTransitioning(true)
    try {
      await pendingSave
      return true
    } catch (err) {
      console.warn('[preset-settings] 切换资源前保存失败，已保留当前编辑器', err)
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

  const { selectPresetResource, handleSelectDirectoryEntry } = usePresetSelection({
    workspace,
    presetUsageMode,
    presetResourceKind,
    setPresetResourceKind,
    activeTellerId,
    setActiveTellerId,
    currentActivePresetId,
    setActivePresetId,
    presetItemsForKind,
    flushPresetResourceAutoSave,
    closeDirectory: () => closeDirectoryRef.current(),
  })

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

  const handleSave = async () => {
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
  const busy = saving || transitioning
  const canRestoreBuiltinPreset = !isTellerConfigAgentActive && currentPresetBuiltinOverridden(presetResourceKind, presetDrafts)
  const presetConfigInvalid = isPresetConfigResourceKind(presetResourceKind) && !presetConfigValid
  const saveDisabled = busy || presetConfigInvalid || !activeDraft
  const titleIcon = presetResourceIcon(presetResourceKind)

  const presetDirectorySections = buildPresetDirectorySections({
    lists: { tellers, storyDirectors, imagePresets, eventPackages, ruleSystems, actorStates },
    presetUsageMode,
    presetResourceKind,
    onCreateKind: (kind) => void createPresetResource(kind),
    t,
  })

  const activeDirectoryId = isTellerConfigAgentActive
    ? TELLER_CONFIG_AGENT_ENTRY_ID
    : presetDirectoryEntryId(presetResourceKind, currentActivePresetId(presetResourceKind))

  const directoryPanel = (
    <div className="preset-directory nova-sidebar flex h-full min-h-0 flex-col overflow-hidden">
      <ResourceDirectory
        sections={presetDirectorySections}
        activeId={activeDirectoryId}
        onSelect={handleSelectDirectoryEntry}
        saving={busy}
        pinnedEntries={[{ id: TELLER_CONFIG_AGENT_ENTRY_ID, label: t('settingPanel.tellerAgent.title'), icon: Bot }]}
        searchPlaceholder={t('settingPanel.directory.search')}
        showExpandCollapseAll
        expandedSectionId={presetResourceKind}
      />
    </div>
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
          desktopClassName: `min-h-0 border-r border-[var(--preset-line)] ${embedded ? 'w-56' : 'w-[280px]'}`,
          mobileClassName: embedded ? 'w-[min(86vw,320px)]' : 'w-[min(90vw,360px)]',
        }}
        className="h-full"
        mainClassName="min-h-0 min-w-0"
        desktopGridClassName={embedded ? 'grid-cols-[14rem_minmax(0,1fr)]' : 'grid-cols-[280px_minmax(0,1fr)]'}
        collapseAt={embedded ? 760 : 820}
      >
        {({ isMobile, openLeft, closePane }) => {
          closeDirectoryRef.current = closePane
          return (
          <main className="preset-workspace-main flex h-full min-h-0 min-w-0 flex-1 flex-col">
            <div className="preset-workspace-toolbar flex shrink-0 items-center justify-between gap-3 border-b px-4">
              <div className="flex min-w-0 flex-1 items-center gap-2">
                {isMobile && (
                  <Button type="button" variant="ghost" size="icon" className="h-8 w-8 shrink-0 rounded-[10px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" aria-label={t('workbench.mobile.openSidePanel', { label: t('settingPanel.mode.teller') })} onClick={openLeft}>
                    <PanelLeft className="h-4 w-4" />
                  </Button>
                )}
                <span className="preset-title-icon">{titleIcon}</span>
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <h2 className="preset-workspace-title truncate text-sm font-semibold text-[var(--preset-ink)]">{isTellerConfigAgentActive ? t('settingPanel.tellerAgent.title') : presetEditorTitle(presetResourceKind, presetDrafts, t)}</h2>
                  </div>
                  <p className="preset-title-subtitle mt-0.5 truncate text-[11px] text-[var(--nova-text-muted)]">{isTellerConfigAgentActive ? t('settingPanel.tellerAgent.subtitle') : presetEditorSubtitle(presetResourceKind, presetDrafts, t)}</p>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-1.5">
                {canRestoreBuiltinPreset && (
                  <Button className={actionButtonClassName} variant="outline" size="sm" disabled={busy} onClick={() => void handleRestoreBuiltinPreset()} aria-label={t('settingPanel.restoreBuiltin')} title={t('settingPanel.restoreBuiltin')}>
                    <RotateCcw className="h-4 w-4" />
                    <span className="preset-action-label">{t('settingPanel.restoreBuiltin')}</span>
                  </Button>
                )}
                {!isTellerConfigAgentActive && activeDraft?.custom ? (
                  <Button className={iconActionClassName} variant="outline" size="icon" disabled={busy} onClick={handleDelete} aria-label={t(PRESET_DELETE_COPY[presetResourceKind].titleKey)}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                ) : null}
                {!isTellerConfigAgentActive && (
                  <Button className="preset-primary-action gap-1.5" variant="outline" size="sm" disabled={saveDisabled} onClick={handleSave} aria-label={t('common.save')}>
                    {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                    <span className="preset-action-label">{t('common.save')}</span>
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
                workspace={workspace}
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
                onSave={handleSave}
                onValidityChange={setPresetConfigValid}
              />
            )}
          </main>
          )
        }}
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

function presetResourceIcon(kind: PresetResourceKind) {
  const iconClassName = 'h-4 w-4'
  if (kind === 'director') return <Compass className={iconClassName} />
  if (kind === 'image') return <Sparkles className={iconClassName} />
  if (kind === 'event') return <ScrollText className={iconClassName} />
  if (kind === 'rule') return <Dice5 className={iconClassName} />
  if (kind === 'actor-state') return <Database className={iconClassName} />
  return <SlidersHorizontal className={iconClassName} />
}
