import { useEffect, useMemo, useState } from 'react'
import { Bot, Dices, ImagePlus, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Switch } from '@/components/ui/switch'
import { getActorStates, getEventPackages, getRuleSystems } from '../../api'
import { gamePresetName } from '../../game-preset'
import { normalizeImageIntervalTurns, normalizeStoryImageSettings } from '../../image-settings'
import { narrativeStyleName } from '../../narrative-style'
import {
  MAX_INTERACTIVE_CHOICE_COUNT,
  MIN_INTERACTIVE_CHOICE_COUNT,
} from '../../opening'
import type {
  ActorStateModule,
  EventPackageModule,
  ImagePreset,
  InteractiveStoryUpdateInput,
  RuleSystemModule,
  StoryDirector,
  StoryDirectorModuleRefs,
  StorySummary,
  Teller,
} from '../../types'
import { StateDisplayPreferenceMenu } from '../story-state/StateDisplayPreferenceMenu'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'
import { ReplyLengthSetting } from './ReplyLengthSetting'
import {
  ControlSection,
  NumberSettingInput,
  TuningLinkButton,
  TuningRow,
  TuningSelect,
  type TuningSelectOption,
  type TuningSource,
} from './StoryTuningControls'
import { EventPackagesRow, ModuleSelectRow, PRESET_VALUE } from './StoryTuningModuleRows'

type ModuleIDKey = 'narrative_style_id' | 'rule_system_id' | 'actor_state_id' | 'image_preset_id'
type ModuleDisabledKey = 'narrative_style_disabled' | 'rule_system_disabled' | 'actor_state_disabled' | 'image_preset_disabled'

export interface StoryTuningViewProps {
  story?: StorySummary
  directors: StoryDirector[]
  tellers: Teller[]
  imagePresets: ImagePreset[]
  stateDisplayPreference: StoryStateDisplayPreference
  onStateDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  onDirectorChange?: (directorId: string) => void | Promise<void>
  onUpdate?: (input: InteractiveStoryUpdateInput) => void | Promise<void>
  onOpenPresets?: () => void
}

export function StoryTuningView({
  story,
  directors,
  tellers,
  imagePresets,
  stateDisplayPreference,
  onStateDisplayPreferenceChange,
  onDirectorChange,
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
      .catch((error) => console.error('[story-console] Failed to load tuning resources', error))
    return () => { cancelled = true }
  }, [])

  const director = directors.find((item) => item.id === story?.story_director_id) || directors[0]
  const presetRefs = director?.module_refs || {}
  const refs = story?.module_refs || presetRefs
  const imageSettings = normalizeStoryImageSettings(story?.image_settings)
  const checkSettings = {
    difficulty_shift: story?.check_settings?.difficulty_shift || 0,
    roll_modifier: story?.check_settings?.roll_modifier || 0,
  }
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

  const selectDirector = async (directorId: string) => {
    if (!story || !onDirectorChange || savingKey || directorId === story.story_director_id) return
    setSavingKey('director')
    try {
      await onDirectorChange(directorId)
    } catch (error) {
      console.error('[story-console] Failed to update Game Preset', { storyId: story.id, directorId, error })
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
    const nextID = value === PRESET_VALUE ? String(presetRefs[idKey] || '') : value
    nextRefs[idKey] = nextID
    nextRefs[disabledKey] = value === PRESET_VALUE ? Boolean(presetRefs[disabledKey]) : false
    const input: InteractiveStoryUpdateInput = { module_refs: nextRefs, ...(extra?.(nextID) || {}) }
    if (rebuildState && !structureLocked && story?.state_schema_policy) {
      // The backend rebuild resolves modules from the director supplied in this request.
      input.story_director_id = story.story_director_id
      input.state_schema_policy = story.state_schema_policy
    }
    void save(key, input)
  }

  const setRuleChecksEnabled = (enabled: boolean) => {
    const nextRefs = cloneRefs(refs)
    nextRefs.rule_system_disabled = !enabled
    if (enabled && !nextRefs.rule_system_id) nextRefs.rule_system_id = String(presetRefs.rule_system_id || ruleSystems[0]?.id || '')
    void save('checks-enabled', { module_refs: nextRefs })
  }

  const setImageMode = (automatic: boolean) => {
    void save('image-mode', {
      image_settings: { ...imageSettings, mode: automatic ? 'interval' : 'manual' },
    })
  }

  const directorOptions = useMemo<TuningSelectOption[]>(
    () => directors.map((item) => ({ id: item.id, label: gamePresetName(item, t) })),
    [directors, t],
  )
  const narrativeOptions = useMemo<TuningSelectOption[]>(
    () => tellers.map((item) => ({ id: item.id, label: narrativeStyleName(item, t) })),
    [t, tellers],
  )
  const imageOptions = useMemo<TuningSelectOption[]>(
    () => imagePresets.map((item) => ({ id: item.id, label: item.name || item.id })),
    [imagePresets],
  )
  const ruleOptions = useMemo<TuningSelectOption[]>(
    () => ruleSystems.map((item) => ({ id: item.id, label: item.name || item.id })),
    [ruleSystems],
  )
  const stateOptions = useMemo<TuningSelectOption[]>(
    () => actorStates.map((item) => ({ id: item.id, label: item.name || item.id })),
    [actorStates],
  )

  const ruleEnabled = !refs.rule_system_disabled

  return (
    <div className="director-console__scroll h-full min-h-0 overflow-y-auto px-2.5 py-2.5">
      <div className="flex flex-col gap-2">
        <ControlSection
          icon={<Bot className="size-4" />}
          title={t('directorPanel.tuning.agent.title')}
          action={<TuningLinkButton label={t('directorPanel.tuning.editPresets')} onClick={onOpenPresets} />}
        >
          <TuningRow title={t('directorPanel.tuning.agent.preset')} busy={savingKey === 'director'}>
            <TuningSelect value={story?.story_director_id || director?.id || ''} options={directorOptions} label={t('directorPanel.tuning.agent.preset')} disabled={!story || !onDirectorChange || Boolean(savingKey)} onChange={(value) => void selectDirector(value)} />
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.agent.planning')} busy={savingKey === 'planning'}>
            <Switch checked={story?.planning_mode === 'enabled'} disabled={disabled} aria-label={t('directorPanel.tuning.agent.planning')} onCheckedChange={(enabled) => void save('planning', { planning_mode: enabled ? 'enabled' : 'disabled' })} />
          </TuningRow>
          <ModuleSelectRow
            label={t('directorPanel.tuning.agent.narrativeStyle')}
            value={String(refs.narrative_style_id || '')}
            baseValue={String(presetRefs.narrative_style_id || '')}
            moduleDisabled={Boolean(refs.narrative_style_disabled)}
            options={narrativeOptions}
            busy={savingKey === 'narrative'}
            disabled={disabled}
            onChange={(value) => updateModule('narrative', 'narrative_style_id', 'narrative_style_disabled', value, (nextID) => ({ story_teller_id: nextID }))}
          />
          <EventPackagesRow
            refs={refs}
            presetRefs={presetRefs}
            options={eventPackages}
            busy={savingKey === 'events'}
            disabled={disabled}
            onChange={(event_package_ids, event_packages_disabled) => void save('events', { module_refs: { ...cloneRefs(refs), event_package_ids, event_packages_disabled } })}
          />
          <TuningRow title={t('directorPanel.tuning.agent.replyLength')} busy={savingKey === 'reply-length'}>
            <ReplyLengthSetting value={story?.reply_target_chars || 2000} label={t('directorPanel.tuning.agent.replyLength')} disabled={disabled} onCommit={(reply_target_chars) => void save('reply-length', { reply_target_chars })} />
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.agent.choiceCount')} busy={savingKey === 'choice-count'}>
            <NumberSettingInput value={story?.choice_count || 5} min={MIN_INTERACTIVE_CHOICE_COUNT} max={MAX_INTERACTIVE_CHOICE_COUNT} label={t('directorPanel.tuning.agent.choiceCount')} disabled={disabled} onCommit={(choice_count) => void save('choice-count', { choice_count })} />
          </TuningRow>
        </ControlSection>

        <ControlSection
          icon={<Dices className="size-4" />}
          title={t('directorPanel.tuning.check.title')}
        >
          <TuningRow title={t('directorPanel.tuning.check.enabled')} busy={savingKey === 'checks-enabled'} source={ruleEnabled ? moduleSource(story, refs.rule_system_id, presetRefs.rule_system_id, false) : 'off'}>
            <Switch checked={ruleEnabled} disabled={disabled} aria-label={t('directorPanel.tuning.check.enabled')} onCheckedChange={setRuleChecksEnabled} />
          </TuningRow>
          <ModuleSelectRow
            label={t('directorPanel.tuning.check.system')}
            value={String(refs.rule_system_id || '')}
            baseValue={String(presetRefs.rule_system_id || '')}
            moduleDisabled={!ruleEnabled}
            options={ruleOptions}
            busy={savingKey === 'rule-system'}
            disabled={disabled || !ruleEnabled || structureLocked}
            locked={structureLocked}
            onChange={(value) => updateModule('rule-system', 'rule_system_id', 'rule_system_disabled', value, undefined, true)}
          />
          <TuningRow title={t('directorPanel.tuning.check.difficulty')} busy={savingKey === 'difficulty'}>
            <TuningSelect value={String(checkSettings.difficulty_shift)} options={difficultyOptions(t)} label={t('directorPanel.tuning.check.difficulty')} disabled={disabled || !ruleEnabled} onChange={(value) => void save('difficulty', { check_settings: { ...checkSettings, difficulty_shift: Number(value) } })} />
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.check.rollModifier')} description={t('directorPanel.tuning.check.formula', { modifier: formatSigned(checkSettings.roll_modifier) })} busy={savingKey === 'roll-modifier'}>
            <NumberSettingInput value={checkSettings.roll_modifier} min={-20} max={20} label={t('directorPanel.tuning.check.rollModifier')} disabled={disabled || !ruleEnabled} onCommit={(roll_modifier) => void save('roll-modifier', { check_settings: { ...checkSettings, roll_modifier } })} />
          </TuningRow>
        </ControlSection>

        <ControlSection
          icon={<ImagePlus className="size-4" />}
          title={t('directorPanel.tuning.image.title')}
        >
          <TuningRow title={t('directorPanel.tuning.image.automatic')} busy={savingKey === 'image-mode'}>
            <Switch checked={imageSettings.mode === 'interval'} disabled={disabled} aria-label={t('directorPanel.tuning.image.automatic')} onCheckedChange={setImageMode} />
          </TuningRow>
          <TuningRow title={t('directorPanel.tuning.image.interval')} busy={savingKey === 'image-interval'}>
            <NumberSettingInput value={imageSettings.interval_turns} min={1} max={50} label={t('directorPanel.tuning.image.interval')} disabled={disabled || imageSettings.mode !== 'interval'} onCommit={(interval_turns) => void save('image-interval', { image_settings: { ...imageSettings, interval_turns: normalizeImageIntervalTurns(interval_turns) } })} />
          </TuningRow>
          <ModuleSelectRow
            label={t('directorPanel.tuning.image.preset')}
            value={imageSettings.preset_id || String(refs.image_preset_id || '')}
            baseValue={String(presetRefs.image_preset_id || '')}
            moduleDisabled={Boolean(refs.image_preset_disabled)}
            options={imageOptions}
            busy={savingKey === 'image-preset'}
            disabled={disabled}
            onChange={(value) => updateModule('image-preset', 'image_preset_id', 'image_preset_disabled', value, (nextID) => ({ image_settings: { ...imageSettings, preset_id: nextID } }))}
          />
        </ControlSection>

        <ControlSection
          icon={<UserRound className="size-4" />}
          title={t('directorPanel.tuning.state.title')}
        >
          <ModuleSelectRow
            label={t('directorPanel.tuning.state.system')}
            value={String(refs.actor_state_id || '')}
            baseValue={String(presetRefs.actor_state_id || '')}
            moduleDisabled={Boolean(refs.actor_state_disabled)}
            options={stateOptions}
            busy={savingKey === 'state-system'}
            disabled={disabled || structureLocked}
            locked={structureLocked}
            onChange={(value) => updateModule('state-system', 'actor_state_id', 'actor_state_disabled', value, undefined, true)}
          />
          <TuningRow title={t('directorPanel.tuning.state.display')}>
            <StateDisplayPreferenceMenu value={stateDisplayPreference} onChange={onStateDisplayPreferenceChange} />
          </TuningRow>
        </ControlSection>
      </div>
    </div>
  )
}

function moduleSource(story: StorySummary | undefined, value: string | undefined, baseValue: string | undefined, disabled: boolean): TuningSource {
  if (disabled) return 'off'
  if (!story?.module_refs || value === baseValue) return 'preset'
  return 'story'
}

function cloneRefs(refs: StoryDirectorModuleRefs): StoryDirectorModuleRefs {
  return { ...refs, event_package_ids: [...(refs.event_package_ids || [])] }
}

function formatSigned(value: number): string {
  return value > 0 ? `+${value}` : String(value)
}

function difficultyOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return [-2, -1, 0, 1, 2].map((value) => ({ id: String(value), label: t(`directorPanel.tuning.check.difficulty.${value}`) }))
}
