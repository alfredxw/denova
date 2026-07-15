import { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, StoryDirectorModuleRefs, Teller } from '../../types'
import { PresetMetadataPanel } from '../preset-config/PresetEditorChrome'
import { BooleanSwitchField } from '../setting-panel/BooleanSwitchField'
import { DirectorModuleConsole } from './ModuleConsole'
import { consoleSectionClassName, DIRECTOR_AGENT_BRIEF_REQUIRED_HEADINGS, EMPTY_DIRECTOR_PLANNING_TEMPLATES, inputClassName, selectClassName, STORY_DIRECTOR_AGENT_MODE_OPTIONS, STORY_DIRECTOR_BRANCH_PLANNING_TURNS_FALLBACK, STORY_DIRECTOR_EVENT_FREQUENCY_OPTIONS, STORY_DIRECTOR_FAILURE_OPTIONS, STORY_DIRECTOR_MAINLINE_OPTIONS, STORY_DIRECTOR_PACING_OPTIONS, STORY_DIRECTOR_PLANNING_TEMPLATE_LIMIT, STORY_DIRECTOR_RULE_STATE_CONSUMPTION_OPTIONS, STORY_DIRECTOR_RULE_VISIBILITY_OPTIONS, STORY_DIRECTOR_STATE_SCHEMA_ADAPTATION_OPTIONS, STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT, type StrategySelectOption } from './constants'
import { EmptyState, Field, SectionTitle } from './shared'
import { directorResolvedEventPackages, findById, normalizeBranchPlanningTurns, normalizedStoryDirectorRefs, presetStatusLabel, strategyOptionText, utf8ByteLength, validateDirectorPlanningTemplate } from './utils'

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
  const [strategyPromptOpen, setStrategyPromptOpen] = useState(false)
  const [planningTemplatesOpen, setPlanningTemplatesOpen] = useState(false)
  const strategyPrompt = draft?.strategy?.prompt_markdown || ''
  const strategyPromptBytes = utf8ByteLength(strategyPrompt)
  const strategyPromptValid = strategyPromptBytes <= STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT
  const planningTemplates = draft?.strategy?.planning_templates || EMPTY_DIRECTOR_PLANNING_TEMPLATES
  const planningTemplateValue = planningTemplates.plan || ''
  const agentBriefTemplateValue = planningTemplates.agent_brief || ''
  const planningTemplateValidity = validateDirectorPlanningTemplate(planningTemplateValue)
  const agentBriefTemplateValidity = validateDirectorPlanningTemplate(agentBriefTemplateValue, DIRECTOR_AGENT_BRIEF_REQUIRED_HEADINGS)
  const planningTemplatesValid = planningTemplateValidity.valid && agentBriefTemplateValidity.valid

  useEffect(() => {
    setStrategyPromptOpen(false)
    setPlanningTemplatesOpen(false)
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

  useEffect(() => {
    setSectionValid('planning_templates', planningTemplatesValid)
  }, [draft?.id, planningTemplatesValid, setSectionValid])

  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noStoryDirectorSelected')} description={t('settingPanel.editor.noStoryDirectorSelectedDesc')} />
  }

  const updateStrategy = (patch: Partial<StoryDirector['strategy']>) => {
    setDraft({
      ...draft,
      strategy: {
        ...(draft.strategy || {}),
        enabled: draft.strategy?.enabled !== false,
        ...patch,
      },
    })
  }
  const updatePlanningTemplate = (key: 'plan' | 'agent_brief', value: string) => {
    updateStrategy({ planning_templates: { ...planningTemplates, [key]: value } })
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
  const selectedTeller = findById(tellers, refs.narrative_style_id || 'classic')

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
          selectedTellerName={selectedTeller?.name || refs.narrative_style_id || 'classic'}
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
            badge={strategyPrompt.trim() ? t('settingPanel.storyDirector.strategyPromptEnabled') : undefined}
          />
          <div
            className="mt-3 grid gap-3"
            style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 220px), 1fr))' }}
          >
            <BooleanSwitchField label={t('settingPanel.field.enabled')} checked={draft.strategy?.enabled !== false} onCheckedChange={(enabled) => updateStrategy({ enabled })} />
            <StrategySelect
              label={t('settingPanel.storyDirector.agentMode')}
              value={draft.strategy?.director_agent_mode || ''}
              fallbackValue="triggered"
              options={STORY_DIRECTOR_AGENT_MODE_OPTIONS}
              onChange={(director_agent_mode) => updateStrategy({ director_agent_mode })}
            />
            <StrategySelect
              label={t('settingPanel.storyDirector.stateSchemaAdaptation')}
              value={draft.strategy?.state_schema_adaptation_mode === 'auto' ? 'after_opening' : draft.strategy?.state_schema_adaptation_mode || ''}
              fallbackValue="after_opening"
              options={STORY_DIRECTOR_STATE_SCHEMA_ADAPTATION_OPTIONS}
              onChange={(state_schema_adaptation_mode) => updateStrategy({ state_schema_adaptation_mode })}
            />
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
            <Field label={t('settingPanel.storyDirector.branchPlanningTurns')}>
              <Input
                className={inputClassName}
                type="number"
                min={1}
                max={12}
                value={draft.strategy?.branch_planning_turns || STORY_DIRECTOR_BRANCH_PLANNING_TURNS_FALLBACK}
                onChange={(event) => updateStrategy({ branch_planning_turns: normalizeBranchPlanningTurns(event.target.value) })}
              />
              <span className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.storyDirector.branchPlanningTurnsDesc')}</span>
            </Field>
            <StrategySelect
              label={t('settingPanel.orchestration.mainlineStrength')}
              value={draft.strategy?.mainline_strength || ''}
              fallbackValue="soft_guidance"
              options={STORY_DIRECTOR_MAINLINE_OPTIONS}
              onChange={(mainline_strength) => updateStrategy({ mainline_strength })}
            />
            <StrategySelect
              label={t('settingPanel.orchestration.failurePolicy')}
              value={draft.strategy?.failure_policy || ''}
              fallbackValue="reversible"
              options={STORY_DIRECTOR_FAILURE_OPTIONS}
              onChange={(failure_policy) => updateStrategy({ failure_policy })}
            />
            <StrategySelect
              label={t('settingPanel.orchestration.pacingCurve')}
              value={draft.strategy?.pacing_curve || ''}
              fallbackValue="progressive"
              options={STORY_DIRECTOR_PACING_OPTIONS}
              onChange={(pacing_curve) => updateStrategy({ pacing_curve })}
            />
			<StrategySelect
				label={t('settingPanel.storyDirector.eventFrequency')}
				value={draft.strategy?.event_frequency || ''}
				fallbackValue="balanced"
				options={STORY_DIRECTOR_EVENT_FREQUENCY_OPTIONS}
				onChange={(event_frequency) => updateStrategy({ event_frequency })}
            />
          </div>

          <div className="mt-3 grid gap-2">
            <DisclosureButton
              open={strategyPromptOpen}
              title={t('settingPanel.storyDirector.strategyPrompt')}
              description={t('settingPanel.storyDirector.strategyPromptDesc')}
              meta={t('settingPanel.storyDirector.strategyPromptBytes', { bytes: strategyPromptBytes, limit: STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT })}
              invalid={!strategyPromptValid}
              onClick={() => setStrategyPromptOpen((open) => !open)}
            />
            {strategyPromptOpen ? (
              <div className="grid gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
                <Textarea
                  autoResize={false}
                  className="nova-field min-h-40 resize-y text-xs focus-visible:ring-0"
                  value={strategyPrompt}
                  onChange={(event) => updateStrategy({ prompt_markdown: event.target.value })}
                  placeholder={t('settingPanel.storyDirector.strategyPromptPlaceholder')}
                />
                {strategyPromptValid ? (
                  <div className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.storyDirector.strategyPromptPriority')}</div>
                ) : (
                  <div className="rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] leading-5 text-[var(--nova-danger)]">
                    {t('settingPanel.storyDirector.strategyPromptTooLong', { bytes: strategyPromptBytes, limit: STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT })}
                  </div>
                )}
              </div>
            ) : null}

            <DisclosureButton
              open={planningTemplatesOpen}
              title={t('settingPanel.storyDirector.planningTemplates')}
              description={t('settingPanel.storyDirector.planningTemplatesDesc')}
              meta={planningTemplatesValid ? t('settingPanel.storyDirector.planningTemplatesValid') : t('settingPanel.storyDirector.planningTemplatesInvalid')}
              invalid={!planningTemplatesValid}
              onClick={() => setPlanningTemplatesOpen((open) => !open)}
            />
            {planningTemplatesOpen ? (
              <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
                <div className="grid gap-3 lg:grid-cols-2">
                  <PlanningTemplateTextarea label={t('settingPanel.storyDirector.planningTemplate.plan')} value={planningTemplateValue} validity={planningTemplateValidity} onChange={(value) => updatePlanningTemplate('plan', value)} />
                  <PlanningTemplateTextarea label={t('settingPanel.storyDirector.planningTemplate.agentBrief')} value={agentBriefTemplateValue} validity={agentBriefTemplateValidity} onChange={(value) => updatePlanningTemplate('agent_brief', value)} />
                </div>
                <div className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.storyDirector.planningTemplatesRequiredHeadings')}</div>
              </div>
            ) : null}
          </div>
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
        </SelectContent>
      </Select>
      <span className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{selectedDescription}</span>
    </Field>
  )
}

function DisclosureButton({
  open,
  title,
  description,
  meta,
  invalid,
  onClick,
}: {
  open: boolean
  title: string
  description: string
  meta: string
  invalid?: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      className="flex min-h-10 w-full items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-left text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
      onClick={onClick}
      aria-expanded={open}
    >
      <ChevronRight className={`h-3.5 w-3.5 shrink-0 transition-transform ${open ? 'rotate-90' : ''}`} />
      <span className="min-w-0 flex-1">
        <span className="block font-medium text-[var(--nova-text)]">{title}</span>
        <span className="block text-[11px] leading-5 text-[var(--nova-text-faint)]">{description}</span>
      </span>
      <span className={`shrink-0 text-[11px] ${invalid ? 'text-[var(--nova-danger)]' : 'text-[var(--nova-text-faint)]'}`}>{meta}</span>
    </button>
  )
}

function PlanningTemplateTextarea({ label, value, validity, onChange }: {
  label: string
  value: string
  validity: ReturnType<typeof validateDirectorPlanningTemplate>
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const hasError = !validity.valid
  return (
    <label className="grid gap-1.5">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
        <span className={`text-[11px] ${hasError ? 'text-[var(--nova-danger)]' : 'text-[var(--nova-text-faint)]'}`}>
          {t('settingPanel.storyDirector.planningTemplateBytes', { bytes: validity.bytes, limit: STORY_DIRECTOR_PLANNING_TEMPLATE_LIMIT })}
        </span>
      </div>
      <Textarea
        autoResize={false}
        minRows={20}
        className="nova-field min-h-[calc(20*1.25rem+1rem)] resize-y font-mono text-xs leading-5 focus-visible:ring-0"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      {validity.missingHeadings.length ? (
        <div className="rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] leading-5 text-[var(--nova-danger)]">
          {t('settingPanel.storyDirector.planningTemplateMissingHeadings', { headings: validity.missingHeadings.join(' / ') })}
        </div>
      ) : null}
    </label>
  )
}

function usePresetSectionValidity(resetKey: string, onValidityChange?: (valid: boolean) => void) {
  const [validity, setValidity] = useState<Record<string, boolean>>({})

  useEffect(() => {
    setValidity({})
  }, [resetKey])

  useEffect(() => {
    onValidityChange?.(Object.values(validity).every((valid) => valid !== false))
  }, [onValidityChange, validity])

  return useCallback((section: string, valid: boolean) => {
    setValidity((current) => {
      if (current[section] === valid) return current
      return { ...current, [section]: valid }
    })
  }, [])
}
