import { useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Check, ChevronDown, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { fetchModelCatalog } from './api'
import { ApiKeyInput } from './ApiKeyInput'
import { ModelDiscoveryInput } from './ModelDiscoveryInput'
import { ModelProfilePingButton } from './ModelProfilePingButton'
import {
  DEFAULT_MODEL_PROFILE_ID,
  FALLBACK_MODEL_PROTOCOLS,
  MODEL_PROTOCOL_ANTHROPIC_MESSAGES,
  MODEL_PROTOCOL_CHAT_COMPLETIONS,
  MODEL_PROTOCOL_RESPONSES,
  MODEL_PROVIDER_OPENAI_COMPATIBLE,
  modelProfileID,
  modelProfileLabel,
} from './model-profiles'
import type { ModelCatalog, ModelProfileSettings, ModelProviderPreset } from './types'

const DEFAULT_CONTEXT_WINDOW_TOKENS = 400000
const MIN_CONTEXT_WINDOW_TOKENS = 1024
const MAX_CONTEXT_WINDOW_TOKENS = 2000000
const CONTEXT_WINDOW_PRESETS = [200000, DEFAULT_CONTEXT_WINDOW_TOKENS, 1000000]
const PROVIDER_DEFAULT_PROTOCOL = '__provider_default__'
const PROVIDER_DEFAULT_SESSION_KEY_MAPPING = '__provider_default__'
const EMPTY_MODEL_CATALOG: ModelCatalog = { providers: [], protocols: FALLBACK_MODEL_PROTOCOLS }

export function ModelProfilesEditor({ profiles, effectiveProfiles, onChange }: {
  profiles: ModelProfileSettings[]
  effectiveProfiles: ModelProfileSettings[]
  onChange: (profiles: ModelProfileSettings[]) => void
}) {
  const { t } = useTranslation()
  const [catalog, setCatalog] = useState<ModelCatalog>(EMPTY_MODEL_CATALOG)
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

  useEffect(() => {
    const request = new AbortController()
    void fetchModelCatalog(request.signal).then(
      (nextCatalog) => setCatalog({
        providers: nextCatalog.providers ?? [],
        protocols: nextCatalog.protocols?.length ? nextCatalog.protocols : FALLBACK_MODEL_PROTOCOLS,
      }),
      () => undefined,
    )
    return () => request.abort()
  }, [])

  const addProfile = () => {
    onChange([...profiles, {
      context_window_tokens: DEFAULT_CONTEXT_WINDOW_TOKENS,
    }])
  }
  const updateProfile = (index: number, patch: Partial<ModelProfileSettings>) => {
    onChange(profiles.map((profile, profileIndex) => (profileIndex === index ? { ...profile, ...patch } : profile)))
  }
  const updateProfileModel = (index: number, model: string) => {
    const profile = profiles[index]
    const previousID = modelProfileID(profile)
    const previousModel = profile?.model?.trim() ?? ''
    const shouldSyncID = !previousID || previousID === previousModel
    updateProfile(index, {
      id: shouldSyncID ? model : profile?.id,
      model,
    })
  }
  const updateProfileProvider = (index: number, provider: string) => {
    const profile = profiles[index]
    updateProfile(index, {
      provider,
      base_url: baseURLAfterRouteChange(profile?.base_url, catalog, provider, profile?.protocol),
      session_key_mapping: profile?.provider === provider ? profile.session_key_mapping : undefined,
    })
  }
  const updateProfileProtocol = (index: number, protocol: string) => {
    const profile = profiles[index]
    const nextProtocol = protocol === PROVIDER_DEFAULT_PROTOCOL ? '' : protocol
    updateProfile(index, {
      protocol: nextProtocol,
      base_url: baseURLAfterRouteChange(profile?.base_url, catalog, profile?.provider, nextProtocol),
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
                    {profile.model?.trim() || t('settings.model.profileModelMissing')}
                  </div>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="icon-sm"
                  onClick={() => removeProfile(index)}
                  aria-label={t('settings.model.deleteProfile')}
                >
                  <Trash2 data-icon="inline-start" />
                </Button>
              </div>
              <Separator />
              <div className="grid gap-2 p-2.5 md:grid-cols-12">
                <ModelProfileField label={t('common.baseUrl')} className="md:col-span-5">
                  <Input
                    value={profile.base_url ?? ''}
                    placeholder={t('common.baseUrl')}
                    onChange={(event) => updateProfile(index, { base_url: event.target.value })}
                  />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileModelLabel')} className="md:col-span-4">
                  <ModelDiscoveryInput
                    profile={profile}
                    defaultProtocol={catalog.providers.find((provider) => provider.id === profile.provider)?.default_protocol}
                    value={profile.model ?? ''}
                    placeholder={t('settings.model.profileModelPlaceholder')}
                    onChange={(model) => updateProfileModel(index, model)}
                  />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileAliasLabel')} className="md:col-span-3">
                  <Input
                    value={profile.name ?? ''}
                    placeholder={t('settings.model.profileAliasPlaceholder')}
                    onChange={(event) => updateProfile(index, { name: event.target.value })}
                  />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileProviderLabel')} className="md:col-span-4">
                  <ProviderPicker
                    label={t('settings.model.profileProviderLabel')}
                    providers={catalog.providers}
                    value={profile.provider ?? ''}
                    placeholder={t('settings.model.profileProviderPlaceholder')}
                    onChange={(provider) => updateProfileProvider(index, provider)}
                  />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileProtocolLabel')} className="md:col-span-4">
                  <Select value={profile.protocol || PROVIDER_DEFAULT_PROTOCOL} onValueChange={(value) => updateProfileProtocol(index, value)}>
                    <SelectTrigger size="sm" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      <SelectItem value={PROVIDER_DEFAULT_PROTOCOL}>{t('settings.model.profileProtocolProviderDefault')}</SelectItem>
                      {catalog.protocols.map((protocol) => (
                        <SelectItem key={protocol} value={protocol}>{modelProtocolLabel(protocol, t)}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileKeyLabel')} className="md:col-span-4">
                  <ApiKeyInput
                    label={t('settings.model.profileKeyLabel')}
                    value={profile.api_key ?? ''}
                    placeholder={t('settings.model.profileKeyInheritPlaceholder')}
                    onChange={(apiKey) => updateProfile(index, { api_key: apiKey })}
                  />
                </ModelProfileField>
                {profile.provider === MODEL_PROVIDER_OPENAI_COMPATIBLE && (
                  <>
                    <ModelProfileField label={t('settings.model.sessionKeyMappingLabel')} className="md:col-span-4">
                      <Select
                        value={profile.session_key_mapping?.location ?? PROVIDER_DEFAULT_SESSION_KEY_MAPPING}
                        onValueChange={(location) => updateProfile(index, {
                          session_key_mapping: location === PROVIDER_DEFAULT_SESSION_KEY_MAPPING
                            ? undefined
                            : location === 'none'
                              ? { location: 'none' }
                              : {
                                  location: location as 'header' | 'body',
                                  name: location === 'header' ? 'X-Session-Id' : 'session_id',
                                },
                        })}
                      >
                        <SelectTrigger size="sm" className="w-full" aria-label={t('settings.model.sessionKeyMappingLabel')}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="nova-panel border text-[var(--nova-text)]">
                          <SelectItem value={PROVIDER_DEFAULT_SESSION_KEY_MAPPING}>{t('settings.model.sessionKeyMappingProviderDefault')}</SelectItem>
                          <SelectItem value="none">{t('settings.model.sessionKeyMappingDisabled')}</SelectItem>
                          <SelectItem value="header">{t('settings.model.sessionKeyMappingHeader')}</SelectItem>
                          <SelectItem value="body">{t('settings.model.sessionKeyMappingBody')}</SelectItem>
                        </SelectContent>
                      </Select>
                    </ModelProfileField>
                    {(profile.session_key_mapping?.location === 'header' || profile.session_key_mapping?.location === 'body') && (
                      <ModelProfileField label={t('settings.model.sessionKeyFieldLabel')} className="md:col-span-4">
                        <Input
                          value={profile.session_key_mapping.name ?? ''}
                          placeholder={profile.session_key_mapping.location === 'header' ? 'X-Session-Id' : 'session_id'}
                          onChange={(event) => updateProfile(index, {
                            session_key_mapping: { ...profile.session_key_mapping!, name: event.target.value },
                          })}
                        />
                      </ModelProfileField>
                    )}
                    <div className="text-[11px] leading-5 text-[var(--nova-text-faint)] md:col-span-12">
                      {t('settings.model.sessionKeyMappingHint')}
                    </div>
                  </>
                )}
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
                <ModelProfileField label={t('settings.model.maxOutputTokens')} className="md:col-span-5">
                  <Input
                    type="number"
                    min={1}
                    step={1}
                    value={profile.max_output_tokens ?? ''}
                    placeholder={t('settings.model.maxOutputTokensPlaceholder')}
                    onChange={(event) => updateProfile(index, {
                      max_output_tokens: event.target.value === '' ? null : Math.max(1, Math.trunc(Number(event.target.value))),
                    })}
                  />
                </ModelProfileField>
              </div>
              <Separator />
              <div className="px-2.5 py-2">
                <ModelProfilePingButton profile={profile} />
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

function ProviderPicker({ label, providers, value, placeholder, onChange }: {
  label: string
  providers: ModelProviderPreset[]
  value: string
  placeholder: string
  onChange: (provider: string) => void
}) {
  const [open, setOpen] = useState(false)
  const normalizedValue = value.trim()
  const selectedProvider = providers.find((provider) => provider.id === normalizedValue)
  // Keep an unknown persisted value visible so users can deliberately migrate
  // away from it, but only catalog presets can be newly selected.
  const includesCurrentCustomProvider = normalizedValue !== '' && !selectedProvider
  const options = includesCurrentCustomProvider
    ? [{ id: normalizedValue, name: normalizedValue, default_protocol: '', endpoints: {} }, ...providers]
    : providers

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          role="combobox"
          aria-label={label}
          aria-expanded={open}
          className="nova-field w-full min-w-0 justify-between px-2.5 text-xs font-normal text-[var(--nova-text)]"
        >
          <span className="min-w-0 flex-1 truncate text-left">
            {selectedProvider?.name || normalizedValue || placeholder}
          </span>
          <ChevronDown className={cn('size-4 shrink-0 text-[var(--nova-text-faint)] transition-transform', open && 'rotate-180')} />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={4}
        collisionPadding={8}
        className="nova-panel w-[var(--radix-popover-trigger-width)] max-w-[calc(100vw-1rem)] gap-0 border p-1 text-[var(--nova-text)]"
      >
        <div
          role="listbox"
          aria-label={label}
          data-provider-list
          className="max-h-64 overflow-y-auto overscroll-contain [scrollbar-gutter:stable]"
        >
          {options.map((provider) => (
            <button
              key={provider.id}
              type="button"
              role="option"
              aria-selected={provider.id === normalizedValue}
              data-provider-option
              className={cn(
                'flex h-8 w-full min-w-0 items-center gap-2 rounded-md px-2 text-left text-xs',
                provider.id === normalizedValue
                  ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
                  : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
              )}
              onClick={() => {
                setOpen(false)
                onChange(provider.id)
              }}
            >
              <span className="min-w-0 flex-1 truncate">{provider.name}</span>
              {provider.name !== provider.id && <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{provider.id}</span>}
              {provider.id === normalizedValue && <Check className="size-3.5 shrink-0" />}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}

function baseURLAfterRouteChange(currentValue: string | undefined, catalog: ModelCatalog, providerID?: string, protocolID?: string) {
  const current = currentValue?.trim() ?? ''
  const knownBaseURLs = new Set(catalog.providers.flatMap((provider) =>
    Object.values(provider.endpoints).map((endpoint) => normalizeBaseURL(endpoint.base_url)).filter(Boolean),
  ))
  if (current && !knownBaseURLs.has(normalizeBaseURL(current))) return current

  const provider = catalog.providers.find((candidate) => candidate.id === providerID?.trim())
  const protocol = protocolID?.trim() || provider?.default_protocol
  return (protocol && provider?.endpoints[protocol]?.base_url?.trim()) || ''
}

function normalizeBaseURL(value?: string) {
  return value?.trim().toLowerCase().replace(/\/+$/, '') ?? ''
}

function modelProtocolLabel(protocol: string, t: (key: string) => string) {
  switch (protocol) {
    case MODEL_PROTOCOL_CHAT_COMPLETIONS:
      return t('settings.model.profileProtocolChatCompletions')
    case MODEL_PROTOCOL_RESPONSES:
      return t('settings.model.profileProtocolResponses')
    case MODEL_PROTOCOL_ANTHROPIC_MESSAGES:
      return t('settings.model.profileProtocolAnthropicMessages')
    default:
      return protocol
  }
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
