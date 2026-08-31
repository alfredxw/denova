import { ChevronDown, Loader2, Play, SlidersHorizontal } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import type { LoreItem } from '@/lib/api'
import { cn } from '@/lib/utils'
import { gamePresetName } from '../game-preset'
import { normalizeStoryImageSettings } from '../image-settings'
import { DEFAULT_NARRATIVE_STYLE_ID, resolveNarrativeStyle } from '../narrative-style'
import { DEFAULT_INTERACTIVE_CHOICE_COUNT, DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS, truncateStoryOpeningText, type BookOpeningPreset, type StoryCreateInput } from '../opening'
import type { ImagePreset, StoryDirector, StoryOpeningConfig, StoryProtagonist, StorySummary, Teller } from '../types'
import { StoryOpeningSelector } from './story-setup/StoryOpeningSelector'
import { StoryProtagonistSelector } from './story-setup/StoryProtagonistSelector'
import { StorySetupAdvanced, type StorySetupSettings } from './story-setup/StorySetupAdvanced'

interface NewStorySetupPanelProps {
  projectId: string
  stories: StorySummary[]
  tellers: Teller[]
  directors: StoryDirector[]
  imagePresets: ImagePreset[]
  loreItems?: LoreItem[]
  bookOpeningPresets?: BookOpeningPreset[]
  recentNarrativeStyleID?: string
  narrativeStyleLoading?: boolean
  story?: StorySummary
  onNarrativeStyleChange?: (id: string) => void | Promise<unknown>
  onRequestLoreInit?: () => void
  onOpenPresets?: () => void
  onCancel: () => void
  onCreate: (input: StoryCreateInput) => void | Promise<void>
}

export function NewStorySetupPanel({
  projectId,
  stories,
  tellers,
  directors,
  imagePresets,
  loreItems = [],
  bookOpeningPresets = [],
  recentNarrativeStyleID = DEFAULT_NARRATIVE_STYLE_ID,
  narrativeStyleLoading = false,
  story,
  onNarrativeStyleChange,
  onRequestLoreInit,
  onOpenPresets,
  onCancel,
  onCreate,
}: NewStorySetupPanelProps) {
  const { t } = useTranslation()
  const initialDirector = directors.find((item) => item.id === story?.story_director_id) || directors[0]
  const recentTeller = resolveNarrativeStyle(tellers, recentNarrativeStyleID)
  const initialProtagonist = story?.protagonist || defaultStoryProtagonist(loreItems)
  const [title, setTitle] = useState(() => story?.title || defaultStoryTitle(stories, t))
  const [origin, setOrigin] = useState(story?.origin || '')
  const [directorId, setDirectorId] = useState(initialDirector?.id || 'default')
  const [protagonist, setProtagonist] = useState<StoryProtagonist>(initialProtagonist)
  const [opening, setOpening] = useState<StoryOpeningConfig>(() => story?.opening || { mode: 'ai' })
  const [settings, setSettings] = useState<StorySetupSettings>(() => initialSettings(story, initialDirector, recentTeller?.id))
  const [advancedOpen, setAdvancedOpen] = useState(true)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const initialProtagonistRef = useRef<StoryProtagonist>(initialProtagonist)
  const protagonistSelectionTouchedRef = useRef(false)
  const narrativeStyleSelectionLockedRef = useRef(Boolean(story))
  const director = directors.find((item) => item.id === directorId) || directors[0]
  const directorName = director ? gamePresetName(director, t) : directorId
  const advancedSummary = useMemo(() => t('storyPicker.setup.advanced.summary', {
    planning: settings.planningEnabled ? t('storyPicker.setup.advanced.planningOn') : t('storyPicker.setup.advanced.planningOff'),
    checks: settings.moduleRefs.rule_system_disabled ? t('storyPicker.setup.advanced.checksOff') : t('storyPicker.setup.advanced.checksOn'),
    images: settings.imageSettings.mode === 'interval' ? t('storyPicker.setup.advanced.imagesOn') : t('storyPicker.setup.advanced.imagesOff'),
  }), [settings.imageSettings.mode, settings.moduleRefs.rule_system_disabled, settings.planningEnabled, t])

  useEffect(() => {
    if (story || narrativeStyleSelectionLockedRef.current || !recentTeller) return
    setSettings((current) => ({
      ...current,
      moduleRefs: { ...current.moduleRefs, narrative_style_id: recentTeller.id },
    }))
  }, [recentTeller, story])

  useEffect(() => {
    if (protagonistSelectionTouchedRef.current || protagonist.mode !== 'default') return
    const tagged = defaultStoryProtagonist(loreItems)
    if (tagged.mode === 'lore') setProtagonist(tagged)
  }, [loreItems, protagonist.mode])

  const changeProtagonist = (next: StoryProtagonist) => {
    protagonistSelectionTouchedRef.current = true
    setProtagonist(next)
  }

  const selectDirector = (id: string) => {
    const next = directors.find((item) => item.id === id)
    const moduleRefs = { ...(next?.module_refs || {}) }
    setDirectorId(id)
    narrativeStyleSelectionLockedRef.current = true
    setSettings((current) => ({
      ...current,
      moduleRefs,
      imageSettings: { ...current.imageSettings, preset_id: moduleRefs.image_preset_id || current.imageSettings.preset_id },
    }))
  }

  const submit = async () => {
    if (creating) return
    setError('')
    const validationError = validateDraft(protagonist, opening, loreItems, t)
    if (validationError) {
      setError(validationError)
      return
    }
    setCreating(true)
    try {
      const tellerID = resolveNarrativeStyle(tellers, settings.moduleRefs.narrative_style_id || recentNarrativeStyleID)?.id || DEFAULT_NARRATIVE_STYLE_ID
      const moduleRefs = {
        ...settings.moduleRefs,
        actor_state_id: settings.moduleRefs.actor_state_id || director?.module_refs?.actor_state_id,
        actor_state_disabled: settings.stateSchemaMode === 'generate',
      }
      const protagonistInput = protagonistForSubmit(protagonist)
      const includeProtagonist = !story || !sameProtagonist(protagonist, initialProtagonistRef.current)
      await onCreate({
        title: title.trim() || defaultStoryTitle(stories, t),
        ...(!story && settings.customAgentId ? { custom_agent_id: settings.customAgentId } : {}),
        origin: origin.trim(),
        ...(includeProtagonist ? { protagonist: protagonistInput } : {}),
        story_teller_id: tellerID,
        story_director_id: directorId,
        planning_mode: settings.planningEnabled ? 'enabled' : 'disabled',
        module_refs: moduleRefs,
        reply_target_chars: settings.replyTargetChars,
        choice_count: settings.choiceCount,
        opening: openingForSubmit(opening),
        image_settings: { ...settings.imageSettings, preset_id: settings.imageSettings.preset_id || moduleRefs.image_preset_id || 'game-cg' },
        check_settings: settings.checkSettings,
        state_schema_policy: { mode: settings.stateSchemaMode },
      })
    } catch (reason) {
      console.error('[story-setup] Failed to save and start story', reason)
      setError(reason instanceof Error ? reason.message : t('storyPicker.createFailed'))
      setCreating(false)
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-6 pt-5 sm:px-7 lg:px-10">
        <section className="mx-auto w-full max-w-5xl" aria-labelledby="new-story-title">
          <header className="mb-4">
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <span className="flex items-center gap-2 text-[11px] font-medium tracking-[0.12em] text-muted-foreground"><span className="h-px w-5 bg-primary/70" />{t('storyPicker.setup.eyebrow')}</span>
              <h2 id="new-story-title" className="text-xl font-semibold tracking-[-0.02em] text-foreground sm:text-2xl">{story ? t('storyPicker.setup.resumeTitle') : t('storyPicker.setup.title')}</h2>
            </div>
            <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground sm:text-sm">{t('storyPicker.setup.description')}</p>
          </header>

          <div className="space-y-4">
            <section className="grid gap-3 rounded-xl border border-border bg-card p-4 sm:grid-cols-2 sm:p-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)_minmax(12rem,0.7fr)]">
              <Field label={t('storyPicker.setup.name')}><Input value={title} maxLength={80} className="bg-background" onChange={(event) => setTitle(event.target.value)} /></Field>
              <Field label={t('storyPicker.setup.brief')} hint={t('storyPicker.setup.briefHint')}><Textarea autoResize value={origin} maxLength={4000} className="min-h-20 resize-y bg-background" placeholder={t('storyPicker.originPlaceholder')} onChange={(event) => setOrigin(event.target.value)} /></Field>
              <Field label={t('storyPicker.storyDirector')} hint={t('storyPicker.setup.presetHint', { director: directorName })}>
                <Select value={directorId} onValueChange={selectDirector}>
                  <SelectTrigger className="w-full bg-background"><SelectValue /></SelectTrigger>
                  <SelectContent position="popper">{directors.map((item) => <SelectItem key={item.id} value={item.id}>{gamePresetName(item, t)}</SelectItem>)}</SelectContent>
                </Select>
              </Field>
            </section>

            <div className="grid items-start gap-4 lg:grid-cols-[minmax(20rem,0.92fr)_minmax(0,1.08fr)]">
              <StoryProtagonistSelector projectId={projectId} value={protagonist} loreItems={loreItems} onChange={changeProtagonist} onRequestLoreInit={onRequestLoreInit} />
              <StoryOpeningSelector value={opening} presets={bookOpeningPresets} onChange={setOpening} />
            </div>

            <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen} className="overflow-hidden rounded-xl border border-border bg-card">
              <CollapsibleTrigger asChild>
                <button type="button" className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/50 sm:px-5" aria-label={t('storyPicker.setup.advanced.title')}>
                  <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-background text-primary"><SlidersHorizontal className="size-4" /></span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-medium text-foreground">{t('storyPicker.setup.advanced.title')}</span>
                    <span className="mt-0.5 block truncate text-xs text-muted-foreground">{advancedSummary}</span>
                  </span>
                  <ChevronDown className={cn('size-4 shrink-0 text-muted-foreground transition-transform', advancedOpen && 'rotate-180')} />
                </button>
              </CollapsibleTrigger>
              <CollapsibleContent className="border-t border-border bg-muted/20 p-3 sm:p-4">
                <StorySetupAdvanced projectId={projectId} newStory={!story} director={director} tellers={tellers} imagePresets={imagePresets} value={settings} onChange={setSettings} onNarrativeStyleChange={(id) => { narrativeStyleSelectionLockedRef.current = true; return onNarrativeStyleChange?.(id) }} onOpenPresets={onOpenPresets} />
              </CollapsibleContent>
            </Collapsible>
          </div>

          {error ? <div role="alert" className="mt-4 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{error}</div> : null}
        </section>
      </div>
      <footer data-testid="story-setup-footer" className="shrink-0 border-t border-border bg-[var(--nova-surface-2)] px-4 py-3 sm:px-7 lg:px-10">
        <div className="mx-auto flex w-full max-w-5xl items-center justify-end gap-2">
          {!story ? <Button type="button" variant="ghost" disabled={creating} onClick={onCancel}>{t('common.cancel')}</Button> : null}
          <Button type="button" disabled={creating || narrativeStyleLoading} onClick={() => void submit()}>
            {creating || narrativeStyleLoading ? <Loader2 className="size-4 animate-spin" /> : <Play className="size-4" />}
            {creating ? t('storyPicker.setup.starting') : narrativeStyleLoading ? t('common.loading') : t('storyPicker.setup.start')}
          </Button>
        </div>
      </footer>
    </div>
  )
}

function Field({ label, hint, className, children }: { label: string; hint?: string; className?: string; children: React.ReactNode }) {
  return <label className={cn('block text-xs text-muted-foreground', className)}><span className="mb-1 block font-medium text-foreground">{label}</span>{children}{hint ? <span className="mt-1 block text-[11px] leading-4 text-muted-foreground">{hint}</span> : null}</label>
}

function initialSettings(story: StorySummary | undefined, director: StoryDirector | undefined, recentTellerID?: string): StorySetupSettings {
  const moduleRefs = { ...(story?.module_refs || director?.module_refs || {}) }
  if (!story && recentTellerID) moduleRefs.narrative_style_id = recentTellerID
  const imageSettings = normalizeStoryImageSettings(story?.image_settings || { mode: 'manual', interval_turns: 3, preset_id: moduleRefs.image_preset_id || 'game-cg' })
  return {
    customAgentId: '',
    planningEnabled: story ? story.planning_mode === 'enabled' : true,
    moduleRefs,
    replyTargetChars: story?.reply_target_chars || DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS,
    choiceCount: story?.choice_count || DEFAULT_INTERACTIVE_CHOICE_COUNT,
    imageSettings,
    checkSettings: story?.check_settings || { difficulty_shift: 0, roll_modifier: 0 },
    stateSchemaMode: story?.state_schema_policy?.mode || 'adapt_template',
  }
}

function validateDraft(protagonist: StoryProtagonist, opening: StoryOpeningConfig, loreItems: LoreItem[], t: ReturnType<typeof useTranslation>['t']): string {
  if (protagonist.mode === 'custom' && !protagonist.name?.trim()) return t('storyPicker.setup.protagonist.custom.nameRequired')
  if (protagonist.mode === 'lore' && !protagonist.source_lore_item_id?.trim()) return t('storyPicker.setup.protagonist.lore.required')
  if (protagonist.mode === 'default' && !loreItems.some((item) => item.enabled && item.type === 'character')) return t('storyPicker.setup.protagonist.lore.emptyRequired')
  if (opening.mode === 'preset' && !opening.preset_text?.trim()) return t('storyPicker.setup.opening.presetRequired')
  if (opening.mode === 'custom' && !opening.custom_text?.trim()) return t('storyPicker.setup.opening.customRequired')
  return ''
}

function defaultStoryProtagonist(loreItems: LoreItem[]): StoryProtagonist {
  const tagged = loreItems.find((item) => item.enabled && item.type === 'character' && item.tags.some((tag) => {
    const normalized = tag.trim().toLowerCase()
    return normalized === '主角' || normalized === 'protagonist'
  }))
  if (!tagged) return { mode: 'default' }
  return {
    mode: 'lore',
    name: tagged.name,
    profile: tagged.content || tagged.brief_description,
    source_lore_item_id: tagged.id,
    source_lore_updated_at: tagged.updated_at,
  }
}

function protagonistForSubmit(protagonist: StoryProtagonist): StoryProtagonist {
  if (protagonist.mode === 'custom') return { mode: 'custom', name: protagonist.name?.trim(), profile: protagonist.profile?.trim() }
  if (protagonist.mode === 'lore') return { mode: 'lore', source_lore_item_id: protagonist.source_lore_item_id?.trim() }
  return { mode: 'default' }
}

function openingForSubmit(opening: StoryOpeningConfig): StoryOpeningConfig {
  if (opening.mode === 'preset') return { mode: 'preset', preset_id: opening.preset_id?.trim(), preset_text: truncateStoryOpeningText(opening.preset_text || '') }
  if (opening.mode === 'custom') return { mode: 'custom', custom_text: truncateStoryOpeningText(opening.custom_text || '') }
  return { mode: 'ai' }
}

function sameProtagonist(left: StoryProtagonist, right: StoryProtagonist): boolean {
  return JSON.stringify(protagonistForSubmit(left)) === JSON.stringify(protagonistForSubmit(right))
}

function defaultStoryTitle(stories: StorySummary[], t: (key: string, options?: Record<string, unknown>) => string) {
  return stories.length === 0 ? t('storyPicker.firstTitle') : t('storyPicker.numberedTitle', { number: stories.length + 1 })
}
