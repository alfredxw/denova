import { Boxes, ChevronDown, ChevronUp, Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS, type StoryCreateInput } from '../opening'
import type { ImagePreset, StoryDirector, StoryDirectorModuleRefs, StorySummary, Teller } from '../types'

interface NewStorySetupPanelProps {
  stories: StorySummary[]
  tellers: Teller[]
  directors: StoryDirector[]
  imagePresets: ImagePreset[]
  story?: StorySummary
  onCancel: () => void
  onCreate: (input: StoryCreateInput) => void | Promise<void>
}

const moduleFields: Array<{ id: keyof StoryDirectorModuleRefs; disabled: keyof StoryDirectorModuleRefs; label: string }> = [
  { id: 'narrative_style_id', disabled: 'narrative_style_disabled', label: 'narrativeStyle' },
  { id: 'rule_system_id', disabled: 'rule_system_disabled', label: 'ruleSystem' },
  { id: 'actor_state_id', disabled: 'actor_state_disabled', label: 'actorState' },
  { id: 'memory_structure_id', disabled: 'memory_structure_disabled', label: 'memoryStructure' },
  { id: 'image_preset_id', disabled: 'image_preset_disabled', label: 'imagePreset' },
]

export function NewStorySetupPanel({ stories, tellers, directors, imagePresets, story, onCancel, onCreate }: NewStorySetupPanelProps) {
  const { t } = useTranslation()
  const defaultDirector = directors[0]
  const initialDirector = directors.find((item) => item.id === story?.story_director_id) || defaultDirector
  const [title, setTitle] = useState(() => story?.title || defaultStoryTitle(stories, t))
  const [origin, setOrigin] = useState(story?.origin || '')
  const [directorId, setDirectorId] = useState(initialDirector?.id || 'default')
  const [replyTargetChars, setReplyTargetChars] = useState(String(story?.reply_target_chars || DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS))
  const [moduleRefs, setModuleRefs] = useState<StoryDirectorModuleRefs>(() => ({ ...(story?.module_refs || initialDirector?.module_refs || {}) }))
  const [customizing, setCustomizing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const director = directors.find((item) => item.id === directorId) || defaultDirector
  const moduleOptions = useMemo(() => collectModuleOptions(directors, tellers, imagePresets), [directors, imagePresets, tellers])

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
      await onCreate({
        title: title.trim() || defaultStoryTitle(stories, t),
        origin: origin.trim(),
        story_teller_id: tellerID,
        story_director_id: directorId,
        reply_target_chars: normalizeReplyTargetChars(replyTargetChars),
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
          <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_12rem]">
            <Field label={t('storyPicker.storyDirector')}><select className="nova-field h-10 w-full rounded-[var(--nova-radius)] px-3 text-sm" value={directorId} onChange={(event) => selectDirector(event.target.value)}>{directors.map((item) => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}</select></Field>
            <Field label={t('storyPicker.replyTargetChars')}><Input type="number" min={1} value={replyTargetChars} onChange={(event) => setReplyTargetChars(event.target.value)} className="nova-field" /></Field>
          </div>

          <section className="border-t border-[var(--nova-border)] pt-5">
            <div className="flex items-start justify-between gap-4">
              <div><h3 className="text-sm font-medium text-[var(--nova-text)]">{t('storyPicker.setup.modules')}</h3><p className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('storyPicker.setup.modulesHint', { director: director?.name || directorId })}</p></div>
              <Button type="button" variant="ghost" size="xs" className="shrink-0 gap-1.5" onClick={() => setCustomizing((value) => !value)}><Boxes className="h-3.5 w-3.5" />{t('storyPicker.setup.customize')}{customizing ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}</Button>
            </div>
            <div className="mt-3 grid gap-x-6 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
              {moduleFields.map((field) => <ModuleItem key={field.label} label={t(`storyPicker.setup.module.${field.label}`)} value={moduleLabel(moduleRefs, field, moduleOptions, t)} />)}
              <ModuleItem label={t('storyPicker.setup.module.eventPackages')} value={moduleRefs.event_packages_disabled ? t('storyPicker.setup.disabled') : moduleRefs.event_package_ids?.join(', ') || t('storyPicker.setup.default')} />
            </div>
            {customizing ? <div className="mt-4 grid gap-3 border-t border-[var(--nova-border-soft)] pt-4 sm:grid-cols-2">{moduleFields.map((field) => (
              <label key={field.label} className="text-[11px] text-[var(--nova-text-faint)]"><span className="mb-1 block">{t(`storyPicker.setup.module.${field.label}`)}</span><select className="nova-field h-9 w-full rounded-[var(--nova-radius)] px-2 text-xs" value={moduleRefs[field.disabled] ? '__disabled' : String(moduleRefs[field.id] || '')} onChange={(event) => setModuleRefs((current) => ({ ...current, [field.disabled]: event.target.value === '__disabled', [field.id]: event.target.value === '__disabled' ? current[field.id] : event.target.value }))}><option value="">{t('storyPicker.setup.default')}</option><option value="__disabled">{t('storyPicker.setup.disabled')}</option>{moduleOptions[field.id].map((option) => <option key={option.id} value={option.id}>{option.label}</option>)}</select></label>
            ))}</div> : null}
          </section>
        </div>

        {error ? <div className="mt-5 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">{error}</div> : null}
        <footer className="mt-8 flex items-center justify-end gap-2 border-t border-[var(--nova-border)] pt-4"><Button type="button" variant="ghost" disabled={creating} onClick={onCancel}>{t('common.cancel')}</Button><Button type="button" disabled={creating} onClick={() => void submit()}>{creating ? <Loader2 className="h-4 w-4 animate-spin" /> : null}{creating ? t('common.creating') : t('storyPicker.setup.continue')}</Button></footer>
      </section>
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) { return <label className="block text-xs text-[var(--nova-text-muted)]"><span className="mb-1.5 block font-medium text-[var(--nova-text)]">{label}</span>{children}{hint ? <span className="mt-1 block text-[11px] leading-5 text-[var(--nova-text-faint)]">{hint}</span> : null}</label> }
function ModuleItem({ label, value }: { label: string; value: string }) { return <div className="min-w-0 border-l border-[var(--nova-border)] pl-3"><div className="text-[10px] text-[var(--nova-text-faint)]">{label}</div><div className="mt-0.5 truncate text-xs text-[var(--nova-text-muted)]" title={value}>{value}</div></div> }
function normalizeReplyTargetChars(value: string) { const parsed = Number(value); return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS }
function defaultStoryTitle(stories: StorySummary[], t: (key: string, options?: Record<string, unknown>) => string) { return stories.length === 0 ? t('storyPicker.firstTitle') : t('storyPicker.numberedTitle', { number: stories.length + 1 }) }

type ModuleOptionMap = Record<keyof StoryDirectorModuleRefs, Array<{ id: string; label: string }>>
function collectModuleOptions(directors: StoryDirector[], tellers: Teller[], imagePresets: ImagePreset[]): ModuleOptionMap {
  const map = {} as ModuleOptionMap
  const keys: Array<keyof StoryDirectorModuleRefs> = ['narrative_style_id', 'rule_system_id', 'actor_state_id', 'memory_structure_id', 'image_preset_id']
  keys.forEach((key) => { map[key] = [] })
  map.narrative_style_id = tellers.map((item) => ({ id: item.id, label: item.name || item.id }))
  map.image_preset_id = imagePresets.map((item) => ({ id: item.id, label: item.name || item.id }))
  for (const director of directors) for (const key of ['rule_system_id', 'actor_state_id', 'memory_structure_id'] as const) { const id = director.module_refs?.[key]; if (typeof id === 'string' && id && !map[key].some((item) => item.id === id)) map[key].push({ id, label: id }) }
  return map
}
function moduleLabel(refs: StoryDirectorModuleRefs, field: (typeof moduleFields)[number], options: ModuleOptionMap, t: (key: string) => string) { if (refs[field.disabled]) return t('storyPicker.setup.disabled'); const id = refs[field.id]; if (typeof id !== 'string' || !id) return t('storyPicker.setup.default'); return options[field.id].find((item) => item.id === id)?.label || id }
