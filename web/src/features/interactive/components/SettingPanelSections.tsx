import { useState, type ReactNode } from 'react'
import { BookMarked, Bot, Building2, ChevronDown, FileText, Folder, Library, MapPin, Plus, ScrollText, Search, SlidersHorizontal, UserRound } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isSaveShortcut } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { type LoreItem } from '@/lib/api'
import type { Teller } from '../types'

const CREATOR_PATH = 'CREATOR.md'
const CREATOR_ENTRY_ID = '__creator__'
const LORE_AGENT_ENTRY_ID = '__lore_agent__'
const TELLER_AGENT_ENTRY_ID = '__teller_agent__'
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
          onClick={() => onSelect(LORE_AGENT_ENTRY_ID)}
          className={`mt-2 flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeId === LORE_AGENT_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
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
  const { t } = useTranslation()
  return (
    <>
      <div className="flex h-10 items-center justify-between border-b border-[var(--nova-border)] px-3">
        <div className="text-xs font-medium text-[var(--nova-text-muted)]">{t('settingPanel.tellerDirectory')}</div>
        <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving} onClick={onCreate} aria-label={t('settingPanel.newTeller')}>
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
          <span className="min-w-0 flex-1 truncate">{t('settingPanel.tellerAgent.title')}</span>
        </button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-2">
          <div className="flex h-8 items-center gap-2 rounded px-2 text-xs text-[var(--nova-text-muted)]">
            <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
            <Folder className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
            <span className="font-medium">{t('settingPanel.rulePackages')}</span>
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

  const residentItemChars = draft.load_mode === 'resident' ? (draft.content || '').length : 0
  const residentWarning = draft.load_mode === 'resident' && (residentItemChars > LORE_RESIDENT_ITEM_WARNING_CHARS || residentTotalChars > LORE_RESIDENT_TOTAL_WARNING_CHARS)

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="grid shrink-0 gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 lg:grid-cols-[minmax(220px,1fr)_160px_160px_180px]">
        <Field label={t('settingPanel.field.name')}>
          <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
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
        <Field className="lg:col-span-4" label={t('settingPanel.field.brief')}>
          <Textarea
            autoResize
            className="nova-field min-h-[96px] resize-y text-xs leading-5 shadow-none focus-visible:ring-0"
            value={draft.brief_description || ''}
            onChange={(event) => setDraft({ ...draft, brief_description: event.target.value })}
            placeholder={t('settingPanel.placeholder.brief')}
          />
        </Field>
        <div className="lg:col-span-4 text-[11px] leading-5 text-[var(--nova-text-faint)]">
          {draft.load_mode === 'resident' ? t('settingPanel.lore.residentDesc') : loadModeDescription(draft.load_mode, t)}
          {residentWarning ? <span className="ml-2 text-[var(--nova-danger)]">{t('settingPanel.lore.residentWarning')}</span> : null}
        </div>
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
    <div className="min-h-0 flex-1 p-4">
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
