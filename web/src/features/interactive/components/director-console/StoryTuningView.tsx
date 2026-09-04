import { Bot, Dices, ImagePlus, UserRound } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Switch } from '@/components/ui/switch'
import { getActorStates, getEventPackages, getRuleSystems } from '../../api'
import { normalizeStoryCheckSettings } from '../../check-settings'
import { gamePlanningTemplateName } from '../../game-planning'
import { normalizeImageIntervalTurns, normalizeStoryImageSettings } from '../../image-settings'
import { narrativeStyleName } from '../../narrative-style'
import { MAX_INTERACTIVE_CHOICE_COUNT, MIN_INTERACTIVE_CHOICE_COUNT } from '../../opening'
import type {
  ActorStateModule,
  EventPackageModule,
  GamePlanningTemplate,
  ImagePreset,
  InteractiveStoryUpdateInput,
  RuleSystemModule,
  StoryCheckSettings,
  StoryDirectorModuleRefs,
  StorySummary,
  Teller,
} from '../../types'
import { StateDisplayPreferenceMenu } from '../story-state/StateDisplayPreferenceMenu'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'
import { ReplyLengthSetting } from './ReplyLengthSetting'
import { ControlSection, NumberSettingInput, TuningLinkButton, TuningRow, TuningSelect, type TuningSelectOption } from './StoryTuningControls'
import { EventPackagesRow, ModuleSelectRow } from './StoryTuningModuleRows'

type ModuleIDKey = 'narrative_style_id' | 'rule_system_id' | 'actor_state_id' | 'image_preset_id'
type ModuleDisabledKey = 'narrative_style_disabled' | 'rule_system_disabled' | 'actor_state_disabled' | 'image_preset_disabled'

export interface StoryTuningViewProps {
  story?: StorySummary
  planningTemplates: GamePlanningTemplate[]
  tellers: Teller[]
  imagePresets: ImagePreset[]
  stateDisplayPreference: StoryStateDisplayPreference
  onStateDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  onPlanningTemplateChange?: (templateId: string) => void | Promise<void>
  onUpdate?: (input: InteractiveStoryUpdateInput) => void | Promise<void>
  onOpenPresets?: () => void
}

const DEFAULT_MODULE_REFS: StoryDirectorModuleRefs = {
  narrative_style_id: 'rhythm',
  event_package_ids: ['default'],
  rule_system_id: 'default',
  actor_state_id: 'default',
  image_preset_id: 'game-cg',
}

export function StoryTuningView({
  story,
  planningTemplates,
  tellers,
  imagePresets,
  stateDisplayPreference,
  onStateDisplayPreferenceChange,
  onPlanningTemplateChange,
  onUpdate,
  onOpenPresets,
}: StoryTuningViewProps) {
  const { t } = useTranslation()
  const [savingKey, setSavingKey] = useState('')
  const [eventPackages, setEventPackages] = useState<EventPackageModule[]>([])
  const [ruleSystems, setRuleSystems] = useState<RuleSystemModule[]>([])
  const [actorStates, setActorStates] = useState<ActorStateModule[]>([])

  useEffect(() => {
    let cancelled = false
    void Promise.all([getEventPackages(), getRuleSystems(), getActorStates()])
      .then(([events, rules, states]) => {
        if (cancelled) return
        setEventPackages(events)
        setRuleSystems(rules)
        setActorStates(states)
      })
      .catch((error) => console.error('[story-console] Failed to load story-owned tuning resources', error))
    return () => {
      cancelled = true
    }
  }, [])

  const refs = { ...DEFAULT_MODULE_REFS, ...(story?.module_refs || {}) }
  const imageSettings = normalizeStoryImageSettings(story?.image_settings)
  const checkSettings = normalizeStoryCheckSettings(story?.check_settings)
  const structureLocked = (story?.turn_count || 0) > 0
  const disabled = !story || !onUpdate || Boolean(savingKey)

  const save = async (key: string, input: InteractiveStoryUpdateInput) => {
    if (!story || !onUpdate || savingKey) return
    setSavingKey(key)
    try {
      await onUpdate(input)
    } catch (error) {
      console.error('[story-console] Failed to update story tuning', { storyId: story.id, key, error })
      toast.error(t('directorPanel.tuning.saveFailed'))
    } finally {
      setSavingKey('')
    }
  }

  const selectPlanningTemplate = async (templateId: string) => {
    if (!story || !onPlanningTemplateChange || savingKey || templateId === story.planning_template_id) return
    setSavingKey('planning-template')
    try {
      await onPlanningTemplateChange(templateId)
    } catch (error) {
      console.error('[story-console] Failed to update planning template', { storyId: story.id, templateId, error })
      toast.error(t('directorPanel.tuning.saveFailed'))
    } finally {
      setSavingKey('')
    }
  }

  const updateModule = (
    key: string,
    idKey: ModuleIDKey,
    disabledKey: ModuleDisabledKey,
    value: string,
    extra?: (nextID: string) => InteractiveStoryUpdateInput,
    rebuildState = false,
  ) => {
    const nextRefs = cloneRefs(refs)
    nextRefs[idKey] = value
    nextRefs[disabledKey] = false
    const input: InteractiveStoryUpdateInput = { module_refs: nextRefs, ...(extra?.(value) || {}) }
    if (rebuildState && !structureLocked && story?.state_schema_policy) input.state_schema_policy = story.state_schema_policy
    void save(key, input)
  }

  const setRuleChecksEnabled = (enabled: boolean) => {
    const nextRefs = cloneRefs(refs)
    nextRefs.rule_system_disabled = !enabled
    if (enabled && !nextRefs.rule_system_id) nextRefs.rule_system_id = ruleSystems[0]?.id || 'default'
    void save('checks-enabled', { module_refs: nextRefs })
  }

  const planningOptions = useMemo<TuningSelectOption[]>(
    () => planningTemplates.map((item) => ({ id: item.id, label: gamePlanningTemplateName(item, t) })),
    [planningTemplates, t],
  )
  const narrativeOptions = useMemo<TuningSelectOption[]>(
    () => tellers.map((item) => ({ id: item.id, label: narrativeStyleName(item, t) })),
    [t, tellers],
  )
  const imageOptions = useMemo<TuningSelectOption[]>(() => imagePresets.map(moduleOption), [imagePresets])
  const ruleOptions = useMemo<TuningSelectOption[]>(() => ruleSystems.map(moduleOption), [ruleSystems])
  const stateOptions = useMemo<TuningSelectOption[]>(() => actorStates.map(moduleOption), [actorStates])
  const ruleEnabled = !refs.rule_system_disabled

  return (
    <div className="director-console__scroll h-full min-h-0 overflow-y-auto px-2.5 py-2.5">
      <div className="flex flex-col gap-2">
        <ControlSection
          icon={<Bot className="size-4" />}
          title={t('directorPanel.tuning.agent.title')}
          action={
            <TuningLinkButton
              label={t('directorPanel.tuning.editPlanning')}
              onClick={onOpenPresets}
            />
          }
        >
          <TuningRow title={t('directorPanel.tuning.agent.planningTemplate')} busy={savingKey === 'planning-template'}>
            <TuningSelect
              value={story?.planning_template_id || 'default'}
              options={planningOptions}
              label={t('directorPanel.tuning.agent.planningTemplate')}
              disabled={!story || !onPlanningTemplateChange || Boolean(savingKey)}
              onChange={(value) => void selectPlanningTemplate(value)}
            />
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.agent.planning')} busy={savingKey === 'planning'}>
            <Switch
              checked={story?.planning_mode === 'enabled'}
              disabled={disabled}
              aria-label={t('directorPanel.tuning.agent.planning')}
              onCheckedChange={(enabled) => void save('planning', { planning_mode: enabled ? 'enabled' : 'disabled' })}
            />
          </TuningRow>
          <ModuleSelectRow
            label={t('directorPanel.tuning.agent.narrativeStyle')}
            value={String(refs.narrative_style_id || '')}
            moduleDisabled={Boolean(refs.narrative_style_disabled)}
            options={narrativeOptions}
            busy={savingKey === 'narrative'}
            disabled={disabled}
            onChange={(value) => updateModule(
              'narrative',
              'narrative_style_id',
              'narrative_style_disabled',
              value,
              (nextID) => ({ story_teller_id: nextID }),
            )}
          />
          <EventPackagesRow
            refs={refs}
            options={eventPackages}
            busy={savingKey === 'events'}
            disabled={disabled}
            onChange={(event_package_ids, event_packages_disabled) => void save('events', {
              module_refs: { ...cloneRefs(refs), event_package_ids, event_packages_disabled },
            })}
          />
          <TuningRow title={t('directorPanel.tuning.agent.replyLength')} busy={savingKey === 'reply-length'}>
            <ReplyLengthSetting
              value={story?.reply_target_chars || 2000}
              label={t('directorPanel.tuning.agent.replyLength')}
              disabled={disabled}
              onCommit={(reply_target_chars) => void save('reply-length', { reply_target_chars })}
            />
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.agent.choiceCount')} busy={savingKey === 'choice-count'}>
            <NumberSettingInput
              value={story?.choice_count || 5}
              min={MIN_INTERACTIVE_CHOICE_COUNT}
              max={MAX_INTERACTIVE_CHOICE_COUNT}
              label={t('directorPanel.tuning.agent.choiceCount')}
              disabled={disabled}
              onCommit={(choice_count) => void save('choice-count', { choice_count })}
            />
          </TuningRow>
        </ControlSection>

        <ControlSection icon={<Dices className="size-4" />} title={t('directorPanel.tuning.check.title')}>
          <TuningRow title={t('directorPanel.tuning.check.enabled')} busy={savingKey === 'checks-enabled'}>
            <Switch
              checked={ruleEnabled}
              disabled={disabled}
              aria-label={t('directorPanel.tuning.check.enabled')}
              onCheckedChange={setRuleChecksEnabled}
            />
          </TuningRow>
          <ModuleSelectRow
            label={t('directorPanel.tuning.check.system')}
            value={String(refs.rule_system_id || '')}
            moduleDisabled={!ruleEnabled}
            options={ruleOptions}
            busy={savingKey === 'rule-system'}
            disabled={disabled || !ruleEnabled || structureLocked}
            locked={structureLocked}
            onChange={(value) => updateModule(
              'rule-system',
              'rule_system_id',
              'rule_system_disabled',
              value,
              undefined,
              true,
            )}
          />
          <TuningRow title={t('directorPanel.tuning.check.difficulty')} busy={savingKey === 'difficulty'}>
            <TuningSelect
              value={String(checkSettings.difficulty_shift)}
              options={difficultyOptions(t)}
              label={t('directorPanel.tuning.check.difficulty')}
              disabled={disabled || !ruleEnabled}
              onChange={(value) => void save('difficulty', {
                check_settings: { ...checkSettings, difficulty_shift: Number(value) },
              })}
            />
          </TuningRow>
          <TuningRow
            title={t('directorPanel.tuning.check.rollModifier')}
            description={t('directorPanel.tuning.check.formula', { modifier: formatSigned(checkSettings.roll_modifier) })}
            busy={savingKey === 'roll-modifier'}
          >
            <NumberSettingInput
              value={checkSettings.roll_modifier}
              min={-20}
              max={20}
              label={t('directorPanel.tuning.check.rollModifier')}
              disabled={disabled || !ruleEnabled}
              onCommit={(roll_modifier) => void save('roll-modifier', {
                check_settings: { ...checkSettings, roll_modifier },
              })}
            />
          </TuningRow>
          <TuningRow
            title={t('directorPanel.tuning.check.stateConsumption')}
            busy={savingKey === 'state-consumption'}
          >
            <TuningSelect
              value={checkSettings.rule_state_consumption_mode || 'hybrid_auto'}
              options={ruleStateConsumptionOptions(t)}
              label={t('directorPanel.tuning.check.stateConsumption')}
              disabled={disabled || !ruleEnabled}
              onChange={(value) => void save('state-consumption', {
                check_settings: {
                  ...checkSettings,
                  rule_state_consumption_mode: value as StoryCheckSettings['rule_state_consumption_mode'],
                },
              })}
            />
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.check.visibility')} busy={savingKey === 'visibility'}>
            <TuningSelect
              value={checkSettings.rule_visibility_mode || 'audit_only'}
              options={ruleVisibilityOptions(t)}
              label={t('directorPanel.tuning.check.visibility')}
              disabled={disabled || !ruleEnabled}
              onChange={(value) => void save('visibility', {
                check_settings: {
                  ...checkSettings,
                  rule_visibility_mode: value as StoryCheckSettings['rule_visibility_mode'],
                },
              })}
            />
          </TuningRow>
        </ControlSection>

        <ControlSection icon={<ImagePlus className="size-4" />} title={t('directorPanel.tuning.image.title')}>
          <TuningRow title={t('directorPanel.tuning.image.automatic')} busy={savingKey === 'image-mode'}>
            <Switch
              checked={imageSettings.mode === 'interval'}
              disabled={disabled}
              aria-label={t('directorPanel.tuning.image.automatic')}
              onCheckedChange={(automatic) => void save('image-mode', {
                image_settings: { ...imageSettings, mode: automatic ? 'interval' : 'manual' },
              })}
            />
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.image.interval')} busy={savingKey === 'image-interval'}>
            <NumberSettingInput
              value={imageSettings.interval_turns}
              min={1}
              max={50}
              label={t('directorPanel.tuning.image.interval')}
              disabled={disabled || imageSettings.mode !== 'interval'}
              onCommit={(interval_turns) => void save('image-interval', {
                image_settings: {
                  ...imageSettings,
                  interval_turns: normalizeImageIntervalTurns(interval_turns),
                },
              })}
            />
          </TuningRow>
          <ModuleSelectRow
            label={t('directorPanel.tuning.image.preset')}
            value={imageSettings.preset_id || String(refs.image_preset_id || '')}
            moduleDisabled={Boolean(refs.image_preset_disabled)}
            options={imageOptions}
            busy={savingKey === 'image-preset'}
            disabled={disabled}
            onChange={(value) => updateModule(
              'image-preset',
              'image_preset_id',
              'image_preset_disabled',
              value,
              (nextID) => ({ image_settings: { ...imageSettings, preset_id: nextID } }),
            )}
          />
        </ControlSection>

        <ControlSection icon={<UserRound className="size-4" />} title={t('directorPanel.tuning.state.title')}>
          <ModuleSelectRow
            label={t('directorPanel.tuning.state.system')}
            value={String(refs.actor_state_id || '')}
            moduleDisabled={Boolean(refs.actor_state_disabled)}
            options={stateOptions}
            busy={savingKey === 'state-system'}
            disabled={disabled || structureLocked}
            locked={structureLocked}
            onChange={(value) => updateModule(
              'state-system',
              'actor_state_id',
              'actor_state_disabled',
              value,
              undefined,
              true,
            )}
          />
          <TuningRow title={t('directorPanel.tuning.state.display')}>
            <StateDisplayPreferenceMenu
              value={stateDisplayPreference}
              onChange={onStateDisplayPreferenceChange}
            />
          </TuningRow>
        </ControlSection>
      </div>
    </div>
  )
}

function cloneRefs(refs: StoryDirectorModuleRefs): StoryDirectorModuleRefs {
  return { ...refs, event_package_ids: [...(refs.event_package_ids || [])] }
}

function moduleOption(item: { id: string; name: string }): TuningSelectOption {
  return { id: item.id, label: item.name || item.id }
}

function formatSigned(value: number): string {
  return value > 0 ? `+${value}` : String(value)
}

function difficultyOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return [-2, -1, 0, 1, 2].map((value) => ({
    id: String(value),
    label: t(`directorPanel.tuning.check.difficulty.${value}`),
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
