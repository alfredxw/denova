import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Images } from 'lucide-react'
import { Switch } from '@/components/ui/switch'
import { Slider } from '@/components/ui/slider'
import { patchSettings } from '@/features/settings/api'
import { toast } from '@/lib/toast'
import type { StoryPresentationSettings, StorySummary } from '../../types'
import { useStagePreferences } from '../story-stage/use-stage-preferences'
import { ControlSection, TuningRow } from './StoryTuningControls'

export function StoryPresentationControls({ projectId, story, disabled, onChange }: { projectId?: string; story?: StorySummary; disabled: boolean; onChange: (settings: StoryPresentationSettings) => void }) {
  const { t } = useTranslation()
  const { scrimOpacity } = useStagePreferences(projectId || '')
  const [draft, setDraft] = useState<number>()
  const [saving, setSaving] = useState(false)
  const settings = { background: true, characters: true, ...story?.presentation_settings }
  const opacity = draft ?? scrimOpacity
  const saveOpacity = async (value: number) => {
    setSaving(true)
    try {
      await patchSettings('user', { interactive_stage_scrim_opacity: value })
    } catch (error) {
      console.warn('[story-presentation] failed to save reading scrim opacity', error)
      toast.error(t('storyStage.presentation.saveFailed'))
    } finally {
      setDraft(undefined)
      setSaving(false)
    }
  }
  return (
    <ControlSection icon={<Images className="size-4" />} title={t('storyStage.presentation.title')}>
      <TuningRow title={t('storyStage.presentation.background')}>
        <Switch aria-label={t('storyStage.presentation.background')} checked={settings.background} disabled={disabled} onCheckedChange={background => onChange({ ...settings, background })} />
      </TuningRow>
      <TuningRow title={t('storyStage.presentation.characters')}>
        <Switch aria-label={t('storyStage.presentation.characters')} checked={settings.characters} disabled={disabled} onCheckedChange={characters => onChange({ ...settings, characters })} />
      </TuningRow>
      <TuningRow title={t('storyStage.presentation.scrim')} description={t('storyStage.presentation.scrimHelp')}>
        <div className="flex w-28 items-center gap-2">
          <Slider aria-label={t('storyStage.presentation.scrim')} aria-valuetext={`${Math.round(opacity * 100)}%`} min={0} max={1} step={0.05} value={[opacity]} disabled={saving} onValueChange={([value]) => setDraft(value)} onValueCommit={([value]) => void saveOpacity(value)} />
          <span className="w-9 shrink-0 text-right text-xs tabular-nums">{Math.round(opacity * 100)}%</span>
        </div>
      </TuningRow>
    </ControlSection>
  )
}
