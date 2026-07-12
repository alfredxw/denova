import { useState } from 'react'
import { Check, ChevronDown, Plus, RefreshCw, Sparkles, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Textarea } from '@/components/ui/textarea'
import { rollInteractiveActorTraits } from '../api'
import { DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS, type StoryCreateInput } from '../opening'
import type { ActorTraitRollResult, ActorTraitSelection, StoryDirector, StorySummary, Teller } from '../types'

interface StoryPickerProps {
  stories: StorySummary[]
  currentStoryId: string
  tellers: Teller[]
  storyDirectors?: StoryDirector[]
  onSelect: (storyId: string) => void
  onCreate: (input: StoryCreateInput) => void | Promise<void>
  onDelete: (storyId: string) => void
  layout?: 'inline' | 'sidebar'
}

export function StoryPicker({ stories, currentStoryId, tellers, storyDirectors = [], onSelect, onCreate, onDelete, layout = 'inline' }: StoryPickerProps) {
  const { t } = useTranslation()
  const [creating, setCreating] = useState(false)
  const [selectorOpen, setSelectorOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [origin, setOrigin] = useState('')
  const [selectedDirectorId, setSelectedDirectorId] = useState('')
  const [replyTargetChars, setReplyTargetChars] = useState(String(DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS))
  const [traitRoll, setTraitRoll] = useState<ActorTraitRollResult | null>(null)
  const [selectedTraitsByPool, setSelectedTraitsByPool] = useState<Record<string, string[]>>({})
  const [openingRolling, setOpeningRolling] = useState(false)
  const [openingRollError, setOpeningRollError] = useState('')
  const [storyCreating, setStoryCreating] = useState(false)
  const [storyCreateError, setStoryCreateError] = useState('')
  const defaultDirector = selectedDirectorId || storyDirectors[0]?.id || 'default'
  const selectedDirector = storyDirectors.find((director) => director.id === defaultDirector) || storyDirectors[0] || null
  const directorNarrativeStyleEnabled = selectedDirector?.module_refs?.narrative_style_disabled !== true
  const directorImagePresetEnabled = selectedDirector?.module_refs?.image_preset_disabled !== true
  const actorStateEnabled = selectedDirector?.module_refs?.actor_state_disabled !== true
  const defaultTeller = (directorNarrativeStyleEnabled ? selectedDirector?.module_refs?.narrative_style_id : '') || tellers[0]?.id || 'classic'
  const derivedTeller = tellers.find((teller) => teller.id === defaultTeller) || null
  const protagonistTemplate = actorStateEnabled
    ? (selectedDirector?.actor_state?.templates || []).find((template) => template.id === 'protagonist')
    : undefined
  const traitPoolById = new Map((selectedDirector?.actor_state?.trait_pools || []).map((pool) => [pool.id, pool]))
  const traitPoolRules = (protagonistTemplate?.trait_rules || []).flatMap((rule) => {
    const pool = traitPoolById.get(rule.pool_id)
    return pool ? [{ pool, rule }] : []
  })
  const sidebar = layout === 'sidebar'
  const suggestedTitle = defaultStoryTitle(stories, t)
  const selectedStory = stories.find((story) => story.id === currentStoryId) || null
  const rolledTraits = traitRoll?.traits || []
  const traitSelections = Object.entries(selectedTraitsByPool).map(([pool_id, trait_ids]) => ({ pool_id, trait_ids } satisfies ActorTraitSelection))
  const hasTraitSelections = traitSelections.some((selection) => selection.trait_ids?.length)

  const closeCreate = () => {
    setTitle('')
    setOrigin('')
    setSelectedDirectorId('')
    setReplyTargetChars(String(DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS))
    setTraitRoll(null)
    setSelectedTraitsByPool({})
    setOpeningRolling(false)
    setOpeningRollError('')
    setStoryCreating(false)
    setStoryCreateError('')
    setCreating(false)
  }

  const rollOpening = async () => {
    if (openingRolling || !protagonistTemplate || traitPoolRules.length === 0) return
    setOpeningRolling(true)
    setOpeningRollError('')
    try {
      const result = await rollInteractiveActorTraits({
        story_director_id: defaultDirector,
        actor_id: 'protagonist',
        template_id: protagonistTemplate.id,
        selections: traitSelections,
      })
      setTraitRoll(result)
    } catch (err) {
      setOpeningRollError(err instanceof Error ? err.message : t('storyPicker.openingBuilder.rollFailed'))
    } finally {
      setOpeningRolling(false)
    }
  }

  const submit = async () => {
    if (storyCreating) return
    setStoryCreating(true)
    setStoryCreateError('')
    try {
      await onCreate({
      title: title.trim() || suggestedTitle,
      origin: origin.trim(),
      story_teller_id: defaultTeller,
      story_director_id: defaultDirector,
      reply_target_chars: normalizeReplyTargetChars(replyTargetChars),
      image_settings: {
        mode: 'manual',
        interval_turns: 3,
        preset_id: directorImagePresetEnabled ? selectedDirector?.module_refs?.image_preset_id || 'game-cg' : 'game-cg',
      },
      initial_trait_rolls: traitRoll || hasTraitSelections
        ? [{ actor_id: 'protagonist', seed: traitRoll?.seed || 0, selections: traitSelections }]
        : undefined,
      })
      closeCreate()
    } catch (error) {
      setStoryCreateError(error instanceof Error ? error.message : t('storyPicker.createFailed'))
      setStoryCreating(false)
    }
  }

  const toggleOpeningTrait = (poolId: string, traitId: string, drawCount: number) => {
    if (!traitId) return
    setTraitRoll(null)
    setOpeningRollError('')
    setSelectedTraitsByPool((current) => {
      const selected = current[poolId] || []
      const next = selected.includes(traitId)
        ? selected.filter((id) => id !== traitId)
        : selected.length >= drawCount ? selected : [...selected, traitId]
      return { ...current, [poolId]: next }
    })
  }

  const selector = (
    <Popover open={selectorOpen} onOpenChange={setSelectorOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={`nova-field ${sidebar ? 'w-full' : 'w-[190px]'} justify-between px-3 py-0.5 text-xs font-normal text-[var(--nova-text)] focus:ring-0`}
          aria-label={t('storyPicker.placeholder')}
          aria-expanded={selectorOpen}
        >
          <span className="min-w-0 flex-1 truncate text-left">{selectedStory?.title || t('storyPicker.placeholder')}</span>
          <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform ${selectorOpen ? 'rotate-180' : ''}`} />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={6}
        className={`${sidebar ? 'w-[min(calc(100vw-2rem),24rem)]' : 'w-[190px]'} max-h-[min(70dvh,28rem)] overflow-y-auto rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1 text-[var(--nova-text)] shadow-[var(--nova-shadow)]`}
      >
        <div role="listbox" aria-label={t('storyPicker.placeholder')} className="space-y-1">
          {stories.length === 0 ? (
            <div className="px-2 py-2 text-xs text-[var(--nova-text-faint)]">{t('storyPicker.empty')}</div>
          ) : (
            stories.map((story) => {
              const selected = story.id === currentStoryId
              return (
                <button
                  key={story.id}
                  type="button"
                  role="option"
                  aria-selected={selected}
                  className={`flex w-full min-w-0 items-center gap-2 rounded-[var(--nova-radius)] px-2 py-1.5 text-left text-xs leading-5 ${selected ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}
                  onClick={() => {
                    setSelectorOpen(false)
                    if (story.id !== currentStoryId) onSelect(story.id)
                  }}
                >
                  <span className="min-w-0 flex-1 truncate">{story.title}</span>
                  {selected ? <Check className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : null}
                </button>
              )
            })
          )}
        </div>
        {currentStoryId ? (
          <div className="mt-1 border-t border-[var(--nova-border)] pt-1">
            <Button
              type="button"
              variant="ghost"
              size="xs"
              className="w-full justify-start gap-1.5 px-2 text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]"
              onClick={() => {
                setSelectorOpen(false)
                onDelete(currentStoryId)
              }}
              aria-label={t('storyPicker.delete')}
            >
              <Trash2 className="h-3 w-3" />
              {t('storyPicker.delete')}
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  )

  const createButton = (
    <Popover
      open={creating}
      onOpenChange={(open) => {
        if (!open) {
          closeCreate()
          return
        }
        setCreating(true)
        setTitle((current) => (current.trim() ? current : suggestedTitle))
        const nextDirectorId = selectedDirectorId || storyDirectors[0]?.id || 'default'
        setSelectedDirectorId(nextDirectorId)
      }}
    >
      <PopoverTrigger asChild>
        <Button variant="ghost" size="xs" className="nova-nav-item">
          <Plus className="h-3 w-3" />
          {t('chat.new')}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="nova-panel w-80 border p-3 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
        <div className="mb-2 text-xs font-medium">{t('storyPicker.create')}</div>
        <Input className="nova-field mb-2 text-xs" placeholder={suggestedTitle} value={title} onChange={(event) => setTitle(event.target.value)} />
        <Textarea autoResize className="nova-field mb-3 min-h-20 resize-none text-xs" placeholder={t('storyPicker.originPlaceholder')} value={origin} onChange={(event) => setOrigin(event.target.value)} />
        <div className="mb-3 grid grid-cols-1 gap-2">
          <label className="min-w-0 text-[11px] text-[var(--nova-text-faint)]">
            <span className="mb-1 block truncate">{t('storyPicker.storyDirector')}</span>
            <select className="nova-field h-8 w-full rounded-[var(--nova-radius)] px-2 text-xs text-[var(--nova-text)]" value={defaultDirector} onChange={(event) => {
              const nextDirectorId = event.target.value
              setSelectedDirectorId(nextDirectorId)
              setTraitRoll(null)
              setSelectedTraitsByPool({})
            }}>
              {(storyDirectors.length ? storyDirectors : [{ id: 'default', name: t('storyPicker.defaultStoryDirector') } as StoryDirector]).map((director) => (
                <option key={director.id} value={director.id}>{director.name || director.id}</option>
              ))}
            </select>
          </label>
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-[11px] text-[var(--nova-text-faint)]">
            <span>{t('storyPicker.directorNarrativeStyle')}</span>
            <span className="ml-1 font-medium text-[var(--nova-text-muted)]">{derivedTeller?.name || defaultTeller || t('storyPicker.defaultNarrativeStyle')}</span>
          </div>
        </div>
        <div className="mb-3">
          <div className="mb-1.5 text-[11px] text-[var(--nova-text-faint)]">{t('storyPicker.replyTargetChars')}</div>
          <Input className="nova-field text-xs" type="number" min={1} value={replyTargetChars} onChange={(event) => setReplyTargetChars(event.target.value)} placeholder={String(DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS)} />
        </div>
        <div className="mb-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
          <div className="flex items-center justify-between gap-2">
            <div className="flex min-w-0 items-center gap-1.5 text-xs font-medium text-[var(--nova-text)]">
              <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
              <span className="truncate">{t('storyPicker.openingBuilder.title')}</span>
            </div>
            <Button type="button" variant="outline" size="xs" className="gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)]" disabled={openingRolling || traitPoolRules.length === 0} onClick={() => void rollOpening()}>
              <RefreshCw className={`h-3 w-3 ${openingRolling ? 'animate-spin' : ''}`} />
              {traitRoll ? t('storyPicker.openingBuilder.reroll') : hasTraitSelections ? t('storyPicker.openingBuilder.applySelection') : t('storyPicker.openingBuilder.roll')}
            </Button>
          </div>
          {openingRollError ? <div className="mt-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] text-[var(--nova-danger)]">{openingRollError}</div> : null}
          {traitPoolRules.length ? (
            <div className="mt-2 space-y-2">
              {traitPoolRules.slice(0, 4).map(({ pool, rule }) => (
                <div key={pool.id || pool.name} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
                  <div className="mb-1 flex items-center justify-between gap-2 text-[11px] text-[var(--nova-text-faint)]">
                    <span className="min-w-0 truncate">{pool.name || pool.id || t('storyPicker.openingBuilder.availableTraits')}</span>
                    <span className="shrink-0">{(selectedTraitsByPool[pool.id] || []).length}/{rule.draw_count}</span>
                  </div>
                  <div className="flex flex-wrap gap-1">
                    {(pool.traits || []).slice(0, 12).map((trait) => {
                      const selected = (selectedTraitsByPool[pool.id] || []).includes(trait.id || '')
                      return (
                        <button
                          key={trait.id || trait.name}
                          type="button"
                          className={`max-w-full truncate rounded-[var(--nova-radius)] border px-2 py-0.5 text-[11px] transition ${selected ? 'border-[var(--nova-accent)] bg-[var(--nova-active)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]'}`}
                          title={trait.summary || trait.name}
                          onClick={() => toggleOpeningTrait(pool.id || '', trait.id || '', rule.draw_count)}
                        >
                          {trait.name || trait.id}
                        </button>
                      )
                    })}
                  </div>
                </div>
              ))}
            </div>
          ) : null}
          {traitRoll ? (
            <div className="mt-2 space-y-2">
              <div className="flex flex-wrap gap-1">
                {rolledTraits.length ? rolledTraits.map((trait) => (
                  <span key={`${trait.pool_id}:${trait.trait_id}`} className="max-w-full truncate rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 text-[11px] text-[var(--nova-text-muted)]" title={trait.summary || trait.name}>
                    {trait.name}
                  </span>
                )) : <span className="text-[11px] text-[var(--nova-text-faint)]">{t('storyPicker.openingBuilder.noTraits')}</span>}
              </div>
            </div>
          ) : traitPoolRules.length ? (
            <div className="mt-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('storyPicker.openingBuilder.empty')}</div>
          ) : (
            <div className="mt-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('storyPicker.openingBuilder.disabled')}</div>
          )}
          {hasTraitSelections ? <div className="mt-2 text-[11px] text-[var(--nova-text-faint)]">{t('storyPicker.openingBuilder.selected', { count: traitSelections.reduce((sum, selection) => sum + (selection.trait_ids?.length || 0), 0) })}</div> : null}
        </div>
        {storyCreateError ? <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] text-[var(--nova-danger)]">{storyCreateError}</div> : null}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" size="xs" disabled={storyCreating} onClick={closeCreate}>
            {t('common.cancel')}
          </Button>
          <Button size="xs" disabled={storyCreating} onClick={() => void submit()}>
            {storyCreating ? <RefreshCw className="mr-1 h-3 w-3 animate-spin" /> : null}
            {storyCreating ? t('storyPicker.adaptingStateSchema') : t('common.create')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )

  if (sidebar) {
    return (
      <div className="flex min-w-0 flex-col gap-1.5">
        <div className="flex items-center justify-between gap-2">
          <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyPicker.label')}</span>
          <div className="flex shrink-0 items-center gap-1">{createButton}</div>
        </div>
        {selector}
      </div>
    )
  }

  return (
    <div className="flex min-w-0 items-center gap-1.5">
      <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyPicker.label')}</span>
      {selector}
      {createButton}
    </div>
  )
}

function normalizeReplyTargetChars(value: string) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS
}

function defaultStoryTitle(stories: StorySummary[], t: (key: string, options?: Record<string, unknown>) => string): string {
  if (stories.length === 0) return t('storyPicker.firstTitle')

  let next = stories.length + 1
  for (const story of stories) {
    const match = story.title.trim().match(/^故事线\s*(\d+)$/)
    if (!match) continue
    next = Math.max(next, Number(match[1]) + 1)
  }
  return t('storyPicker.numberedTitle', { number: Math.max(2, next) })
}
