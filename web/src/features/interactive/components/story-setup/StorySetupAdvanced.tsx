import { Bot, Dices, ImagePlus, UserRound } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { CustomAgentSelect } from '@/features/agents/CustomAgentSelect'
import { fetchSettings } from '@/features/settings/api'
import { buildModelProfileOptions } from '@/features/settings/model-profile-options'
import { THINKING_LEVELS, type ThinkingLevel } from '@/features/settings/thinking-levels'
import type { LayeredSettings } from '@/features/settings/types'
import { getActorStates, getEventPackages, getRuleSystems } from '../../api'
import { normalizeImageIntervalTurns } from '../../image-settings'
import { narrativeStyleName } from '../../narrative-style'
import { MAX_INTERACTIVE_CHOICE_COUNT, MIN_INTERACTIVE_CHOICE_COUNT } from '../../opening'
import type {
  ActorStateModule,
  EventPackageModule,
  ImagePreset,
  RuleSystemModule,
  StoryCheckSettings,
  StoryDirectorModuleRefs,
  StoryImageSettings,
  StoryStateSchemaMode,
  Teller,
} from '../../types'
import { ReplyLengthSetting } from '../director-console/ReplyLengthSetting'
import { ControlSection, NumberSettingInput, TuningLinkButton, TuningRow, TuningSelect, type TuningSelectOption } from '../director-console/StoryTuningControls'
import { EventPackagesRow, ModuleSelectRow } from '../director-console/StoryTuningModuleRows'

export interface StorySetupSettings {
  customAgentId: string
  modelProfileId: string
  thinkingLevel: ThinkingLevel
  planningEnabled: boolean
  moduleRefs: StoryDirectorModuleRefs
  replyTargetChars: number
  choiceCount: number
  imageSettings: StoryImageSettings
  checkSettings: StoryCheckSettings
  stateSchemaMode: StoryStateSchemaMode
}

interface StorySetupAdvancedProps {
  projectId: string
  newStory: boolean
  tellers: Teller[]
  imagePresets: ImagePreset[]
  value: StorySetupSettings
  onChange: (value: StorySetupSettings) => void
  runtimeConfigLoading?: boolean
  runtimeConfigError?: string | null
  onRuntimeConfigReload?: () => void
  onNarrativeStyleChange?: (id: string) => void | Promise<unknown>
  onOpenPresets?: () => void
}

type ModuleIDKey = 'narrative_style_id' | 'rule_system_id' | 'actor_state_id' | 'image_preset_id'
type ModuleDisabledKey = 'narrative_style_disabled' | 'rule_system_disabled' | 'actor_state_disabled' | 'image_preset_disabled'

export function StorySetupAdvanced({
  projectId,
  newStory,
  tellers,
  imagePresets,
  value,
  onChange,
  runtimeConfigLoading = false,
  runtimeConfigError,
  onRuntimeConfigReload,
  onNarrativeStyleChange,
  onOpenPresets,
}: StorySetupAdvancedProps) {
  const { t } = useTranslation()
  const [eventPackages, setEventPackages] = useState<EventPackageModule[]>([])
  const [ruleSystems, setRuleSystems] = useState<RuleSystemModule[]>([])
  const [actorStates, setActorStates] = useState<ActorStateModule[]>([])
  const [modelCatalog, setModelCatalog] = useState<LayeredSettings | null>(null)
  const [modelCatalogLoading, setModelCatalogLoading] = useState(true)
  const [modelCatalogFailed, setModelCatalogFailed] = useState(false)
  const modelCatalogRequestRef = useRef(0)
  const refs = value.moduleRefs

  useEffect(() => {
    let cancelled = false
    void Promise.all([getEventPackages(), getRuleSystems(), getActorStates()])
      .then(([events, rules, states]) => {
        if (cancelled) return
        setEventPackages(events)
        setRuleSystems(rules)
        setActorStates(states)
      })
      .catch((error) => console.error('[story-setup] Failed to load initialization resources', error))
    return () => {
      cancelled = true
    }
  }, [])

  const loadModelCatalog = useCallback(() => {
    const request = ++modelCatalogRequestRef.current
    setModelCatalogLoading(true)
    setModelCatalogFailed(false)
    void fetchSettings()
      .then((settings) => {
        if (request === modelCatalogRequestRef.current) setModelCatalog(settings)
      })
      .catch((error) => {
        if (request !== modelCatalogRequestRef.current) return
        console.error('[story-setup] Failed to load model profiles', error)
        setModelCatalogFailed(true)
      })
      .finally(() => {
        if (request === modelCatalogRequestRef.current) setModelCatalogLoading(false)
      })
  }, [])

  useEffect(() => {
    loadModelCatalog()
    return () => { modelCatalogRequestRef.current += 1 }
  }, [loadModelCatalog])

  const narrativeOptions = useMemo<TuningSelectOption[]>(
    () => tellers.map((item) => ({ id: item.id, label: narrativeStyleName(item, t) })),
    [t, tellers],
  )
  const imageOptions = useMemo<TuningSelectOption[]>(() => imagePresets.map(moduleOption), [imagePresets])
  const ruleOptions = useMemo<TuningSelectOption[]>(() => ruleSystems.map(moduleOption), [ruleSystems])
  const stateOptions = useMemo<TuningSelectOption[]>(() => actorStates.map(moduleOption), [actorStates])
  const modelOptions = useMemo<TuningSelectOption[]>(() => {
    const options: TuningSelectOption[] = buildModelProfileOptions(modelCatalog, t)
      .map(({ id, label }) => ({ id, label }))
    const currentID = value.modelProfileId.trim()
    if (currentID && !options.some((option) => option.id === currentID)) {
      options.unshift({ id: currentID, label: currentID })
    }
    return options
  }, [modelCatalog, t, value.modelProfileId])
  const ruleEnabled = !refs.rule_system_disabled
  const modelSelectionUnavailable = runtimeConfigLoading || Boolean(runtimeConfigError) || modelCatalogLoading || modelCatalogFailed
  const runtimeSelectionUnavailable = runtimeConfigLoading || Boolean(runtimeConfigError)
  let modelDescription = t('storyPicker.setup.model.description')
  if (runtimeConfigLoading) modelDescription = t('storyPicker.setup.model.loading')
  if (modelCatalogFailed) modelDescription = t('storyPicker.setup.model.catalogFailed')
  if (runtimeConfigError) modelDescription = t('storyPicker.setup.model.loadFailed')

  const patch = (next: Partial<StorySetupSettings>) => onChange({ ...value, ...next })
  const patchRefs = (next: StoryDirectorModuleRefs) => patch({ moduleRefs: next })
  const updateModule = (idKey: ModuleIDKey, disabledKey: ModuleDisabledKey, selected: string) => {
    const next = cloneRefs(refs)
    next[idKey] = selected
    next[disabledKey] = false
    const nextID = String(next[idKey] || '')
    if (idKey === 'image_preset_id') {
      patch({ moduleRefs: next, imageSettings: { ...value.imageSettings, preset_id: nextID } })
    } else {
      patchRefs(next)
    }
    if (idKey === 'narrative_style_id' && nextID) void onNarrativeStyleChange?.(nextID)
  }
  const setRuleChecksEnabled = (enabled: boolean) => {
    const next = cloneRefs(refs)
    next.rule_system_disabled = !enabled
    if (enabled && !next.rule_system_id) {
      next.rule_system_id = String(ruleSystems[0]?.id || 'default')
    }
    patchRefs(next)
  }
  const setStateSchemaMode = (mode: StoryStateSchemaMode) => {
    const next = cloneRefs(refs)
    next.actor_state_disabled = mode === 'generate'
    patch({ stateSchemaMode: mode, moduleRefs: next })
  }

  return (
    <div className="grid gap-3 lg:grid-cols-2">
      <ControlSection
        icon={<Bot className="size-4" />}
        title={t('directorPanel.tuning.agent.title')}
        action={<TuningLinkButton label={t('directorPanel.tuning.editPresets')} onClick={onOpenPresets} />}
      >
        {newStory ? (
          <TuningRow title={t('agents.custom.select')} description={t('agents.custom.switchNote')}>
            <CustomAgentSelect
              projectId={projectId}
              runtimeKind="interactive_story"
              value={value.customAgentId}
              onValueChange={(customAgentId) => patch({ customAgentId: customAgentId || '' })}
              className="director-control-select h-7 w-[min(10rem,60cqw)]"
            />
          </TuningRow>
        ) : null}
        <TuningRow
          title={t('storyPicker.setup.model.profile')}
          description={modelDescription}
          busy={runtimeConfigLoading || modelCatalogLoading}
          disabled={modelSelectionUnavailable}
        >
          {runtimeConfigError || modelCatalogFailed ? (
            <Button
              type="button"
              variant="ghost"
              size="xs"
              onClick={() => {
                if (runtimeConfigError) onRuntimeConfigReload?.()
                if (modelCatalogFailed) loadModelCatalog()
              }}
            >
              {t('common.retry')}
            </Button>
          ) : (
            <TuningSelect
              value={value.modelProfileId}
              options={modelOptions}
              label={t('storyPicker.setup.model.profile')}
              disabled={modelSelectionUnavailable}
              onChange={(modelProfileId) => patch({ modelProfileId })}
            />
          )}
        </TuningRow>
        <TuningRow title={t('storyPicker.setup.model.thinking')} disabled={runtimeSelectionUnavailable}>
          <TuningSelect
            value={value.thinkingLevel}
            options={thinkingLevelOptions(t)}
            label={t('storyPicker.setup.model.thinking')}
            disabled={runtimeSelectionUnavailable}
            onChange={(thinkingLevel) => patch({ thinkingLevel: thinkingLevel as ThinkingLevel })}
          />
        </TuningRow>
        <TuningRow title={t('directorPanel.tuning.agent.planning')}>
          <Switch
            checked={value.planningEnabled}
            aria-label={t('directorPanel.tuning.agent.planning')}
            onCheckedChange={(planningEnabled) => patch({ planningEnabled })}
          />
        </TuningRow>
        <ModuleSelectRow
          label={t('directorPanel.tuning.agent.narrativeStyle')}
          value={String(refs.narrative_style_id || '')}
          moduleDisabled={Boolean(refs.narrative_style_disabled)}
          options={narrativeOptions}
          busy={false}
          disabled={false}
          onChange={(selected) => updateModule('narrative_style_id', 'narrative_style_disabled', selected)}
        />
        <EventPackagesRow
          refs={refs}
          options={eventPackages}
          busy={false}
          disabled={false}
          onChange={(event_package_ids, event_packages_disabled) => patchRefs({
            ...cloneRefs(refs),
            event_package_ids,
            event_packages_disabled,
          })}
        />
        <TuningRow title={t('directorPanel.tuning.agent.replyLength')}>
          <ReplyLengthSetting
            value={value.replyTargetChars}
            label={t('directorPanel.tuning.agent.replyLength')}
            onCommit={(replyTargetChars) => patch({ replyTargetChars })}
          />
        </TuningRow>
        <TuningRow title={t('directorPanel.tuning.agent.choiceCount')}>
          <NumberSettingInput
            value={value.choiceCount}
            min={MIN_INTERACTIVE_CHOICE_COUNT}
            max={MAX_INTERACTIVE_CHOICE_COUNT}
            label={t('directorPanel.tuning.agent.choiceCount')}
            onCommit={(choiceCount) => patch({ choiceCount })}
          />
        </TuningRow>
      </ControlSection>

      <ControlSection icon={<Dices className="size-4" />} title={t('directorPanel.tuning.check.title')}>
        <TuningRow title={t('directorPanel.tuning.check.enabled')}>
          <Switch
            checked={ruleEnabled}
            aria-label={t('directorPanel.tuning.check.enabled')}
            onCheckedChange={setRuleChecksEnabled}
          />
        </TuningRow>
        <ModuleSelectRow
          label={t('directorPanel.tuning.check.system')}
          value={String(refs.rule_system_id || '')}
          moduleDisabled={!ruleEnabled}
          options={ruleOptions}
          busy={false}
          disabled={!ruleEnabled}
          onChange={(selected) => updateModule('rule_system_id', 'rule_system_disabled', selected)}
        />
        <TuningRow title={t('directorPanel.tuning.check.difficulty')}>
          <TuningSelect
            value={String(value.checkSettings.difficulty_shift)}
            options={difficultyOptions(t)}
            label={t('directorPanel.tuning.check.difficulty')}
            disabled={!ruleEnabled}
            onChange={(difficulty) => patch({
              checkSettings: { ...value.checkSettings, difficulty_shift: Number(difficulty) },
            })}
          />
        </TuningRow>
        <TuningRow
          title={t('directorPanel.tuning.check.rollModifier')}
          description={t('directorPanel.tuning.check.formula', {
            modifier: formatSigned(value.checkSettings.roll_modifier),
          })}
        >
          <NumberSettingInput
            value={value.checkSettings.roll_modifier}
            min={-20}
            max={20}
            label={t('directorPanel.tuning.check.rollModifier')}
            disabled={!ruleEnabled}
            onCommit={(rollModifier) => patch({
              checkSettings: { ...value.checkSettings, roll_modifier: rollModifier },
            })}
          />
        </TuningRow>
        <TuningRow title={t('directorPanel.tuning.check.stateConsumption')}>
          <TuningSelect
            value={value.checkSettings.rule_state_consumption_mode || 'hybrid_auto'}
            options={ruleStateConsumptionOptions(t)}
            label={t('directorPanel.tuning.check.stateConsumption')}
            disabled={!ruleEnabled}
            onChange={(ruleStateConsumptionMode) => patch({
              checkSettings: {
                ...value.checkSettings,
                rule_state_consumption_mode: ruleStateConsumptionMode as StoryCheckSettings['rule_state_consumption_mode'],
              },
            })}
          />
        </TuningRow>
        <TuningRow title={t('directorPanel.tuning.check.visibility')}>
          <TuningSelect
            value={value.checkSettings.rule_visibility_mode || 'audit_only'}
            options={ruleVisibilityOptions(t)}
            label={t('directorPanel.tuning.check.visibility')}
            disabled={!ruleEnabled}
            onChange={(ruleVisibilityMode) => patch({
              checkSettings: {
                ...value.checkSettings,
                rule_visibility_mode: ruleVisibilityMode as StoryCheckSettings['rule_visibility_mode'],
              },
            })}
          />
        </TuningRow>
      </ControlSection>

      <ControlSection icon={<ImagePlus className="size-4" />} title={t('directorPanel.tuning.image.title')}>
        <TuningRow title={t('directorPanel.tuning.image.automatic')}>
          <Switch
            checked={value.imageSettings.mode === 'interval'}
            aria-label={t('directorPanel.tuning.image.automatic')}
            onCheckedChange={(automatic) => patch({
              imageSettings: { ...value.imageSettings, mode: automatic ? 'interval' : 'manual' },
            })}
          />
        </TuningRow>
        <TuningRow title={t('directorPanel.tuning.image.interval')}>
          <NumberSettingInput
            value={value.imageSettings.interval_turns}
            min={1}
            max={50}
            label={t('directorPanel.tuning.image.interval')}
            disabled={value.imageSettings.mode !== 'interval'}
            onCommit={(intervalTurns) => patch({
              imageSettings: {
                ...value.imageSettings,
                interval_turns: normalizeImageIntervalTurns(intervalTurns),
              },
            })}
          />
        </TuningRow>
        <ModuleSelectRow
          label={t('directorPanel.tuning.image.preset')}
          value={value.imageSettings.preset_id || String(refs.image_preset_id || '')}
          moduleDisabled={Boolean(refs.image_preset_disabled)}
          options={imageOptions}
          busy={false}
          disabled={false}
          onChange={(selected) => updateModule('image_preset_id', 'image_preset_disabled', selected)}
        />
      </ControlSection>

      <ControlSection icon={<UserRound className="size-4" />} title={t('directorPanel.tuning.state.title')}>
        <ModuleSelectRow
          label={t('directorPanel.tuning.state.system')}
          value={String(refs.actor_state_id || '')}
          moduleDisabled={Boolean(refs.actor_state_disabled)}
          options={stateOptions}
          busy={false}
          disabled={value.stateSchemaMode === 'generate'}
          onChange={(selected) => updateModule('actor_state_id', 'actor_state_disabled', selected)}
        />
        <TuningRow title={t('storyPicker.setup.stateSchema.title')} description={t('storyPicker.setup.stateSchema.description')}>
          <TuningSelect
            value={value.stateSchemaMode}
            options={stateSchemaOptions(t)}
            label={t('storyPicker.setup.stateSchema.title')}
            onChange={(mode) => setStateSchemaMode(mode as StoryStateSchemaMode)}
          />
        </TuningRow>
      </ControlSection>
    </div>
  )
}

function cloneRefs(refs: StoryDirectorModuleRefs): StoryDirectorModuleRefs {
  return { ...refs, event_package_ids: [...(refs.event_package_ids || [])] }
}

function moduleOption(item: { id: string; name: string }): TuningSelectOption {
  return { id: item.id, label: item.name || item.id }
}

function difficultyOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return [-2, -1, 0, 1, 2].map((value) => ({
    id: String(value),
    label: t(`directorPanel.tuning.check.difficulty.${value}`),
  }))
}

function thinkingLevelOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return THINKING_LEVELS.map((level) => ({
    id: level,
    label: t(`chat.modelProfile.thinking.${level}`),
  }))
}

function stateSchemaOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return ['adapt_template', 'fixed_template', 'generate'].map((mode) => ({
    id: mode,
    label: t(`storyPicker.setup.stateSchema.${mode}.title`),
  }))
}

function ruleStateConsumptionOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return ['hybrid_auto', 'suggestions_only'].map((mode) => ({
    id: mode,
    label: t(`directorPanel.tuning.check.stateConsumption.${mode}`),
  }))
}

function ruleVisibilityOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return ['audit_only', 'public_roll'].map((mode) => ({
    id: mode,
    label: t(`directorPanel.tuning.check.visibility.${mode}`),
  }))
}

function formatSigned(value: number): string {
  return value > 0 ? `+${value}` : String(value)
}
