import { Volume2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Switch } from '@/components/ui/switch'
import { useSpeechSettings } from '@/features/speech/hooks'
import { openSpeechSettings, SpeechPreferences } from '@/features/speech/SpeechPlayback'
import type { StorySpeechSettings, StorySummary } from '../../types'
import { ControlSection, TuningLinkButton, TuningRow, TuningSelect } from './StoryTuningControls'

export const DEFAULT_STORY_SPEECH: StorySpeechSettings = { auto_read: false, mode: 'all', ignore_asterisks: false }

export function StorySpeechControls({ story, disabled, onChange }: { story?: StorySummary; disabled: boolean; onChange: (settings: StorySpeechSettings) => void }) {
  const { t } = useTranslation()
  const { settings } = useSpeechSettings()
  const value = { ...DEFAULT_STORY_SPEECH, ...story?.speech_settings }
  return (
    <ControlSection icon={<Volume2 className="size-3.5" />} title={t('speech.title')} action={<TuningLinkButton label={t('speech.configure')} onClick={openSpeechSettings} />}>
      <p className="break-all px-3 py-2 text-xs text-muted-foreground">{settings?.model && settings.voice ? `${settings.model} · ${settings.voice}` : t('speech.error.unconfigured')}</p>
      <TuningRow title={t('speech.auto')} description={t('speech.autoHelp')}><Switch aria-label={t('speech.auto')} checked={value.auto_read} disabled={disabled} onCheckedChange={auto_read => onChange({ ...value, auto_read })} /></TuningRow>
      <TuningRow title={t('speech.content')} description={t('speech.quoteHelp')}><TuningSelect label={t('speech.content')} value={value.mode} disabled={disabled} options={[{ id: 'all', label: t('speech.all') }, { id: 'quoted', label: t('speech.quoted') }]} onChange={mode => onChange({ ...value, mode: mode as 'all' | 'quoted' })} /></TuningRow>
      <TuningRow title={t('speech.ignoreAsterisks')} description={t('speech.asteriskHelp')}><Switch aria-label={t('speech.ignoreAsterisks')} checked={value.ignore_asterisks} disabled={disabled} onCheckedChange={ignore_asterisks => onChange({ ...value, ignore_asterisks })} /></TuningRow>
      <SpeechPreferences />
    </ControlSection>
  )
}
