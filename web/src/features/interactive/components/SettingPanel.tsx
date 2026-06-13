import { useEffect, useRef, useState } from 'react'
import { BookMarked, Building2, Database, FileText, Library, MapPin, Save, ScrollText, SlidersHorizontal, Trash2, UserRound } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { createLoreVersion, createLoreItem, deleteLoreItem, getLoreItems, getLoreVersions, readFile, restoreLoreVersion, saveFile, updateLoreItem, type LoreAgentResult, type LoreItem, type LoreVersion } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { createInteractiveTeller, deleteInteractiveTeller, getInteractiveTellers, updateInteractiveTeller } from '../api'
import type { Teller, TellerAgentResult } from '../types'
import { LoreAgentChat } from './SettingPanelAgentChats'
import { TellerAgentChat } from './SettingPanelTellerAgentChat'
import { CreatorDirectory, CreatorEditor, LoreDirectory, LoreEditor, TellerDirectory } from './SettingPanelSections'
import { TellerEditor } from './SettingPanelTellerEditor'

const CREATOR_PATH = 'CREATOR.md'
const LORE_AGENT_ENTRY_ID = '__lore_agent__'
const TELLER_AGENT_ENTRY_ID = '__teller_agent__'
const EMPTY_TELLERS: Teller[] = []

export type SettingPanelMode = 'lore' | 'creator' | 'teller'

type LoreType = LoreItem['type']

interface KnowledgeSection {
  id: string
  labelKey: string
  icon: LucideIcon
  types: LoreType[]
  createType: LoreType
  createName: string
  tag?: string
  excludeTag?: string
}

const KNOWLEDGE_SECTIONS: KnowledgeSection[] = [
  {
    id: 'characters',
    labelKey: 'lore.type.character',
    icon: UserRound,
    types: ['character'],
    createType: 'character',
    createName: '新角色',
  },
  {
    id: 'locations',
    labelKey: 'lore.type.location',
    icon: MapPin,
    types: ['location'],
    createType: 'location',
    createName: '新地点',
  },
  {
    id: 'factions',
    labelKey: 'lore.type.faction',
    icon: Building2,
    types: ['faction'],
    createType: 'faction',
    createName: '新组织',
  },
  {
    id: 'rules',
    labelKey: 'lore.type.rule',
    icon: ScrollText,
    types: ['world', 'rule'],
    createType: 'rule',
    createName: '新规则',
  },
  {
    id: 'templates',
    labelKey: 'settingPanel.section.templates',
    icon: FileText,
    types: ['other'],
    createType: 'other',
    createName: '新模板',
    tag: '模板',
  },
  {
    id: 'assets',
    labelKey: 'settingPanel.section.assets',
    icon: Library,
    types: ['item', 'other'],
    createType: 'item',
    createName: '新素材',
    excludeTag: '模板',
  },
]

interface SettingPanelProps {
  mode?: SettingPanelMode
  workspace?: string
  tellers?: Teller[]
  onTellersChange?: (tellers: Teller[]) => void
  embedded?: boolean
}

export function SettingPanel({ mode, workspace = '', tellers: externalTellers = EMPTY_TELLERS, onTellersChange, embedded = false }: SettingPanelProps) {
  const { t } = useTranslation()
  const activeMode = mode || 'lore'
  const [items, setItems] = useState<LoreItem[]>([])
  const [activeId, setActiveId] = useState('')
  const [draft, setDraft] = useState<LoreItem | null>(null)
  const [tagDraft, setTagDraft] = useState('')
  const [query, setQuery] = useState('')
  const [versions, setVersions] = useState<LoreVersion[]>([])
  const [versionsVisible, setVersionsVisible] = useState(false)
  const [creatorContent, setCreatorContent] = useState('')
  const [tellers, setTellers] = useState<Teller[]>(externalTellers)
  const [activeTellerId, setActiveTellerId] = useState('')
  const [tellerAgentTargetId, setTellerAgentTargetId] = useState('')
  const [tellerDraft, setTellerDraft] = useState<Teller | null>(null)
  const [tellerTagDraft, setTellerTagDraft] = useState('')
  const [activeSlotId, setActiveSlotId] = useState('')
  const [saving, setSaving] = useState(false)
  const loreDraftRef = useRef<LoreItem | null>(null)
  const loreTagDraftRef = useRef('')
  const loreAutoSaveTimer = useRef<number | null>(null)
  const loreSavedSignature = useRef('')
  const tellerAutoSaveTimer = useRef<number | null>(null)
  const tellerSavedSignature = useRef('')

  useEffect(() => {
    let cancelled = false
    setItems([])
    setActiveId(LORE_AGENT_ENTRY_ID)
    setDraft(null)
    setTagDraft('')
    setQuery('')
    setVersions([])
    setVersionsVisible(false)
    if (!workspace)
      return () => {
        cancelled = true
      }
    getLoreItems()
      .then((data) => {
        if (cancelled) return
        setItems(data)
        setActiveId(LORE_AGENT_ENTRY_ID)
      })
      .catch(() => {
        if (!cancelled) {
          setItems([])
          setActiveId(LORE_AGENT_ENTRY_ID)
        }
      })
    return () => {
      cancelled = true
    }
  }, [workspace])

  useEffect(() => {
    if (activeMode !== 'lore') return
    let cancelled = false
    if (!workspace) {
      setVersions([])
      return () => {
        cancelled = true
      }
    }
    getLoreVersions()
      .then((data) => {
        if (!cancelled) setVersions(data)
      })
      .catch(() => {
        if (!cancelled) setVersions([])
      })
    return () => {
      cancelled = true
    }
  }, [activeMode, workspace])

  useEffect(() => {
    const item = items.find((entry) => entry.id === activeId) || null
    const nextDraft = item ? { ...item, tags: [...(item.tags || [])] } : null
    const nextTagDraft = (item?.tags || []).join('，')
    const currentDraft = loreDraftRef.current
    const currentTagDraft = loreTagDraftRef.current
    const hasUnsavedCurrentDraft = Boolean(currentDraft?.id && currentDraft.id === item?.id && loreDraftSignature(currentDraft, currentTagDraft) !== loreSavedSignature.current)
    if (!hasUnsavedCurrentDraft) {
      setDraft(nextDraft)
      setTagDraft(nextTagDraft)
      loreSavedSignature.current = nextDraft ? loreDraftSignature(nextDraft, nextTagDraft) : ''
    }
  }, [activeId, items])

  useEffect(() => {
    loreDraftRef.current = draft
    loreTagDraftRef.current = tagDraft
  }, [draft, tagDraft])

  useEffect(() => {
    if (activeMode !== 'creator') return
    let cancelled = false
    setCreatorContent('')
    if (!workspace)
      return () => {
        cancelled = true
      }
    readFile(CREATOR_PATH)
      .then((data) => {
        if (!cancelled) setCreatorContent(data.content)
      })
      .catch(() => {
        if (!cancelled) setCreatorContent('')
      })
    return () => {
      cancelled = true
    }
  }, [activeMode, workspace])

  useEffect(() => {
    setTellers(externalTellers)
    setActiveTellerId((current) => current || externalTellers[0]?.id || '')
    setTellerAgentTargetId((current) => current || externalTellers[0]?.id || '')
  }, [externalTellers])

  useEffect(() => {
    if (activeMode !== 'teller' || onTellersChange || externalTellers.length > 0 || !workspace) return
    let cancelled = false
    getInteractiveTellers()
      .then((data) => {
        if (cancelled) return
        setTellers(data)
        setActiveTellerId((current) => current || data[0]?.id || '')
        setTellerAgentTargetId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setTellers([])
      })
    return () => {
      cancelled = true
    }
  }, [activeMode, externalTellers.length, onTellersChange, workspace])

  useEffect(() => {
    setTellers(externalTellers)
    setActiveTellerId((current) => {
      if (current && externalTellers.some((teller) => teller.id === current)) return current
      return externalTellers[0]?.id || ''
    })
    setTellerAgentTargetId((current) => {
      if (current && externalTellers.some((teller) => teller.id === current)) return current
      return externalTellers[0]?.id || ''
    })
    setTellerDraft(null)
    setTellerTagDraft('')
    setActiveSlotId('')
  }, [externalTellers, workspace])

  useEffect(() => {
    if (activeTellerId === TELLER_AGENT_ENTRY_ID) {
      setTellerDraft(null)
      setTellerTagDraft('')
      setActiveSlotId('')
      return
    }
    const teller = tellers.find((entry) => entry.id === activeTellerId) || null
    const nextDraft = teller
      ? {
          ...teller,
          tags: [...(teller.tags || [])],
          slots: [...(teller.slots || [])],
          context_policy: { ...teller.context_policy },
          style_rules: [...(teller.style_rules || [])],
        }
      : null
    setTellerDraft(nextDraft)
    setTellerTagDraft((teller?.tags || []).join('，'))
    setActiveSlotId((current) => {
      if (current && teller?.slots?.some((slot) => slot.id === current)) return current
      return teller?.slots?.[0]?.id || ''
    })
    tellerSavedSignature.current = nextDraft ? tellerDraftSignature(nextDraft, (teller?.tags || []).join('，')) : ''
  }, [activeTellerId, tellers])

  const refreshItems = async (nextActiveId?: string) => {
    const data = await getLoreItems()
    setItems(data)
    setActiveId(nextActiveId || LORE_AGENT_ENTRY_ID)
  }

  const refreshVersions = async () => {
    const data = await getLoreVersions()
    setVersions(data)
  }

  useEffect(() => {
    const onLoreUpdated = (event: Event) => {
      const detail = (event as CustomEvent<{ item_ids?: string[] }>).detail
      void refreshItems(detail?.item_ids?.[0])
      void refreshVersions()
    }
    window.addEventListener('nova:lore-updated', onLoreUpdated)
    return () => window.removeEventListener('nova:lore-updated', onLoreUpdated)
  }, [])

  const refreshTellers = async (nextActiveId?: string) => {
    const data = await getInteractiveTellers()
    setTellers(data)
    onTellersChange?.(data)
    setActiveTellerId((current) => {
      if (nextActiveId) return nextActiveId
      if (current && data.some((teller) => teller.id === current)) return current
      return data[0]?.id || ''
    })
    setTellerAgentTargetId((current) => (data.some((teller) => teller.id === current) ? current : nextActiveId || data[0]?.id || ''))
  }

  const mergeSavedTeller = (teller: Teller) => {
    setTellers((current) => current.map((entry) => (entry.id === teller.id ? teller : entry)))
    onTellersChange?.(tellers.map((entry) => (entry.id === teller.id ? teller : entry)))
    setActiveTellerId(teller.id)
    setTellerAgentTargetId((current) => current || teller.id)
  }

  const mergeSavedLoreItem = (item: LoreItem) => {
    setItems((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
  }

  const saveLoreDraft = async (mode: 'manual' | 'auto') => {
    if (!draft) return null
    const payload = { ...draft, tags: splitTags(tagDraft) }
    const signature = loreDraftSignature(payload, tagDraft)
    if (mode === 'auto' && signature === loreSavedSignature.current) return null
    const item = await updateLoreItem(draft.id, payload)
    loreSavedSignature.current = loreDraftSignature(item, (item.tags || []).join('，'))
    mergeSavedLoreItem(item)
    return item
  }

  const saveTellerDraft = async (mode: 'manual' | 'auto') => {
    if (!tellerDraft) return
    const payload = {
      ...tellerDraft,
      tags: splitTags(tellerTagDraft),
    }
    const signature = tellerDraftSignature(payload, tellerTagDraft)
    if (mode === 'auto' && signature === tellerSavedSignature.current) return
    const teller = await updateInteractiveTeller(tellerDraft.id, payload)
    tellerSavedSignature.current = tellerDraftSignature(teller, (teller.tags || []).join('，'))
    if (mode === 'manual') {
      mergeSavedTeller(teller)
    }
  }

  const handleCreateLore = async (section: KnowledgeSection = KNOWLEDGE_SECTIONS[0]) => {
    setSaving(true)
    try {
      const item = await createLoreItem({
        type: section.createType,
        name: section.createName,
        importance: section.createType === 'character' ? 'major' : 'important',
        load_mode: section.createType === 'character' ? 'resident' : 'auto',
        tags: section.tag ? [section.tag] : [],
        brief_description: `${loreTypeLabel(section.createType, t)} ${section.createName}。用 3-5 句概括本项的身份、别名、关键事实、适用场景和触发词。上下文出现相关内容时，一定要参考本项详情。`,
        content: `## ${section.createName}\n\n`,
      })
      await refreshItems(item.id)
      notifyLoreUpdated([item.id])
    } finally {
      setSaving(false)
    }
  }

  const handleCreateTeller = async () => {
    setSaving(true)
    try {
      const teller = await createInteractiveTeller(newTellerDraft())
      await refreshTellers(teller.id)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (activeMode === 'teller') {
      if (!tellerDraft || !tellerDraft.custom) return
      if (!window.confirm(t('settingPanel.confirmDeleteTeller', { name: tellerDraft.name }))) return
      setSaving(true)
      try {
        await deleteInteractiveTeller(tellerDraft.id)
        await refreshTellers()
      } finally {
        setSaving(false)
      }
      return
    }
    if (!draft) return
    if (!window.confirm(t('settingPanel.confirmDeleteLore', { name: draft.name }))) return
    setSaving(true)
    try {
      if (loreAutoSaveTimer.current) {
        window.clearTimeout(loreAutoSaveTimer.current)
        loreAutoSaveTimer.current = null
      }
      await deleteLoreItem(draft.id)
      await refreshItems()
      notifyLoreUpdated([draft.id])
    } finally {
      setSaving(false)
    }
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      if (activeMode === 'creator') {
        await saveFile(CREATOR_PATH, creatorContent)
        return
      }
      if (activeMode === 'teller') {
        if (tellerAutoSaveTimer.current) {
          window.clearTimeout(tellerAutoSaveTimer.current)
          tellerAutoSaveTimer.current = null
        }
        await saveTellerDraft('manual')
        return
      }
      if (loreAutoSaveTimer.current) {
        window.clearTimeout(loreAutoSaveTimer.current)
        loreAutoSaveTimer.current = null
      }
      const item = await saveLoreDraft('manual')
      if (item) {
        notifyLoreUpdated([item.id])
      }
    } finally {
      setSaving(false)
    }
  }

  useEffect(() => {
    if (activeMode !== 'lore' || !draft || activeId === LORE_AGENT_ENTRY_ID) return
    const signature = loreDraftSignature(draft, tagDraft)
    if (signature === loreSavedSignature.current) return
    if (loreAutoSaveTimer.current) {
      window.clearTimeout(loreAutoSaveTimer.current)
    }
    loreAutoSaveTimer.current = window.setTimeout(() => {
      loreAutoSaveTimer.current = null
      void saveLoreDraft('auto').catch((err) => {
        console.warn('[lore-editor] 自动保存资料库条目失败', err)
      })
    }, 1200)
    return () => {
      if (loreAutoSaveTimer.current) {
        window.clearTimeout(loreAutoSaveTimer.current)
        loreAutoSaveTimer.current = null
      }
    }
  }, [activeMode, activeId, draft, tagDraft])

  useEffect(() => {
    if (activeMode !== 'teller' || !tellerDraft || activeTellerId === TELLER_AGENT_ENTRY_ID) return
    const signature = tellerDraftSignature(tellerDraft, tellerTagDraft)
    if (signature === tellerSavedSignature.current) return
    if (tellerAutoSaveTimer.current) {
      window.clearTimeout(tellerAutoSaveTimer.current)
    }
    tellerAutoSaveTimer.current = window.setTimeout(() => {
      tellerAutoSaveTimer.current = null
      void saveTellerDraft('auto').catch((err) => {
        console.warn('[teller-editor] 自动保存叙事方案失败', err)
      })
    }, 1200)
    return () => {
      if (tellerAutoSaveTimer.current) {
        window.clearTimeout(tellerAutoSaveTimer.current)
        tellerAutoSaveTimer.current = null
      }
    }
  }, [activeMode, activeTellerId, tellerDraft, tellerTagDraft])

  const handleCreateLoreVersion = async () => {
    setSaving(true)
    try {
      await createLoreVersion(t('settingPanel.manualLoreVersion'))
      await refreshVersions()
      setVersionsVisible(true)
    } finally {
      setSaving(false)
    }
  }

  const handleRestoreLoreVersion = async (version: LoreVersion) => {
    if (
      !window.confirm(
        t('settingPanel.confirmRestoreLoreVersion', {
          name: version.message || version.id,
        }),
      )
    )
      return
    setSaving(true)
    try {
      const restored = await restoreLoreVersion(version.id)
      setItems(restored)
      setActiveId(LORE_AGENT_ENTRY_ID)
      await refreshVersions()
      notifyLoreUpdated(restored.map((item) => item.id))
    } finally {
      setSaving(false)
    }
  }

  const handleLoreAgentResult = async (result: LoreAgentResult) => {
    setItems(result.items || [])
    await refreshVersions()
    notifyLoreUpdated(result.items?.map((item) => item.id) || [])
  }

  const handleTellerAgentResult = (result: TellerAgentResult) => {
    setTellers(result.tellers || [])
    onTellersChange?.(result.tellers || [])
    setActiveTellerId(result.teller.id)
    setTellerAgentTargetId(result.teller.id)
  }

  const handleSelectTeller = (id: string) => {
    setActiveTellerId(id)
    if (id !== TELLER_AGENT_ENTRY_ID) {
      setTellerAgentTargetId(id)
    }
  }

  const handleSelectLore = (id: string) => {
    if (loreAutoSaveTimer.current) {
      window.clearTimeout(loreAutoSaveTimer.current)
      loreAutoSaveTimer.current = null
      void saveLoreDraft('auto').catch((err) => {
        console.warn('[lore-editor] 切换条目前自动保存资料库条目失败', err)
      })
    }
    setActiveId(id)
  }

  const isLoreAgentActive = activeMode === 'lore' && activeId === LORE_AGENT_ENTRY_ID
  const isTellerAgentActive = activeMode === 'teller' && activeTellerId === TELLER_AGENT_ENTRY_ID
  return (
    <section className="flex h-full min-h-0 bg-[var(--nova-surface-2)] text-[var(--nova-text)]">
      <aside className={`nova-sidebar flex shrink-0 flex-col border-r ${embedded ? 'w-56' : 'w-[320px]'}`}>
        <div className="border-b border-[var(--nova-border)] px-3 py-3">
          <div className="flex items-center gap-2">
            <ModeIcon mode={activeMode} />
            <div className="text-sm font-semibold text-[var(--nova-text)]">{panelTitle(activeMode, t)}</div>
          </div>
          <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.directoryHint')}</div>
        </div>

        {activeMode === 'lore' ? <LoreDirectory items={items} activeId={activeId} query={query} saving={saving} onQueryChange={setQuery} onSelect={handleSelectLore} onCreate={(section) => void handleCreateLore(section)} /> : activeMode === 'creator' ? <CreatorDirectory /> : <TellerDirectory tellers={tellers} activeTellerId={activeTellerId} saving={saving} onSelect={handleSelectTeller} onCreate={() => void handleCreateTeller()} />}
      </aside>

      <main className="flex min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
        <div className="nova-topbar flex min-h-12 shrink-0 items-center justify-between gap-3 border-b px-4">
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <ModeIcon mode={activeMode} />
              <h2 className="truncate text-sm font-semibold text-[var(--nova-text)]">{isLoreAgentActive ? t('settingPanel.loreAgent.title') : isTellerAgentActive ? t('settingPanel.tellerAgent.title') : editorTitle(activeMode, draft, tellerDraft, t)}</h2>
            </div>
            <p className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">{isLoreAgentActive ? t('settingPanel.loreAgent.subtitle') : isTellerAgentActive ? t('settingPanel.tellerAgent.subtitle') : editorSubtitle(activeMode, draft, tellerDraft, t)}</p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {activeMode === 'lore' && !isLoreAgentActive && (
              <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || !draft} onClick={handleDelete} aria-label={t('settingPanel.deleteLore')}>
                <Trash2 className="h-4 w-4" />
              </Button>
            )}
            {activeMode === 'teller' && !isTellerAgentActive && (
              <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || !tellerDraft?.custom} onClick={handleDelete} aria-label={t('settingPanel.deleteTeller')}>
                <Trash2 className="h-4 w-4" />
              </Button>
            )}
            {!isLoreAgentActive && !isTellerAgentActive && (
              <Button className={actionButtonClassName} variant="outline" size="sm" disabled={saving || (activeMode === 'lore' && !draft) || (activeMode === 'teller' && !tellerDraft)} onClick={handleSave}>
                <Save className="h-4 w-4" />
                {saving ? t('common.saving') : t('common.save')}
              </Button>
            )}
          </div>
        </div>

        {activeMode === 'lore' ? (
          <>
            {activeId === LORE_AGENT_ENTRY_ID ? (
              <LoreAgentChat
                workspace={workspace}
                items={items}
                versions={versions}
                versionsVisible={versionsVisible}
                saving={saving}
                onResult={(result) => void handleLoreAgentResult(result)}
                onToolMutation={(itemIds) => {
                  void refreshItems(itemIds[0])
                  void refreshVersions()
                  notifyLoreUpdated(itemIds)
                }}
                onToggleVersions={() => setVersionsVisible((value) => !value)}
                onCreateVersion={() => void handleCreateLoreVersion()}
                onRestoreVersion={(version) => void handleRestoreLoreVersion(version)}
              />
            ) : (
              <LoreEditor draft={draft} tagDraft={tagDraft} residentTotalChars={items.filter((item) => item.load_mode === 'resident' && item.id !== draft?.id).reduce((total, item) => total + (item.content || '').length, draft?.load_mode === 'resident' ? (draft.content || '').length : 0)} setDraft={setDraft} setTagDraft={setTagDraft} onSave={handleSave} />
            )}
          </>
        ) : activeMode === 'creator' ? (
          <CreatorEditor content={creatorContent} setContent={setCreatorContent} onSave={handleSave} />
        ) : isTellerAgentActive ? (
          <TellerAgentChat workspace={workspace} tellers={tellers} targetTellerId={tellerAgentTargetId} onTargetTellerIdChange={setTellerAgentTargetId} onResult={handleTellerAgentResult} />
        ) : (
          <TellerEditor workspace={workspace} draft={tellerDraft} setDraft={setTellerDraft} tagDraft={tellerTagDraft} setTagDraft={setTellerTagDraft} activeSlotId={activeSlotId} setActiveSlotId={setActiveSlotId} onSave={handleSave} />
        )}
      </main>
    </section>
  )
}

const actionButtonClassName = 'nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

function splitTags(value: string) {
  return value
    .split(/[，,]/)
    .map((tag) => tag.trim())
    .filter(Boolean)
}

function loreDraftSignature(item: Partial<LoreItem>, tagDraft: string) {
  return JSON.stringify({
    ...item,
    tags: splitTags(tagDraft),
  })
}

function tellerDraftSignature(teller: Partial<Teller>, tagDraft: string) {
  return JSON.stringify({
    ...teller,
    tags: splitTags(tagDraft),
  })
}

function notifyLoreUpdated(itemIds: string[] = []) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent('nova:lore-updated', { detail: { item_ids: itemIds } }))
}

function ModeIcon({ mode }: { mode: SettingPanelMode }) {
  if (mode === 'creator') return <BookMarked className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
  if (mode === 'teller') return <SlidersHorizontal className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
  return <Database className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
}

function loreTypeLabel(type: LoreItem['type'], t: (key: string) => string) {
  const key = `lore.type.${type}`
  const label = t(key)
  return label === key ? t('lore.type.other') : label
}

function loreImportanceLabel(importance: LoreItem['importance'], t: (key: string) => string) {
  const key = `lore.importance.${importance}`
  const label = t(key)
  return label === key ? t('lore.importance.important') : label
}

function loreLoadModeLabel(loadMode: LoreItem['load_mode'] | undefined, t: (key: string) => string) {
  const key = `lore.loadMode.${loadMode || 'auto'}`
  const label = t(key)
  return label === key ? t('lore.loadMode.auto') : label
}

function panelTitle(mode: SettingPanelMode, t: (key: string) => string) {
  if (mode === 'creator') return t('settingPanel.mode.creator')
  if (mode === 'teller') return t('settingPanel.mode.teller')
  return t('settingPanel.mode.lore')
}

function editorTitle(mode: SettingPanelMode, draft: LoreItem | null, tellerDraft: Teller | null, t: (key: string) => string) {
  if (mode === 'creator') return CREATOR_PATH
  if (mode === 'teller') return tellerDraft?.name || t('settingPanel.editor.defaultTeller')
  return draft?.name || t('settingPanel.mode.lore')
}

function editorSubtitle(mode: SettingPanelMode, draft: LoreItem | null, tellerDraft: Teller | null, t: (key: string) => string) {
  if (mode === 'creator') return t('settingPanel.editor.creatorSubtitle')
  if (mode === 'teller') return tellerDraft?.description || t('settingPanel.editor.tellerSubtitle')
  if (!draft) return t('settingPanel.editor.loreSubtitle')
  return `${loreTypeLabel(draft.type, t)} · ${loreImportanceLabel(draft.importance, t)} · ${loreLoadModeLabel(draft.load_mode, t)} · ${(draft.tags || []).join('，') || t('settingPanel.editor.noTags')}`
}

function newTellerDraft(): Partial<Teller> {
  const id = `custom-${Date.now()}`
  return {
    id,
    name: '自定义叙事',
    description: '新的叙事编排方案',
    random_event_rate: 0.15,
    style_rules: [],
    tags: ['自定义'],
    context_policy: {
      creator: 'always',
      lore: 'relevant',
      runtime_state: 'always',
      recent_turns: 8,
    },
    slots: [
      {
        id: 'identity',
        name: '系统提示',
        target: 'system',
        enabled: true,
        content: '你是一套自定义叙事编排。你要明确影响故事的题材倾向、角色反应、剧情裁定、节奏推进和长期叙事原则。',
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
        name: '状态记忆',
        target: 'state_memory',
        enabled: true,
        content: '记录本回合已经成立的关系变化、风险、线索、资源、暗线和可继续行动的入口。',
      },
    ],
  }
}
