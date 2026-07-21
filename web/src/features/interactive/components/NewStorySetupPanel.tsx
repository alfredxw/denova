import { ChevronDown, Clock3, GitBranch, Image, Loader2, Lock, MousePointerClick, Package, Scale, Sparkles, UserRound, WandSparkles, Zap } from 'lucide-react'
import type { ElementType } from 'react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { getActorStates, getEventPackages, getRuleSystems } from '../api'
import { DEFAULT_INTERACTIVE_CHOICE_COUNT, DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS, MAX_INTERACTIVE_CHOICE_COUNT, MIN_INTERACTIVE_CHOICE_COUNT, type StoryCreateInput } from '../opening'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, StoryDirectorModuleRefs, StoryDirectorRunMode, StoryDirectorRunPolicy, StoryStateSchemaMode, StorySummary, Teller } from '../types'

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
  const [stateSchemaMode, setStateSchemaMode] = useState<StoryStateSchemaMode>(story?.state_schema_policy?.mode || 'adapt_template')
  const initialRunPolicy = story?.director_run_policy || defaultDirectorRunPolicy(initialDirector)
  const [directorRunMode, setDirectorRunMode] = useState<StoryDirectorRunMode>(initialRunPolicy.mode)
  const [directorIntervalTurns, setDirectorIntervalTurns] = useState(String(initialRunPolicy.interval_turns || 3))
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
    const nextRunPolicy = defaultDirectorRunPolicy(next)
    setDirectorId(id)
    setModuleRefs({ ...(next?.module_refs || {}) })
    setDirectorRunMode(nextRunPolicy.mode)
    setDirectorIntervalTurns(String(nextRunPolicy.interval_turns || 3))
  }
  const submit = async () => {
    if (creating) return
    setCreating(true)
    setError('')
    try {
      const tellerID = moduleRefs.narrative_style_disabled ? 'classic' : moduleRefs.narrative_style_id || tellers[0]?.id || 'classic'
      const normalizedChoiceCount = parseChoiceCount(choiceCount)
      if (normalizedChoiceCount === null) throw new Error(t('storyPicker.choiceCountError'))
      const directorRunPolicy = buildDirectorRunPolicy(directorRunMode, directorIntervalTurns)
      if (!directorRunPolicy) throw new Error(t('storyPicker.setup.directorRun.intervalError'))
      const selectedActorStateID = moduleRefs.actor_state_id || director?.module_refs?.actor_state_id
      const submittedModuleRefs: StoryDirectorModuleRefs = {
        ...moduleRefs,
        actor_state_id: selectedActorStateID,
        actor_state_disabled: stateSchemaMode === 'generate',
      }
      await onCreate({
        title: title.trim() || defaultStoryTitle(stories, t),
        origin: origin.trim(),
        story_teller_id: tellerID,
        story_director_id: directorId,
        director_run_policy: directorRunPolicy,
        reply_target_chars: normalizeReplyTargetChars(replyTargetChars),
        choice_count: normalizedChoiceCount,
        module_refs: submittedModuleRefs,
        state_schema_policy: { mode: stateSchemaMode },
        image_settings: { mode: 'manual', interval_turns: 3, preset_id: moduleRefs.image_preset_id || 'game-cg' },
      })
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t('storyPicker.createFailed'))
      setCreating(false)
    }
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 pt-5 sm:px-7 lg:px-10">
      <section className="mx-auto w-full max-w-4xl" aria-labelledby="new-story-title">
        <header className="mb-4">
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <span className="flex items-center gap-2 text-[11px] font-medium tracking-[0.12em] text-[var(--nova-text-faint)]"><span className="h-px w-5 bg-[var(--nova-accent)]/70" />{t('storyPicker.setup.eyebrow')}</span>
            <h2 id="new-story-title" className="text-lg font-semibold tracking-[-0.02em] text-[var(--nova-text)] sm:text-xl">{story ? t('storyPicker.setup.editTitle') : t('storyPicker.setup.title')}</h2>
          </div>
          <p className="mt-1 text-xs leading-5 text-[var(--nova-text-faint)]">{t('storyPicker.setup.description')}</p>
        </header>

        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_max-content_max-content]">
            <Field label={t('storyPicker.setup.name')}><Input value={title} maxLength={80} onChange={(event) => setTitle(event.target.value)} className="nova-field" /></Field>
            <Field label={t('storyPicker.storyDirector')}>
              <Select value={directorId} onValueChange={selectDirector}>
                <SelectTrigger className="nova-field h-8 w-full text-sm"><SelectValue /></SelectTrigger>
                <SelectContent position="popper" className="nova-panel border text-[var(--nova-text)]">{directors.map((item) => <SelectItem key={item.id} value={item.id}>{item.name || item.id}</SelectItem>)}</SelectContent>
              </Select>
            </Field>
            <Field label={t('storyPicker.replyTargetChars')}><Input type="number" min={1} value={replyTargetChars} onChange={(event) => setReplyTargetChars(event.target.value)} className="nova-field" /></Field>
            <Field label={t('storyPicker.choiceCount')}><Input type="number" min={MIN_INTERACTIVE_CHOICE_COUNT} max={MAX_INTERACTIVE_CHOICE_COUNT} value={choiceCount} onChange={(event) => setChoiceCount(event.target.value)} className="nova-field" /></Field>
            <p className="-mt-1.5 text-[11px] leading-4 text-[var(--nova-text-faint)] sm:col-span-2 lg:col-span-4">{t('storyPicker.choiceCountHint')}</p>
          </div>
          <Field label={t('storyPicker.setup.brief')} hint={t('storyPicker.setup.briefHint')}><Textarea autoResize value={origin} maxLength={4000} onChange={(event) => setOrigin(event.target.value)} className="nova-field min-h-20 resize-y" placeholder={t('storyPicker.originPlaceholder')} /></Field>

          <DirectorRunPolicyCard mode={directorRunMode} intervalTurns={directorIntervalTurns} onModeChange={setDirectorRunMode} onIntervalTurnsChange={setDirectorIntervalTurns} t={t} />

          <section className="border-t border-[var(--nova-border)] pt-4">
            <SectionHeader title={t('storyPicker.setup.modules')} description={t('storyPicker.setup.modulesHint', { director: director?.name || directorId })} />
            <div className="mt-3 grid gap-2.5 sm:grid-cols-2 lg:grid-cols-4">
              {moduleFields.map((field) => <ModuleSelectCard key={field.label} field={field} refs={moduleRefs} directorRefs={director?.module_refs || {}} options={moduleOptions[field.id]} onChange={setModuleRefs} t={t} />)}
              <EventPackagesCard refs={moduleRefs} directorRefs={director?.module_refs || {}} options={moduleCatalog.eventPackages.map(moduleOption)} onChange={setModuleRefs} t={t} />
              <StateSchemaPolicyCard mode={stateSchemaMode} onModeChange={setStateSchemaMode} refs={moduleRefs} directorRefs={director?.module_refs || {}} options={moduleOptions.actor_state_id} onRefsChange={setModuleRefs} t={t} />
            </div>
          </section>
        </div>

        {error ? <div className="mt-4 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">{error}</div> : null}
        <footer className="sticky bottom-0 z-10 mt-5 flex items-center justify-end gap-2 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] pb-1 pt-3"><Button type="button" variant="ghost" disabled={creating} onClick={onCancel}>{t('common.cancel')}</Button><Button type="button" disabled={creating} onClick={() => void submit()}>{creating ? <Loader2 className="h-4 w-4 animate-spin" /> : null}{creating ? t('common.creating') : t('storyPicker.setup.continue')}</Button></footer>
      </section>
    </div>
  )
}

function Field({ label, hint, className, children }: { label: string; hint?: string; className?: string; children: React.ReactNode }) { return <label className={cn('block text-xs text-[var(--nova-text-muted)]', className)}><span className="mb-1 block font-medium text-[var(--nova-text)]">{label}</span>{children}{hint ? <span className="mt-1 block text-[11px] leading-4 text-[var(--nova-text-faint)]">{hint}</span> : null}</label> }

/** 分区标题：标题与说明同行排列，保持表单段落紧凑。 */
function SectionHeader({ title, description, titleId }: { title: string; description: string; titleId?: string }) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-0.5">
      <h3 id={titleId} className="text-xs font-medium text-[var(--nova-text)]">{title}</h3>
      <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{description}</p>
    </div>
  )
}

/** 单选选项卡片：后台导演运行方式与状态结构共用的紧凑选项样式。 */
function OptionRadioCard({ id, value, icon: Icon, selected, title, badge, description }: { id: string; value: string; icon: ElementType; selected: boolean; title: string; badge?: string; description: string }) {
  return (
    <label htmlFor={id} className={cn('relative flex cursor-pointer items-start gap-2 rounded-[10px] border p-2.5 transition-colors', selected ? 'border-[var(--nova-field-focus-border)] bg-[var(--nova-surface)]' : 'border-[var(--nova-border)] hover:bg-[var(--nova-surface)]/60')}>
      <RadioGroupItem id={id} value={value} className="mt-0.5 border-[var(--nova-border)] text-[var(--nova-accent)]" />
      <span className="min-w-0">
        <span className="flex flex-wrap items-center gap-1.5 text-xs font-medium text-[var(--nova-text)]"><Icon className="size-3.5 text-[var(--nova-text-muted)]" />{title}{badge ? <span className="rounded-full bg-[var(--nova-accent)]/10 px-1.5 py-0.5 text-[9px] font-medium text-[var(--nova-accent)]">{badge}</span> : null}</span>
        <span className="mt-0.5 block text-[10px] leading-4 text-[var(--nova-text-faint)]">{description}</span>
      </span>
    </label>
  )
}

function normalizeReplyTargetChars(value: string) { const parsed = Number(value); return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS }
function parseChoiceCount(value: string) { const parsed = Number(value); return Number.isInteger(parsed) && parsed >= MIN_INTERACTIVE_CHOICE_COUNT && parsed <= MAX_INTERACTIVE_CHOICE_COUNT ? parsed : null }
function defaultStoryTitle(stories: StorySummary[], t: (key: string, options?: Record<string, unknown>) => string) { return stories.length === 0 ? t('storyPicker.firstTitle') : t('storyPicker.numberedTitle', { number: stories.length + 1 }) }

function defaultDirectorRunPolicy(director?: StoryDirector): StoryDirectorRunPolicy {
  switch (director?.strategy?.director_agent_mode) {
    case 'every_turn': return { mode: 'interval', interval_turns: 1 }
    case 'off': return { mode: 'manual' }
    case 'triggered':
    default: return { mode: 'on_demand' }
  }
}

function buildDirectorRunPolicy(mode: StoryDirectorRunMode, intervalTurns: string): StoryDirectorRunPolicy | null {
  if (mode !== 'interval') return { mode }
  const parsed = Number(intervalTurns)
  if (!Number.isInteger(parsed) || parsed <= 0) return null
  return { mode, interval_turns: parsed }
}

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

const stateSchemaModes: Array<{ mode: StoryStateSchemaMode; icon: ElementType }> = [
  { mode: 'adapt_template', icon: GitBranch },
  { mode: 'fixed_template', icon: Lock },
  { mode: 'generate', icon: WandSparkles },
]

const directorRunModes: Array<{ mode: StoryDirectorRunMode; icon: ElementType }> = [
  { mode: 'on_demand', icon: Zap },
  { mode: 'manual', icon: MousePointerClick },
  { mode: 'interval', icon: Clock3 },
]

function DirectorRunPolicyCard({ mode, intervalTurns, onModeChange, onIntervalTurnsChange, t }: { mode: StoryDirectorRunMode; intervalTurns: string; onModeChange: (mode: StoryDirectorRunMode) => void; onIntervalTurnsChange: (value: string) => void; t: (key: string, options?: Record<string, unknown>) => string }) {
  return (
    <section className="border-t border-[var(--nova-border)] pt-4" aria-labelledby="director-run-policy-title">
      <SectionHeader titleId="director-run-policy-title" title={t('storyPicker.setup.directorRun.title')} description={t('storyPicker.setup.directorRun.description')} />
      <RadioGroup value={mode} onValueChange={(value) => onModeChange(value as StoryDirectorRunMode)} className="mt-3 grid gap-2 md:grid-cols-3" aria-label={t('storyPicker.setup.directorRun.title')}>
        {directorRunModes.map(({ mode: optionMode, icon }) => (
          <OptionRadioCard key={optionMode} id={`director-run-${optionMode}`} value={optionMode} icon={icon} selected={mode === optionMode} title={t(`storyPicker.setup.directorRun.${optionMode}.title`)} badge={optionMode === 'on_demand' ? t('storyPicker.setup.directorRun.recommended') : undefined} description={t(`storyPicker.setup.directorRun.${optionMode}.description`)} />
        ))}
      </RadioGroup>
      {mode === 'interval' ? <Field label={t('storyPicker.setup.directorRun.intervalLabel')} hint={t('storyPicker.setup.directorRun.intervalHint')} className="mt-2.5 sm:max-w-xs"><Input aria-label={t('storyPicker.setup.directorRun.intervalLabel')} type="number" min={1} step={1} value={intervalTurns} onChange={(event) => onIntervalTurnsChange(event.target.value)} className="nova-field" /></Field> : null}
    </section>
  )
}

function StateSchemaPolicyCard({ mode, onModeChange, refs, directorRefs, options, onRefsChange, t }: { mode: StoryStateSchemaMode; onModeChange: (mode: StoryStateSchemaMode) => void; refs: StoryDirectorModuleRefs; directorRefs: StoryDirectorModuleRefs; options: Array<{ id: string; label: string }>; onRefsChange: React.Dispatch<React.SetStateAction<StoryDirectorModuleRefs>>; t: (key: string, options?: Record<string, unknown>) => string }) {
  const directorID = directorRefs.actor_state_id || ''
  const currentID = refs.actor_state_id || directorID
  const visibleOptions = [...options]
  for (const id of [directorID, currentID]) if (id && !visibleOptions.some((option) => option.id === id)) visibleOptions.push({ id, label: id })
  return (
    <section className="min-w-0 rounded-[12px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 sm:col-span-2 lg:col-span-4" aria-labelledby="state-schema-policy-title">
      <div className="flex items-center gap-2.5">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)]"><UserRound className="size-3.5 text-[var(--nova-accent)]" /></span>
        <div className="min-w-0 flex flex-wrap items-baseline gap-x-2.5 gap-y-0.5">
          <h4 id="state-schema-policy-title" className="text-xs font-medium text-[var(--nova-text)]">{t('storyPicker.setup.stateSchema.title')}</h4>
          <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('storyPicker.setup.stateSchema.description')}</p>
        </div>
      </div>
      <RadioGroup value={mode} onValueChange={(value) => onModeChange(value as StoryStateSchemaMode)} className="mt-2.5 grid gap-2 md:grid-cols-3" aria-label={t('storyPicker.setup.stateSchema.title')}>
        {stateSchemaModes.map(({ mode: optionMode, icon }) => (
          <OptionRadioCard key={optionMode} id={`state-schema-${optionMode}`} value={optionMode} icon={icon} selected={mode === optionMode} title={t(`storyPicker.setup.stateSchema.${optionMode}.title`)} badge={optionMode === 'adapt_template' ? t('storyPicker.setup.stateSchema.recommended') : undefined} description={t(`storyPicker.setup.stateSchema.${optionMode}.description`)} />
        ))}
      </RadioGroup>
      {mode !== 'generate' ? (
        <div className="mt-2.5 flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-[var(--nova-border)] pt-2.5">
          <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-muted)]">{t('storyPicker.setup.stateSchema.template')}</span>
          <Select value={currentID} onValueChange={(actor_state_id) => onRefsChange((current) => ({ ...current, actor_state_id, actor_state_disabled: false }))}>
            <SelectTrigger size="sm" aria-label={t('storyPicker.setup.stateSchema.template')} className="nova-field h-8 min-w-40 flex-1 bg-[var(--nova-surface)] px-2 text-xs sm:max-w-sm"><SelectValue placeholder={t('storyPicker.setup.stateSchema.templatePlaceholder')} /></SelectTrigger>
            <SelectContent position="popper" className="nova-panel border text-[var(--nova-text)]">{visibleOptions.map((option) => <SelectItem key={option.id} value={option.id}>{option.label}</SelectItem>)}</SelectContent>
          </Select>
        </div>
      ) : <p className="mt-2.5 border-t border-[var(--nova-border)] pt-2.5 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('storyPicker.setup.stateSchema.generateCore')}</p>}
    </section>
  )
}

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
    <div className="min-w-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2.5 transition-colors focus-within:border-[var(--nova-field-focus-border)]">
      <div className="mb-1.5 flex items-center gap-2 text-[11px] font-medium text-[var(--nova-text-muted)]"><Icon className="h-3.5 w-3.5 text-[var(--nova-accent)]" /><span>{label}</span></div>
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
    <div className="min-w-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2.5 transition-colors focus-within:border-[var(--nova-field-focus-border)]">
      <div className="mb-1.5 flex items-center gap-2 text-[11px] font-medium text-[var(--nova-text-muted)]"><Package className="h-3.5 w-3.5 text-[var(--nova-accent)]" /><span>{t('storyPicker.setup.module.eventPackages')}</span></div>
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
