import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { LoreItem } from '@/lib/api'
import type { PresetResourceKind } from '../../preset-ownership'
import type { EventPackageModule, StoryDirector, TellerEventPackage } from '../../types'
import { PresetMetadataPanel } from '../preset-config/PresetEditorChrome'

export const TYPE_OPTIONS = [
  { value: 'character' },
  { value: 'world' },
  { value: 'location' },
  { value: 'faction' },
  { value: 'rule' },
  { value: 'item' },
  { value: 'other' },
] as const
export const IMPORTANCE_OPTIONS = [
  { value: 'major' },
  { value: 'important' },
  { value: 'minor' },
] as const
export const LOAD_MODE_OPTIONS = [
  { value: 'resident' },
  { value: 'auto' },
  { value: 'manual' },
] as const
export const LORE_RESIDENT_TOTAL_WARNING_BYTES = 32 * 1024

export const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
export const actionButtonClassName = 'nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
export const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
export const selectClassName = 'nova-field h-8 w-full text-xs focus:ring-0'

/** 设置面板通用的字段容器：小号标签 + 控件。 */
export function Field({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return (
    <label className={cn('grid min-w-0 gap-1.5', className)}>
      <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

/** 设置面板内的虚线空态（与通用 EmptyState 组件不同的轻量变体）。 */
export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 text-center">
        <div className="text-sm font-medium text-[var(--nova-text)]">{title}</div>
        <div className="mt-1 text-xs text-[var(--nova-text-faint)]">{description}</div>
      </div>
    </div>
  )
}

export function loreTypeLabel(type: LoreItem['type'], t: (key: string) => string) {
  const key = `lore.type.${type}`
  const label = t(key)
  return label === key ? t('lore.type.other') : label
}

export function loreImportanceLabel(importance: LoreItem['importance'], t: (key: string) => string) {
  const key = `lore.importance.${importance}`
  const label = t(key)
  return label === key ? t('lore.importance.important') : label
}

export function loreLoadModeLabel(loadMode: LoreItem['load_mode'] | undefined, t: (key: string) => string) {
  const key = `lore.loadMode.${loadMode || 'auto'}`
  const label = t(key)
  return label === key ? t('lore.loadMode.auto') : label
}

export function loadModeDescription(loadMode: LoreItem['load_mode'] | undefined, t: (key: string) => string) {
  if (loadMode === 'resident') return t('settingPanel.lore.residentDesc')
  if (loadMode === 'manual') return t('settingPanel.lore.manualDesc')
  if (loadMode === 'auto') return t('settingPanel.lore.autoDesc')
  return t('settingPanel.lore.indexDesc')
}

export function presetStatusLabel(item: { custom?: boolean; builtin_overridden?: boolean }, t: (key: string) => string) {
  if (item.custom) return t('settingPanel.custom')
  if (item.builtin_overridden) return t('settingPanel.builtInOverridden')
  return t('settingPanel.builtIn')
}

/** 模块编辑器的统一外壳：元数据面板 + 内容区。 */
export function ModuleEditorShell<T extends { name: string; description: string; custom: boolean; builtin_overridden?: boolean }>({
  draft,
  setDraft,
  metadata = 'full',
  contentClassName = 'grid min-h-0 flex-1 gap-4 overflow-y-auto p-3 sm:p-4',
  children,
}: {
  draft: T
  setDraft: (draft: T | null) => void
  metadata?: 'full' | 'compact' | 'none'
  contentClassName?: string
  children: ReactNode
}) {
  const { t } = useTranslation()
  const editHint = draft.custom ? t('settingPanel.storyDirector.customEditable') : t('settingPanel.storyDirector.builtInCopyHint')
  return (
    <div className="preset-module-editor flex min-h-0 flex-1 flex-col overflow-hidden">
      {metadata !== 'none' ? (
        <PresetMetadataPanel
          name={draft.name}
          description={draft.description}
          status={presetStatusLabel(draft, t)}
          hint={editHint}
          onNameChange={(name) => setDraft({ ...draft, name })}
          onDescriptionChange={(description) => setDraft({ ...draft, description })}
        />
      ) : null}
      <div className={contentClassName}>
        {children}
      </div>
    </div>
  )
}

/** 聚合编辑器内各分区的有效性，任一分区无效则整体无效；resetKey 变化时重置。 */
export function usePresetSectionValidity(resetKey: string, onValidityChange?: (valid: boolean) => void) {
  const [validity, setValidity] = useState<Record<string, boolean>>({})

  useEffect(() => {
    setValidity({})
  }, [resetKey])

  useEffect(() => {
    onValidityChange?.(Object.values(validity).every((valid) => valid !== false))
  }, [onValidityChange, validity])

  return useCallback((section: string, valid: boolean) => {
    setValidity((current) => {
      if (current[section] === valid) return current
      return { ...current, [section]: valid }
    })
  }, [])
}

export function storyDirectorSummaryCount(director: StoryDirector) {
  return directorEventCardCount(directorResolvedEventPackages(director))
    + (director.trpg_system?.rule_templates?.length || 0)
}

function directorResolvedEventPackages(director: StoryDirector): TellerEventPackage[] {
  return director.event_packages?.length
    ? director.event_packages
    : director.resolved_snapshot?.event_packages?.length
      ? director.resolved_snapshot.event_packages
      : []
}

function directorEventCardCount(eventPackages: TellerEventPackage[] | undefined) {
  return (eventPackages || []).reduce((total, pkg) => total + (pkg.events?.length || 0), 0)
}

export function eventPackageSummaryCount(item: EventPackageModule) {
  return item.events?.length || 0
}

export function presetKindDirectoryLabel(kind: PresetResourceKind, t: (key: string) => string) {
  if (kind === 'image') return t('settingPanel.imagePresetDirectory')
  if (kind === 'director') return t('settingPanel.storyDirectorDirectory')
  if (kind === 'event') return t('settingPanel.eventPackageDirectory')
  if (kind === 'rule') return t('settingPanel.ruleSystemDirectory')
  if (kind === 'actor-state') return t('settingPanel.actorStateDirectory')
  return t('settingPanel.rulePackages')
}

export function presetKindCreateLabel(kind: PresetResourceKind, t: (key: string) => string) {
  if (kind === 'image') return t('settingPanel.newImagePreset')
  if (kind === 'director') return t('settingPanel.newStoryDirector')
  if (kind === 'event') return t('settingPanel.newEventPackage')
  if (kind === 'rule') return t('settingPanel.newRuleSystem')
  if (kind === 'actor-state') return t('settingPanel.newActorState')
  return t('settingPanel.newTeller')
}
