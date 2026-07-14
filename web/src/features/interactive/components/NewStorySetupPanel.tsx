import { ChevronDown, Image, Loader2, Package, Scale, Sparkles, UserRound } from 'lucide-react'
import type { ElementType } from 'react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { getActorStates, getEventPackages, getRuleSystems } from '../api'
import { DEFAULT_INTERACTIVE_CHOICE_COUNT, DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS, MAX_INTERACTIVE_CHOICE_COUNT, MIN_INTERACTIVE_CHOICE_COUNT, type StoryCreateInput } from '../opening'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, StoryDirectorModuleRefs, StorySummary, Teller } from '../types'

interface NewStorySetupPanelProps {
  stories: StorySummary[]
  tellers: Teller[]
  directors: StoryDirector[]
  imagePresets: ImagePreset[]
  story?: StorySummary
  onCancel: () => void
  onCreate: (input: StoryCreateInput) => void | Promise<void>
}

const moduleFields: Array<{ id: keyof StoryDirectorModuleRefs; disabled: keyof StoryDirectorModuleRefs; label: string; icon: ElementType }> = [
  { id: 'narrative_style_id', disabled: 'narrative_style_disabled', label: 'narrativeStyle', icon: Sparkles },
  { id: 'rule_system_id', disabled: 'rule_system_disabled', label: 'ruleSystem', icon: Scale },
  { id: 'actor_state_id', disabled: 'actor_state_disabled', label: 'actorState', icon: UserRound },
  { id: 'image_preset_id', disabled: 'image_preset_disabled', label: 'imagePreset', icon: Image },
]

export function NewStorySetupPanel({ stories, tellers, directors, imagePresets, story, onCancel, onCreate }: NewStorySetupPanelProps) {
  const { t } = useTranslation()
  const defaultDirector = directors[0]
  const initialDirector = directors.find((item) => item.id === story?.story_director_id) || defaultDirector
  const [title, setTitle] = useState(() => story?.title || defaultStoryTitle(stories, t))
  const [origin, setOrigin] = useState(story?.origin || '')
  const [directorId, setDirectorId] = useState(initialDirector?.id || 'default')
  const [replyTargetChars, setReplyTargetChars] = useState(String(story?.reply_target_chars || DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS))
  const [choiceCount, setChoiceCount] = useState(String(story?.choice_count || DEFAULT_INTERACTIVE_CHOICE_COUNT))
  const [moduleRefs, setModuleRefs] = useState<StoryDirectorModuleRefs>(() => ({ ...(story?.module_refs || initialDirector?.module_refs || {}) }))
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [moduleCatalog, setModuleCatalog] = useState<DirectorModuleCatalog>({ eventPackages: [], ruleSystems: [], actorStates: [] })
  const director = directors.find((item) => item.id === directorId) || defaultDirector
  const moduleOptions = useMemo(() => collectModuleOptions(directors, tellers, imagePresets, moduleCatalog), [directors, imagePresets, moduleCatalog, tellers])

  useEffect(() => {
    let cancelled = false
    void Promise.all([getEventPackages(), getRuleSystems(), getActorStates()])
      .then(([eventPackages, ruleSystems, actorStates]) => {
        if (!cancelled) setModuleCatalog({ eventPackages, ruleSystems, actorStates })
      })
      .catch((reason) => console.error('[new-story-setup] 加载导演模块方案预设失败', reason))
    return () => { cancelled = true }
  }, [])

  const selectDirector = (id: string) => {
    const next = directors.find((item) => item.id === id)
    setDirectorId(id)
    setModuleRefs({ ...(next?.module_refs || {}) })
  }
  const submit = async () => {
    if (creating) return
    setCreating(true)
    setError('')
    try {
      const tellerID = moduleRefs.narrative_style_disabled ? 'classic' : moduleRefs.narrative_style_id || tellers[0]?.id || 'classic'
      const normalizedChoiceCount = parseChoiceCount(choiceCount)
      if (normalizedChoiceCount === null) throw new Error(t('storyPicker.choiceCountError'))
      await onCreate({
        title: title.trim() || defaultStoryTitle(stories, t),
        origin: origin.trim(),
        story_teller_id: tellerID,
        story_director_id: directorId,
        reply_target_chars: normalizeReplyTargetChars(replyTargetChars),
        choice_count: normalizedChoiceCount,
        module_refs: moduleRefs,
        image_settings: { mode: 'manual', interval_turns: 3, preset_id: moduleRefs.image_preset_id || 'game-cg' },
      })
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t('storyPicker.createFailed'))
      setCreating(false)
    }
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 pt-6 sm:px-7 sm:pt-8 lg:px-10">
      <section className="mx-auto w-full max-w-4xl" aria-labelledby="new-story-title">
        <header className="mb-7">
          <div className="mb-1 flex items-center gap-2 text-[11px] font-medium tracking-[0.12em] text-[var(--nova-text-faint)]"><span className="h-px w-5 bg-[var(--nova-accent)]/70" />{t('storyPicker.setup.eyebrow')}</div>
          <h2 id="new-story-title" className="text-xl font-semibold tracking-[-0.02em] text-[var(--nova-text)] sm:text-2xl">{story ? t('storyPicker.setup.editTitle') : t('storyPicker.setup.title')}</h2>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-[var(--nova-text-faint)] sm:text-sm sm:leading-6">{t('storyPicker.setup.description')}</p>
        </header>

        <div className="space-y-5">
          <Field label={t('storyPicker.setup.name')}><Input value={title} maxLength={80} onChange={(event) => setTitle(event.target.value)} className="nova-field" /></Field>
          <Field label={t('storyPicker.setup.brief')} hint={t('storyPicker.setup.briefHint')}><Textarea autoResize value={origin} maxLength={4000} onChange={(event) => setOrigin(event.target.value)} className="nova-field min-h-28 resize-y" placeholder={t('storyPicker.originPlaceholder')} /></Field>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-[minmax(0,1fr)_10rem_10rem]">
            <div className="sm:col-span-2 lg:col-span-1">
              <Field label={t('storyPicker.storyDirector')}>
                <Select value={directorId} onValueChange={selectDirector}>
                  <SelectTrigger className="nova-field h-10 w-full text-sm"><SelectValue /></SelectTrigger>
                  <SelectContent position="popper" className="nova-panel border text-[var(--nova-text)]">{directors.map((item) => <SelectItem key={item.id} value={item.id}>{item.name || item.id}</SelectItem>)}</SelectContent>
                </Select>
              </Field>
            </div>
            <Field label={t('storyPicker.replyTargetChars')}><Input type="number" min={1} value={replyTargetChars} onChange={(event) => setReplyTargetChars(event.target.value)} className="nova-field" /></Field>
            <Field label={t('storyPicker.choiceCount')} hint={t('storyPicker.choiceCountHint')}><Input type="number" min={MIN_INTERACTIVE_CHOICE_COUNT} max={MAX_INTERACTIVE_CHOICE_COUNT} value={choiceCount} onChange={(event) => setChoiceCount(event.target.value)} className="nova-field" /></Field>
          </div>

          <section className="border-t border-[var(--nova-border)] pt-5">
            <div><h3 className="text-sm font-medium text-[var(--nova-text)]">{t('storyPicker.setup.modules')}</h3><p className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('storyPicker.setup.modulesHint', { director: director?.name || directorId })}</p></div>
            <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {moduleFields.map((field) => <ModuleSelectCard key={field.label} field={field} refs={moduleRefs} directorRefs={director?.module_refs || {}} options={moduleOptions[field.id]} onChange={setModuleRefs} t={t} />)}
              <EventPackagesCard refs={moduleRefs} directorRefs={director?.module_refs || {}} options={moduleCatalog.eventPackages.map(moduleOption)} onChange={setModuleRefs} t={t} />
            </div>
          </section>
        </div>

        {error ? <div className="mt-5 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">{error}</div> : null}
        <footer className="mt-8 flex items-center justify-end gap-2 border-t border-[var(--nova-border)] pt-4"><Button type="button" variant="ghost" disabled={creating} onClick={onCancel}>{t('common.cancel')}</Button><Button type="button" disabled={creating} onClick={() => void submit()}>{creating ? <Loader2 className="h-4 w-4 animate-spin" /> : null}{creating ? t('common.creating') : t('storyPicker.setup.continue')}</Button></footer>
      </section>
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) { return <label className="block text-xs text-[var(--nova-text-muted)]"><span className="mb-1.5 block font-medium text-[var(--nova-text)]">{label}</span>{children}{hint ? <span className="mt-1 block text-[11px] leading-5 text-[var(--nova-text-faint)]">{hint}</span> : null}</label> }
function normalizeReplyTargetChars(value: string) { const parsed = Number(value); return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS }
function parseChoiceCount(value: string) { const parsed = Number(value); return Number.isInteger(parsed) && parsed >= MIN_INTERACTIVE_CHOICE_COUNT && parsed <= MAX_INTERACTIVE_CHOICE_COUNT ? parsed : null }
function defaultStoryTitle(stories: StorySummary[], t: (key: string, options?: Record<string, unknown>) => string) { return stories.length === 0 ? t('storyPicker.firstTitle') : t('storyPicker.numberedTitle', { number: stories.length + 1 }) }

type ModuleOptionMap = Record<keyof StoryDirectorModuleRefs, Array<{ id: string; label: string }>>
interface DirectorModuleCatalog {
  eventPackages: EventPackageModule[]
  ruleSystems: RuleSystemModule[]
  actorStates: ActorStateModule[]
}

function collectModuleOptions(directors: StoryDirector[], tellers: Teller[], imagePresets: ImagePreset[], catalog: DirectorModuleCatalog): ModuleOptionMap {
  const map = {} as ModuleOptionMap
  const keys: Array<keyof StoryDirectorModuleRefs> = ['narrative_style_id', 'rule_system_id', 'actor_state_id', 'image_preset_id']
  keys.forEach((key) => { map[key] = [] })
  map.narrative_style_id = tellers.map(moduleOption)
  map.rule_system_id = catalog.ruleSystems.map(moduleOption)
  map.actor_state_id = catalog.actorStates.map(moduleOption)
  map.image_preset_id = imagePresets.map(moduleOption)
  for (const director of directors) for (const key of ['rule_system_id', 'actor_state_id'] as const) { const id = director.module_refs?.[key]; if (typeof id === 'string' && id && !map[key].some((item) => item.id === id)) map[key].push({ id, label: id }) }
  return map
}

function moduleOption(item: { id: string; name: string }) { return { id: item.id, label: item.name || item.id } }

function ModuleSelectCard({ field, refs, directorRefs, options, onChange, t }: { field: (typeof moduleFields)[number]; refs: StoryDirectorModuleRefs; directorRefs: StoryDirectorModuleRefs; options: Array<{ id: string; label: string }>; onChange: React.Dispatch<React.SetStateAction<StoryDirectorModuleRefs>>; t: (key: string, options?: Record<string, unknown>) => string }) {
  const Icon = field.icon
  const currentID = typeof refs[field.id] === 'string' ? String(refs[field.id]) : ''
  const directorID = typeof directorRefs[field.id] === 'string' ? String(directorRefs[field.id]) : ''
  const value = refs[field.disabled] ? '__disabled' : currentID === directorID ? '__default' : currentID || '__default'
  const label = t(`storyPicker.setup.module.${field.label}`)
  const visibleOptions = [...options]
  for (const id of [directorID, currentID]) if (id && !visibleOptions.some((option) => option.id === id)) visibleOptions.push({ id, label: id })
  const directorLabel = visibleOptions.find((option) => option.id === directorID)?.label || directorID || t('storyPicker.setup.default')
  return (
    <div className="min-w-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 transition-colors focus-within:border-[var(--nova-field-focus-border)]">
      <div className="mb-2 flex items-center gap-2 text-[11px] font-medium text-[var(--nova-text-muted)]"><Icon className="h-3.5 w-3.5 text-[var(--nova-accent)]" /><span>{label}</span></div>
      <Select value={value} onValueChange={(next) => onChange((current) => next === '__default' ? { ...current, [field.disabled]: Boolean(directorRefs[field.disabled]), [field.id]: directorID } : next === '__disabled' ? { ...current, [field.disabled]: true } : { ...current, [field.disabled]: false, [field.id]: next })}>
        <SelectTrigger size="sm" aria-label={label} className="nova-field h-8 w-full border-transparent bg-[var(--nova-surface)] px-2 text-xs"><SelectValue /></SelectTrigger>
        <SelectContent position="popper" className="nova-panel border text-[var(--nova-text)]">
          <SelectItem value="__default">{t('storyPicker.setup.defaultWithValue', { value: directorLabel })}</SelectItem>
          <SelectItem value="__disabled">{t('storyPicker.setup.disabled')}</SelectItem>
          {visibleOptions.map((option) => <SelectItem key={option.id} value={option.id}>{option.label}</SelectItem>)}
        </SelectContent>
      </Select>
    </div>
  )
}

function EventPackagesCard({ refs, directorRefs, options, onChange, t }: { refs: StoryDirectorModuleRefs; directorRefs: StoryDirectorModuleRefs; options: Array<{ id: string; label: string }>; onChange: React.Dispatch<React.SetStateAction<StoryDirectorModuleRefs>>; t: (key: string, options?: Record<string, unknown>) => string }) {
  const selected = refs.event_package_ids || []
  const available = [...options]
  for (const id of [...(directorRefs.event_package_ids || []), ...selected]) if (!available.some((option) => option.id === id)) available.push({ id, label: id })
  const inherited = !refs.event_packages_disabled && arraysEqual(selected, directorRefs.event_package_ids || [])
  const selectedLabel = selected.map((id) => available.find((option) => option.id === id)?.label || id).join(', ') || t('storyPicker.setup.none')
  const label = refs.event_packages_disabled ? t('storyPicker.setup.disabled') : inherited ? t('storyPicker.setup.defaultWithValue', { value: selectedLabel }) : selectedLabel
  return (
    <div className="min-w-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 transition-colors focus-within:border-[var(--nova-field-focus-border)]">
      <div className="mb-2 flex items-center gap-2 text-[11px] font-medium text-[var(--nova-text-muted)]"><Package className="h-3.5 w-3.5 text-[var(--nova-accent)]" /><span>{t('storyPicker.setup.module.eventPackages')}</span></div>
      <DropdownMenu>
        <DropdownMenuTrigger asChild><Button type="button" variant="outline" size="sm" aria-label={t('storyPicker.setup.module.eventPackages')} className="nova-field h-8 w-full justify-between border-transparent bg-[var(--nova-surface)] px-2 text-xs font-normal"><span className="min-w-0 truncate">{label}</span><ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /></Button></DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="nova-panel w-64 border text-[var(--nova-text)]">
          <DropdownMenuItem onSelect={() => onChange((current) => ({ ...current, event_packages_disabled: Boolean(directorRefs.event_packages_disabled), event_package_ids: [...(directorRefs.event_package_ids || [])] }))}>{t('storyPicker.setup.default')}</DropdownMenuItem>
          <DropdownMenuCheckboxItem checked={Boolean(refs.event_packages_disabled)} onCheckedChange={(checked) => onChange((current) => ({ ...current, event_packages_disabled: checked === true }))}>{t('storyPicker.setup.disabled')}</DropdownMenuCheckboxItem>
          {available.length ? <DropdownMenuSeparator /> : null}
          {available.map((option) => <DropdownMenuCheckboxItem key={option.id} checked={!refs.event_packages_disabled && selected.includes(option.id)} onCheckedChange={(checked) => onChange((current) => ({ ...current, event_packages_disabled: false, event_package_ids: checked ? Array.from(new Set([...(current.event_package_ids || []), option.id])) : (current.event_package_ids || []).filter((item) => item !== option.id) }))}>{option.label}</DropdownMenuCheckboxItem>)}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

function arraysEqual(a: string[], b: string[]) { return a.length === b.length && a.every((value, index) => value === b[index]) }
