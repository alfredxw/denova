import { Pause, Play, RotateCcw, Square } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Slider } from '@/components/ui/slider'
import { requestSettingsSection } from '@/features/onboarding/events'
import { useSpeechPlayer } from './hooks'
import { speechPlayer } from './player'

export function openSpeechSettings() { requestSettingsSection('speech') }

export function SpeechPlayback({ owner }: { owner: string }) {
  const { t } = useTranslation()
  const state = useSpeechPlayer()
  if (state.owner !== owner || state.status === 'idle') return null
  const canPause = state.status === 'playing' || state.status === 'loading'
  const canResume = ['paused', 'blocked', 'error'].includes(state.status) && state.total > 0
  const canReplay = ['ended', 'stopped'].includes(state.status) && state.total > 0
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2 rounded-md border border-border bg-card px-3 py-2 text-xs text-foreground" data-testid="speech-playback">
      <span className="min-w-0 flex-1 break-words" role="status" aria-live="polite">
        {state.error ? t(state.error) : t(`speech.status.${state.status}`)}
        {state.total > 0 ? ` · ${state.segment}/${state.total}` : ''}
        {state.queued > 0 ? ` · ${t('speech.queued', { count: state.queued })}` : ''}
      </span>
      {state.error === 'speech.error.unconfigured' ? <Button size="xs" variant="outline" onClick={openSpeechSettings}>{t('speech.configure')}</Button> : null}
      {canPause ? <Button variant="ghost" size="icon-sm" aria-label={t('speech.pause')} onClick={speechPlayer.pause}><Pause /></Button> : null}
      {canResume ? <Button variant="ghost" size="sm" onClick={speechPlayer.resume}><Play />{t(state.status === 'error' ? 'speech.retry' : 'speech.resume')}</Button> : null}
      {canReplay ? <Button variant="ghost" size="sm" onClick={speechPlayer.replay}><RotateCcw />{t('speech.replay')}</Button> : null}
      {!['ended', 'stopped', 'empty'].includes(state.status) ? <Button variant="ghost" size="icon-sm" aria-label={t('speech.stop')} onClick={speechPlayer.stop}><Square /></Button> : null}
    </div>
  )
}

export function SpeechPreferences() {
  const { t } = useTranslation()
  const { rate, volume } = useSpeechPlayer()
  return (
    <div className="grid gap-4 px-3 py-3">
      <div className="flex items-center gap-3"><span className="shrink-0 text-xs text-muted-foreground">{t('speech.rate')}</span><Slider aria-label={t('speech.rate')} value={[rate]} min={0.5} max={2} step={0.1} onValueChange={([value]) => speechPlayer.setRate(value)} /><span className="w-9 shrink-0 text-right text-xs tabular-nums">{rate.toFixed(1)}×</span></div>
      <div className="flex items-center gap-3"><span className="shrink-0 text-xs text-muted-foreground">{t('speech.volume')}</span><Slider aria-label={t('speech.volume')} value={[volume]} min={0} max={1} step={0.05} onValueChange={([value]) => speechPlayer.setVolume(value)} /><span className="w-9 shrink-0 text-right text-xs tabular-nums">{Math.round(volume * 100)}%</span></div>
    </div>
  )
}
