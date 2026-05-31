import { useEffect, useRef, useState, type ChangeEvent, type KeyboardEvent, type ReactNode } from 'react'
import { AtSign, BookMarked, Bot, Building2, Check, ChevronDown, ChevronUp, Database, FileText, Folder, History, Library, Loader2, MapPin, Plus, RotateCcw, Save, ScrollText, Search, Send, SlidersHorizontal, Trash2, UserRound, X } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import {
  createLoreVersion,
  createLoreItem,
  clearLoreAgentSession,
  deleteLoreItem,
  getLoreAgentMessages,
  getLoreItems,
  getLoreVersions,
  getStyles,
  readFile,
  restoreLoreVersion,
  runLoreAgentStream,
  saveFile,
  updateLoreItem,
  type ChatMessage,
  type LoreAgentResult,
  type LoreItem,
  type LoreVersion,
  type SSEEvent,
} from '@/lib/api'
import { isSaveShortcut } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { clearInteractiveTellerAgentSession, createInteractiveTeller, deleteInteractiveTeller, getInteractiveTellerAgentMessages, getInteractiveTellers, runInteractiveTellerAgentStream, updateInteractiveTeller } from '../api'
import type { StyleRule, Teller, TellerAgentResult, TellerPromptSlot } from '../types'

const CREATOR_PATH = 'CREATOR.md'
const LORE_AGENT_ENTRY_ID = '__lore_agent__'
const TELLER_AGENT_ENTRY_ID = '__teller_agent__'
const EMPTY_TELLERS: Teller[] = []

const TYPE_OPTIONS = [
  { value: 'character', label: '角色' },
  { value: 'world', label: '世界观' },
  { value: 'location', label: '地点' },
  { value: 'faction', label: '势力' },
  { value: 'rule', label: '规则' },
  { value: 'item', label: '物品' },
  { value: 'other', label: '其他' },
] as const

const IMPORTANCE_OPTIONS = [
  { value: 'major', label: '主要' },
  { value: 'important', label: '重要' },
  { value: 'minor', label: '次要' },
] as const

const TELLER_TARGET_OPTIONS = [
  {
    value: 'system',
    label: '系统提示',
    summary: 'Agent 初始化时注入',
    detail: '和 CREATOR.md 同处系统提示层，定义讲述者身份、题材倾向和长期叙事原则。',
  },
  {
    value: 'turn_context',
    label: '本轮上下文',
    summary: '每轮贴近用户行动',
    detail: '每次生成下一回合时注入，强约束本轮裁定、NPC 主动反应、代价、暗线推进和可选择。',
  },
  {
    value: 'state_memory',
    label: '状态记忆',
    summary: '只影响状态记录',
    detail: '正文生成后注入状态 Agent，用于稳定记录危机、关系变化、资源压力、暗线和行动入口。',
  },
] as const

type TellerTarget = TellerPromptSlot['target']
export type SettingPanelMode = 'lore' | 'creator' | 'teller'

type LoreType = LoreItem['type']

interface KnowledgeSection {
  id: string
  label: string
  icon: LucideIcon
  types: LoreType[]
  createType: LoreType
  createName: string
  tag?: string
  excludeTag?: string
}

const KNOWLEDGE_SECTIONS: KnowledgeSection[] = [
  { id: 'characters', label: '角色', icon: UserRound, types: ['character'], createType: 'character', createName: '新角色' },
  { id: 'locations', label: '地点', icon: MapPin, types: ['location'], createType: 'location', createName: '新地点' },
  { id: 'factions', label: '组织', icon: Building2, types: ['faction'], createType: 'faction', createName: '新组织' },
  { id: 'rules', label: '规则', icon: ScrollText, types: ['world', 'rule'], createType: 'rule', createName: '新规则' },
  { id: 'templates', label: '模板', icon: FileText, types: ['other'], createType: 'other', createName: '新模板', tag: '模板' },
  { id: 'assets', label: '素材库', icon: Library, types: ['item', 'other'], createType: 'item', createName: '新素材', excludeTag: '模板' },
]

interface SettingPanelProps {
  mode?: SettingPanelMode
  workspace?: string
  tellers?: Teller[]
  onTellersChange?: (tellers: Teller[]) => void
  embedded?: boolean
}

export function SettingPanel({ mode, workspace = '', tellers: externalTellers = EMPTY_TELLERS, onTellersChange, embedded = false }: SettingPanelProps) {
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
    if (!workspace) return () => { cancelled = true }
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
    return () => { cancelled = true }
  }, [workspace])

  useEffect(() => {
    if (activeMode !== 'lore') return
    let cancelled = false
    if (!workspace) {
      setVersions([])
      return () => { cancelled = true }
    }
    getLoreVersions()
      .then((data) => {
        if (!cancelled) setVersions(data)
      })
      .catch(() => {
        if (!cancelled) setVersions([])
      })
    return () => { cancelled = true }
  }, [activeMode, workspace])

  useEffect(() => {
    const item = items.find((entry) => entry.id === activeId) || null
    const nextDraft = item ? { ...item, tags: [...(item.tags || [])] } : null
    const nextTagDraft = (item?.tags || []).join('，')
    const currentDraft = loreDraftRef.current
    const currentTagDraft = loreTagDraftRef.current
    const hasUnsavedCurrentDraft = Boolean(
      currentDraft?.id &&
      currentDraft.id === item?.id &&
      loreDraftSignature(currentDraft, currentTagDraft) !== loreSavedSignature.current,
    )
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
    if (!workspace) return () => { cancelled = true }
    readFile(CREATOR_PATH)
      .then((data) => { if (!cancelled) setCreatorContent(data.content) })
      .catch(() => { if (!cancelled) setCreatorContent('') })
    return () => { cancelled = true }
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
    return () => { cancelled = true }
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
    const nextDraft = teller ? {
      ...teller,
      tags: [...(teller.tags || [])],
      slots: [...(teller.slots || [])],
      context_policy: { ...teller.context_policy },
      style_rules: [...(teller.style_rules || [])],
    } : null
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
    setTellerAgentTargetId((current) => data.some((teller) => teller.id === current) ? current : (nextActiveId || data[0]?.id || ''))
  }

  const mergeSavedTeller = (teller: Teller) => {
    setTellers((current) => current.map((entry) => entry.id === teller.id ? teller : entry))
    onTellersChange?.(tellers.map((entry) => entry.id === teller.id ? teller : entry))
    setActiveTellerId(teller.id)
    setTellerAgentTargetId((current) => current || teller.id)
  }

  const mergeSavedLoreItem = (item: LoreItem) => {
    setItems((current) => current.map((entry) => entry.id === item.id ? item : entry))
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
        tags: section.tag ? [section.tag] : [],
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
      if (!window.confirm(`删除讲述者「${tellerDraft.name}」？`)) return
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
    if (!window.confirm(`删除资料「${draft.name}」？`)) return
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
        console.warn('[teller-editor] 自动保存讲述者失败', err)
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
      await createLoreVersion('手动创建资料库版本')
      await refreshVersions()
      setVersionsVisible(true)
    } finally {
      setSaving(false)
    }
  }

  const handleRestoreLoreVersion = async (version: LoreVersion) => {
    if (!window.confirm(`恢复资料库版本「${version.message || version.id}」？`)) return
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
            <div className="text-sm font-semibold text-[var(--nova-text)]">{panelTitle(activeMode)}</div>
          </div>
          <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">在目录中选择条目，右侧打开编辑。</div>
        </div>

        {activeMode === 'lore' ? (
          <LoreDirectory
            items={items}
            activeId={activeId}
            query={query}
            saving={saving}
            onQueryChange={setQuery}
            onSelect={handleSelectLore}
            onCreate={(section) => void handleCreateLore(section)}
          />
        ) : activeMode === 'creator' ? (
          <CreatorDirectory />
        ) : (
          <TellerDirectory
            tellers={tellers}
            activeTellerId={activeTellerId}
            saving={saving}
            onSelect={handleSelectTeller}
            onCreate={() => void handleCreateTeller()}
          />
        )}
      </aside>

      <main className="flex min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
        <div className="nova-topbar flex min-h-12 shrink-0 items-center justify-between gap-3 border-b px-4">
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <ModeIcon mode={activeMode} />
              <h2 className="truncate text-sm font-semibold text-[var(--nova-text)]">{isLoreAgentActive ? '资料库 Agent' : isTellerAgentActive ? '讲述者 Agent' : editorTitle(activeMode, draft, tellerDraft)}</h2>
            </div>
            <p className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">{isLoreAgentActive ? '用自然语言批量整理、补充和修改资料库' : isTellerAgentActive ? '用自然语言创建或修改单个讲述者规则包' : editorSubtitle(activeMode, draft, tellerDraft)}</p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {activeMode === 'lore' && !isLoreAgentActive && (
              <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || !draft} onClick={handleDelete} aria-label="删除资料">
                <Trash2 className="h-4 w-4" />
              </Button>
            )}
            {activeMode === 'teller' && !isTellerAgentActive && (
              <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || !tellerDraft?.custom} onClick={handleDelete} aria-label="删除讲述者">
                <Trash2 className="h-4 w-4" />
              </Button>
            )}
            {!isLoreAgentActive && !isTellerAgentActive && (
              <Button className={actionButtonClassName} variant="outline" size="sm" disabled={saving || (activeMode === 'lore' && !draft) || (activeMode === 'teller' && !tellerDraft)} onClick={handleSave}>
                <Save className="h-4 w-4" />
                {saving ? '保存中...' : '保存'}
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
                onToggleVersions={() => setVersionsVisible((value) => !value)}
                onCreateVersion={() => void handleCreateLoreVersion()}
                onRestoreVersion={(version) => void handleRestoreLoreVersion(version)}
              />
            ) : (
              <LoreEditor draft={draft} tagDraft={tagDraft} setDraft={setDraft} setTagDraft={setTagDraft} onSave={handleSave} />
            )}
          </>
        ) : activeMode === 'creator' ? (
          <CreatorEditor content={creatorContent} setContent={setCreatorContent} onSave={handleSave} />
        ) : isTellerAgentActive ? (
          <TellerAgentChat
            workspace={workspace}
            tellers={tellers}
            targetTellerId={tellerAgentTargetId}
            onTargetTellerIdChange={setTellerAgentTargetId}
            onResult={handleTellerAgentResult}
          />
        ) : (
          <TellerEditor
            workspace={workspace}
            draft={tellerDraft}
            setDraft={setTellerDraft}
            tagDraft={tellerTagDraft}
            setTagDraft={setTellerTagDraft}
            activeSlotId={activeSlotId}
            setActiveSlotId={setActiveSlotId}
            onSave={handleSave}
          />
        )}
      </main>
    </section>
  )
}

type LoreAgentChatMessage = {
  id: string
  role: 'user' | 'assistant' | 'thinking' | 'tool_call' | 'error' | 'clear'
  content: string
  name?: string
  args?: string
  status?: 'running' | 'success' | 'error'
  toolResult?: string
  references?: LoreItem[]
  result?: LoreAgentResult
}

type TellerAgentChatMessage = {
  id: string
  role: 'user' | 'assistant' | 'thinking' | 'tool_call' | 'error' | 'clear'
  content: string
  name?: string
  args?: string
  status?: 'running' | 'success' | 'error'
  toolResult?: string
  targetTeller?: Teller
  tellerReferences?: Teller[]
  result?: TellerAgentResult
}

interface LoreStatusPayload {
  stage?: string
  message?: string
  ops?: number
}

interface LoreToolPayload {
  id?: string
  name?: string
  args?: string
  delta?: string
  content?: string
}

function LoreAgentChat({
  workspace,
  items,
  versions,
  versionsVisible,
  saving,
  onResult,
  onToggleVersions,
  onCreateVersion,
  onRestoreVersion,
}: {
  workspace: string
  items: LoreItem[]
  versions: LoreVersion[]
  versionsVisible: boolean
  saving: boolean
  onResult: (result: LoreAgentResult) => void
  onToggleVersions: () => void
  onCreateVersion: () => void
  onRestoreVersion: (version: LoreVersion) => void
}) {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const messageEndRef = useRef<HTMLDivElement>(null)
  const historyWorkspaceRef = useRef<string | null>(null)
  const workspaceRef = useRef(workspace)
  const [value, setValue] = useState('')
  const [referenceIds, setReferenceIds] = useState<string[]>([])
  const [referenceQuery, setReferenceQuery] = useState<string | null>(null)
  const [messages, setMessages] = useState<LoreAgentChatMessage[]>([])
  const [running, setRunning] = useState(false)
  const referencedItems = referenceIds
    .map((id) => items.find((item) => item.id === id))
    .filter((item): item is LoreItem => Boolean(item))
  const normalizedQuery = (referenceQuery || '').trim().toLowerCase()
  const visibleItems = items
    .filter((item) => {
      if (referenceIds.includes(item.id)) return false
      if (!normalizedQuery) return true
      const haystack = `${item.name}\n${item.id}\n${item.content || ''}\n${(item.tags || []).join('\n')}`.toLowerCase()
      return haystack.includes(normalizedQuery)
    })
    .slice(0, 30)

  useEffect(() => {
    setReferenceIds((current) => current.filter((id) => items.some((item) => item.id === id)))
  }, [items])

  useEffect(() => {
    messageEndRef.current?.scrollIntoView({ block: 'end' })
  }, [messages, running])

  useEffect(() => {
    workspaceRef.current = workspace
    if (historyWorkspaceRef.current === workspace) return
    historyWorkspaceRef.current = workspace || null
    setValue('')
    setReferenceIds([])
    setReferenceQuery(null)
    setMessages([])
    setRunning(false)
    if (!workspace) return
    let cancelled = false
    getLoreAgentMessages()
      .then((history) => {
        if (cancelled) return
        setMessages(history.map((message, index) => loreHistoryMessageToChat(message, index, items)))
      })
      .catch((error) => {
        if (!cancelled) {
          setMessages([{ id: 'load-error', role: 'error', content: error instanceof Error ? error.message : '资料库 Agent 历史加载失败' }])
        }
      })
    return () => { cancelled = true }
  }, [workspace])

  const handleChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    const nextValue = event.target.value
    setValue(nextValue)
    const atMatch = nextValue.match(/(?:^|\s)@([^\s@]*)$/)
    setReferenceQuery(atMatch ? atMatch[1] : null)
  }

  const selectReference = (item: LoreItem) => {
    const nextValue = value.replace(/(?:^|\s)@([^\s@]*)$/, (match) => {
      const prefix = match.startsWith(' ') ? ' ' : ''
      return `${prefix}@${item.name} `
    })
    setValue(nextValue === value ? `${value.trimEnd()} @${item.name} ` : nextValue)
    setReferenceIds((current) => current.includes(item.id) ? current : [...current, item.id])
    setReferenceQuery(null)
    textareaRef.current?.focus()
  }

  const removeReference = (id: string) => {
    setReferenceIds((current) => current.filter((entry) => entry !== id))
  }

  const appendMessage = (message: Omit<LoreAgentChatMessage, 'id'>) => {
    setMessages((current) => [...current, { ...message, id: `${Date.now()}-${current.length}` }])
  }

  const appendStreamingMessage = (role: 'assistant' | 'thinking', content: string) => {
    if (!content) return
    setMessages((current) => {
      const last = current[current.length - 1]
      if (last?.role === role && !last.result) {
        return [...current.slice(0, -1), { ...last, content: `${last.content}${content}` }]
      }
      return [...current, { id: `${Date.now()}-${current.length}`, role, content }]
    })
  }

  const upsertToolCall = (payload: LoreToolPayload) => {
    const id = payload.id || `tool-${Date.now()}`
    const name = payload.name || '资料库工具'
    setMessages((current) => {
      const existing = current.findIndex((message) => message.id === id)
      const nextMessage: LoreAgentChatMessage = {
        id,
        role: 'tool_call',
        content: name,
        name,
        args: payload.args || '',
        status: 'running',
      }
      if (existing >= 0) {
        return current.map((message, index) => index === existing ? { ...message, ...nextMessage, args: message.args || nextMessage.args } : message)
      }
      return [...current, nextMessage]
    })
  }

  const appendToolArgs = (payload: LoreToolPayload) => {
    if (!payload.id || !payload.delta) return
    setMessages((current) => current.map((message) => (
      message.id === payload.id && message.role === 'tool_call'
        ? { ...message, args: `${message.args || ''}${payload.delta}` }
        : message
    )))
  }

  const finishToolCall = (payload: LoreToolPayload) => {
    const id = payload.id
    if (!id) return
    setMessages((current) => current.map((message) => (
      message.id === id && message.role === 'tool_call'
        ? { ...message, status: 'success', toolResult: payload.content || '' }
        : message
    )))
  }

  const send = async () => {
    const instruction = value.trim()
    if (!instruction || running) return
    const activeWorkspace = workspace
    if (instruction === '/clear') {
      setRunning(true)
      try {
        await clearLoreAgentSession()
        if (workspaceRef.current !== activeWorkspace) return
        appendMessage({ role: 'clear', content: '已清理资料库 Agent 上下文，历史消息仍保留。' })
        setValue('')
        setReferenceIds([])
        setReferenceQuery(null)
      } catch (error) {
        if (workspaceRef.current !== activeWorkspace) return
        appendMessage({ role: 'error', content: error instanceof Error ? error.message : '资料库 Agent 上下文清理失败' })
      } finally {
        if (workspaceRef.current === activeWorkspace) setRunning(false)
      }
      return
    }
    const refs = [...referenceIds]
    const userReferences = refs
      .map((id) => items.find((item) => item.id === id))
      .filter((item): item is LoreItem => Boolean(item))
    appendMessage({ role: 'user', content: instruction, references: userReferences })
    setValue('')
    setReferenceIds([])
    setReferenceQuery(null)
    setRunning(true)
    try {
      const stream = await runLoreAgentStream(instruction, refs)
      const reader = stream.getReader()
      while (true) {
        const { done, value: event } = await reader.read()
        if (done) break
        if (workspaceRef.current !== activeWorkspace) break
        handleLoreAgentEvent(event)
      }
    } catch (error) {
      if (workspaceRef.current !== activeWorkspace) return
      appendMessage({ role: 'error', content: error instanceof Error ? error.message : '资料库 Agent 执行失败' })
    } finally {
      if (workspaceRef.current === activeWorkspace) {
        setRunning(false)
        textareaRef.current?.focus()
      }
    }
  }

  const handleLoreAgentEvent = (event: SSEEvent) => {
    if (event.event === 'thinking') {
      const payload = parseLoreEventData<{ content?: string }>(event.data)
      appendStreamingMessage('thinking', payload?.content || '')
      return
    }
    if (event.event === 'chunk') {
      const payload = parseLoreEventData<{ content?: string }>(event.data)
      appendStreamingMessage('assistant', payload?.content || '')
      return
    }
    if (event.event === 'tool_call') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) upsertToolCall(payload)
      return
    }
    if (event.event === 'tool_args_delta') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) appendToolArgs(payload)
      return
    }
    if (event.event === 'tool_result') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) finishToolCall(payload)
      return
    }
    if (event.event === 'lore_status') {
      const payload = parseLoreEventData<LoreStatusPayload>(event.data)
      const content = payload?.message || '资料库 Agent 正在处理...'
      appendMessage({ role: 'assistant', content: payload?.ops ? `${content}（${payload.ops} 个操作）` : content })
      return
    }
    if (event.event === 'lore_result') {
      const result = parseLoreEventData<LoreAgentResult>(event.data)
      if (!result) {
        appendMessage({ role: 'error', content: '资料库 Agent 返回结果无法解析' })
        return
      }
      onResult(result)
      appendMessage({ role: 'assistant', content: loreAgentResultSummary(result), result })
      return
    }
    if (event.event === 'error') {
      const payload = parseLoreEventData<{ message?: string }>(event.data)
      appendMessage({ role: 'error', content: payload?.message || '资料库 Agent 执行失败' })
    }
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      void send()
      return
    }
    if (event.key === 'Escape') {
      setReferenceQuery(null)
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
      <div className="flex h-10 shrink-0 items-center justify-between border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4">
        <div className="text-xs text-[var(--nova-text-faint)]">对话已保存到当前 workspace，输入 /clear 可清理后续上下文。</div>
        <Button className={actionButtonClassName} variant="outline" size="sm" onClick={onToggleVersions}>
          <History className="h-4 w-4" />
          版本
        </Button>
      </div>

      {versionsVisible && (
        <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3">
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
            <div className="flex h-9 items-center justify-between border-b border-[var(--nova-border)] px-3">
              <span className="text-xs font-medium text-[var(--nova-text-muted)]">资料库版本</span>
              <Button className={actionButtonClassName} variant="outline" size="sm" disabled={saving} onClick={onCreateVersion}>
                <Plus className="h-3.5 w-3.5" />
              </Button>
            </div>
            <div className="max-h-36 overflow-auto p-2">
              {versions.length ? versions.map((version) => (
                <div key={version.id} className="flex items-center gap-2 rounded px-2 py-1.5 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]">
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-[var(--nova-text)]">{version.message || version.id}</div>
                    <div className="truncate text-[11px] text-[var(--nova-text-faint)]">{formatDateTime(version.created_at)} · {version.item_count} 条</div>
                  </div>
                  <button
                    type="button"
                    className="nova-nav-item rounded p-1 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    onClick={() => onRestoreVersion(version)}
                    aria-label="恢复资料库版本"
                  >
                    <RotateCcw className="h-3.5 w-3.5" />
                  </button>
                </div>
              )) : (
                <div className="px-2 py-3 text-xs text-[var(--nova-text-faint)]">暂无版本</div>
              )}
            </div>
          </div>
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-auto px-5 py-4">
        {messages.length === 0 ? (
          <div className="flex h-full items-center justify-center">
            <div className="max-w-md rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 text-center">
              <div className="text-sm font-medium text-[var(--nova-text)]">和资料库 Agent 对话</div>
              <div className="mt-1 text-xs leading-5 text-[var(--nova-text-faint)]">直接描述要整理、补充或修改的设定；需要限定对象时，在输入框里用 @ 引用资料条目。</div>
            </div>
          </div>
        ) : (
          <div className="mx-auto flex max-w-4xl flex-col gap-3">
            {messages.map((message) => (
              <LoreAgentMessage key={message.id} message={message} />
            ))}
            {running && (
              <div className="flex items-center gap-2 self-start rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-xs text-[var(--nova-text-muted)]">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                Agent 正在处理...
              </div>
            )}
            <div ref={messageEndRef} />
          </div>
        )}
      </div>

      <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
        <div className="mx-auto max-w-4xl">
          <div className="nova-field flex min-w-0 items-end gap-2 rounded-[var(--nova-radius)] px-3 py-2">
            <Bot className="mb-2 h-4 w-4 shrink-0 text-[var(--nova-text-faint)]" />
            <div className="relative min-w-0 flex-1">
              <Popover open={referenceQuery !== null && visibleItems.length > 0}>
                <PopoverTrigger asChild>
                  <span className="absolute bottom-full left-0 h-0 w-0" />
                </PopoverTrigger>
                <PopoverContent
                  align="start"
                  side="top"
                  className="mb-2 w-[360px] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)]"
                  onOpenAutoFocus={(event) => event.preventDefault()}
                >
                  <Command shouldFilter={false} className="bg-transparent">
                    <CommandInput value={referenceQuery || ''} readOnly placeholder="搜索资料条目..." />
                    <CommandList>
                      <CommandEmpty>未找到资料</CommandEmpty>
                      <CommandGroup heading="引用资料">
                        {visibleItems.map((item) => (
                          <CommandItem
                            key={item.id}
                            value={item.id}
                            onSelect={() => selectReference(item)}
                            className="cursor-pointer"
                          >
                            <span className="min-w-0 flex-1 truncate">@{item.name}</span>
                            <span className="text-[11px] text-[var(--nova-text-faint)]">{loreTypeLabel(item.type)}</span>
                          </CommandItem>
                        ))}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
              <textarea
                ref={textareaRef}
                className="max-h-36 min-h-10 w-full resize-none bg-transparent text-sm leading-5 text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)] disabled:opacity-60"
                value={value}
                onChange={handleChange}
                onKeyDown={handleKeyDown}
                placeholder={running ? '资料库 Agent 正在执行...' : '输入资料库修改指令，Enter 发送，Shift+Enter 换行'}
                rows={2}
                disabled={running}
              />
            </div>
            {referencedItems.length > 0 && (
              <div className="flex max-w-[220px] flex-wrap justify-end gap-1.5">
                {referencedItems.map((item) => (
                  <span
                    key={item.id}
                    className="inline-flex max-w-full items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-xs text-[var(--nova-text-muted)]"
                  >
                    <AtSign className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" />
                    <span className="truncate">{item.name}</span>
                    <button
                      type="button"
                      className="rounded text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
                      onClick={() => removeReference(item.id)}
                      aria-label={`移除引用 ${item.name}`}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                ))}
              </div>
            )}
            <Button className={actionButtonClassName} variant="outline" size="sm" disabled={running || !value.trim()} onClick={() => void send()}>
              {running ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
              {running ? '执行中...' : '发送'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function TellerAgentChat({
  workspace,
  tellers,
  targetTellerId,
  onTargetTellerIdChange,
  onResult,
}: {
  workspace: string
  tellers: Teller[]
  targetTellerId: string
  onTargetTellerIdChange: (id: string) => void
  onResult: (result: TellerAgentResult) => void
}) {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const messageEndRef = useRef<HTMLDivElement>(null)
  const historyWorkspaceRef = useRef<string | null>(null)
  const workspaceRef = useRef(workspace)
  const [value, setValue] = useState('')
  const [referenceIds, setReferenceIds] = useState<string[]>([])
  const [referenceQuery, setReferenceQuery] = useState<string | null>(null)
  const [messages, setMessages] = useState<TellerAgentChatMessage[]>([])
  const [running, setRunning] = useState(false)
  const targetTeller = tellers.find((teller) => teller.id === targetTellerId) || null
  const normalizedQuery = (referenceQuery || '').trim().toLowerCase()
  const referencedTellers = referenceIds
    .map((id) => tellers.find((teller) => teller.id === id))
    .filter((teller): teller is Teller => Boolean(teller))
  const visibleTellers = tellers
    .filter((teller) => {
      if (referenceIds.includes(teller.id)) return false
      if (!normalizedQuery) return true
      const haystack = `${teller.name}\n${teller.id}\n${teller.description}\n${(teller.tags || []).join('\n')}`.toLowerCase()
      return haystack.includes(normalizedQuery)
    })
    .slice(0, 30)

  useEffect(() => {
    messageEndRef.current?.scrollIntoView({ block: 'end' })
  }, [messages, running])

  useEffect(() => {
    setReferenceIds((current) => current.filter((id) => tellers.some((teller) => teller.id === id)))
  }, [tellers])

  useEffect(() => {
    workspaceRef.current = workspace
    if (historyWorkspaceRef.current === workspace) return
    historyWorkspaceRef.current = workspace || null
    setValue('')
    setReferenceIds([])
    setReferenceQuery(null)
    setMessages([])
    setRunning(false)
    if (!workspace) return
    let cancelled = false
    getInteractiveTellerAgentMessages()
      .then((history) => {
        if (cancelled) return
        setMessages(history.map((message, index) => tellerHistoryMessageToChat(message, index, tellers)))
      })
      .catch((error) => {
        if (!cancelled) {
          setMessages([{ id: 'load-error', role: 'error', content: error instanceof Error ? error.message : '讲述者 Agent 历史加载失败' }])
        }
      })
    return () => { cancelled = true }
  }, [workspace])

  const handleChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    const nextValue = event.target.value
    setValue(nextValue)
    const atMatch = nextValue.match(/(?:^|\s)@([^\s@]*)$/)
    setReferenceQuery(atMatch ? atMatch[1] : null)
  }

  const selectReference = (teller: Teller) => {
    const nextValue = value.replace(/(?:^|\s)@([^\s@]*)$/, (match) => {
      const prefix = match.startsWith(' ') ? ' ' : ''
      return `${prefix}@${teller.name} `
    })
    setValue(nextValue === value ? `${value.trimEnd()} @${teller.name} ` : nextValue)
    setReferenceIds((current) => current.includes(teller.id) ? current : [...current, teller.id])
    setReferenceQuery(null)
    textareaRef.current?.focus()
  }

  const removeReference = (id: string) => {
    setReferenceIds((current) => current.filter((entry) => entry !== id))
  }

  const appendMessage = (message: Omit<TellerAgentChatMessage, 'id'>) => {
    setMessages((current) => [...current, { ...message, id: `${Date.now()}-${current.length}` }])
  }

  const appendStreamingMessage = (role: 'assistant' | 'thinking', content: string) => {
    if (!content) return
    setMessages((current) => {
      const last = current[current.length - 1]
      if (last?.role === role && !last.result) {
        return [...current.slice(0, -1), { ...last, content: `${last.content}${content}` }]
      }
      return [...current, { id: `${Date.now()}-${current.length}`, role, content }]
    })
  }

  const upsertToolCall = (payload: LoreToolPayload) => {
    const id = payload.id || `tool-${Date.now()}`
    const name = payload.name || '讲述者工具'
    setMessages((current) => {
      const existing = current.findIndex((message) => message.id === id)
      const nextMessage: TellerAgentChatMessage = {
        id,
        role: 'tool_call',
        content: name,
        name,
        args: payload.args || '',
        status: 'running',
      }
      if (existing >= 0) {
        return current.map((message, index) => index === existing ? { ...message, ...nextMessage, args: message.args || nextMessage.args } : message)
      }
      return [...current, nextMessage]
    })
  }

  const appendToolArgs = (payload: LoreToolPayload) => {
    if (!payload.id || !payload.delta) return
    setMessages((current) => current.map((message) => (
      message.id === payload.id && message.role === 'tool_call'
        ? { ...message, args: `${message.args || ''}${payload.delta}` }
        : message
    )))
  }

  const finishToolCall = (payload: LoreToolPayload) => {
    const id = payload.id
    if (!id) return
    setMessages((current) => current.map((message) => (
      message.id === id && message.role === 'tool_call'
        ? { ...message, status: 'success', toolResult: payload.content || '' }
        : message
    )))
  }

  const send = async () => {
    const instruction = value.trim()
    if (!instruction || running) return
    const activeWorkspace = workspace
    if (instruction === '/clear') {
      setRunning(true)
      try {
        await clearInteractiveTellerAgentSession()
        if (workspaceRef.current !== activeWorkspace) return
        appendMessage({ role: 'clear', content: '已清理讲述者 Agent 上下文，历史消息仍保留。' })
        setValue('')
        setReferenceIds([])
        setReferenceQuery(null)
      } catch (error) {
        if (workspaceRef.current !== activeWorkspace) return
        appendMessage({ role: 'error', content: error instanceof Error ? error.message : '讲述者 Agent 上下文清理失败' })
      } finally {
        if (workspaceRef.current === activeWorkspace) setRunning(false)
      }
      return
    }
    const refs = [...referenceIds]
    const userReferences = refs
      .map((id) => tellers.find((teller) => teller.id === id))
      .filter((teller): teller is Teller => Boolean(teller))
    appendMessage({ role: 'user', content: instruction, targetTeller: targetTeller || undefined, tellerReferences: userReferences })
    setValue('')
    setReferenceIds([])
    setReferenceQuery(null)
    setRunning(true)
    try {
      const stream = await runInteractiveTellerAgentStream(instruction, targetTeller?.id || '', refs)
      const reader = stream.getReader()
      while (true) {
        const { done, value: event } = await reader.read()
        if (done) break
        if (workspaceRef.current !== activeWorkspace) break
        handleTellerAgentEvent(event)
      }
    } catch (error) {
      if (workspaceRef.current !== activeWorkspace) return
      appendMessage({ role: 'error', content: error instanceof Error ? error.message : '讲述者 Agent 执行失败' })
    } finally {
      if (workspaceRef.current === activeWorkspace) {
        setRunning(false)
        textareaRef.current?.focus()
      }
    }
  }

  const handleTellerAgentEvent = (event: SSEEvent) => {
    if (event.event === 'thinking') {
      const payload = parseLoreEventData<{ content?: string }>(event.data)
      appendStreamingMessage('thinking', payload?.content || '')
      return
    }
    if (event.event === 'chunk') {
      const payload = parseLoreEventData<{ content?: string }>(event.data)
      appendStreamingMessage('assistant', payload?.content || '')
      return
    }
    if (event.event === 'tool_call') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) upsertToolCall(payload)
      return
    }
    if (event.event === 'tool_args_delta') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) appendToolArgs(payload)
      return
    }
    if (event.event === 'tool_result') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) finishToolCall(payload)
      return
    }
    if (event.event === 'teller_result') {
      const result = parseLoreEventData<TellerAgentResult>(event.data)
      if (!result) {
        appendMessage({ role: 'error', content: '讲述者 Agent 返回结果无法解析' })
        return
      }
      onResult(result)
      appendMessage({ role: 'assistant', content: tellerAgentResultSummary(result), result })
      return
    }
    if (event.event === 'error') {
      const payload = parseLoreEventData<{ message?: string }>(event.data)
      appendMessage({ role: 'error', content: payload?.message || '讲述者 Agent 执行失败' })
    }
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      void send()
      return
    }
    if (event.key === 'Escape') {
      setReferenceQuery(null)
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
      <div className="flex h-10 shrink-0 items-center justify-between border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4">
        <div className="text-xs text-[var(--nova-text-faint)]">输入 /clear 可清理后续上下文；Agent 会根据本轮意图新建或修改一个讲述者。</div>
      </div>

      <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3">
        <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_220px]">
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs text-[var(--nova-text-muted)]">
            {targetTeller ? `当前参考「${targetTeller.name}」；也可以在输入中 @ 其他讲述者或直接要求新建` : '未选择参考讲述者；Agent 会按指令自由判断'}
          </div>
          <Select value={targetTellerId || 'none'} onValueChange={(value) => onTargetTellerIdChange(value === 'none' ? '' : value)}>
            <SelectTrigger size="sm" className={selectClassName}>
              <SelectValue placeholder="选择参考讲述者" />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectItem value="none">不选择，按指令判断</SelectItem>
              {tellers.map((teller) => (
                <SelectItem key={teller.id} value={teller.id}>{teller.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-4">
        {messages.length === 0 ? (
          <EmptyState title="讲述者 Agent" description="描述要新建或修改的讲述者；需要限定对象时，在输入框里用 @ 引用讲述者。" />
        ) : (
          <div className="mx-auto flex max-w-4xl flex-col gap-3">
            {messages.map((message) => (
              <TellerAgentMessage key={message.id} message={message} />
            ))}
            {running && (
              <div className="flex items-center gap-2 self-start rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-xs text-[var(--nova-text-muted)]">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                Agent 正在处理...
              </div>
            )}
            <div ref={messageEndRef} />
          </div>
        )}
      </div>

      <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
        <div className="mx-auto max-w-4xl">
          <div className="nova-field flex min-w-0 items-end gap-2 rounded-[var(--nova-radius)] px-3 py-2">
            <Bot className="mb-2 h-4 w-4 shrink-0 text-[var(--nova-text-faint)]" />
            <div className="relative min-w-0 flex-1">
              <Popover open={referenceQuery !== null && visibleTellers.length > 0}>
                <PopoverTrigger asChild>
                  <span className="absolute bottom-full left-0 h-0 w-0" />
                </PopoverTrigger>
                <PopoverContent
                  align="start"
                  side="top"
                  className="mb-2 w-[360px] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)]"
                  onOpenAutoFocus={(event) => event.preventDefault()}
                >
                  <Command shouldFilter={false} className="bg-transparent">
                    <CommandInput value={referenceQuery || ''} readOnly placeholder="搜索讲述者..." />
                    <CommandList>
                      <CommandEmpty>未找到讲述者</CommandEmpty>
                      <CommandGroup heading="引用讲述者">
                        {visibleTellers.map((teller) => (
                          <CommandItem
                            key={teller.id}
                            value={teller.id}
                            onSelect={() => selectReference(teller)}
                            className="cursor-pointer"
                          >
                            <span className="min-w-0 flex-1 truncate">@{teller.name}</span>
                            <span className="text-[11px] text-[var(--nova-text-faint)]">{teller.id}</span>
                          </CommandItem>
                        ))}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
              <textarea
                ref={textareaRef}
                className="max-h-36 min-h-10 w-full resize-none bg-transparent text-sm leading-5 text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)] disabled:opacity-60"
                value={value}
                onChange={handleChange}
                onKeyDown={handleKeyDown}
                placeholder={running ? '讲述者 Agent 正在执行...' : '输入讲述者创建或修改指令，可用 @ 引用，Enter 发送'}
                rows={2}
                disabled={running}
              />
            </div>
            {referencedTellers.length > 0 && (
              <div className="flex max-w-[220px] flex-wrap justify-end gap-1.5">
                {referencedTellers.map((teller) => (
                  <span
                    key={teller.id}
                    className="inline-flex max-w-full items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-xs text-[var(--nova-text-muted)]"
                  >
                    <AtSign className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" />
                    <span className="truncate">{teller.name}</span>
                    <button
                      type="button"
                      className="rounded text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
                      onClick={() => removeReference(teller.id)}
                      aria-label={`移除引用 ${teller.name}`}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                ))}
              </div>
            )}
            <Button className={actionButtonClassName} variant="outline" size="sm" disabled={running || !value.trim()} onClick={() => void send()}>
              {running ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
              {running ? '执行中...' : '发送'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function LoreAgentMessage({ message }: { message: LoreAgentChatMessage }) {
  if (message.role === 'clear') {
    return (
      <div className="flex items-center gap-3 py-2">
        <div className="h-px flex-1 bg-[var(--nova-border)]" />
        <div className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-1 text-[11px] text-[var(--nova-text-faint)]">
          {message.content || '上下文已清理'}
        </div>
        <div className="h-px flex-1 bg-[var(--nova-border)]" />
      </div>
    )
  }
  if (message.role === 'thinking') {
    return (
      <div className="flex justify-start">
        <div className="max-w-[78%] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
          <div className="mb-1 flex items-center gap-1.5 text-[11px] font-medium text-[var(--nova-text-muted)]">
            <Loader2 className="h-3 w-3 animate-spin" />
            思考过程
          </div>
          <div className="whitespace-pre-wrap break-words">{message.content}</div>
        </div>
      </div>
    )
  }
  if (message.role === 'tool_call') {
    return (
      <div className="flex justify-start">
        <div className="max-w-[78%] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-muted)]">
          <div className="flex items-center gap-2">
            {message.status === 'running' ? <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--nova-text-faint)]" /> : <Bot className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />}
            <span className="font-medium text-[var(--nova-text)]">{message.name || message.content}</span>
            <span className="text-[11px] text-[var(--nova-text-faint)]">{message.status === 'running' ? '执行中' : '完成'}</span>
          </div>
          {message.args && (
            <pre className="mt-2 max-h-40 overflow-auto rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 font-mono text-[11px] leading-4 text-[var(--nova-text-faint)]">
              {message.args}
            </pre>
          )}
          {message.toolResult && (
            <div className="mt-2 whitespace-pre-wrap rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-[11px] text-[var(--nova-text-muted)]">
              {message.toolResult}
            </div>
          )}
        </div>
      </div>
    )
  }
  const isUser = message.role === 'user'
  const isError = message.role === 'error'
  return (
    <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-[78%] rounded-[var(--nova-radius)] border px-3 py-2 text-sm leading-6 ${
          isUser
            ? 'border-[var(--nova-active)] bg-[var(--nova-active)] text-[var(--nova-text)]'
            : isError
              ? 'border-red-500/40 bg-red-500/10 text-red-100'
              : 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)]'
        }`}
      >
        <div className="whitespace-pre-wrap break-words">{message.content}</div>
        {message.references && message.references.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {message.references.map((item) => (
              <span key={item.id} className="inline-flex max-w-full items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-xs text-[var(--nova-text-muted)]">
                <AtSign className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" />
                <span className="truncate">{item.name}</span>
              </span>
            ))}
          </div>
        )}
        {message.result && (
          <div className="mt-2 space-y-1 border-t border-[var(--nova-border)] pt-2 text-xs text-[var(--nova-text-faint)]">
            {message.result.created?.length ? <div>新增：{message.result.created.map((item) => item.name).join('，')}</div> : null}
            {message.result.updated?.length ? <div>更新：{message.result.updated.map((item) => item.name).join('，')}</div> : null}
            {message.result.deleted_ids?.length ? <div>删除：{message.result.deleted_ids.join('，')}</div> : null}
          </div>
        )}
      </div>
    </div>
  )
}

function TellerAgentMessage({ message }: { message: TellerAgentChatMessage }) {
  if (message.role === 'clear') {
    return (
      <div className="flex items-center gap-3 py-2">
        <div className="h-px flex-1 bg-[var(--nova-border)]" />
        <div className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-1 text-[11px] text-[var(--nova-text-faint)]">
          {message.content || '上下文已清理'}
        </div>
        <div className="h-px flex-1 bg-[var(--nova-border)]" />
      </div>
    )
  }
  if (message.role === 'thinking') {
    return (
      <div className="flex justify-start">
        <div className="max-w-[78%] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
          <div className="mb-1 flex items-center gap-1.5 text-[11px] font-medium text-[var(--nova-text-muted)]">
            <Loader2 className="h-3 w-3 animate-spin" />
            思考过程
          </div>
          <div className="whitespace-pre-wrap break-words">{message.content}</div>
        </div>
      </div>
    )
  }
  if (message.role === 'tool_call') {
    return (
      <div className="flex justify-start">
        <div className="max-w-[78%] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-muted)]">
          <div className="flex items-center gap-2">
            {message.status === 'running' ? <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--nova-text-faint)]" /> : <Bot className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />}
            <span className="font-medium text-[var(--nova-text)]">{message.name || message.content}</span>
            <span className="text-[11px] text-[var(--nova-text-faint)]">{message.status === 'running' ? '执行中' : '完成'}</span>
          </div>
          {message.args && (
            <pre className="mt-2 max-h-40 overflow-auto rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 font-mono text-[11px] leading-4 text-[var(--nova-text-faint)]">
              {message.args}
            </pre>
          )}
          {message.toolResult && (
            <div className="mt-2 whitespace-pre-wrap rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-[11px] text-[var(--nova-text-muted)]">
              {message.toolResult}
            </div>
          )}
        </div>
      </div>
    )
  }
  const isUser = message.role === 'user'
  const isError = message.role === 'error'
  return (
    <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-[78%] rounded-[var(--nova-radius)] border px-3 py-2 text-sm leading-6 ${
          isUser
            ? 'border-[var(--nova-active)] bg-[var(--nova-active)] text-[var(--nova-text)]'
            : isError
              ? 'border-red-500/40 bg-red-500/10 text-red-100'
              : 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)]'
        }`}
      >
        <div className="whitespace-pre-wrap break-words">{message.content}</div>
        {message.targetTeller && (
          <div className="mt-2 inline-flex max-w-full items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-xs text-[var(--nova-text-muted)]">
            <SlidersHorizontal className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" />
            <span className="truncate">参考：{message.targetTeller.name}</span>
          </div>
        )}
        {message.tellerReferences && message.tellerReferences.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {message.tellerReferences.map((teller) => (
              <span key={teller.id} className="inline-flex max-w-full items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-xs text-[var(--nova-text-muted)]">
                <AtSign className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" />
                <span className="truncate">{teller.name}</span>
              </span>
            ))}
          </div>
        )}
        {message.result && (
          <div className="mt-2 space-y-1 border-t border-[var(--nova-border)] pt-2 text-xs text-[var(--nova-text-faint)]">
            <div>{message.result.action === 'update' ? '更新' : '新增'}：{message.result.teller.name}</div>
            <div>ID：{message.result.teller.id}</div>
          </div>
        )}
      </div>
    </div>
  )
}

function LoreDirectory({
  items,
  activeId,
  query,
  saving,
  onQueryChange,
  onSelect,
  onCreate,
}: {
  items: LoreItem[]
  activeId: string
  query: string
  saving: boolean
  onQueryChange: (value: string) => void
  onSelect: (id: string) => void
  onCreate: (section: KnowledgeSection) => void
}) {
  const [collapsedSections, setCollapsedSections] = useState<Record<string, boolean>>({})
  const sections = KNOWLEDGE_SECTIONS
    .map((section) => ({ section, entries: sectionItems(items, section, query) }))
    .sort((a, b) => {
      if (a.entries.length === 0 && b.entries.length > 0) return 1
      if (a.entries.length > 0 && b.entries.length === 0) return -1
      return KNOWLEDGE_SECTIONS.findIndex((section) => section.id === a.section.id) - KNOWLEDGE_SECTIONS.findIndex((section) => section.id === b.section.id)
    })

  const isCollapsed = (section: KnowledgeSection, entries: LoreItem[]) => collapsedSections[section.id] ?? entries.length === 0
  const toggleSection = (section: KnowledgeSection, entries: LoreItem[]) => {
    setCollapsedSections((current) => ({
      ...current,
      [section.id]: !(current[section.id] ?? entries.length === 0),
    }))
  }

  return (
    <>
      <div className="border-b border-[var(--nova-border)] p-2">
        <div className="nova-field flex h-8 items-center gap-2 rounded-[var(--nova-radius)] px-2 text-xs text-[var(--nova-text-faint)]">
          <Search className="h-3.5 w-3.5" />
          <input
            className="min-w-0 flex-1 bg-transparent text-[var(--nova-text-muted)] outline-none placeholder:text-[var(--nova-text-faint)]"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="搜索资料"
          />
        </div>
        <button
          type="button"
          onClick={() => onSelect(LORE_AGENT_ENTRY_ID)}
          className={`mt-2 flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeId === LORE_AGENT_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
          }`}
        >
          <Bot className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 flex-1 truncate">资料库 Agent</span>
        </button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-2">
          {sections.map(({ section, entries }) => {
            const Icon = section.icon
            const collapsed = isCollapsed(section, entries)
            return (
              <section key={section.id} className={entries.length ? 'mb-2' : 'mb-1'}>
                <div className={`flex h-8 items-center gap-2 rounded px-2 text-xs ${entries.length ? 'text-[var(--nova-text-muted)]' : 'text-[var(--nova-text-faint)]'}`}>
                  <button
                    type="button"
                    className="nova-nav-item rounded p-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    onClick={() => toggleSection(section, entries)}
                    aria-label={collapsed ? `展开${section.label}` : `折叠${section.label}`}
                  >
                    <ChevronDown className={`h-3.5 w-3.5 transition-transform ${collapsed ? '-rotate-90' : ''}`} />
                  </button>
                  <Icon className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
                  <span className="min-w-0 flex-1 truncate font-medium">{section.label}</span>
                  <span className="text-[11px] text-[var(--nova-text-faint)]">{entries.length}</span>
                  <button
                    type="button"
                    className="nova-nav-item rounded p-1 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    disabled={saving}
                    onClick={() => onCreate(section)}
                    aria-label={`新建${section.label}`}
                  >
                    <Plus className="h-3.5 w-3.5" />
                  </button>
                </div>
                {!collapsed && entries.length > 0 && (
                  <div className="ml-5 space-y-0.5 border-l border-[var(--nova-border)] pl-2">
                    {entries.map((item) => (
                      <button
                        key={item.id}
                        type="button"
                        onClick={() => onSelect(item.id)}
                        className={`flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
                          activeId === item.id ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                        }`}
                      >
                        <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                        <span className="min-w-0 flex-1 truncate">{item.name}</span>
                      </button>
                    ))}
                  </div>
                )}
              </section>
            )
          })}
        </div>
      </ScrollArea>
    </>
  )
}

function CreatorDirectory() {
  return (
    <div className="p-2">
      <div className="flex h-8 items-center gap-2 rounded px-2 text-xs text-[var(--nova-text-muted)]">
        <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        <Folder className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        <span className="font-medium">作品根目录</span>
      </div>
      <div className="ml-5 border-l border-[var(--nova-border)] pl-2">
        <div className="flex h-8 items-center gap-2 rounded-[var(--nova-radius)] bg-[var(--nova-active)] px-2 text-xs text-[var(--nova-text)]">
          <BookMarked className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
          <span className="truncate">{CREATOR_PATH}</span>
        </div>
      </div>
    </div>
  )
}

function TellerDirectory({
  tellers,
  activeTellerId,
  saving,
  onSelect,
  onCreate,
}: {
  tellers: Teller[]
  activeTellerId: string
  saving: boolean
  onSelect: (id: string) => void
  onCreate: () => void
}) {
  return (
    <>
      <div className="flex h-10 items-center justify-between border-b border-[var(--nova-border)] px-3">
        <div className="text-xs font-medium text-[var(--nova-text-muted)]">讲述者目录</div>
        <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving} onClick={onCreate} aria-label="新建讲述者">
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>
      <div className="border-b border-[var(--nova-border)] p-2">
        <button
          type="button"
          onClick={() => onSelect(TELLER_AGENT_ENTRY_ID)}
          className={`flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeTellerId === TELLER_AGENT_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
          }`}
        >
          <Bot className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 flex-1 truncate">讲述者 Agent</span>
        </button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-2">
          <div className="flex h-8 items-center gap-2 rounded px-2 text-xs text-[var(--nova-text-muted)]">
            <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
            <Folder className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
            <span className="font-medium">规则包</span>
          </div>
          <div className="ml-5 space-y-0.5 border-l border-[var(--nova-border)] pl-2">
            {tellers.map((teller) => (
              <button
                key={teller.id}
                type="button"
                onClick={() => onSelect(teller.id)}
                className={`flex min-h-9 w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs transition ${
                  activeTellerId === teller.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                }`}
              >
                <SlidersHorizontal className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{teller.name}</span>
                  <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{teller.custom ? '自定义' : '内置'} · {(teller.slots || []).filter((slot) => slot.enabled).length} 条启用规则</span>
                </span>
              </button>
            ))}
          </div>
        </div>
      </ScrollArea>
    </>
  )
}

function LoreEditor({
  draft,
  tagDraft,
  setDraft,
  setTagDraft,
  onSave,
}: {
  draft: LoreItem | null
  tagDraft: string
  setDraft: (draft: LoreItem | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
}) {
  if (!draft) {
    return <EmptyState title="未选择资料" description="从左侧资料库目录选择或新建一个条目。" />
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="grid shrink-0 gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 lg:grid-cols-[minmax(220px,1fr)_180px_180px]">
        <Field label="名称">
          <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
        </Field>
        <Field label="类型">
          <Select value={draft.type} onValueChange={(value) => setDraft({ ...draft, type: value as LoreItem['type'] })}>
            <SelectTrigger size="sm" className={selectClassName}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              {TYPE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label="重要度">
          <Select value={draft.importance} onValueChange={(value) => setDraft({ ...draft, importance: value as LoreItem['importance'] })}>
            <SelectTrigger size="sm" className={selectClassName}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              {IMPORTANCE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label="标签">
          <Input className={inputClassName} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder="用逗号分隔" />
        </Field>
      </div>
      <div className="min-h-0 flex-1 p-4">
        <Textarea
          className="nova-field h-full min-h-[360px] resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
          value={draft.content || ''}
          onChange={(event) => setDraft({ ...draft, content: event.target.value })}
          onKeyDown={(event) => {
            if (isSaveShortcut(event)) {
              event.preventDefault()
              event.stopPropagation()
              onSave()
            }
          }}
        />
      </div>
    </div>
  )
}

function CreatorEditor({
  content,
  setContent,
  onSave,
}: {
  content: string
  setContent: (value: string) => void
  onSave: () => void
}) {
  return (
    <div className="min-h-0 flex-1 p-4">
      <Textarea
        className="nova-field h-full min-h-[520px] resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
        value={content}
        onChange={(event) => setContent(event.target.value)}
        placeholder="写下本书最高优先级的创作规则..."
        onKeyDown={(event) => {
          if (isSaveShortcut(event)) {
            event.preventDefault()
            event.stopPropagation()
            onSave()
          }
        }}
      />
    </div>
  )
}

function TellerEditor({
  workspace,
  draft,
  setDraft,
  tagDraft,
  setTagDraft,
  activeSlotId,
  setActiveSlotId,
  onSave,
}: {
  workspace: string
  draft: Teller | null
  setDraft: (draft: Teller | null) => void
  tagDraft: string
  setTagDraft: (value: string) => void
  activeSlotId: string
  setActiveSlotId: (id: string) => void
  onSave: () => void
}) {
  const activeSlot = draft?.slots?.find((slot) => slot.id === activeSlotId) || draft?.slots?.[0] || null
  const [targetPickerOpen, setTargetPickerOpen] = useState(false)
  const [availableStyles, setAvailableStyles] = useState<string[]>([])

  useEffect(() => {
    setTargetPickerOpen(false)
  }, [activeSlotId])

  useEffect(() => {
    let cancelled = false
    if (!workspace) {
      setAvailableStyles([])
      return () => { cancelled = true }
    }
    getStyles()
      .then((styles) => {
        if (!cancelled) setAvailableStyles(styles)
      })
      .catch((err) => {
        console.warn('[teller-editor] 获取风格参考列表失败', err)
        if (!cancelled) setAvailableStyles([])
      })
    return () => { cancelled = true }
  }, [workspace])

  const updateSlotById = (slotId: string, patch: Partial<TellerPromptSlot>) => {
    if (!draft) return
    setDraft({
      ...draft,
      slots: draft.slots.map((slot) => slot.id === slotId ? { ...slot, ...patch } : slot),
    })
  }

  const updateSlot = (patch: Partial<TellerPromptSlot>) => {
    if (!draft || !activeSlot) return
    updateSlotById(activeSlot.id, patch)
  }

  const addSlot = () => {
    if (!draft) return
    const id = `slot-${Date.now()}`
    const slot: TellerPromptSlot = {
      id,
      name: '新规则',
      target: 'turn_context',
      enabled: true,
      content: '',
    }
    setDraft({ ...draft, slots: [...(draft.slots || []), slot] })
    setActiveSlotId(id)
  }

  const deleteSlot = () => {
    if (!draft || !activeSlot) return
    const nextSlots = draft.slots.filter((slot) => slot.id !== activeSlot.id)
    setDraft({ ...draft, slots: nextSlots })
    setActiveSlotId(nextSlots[0]?.id || '')
  }

  if (!draft) {
    return <EmptyState title="未选择讲述者" description="从左侧讲述者目录选择或新建一个规则包。" />
  }

  const selectedTarget = targetOption(activeSlot?.target || 'turn_context')

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="grid shrink-0 gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,1fr)_150px_150px]">
        <Field label="名称">
          <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
        </Field>
        <Field label="描述">
          <Input className={inputClassName} value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} placeholder="选择时显示的简介" />
        </Field>
        <Field label="随机事件率">
          <Input className={inputClassName} value={String(draft.random_event_rate ?? 0)} onChange={(event) => setDraft({ ...draft, random_event_rate: Number(event.target.value) || 0 })} />
        </Field>
        <SettingsNumberField
          label="单轮目标字数"
          value={draft.reply_target_chars ?? null}
          placeholder="默认：1200"
          onChange={(value) => setDraft({ ...draft, reply_target_chars: value })}
        />
        <Field label="标签">
          <Input className={inputClassName} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder="用逗号分隔" />
        </Field>
        <Field label="上下文回合数">
          <Input
            className={inputClassName}
            value={String(draft.context_policy?.recent_turns ?? 0)}
            onChange={(event) => setDraft({
              ...draft,
              context_policy: { ...draft.context_policy, recent_turns: Number(event.target.value) || 0 },
            })}
          />
        </Field>
        <div className="flex items-end">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs text-[var(--nova-text-faint)]">{draft.custom ? '自定义' : '内置'}</span>
        </div>
      </div>

      <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
        <div className="mb-3">
          <div className="text-xs font-medium text-[var(--nova-text)]">场景风格规则</div>
          <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">
            当前讲述者在 IDE 章节创作和互动故事生成时使用；本轮显式输入 # 风格参考时优先生效。
          </div>
        </div>
        <InteractiveStyleRulesEditor
          available={availableStyles}
          rules={draft.style_rules ?? []}
          onChange={(rules) => setDraft({ ...draft, style_rules: rules })}
        />
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-[280px_minmax(0,1fr)]">
        <aside className="flex min-h-0 flex-col border-r border-[var(--nova-border)] bg-[var(--nova-surface)]">
          <div className="flex h-11 items-center justify-between border-b border-[var(--nova-border)] px-3">
            <div className="text-xs font-medium text-[var(--nova-text-muted)]">注入规则</div>
            <Button className={iconActionClassName} variant="outline" size="icon" onClick={addSlot} aria-label="新增注入规则">
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </div>
          <ScrollArea className="min-h-0 flex-1">
            <div className="p-2">
              {(draft.slots || []).map((slot) => (
                <div
                  key={slot.id}
                  className={`mb-1 flex min-h-12 w-full items-center gap-2 rounded-md border px-3 py-2 text-xs transition ${
                    activeSlot?.id === slot.id
                      ? 'border-[var(--nova-accent)]/45 bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_3px_0_0_var(--nova-accent)]'
                      : 'border-transparent text-[var(--nova-text-muted)] hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                  }`}
                >
                  <button
                    type="button"
                    onClick={() => setActiveSlotId(slot.id)}
                    className="flex min-w-0 flex-1 items-center gap-2 text-left"
                  >
                    <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{slot.name}</span>
                      <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11px] text-[var(--nova-text-faint)]">
                        <span className="truncate">{targetLabel(slot.target)}</span>
                        <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${slot.enabled ? 'bg-[var(--nova-accent-green)]' : 'bg-[var(--nova-text-faint)]/35'}`} />
                        <span className="shrink-0">{slot.enabled ? '启用' : '停用'}</span>
                      </span>
                    </span>
                  </button>
                  <ToggleSwitch
                    checked={slot.enabled}
                    compact
                    onChange={(enabled) => updateSlotById(slot.id, { enabled })}
                  />
                </div>
              ))}
            </div>
          </ScrollArea>
        </aside>

        {activeSlot ? (
          <section className="flex min-h-0 flex-col">
            <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
              <div className="grid gap-3 lg:grid-cols-[minmax(220px,1fr)_minmax(240px,320px)_32px]">
                <Field label="规则名称">
                  <Input className={inputClassName} value={activeSlot.name} onChange={(event) => updateSlot({ name: event.target.value })} />
                </Field>
                <div className="grid gap-1.5">
                  <span className="text-[11px] text-[var(--nova-text-faint)]">注入位置</span>
                  <Popover open={targetPickerOpen} onOpenChange={setTargetPickerOpen}>
                    <PopoverTrigger asChild>
                      <button
                        type="button"
                        aria-label="注入位置"
                        className={`${selectClassName} flex w-full items-center justify-between gap-2 px-3 text-left text-[var(--nova-text)]`}
                      >
                        <span className="min-w-0 flex-1 truncate">{selectedTarget.label} · {selectedTarget.summary}</span>
                        <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition ${targetPickerOpen ? 'rotate-180' : ''}`} />
                      </button>
                    </PopoverTrigger>
                    <PopoverContent align="start" sideOffset={6} className="nova-panel w-[320px] border border-[var(--nova-border)] p-1.5 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
                      {TELLER_TARGET_OPTIONS.map((option) => (
                        <button
                          key={option.value}
                          type="button"
                          onClick={() => {
                            updateSlot({ target: option.value as TellerTarget })
                            setTargetPickerOpen(false)
                          }}
                          className={`flex w-full items-start gap-2 rounded-md px-3 py-2.5 text-left transition ${
                            activeSlot.target === option.value
                              ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
                              : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                          }`}
                        >
                          <span className={`mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full border ${
                            activeSlot.target === option.value ? 'border-[var(--nova-accent)] bg-[var(--nova-accent)]/15 text-[var(--nova-accent)]' : 'border-[var(--nova-border)] text-transparent'
                          }`}>
                            <Check className="h-3 w-3" />
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="block text-xs font-medium">{option.label}</span>
                            <span className="mt-0.5 block text-[11px] leading-4 text-[var(--nova-text-faint)]">{option.summary}</span>
                          </span>
                        </button>
                      ))}
                    </PopoverContent>
                  </Popover>
                </div>
                <div className="flex items-end justify-end">
                  <Button className={iconActionClassName} variant="outline" size="icon" disabled={(draft.slots || []).length <= 1} onClick={deleteSlot} aria-label="删除注入规则">
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
                <div className="lg:col-span-3">
                  <div className="min-w-0 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5">
                    <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
                      <span>{selectedTarget.label}</span>
                      <span className="h-1 w-1 rounded-full bg-[var(--nova-text-faint)]/50" />
                      <span className="text-[var(--nova-text-faint)]">{selectedTarget.summary}</span>
                    </div>
                    <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-muted)]">{selectedTarget.detail}</div>
                  </div>
                </div>
              </div>
            </div>
            <div className="min-h-0 flex-1 p-4">
              <Textarea
                className="nova-field h-full min-h-[360px] resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
                value={activeSlot.content}
                onChange={(event) => updateSlot({ content: event.target.value })}
                onKeyDown={(event) => {
                  if (isSaveShortcut(event)) {
                    event.preventDefault()
                    event.stopPropagation()
                    onSave()
                  }
                }}
              />
            </div>
          </section>
        ) : (
          <EmptyState title="暂无注入规则" description="为这个讲述者新增一条规则。" />
        )}
      </div>
    </div>
  )
}

function SettingsNumberField({ label, value, placeholder, onChange }: {
  label: string
  value: number | null
  placeholder?: string
  onChange: (value: number | null) => void
}) {
  return (
    <Field label={label}>
      <Input
        className={inputClassName}
        type="number"
        value={value ?? ''}
        placeholder={placeholder}
        onChange={(event) => {
          const raw = event.target.value
          onChange(raw === '' ? null : Number(raw))
        }}
      />
    </Field>
  )
}

function InteractiveStyleRulesEditor({ available, rules, onChange }: {
  available: string[]
  rules: StyleRule[]
  onChange: (rules: StyleRule[]) => void
}) {
  const addRule = () => onChange([...rules, { scene: '', styles: [] }])
  const removeRule = (index: number) => onChange(rules.filter((_, i) => i !== index))
  const updateRule = (index: number, patch: Partial<StyleRule>) => {
    onChange(rules.map((rule, i) => (i === index ? { ...rule, ...patch } : rule)))
  }

  return (
    <div className="flex flex-col gap-3">
      {rules.length > 0 && (
        <div className="space-y-2">
          {rules.map((rule, index) => (
            <InteractiveStyleRuleRow
              key={index}
              available={available}
              rule={rule}
              onChange={(patch) => updateRule(index, patch)}
              onRemove={() => removeRule(index)}
            />
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <Button className={actionButtonClassName} variant="outline" size="sm" onClick={addRule}>
          <Plus className="h-3.5 w-3.5" />
          新增规则
        </Button>
        {available.length === 0 && (
          <span className="text-xs text-[var(--nova-text-faint)]">当前用户目录 styles/ 下暂无风格文件，也可以手动添加 .md 或 .txt 路径。</span>
        )}
      </div>
    </div>
  )
}

function InteractiveStyleRuleRow({ available, rule, onChange, onRemove }: {
  available: string[]
  rule: StyleRule
  onChange: (patch: Partial<StyleRule>) => void
  onRemove: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const [customPath, setCustomPath] = useState('')
  const selectedCustomStyles = rule.styles.filter((path) => !available.includes(path))
  const summary = rule.styles.length === 0 ? '尚未选择风格文件' : rule.styles.join('、')
  const toggleStyle = (path: string) => {
    onChange({
      styles: rule.styles.includes(path)
        ? rule.styles.filter((item) => item !== path)
        : [...rule.styles, path],
    })
  }
  const addCustomStyle = () => {
    const path = customPath.trim()
    if (!path || rule.styles.includes(path)) return
    onChange({ styles: [...rule.styles, path] })
    setCustomPath('')
  }

  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
      <div className="flex flex-col gap-2 md:flex-row md:items-center">
        <Input
          className={inputClassName}
          value={rule.scene}
          placeholder="场景描述，如：激烈打斗 / 日常对话 / 压抑悬疑"
          onChange={(event) => onChange({ scene: event.target.value })}
        />
        <Button className={`${actionButtonClassName} justify-center`} variant="outline" size="sm" onClick={() => setExpanded((value) => !value)}>
          {expanded ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          {expanded ? '收起' : `风格 (${rule.styles.length})`}
        </Button>
        <Button className={`${actionButtonClassName} justify-center hover:bg-red-500/15 hover:text-red-200`} variant="outline" size="sm" onClick={onRemove}>
          <Trash2 className="h-3.5 w-3.5" />
          删除
        </Button>
      </div>

      {!expanded && (
        <div className="mt-1 truncate px-1 text-xs text-[var(--nova-text-faint)]">→ {summary}</div>
      )}

      {expanded && (
        <div className="mt-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
          <div className="max-h-52 overflow-y-auto">
            {available.length === 0 ? (
              <div className="px-2 py-2 text-xs text-[var(--nova-text-faint)]">无可用风格文件</div>
            ) : (
              available.map((path) => (
                <label key={path} className="flex cursor-pointer items-center gap-2 px-2 py-1.5 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]">
                  <input type="checkbox" checked={rule.styles.includes(path)} onChange={() => toggleStyle(path)} />
                  <span className="truncate" title={path}>{path}</span>
                </label>
              ))
            )}
            {selectedCustomStyles.map((path) => (
              <label key={path} className="flex cursor-pointer items-center gap-2 px-2 py-1.5 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]">
                <input type="checkbox" checked onChange={() => toggleStyle(path)} />
                <span className="truncate" title={path}>{path}</span>
              </label>
            ))}
          </div>
          <div className="flex flex-col gap-2 border-t border-[var(--nova-border)] p-2 md:flex-row">
            <Input
              className={inputClassName}
              value={customPath}
              placeholder="手动添加 .md 或 .txt 文件路径"
              onChange={(event) => setCustomPath(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  addCustomStyle()
                }
              }}
            />
            <Button className={`${actionButtonClassName} justify-center`} variant="outline" size="sm" onClick={addCustomStyle}>
              <Plus className="h-3.5 w-3.5" />
              添加
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="grid gap-1.5">
      <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 text-center">
        <div className="text-sm font-medium text-[var(--nova-text)]">{title}</div>
        <div className="mt-1 text-xs text-[var(--nova-text-faint)]">{description}</div>
      </div>
    </div>
  )
}

function ToggleSwitch({ checked, onChange, compact = false }: { checked: boolean; onChange: (checked: boolean) => void; compact?: boolean }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      title={checked ? '停用规则' : '启用规则'}
      className={`relative shrink-0 rounded-full border transition ${
        checked ? 'border-[var(--nova-accent-green)]/60 bg-[var(--nova-accent-green)]/25' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)]'
      } ${compact ? 'h-5 w-9' : 'h-6 w-11'}`}
    >
      <span
        className={`absolute rounded-full bg-[var(--nova-text)] shadow transition ${
          compact
            ? `top-0.5 h-4 w-4 ${checked ? 'left-[18px]' : 'left-0.5'}`
            : `top-0.5 h-5 w-5 ${checked ? 'left-[22px]' : 'left-0.5'}`
        }`}
      />
      <span className="sr-only">{checked ? '停用规则' : '启用规则'}</span>
    </button>
  )
}

const actionButtonClassName = 'nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
const selectClassName = 'nova-field h-8 text-xs focus:ring-0'

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

function parseLoreEventData<T>(data: string): T | null {
  try {
    return JSON.parse(data) as T
  } catch {
    return null
  }
}

function loreHistoryMessageToChat(message: ChatMessage, index: number, items: LoreItem[]): LoreAgentChatMessage {
  if (message.type === 'clear') {
    return {
      id: `history-clear-${index}`,
      role: 'clear',
      content: '已清理资料库 Agent 上下文，历史消息仍保留。',
    }
  }
  const role = message.role === 'user' ? 'user' : message.role === 'error' ? 'error' : 'assistant'
  return {
    id: `history-${index}`,
    role,
    content: message.content || '',
    references: role === 'user' ? loreReferencesFromContent(message.content || '', items) : undefined,
  }
}

function loreReferencesFromContent(content: string, items: LoreItem[]) {
  return items.filter((item) => item.name && content.includes(`@${item.name}`))
}

function tellerHistoryMessageToChat(message: ChatMessage, index: number, tellers: Teller[]): TellerAgentChatMessage {
  if (message.type === 'clear') {
    return {
      id: `history-clear-${index}`,
      role: 'clear',
      content: '已清理讲述者 Agent 上下文，历史消息仍保留。',
    }
  }
  const role = message.role === 'user' ? 'user' : message.role === 'error' ? 'error' : 'assistant'
  return {
    id: `history-${index}`,
    role,
    content: message.content || '',
    tellerReferences: role === 'user' ? tellerReferencesFromContent(message.content || '', tellers) : undefined,
  }
}

function tellerReferencesFromContent(content: string, tellers: Teller[]) {
  return tellers.filter((teller) => teller.name && content.includes(`@${teller.name}`))
}

function loreAgentResultSummary(result: LoreAgentResult) {
  const changed = [
    result.created?.length ? `新增 ${result.created.length}` : '',
    result.updated?.length ? `更新 ${result.updated.length}` : '',
    result.deleted_ids?.length ? `删除 ${result.deleted_ids.length}` : '',
  ].filter(Boolean).join('，')
  return `${result.message || '资料库 Agent 已完成'}${changed ? `（${changed}）` : ''}`
}

function tellerAgentResultSummary(result: TellerAgentResult) {
  const action = result.action === 'update' ? '更新' : '新增'
  return `${result.message || '讲述者 Agent 已完成'}（${action}：${result.teller.name}）`
}

function formatDateTime(value: string) {
  if (!value) return '未知时间'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function ModeIcon({ mode }: { mode: SettingPanelMode }) {
  if (mode === 'creator') return <BookMarked className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
  if (mode === 'teller') return <SlidersHorizontal className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
  return <Database className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
}

function sectionItems(items: LoreItem[], section: KnowledgeSection, query = '') {
  const normalizedQuery = query.trim().toLowerCase()
  return items.filter((item) => {
    if (!section.types.includes(item.type)) return false
    const tags = item.tags || []
    if (section.tag && !tags.includes(section.tag)) return false
    if (section.excludeTag && tags.includes(section.excludeTag)) return false
    if (normalizedQuery) {
      const haystack = `${item.name}\n${item.content || ''}\n${tags.join('\n')}`.toLowerCase()
      if (!haystack.includes(normalizedQuery)) return false
    }
    return true
  })
}

function targetLabel(target: TellerTarget) {
  return targetOption(target).label
}

function targetOption(target: TellerTarget) {
  return TELLER_TARGET_OPTIONS.find((option) => option.value === target) || TELLER_TARGET_OPTIONS[1]
}

function loreTypeLabel(type: LoreItem['type']) {
  return TYPE_OPTIONS.find((option) => option.value === type)?.label || '其他'
}

function panelTitle(mode: SettingPanelMode) {
  if (mode === 'creator') return '创作者'
  if (mode === 'teller') return '讲述者'
  return '资料库'
}

function editorTitle(mode: SettingPanelMode, draft: LoreItem | null, tellerDraft: Teller | null) {
  if (mode === 'creator') return CREATOR_PATH
  if (mode === 'teller') return tellerDraft?.name || '故事讲述者'
  return draft?.name || '资料库'
}

function editorSubtitle(mode: SettingPanelMode, draft: LoreItem | null, tellerDraft: Teller | null) {
  if (mode === 'creator') return '当前作品最高优先级规则'
  if (mode === 'teller') return tellerDraft?.description || '用户级 prompt slot 配置'
  if (!draft) return '角色、地点、组织、规则与素材'
  return `${loreTypeLabel(draft.type)} · ${draft.importance} · ${(draft.tags || []).join('，') || '无标签'}`
}

function newTellerDraft(): Partial<Teller> {
  const id = `custom-${Date.now()}`
  return {
    id,
    name: '自定义讲述者',
    description: '新的故事讲述规则包',
    random_event_rate: 0.15,
    reply_target_chars: 1200,
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
        content: '你是一位自定义故事讲述者。你要明确影响故事的题材倾向、角色反应和剧情推进方式。',
      },
      {
        id: 'turn_context',
        name: '本轮上下文',
        target: 'turn_context',
        enabled: true,
        content: '每轮都要让用户行动带来具体后果，并主动制造符合讲述者风格的反馈、阻碍、发现、NPC 反应或新的行动入口。',
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
