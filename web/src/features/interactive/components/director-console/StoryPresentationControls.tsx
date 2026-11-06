import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Images } from 'lucide-react'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import type { StageCharacterLayout } from '@/features/settings/types'
import { Switch } from '@/components/ui/switch'
import { Slider } from '@/components/ui/slider'
import { Field, FieldTitle } from '@/components/ui/field'
import { patchProjectSettings, patchSettings, type SettingsPatch } from '@/features/settings/api'
import { toast } from '@/lib/toast'
import { visibleStoryPresentation } from '../../presentation'
import { StoryMaterialSelect } from './StoryMaterialSelect'
import type { TurnPresentationPatch, PresentationMaterial, TurnEvent, StoryPresentationSettings } from '../../types'
import { useStagePreferences } from '../story-stage/use-stage-preferences'
import { ControlSection, NumberSettingInput, TuningRow, TuningSelect } from './StoryTuningControls'

interface StoryPresentationControlsProps {
  projectId?: string
  value?: StoryPresentationSettings
  disabled: boolean
  onChange: (settings: StoryPresentationSettings) => void
  currentTurn?: TurnEvent
  onPresentationChange?: (turnId: string, patch: TurnPresentationPatch) => Promise<void>
  presentationDisabled?: boolean
}

export function StoryPresentationControls({ projectId, value, disabled, onChange, currentTurn, onPresentationChange, presentationDisabled = false }: StoryPresentationControlsProps) {
  const { t } = useTranslation()
  const { theme, scrimOpacity, textOutline, textMaxWidth, lineHeight, characterLayout, characterSize } = useStagePreferences(projectId || '')
  const [savingDisplayPreference, setSavingDisplayPreference] = useState(false)
  const saveDisplayPreference = async (changes: SettingsPatch) => {
    setSavingDisplayPreference(true)
    try {
      if (projectId) await patchProjectSettings(projectId, 'user', changes)
      else await patchSettings('user', changes)
      console.info('[story-presentation] user display preference saved', changes)
    } catch (error) {
      console.warn('[story-presentation] failed to save user display preference', { changes, error })
      toast.error(t('storyStage.presentation.saveFailed'))
    } finally {
      setSavingDisplayPreference(false)
    }
  }
  const [savingPresentation, setSavingPresentation] = useState(false)
  const changeCurrentPresentation = async (layer: 'background' | 'ambient', material?: PresentationMaterial) => {
    if (!currentTurn || !onPresentationChange) return
    setSavingPresentation(true)
    try {
      await onPresentationChange(currentTurn.id, { [layer]: material ?? null })
    } catch (error) {
      console.warn('[story-presentation] failed to change current presentation', error)
      toast.error(t('storyStage.presentation.updateFailed'))
    } finally {
      setSavingPresentation(false)
    }
  }
  const settings = { background: true, characters: true, ambient: false, ...value }
  return (
    <ControlSection icon={<Images className="size-4" />} title={t('storyStage.presentation.title')}>
      {(['background', 'ambient'] as const).map(layer => <StoryMaterialSelect
        key={layer}
        layer={layer}
        projectId={projectId}
        value={visibleStoryPresentation(currentTurn?.turn_result?.presentation, settings)[layer]}
        disabled={disabled || savingPresentation || (!!currentTurn && (presentationDisabled || !onPresentationChange))}
        onChange={material => {
          if (currentTurn) void changeCurrentPresentation(layer, material)
          else onChange({ ...settings, [layer === 'background' ? 'default_background' : 'default_ambient']: material })
        }}
      />)}
      <TuningRow title={t('storyStage.presentation.ambient')}>
        <Switch aria-label={t('storyStage.presentation.ambient')} checked={settings.ambient} disabled={disabled} onCheckedChange={ambient => onChange({ ...settings, ambient })} />
      </TuningRow>
      <TuningRow title={t('storyStage.presentation.background')}>
        <Switch aria-label={t('storyStage.presentation.background')} checked={settings.background} disabled={disabled} onCheckedChange={background => onChange({ ...settings, background })} />
      </TuningRow>
      <TuningRow title={t('storyStage.presentation.characters')}>
        <Switch aria-label={t('storyStage.presentation.characters')} checked={settings.characters} disabled={disabled} onCheckedChange={characters => onChange({ ...settings, characters })} />
      </TuningRow>
      <Field className="director-control-row min-w-0 gap-2 px-2.5 py-2">
        <FieldTitle>{t('storyStage.presentation.characterLayout')}</FieldTitle>
        <ToggleGroup type="single" variant="outline" size="sm" className="w-full flex-wrap" disabled={savingDisplayPreference}
          aria-label={t('storyStage.presentation.characterLayout')}
          value={characterLayout}
          onValueChange={value => { if (value) void saveDisplayPreference({ interactive_stage_character_layout: value as StageCharacterLayout }) }}>
          {(['center', 'left', 'right', 'sides'] as const).map(layout => (
            <ToggleGroupItem key={layout} value={layout}>{t(`storyStage.presentation.layout.${layout}`)}</ToggleGroupItem>
          ))}
        </ToggleGroup>
      </Field>
      <StageDisplaySlider projectId={projectId} setting="interactive_stage_character_size" label={t('storyStage.presentation.characterSize')} value={characterSize} min={0.4} />
      <StageDisplayNumber projectId={projectId} setting="interactive_stage_text_max_width" label={t('storyStage.presentation.textMaxWidth')} value={textMaxWidth} min={480} max={1600} />
      <StageDisplayNumber projectId={projectId} setting="interactive_stage_line_height" label={t('storyStage.presentation.lineHeight')} value={lineHeight} min={1.35} max={2.4} step={0.01} />
      <TuningRow title={t('storyStage.presentation.theme')}>
        <TuningSelect label={t('storyStage.presentation.theme')} value={theme ?? 'app'} disabled={savingDisplayPreference}
          options={[
            { id: 'app', label: t('storyStage.presentation.followAppTheme') },
            { id: 'light', label: t('settings.theme.light') },
            { id: 'dark', label: t('settings.theme.dark') },
          ]}
          onChange={value => void saveDisplayPreference({ interactive_stage_theme: value === 'light' || value === 'dark' ? value : null })} />
      </TuningRow>
      <TuningRow title={t('storyStage.presentation.textOutline')}>
        <Switch aria-label={t('storyStage.presentation.textOutline')} checked={textOutline} disabled={savingDisplayPreference}
          onCheckedChange={checked => void saveDisplayPreference({ interactive_stage_text_outline: checked })} />
      </TuningRow>
      <StageDisplaySlider projectId={projectId} setting="interactive_stage_scrim_opacity" label={t('storyStage.presentation.scrim')} value={scrimOpacity} min={0} />
    </ControlSection>
  )
}

// Display controls save user preferences; they never change story state.
function StageDisplayNumber({ projectId, setting, label, value, min, max, step }: {
  projectId?: string
  setting: 'interactive_stage_text_max_width' | 'interactive_stage_line_height'
  label: string
  value: number
  min: number
  max: number
  step?: number
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<number>()
  const [saving, setSaving] = useState(false)
  const save = async (next: number) => {
    setDraft(next)
    setSaving(true)
    try {
      const changes = { [setting]: next }
      const saved = projectId
        ? await patchProjectSettings(projectId, 'user', changes)
        : await patchSettings('user', changes)
      setDraft(projectId ? undefined : saved.effective[setting] ?? next)
      console.info('[story-presentation] user display preference saved', { setting, value: next })
    } catch (error) {
      console.warn('[story-presentation] failed to save user display preference', { setting, error })
      toast.error(t('storyStage.presentation.saveFailed'))
      setDraft(undefined)
    } finally {
      setSaving(false)
    }
  }
  return (
    <TuningRow title={label}>
      <NumberSettingInput label={label} value={draft ?? value} min={min} max={max} step={step} disabled={saving} onCommit={next => void save(next)} />
    </TuningRow>
  )
}

function StageDisplaySlider({ projectId, setting, label, value, min }: {
  projectId?: string
  setting: 'interactive_stage_character_size' | 'interactive_stage_scrim_opacity'
  label: string
  value: number
  min: number
}) {
  const { t } = useTranslation()
  const labelId = useId()
  const [draft, setDraft] = useState<number>()
  const latestEdit = useRef({ value, version: 0 })
  const saveQueue = useRef(Promise.resolve())
  const change = (next: number) => {
    // Keyboard input may report the commit before the matching value change.
    if (latestEdit.current.value !== next) {
      latestEdit.current = { value: next, version: latestEdit.current.version + 1 }
    }
    setDraft(next)
  }
  const save = (next: number) => {
    change(next)
    const version = latestEdit.current.version
    // Serialize saves and retain newer edits when an older response arrives.
    saveQueue.current = saveQueue.current.then(async () => {
      try {
        const changes = { [setting]: next }
        const saved = projectId
          ? await patchProjectSettings(projectId, 'user', changes)
          : await patchSettings('user', changes)
        console.info('[story-presentation] user display preference saved', { setting, value: next })
        if (version === latestEdit.current.version) {
          setDraft(projectId ? undefined : saved.effective[setting] ?? next)
        }
      } catch (error) {
        console.warn('[story-presentation] failed to save user display preference', { setting, error })
        toast.error(t('storyStage.presentation.saveFailed'))
        if (version === latestEdit.current.version) setDraft(undefined)
      }
    })
  }
  const current = draft ?? value
  return (
    <Field className="director-control-row min-w-0 gap-2 px-2.5 py-2">
      <div className="flex min-w-0 items-start justify-between gap-2">
        <FieldTitle id={labelId} className="min-w-0 text-xs">{label}</FieldTitle>
        <span className="w-9 shrink-0 text-right text-xs tabular-nums" aria-hidden="true">{Math.round(current * 100)}%</span>
      </div>
      <Slider aria-labelledby={labelId} aria-valuetext={`${Math.round(current * 100)}%`} min={min} max={1} step={0.01} value={[current]} className="min-h-5" onValueChange={([next]) => change(next)} onValueCommit={([next]) => save(next)} />
    </Field>
  )
}
