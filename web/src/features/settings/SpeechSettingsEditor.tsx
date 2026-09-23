import { useEffect, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Volume2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { SpeechPlayback } from '@/features/speech/SpeechPlayback'
import { speechConfigError, speechPlayer } from '@/features/speech/player'
import { ApiKeyInput } from './ApiKeyInput'
import type { SpeechSettings } from './types'

export const EMPTY_SPEECH_SETTINGS: SpeechSettings = { endpoint: '', api_key: '', model: '', voice: '' }

export function SpeechSettingsEditor({ value, onChange, visible }: { value: SpeechSettings; onChange: (settings: SpeechSettings) => void; visible: boolean }) {
  const { t } = useTranslation()
  const id = useId()
  useEffect(() => { if (!visible) speechPlayer.stopOwner('preview'); return () => speechPlayer.stopOwner('preview') }, [visible])
  const change = (key: keyof SpeechSettings, text: string) => { speechPlayer.stopOwner('preview'); onChange({ ...value, [key]: text }) }
  return (
    <FieldGroup className="gap-4">
      <p className="text-sm text-muted-foreground">{t('speech.description')}</p>
      <Field><FieldLabel htmlFor={`${id}-endpoint`}>{t('speech.endpoint')}</FieldLabel><Input id={`${id}-endpoint`} value={value.endpoint} placeholder="https://api.openai.com/v1/audio/speech" onChange={event => change('endpoint', event.target.value)} /><FieldDescription>{t('speech.endpointHelp')}</FieldDescription></Field>
      <Field><FieldLabel>{t('speech.apiKey')}</FieldLabel><ApiKeyInput label={t('speech.apiKey')} value={value.api_key} placeholder={t('speech.keyOptional')} onChange={text => change('api_key', text)} /></Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field><FieldLabel htmlFor={`${id}-model`}>{t('speech.model')}</FieldLabel><Input id={`${id}-model`} value={value.model} placeholder="tts-1" onChange={event => change('model', event.target.value)} /></Field>
        <Field><FieldLabel htmlFor={`${id}-voice`}>{t('speech.voice')}</FieldLabel><Input id={`${id}-voice`} value={value.voice} placeholder="alloy" onChange={event => change('voice', event.target.value)} /></Field>
      </div>
      <FieldDescription>{t('speech.voiceHelp')}</FieldDescription>
      <div><Button variant="outline" disabled={Boolean(speechConfigError(value))} onClick={() => speechPlayer.read({ owner: 'preview', turnId: 'preview', text: t('speech.sample'), settings: value, preview: true })}><Volume2 />{t('speech.preview')}</Button></div>
      {value.endpoint && speechConfigError(value) === 'speech.error.url' ? <p className="text-sm text-destructive" role="alert">{t('speech.error.url')}</p> : null}
      <SpeechPlayback owner="preview" />
    </FieldGroup>
  )
}
