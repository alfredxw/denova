import { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import type { DirectorPlanDocs, EventPackageModule, ImagePreset, OpeningSelectorModule, RuleSystemModule, StoryDirector, StoryDirectorModuleRefs, Teller } from '../../types'
import { PresetConfigSectionEditor } from '../preset-config/PresetConfigSectionEditor'
import { OpeningSelectorVisualEditor, StatSystemVisualEditor, TRPGSystemVisualEditor } from '../preset-config/visual-editors'
import { DirectorModuleConsole } from './ModuleConsole'
import { consoleSectionClassName, DIRECTOR_PLANNING_TEMPLATE_KEYS, EDITOR_TABS, EMPTY_DIRECTOR_PLANNING_TEMPLATES, inputClassName, selectClassName, STORY_DIRECTOR_AGENT_MODE_OPTIONS, STORY_DIRECTOR_BRANCH_PLANNING_TURNS_FALLBACK, STORY_DIRECTOR_FAILURE_OPTIONS, STORY_DIRECTOR_MAINLINE_OPTIONS, STORY_DIRECTOR_PACING_OPTIONS, STORY_DIRECTOR_PLANNING_TEMPLATE_LIMIT, STORY_DIRECTOR_RANDOM_RATE_OPTIONS, STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT, type StoryDirectorEditorTab, type StrategySelectOption } from './constants'
import { EmptyState, Field, SectionTitle } from './shared'
import { directorResolvedEventPackages, findById, newEmptyStoryDirectorSections, normalizeBranchPlanningTurns, normalizedStoryDirectorRefs, parseDecimalInput, planningTemplateLabelKey, presetStatusLabel, strategyOptionText, strategyRateValue, utf8ByteLength, validateDirectorPlanningTemplate } from './utils'

export function StoryDirectorEditor({
  draft,
  tellers,
  eventPackages,
  ruleSystems,
  openingSelectors,
  imagePresets,
  tagDraft,
  setDraft,
  setTagDraft,
  onSave,
  onValidityChange,
}: {
  draft: StoryDirector | null
  tellers: Teller[]
  eventPackages: EventPackageModule[]
  ruleSystems: RuleSystemModule[]
  openingSelectors: OpeningSelectorModule[]
  imagePresets: ImagePreset[]
  tagDraft: string
  setDraft: (draft: StoryDirector | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const setSectionValid = usePresetSectionValidity(draft?.id || '', onValidityChange)
  const [strategyPromptOpen, setStrategyPromptOpen] = useState(false)
  const [planningTemplatesOpen, setPlanningTemplatesOpen] = useState(false)
  const [activeEditorTab, setActiveEditorTab] = useState<StoryDirectorEditorTab>('stats')
  const strategyPrompt = draft?.strategy?.prompt_markdown || ''
  const strategyPromptBytes = utf8ByteLength(strategyPrompt)
  const strategyPromptValid = strategyPromptBytes <= STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT
  const planningTemplates = draft?.strategy?.planning_templates || EMPTY_DIRECTOR_PLANNING_TEMPLATES
  const planningTemplateValidity = {
    mainline: validateDirectorPlanningTemplate(planningTemplates.mainline),
    current_event: validateDirectorPlanningTemplate(planningTemplates.current_event),
    next_branches: validateDirectorPlanningTemplate(planningTemplates.next_branches),
  }
  const planningTemplatesValid = Object.values(planningTemplateValidity).every((item) => item.valid)
  const planningTemplateTabs = DIRECTOR_PLANNING_TEMPLATE_KEYS.map((key) => ({
    key,
    label: t(`settingPanel.storyDirector.planningTemplate.${planningTemplateLabelKey(key)}`),
    value: planningTemplates[key],
    validity: planningTemplateValidity[key],
  }))

  useEffect(() => {
    setStrategyPromptOpen(false)
    setPlanningTemplatesOpen(false)
    setActiveEditorTab('stats')
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
  const updatePlanningTemplate = (key: keyof DirectorPlanDocs, value: string) => {
    updateStrategy({ planning_templates: { ...planningTemplates, [key]: value } })
  }
  const refs = normalizedStoryDirectorRefs(draft.module_refs)
  const updateModuleRef = <K extends keyof StoryDirectorModuleRefs>(key: K, value: StoryDirectorModuleRefs[K]) => {
    setDraft({
      ...draft,
      module_refs: {
        ...refs,
        [key]: value,
      },
    })
  }
  const emptySections = newEmptyStoryDirectorSections()
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
  const selectedOpeningSelector = findById(openingSelectors, refs.opening_selector_id || 'default')
  const selectedImagePreset = findById(imagePresets, refs.image_preset_id || 'game-cg')
  const selectedTeller = findById(tellers, refs.narrative_style_id || 'classic')
  const editorTabSummaries = {
    stats: t('settingPanel.storyDirector.statSystemSummary', { count: draft.stat_system?.attributes?.length || 0 }),
    trpg: t('settingPanel.storyDirector.trpgSystemSummary', { count: draft.trpg_system?.rule_templates?.length || 0 }),
    opening: t('settingPanel.storyDirector.openingSelectorSummary', { pools: draft.opening_selector?.trait_pools?.length || 0, ops: draft.opening_selector?.initial_state_ops?.length || 0 }),
    events: t('settingPanel.storyDirector.eventPackagesSummary', { packages: selectedEventPackageIDs.length, cards: selectedEventCardCount }),
  }

  return (
    <div ref={scrollRef} className="flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden">
      <div className="sticky top-0 z-20 border-b border-[var(--nova-border)] bg-[color-mix(in_srgb,var(--nova-surface)_92%,transparent)] px-4 py-3 backdrop-blur-xl">
        <div className="grid gap-3 xl:grid-cols-[minmax(180px,1fr)_minmax(260px,1.35fr)_minmax(180px,0.7fr)_auto]">
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
            <span className="inline-flex h-8 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-xs text-[var(--nova-text-faint)]">{presetStatusLabel(draft, t)}</span>
          </div>
        </div>
      </div>

      <div className="grid gap-4 p-4">
        <DirectorModuleConsole
          refs={refs}
          selectedTellerName={selectedTeller?.name || refs.narrative_style_id || 'classic'}
          selectedRuleName={selectedRuleSystem?.name || refs.rule_system_id || 'default'}
          selectedOpeningName={selectedOpeningSelector?.name || refs.opening_selector_id || 'default'}
          selectedImageName={selectedImagePreset?.name || refs.image_preset_id || 'game-cg'}
          selectedEventPackages={selectedEventPackages}
          selectedEventCardCount={selectedEventCardCount}
          tellers={tellers}
          eventPackages={eventPackages}
          ruleSystems={ruleSystems}
          openingSelectors={openingSelectors}
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
          <div className="mt-3 grid gap-3 lg:grid-cols-2 2xl:grid-cols-3">
            <Field label={t('settingPanel.field.enabled')}>
              <Select value={String(draft.strategy?.enabled !== false)} onValueChange={(value) => updateStrategy({ enabled: value === 'true' })}>
                <SelectTrigger size="sm" className={selectClassName}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  <SelectItem value="true">{t('settingPanel.enabled')}</SelectItem>
                  <SelectItem value="false">{t('settingPanel.disabled')}</SelectItem>
                </SelectContent>
              </Select>
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
            <StrategyRateSelect
              label={t('settingPanel.field.randomEventRate')}
              value={draft.strategy?.random_event_rate}
              fallbackValue="0.15"
              onChange={(random_event_rate) => updateStrategy({ random_event_rate })}
            />
            <StrategySelect
              label={t('settingPanel.storyDirector.agentMode')}
              value={draft.strategy?.director_agent_mode || ''}
              fallbackValue="triggered"
              options={STORY_DIRECTOR_AGENT_MODE_OPTIONS}
              onChange={(director_agent_mode) => updateStrategy({ director_agent_mode })}
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
                <Tabs defaultValue="mainline" className="gap-3">
                  <TabsList aria-label={t('settingPanel.storyDirector.planningTemplates')} className="h-auto w-full justify-start rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-1 group-data-horizontal/tabs:h-auto">
                    {planningTemplateTabs.map((tab) => (
                      <TabsTrigger
                        key={tab.key}
                        value={tab.key}
                        className={`min-h-8 px-3 text-xs ${tab.validity.valid ? '' : 'text-[var(--nova-danger)] data-active:text-[var(--nova-danger)]'}`}
                      >
                        {tab.label}
                      </TabsTrigger>
                    ))}
                  </TabsList>
                  {planningTemplateTabs.map((tab) => (
                    <TabsContent key={tab.key} value={tab.key} className="mt-0">
                      <PlanningTemplateTextarea label={tab.label} value={tab.value} validity={tab.validity} onChange={(value) => updatePlanningTemplate(tab.key, value)} />
                    </TabsContent>
                  ))}
                </Tabs>
                <div className="text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.storyDirector.planningTemplatesRequiredHeadings')}</div>
              </div>
            ) : null}
          </div>
        </section>

        <section className={`${consoleSectionClassName} p-4`}>
          <SectionTitle title={t('settingPanel.storyDirector.editorDeck')} description={t('settingPanel.storyDirector.editorDeckDesc')} />
          <Tabs value={activeEditorTab} onValueChange={(value) => setActiveEditorTab(value as StoryDirectorEditorTab)} className="mt-3 gap-3">
            <TabsList aria-label={t('settingPanel.storyDirector.editorDeck')} className="grid h-auto w-full grid-cols-[repeat(auto-fit,minmax(12rem,1fr))] items-stretch justify-stretch gap-1 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1 group-data-horizontal/tabs:h-auto">
              {EDITOR_TABS.map((tab) => (
                <TabsTrigger key={tab} value={tab} className="h-auto min-h-11 min-w-0 flex-col items-start justify-center gap-0.5 whitespace-normal px-3 py-2 text-left text-xs">
                  <span className="w-full truncate">{editorTabLabel(tab, t)}</span>
                  <span className="hidden w-full truncate text-[10px] text-[var(--nova-text-faint)] md:block">{editorTabSummaries[tab]}</span>
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent value="stats" className="mt-0">
              <PresetConfigSectionEditor
                sectionId="story-director.stat-system"
                resetKey={`${draft.id}:stat_system`}
                title={t('settingPanel.storyDirector.statSystem')}
                description={t('settingPanel.storyDirector.statSystemDesc')}
                value={draft.stat_system || emptySections.stat_system}
                summary={editorTabSummaries.stats}
                onChange={(stat_system) => setDraft({ ...draft, stat_system })}
                onSave={onSave}
                onValidityChange={(valid) => setSectionValid('stat_system', valid)}
              >
                {(props) => <StatSystemVisualEditor {...props} />}
              </PresetConfigSectionEditor>
            </TabsContent>
            <TabsContent value="trpg" className="mt-0">
              <PresetConfigSectionEditor
                sectionId="story-director.trpg-system"
                resetKey={`${draft.id}:trpg_system`}
                title={t('settingPanel.storyDirector.trpgSystem')}
                description={t('settingPanel.storyDirector.trpgSystemDesc')}
                value={draft.trpg_system || emptySections.trpg_system}
                summary={editorTabSummaries.trpg}
                onChange={(trpg_system) => setDraft({ ...draft, trpg_system })}
                onSave={onSave}
                onValidityChange={(valid) => setSectionValid('trpg_system', valid)}
              >
                {(props) => <TRPGSystemVisualEditor {...props} />}
              </PresetConfigSectionEditor>
            </TabsContent>
            <TabsContent value="opening" className="mt-0">
              <PresetConfigSectionEditor
                sectionId="story-director.opening-selector"
                resetKey={`${draft.id}:opening_selector`}
                title={t('settingPanel.storyDirector.openingSelector')}
                description={t('settingPanel.storyDirector.openingSelectorDesc')}
                value={draft.opening_selector || emptySections.opening_selector}
                summary={editorTabSummaries.opening}
                onChange={(opening_selector) => setDraft({ ...draft, opening_selector })}
                onSave={onSave}
                onValidityChange={(valid) => setSectionValid('opening_selector', valid)}
              >
                {(props) => <OpeningSelectorVisualEditor {...props} />}
              </PresetConfigSectionEditor>
            </TabsContent>
            <TabsContent value="events" className="mt-0">
              <EventPackageReferencePanel disabled={refs.event_packages_disabled === true} selectedPackages={selectedEventPackages} summary={editorTabSummaries.events} />
            </TabsContent>
          </Tabs>
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

function StrategyRateSelect({
  label,
  value,
  fallbackValue,
  onChange,
}: {
  label: string
  value: number | undefined
  fallbackValue: string
  onChange: (value: number) => void
}) {
  return (
    <StrategySelect
      label={label}
      value={strategyRateValue(value, fallbackValue)}
      fallbackValue={fallbackValue}
      options={STORY_DIRECTOR_RANDOM_RATE_OPTIONS}
      onChange={(next) => onChange(parseDecimalInput(next))}
    />
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

function EventPackageReferencePanel({
  disabled,
  selectedPackages,
  summary,
}: {
  disabled: boolean
  selectedPackages: Array<{ id: string; name: string; invalid?: boolean; cards: number }>
  summary: string
}) {
  const { t } = useTranslation()
  return (
    <div className="grid gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
      <SectionTitle title={t('settingPanel.storyDirector.eventPackages')} description={t('settingPanel.storyDirector.eventPackagesDesc')} badge={summary} />
      {disabled ? (
        <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.storyDirector.eventPackagesDisabled')}</div>
      ) : null}
      <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
        {selectedPackages.map((item) => (
          <div key={item.id} className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2">
            <div className="truncate text-xs text-[var(--nova-text)]">{item.name}</div>
            <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">
              {item.invalid ? `${t('settingPanel.invalid')} · ` : ''}{t('settingPanel.eventPackage.summaryCount', { count: item.cards })}
            </div>
          </div>
        ))}
      </div>
    </div>
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

function editorTabLabel(tab: StoryDirectorEditorTab, t: (key: string) => string) {
  if (tab === 'events') return t('settingPanel.storyDirector.editorTab.events')
  if (tab === 'trpg') return t('settingPanel.storyDirector.editorTab.trpg')
  if (tab === 'opening') return t('settingPanel.storyDirector.editorTab.opening')
  return t('settingPanel.storyDirector.editorTab.stats')
}
