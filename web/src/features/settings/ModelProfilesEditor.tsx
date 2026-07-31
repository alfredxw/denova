import { useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  DEFAULT_MODEL_PROFILE_ID,
  MODEL_PROTOCOL_CHAT_COMPLETIONS,
  MODEL_PROTOCOL_RESPONSES,
  MODEL_PROVIDER_DEEPSEEK,
  MODEL_PROVIDER_OPENAI,
  MODEL_PROVIDER_OPENAI_COMPATIBLE,
  defaultModelProviderBaseURL,
  modelProfileID,
  modelProfileLabel,
} from './model-profiles'
import type { ModelProfileSettings } from './types'

const DEFAULT_CONTEXT_WINDOW_TOKENS = 400000
const MIN_CONTEXT_WINDOW_TOKENS = 1024
const MAX_CONTEXT_WINDOW_TOKENS = 2000000
const CONTEXT_WINDOW_PRESETS = [200000, DEFAULT_CONTEXT_WINDOW_TOKENS, 1000000]

export function ModelProfilesEditor({ profiles, effectiveProfiles, onChange }: {
  profiles: ModelProfileSettings[]
  effectiveProfiles: ModelProfileSettings[]
  onChange: (profiles: ModelProfileSettings[]) => void
}) {
  const { t } = useTranslation()
  const profileKeysRef = useRef<string[]>([])
  const profileKeys = useMemo(() => {
    if (profileKeysRef.current.length > profiles.length) {
      profileKeysRef.current = profileKeysRef.current.slice(0, profiles.length)
    }
    while (profileKeysRef.current.length < profiles.length) {
      profileKeysRef.current.push(`profile-${Date.now()}-${profileKeysRef.current.length}`)
    }
    return profileKeysRef.current
  }, [profiles.length])

  const addProfile = () => {
    onChange([...profiles, {
      provider: MODEL_PROVIDER_OPENAI_COMPATIBLE,
      protocol: MODEL_PROTOCOL_CHAT_COMPLETIONS,
      context_window_tokens: DEFAULT_CONTEXT_WINDOW_TOKENS,
    }])
  }
  const updateProfile = (index: number, patch: Partial<ModelProfileSettings>) => {
    onChange(profiles.map((profile, profileIndex) => (profileIndex === index ? { ...profile, ...patch } : profile)))
  }
  const updateProfileModel = (index: number, openaiModel: string) => {
    const profile = profiles[index]
    const previousID = modelProfileID(profile)
    const previousModel = profile?.openai_model?.trim() ?? ''
    const shouldSyncID = !previousID || previousID === previousModel
    updateProfile(index, {
      id: shouldSyncID ? openaiModel : profile?.id,
      openai_model: openaiModel,
    })
  }
  const updateProfileProvider = (index: number, provider: string) => {
    updateProfile(index, {
      provider,
      protocol: provider === MODEL_PROVIDER_OPENAI
        ? MODEL_PROTOCOL_RESPONSES
        : MODEL_PROTOCOL_CHAT_COMPLETIONS,
      openai_base_url: baseURLAfterProviderChange(profiles[index]?.openai_base_url, provider),
    })
  }
  const removeProfile = (index: number) => {
    profileKeysRef.current.splice(index, 1)
    onChange(profiles.filter((_, profileIndex) => profileIndex !== index))
  }

  return (
    <div className="nova-settings-row rounded-md px-2 py-1.5">
      <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('settings.model.modelProfiles')}</div>
      <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('settings.model.routingHint')}
      </div>
      <div className="flex flex-col gap-2">
        {profiles.length === 0 && (
          <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2 text-[var(--nova-text-faint)]">
            {t('settings.model.profileEmpty', { count: effectiveProfiles.length || 1 })}
          </div>
        )}
        {profiles.map((profile, index) => {
          const isDefaultProfile = modelProfileID(profile) === DEFAULT_MODEL_PROFILE_ID
          return (
            <div key={profileKeys[index]} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
              <div className="flex items-center gap-2 px-2.5 py-2">
                <Badge variant="outline" className="shrink-0">
                  {isDefaultProfile ? t('settings.model.defaultProfileName') : t('settings.model.profileName', { index: index + 1 })}
                </Badge>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-xs font-medium text-[var(--nova-text)]">
                    {modelProfileLabel(profile) || t('settings.model.profileUntitled')}
                  </div>
                  <div className="truncate text-[11px] text-[var(--nova-text-faint)]">
                    {profile.openai_model?.trim() || t('settings.model.profileModelMissing')}
                  </div>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="icon-sm"
                  onClick={() => removeProfile(index)}
                  aria-label={t('settings.model.deleteProfile')}
                  title={t('settings.model.deleteProfile')}
                >
                  <Trash2 data-icon="inline-start" />
                </Button>
              </div>
              <Separator />
              <div className="grid gap-2 p-2.5 md:grid-cols-12">
                <ModelProfileField label={t('common.baseUrl')} className="md:col-span-5">
                  <Input
                    value={profile.openai_base_url ?? ''}
                    placeholder={t('common.baseUrl')}
                    onChange={(event) => updateProfile(index, { openai_base_url: event.target.value })}
                  />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileModelLabel')} className="md:col-span-4">
                  <Input
                    value={profile.openai_model ?? ''}
                    placeholder={t('settings.model.profileModelPlaceholder')}
                    onChange={(event) => updateProfileModel(index, event.target.value)}
                  />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileAliasLabel')} className="md:col-span-3">
                  <Input
                    value={profile.name ?? ''}
                    placeholder={t('settings.model.profileAliasPlaceholder')}
                    onChange={(event) => updateProfile(index, { name: event.target.value })}
                  />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileProviderLabel')} className="md:col-span-5">
                  <Select value={profile.provider || MODEL_PROVIDER_OPENAI_COMPATIBLE} onValueChange={(value) => updateProfileProvider(index, value)}>
                    <SelectTrigger size="sm" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      <SelectItem value={MODEL_PROVIDER_OPENAI}>{t('settings.model.profileProviderOpenAI')}</SelectItem>
                      <SelectItem value={MODEL_PROVIDER_DEEPSEEK}>{t('settings.model.profileProviderDeepSeek')}</SelectItem>
                      <SelectItem value={MODEL_PROVIDER_OPENAI_COMPATIBLE}>{t('settings.model.profileProviderCompatible')}</SelectItem>
                    </SelectContent>
                  </Select>
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileProtocolLabel')} className="md:col-span-4">
                  <Select value={profile.protocol || MODEL_PROTOCOL_CHAT_COMPLETIONS} onValueChange={(value) => updateProfile(index, { protocol: value })}>
                    <SelectTrigger size="sm" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      <SelectItem value={MODEL_PROTOCOL_CHAT_COMPLETIONS}>{t('settings.model.profileProtocolChatCompletions')}</SelectItem>
                      <SelectItem value={MODEL_PROTOCOL_RESPONSES} disabled={profile.provider === MODEL_PROVIDER_DEEPSEEK}>
                        {t('settings.model.profileProtocolResponses')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileKeyLabel')} className="md:col-span-3">
                  <Input
                    type="password"
                    value={profile.openai_api_key ?? ''}
                    placeholder={t('settings.model.profileKeyInheritPlaceholder')}
                    onChange={(event) => updateProfile(index, { openai_api_key: event.target.value })}
                  />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileTemperatureLabel')} className="md:col-span-2">
                  <Input
                    type="number"
                    step={0.01}
                    min={0}
                    max={1}
                    value={profile.temperature ?? ''}
                    placeholder="0-1"
                    onChange={(event) => updateProfile(index, { temperature: event.target.value === '' ? null : Number(event.target.value) })}
                    className="max-w-24"
                  />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.contextWindow')} className="md:col-span-5">
                  <ContextWindowInput
                    value={profile.context_window_tokens ?? DEFAULT_CONTEXT_WINDOW_TOKENS}
                    onChange={(value) => updateProfile(index, { context_window_tokens: value })}
                  />
                </ModelProfileField>
              </div>
            </div>
          )
        })}
        <Button type="button" onClick={addProfile} variant="outline" size="sm">
          <Plus data-icon="inline-start" />
          {t('settings.model.addProfile')}
        </Button>
      </div>
    </div>
  )
}

function baseURLAfterProviderChange(currentValue: string | undefined, provider: string) {
  const current = currentValue?.trim() ?? ''
  const normalized = current.toLowerCase().replace(/\/+$/, '')
  const isKnownProviderURL = current === '' ||
    normalized === 'https://api.openai.com/v1' ||
    normalized === 'https://api.deepseek.com'
  if (!isKnownProviderURL) return current

  return defaultModelProviderBaseURL(provider)
}

function ModelProfileField({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return (
    <label className={`flex min-w-0 flex-col gap-1 ${className ?? ''}`}>
      <span className="text-[11px] leading-none text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

function ContextWindowInput({ value, onChange }: {
  value: number
  onChange: (value: number | null) => void
}) {
  const { t } = useTranslation()
  const [customDraft, setCustomDraft] = useState<string | null>(null)
  const customEditing = customDraft !== null
  const preset = !customEditing && CONTEXT_WINDOW_PRESETS.includes(value) ? String(value) : 'custom'
  const customValue = customDraft ?? String(value)

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row">
      <Select
        value={preset}
        onValueChange={(nextValue) => {
          if (nextValue === 'custom') {
            setCustomDraft(String(value))
            return
          }
          setCustomDraft(null)
          onChange(Number(nextValue))
        }}
      >
        <SelectTrigger
          size="sm"
          className="min-w-0 flex-1"
          aria-label={t('settings.model.contextWindow')}
          title={t('settings.model.contextWindow')}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value="200000">{t('settings.model.contextWindow200k')}</SelectItem>
            <SelectItem value={String(DEFAULT_CONTEXT_WINDOW_TOKENS)}>{t('settings.model.contextWindow400k')}</SelectItem>
            <SelectItem value="1000000">{t('settings.model.contextWindow1m')}</SelectItem>
            <SelectItem value="custom">{t('settings.model.contextWindowCustom')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
      {preset === 'custom' && (
        <Input
          type="number"
          min={MIN_CONTEXT_WINDOW_TOKENS}
          max={MAX_CONTEXT_WINDOW_TOKENS}
          step={1000}
          value={customValue}
          placeholder={t('settings.model.contextWindowPlaceholder')}
          onBlur={() => {
            if (customDraft === null) return
            const normalized = normalizeContextWindowDraft(customDraft)
            setCustomDraft(normalized)
            if (normalized === '') {
              onChange(null)
            } else {
              const numeric = Number(normalized)
              if (Number.isFinite(numeric)) onChange(numeric)
            }
          }}
          onChange={(event) => {
            const raw = event.target.value
            setCustomDraft(raw)
            if (raw.trim() === '') return
            const numeric = Number(raw)
            if (Number.isFinite(numeric) && numeric >= MIN_CONTEXT_WINDOW_TOKENS && numeric <= MAX_CONTEXT_WINDOW_TOKENS) {
              onChange(Math.trunc(numeric))
            }
          }}
          className="sm:max-w-40"
        />
      )}
    </div>
  )
}

function normalizeContextWindowDraft(value: string) {
  const trimmed = value.trim()
  if (trimmed === '') return ''
  const numeric = Number(trimmed)
  if (!Number.isFinite(numeric)) return trimmed
  return String(Math.min(Math.max(Math.trunc(numeric), MIN_CONTEXT_WINDOW_TOKENS), MAX_CONTEXT_WINDOW_TOKENS))
}
