import { useEffect, useRef, useState } from 'react'
import { BookMarked, Building2, Database, FileText, Image as ImageIcon, Library, Loader2, MapPin, PanelLeft, Save, ScrollText, Search, SlidersHorizontal, Sparkles, Trash2, UserRound } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { abortLoreImagesGenerate, clearLoreItemImage, createLoreItem, deleteLoreItem, generateLoreItemImage, getLoreItems, readFile, saveFile, streamLoreImagesGenerate, updateLoreItem, workspaceAssetURL, type LoreImageProgressEvent, type LoreItem, type SSEEvent } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { createImagePreset, createInteractiveTeller, deleteImagePreset, deleteInteractiveTeller, getImagePresets, getInteractiveTellers, updateImagePreset, updateInteractiveTeller } from '../api'
import { INTERACTIVE_OPENING_PRESET_PATH, INTERACTIVE_OPENING_PRESET_UPDATED_EVENT, INTERACTIVE_OPENING_PRESET_ENTRY_ID, LEGACY_INTERACTIVE_OPENING_PRESET_PATH, parseBookOpeningPresets, serializeBookOpeningPresets, type BookOpeningPreset } from '../opening'
import type { ImagePreset, Teller } from '../types'
import { CreatorDirectory, CreatorEditor, ImagePresetEditor, LoreDirectory, LoreEditor, OpeningPresetEditor, TellerDirectory } from './SettingPanelSections'
import { TellerEditor } from './SettingPanelTellerEditor'

const CREATOR_PATH = 'CREATOR.md'
const CREATOR_ENTRY_ID = '__creator__'
const LORE_CONFIG_AGENT_ENTRY_ID = '__config_manager_lore__'
const TELLER_CONFIG_AGENT_ENTRY_ID = '__config_manager_teller__'
const EMPTY_TELLERS: Teller[] = []
const EMPTY_IMAGE_PRESETS: ImagePreset[] = []
type PresetResourceKind = 'teller' | 'image'

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
const LORE_TYPE_FILTER_OPTIONS: LoreType[] = ['character', 'world', 'location', 'faction', 'rule', 'item', 'other']

interface SettingPanelProps {
  mode?: SettingPanelMode
  workspace?: string
  tellers?: Teller[]
  imagePresets?: ImagePreset[]
  onTellersChange?: (tellers: Teller[]) => void
  onImagePresetsChange?: (presets: ImagePreset[]) => void
  embedded?: boolean
}

export function SettingPanel({ mode, workspace = '', tellers: externalTellers = EMPTY_TELLERS, imagePresets: externalImagePresets = EMPTY_IMAGE_PRESETS, onTellersChange, onImagePresetsChange, embedded = false }: SettingPanelProps) {
  const { t } = useTranslation()
  const activeMode = mode || 'lore'
  const [items, setItems] = useState<LoreItem[]>([])
  const [activeId, setActiveId] = useState('')
  const [draft, setDraft] = useState<LoreItem | null>(null)
  const [tagDraft, setTagDraft] = useState('')
  const [query, setQuery] = useState('')
  const [creatorContent, setCreatorContent] = useState('')
  const [creatorRevision, setCreatorRevision] = useState('')
  const [openingPresets, setOpeningPresets] = useState<BookOpeningPreset[]>([])
  const [openingPresetRevision, setOpeningPresetRevision] = useState('')
  const [activeOpeningPresetId, setActiveOpeningPresetId] = useState('')
  const [tellers, setTellers] = useState<Teller[]>(externalTellers)
  const [activeTellerId, setActiveTellerId] = useState('')
  const [tellerDraft, setTellerDraft] = useState<Teller | null>(null)
  const [tellerTagDraft, setTellerTagDraft] = useState('')
  const [presetResourceKind, setPresetResourceKind] = useState<PresetResourceKind>('teller')
  const [imagePresets, setImagePresets] = useState<ImagePreset[]>(externalImagePresets)
  const [activeImagePresetId, setActiveImagePresetId] = useState('')
  const [imagePresetDraft, setImagePresetDraft] = useState<ImagePreset | null>(null)
  const [imagePresetTagDraft, setImagePresetTagDraft] = useState('')
  const [activeSlotId, setActiveSlotId] = useState('')
  const [loreImageInstruction, setLoreImageInstruction] = useState('')
  const [loreImageGeneratingId, setLoreImageGeneratingId] = useState('')
  const [loreImageBatchOpen, setLoreImageBatchOpen] = useState(false)
  const [loreImageBatchSelectedIds, setLoreImageBatchSelectedIds] = useState<string[]>([])
  const [loreImageBatchQuery, setLoreImageBatchQuery] = useState('')
  const [loreImageBatchType, setLoreImageBatchType] = useState<LoreType | 'all'>('all')
  const [loreImageBatchPresetId, setLoreImageBatchPresetId] = useState('')
  const [loreImageBatchInstruction, setLoreImageBatchInstruction] = useState('')
  const [loreImageBatchOverwrite, setLoreImageBatchOverwrite] = useState(false)
  const [loreImageBatchRunning, setLoreImageBatchRunning] = useState(false)
  const [loreImageBatchProgress, setLoreImageBatchProgress] = useState<Record<string, LoreImageProgressEvent>>({})
  const [deleteLoreTarget, setDeleteLoreTarget] = useState<LoreItem | null>(null)
  const [saving, setSaving] = useState(false)
  const loreDraftRef = useRef<LoreItem | null>(null)
  const loreTagDraftRef = useRef('')
  const loreAutoSaveTimer = useRef<number | null>(null)
  const loreSavedSignature = useRef('')
  const loreBaseRevisionRef = useRef('')
  const tellerAutoSaveTimer = useRef<number | null>(null)
  const tellerSavedSignature = useRef('')
  const tellerBaseRevisionRef = useRef('')
  const imagePresetAutoSaveTimer = useRef<number | null>(null)
  const imagePresetSavedSignature = useRef('')
  const imagePresetBaseRevisionRef = useRef('')
  const loreImageBatchAbortRef = useRef<AbortController | null>(null)

  useEffect(() => {
    let cancelled = false
    setItems([])
    setActiveId(LORE_CONFIG_AGENT_ENTRY_ID)
    setDraft(null)
    setTagDraft('')
    setQuery('')
    if (!workspace)
      return () => {
        cancelled = true
      }
    getLoreItems()
      .then((data) => {
        if (cancelled) return
        setItems(data)
        setActiveId(LORE_CONFIG_AGENT_ENTRY_ID)
      })
      .catch(() => {
        if (!cancelled) {
          setItems([])
          setActiveId(LORE_CONFIG_AGENT_ENTRY_ID)
        }
      })
    return () => {
      cancelled = true
    }
  }, [workspace])

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
      loreBaseRevisionRef.current = nextDraft?.updated_at || ''
      loreSavedSignature.current = nextDraft ? loreDraftSignature(nextDraft, nextTagDraft) : ''
    }
  }, [activeId, items])

  useEffect(() => {
    loreDraftRef.current = draft
    loreTagDraftRef.current = tagDraft
  }, [draft, tagDraft])

  useEffect(() => {
    if (activeMode !== 'creator' && !(activeMode === 'lore' && activeId === CREATOR_ENTRY_ID)) return
    let cancelled = false
    setCreatorContent('')
    setCreatorRevision('')
    if (!workspace)
      return () => {
        cancelled = true
      }
    readFile(CREATOR_PATH)
      .then((data) => {
        if (!cancelled) {
          setCreatorContent(data.content)
          setCreatorRevision(data.revision || '')
        }
      })
      .catch(() => {
        if (!cancelled) {
          setCreatorContent('')
          setCreatorRevision('')
        }
      })
    return () => {
      cancelled = true
    }
  }, [activeId, activeMode, workspace])

  useEffect(() => {
    if (activeMode !== 'lore' || activeId !== INTERACTIVE_OPENING_PRESET_ENTRY_ID) return
    let cancelled = false
    setOpeningPresets([])
    setOpeningPresetRevision('')
    setActiveOpeningPresetId('')
    if (!workspace)
      return () => {
        cancelled = true
      }
    readFile(INTERACTIVE_OPENING_PRESET_PATH)
      .then((data) => {
        if (cancelled) return
        const presets = parseBookOpeningPresets(data.content)
        setOpeningPresets(presets)
        setOpeningPresetRevision(data.revision || '')
        setActiveOpeningPresetId((current) => (current && presets.some((preset) => preset.id === current) ? current : presets[0]?.id || ''))
      })
      .catch(async () => {
        try {
          const legacy = await readFile(LEGACY_INTERACTIVE_OPENING_PRESET_PATH)
          if (cancelled) return
          const presets = parseBookOpeningPresets(legacy.content)
          setOpeningPresets(presets)
          setOpeningPresetRevision('')
          setActiveOpeningPresetId((current) => (current && presets.some((preset) => preset.id === current) ? current : presets[0]?.id || ''))
        } catch {
          if (!cancelled) {
            setOpeningPresets([])
            setOpeningPresetRevision('')
            setActiveOpeningPresetId('')
          }
        }
      })
    return () => {
      cancelled = true
    }
  }, [activeId, activeMode, workspace])

  useEffect(() => {
    setTellers(externalTellers)
    setActiveTellerId((current) => current || externalTellers[0]?.id || '')
  }, [externalTellers])

  useEffect(() => {
    if (activeMode !== 'teller' || onTellersChange || externalTellers.length > 0 || !workspace) return
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
  }, [activeMode, externalTellers.length, onTellersChange, workspace])

  useEffect(() => {
    if ((activeMode !== 'teller' && activeMode !== 'lore') || onImagePresetsChange || externalImagePresets.length > 0 || !workspace) return
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
  }, [activeMode, externalImagePresets.length, onImagePresetsChange, workspace])

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
    setImagePresets(externalImagePresets)
    setActiveImagePresetId((current) => {
      if (current && externalImagePresets.some((preset) => preset.id === current)) return current
      return externalImagePresets[0]?.id || ''
    })
    setImagePresetDraft(null)
    setImagePresetTagDraft('')
  }, [externalImagePresets, workspace])

  useEffect(() => {
    if (activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID) {
      setTellerDraft(null)
      setTellerTagDraft('')
      tellerBaseRevisionRef.current = ''
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
    tellerBaseRevisionRef.current = nextDraft?.updated_at || ''
    setActiveSlotId((current) => {
      if (current && teller?.slots?.some((slot) => slot.id === current)) return current
      return teller?.slots?.[0]?.id || ''
    })
    tellerSavedSignature.current = nextDraft ? tellerDraftSignature(nextDraft, (teller?.tags || []).join('，')) : ''
  }, [activeTellerId, tellers])

  useEffect(() => {
    const preset = imagePresets.find((entry) => entry.id === activeImagePresetId) || null
    const nextDraft = preset ? { ...preset, tags: [...(preset.tags || [])] } : null
    setImagePresetDraft(nextDraft)
    setImagePresetTagDraft((preset?.tags || []).join('，'))
    imagePresetBaseRevisionRef.current = nextDraft?.updated_at || ''
    imagePresetSavedSignature.current = nextDraft ? imagePresetDraftSignature(nextDraft, (preset?.tags || []).join('，')) : ''
  }, [activeImagePresetId, imagePresets])

  const refreshItems = async (nextActiveId?: string) => {
    const data = await getLoreItems()
    setItems(data)
    setActiveId(nextActiveId || LORE_CONFIG_AGENT_ENTRY_ID)
  }

  useEffect(() => {
    const onLoreUpdated = (event: Event) => {
      const detail = (event as CustomEvent<{ item_ids?: string[] }>).detail
      void refreshItems(detail?.item_ids?.[0])
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
      if (current === TELLER_CONFIG_AGENT_ENTRY_ID) return current
      if (current && data.some((teller) => teller.id === current)) return current
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

  const mergeSavedTeller = (teller: Teller) => {
    setTellers((current) => current.map((entry) => (entry.id === teller.id ? teller : entry)))
    onTellersChange?.(tellers.map((entry) => (entry.id === teller.id ? teller : entry)))
    setActiveTellerId(teller.id)
  }

  const mergeSavedImagePreset = (preset: ImagePreset) => {
    setImagePresets((current) => {
      const next = current.map((entry) => (entry.id === preset.id ? preset : entry))
      onImagePresetsChange?.(next)
      return next
    })
    setActiveImagePresetId(preset.id)
  }

  const mergeSavedLoreItem = (item: LoreItem) => {
    setItems((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
    if (loreDraftRef.current?.id === item.id) {
      const nextDraft = { ...item, tags: [...(item.tags || [])] }
      const nextTagDraft = (item.tags || []).join('，')
      setDraft(nextDraft)
      setTagDraft(nextTagDraft)
      loreBaseRevisionRef.current = item.updated_at || ''
      loreSavedSignature.current = loreDraftSignature(nextDraft, nextTagDraft)
    }
  }

  const saveLoreDraft = async (mode: 'manual' | 'auto') => {
    if (!draft) return null
    const payload = { ...draft, tags: splitTags(tagDraft) }
    const signature = loreDraftSignature(payload, tagDraft)
    if (mode === 'auto' && signature === loreSavedSignature.current) return null
    const item = await updateLoreItem(draft.id, payload, loreBaseRevisionRef.current)
    loreBaseRevisionRef.current = item.updated_at || ''
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
    const teller = await updateInteractiveTeller(tellerDraft.id, payload, tellerBaseRevisionRef.current)
    tellerBaseRevisionRef.current = teller.updated_at || ''
    tellerSavedSignature.current = tellerDraftSignature(teller, (teller.tags || []).join('，'))
    if (mode === 'manual') {
      mergeSavedTeller(teller)
    }
  }

  const saveImagePresetDraft = async (mode: 'manual' | 'auto') => {
    if (!imagePresetDraft) return
    const payload = {
      ...imagePresetDraft,
      tags: splitTags(imagePresetTagDraft),
    }
    const signature = imagePresetDraftSignature(payload, imagePresetTagDraft)
    if (mode === 'auto' && signature === imagePresetSavedSignature.current) return
    const preset = await updateImagePreset(imagePresetDraft.id, payload, imagePresetBaseRevisionRef.current)
    imagePresetBaseRevisionRef.current = preset.updated_at || ''
    imagePresetSavedSignature.current = imagePresetDraftSignature(preset, (preset.tags || []).join('，'))
    if (mode === 'manual') {
      mergeSavedImagePreset(preset)
    }
  }

  const handleCreateLore = async (section: KnowledgeSection = KNOWLEDGE_SECTIONS[0]) => {
    setSaving(true)
    try {
      const item = await createLoreItem({
        enabled: true,
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

  const handleDelete = async () => {
    if (activeMode === 'teller') {
      if (activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID) return
      if (presetResourceKind === 'image') {
        if (!imagePresetDraft?.custom) return
        if (!window.confirm(t('settingPanel.confirmDeleteImagePreset', { name: imagePresetDraft.name }))) return
        setSaving(true)
        try {
          if (imagePresetAutoSaveTimer.current) {
            window.clearTimeout(imagePresetAutoSaveTimer.current)
            imagePresetAutoSaveTimer.current = null
          }
          await deleteImagePreset(imagePresetDraft.id)
          await refreshImagePresets()
        } finally {
          setSaving(false)
        }
        return
      }
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
    setDeleteLoreTarget(draft)
  }

  const confirmDeleteLoreTarget = async () => {
    if (!deleteLoreTarget) return
    setSaving(true)
    try {
      if (loreAutoSaveTimer.current) {
        window.clearTimeout(loreAutoSaveTimer.current)
        loreAutoSaveTimer.current = null
      }
      await deleteLoreItem(deleteLoreTarget.id)
      await refreshItems()
      notifyLoreUpdated([deleteLoreTarget.id])
      setDeleteLoreTarget(null)
    } finally {
      setSaving(false)
    }
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      if (activeMode === 'creator' || (activeMode === 'lore' && activeId === CREATOR_ENTRY_ID)) {
        const result = await saveFile(CREATOR_PATH, creatorContent, creatorRevision)
        setCreatorRevision(result.revision || '')
        return
      }
      if (activeMode === 'lore' && activeId === INTERACTIVE_OPENING_PRESET_ENTRY_ID) {
        const result = await saveFile(INTERACTIVE_OPENING_PRESET_PATH, serializeBookOpeningPresets(openingPresets), openingPresetRevision)
        setOpeningPresetRevision(result.revision || '')
        notifyOpeningPresetUpdated()
        return
      }
      if (activeMode === 'teller') {
        if (presetResourceKind === 'image') {
          if (imagePresetAutoSaveTimer.current) {
            window.clearTimeout(imagePresetAutoSaveTimer.current)
            imagePresetAutoSaveTimer.current = null
          }
          await saveImagePresetDraft('manual')
        } else {
          if (tellerAutoSaveTimer.current) {
            window.clearTimeout(tellerAutoSaveTimer.current)
            tellerAutoSaveTimer.current = null
          }
          await saveTellerDraft('manual')
        }
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
    } catch (err) {
      toast.error((err as Error).message || t('editor.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  useEffect(() => {
    if (activeMode !== 'lore' || !draft || activeId === LORE_CONFIG_AGENT_ENTRY_ID) return
    const signature = loreDraftSignature(draft, tagDraft)
    if (signature === loreSavedSignature.current) return
    if (loreAutoSaveTimer.current) {
      window.clearTimeout(loreAutoSaveTimer.current)
    }
    loreAutoSaveTimer.current = window.setTimeout(() => {
      loreAutoSaveTimer.current = null
      void saveLoreDraft('auto').catch((err) => {
        console.warn('[lore-editor] 自动保存资料库条目失败', err)
        toast.error((err as Error).message || t('editor.saveFailed'))
      })
    }, 1200)
    return () => {
      if (loreAutoSaveTimer.current) {
        window.clearTimeout(loreAutoSaveTimer.current)
        loreAutoSaveTimer.current = null
      }
    }
  }, [activeMode, activeId, draft, tagDraft, t])

  useEffect(() => {
    if (activeMode !== 'teller' || !tellerDraft || activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID) return
    const signature = tellerDraftSignature(tellerDraft, tellerTagDraft)
    if (signature === tellerSavedSignature.current) return
    if (tellerAutoSaveTimer.current) {
      window.clearTimeout(tellerAutoSaveTimer.current)
    }
    tellerAutoSaveTimer.current = window.setTimeout(() => {
      tellerAutoSaveTimer.current = null
      void saveTellerDraft('auto').catch((err) => {
        console.warn('[teller-editor] 自动保存叙事方案失败', err)
        toast.error((err as Error).message || t('editor.saveFailed'))
      })
    }, 1200)
    return () => {
      if (tellerAutoSaveTimer.current) {
        window.clearTimeout(tellerAutoSaveTimer.current)
        tellerAutoSaveTimer.current = null
      }
    }
  }, [activeMode, activeTellerId, tellerDraft, tellerTagDraft, t])

  useEffect(() => {
    if (activeMode !== 'teller' || presetResourceKind !== 'image' || !imagePresetDraft) return
    const signature = imagePresetDraftSignature(imagePresetDraft, imagePresetTagDraft)
    if (signature === imagePresetSavedSignature.current) return
    if (imagePresetAutoSaveTimer.current) {
      window.clearTimeout(imagePresetAutoSaveTimer.current)
    }
    imagePresetAutoSaveTimer.current = window.setTimeout(() => {
      imagePresetAutoSaveTimer.current = null
      void saveImagePresetDraft('auto').catch((err) => {
        console.warn('[image-preset-editor] 自动保存图像方案失败', err)
        toast.error((err as Error).message || t('editor.saveFailed'))
      })
    }, 1200)
    return () => {
      if (imagePresetAutoSaveTimer.current) {
        window.clearTimeout(imagePresetAutoSaveTimer.current)
        imagePresetAutoSaveTimer.current = null
      }
    }
  }, [activeMode, activeImagePresetId, imagePresetDraft, imagePresetTagDraft, presetResourceKind, t])

  const flushImagePresetAutoSave = () => {
    if (!imagePresetAutoSaveTimer.current) return
    window.clearTimeout(imagePresetAutoSaveTimer.current)
    imagePresetAutoSaveTimer.current = null
    void saveImagePresetDraft('auto').catch((err) => {
      console.warn('[image-preset-editor] 切换条目前自动保存图像方案失败', err)
    })
  }

  const handlePresetResourceKindChange = (kind: PresetResourceKind) => {
    if (kind !== presetResourceKind && presetResourceKind === 'image') flushImagePresetAutoSave()
    setPresetResourceKind(kind)
    if (kind === 'image' && !activeImagePresetId && imagePresets[0]) setActiveImagePresetId(imagePresets[0].id)
    if (kind === 'teller' && !activeTellerId && tellers[0]) setActiveTellerId(tellers[0].id)
  }

  const handleSelectTeller = (id: string) => {
    if (presetResourceKind === 'image') flushImagePresetAutoSave()
    if (id !== TELLER_CONFIG_AGENT_ENTRY_ID) setPresetResourceKind('teller')
    setActiveTellerId(id)
  }

  const handleSelectImagePreset = (id: string) => {
    flushImagePresetAutoSave()
    setPresetResourceKind('image')
    setActiveTellerId((current) => current === TELLER_CONFIG_AGENT_ENTRY_ID ? '' : current)
    setActiveImagePresetId(id)
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

  const selectedLoreImagePresetId = () => activeImagePresetId || imagePresets.find((preset) => !preset.invalid)?.id || 'game-cg'

  const handleGenerateLoreImage = async () => {
    if (!draft || loreImageGeneratingId) return
    setLoreImageGeneratingId(draft.id)
    try {
      if (loreAutoSaveTimer.current) {
        window.clearTimeout(loreAutoSaveTimer.current)
        loreAutoSaveTimer.current = null
      }
      const saved = await saveLoreDraft('manual')
      const target = saved || loreDraftRef.current || draft
      const item = await generateLoreItemImage(target.id, {
        instruction: loreImageInstruction,
        image_preset_id: selectedLoreImagePresetId(),
      })
      mergeSavedLoreItem(item)
      notifyLoreUpdated([item.id])
      toast.success(t('settingPanel.loreImage.generated'))
    } catch (err) {
      toast.error((err as Error).message || t('settingPanel.loreImage.failed'))
    } finally {
      setLoreImageGeneratingId('')
    }
  }

  const handleClearLoreImage = async () => {
    if (!draft || loreImageGeneratingId) return
    setLoreImageGeneratingId(draft.id)
    try {
      if (loreAutoSaveTimer.current) {
        window.clearTimeout(loreAutoSaveTimer.current)
        loreAutoSaveTimer.current = null
      }
      const saved = await saveLoreDraft('manual')
      const target = saved || loreDraftRef.current || draft
      const item = await clearLoreItemImage(target.id)
      mergeSavedLoreItem(item)
      notifyLoreUpdated([item.id])
      toast.success(t('settingPanel.loreImage.cleared'))
    } catch (err) {
      toast.error((err as Error).message || t('settingPanel.loreImage.failed'))
    } finally {
      setLoreImageGeneratingId('')
    }
  }

  const handleOpenLoreImageBatch = () => {
    setLoreImageBatchSelectedIds([])
    setLoreImageBatchProgress({})
    setLoreImageBatchPresetId(selectedLoreImagePresetId())
    setLoreImageBatchOpen(true)
  }

  const handleRunLoreImageBatch = async () => {
    if (loreImageBatchSelectedIds.length === 0 || loreImageBatchRunning) {
      toast.error(t('settingPanel.loreImage.noSelection'))
      return
    }
    const controller = new AbortController()
    loreImageBatchAbortRef.current = controller
    setLoreImageBatchRunning(true)
    setLoreImageBatchProgress({})
    try {
      const stream = await streamLoreImagesGenerate({
        item_ids: loreImageBatchSelectedIds,
        instruction: loreImageBatchInstruction,
        overwrite_existing: loreImageBatchOverwrite,
        image_preset_id: loreImageBatchPresetId || selectedLoreImagePresetId(),
      }, controller.signal)
      const reader = stream.getReader()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        handleLoreImageBatchEvent(value)
      }
    } catch (err) {
      if (!isAbortError(err)) {
        toast.error((err as Error).message || t('settingPanel.loreImage.failed'))
      }
    } finally {
      loreImageBatchAbortRef.current = null
      setLoreImageBatchRunning(false)
    }
  }

  const handleLoreImageBatchEvent = (event: SSEEvent) => {
    if (event.event === 'lore_image_progress') {
      const progress = parseSSEData<LoreImageProgressEvent>(event)
      if (!progress?.item_id) return
      setLoreImageBatchProgress((current) => ({ ...current, [progress.item_id]: progress }))
      if (progress.item) mergeSavedLoreItem(progress.item)
      return
    }
    if (event.event === 'lore_image_result') {
      const result = parseSSEData<{ item?: LoreItem }>(event)
      if (result?.item) mergeSavedLoreItem(result.item)
      return
    }
    if (event.event === 'done') {
      const result = parseSSEData<{ generated?: number; skipped?: number; failed?: number }>(event)
      toast.success(t('settingPanel.loreImage.batchDone', {
        generated: result?.generated ?? 0,
        skipped: result?.skipped ?? 0,
        failed: result?.failed ?? 0,
      }))
      return
    }
    if (event.event === 'error') {
      const result = parseSSEData<{ message?: string }>(event)
      toast.error(result?.message || t('settingPanel.loreImage.failed'))
    }
  }

  const handleAbortLoreImageBatch = () => {
    void abortLoreImagesGenerate().catch((err) => {
      console.warn('[lore-image] 中止批量生成请求失败', err)
    })
    loreImageBatchAbortRef.current?.abort()
    loreImageBatchAbortRef.current = null
    setLoreImageBatchRunning(false)
  }

  useEffect(() => {
    return () => {
      loreImageBatchAbortRef.current?.abort()
      loreImageBatchAbortRef.current = null
    }
  }, [])

  const isCreatorActive = activeMode === 'creator' || (activeMode === 'lore' && activeId === CREATOR_ENTRY_ID)
  const isOpeningPresetActive = activeMode === 'lore' && activeId === INTERACTIVE_OPENING_PRESET_ENTRY_ID
  const isLoreConfigAgentActive = activeMode === 'lore' && activeId === LORE_CONFIG_AGENT_ENTRY_ID
  const isTellerConfigAgentActive = activeMode === 'teller' && activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID
  const isImagePresetEditorActive = activeMode === 'teller' && presetResourceKind === 'image'
  const directoryPanel = (
    <div className="nova-sidebar flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
      <div className="border-b border-[var(--nova-border)] px-3 py-3">
        <div className="flex items-center gap-2">
          <ModeIcon mode={activeMode} />
          <div className="text-sm font-semibold text-[var(--nova-text)]">{panelTitle(activeMode, t)}</div>
        </div>
        <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.directoryHint')}</div>
      </div>

      {activeMode === 'lore' ? <LoreDirectory items={items} activeId={activeId} query={query} saving={saving} onQueryChange={setQuery} onSelect={handleSelectLore} onCreate={(section) => void handleCreateLore(section)} onBatchGenerate={handleOpenLoreImageBatch} /> : activeMode === 'creator' ? <CreatorDirectory /> : <TellerDirectory resourceKind={presetResourceKind} tellers={tellers} imagePresets={imagePresets} activeTellerId={activeTellerId} activeImagePresetId={activeImagePresetId} saving={saving} onResourceKindChange={handlePresetResourceKindChange} onSelectTeller={handleSelectTeller} onSelectImagePreset={handleSelectImagePreset} onCreateTeller={() => void handleCreateTeller()} onCreateImagePreset={() => void handleCreateImagePreset()} />}
    </div>
  )
  return (
    <section className="h-full min-h-0 bg-[var(--nova-surface-2)] text-[var(--nova-text)]">
      <AdaptiveSurface
        left={{
          id: 'setting-directory',
          title: panelTitle(activeMode, t),
          side: 'left',
          icon: <ModeIcon mode={activeMode} />,
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
              <button type="button" className="nova-icon-button flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('workbench.mobile.openSidePanel', { label: panelTitle(activeMode, t) })} onClick={openLeft}>
                <PanelLeft className="h-4 w-4" />
              </button>
            )}
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              {isCreatorActive ? <BookMarked className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" /> : isOpeningPresetActive || isImagePresetEditorActive ? <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" /> : <ModeIcon mode={activeMode} />}
              <h2 className="truncate text-sm font-semibold text-[var(--nova-text)]">{isLoreConfigAgentActive ? t('settingPanel.loreAgent.title') : isTellerConfigAgentActive ? t('settingPanel.tellerAgent.title') : isCreatorActive ? CREATOR_PATH : isOpeningPresetActive ? t('settingPanel.openingPreset.title') : editorTitle(activeMode, draft, tellerDraft, imagePresetDraft, presetResourceKind, t)}</h2>
            </div>
            <p className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">{isLoreConfigAgentActive ? t('settingPanel.loreAgent.subtitle') : isTellerConfigAgentActive ? t('settingPanel.tellerAgent.subtitle') : isCreatorActive ? t('settingPanel.editor.creatorSubtitle') : isOpeningPresetActive ? t('settingPanel.openingPreset.subtitle') : editorSubtitle(activeMode, draft, tellerDraft, imagePresetDraft, presetResourceKind, t)}</p>
          </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {activeMode === 'lore' && !isLoreConfigAgentActive && !isCreatorActive && !isOpeningPresetActive && (
              <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || !draft} onClick={handleDelete} aria-label={t('settingPanel.deleteLore')}>
                <Trash2 className="h-4 w-4" />
              </Button>
            )}
            {activeMode === 'teller' && presetResourceKind === 'teller' && !isTellerConfigAgentActive && (
              <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || !tellerDraft?.custom} onClick={handleDelete} aria-label={t('settingPanel.deleteTeller')}>
                <Trash2 className="h-4 w-4" />
              </Button>
            )}
            {activeMode === 'teller' && presetResourceKind === 'image' && !isTellerConfigAgentActive && (
              <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || !imagePresetDraft?.custom} onClick={handleDelete} aria-label={t('settingPanel.deleteImagePreset')}>
                <Trash2 className="h-4 w-4" />
              </Button>
            )}
            {!isLoreConfigAgentActive && !isTellerConfigAgentActive && (
              <Button className={actionButtonClassName} variant="outline" size="sm" disabled={saving || (activeMode === 'lore' && !isCreatorActive && !isOpeningPresetActive && !draft) || (activeMode === 'teller' && presetResourceKind === 'teller' && !tellerDraft) || (activeMode === 'teller' && presetResourceKind === 'image' && !imagePresetDraft)} onClick={handleSave}>
                {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                {t('common.save')}
              </Button>
            )}
          </div>
        </div>

        {activeMode === 'lore' ? (
          <>
            {activeId === LORE_CONFIG_AGENT_ENTRY_ID ? (
              <ConfigManagerChat
                workspace={workspace}
                origin="lore"
                resourceId={LORE_CONFIG_AGENT_ENTRY_ID}
                context={{ item_count: String(items.length) }}
                onMutated={() => {
                  void refreshItems()
                  notifyLoreUpdated()
                }}
              />
            ) : activeId === CREATOR_ENTRY_ID ? (
              <CreatorEditor content={creatorContent} setContent={setCreatorContent} onSave={handleSave} />
            ) : activeId === INTERACTIVE_OPENING_PRESET_ENTRY_ID ? (
              <OpeningPresetEditor presets={openingPresets} activeId={activeOpeningPresetId} setActiveId={setActiveOpeningPresetId} setPresets={setOpeningPresets} onSave={handleSave} />
            ) : (
              <LoreEditor draft={draft} tagDraft={tagDraft} residentTotalChars={items.filter((item) => item.enabled !== false && item.load_mode === 'resident' && item.id !== draft?.id).reduce((total, item) => total + (item.content || '').length, draft?.enabled !== false && draft?.load_mode === 'resident' ? (draft.content || '').length : 0)} imagePresets={imagePresets} imagePresetId={activeImagePresetId || imagePresets.find((preset) => !preset.invalid)?.id || 'game-cg'} imageInstruction={loreImageInstruction} imageGenerating={loreImageGeneratingId === draft?.id} setDraft={setDraft} setTagDraft={setTagDraft} onImagePresetChange={setActiveImagePresetId} setImageInstruction={setLoreImageInstruction} onGenerateImage={() => void handleGenerateLoreImage()} onClearImage={() => void handleClearLoreImage()} onSave={handleSave} />
            )}
          </>
        ) : activeMode === 'creator' ? (
          <CreatorEditor content={creatorContent} setContent={setCreatorContent} onSave={handleSave} />
        ) : isTellerConfigAgentActive ? (
          <ConfigManagerChat
            workspace={workspace}
            origin="teller"
            resourceId={TELLER_CONFIG_AGENT_ENTRY_ID}
            context={{ teller_count: String(tellers.length), image_preset_count: String(imagePresets.length) }}
            onMutated={() => {
              void refreshTellers()
              void refreshImagePresets()
            }}
          />
        ) : activeMode === 'teller' && presetResourceKind === 'image' ? (
          <ImagePresetEditor draft={imagePresetDraft} setDraft={setImagePresetDraft} tagDraft={imagePresetTagDraft} setTagDraft={setImagePresetTagDraft} onSave={handleSave} />
        ) : (
          <TellerEditor workspace={workspace} draft={tellerDraft} setDraft={setTellerDraft} tagDraft={tellerTagDraft} setTagDraft={setTellerTagDraft} activeSlotId={activeSlotId} setActiveSlotId={setActiveSlotId} onSave={handleSave} />
        )}
      </main>
        )}
      </AdaptiveSurface>
      <LoreImageBatchDialog
        open={loreImageBatchOpen}
        items={items}
        query={loreImageBatchQuery}
        type={loreImageBatchType}
        selectedIds={loreImageBatchSelectedIds}
        imagePresets={imagePresets.filter((preset) => !preset.invalid)}
        imagePresetId={loreImageBatchPresetId || selectedLoreImagePresetId()}
        instruction={loreImageBatchInstruction}
        overwriteExisting={loreImageBatchOverwrite}
        progress={loreImageBatchProgress}
        running={loreImageBatchRunning}
        onOpenChange={setLoreImageBatchOpen}
        onQueryChange={setLoreImageBatchQuery}
        onTypeChange={setLoreImageBatchType}
        onSelectedIdsChange={setLoreImageBatchSelectedIds}
        onImagePresetChange={setLoreImageBatchPresetId}
        onInstructionChange={setLoreImageBatchInstruction}
        onOverwriteExistingChange={setLoreImageBatchOverwrite}
        onRun={() => void handleRunLoreImageBatch()}
        onAbort={handleAbortLoreImageBatch}
      />
      <AlertDialog open={Boolean(deleteLoreTarget)} onOpenChange={(open) => {
        if (!open && !saving) setDeleteLoreTarget(null)
      }}>
        <AlertDialogContent className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
          <AlertDialogHeader>
            <AlertDialogTitle>{t('settingPanel.deleteLore')}</AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--nova-text-muted)]">
              {t('settingPanel.confirmDeleteLore', { name: deleteLoreTarget?.name || '' })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={saving}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-[var(--nova-danger-bg)] text-[var(--nova-danger)] hover:bg-[var(--nova-danger-bg)]"
              disabled={saving || !deleteLoreTarget}
              onClick={(event) => {
                event.preventDefault()
                void confirmDeleteLoreTarget()
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

interface LoreImageBatchDialogProps {
  open: boolean
  items: LoreItem[]
  query: string
  type: LoreType | 'all'
  selectedIds: string[]
  imagePresets: ImagePreset[]
  imagePresetId: string
  instruction: string
  overwriteExisting: boolean
  progress: Record<string, LoreImageProgressEvent>
  running: boolean
  onOpenChange: (open: boolean) => void
  onQueryChange: (value: string) => void
  onTypeChange: (value: LoreType | 'all') => void
  onSelectedIdsChange: (ids: string[]) => void
  onImagePresetChange: (id: string) => void
  onInstructionChange: (value: string) => void
  onOverwriteExistingChange: (value: boolean) => void
  onRun: () => void
  onAbort: () => void
}

function LoreImageBatchDialog({
  open,
  items,
  query,
  type,
  selectedIds,
  imagePresets,
  imagePresetId,
  instruction,
  overwriteExisting,
  progress,
  running,
  onOpenChange,
  onQueryChange,
  onTypeChange,
  onSelectedIdsChange,
  onImagePresetChange,
  onInstructionChange,
  onOverwriteExistingChange,
  onRun,
  onAbort,
}: LoreImageBatchDialogProps) {
  const { t } = useTranslation()
  const selectedSet = new Set(selectedIds)
  const filteredItems = filterLoreImageBatchItems(items, query, type)

  const toggleSelected = (id: string) => {
    if (running) return
    onSelectedIdsChange(selectedSet.has(id) ? selectedIds.filter((entry) => entry !== id) : [...selectedIds, id])
  }

  const selectVisible = () => {
    if (running) return
    const next = new Set(selectedIds)
    filteredItems.forEach((item) => next.add(item.id))
    onSelectedIdsChange(Array.from(next))
  }

  const clearSelection = () => {
    if (!running) onSelectedIdsChange([])
  }

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => {
      if (running && !nextOpen) return
      onOpenChange(nextOpen)
    }}>
      <DialogContent className="max-w-[min(calc(100vw-2rem),760px)] gap-3 border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
        <DialogHeader>
          <DialogTitle>{t('settingPanel.loreImage.batchTitle')}</DialogTitle>
          <DialogDescription>{t('settingPanel.loreImage.batchDesc')}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_180px]">
          <div className="nova-field flex h-8 items-center gap-2 rounded-[var(--nova-radius)] px-2 text-xs text-[var(--nova-text-faint)]">
            <Search className="h-3.5 w-3.5" />
            <input
              className="min-w-0 flex-1 bg-transparent text-[var(--nova-text-muted)] outline-none placeholder:text-[var(--nova-text-faint)]"
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
              placeholder={t('settingPanel.loreImage.search')}
              disabled={running}
            />
          </div>
          <Select value={type} onValueChange={(value) => onTypeChange(value as LoreType | 'all')} disabled={running}>
            <SelectTrigger size="sm" className="nova-field h-8 text-xs focus:ring-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectItem value="all">{t('settingPanel.loreImage.typeAll')}</SelectItem>
              {LORE_TYPE_FILTER_OPTIONS.map((option) => (
                <SelectItem key={option} value={option}>{loreTypeLabel(option, t)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="text-xs text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.selectedCount', { count: selectedIds.length })}</div>
          <div className="flex items-center gap-2">
            <Button className={actionButtonClassName} variant="outline" size="sm" disabled={running || filteredItems.length === 0} onClick={selectVisible}>
              {t('settingPanel.loreImage.selectVisible')}
            </Button>
            <Button className={actionButtonClassName} variant="outline" size="sm" disabled={running || selectedIds.length === 0} onClick={clearSelection}>
              {t('settingPanel.loreImage.clearSelection')}
            </Button>
          </div>
        </div>

        <ScrollArea className="h-[min(42vh,360px)] rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <div className="divide-y divide-[var(--nova-border)]">
            {filteredItems.length === 0 ? (
              <div className="px-3 py-8 text-center text-xs text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.noItems')}</div>
            ) : filteredItems.map((item) => {
              const status = progress[item.id]
              return (
                <label key={item.id} className="flex min-h-16 cursor-pointer items-center gap-3 px-3 py-2 text-xs hover:bg-[var(--nova-hover)]">
                  <input
                    type="checkbox"
                    className="h-4 w-4 accent-[var(--nova-accent)]"
                    checked={selectedSet.has(item.id)}
                    disabled={running}
                    onChange={() => toggleSelected(item.id)}
                    aria-label={item.name}
                  />
                  <LoreImageBatchThumb item={item} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium text-[var(--nova-text)]">{item.name}</span>
                    <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]">{loreTypeLabel(item.type, t)} · {item.brief_description || t('settingPanel.loreImage.missingImage')}</span>
                  </span>
                  <span className={`shrink-0 rounded-full border px-2 py-1 text-[10px] ${loreImageStatusClassName(status?.status, item)}`}>
                    {loreImageStatusLabel(status, item, t)}
                  </span>
                </label>
              )
            })}
          </div>
        </ScrollArea>

        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_220px]">
          <label className="grid gap-1.5">
            <span className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.instruction')}</span>
            <Textarea
              className="nova-field min-h-20 resize-y text-xs leading-5 shadow-none focus-visible:ring-0"
              value={instruction}
              onChange={(event) => onInstructionChange(event.target.value)}
              placeholder={t('settingPanel.loreImage.instructionPlaceholder')}
              disabled={running}
            />
          </label>
          <div className="grid content-start gap-3">
            <label className="grid gap-1.5">
              <span className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.preset')}</span>
              <Select value={imagePresetId} onValueChange={onImagePresetChange} disabled={running}>
                <SelectTrigger size="sm" className="nova-field h-8 text-xs focus:ring-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  {imagePresets.length > 0 ? imagePresets.map((preset) => (
                    <SelectItem key={preset.id} value={preset.id}>{preset.name}</SelectItem>
                  )) : (
                    <SelectItem value="game-cg">{t('settingPanel.editor.defaultImagePreset')}</SelectItem>
                  )}
                </SelectContent>
              </Select>
            </label>
            <div className="flex items-center justify-between gap-3 rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2">
              <span className="min-w-0 text-xs text-[var(--nova-text-muted)]">{t('settingPanel.loreImage.overwriteExisting')}</span>
              <Switch checked={overwriteExisting} onCheckedChange={onOverwriteExistingChange} disabled={running} />
            </div>
          </div>
        </div>

        <DialogFooter className="border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <Button className={actionButtonClassName} variant="outline" size="sm" disabled={running} onClick={() => onOpenChange(false)}>
            {t('common.close')}
          </Button>
          {running ? (
            <Button className={actionButtonClassName} variant="outline" size="sm" onClick={onAbort}>
              {t('settingPanel.loreImage.abortBatch')}
            </Button>
          ) : (
            <Button className={actionButtonClassName} variant="outline" size="sm" disabled={selectedIds.length === 0} onClick={onRun}>
              <Sparkles className="h-4 w-4" />
              {t('settingPanel.loreImage.startBatch')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function LoreImageBatchThumb({ item }: { item: LoreItem }) {
  const imagePath = item.image?.image_path || ''
  if (!imagePath) {
    return (
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]">
        <ImageIcon className="h-4 w-4" />
      </span>
    )
  }
  return (
    <span className="h-10 w-10 shrink-0 overflow-hidden rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <img src={workspaceAssetURL(imagePath)} alt="" className="h-full w-full object-cover" />
    </span>
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

function filterLoreImageBatchItems(items: LoreItem[], query: string, type: LoreType | 'all') {
  const normalizedQuery = query.trim().toLowerCase()
  return items.filter((item) => {
    if (type !== 'all' && item.type !== type) return false
    if (!normalizedQuery) return true
    const haystack = [item.name, item.brief_description || '', item.content || '', (item.tags || []).join('\n')].join('\n').toLowerCase()
    return haystack.includes(normalizedQuery)
  })
}

function loreImageStatusLabel(progress: LoreImageProgressEvent | undefined, item: LoreItem, t: (key: string) => string) {
  if (progress?.status) return t(`settingPanel.loreImage.status.${progress.status}`)
  return item.image?.image_path ? t('settingPanel.loreImage.hasImage') : t('settingPanel.loreImage.missingImage')
}

function loreImageStatusClassName(status: LoreImageProgressEvent['status'] | undefined, item: LoreItem) {
  if (status === 'running') return 'border-[var(--nova-accent)]/45 bg-[var(--nova-accent)]/15 text-[var(--nova-text)]'
  if (status === 'success') return 'border-[var(--nova-accent-green)]/45 bg-[var(--nova-accent-green)]/15 text-[var(--nova-text)]'
  if (status === 'error') return 'border-[var(--nova-danger)]/45 bg-[var(--nova-danger)]/10 text-[var(--nova-danger)]'
  if (status === 'skipped') return 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]'
  if (item.image?.image_path) return 'border-[var(--nova-accent-green)]/35 bg-[var(--nova-accent-green)]/10 text-[var(--nova-text-muted)]'
  return 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]'
}

function parseSSEData<T>(event: SSEEvent): T | null {
  if (!event.data) return null
  try {
    return JSON.parse(event.data) as T
  } catch (err) {
    console.warn('[lore-image] SSE 数据解析失败', event.event, err)
    return null
  }
}

function isAbortError(err: unknown) {
  if (typeof DOMException !== 'undefined' && err instanceof DOMException && err.name === 'AbortError') return true
  return err instanceof Error && err.name === 'AbortError'
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

function notifyOpeningPresetUpdated() {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(INTERACTIVE_OPENING_PRESET_UPDATED_EVENT))
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

function editorTitle(mode: SettingPanelMode, draft: LoreItem | null, tellerDraft: Teller | null, imagePresetDraft: ImagePreset | null, presetResourceKind: PresetResourceKind, t: (key: string) => string) {
  if (mode === 'creator') return CREATOR_PATH
  if (mode === 'teller' && presetResourceKind === 'image') return imagePresetDraft?.name || t('settingPanel.editor.defaultImagePreset')
  if (mode === 'teller') return tellerDraft?.name || t('settingPanel.editor.defaultTeller')
  return draft?.name || t('settingPanel.mode.lore')
}

function editorSubtitle(mode: SettingPanelMode, draft: LoreItem | null, tellerDraft: Teller | null, imagePresetDraft: ImagePreset | null, presetResourceKind: PresetResourceKind, t: (key: string) => string) {
  if (mode === 'creator') return t('settingPanel.editor.creatorSubtitle')
  if (mode === 'teller' && presetResourceKind === 'image') return imagePresetDraft?.description || t('settingPanel.editor.imagePresetSubtitle')
  if (mode === 'teller') return tellerDraft?.description || t('settingPanel.editor.tellerSubtitle')
  if (!draft) return t('settingPanel.editor.loreSubtitle')
  return `${draft.enabled === false ? t('settingPanel.disabled') : t('settingPanel.enabled')} · ${loreTypeLabel(draft.type, t)} · ${loreImportanceLabel(draft.importance, t)} · ${loreLoadModeLabel(draft.load_mode, t)} · ${(draft.tags || []).join('，') || t('settingPanel.editor.noTags')}`
}

function newTellerDraft(): Partial<Teller> {
  const id = `custom-${Date.now()}`
  return {
    id,
    name: '自定义叙事',
    description: '新的叙事方案',
    random_event_rate: 0.15,
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
        content: '你是一套自定义叙事方案。你要明确影响故事的题材倾向、角色反应、剧情裁定、节奏推进和长期叙事原则。',
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

function newImagePresetDraft(): Partial<ImagePreset> {
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

function imagePresetDraftSignature(preset: Partial<ImagePreset>, tagDraft: string) {
  return JSON.stringify({
    ...preset,
    tags: splitTags(tagDraft),
  })
}
