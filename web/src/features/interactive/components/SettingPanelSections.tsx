import { useState, type ReactNode } from 'react'
import { BookMarked, Bot, Building2, ChevronDown, FileText, Folder, Library, MapPin, Plus, ScrollText, Search, SlidersHorizontal, Sparkles, Trash2, UserRound } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isSaveShortcut } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { type LoreItem } from '@/lib/api'
import { INTERACTIVE_OPENING_PRESET_ENTRY_ID, newBookOpeningPreset, type BookOpeningPreset } from '../opening'
import type { ImagePreset, Teller } from '../types'

const CREATOR_PATH = 'CREATOR.md'
const CREATOR_ENTRY_ID = '__creator__'
const LORE_CONFIG_AGENT_ENTRY_ID = '__config_manager_lore__'
const TELLER_CONFIG_AGENT_ENTRY_ID = '__config_manager_teller__'
const TYPE_OPTIONS = [
  { value: 'character' },
  { value: 'world' },
  { value: 'location' },
  { value: 'faction' },
  { value: 'rule' },
  { value: 'item' },
  { value: 'other' },
] as const
const IMPORTANCE_OPTIONS = [
  { value: 'major' },
  { value: 'important' },
  { value: 'minor' },
] as const
const LOAD_MODE_OPTIONS = [
  { value: 'resident' },
  { value: 'auto' },
  { value: 'manual' },
] as const
const LORE_RESIDENT_ITEM_WARNING_CHARS = 8000
const LORE_RESIDENT_TOTAL_WARNING_CHARS = 40000
const IMAGE_PRESET_PROMPT_LIMIT = 4000
type PresetResourceKind = 'teller' | 'image'
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
  { id: 'characters', labelKey: 'lore.type.character', icon: UserRound, types: ['character'], createType: 'character', createName: '新角色' },
  { id: 'locations', labelKey: 'lore.type.location', icon: MapPin, types: ['location'], createType: 'location', createName: '新地点' },
  { id: 'factions', labelKey: 'lore.type.faction', icon: Building2, types: ['faction'], createType: 'faction', createName: '新组织' },
  { id: 'rules', labelKey: 'lore.type.rule', icon: ScrollText, types: ['world', 'rule'], createType: 'rule', createName: '新规则' },
  { id: 'templates', labelKey: 'settingPanel.section.templates', icon: FileText, types: ['other'], createType: 'other', createName: '新模板', tag: '模板' },
  { id: 'assets', labelKey: 'settingPanel.section.assets', icon: Library, types: ['item', 'other'], createType: 'item', createName: '新素材', excludeTag: '模板' },
]

const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
const selectClassName = 'nova-field h-8 text-xs focus:ring-0'

export function LoreDirectory({
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
  const { t } = useTranslation()
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
            placeholder={t('settingPanel.searchLore')}
          />
        </div>
        <button
          type="button"
          onClick={() => onSelect(CREATOR_ENTRY_ID)}
          className={`mt-2 flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeId === CREATOR_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
          }`}
        >
          <BookMarked className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 flex-1 truncate">{CREATOR_PATH}</span>
        </button>
        <button
          type="button"
          onClick={() => onSelect(INTERACTIVE_OPENING_PRESET_ENTRY_ID)}
          className={`mt-2 flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeId === INTERACTIVE_OPENING_PRESET_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
          }`}
        >
          <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 flex-1 truncate">{t('settingPanel.openingPreset.title')}</span>
        </button>
        <button
          type="button"
          onClick={() => onSelect(LORE_CONFIG_AGENT_ENTRY_ID)}
          className={`mt-2 flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeId === LORE_CONFIG_AGENT_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
          }`}
        >
          <Bot className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 flex-1 truncate">{t('settingPanel.loreAgent.title')}</span>
        </button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-2">
          {sections.map(({ section, entries }) => {
            const Icon = section.icon
            const collapsed = isCollapsed(section, entries)
            const label = t(section.labelKey)
            return (
              <section key={section.id} className={entries.length ? 'mb-2' : 'mb-1'}>
                <div className={`flex h-8 items-center gap-2 rounded px-2 text-xs ${entries.length ? 'text-[var(--nova-text-muted)]' : 'text-[var(--nova-text-faint)]'}`}>
                  <button
                    type="button"
                    className="nova-nav-item rounded p-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    onClick={() => toggleSection(section, entries)}
                    aria-label={collapsed ? `${t('chat.tool.expand')}${label}` : `${t('chat.tool.collapse')}${label}`}
                  >
                    <ChevronDown className={`h-3.5 w-3.5 transition-transform ${collapsed ? '-rotate-90' : ''}`} />
                  </button>
                  <Icon className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
                  <span className="min-w-0 flex-1 truncate font-medium">{label}</span>
                  <span className="text-[11px] text-[var(--nova-text-faint)]">{entries.length}</span>
                  <button
                    type="button"
                    className="nova-nav-item rounded p-1 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    disabled={saving}
                    onClick={() => onCreate(section)}
                    aria-label={`${t('chat.new')}${label}`}
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
                        } ${item.enabled === false ? 'opacity-50' : ''}`}
                      >
                        <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                        <span className="min-w-0 flex-1 truncate">{item.name}</span>
                        {item.enabled === false ? <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{t('settingPanel.disabled')}</span> : null}
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

export function CreatorDirectory() {
  const { t } = useTranslation()
  return (
    <div className="p-2">
      <div className="flex h-8 items-center gap-2 rounded px-2 text-xs text-[var(--nova-text-muted)]">
        <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        <Folder className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        <span className="font-medium">{t('settingPanel.rootDirectory')}</span>
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

export function TellerDirectory({
  resourceKind,
  tellers,
  imagePresets,
  activeTellerId,
  activeImagePresetId,
  saving,
  onResourceKindChange,
  onSelectTeller,
  onSelectImagePreset,
  onCreateTeller,
  onCreateImagePreset,
}: {
  resourceKind: PresetResourceKind
  tellers: Teller[]
  imagePresets: ImagePreset[]
  activeTellerId: string
  activeImagePresetId: string
  saving: boolean
  onResourceKindChange: (kind: PresetResourceKind) => void
  onSelectTeller: (id: string) => void
  onSelectImagePreset: (id: string) => void
  onCreateTeller: () => void
  onCreateImagePreset: () => void
}) {
  const { t } = useTranslation()
  const createLabel = resourceKind === 'image' ? t('settingPanel.newImagePreset') : t('settingPanel.newTeller')
  return (
    <>
      <div className="flex min-h-12 items-center justify-between gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        <div className="grid min-w-0 flex-1 grid-cols-2 gap-1 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0.5">
          {(['teller', 'image'] as const).map((kind) => (
            <button
              key={kind}
              type="button"
              onClick={() => onResourceKindChange(kind)}
              className={`h-7 min-w-0 truncate rounded-[6px] px-2 text-[11px] transition ${
                resourceKind === kind ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'
              }`}
            >
              {kind === 'image' ? t('settingPanel.presetKind.image') : t('settingPanel.presetKind.teller')}
            </button>
          ))}
        </div>
        <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving} onClick={resourceKind === 'image' ? onCreateImagePreset : onCreateTeller} aria-label={createLabel}>
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>
      <div className="border-b border-[var(--nova-border)] p-2">
        <button
          type="button"
          onClick={() => onSelectTeller(TELLER_CONFIG_AGENT_ENTRY_ID)}
          className={`flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
          }`}
        >
          <Bot className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 flex-1 truncate">{t('settingPanel.tellerAgent.title')}</span>
        </button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-2">
          <div className="flex h-8 items-center gap-2 rounded px-2 text-xs text-[var(--nova-text-muted)]">
            <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
            <Folder className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
            <span className="font-medium">{resourceKind === 'image' ? t('settingPanel.imagePresetDirectory') : t('settingPanel.rulePackages')}</span>
          </div>
          <div className="ml-5 space-y-0.5 border-l border-[var(--nova-border)] pl-2">
            {resourceKind === 'image' ? imagePresets.map((preset) => (
              <button
                key={preset.id}
                type="button"
                onClick={() => onSelectImagePreset(preset.id)}
                className={`flex min-h-9 w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs transition ${
                  activeTellerId !== TELLER_CONFIG_AGENT_ENTRY_ID && activeImagePresetId === preset.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                }`}
              >
                <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{preset.name}</span>
                  <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{preset.custom ? t('settingPanel.custom') : t('settingPanel.builtIn')} · {t('settingPanel.imagePreset.promptChars', { count: (preset.prompt || '').length })}</span>
                </span>
              </button>
            )) : tellers.map((teller) => (
              <button
                key={teller.id}
                type="button"
                onClick={() => onSelectTeller(teller.id)}
                className={`flex min-h-9 w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs transition ${
                  activeTellerId !== TELLER_CONFIG_AGENT_ENTRY_ID && activeTellerId === teller.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                }`}
              >
                <SlidersHorizontal className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{teller.name}</span>
                  <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{teller.custom ? t('settingPanel.custom') : t('settingPanel.builtIn')} · {t('settingPanel.enabledRules', { count: (teller.slots || []).filter((slot) => slot.enabled).length })}</span>
                </span>
              </button>
            ))}
          </div>
        </div>
      </ScrollArea>
    </>
  )
}

export function ImagePresetEditor({
  draft,
  tagDraft,
  setDraft,
  setTagDraft,
  onSave,
}: {
  draft: ImagePreset | null
  tagDraft: string
  setDraft: (draft: ImagePreset | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noImagePresetSelected')} description={t('settingPanel.editor.noImagePresetSelectedDesc')} />
  }

  const promptValue = draft.prompt || ''

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden">
      <div className="grid shrink-0 gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,1fr)_180px_120px]">
        <Field label={t('settingPanel.field.name')}>
          <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
        </Field>
        <Field label={t('settingPanel.field.description')}>
          <Input className={inputClassName} value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} placeholder={t('settingPanel.placeholder.description')} />
        </Field>
        <Field label={t('settingPanel.field.tags')}>
          <Input className={inputClassName} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder={t('settingPanel.placeholder.tags')} />
        </Field>
        <div className="flex items-end">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs text-[var(--nova-text-faint)]">{draft.custom ? t('settingPanel.custom') : t('settingPanel.builtIn')}</span>
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-surface)] p-4">
        <div className="mb-2 flex min-w-0 items-end justify-between gap-3">
          <div className="min-w-0">
            <div className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.imagePreset.promptTitle')}</div>
            <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.imagePreset.promptDesc', { count: IMAGE_PRESET_PROMPT_LIMIT })}</div>
          </div>
          <span className="shrink-0 font-mono text-[10px] text-[var(--nova-text-faint)]">{promptValue.length}/{IMAGE_PRESET_PROMPT_LIMIT}</span>
        </div>
        <Textarea
          className="nova-field min-h-[420px] flex-1 resize-none text-sm leading-7 shadow-none focus-visible:ring-0"
          value={promptValue}
          maxLength={IMAGE_PRESET_PROMPT_LIMIT}
          onChange={(event) => setDraft({ ...draft, prompt: event.target.value.slice(0, IMAGE_PRESET_PROMPT_LIMIT) })}
          placeholder={t('settingPanel.imagePreset.promptPlaceholder')}
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

export function LoreEditor({
  draft,
  tagDraft,
  residentTotalChars,
  setDraft,
  setTagDraft,
  onSave,
}: {
  draft: LoreItem | null
  tagDraft: string
  residentTotalChars: number
  setDraft: (draft: LoreItem | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noLoreSelected')} description={t('settingPanel.editor.noLoreSelectedDesc')} />
  }

  const residentItemChars = draft.enabled !== false && draft.load_mode === 'resident' ? (draft.content || '').length : 0
  const residentWarning = draft.enabled !== false && draft.load_mode === 'resident' && (residentItemChars > LORE_RESIDENT_ITEM_WARNING_CHARS || residentTotalChars > LORE_RESIDENT_TOTAL_WARNING_CHARS)

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto md:overflow-hidden">
      <div className="grid shrink-0 gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 lg:grid-cols-[minmax(220px,1fr)_120px_150px_150px_170px]">
        <Field label={t('settingPanel.field.name')}>
          <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
        </Field>
        <Field label={t('settingPanel.field.enabled')}>
          <Select value={String(draft.enabled ?? true)} onValueChange={(value) => setDraft({ ...draft, enabled: value === 'true' })}>
            <SelectTrigger size="sm" className={selectClassName}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectItem value="true">{t('settingPanel.enabled')}</SelectItem>
              <SelectItem value="false">{t('settingPanel.disabled')}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field label={t('settingPanel.field.type')}>
          <Select value={draft.type} onValueChange={(value) => setDraft({ ...draft, type: value as LoreItem['type'] })}>
            <SelectTrigger size="sm" className={selectClassName}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              {TYPE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>{loreTypeLabel(option.value, t)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label={t('settingPanel.field.importance')}>
          <Select value={draft.importance} onValueChange={(value) => setDraft({ ...draft, importance: value as LoreItem['importance'] })}>
            <SelectTrigger size="sm" className={selectClassName}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              {IMPORTANCE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>{loreImportanceLabel(option.value, t)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label={t('settingPanel.field.loadMode')}>
          <Select value={draft.load_mode || 'auto'} onValueChange={(value) => setDraft({ ...draft, load_mode: value as LoreItem['load_mode'] })}>
            <SelectTrigger size="sm" className={selectClassName}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              {LOAD_MODE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>{loreLoadModeLabel(option.value, t)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label={t('settingPanel.field.tags')}>
          <Input className={inputClassName} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder={t('settingPanel.placeholder.tags')} />
        </Field>
        <Field className="lg:col-span-5" label={t('settingPanel.field.brief')}>
          <Textarea
            autoResize
            className="nova-field min-h-[96px] resize-y text-xs leading-5 shadow-none focus-visible:ring-0"
            value={draft.brief_description || ''}
            onChange={(event) => setDraft({ ...draft, brief_description: event.target.value })}
            placeholder={t('settingPanel.placeholder.brief')}
          />
        </Field>
        <div className="lg:col-span-5 text-[11px] leading-5 text-[var(--nova-text-faint)]">
          {draft.load_mode === 'resident' ? t('settingPanel.lore.residentDesc') : loadModeDescription(draft.load_mode, t)}
          {residentWarning ? <span className="ml-2 text-[var(--nova-danger)]">{t('settingPanel.lore.residentWarning')}</span> : null}
        </div>
      </div>
      <div className="min-h-[420px] flex-1 p-4 md:min-h-0">
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

export function CreatorEditor({
  content,
  setContent,
  onSave,
}: {
  content: string
  setContent: (value: string) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-4">
      <Textarea
        className="nova-field h-full min-h-[520px] resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
        value={content}
        onChange={(event) => setContent(event.target.value)}
        placeholder={t('settingPanel.placeholder.creator')}
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

export function OpeningPresetEditor({
  presets,
  activeId,
  setActiveId,
  setPresets,
  onSave,
}: {
  presets: BookOpeningPreset[]
  activeId: string
  setActiveId: (id: string) => void
  setPresets: (presets: BookOpeningPreset[]) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  const activePreset = presets.find((preset) => preset.id === activeId) || presets[0] || null
  const updateActivePreset = (patch: Partial<BookOpeningPreset>) => {
    if (!activePreset) return
    setPresets(presets.map((preset) => (preset.id === activePreset.id ? { ...preset, ...patch } : preset)))
  }
  const addPreset = () => {
    const preset = newBookOpeningPreset(t('settingPanel.openingPreset.defaultName', { number: presets.length + 1 }))
    setPresets([...presets, preset])
    setActiveId(preset.id)
  }
  const deleteActivePreset = () => {
    if (!activePreset) return
    const nextPresets = presets.filter((preset) => preset.id !== activePreset.id)
    setPresets(nextPresets)
    setActiveId(nextPresets[0]?.id || '')
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto md:overflow-hidden">
      <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.openingPreset.title')}</div>
            <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.openingPreset.description')}</div>
          </div>
          <Button className={iconActionClassName} variant="outline" size="sm" onClick={addPreset}>
            <Plus className="h-3.5 w-3.5" />
            {t('settingPanel.openingPreset.add')}
          </Button>
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <aside className="max-h-48 shrink-0 overflow-y-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-2 md:max-h-none md:w-56 md:border-b-0 md:border-r">
          {presets.length === 0 ? (
            <div className="px-2 py-3 text-xs leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.openingPreset.empty')}</div>
          ) : (
            <div className="space-y-1">
              {presets.map((preset) => (
                <button
                  key={preset.id}
                  type="button"
                  onClick={() => setActiveId(preset.id)}
                  className={`flex min-h-8 w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs transition ${
                    activePreset?.id === preset.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                  }`}
                >
                  <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                  <span className="min-w-0 flex-1 truncate">{preset.title || t('settingPanel.openingPreset.untitled')}</span>
                </button>
              ))}
            </div>
          )}
        </aside>
        <div className="min-h-[420px] flex-1 p-4 md:min-h-0">
          {activePreset ? (
            <div className="flex h-full min-h-0 flex-col gap-3">
              <div className="flex items-end gap-3">
                <Field className="min-w-0 flex-1" label={t('settingPanel.openingPreset.name')}>
                  <Input className={inputClassName} value={activePreset.title} onChange={(event) => updateActivePreset({ title: event.target.value })} placeholder={t('settingPanel.openingPreset.untitled')} />
                </Field>
                <Button className={iconActionClassName} variant="outline" size="icon" onClick={deleteActivePreset} aria-label={t('settingPanel.openingPreset.delete')}>
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
              <Textarea
                className="nova-field min-h-0 flex-1 resize-none text-sm leading-7 shadow-none focus-visible:ring-0"
                value={activePreset.content}
                onChange={(event) => updateActivePreset({ content: event.target.value })}
                placeholder={t('settingPanel.openingPreset.placeholder')}
                onKeyDown={(event) => {
                  if (isSaveShortcut(event)) {
                    event.preventDefault()
                    event.stopPropagation()
                    onSave()
                  }
                }}
              />
            </div>
          ) : (
            <EmptyState title={t('settingPanel.openingPreset.emptyTitle')} description={t('settingPanel.openingPreset.emptyDesc')} />
          )}
        </div>
      </div>
    </div>
  )
}

function Field({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return (
    <label className={`grid gap-1.5 ${className}`}>
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

function sectionItems(items: LoreItem[], section: KnowledgeSection, query = '') {
  const normalizedQuery = query.trim().toLowerCase()
  return items.filter((item) => {
    if (!section.types.includes(item.type)) return false
    const tags = item.tags || []
    if (section.tag && !tags.includes(section.tag)) return false
    if (section.excludeTag && tags.includes(section.excludeTag)) return false
    if (normalizedQuery) {
      const haystack = [item.name, item.brief_description || '', item.content || '', tags.join('\n')].join('\n').toLowerCase()
      if (!haystack.includes(normalizedQuery)) return false
    }
    return true
  })
}

function loadModeDescription(loadMode: LoreItem['load_mode'] | undefined, t: (key: string) => string) {
  if (loadMode === 'resident') return t('settingPanel.lore.residentDesc')
  if (loadMode === 'manual') return t('settingPanel.lore.manualDesc')
  if (loadMode === 'auto') return t('settingPanel.lore.autoDesc')
  return t('settingPanel.lore.indexDesc')
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
