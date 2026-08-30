import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, StoryDirectorModuleRefs, Teller } from '../../types'
import { DEFAULT_NARRATIVE_STYLE_ID, narrativeStyleName } from '../../narrative-style'
import { presetSelectClassName as selectClassName } from '../preset-config/editor-styles'
import { PresetEmptyState as EmptyState } from '../preset-config/PresetEmptyState'
import { PresetMetadataPanel } from '../preset-config/PresetEditorChrome'
import { PresetField as Field } from '../preset-config/PresetField'
import { PresetSectionHeader as SectionTitle } from '../preset-config/PresetSectionHeader'
import { presetStatusLabel } from '../preset-config/preset-status'
import { usePresetSectionValidity } from '../preset-config/use-preset-section-validity'
import { DirectorModuleConsole } from './ModuleConsole'
import { consoleSectionClassName, STORY_DIRECTOR_RULE_STATE_CONSUMPTION_OPTIONS, STORY_DIRECTOR_RULE_VISIBILITY_OPTIONS, STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT, type StrategySelectOption } from './constants'
import { directorResolvedEventPackages, findById, normalizedStoryDirectorRefs, strategyOptionText, utf8ByteLength } from './utils'

export function StoryDirectorEditor({
  draft,
  tellers,
  eventPackages,
  ruleSystems,
  actorStates,
  imagePresets,
  setDraft,
  onValidityChange,
}: {
  draft: StoryDirector | null
  tellers: Teller[]
  eventPackages: EventPackageModule[]
  ruleSystems: RuleSystemModule[]
  actorStates: ActorStateModule[]
  imagePresets: ImagePreset[]
  setDraft: (draft: StoryDirector | null) => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const setSectionValid = usePresetSectionValidity(draft?.id || '', onValidityChange)
  const strategyPrompt = draft?.strategy?.prompt_markdown || ''
  const strategyPromptBytes = utf8ByteLength(strategyPrompt)
  const strategyPromptValid = strategyPromptBytes <= STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT

  useEffect(() => {
    const scrollElement = scrollRef.current
    if (scrollElement) {
      if (typeof scrollElement.scrollTo === 'function') {
        scrollElement.scrollTo({ top: 0 })
      } else {
        scrollElement.scrollTop = 0
      }
    }
  }, [draft?.id])

  useEffect(() => {
    setSectionValid('strategy_prompt', strategyPromptValid)
  }, [draft?.id, strategyPromptValid, setSectionValid])

  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noStoryDirectorSelected')} description={t('settingPanel.editor.noStoryDirectorSelectedDesc')} />
  }

  const updateStrategy = (patch: Partial<StoryDirector['strategy']>) => {
    setDraft({
      ...draft,
      strategy: {
        ...(draft.strategy || {}),
        ...patch,
      },
    })
  }
  const refs = normalizedStoryDirectorRefs(draft.module_refs)
  const updateModuleRef = <K extends keyof StoryDirectorModuleRefs>(key: K, value: StoryDirectorModuleRefs[K]) => {
    const nextRefs: StoryDirectorModuleRefs = {
      ...refs,
      [key]: value,
    }
    if (key === 'rule_system_id') {
      const selected = ruleSystems.find((item) => item.id === value)
      if (selected?.actor_state_id) {
        nextRefs.actor_state_id = selected.actor_state_id
        nextRefs.actor_state_disabled = false
      }
    }
    setDraft({
      ...draft,
      module_refs: nextRefs,
    })
  }
  const resolvedEventPackages = directorResolvedEventPackages(draft)
  const selectedEventPackageIDs = refs.event_package_ids?.length ? refs.event_package_ids : ['default']
  const selectedEventPackages = selectedEventPackageIDs.map((id) => {
    const module = eventPackages.find((item) => item.id === id)
    const resolved = resolvedEventPackages.find((item) => item.id === id)
    return {
      id,
      name: module?.name || resolved?.name || id,
      invalid: module?.invalid,
      cards: module?.events?.length ?? resolved?.events?.length ?? 0,
    }
  })
  const selectedEventCardCount = selectedEventPackages.reduce((total, item) => total + item.cards, 0)
  const selectedRuleSystem = findById(ruleSystems, refs.rule_system_id || 'default')
  const selectedActorState = findById(actorStates, refs.actor_state_id || 'default')
  const selectedImagePreset = findById(imagePresets, refs.image_preset_id || 'game-cg')
  const selectedTeller = findById(tellers, refs.narrative_style_id || DEFAULT_NARRATIVE_STYLE_ID)

  return (
    <div ref={scrollRef} className="preset-director-editor flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden">
      <PresetMetadataPanel
        name={draft.name}
        description={draft.description}
        status={presetStatusLabel(draft, t)}
        hint={draft.custom ? t('settingPanel.storyDirector.customEditable') : t('settingPanel.storyDirector.builtInCopyHint')}
        onNameChange={(name) => setDraft({ ...draft, name })}
        onDescriptionChange={(description) => setDraft({ ...draft, description })}
        sticky
      />

      <div className="grid gap-4 p-3 sm:p-4">
        <DirectorModuleConsole
          refs={refs}
          selectedTellerName={selectedTeller ? narrativeStyleName(selectedTeller, t) : refs.narrative_style_id || DEFAULT_NARRATIVE_STYLE_ID}
          selectedRuleName={selectedRuleSystem?.name || refs.rule_system_id || 'default'}
          selectedActorStateName={selectedActorState?.name || refs.actor_state_id || 'default'}
          selectedImageName={selectedImagePreset?.name || refs.image_preset_id || 'game-cg'}
          selectedEventCardCount={selectedEventCardCount}
          tellers={tellers}
          eventPackages={eventPackages}
          ruleSystems={ruleSystems}
          actorStates={actorStates}
          imagePresets={imagePresets}
          onModuleRefChange={updateModuleRef}
        />

        {draft.resolved_snapshot?.warnings?.length ? (
          <div className="grid gap-2">
            {draft.resolved_snapshot.warnings.map((warning, index) => (
              <div key={`${warning.module}-${warning.id || index}`} className="rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-danger)]">
                {t('settingPanel.storyDirector.moduleWarning', { module: warning.module, id: warning.id || '-', message: warning.message })}
              </div>
            ))}
          </div>
        ) : null}

        <section className={`${consoleSectionClassName} p-4`}>
          <SectionTitle
            title={t('settingPanel.storyDirector.strategy')}
            description={t('settingPanel.storyDirector.strategyDesc')}
            badge={strategyPrompt.trim() ? t('settingPanel.storyDirector.planningStyleConfigured') : undefined}
          />
          <div
            className="mt-3 grid gap-3"
            style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 220px), 1fr))' }}
          >
            <StrategySelect
              label={t('settingPanel.storyDirector.ruleStateConsumption')}
              value={draft.strategy?.rule_state_consumption_mode || ''}
              fallbackValue="hybrid_auto"
              options={STORY_DIRECTOR_RULE_STATE_CONSUMPTION_OPTIONS}
              onChange={(rule_state_consumption_mode) => updateStrategy({ rule_state_consumption_mode })}
            />
            <StrategySelect
              label={t('settingPanel.storyDirector.ruleVisibility')}
              value={draft.strategy?.rule_visibility_mode || ''}
              fallbackValue="audit_only"
              options={STORY_DIRECTOR_RULE_VISIBILITY_OPTIONS}
              onChange={(rule_visibility_mode) => updateStrategy({ rule_visibility_mode })}
            />
          </div>

          <Field label={t('settingPanel.storyDirector.planningStyle')} className="mt-4">
            <Textarea
              autoResize={false}
              className="nova-field min-h-40 resize-y text-xs focus-visible:ring-0"
              value={strategyPrompt}
              onChange={(event) => updateStrategy({ prompt_markdown: event.target.value })}
              placeholder={t('settingPanel.storyDirector.planningStylePlaceholder')}
            />
            <div className="mt-1 flex flex-wrap items-center justify-between gap-2 text-[11px] leading-5">
              <span className={strategyPromptValid ? 'text-[var(--nova-text-faint)]' : 'text-[var(--nova-danger)]'}>{t('settingPanel.storyDirector.strategyPromptBytes', { bytes: strategyPromptBytes, limit: STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT })}</span>
              <span className="text-[var(--nova-text-faint)]">{t('settingPanel.storyDirector.planningStyleHint')}</span>
            </div>
          </Field>
        </section>
      </div>
    </div>
  )
}
function StrategySelect({
  label,
  value,
  fallbackValue,
  options,
  onChange,
}: {
  label: string
  value: string
  fallbackValue: string
  options: readonly StrategySelectOption[]
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const selectedValue = value || fallbackValue
  const hasSelected = options.some((option) => option.value === selectedValue)
  const displayedOptions = hasSelected
    ? options
    : [
      ...options,
      {
        value: selectedValue,
        labelKey: 'settingPanel.storyDirector.strategy.custom',
        descriptionKey: 'settingPanel.storyDirector.strategy.customDesc',
      },
    ]
  const selectedOption = displayedOptions.find((option) => option.value === selectedValue) || displayedOptions[0]
  const selectedLabel = strategyOptionText(t, selectedOption.labelKey, selectedOption.value)
  const selectedDescription = strategyOptionText(t, selectedOption.descriptionKey, selectedOption.value)

  return (
    <Field label={label}>
      <Select value={selectedValue} onValueChange={onChange}>
        <SelectTrigger size="sm" className={selectClassName}>
          <SelectValue>{selectedLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent className="nova-panel min-w-72 border text-[var(--nova-text)]">
          <SelectGroup>
            {displayedOptions.map((option) => {
              const optionLabel = strategyOptionText(t, option.labelKey, option.value)
              const optionDescription = strategyOptionText(t, option.descriptionKey, option.value)
              return (
                <SelectItem key={option.value} value={option.value} textValue={optionLabel} className="items-start py-2">
                  <div className="grid gap-0.5 text-left">
                    <span className="text-xs text-[var(--nova-text)]">{optionLabel}</span>
                    <span className="text-[11px] leading-4 text-[var(--nova-text-faint)]">{optionDescription}</span>
                  </div>
                </SelectItem>
              )
            })}
          </SelectGroup>
        </SelectContent>
      </Select>
      <span className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{selectedDescription}</span>
    </Field>
  )
}
