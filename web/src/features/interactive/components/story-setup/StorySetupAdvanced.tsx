import { Bot, Dices, ImagePlus, UserRound } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Switch } from '@/components/ui/switch'
import { CustomAgentSelect } from '@/features/agents/CustomAgentSelect'
import { getActorStates, getEventPackages, getRuleSystems } from '../../api'
import { normalizeImageIntervalTurns } from '../../image-settings'
import { narrativeStyleName } from '../../narrative-style'
import { MAX_INTERACTIVE_CHOICE_COUNT, MIN_INTERACTIVE_CHOICE_COUNT } from '../../opening'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryCheckSettings, StoryDirector, StoryDirectorModuleRefs, StoryImageSettings, StoryStateSchemaMode, Teller } from '../../types'
import { ReplyLengthSetting } from '../director-console/ReplyLengthSetting'
import { ControlSection, NumberSettingInput, TuningLinkButton, TuningRow, TuningSelect, type TuningSelectOption } from '../director-console/StoryTuningControls'
import { EventPackagesRow, ModuleSelectRow, PRESET_VALUE } from '../director-console/StoryTuningModuleRows'

export interface StorySetupSettings {
  customAgentId: string
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
  director?: StoryDirector
  tellers: Teller[]
  imagePresets: ImagePreset[]
  value: StorySetupSettings
  onChange: (value: StorySetupSettings) => void
  onNarrativeStyleChange?: (id: string) => void | Promise<unknown>
  onOpenPresets?: () => void
}

type ModuleIDKey = 'narrative_style_id' | 'rule_system_id' | 'actor_state_id' | 'image_preset_id'
type ModuleDisabledKey = 'narrative_style_disabled' | 'rule_system_disabled' | 'actor_state_disabled' | 'image_preset_disabled'

export function StorySetupAdvanced({ projectId, newStory, director, tellers, imagePresets, value, onChange, onNarrativeStyleChange, onOpenPresets }: StorySetupAdvancedProps) {
  const { t } = useTranslation()
  const [eventPackages, setEventPackages] = useState<EventPackageModule[]>([])
  const [ruleSystems, setRuleSystems] = useState<RuleSystemModule[]>([])
  const [actorStates, setActorStates] = useState<ActorStateModule[]>([])
  const presetRefs = director?.module_refs || {}
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
    return () => { cancelled = true }
  }, [])

  const narrativeOptions = useMemo<TuningSelectOption[]>(() => tellers.map((item) => ({ id: item.id, label: narrativeStyleName(item, t) })), [t, tellers])
  const imageOptions = useMemo<TuningSelectOption[]>(() => imagePresets.map(moduleOption), [imagePresets])
  const ruleOptions = useMemo<TuningSelectOption[]>(() => ruleSystems.map(moduleOption), [ruleSystems])
  const stateOptions = useMemo<TuningSelectOption[]>(() => actorStates.map(moduleOption), [actorStates])
  const ruleEnabled = !refs.rule_system_disabled

  const patch = (next: Partial<StorySetupSettings>) => onChange({ ...value, ...next })
  const patchRefs = (next: StoryDirectorModuleRefs) => patch({ moduleRefs: next })
  const updateModule = (idKey: ModuleIDKey, disabledKey: ModuleDisabledKey, selected: string) => {
    const next = cloneRefs(refs)
    next[idKey] = selected === PRESET_VALUE ? String(presetRefs[idKey] || '') : selected
    next[disabledKey] = selected === PRESET_VALUE ? Boolean(presetRefs[disabledKey]) : false
    const nextID = String(next[idKey] || '')
    if (idKey === 'image_preset_id') patch({ moduleRefs: next, imageSettings: { ...value.imageSettings, preset_id: nextID } })
    else patchRefs(next)
    if (idKey === 'narrative_style_id' && nextID) void onNarrativeStyleChange?.(nextID)
  }
  const setRuleChecksEnabled = (enabled: boolean) => {
    const next = cloneRefs(refs)
    next.rule_system_disabled = !enabled
    if (enabled && !next.rule_system_id) next.rule_system_id = String(presetRefs.rule_system_id || ruleSystems[0]?.id || '')
    patchRefs(next)
  }
  const setStateSchemaMode = (mode: StoryStateSchemaMode) => {
    const next = cloneRefs(refs)
    next.actor_state_disabled = mode === 'generate'
    patch({ stateSchemaMode: mode, moduleRefs: next })
  }

  return (
    <div className="grid gap-3 lg:grid-cols-2">
      <ControlSection icon={<Bot className="size-4" />} title={t('directorPanel.tuning.agent.title')} action={<TuningLinkButton label={t('directorPanel.tuning.editPresets')} onClick={onOpenPresets} />}>
        {newStory ? (
          <TuningRow title={t('agents.custom.select')} description={t('agents.custom.switchNote')}>
            <CustomAgentSelect projectId={projectId} runtimeKind="interactive_story" value={value.customAgentId} onValueChange={(customAgentId) => patch({ customAgentId: customAgentId || '' })} className="director-control-select h-7 w-[min(10rem,60cqw)]" />
          </TuningRow>
        ) : null}
        <TuningRow title={t('directorPanel.tuning.agent.planning')}>
          <Switch checked={value.planningEnabled} aria-label={t('directorPanel.tuning.agent.planning')} onCheckedChange={(planningEnabled) => patch({ planningEnabled })} />
        </TuningRow>
        <ModuleSelectRow label={t('directorPanel.tuning.agent.narrativeStyle')} value={String(refs.narrative_style_id || '')} baseValue={String(presetRefs.narrative_style_id || '')} moduleDisabled={Boolean(refs.narrative_style_disabled)} options={narrativeOptions} busy={false} disabled={false} onChange={(selected) => updateModule('narrative_style_id', 'narrative_style_disabled', selected)} />
        <EventPackagesRow refs={refs} presetRefs={presetRefs} options={eventPackages} busy={false} disabled={false} onChange={(event_package_ids, event_packages_disabled) => patchRefs({ ...cloneRefs(refs), event_package_ids, event_packages_disabled })} />
        <TuningRow title={t('directorPanel.tuning.agent.replyLength')}>
          <ReplyLengthSetting value={value.replyTargetChars} label={t('directorPanel.tuning.agent.replyLength')} onCommit={(replyTargetChars) => patch({ replyTargetChars })} />
        </TuningRow>
        <TuningRow title={t('directorPanel.tuning.agent.choiceCount')}>
          <NumberSettingInput value={value.choiceCount} min={MIN_INTERACTIVE_CHOICE_COUNT} max={MAX_INTERACTIVE_CHOICE_COUNT} label={t('directorPanel.tuning.agent.choiceCount')} onCommit={(choiceCount) => patch({ choiceCount })} />
        </TuningRow>
      </ControlSection>

      <ControlSection icon={<Dices className="size-4" />} title={t('directorPanel.tuning.check.title')}>
        <TuningRow title={t('directorPanel.tuning.check.enabled')}>
          <Switch checked={ruleEnabled} aria-label={t('directorPanel.tuning.check.enabled')} onCheckedChange={setRuleChecksEnabled} />
        </TuningRow>
        <ModuleSelectRow label={t('directorPanel.tuning.check.system')} value={String(refs.rule_system_id || '')} baseValue={String(presetRefs.rule_system_id || '')} moduleDisabled={!ruleEnabled} options={ruleOptions} busy={false} disabled={!ruleEnabled} onChange={(selected) => updateModule('rule_system_id', 'rule_system_disabled', selected)} />
        <TuningRow title={t('directorPanel.tuning.check.difficulty')}>
          <TuningSelect value={String(value.checkSettings.difficulty_shift)} options={difficultyOptions(t)} label={t('directorPanel.tuning.check.difficulty')} disabled={!ruleEnabled} onChange={(difficulty) => patch({ checkSettings: { ...value.checkSettings, difficulty_shift: Number(difficulty) } })} />
        </TuningRow>
        <TuningRow title={t('directorPanel.tuning.check.rollModifier')} description={t('directorPanel.tuning.check.formula', { modifier: formatSigned(value.checkSettings.roll_modifier) })}>
          <NumberSettingInput value={value.checkSettings.roll_modifier} min={-20} max={20} label={t('directorPanel.tuning.check.rollModifier')} disabled={!ruleEnabled} onCommit={(rollModifier) => patch({ checkSettings: { ...value.checkSettings, roll_modifier: rollModifier } })} />
        </TuningRow>
      </ControlSection>

      <ControlSection icon={<ImagePlus className="size-4" />} title={t('directorPanel.tuning.image.title')}>
        <TuningRow title={t('directorPanel.tuning.image.automatic')}>
          <Switch checked={value.imageSettings.mode === 'interval'} aria-label={t('directorPanel.tuning.image.automatic')} onCheckedChange={(automatic) => patch({ imageSettings: { ...value.imageSettings, mode: automatic ? 'interval' : 'manual' } })} />
        </TuningRow>
        <TuningRow title={t('directorPanel.tuning.image.interval')}>
          <NumberSettingInput value={value.imageSettings.interval_turns} min={1} max={50} label={t('directorPanel.tuning.image.interval')} disabled={value.imageSettings.mode !== 'interval'} onCommit={(intervalTurns) => patch({ imageSettings: { ...value.imageSettings, interval_turns: normalizeImageIntervalTurns(intervalTurns) } })} />
        </TuningRow>
        <ModuleSelectRow label={t('directorPanel.tuning.image.preset')} value={value.imageSettings.preset_id || String(refs.image_preset_id || '')} baseValue={String(presetRefs.image_preset_id || '')} moduleDisabled={Boolean(refs.image_preset_disabled)} options={imageOptions} busy={false} disabled={false} onChange={(selected) => updateModule('image_preset_id', 'image_preset_disabled', selected)} />
      </ControlSection>

      <ControlSection icon={<UserRound className="size-4" />} title={t('directorPanel.tuning.state.title')}>
        <ModuleSelectRow label={t('directorPanel.tuning.state.system')} value={String(refs.actor_state_id || '')} baseValue={String(presetRefs.actor_state_id || '')} moduleDisabled={Boolean(refs.actor_state_disabled)} options={stateOptions} busy={false} disabled={value.stateSchemaMode === 'generate'} onChange={(selected) => updateModule('actor_state_id', 'actor_state_disabled', selected)} />
        <TuningRow title={t('storyPicker.setup.stateSchema.title')} description={t('storyPicker.setup.stateSchema.description')}>
          <TuningSelect value={value.stateSchemaMode} options={stateSchemaOptions(t)} label={t('storyPicker.setup.stateSchema.title')} onChange={(mode) => setStateSchemaMode(mode as StoryStateSchemaMode)} />
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
  return [-2, -1, 0, 1, 2].map((value) => ({ id: String(value), label: t(`directorPanel.tuning.check.difficulty.${value}`) }))
}

function stateSchemaOptions(t: ReturnType<typeof useTranslation>['t']): TuningSelectOption[] {
  return ['adapt_template', 'fixed_template', 'generate'].map((mode) => ({ id: mode, label: t(`storyPicker.setup.stateSchema.${mode}.title`) }))
}

function formatSigned(value: number): string {
  return value > 0 ? `+${value}` : String(value)
}
