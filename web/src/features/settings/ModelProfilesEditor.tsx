import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Check, ChevronDown, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { fetchModelCatalog } from './api'
import { ApiKeyInput } from './ApiKeyInput'
import { ModelDiscoveryInput } from './ModelDiscoveryInput'
import { ModelDiscoveryPicker } from './ModelDiscoveryPicker'
import { ModelProfilePingButton } from './ModelProfilePingButton'
import {
  DEFAULT_MODEL_PROFILE_ID,
  FALLBACK_MODEL_PROTOCOLS,
  MODEL_PROTOCOL_ANTHROPIC_MESSAGES,
  MODEL_PROTOCOL_CHAT_COMPLETIONS,
  MODEL_PROTOCOL_RESPONSES,
  MODEL_PROVIDER_OPENAI_COMPATIBLE,
  modelEndpointID,
  modelEndpointLabel,
  modelProfileID,
  modelProfileLabel,
} from './model-profiles'
import { nextProfileIDAfterRemoval } from './profile-list'
import { SettingsDisclosureCard } from './SettingsDisclosureCard'
import type { ModelCatalog, ModelEndpointSettings, ModelInfo, ModelProfileSettings, ModelProviderPreset } from './types'

const DEFAULT_CONTEXT_WINDOW_TOKENS = 400000
const MIN_CONTEXT_WINDOW_TOKENS = 1024
const MAX_CONTEXT_WINDOW_TOKENS = 2000000
const CONTEXT_WINDOW_PRESETS = [200000, DEFAULT_CONTEXT_WINDOW_TOKENS, 1000000]
const INHERIT_VALUE = '__inherit__'
const PROVIDER_DEFAULT_PROTOCOL = '__provider_default__'
const PROVIDER_DEFAULT_SESSION_KEY_MAPPING = '__provider_default__'
const EMPTY_MODEL_CATALOG: ModelCatalog = { providers: [], protocols: FALLBACK_MODEL_PROTOCOLS }

interface ModelProfilesEditorProps {
  endpoints: ModelEndpointSettings[]
  effectiveEndpoints: ModelEndpointSettings[]
  profiles: ModelProfileSettings[]
  effectiveProfiles: ModelProfileSettings[]
  defaultProfileID: string
  effectiveDefaultProfileID: string
  onDefaultProfileChange: (profileID: string) => void
  onEndpointsChange: (endpoints: ModelEndpointSettings[]) => void
  onProfilesChange: (profiles: ModelProfileSettings[]) => void
}

export function ModelProfilesEditor({
  endpoints,
  effectiveEndpoints,
  profiles,
  effectiveProfiles,
  defaultProfileID,
  effectiveDefaultProfileID,
  onDefaultProfileChange,
  onEndpointsChange,
  onProfilesChange,
}: ModelProfilesEditorProps) {
  const { t } = useTranslation()
  const [catalog, setCatalog] = useState<ModelCatalog>(EMPTY_MODEL_CATALOG)
  const profileKeysRef = useRef<string[]>([])
  const profileKeys = useMemo(() => stableKeys(profileKeysRef, profiles.length, 'model-profile'), [profiles.length])
  const profileOptions = modelProfileOptions(profiles, effectiveProfiles)
  const selectedDefaultProfileID = defaultProfileID || effectiveDefaultProfileID || DEFAULT_MODEL_PROFILE_ID
  const effectiveDefaultLabel = profileOptions.find((profile) => profile.id === effectiveDefaultProfileID)?.label
    || effectiveDefaultProfileID
    || DEFAULT_MODEL_PROFILE_ID

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

  const updateEndpoint = (index: number, patch: Partial<ModelEndpointSettings>) => {
    onEndpointsChange(endpoints.map((endpoint, current) => current === index ? { ...endpoint, ...patch } : endpoint))
  }
  const updateEndpointProvider = (index: number, provider: string) => {
    const endpoint = endpoints[index]
    updateEndpoint(index, {
      provider,
      base_url: baseURLAfterRouteChange(endpoint.base_url, catalog, provider, endpoint.protocol),
      session_key_mapping: endpoint.provider === provider ? endpoint.session_key_mapping : undefined,
    })
  }
  const updateEndpointProtocol = (index: number, protocol: string) => {
    const endpoint = endpoints[index]
    const nextProtocol = protocol === PROVIDER_DEFAULT_PROTOCOL ? '' : protocol
    updateEndpoint(index, {
      protocol: nextProtocol,
      base_url: baseURLAfterRouteChange(endpoint.base_url, catalog, endpoint.provider, nextProtocol),
    })
  }
  const updateProfile = (index: number, patch: Partial<ModelProfileSettings>) => {
    onProfilesChange(profiles.map((profile, current) => current === index ? { ...profile, ...patch } : profile))
  }
  const updateProfileModel = (index: number, model: string) => {
    const profile = profiles[index]
    const previousID = modelProfileID(profile)
    const previousModel = profile.model?.trim() ?? ''
    const shouldSyncID = !previousID || previousID === previousModel
    const nextID = shouldSyncID ? uniqueModelProfileID(model, profiles, index) : profile.id
    updateProfile(index, { id: nextID, model })
    if (shouldSyncID && previousID && selectedDefaultProfileID === previousID && nextID !== previousID) {
      onDefaultProfileChange(nextID || '')
    }
  }
  const removeProfile = (index: number) => {
    const removedID = modelProfileID(profiles[index])
    if (removedID && selectedDefaultProfileID === removedID) {
      onDefaultProfileChange(nextProfileIDAfterRemoval(profiles, index, modelProfileID))
    }
    profileKeysRef.current.splice(index, 1)
    onProfilesChange(profiles.filter((_, current) => current !== index))
  }
  const addModels = (endpoint: ModelEndpointSettings, models: ModelInfo[]) => {
    const next = [...profiles]
    for (const model of models) {
      next.push({
        id: uniqueModelProfileID(model.id, next, -1),
        name: model.display_name && model.display_name !== model.id ? model.display_name : undefined,
        endpoint_id: modelEndpointID(endpoint),
        model: model.id,
        context_window_tokens: DEFAULT_CONTEXT_WINDOW_TOKENS,
      })
    }
    onProfilesChange(next)
  }

  return (
    <div className="nova-settings-row rounded-md px-2 py-1.5">
      <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('settings.model.modelProfiles')}</div>
      <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('settings.model.endpointRoutingHint')}
      </div>
      <div className="flex flex-col gap-2">
        <ModelProfileField label={t('settings.model.defaultProfile')}>
          <Select value={defaultProfileID || INHERIT_VALUE} onValueChange={(value) => onDefaultProfileChange(value === INHERIT_VALUE ? '' : value)}>
            <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectGroup>
                <SelectItem value={INHERIT_VALUE}>{t('common.inherit', { value: effectiveDefaultLabel })}</SelectItem>
                {profileOptions.map((profile) => <SelectItem key={profile.id} value={profile.id}>{profile.label}</SelectItem>)}
              </SelectGroup>
            </SelectContent>
          </Select>
        </ModelProfileField>

        {endpoints.length === 0 && (
          <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-3 text-[var(--nova-text-faint)]">
            {t('settings.model.endpointEmpty', { count: effectiveEndpoints.length })}
          </div>
        )}

        {endpoints.map((endpoint, endpointIndex) => {
          const endpointID = modelEndpointID(endpoint)
          const endpointProfiles = profiles
            .map((profile, index) => ({ profile, index }))
            .filter(({ profile }) => profile.endpoint_id?.trim() === endpointID)
          const defaultProtocol = catalog.providers.find((provider) => provider.id === endpoint.provider)?.default_protocol
          return (
            <SettingsDisclosureCard
              key={endpointID || `endpoint-${endpointIndex}`}
              level="connection"
              badge={t('settings.model.endpointName', { index: endpointIndex + 1 })}
              title={modelEndpointLabel(endpoint) || t('settings.model.endpointUntitled')}
              subtitle={endpointSummary(endpoint, endpointProfiles.length, t)}
              defaultOpen={!isEndpointComplete(endpoint, endpointProfiles.length)}
              actions={(
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={endpointProfiles.length > 0}
                  title={endpointProfiles.length > 0 ? t('settings.model.endpointDeleteBlocked') : t('settings.model.deleteEndpoint')}
                  aria-label={t('settings.model.deleteEndpoint')}
                  onClick={() => onEndpointsChange(endpoints.filter((_, current) => current !== endpointIndex))}
                >
                  <Trash2 />
                </Button>
              )}
            >
              <div className="px-3 pt-2.5 text-[11px] font-medium text-[var(--nova-text-muted)]">
                {t('settings.model.endpointSettings')}
              </div>
              <div className="grid gap-2 p-3 pt-2 md:grid-cols-12">
                <ModelProfileField label={t('settings.model.endpointAliasLabel')} className="md:col-span-4">
                  <Input value={endpoint.name ?? ''} placeholder={t('settings.model.endpointAliasPlaceholder')} onChange={(event) => updateEndpoint(endpointIndex, { name: event.target.value })} />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileProviderLabel')} className="md:col-span-4">
                  <ProviderPicker label={t('settings.model.profileProviderLabel')} providers={catalog.providers} value={endpoint.provider ?? ''} placeholder={t('settings.model.profileProviderPlaceholder')} onChange={(provider) => updateEndpointProvider(endpointIndex, provider)} />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileProtocolLabel')} className="md:col-span-4">
                  <Select value={endpoint.protocol || PROVIDER_DEFAULT_PROTOCOL} onValueChange={(value) => updateEndpointProtocol(endpointIndex, value)}>
                    <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      <SelectItem value={PROVIDER_DEFAULT_PROTOCOL}>{t('settings.model.profileProtocolProviderDefault')}</SelectItem>
                      {catalog.protocols.map((protocol) => <SelectItem key={protocol} value={protocol}>{modelProtocolLabel(protocol, t)}</SelectItem>)}
                    </SelectContent>
                  </Select>
                </ModelProfileField>
                <ModelProfileField label={t('common.baseUrl')} className="md:col-span-6">
                  <Input value={endpoint.base_url ?? ''} placeholder={t('common.baseUrl')} onChange={(event) => updateEndpoint(endpointIndex, { base_url: event.target.value })} />
                </ModelProfileField>
                <ModelProfileField label={t('settings.model.profileKeyLabel')} className="md:col-span-6">
                  <ApiKeyInput label={t('settings.model.profileKeyLabel')} value={endpoint.api_key ?? ''} placeholder={t('settings.model.profileKeyInheritPlaceholder')} onChange={(apiKey) => updateEndpoint(endpointIndex, { api_key: apiKey })} />
                </ModelProfileField>
                {endpoint.provider === MODEL_PROVIDER_OPENAI_COMPATIBLE && (
                  <SessionKeyMapping endpoint={endpoint} onChange={(patch) => updateEndpoint(endpointIndex, patch)} />
                )}
              </div>
              <div className="border-t border-[var(--nova-border)] p-2.5">
                <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                  <span className="text-[11px] font-medium text-[var(--nova-text-muted)]">{t('settings.model.endpointModels', { count: endpointProfiles.length })}</span>
                  <div className="flex flex-wrap gap-2">
                    <ModelDiscoveryPicker endpoint={endpoint} defaultProtocol={defaultProtocol} existingModels={endpointProfiles.map(({ profile }) => profile.model ?? '')} onAdd={(models) => addModels(endpoint, models)} />
                    <Button type="button" variant="outline" size="sm" onClick={() => onProfilesChange([...profiles, {
                      endpoint_id: endpointID,
                      context_window_tokens: DEFAULT_CONTEXT_WINDOW_TOKENS,
                    }])}>
                      <Plus data-icon="inline-start" />{t('settings.model.addProfileManual')}
                    </Button>
                  </div>
                </div>
                <div className={cn(
                  'flex flex-col gap-1.5',
                  endpointProfiles.length > 0 && 'ml-1 border-l border-[var(--nova-border)] pl-2.5',
                )}>
                  {endpointProfiles.length === 0 && <div className="rounded-md border border-dashed border-[var(--nova-border)] px-2.5 py-3 text-xs text-[var(--nova-text-faint)]">{t('settings.model.endpointModelsEmpty')}</div>}
                  {endpointProfiles.map(({ profile, index }, childIndex) => {
                    const profileTitle = modelProfileLabel(profile) || t('settings.model.profileUntitled')
                    return (
                      <SettingsDisclosureCard
                        key={profileKeys[index]}
                        level="model"
                        badge={modelProfileID(profile) === selectedDefaultProfileID ? t('settings.model.defaultProfileName') : t('settings.model.profileName', { index: childIndex + 1 })}
                        title={profileTitle}
                        subtitle={modelProfileSummary(profile, endpoint, profileTitle, t('settings.model.profileModelMissing'))}
                        defaultOpen={!profile.model?.trim()}
                        actions={<Button type="button" variant="ghost" size="icon-sm" onClick={() => removeProfile(index)} aria-label={t('settings.model.deleteProfile')}><Trash2 /></Button>}
                      >
                        <div className="px-2.5 pt-2.5 text-[11px] font-medium text-[var(--nova-text-muted)]">
                          {t('settings.model.profileSettings')}
                        </div>
                        <div className="grid gap-2 p-2.5 pt-2 md:grid-cols-12">
                          <ModelProfileField label={t('settings.model.profileModelLabel')} className="md:col-span-8">
                            <ModelDiscoveryInput endpoint={endpoint} profile={profile} defaultProtocol={defaultProtocol} value={profile.model ?? ''} placeholder={t('settings.model.profileModelPlaceholder')} onChange={(model) => updateProfileModel(index, model)} />
                          </ModelProfileField>
                          <ModelProfileField label={t('settings.model.profileAliasLabel')} className="md:col-span-4">
                            <Input value={profile.name ?? ''} placeholder={t('settings.model.profileAliasPlaceholder')} onChange={(event) => updateProfile(index, { name: event.target.value })} />
                          </ModelProfileField>
                          <ModelProfileField label={t('settings.model.profileTemperatureLabel')} className="md:col-span-3">
                            <Input type="number" step={0.01} min={0} max={1} value={profile.temperature ?? ''} placeholder="0-1" onChange={(event) => updateProfile(index, { temperature: event.target.value === '' ? null : Number(event.target.value) })} className="max-w-24" />
                          </ModelProfileField>
                          <ModelProfileField label={t('settings.model.contextWindow')} className="md:col-span-9">
                            <ContextWindowInput value={profile.context_window_tokens ?? DEFAULT_CONTEXT_WINDOW_TOKENS} onChange={(value) => updateProfile(index, { context_window_tokens: value })} />
                          </ModelProfileField>
                        </div>
                        <div className="border-t border-[var(--nova-border-soft)] px-2.5 py-2"><ModelProfilePingButton endpoint={endpoint} profile={profile} /></div>
                      </SettingsDisclosureCard>
                    )
                  })}
                </div>
              </div>
            </SettingsDisclosureCard>
          )
        })}

        <Button type="button" variant="outline" size="sm" onClick={() => {
          const id = uniqueID('endpoint', endpoints.map(modelEndpointID))
          onEndpointsChange([...endpoints, { id, name: '', provider: '', protocol: MODEL_PROTOCOL_CHAT_COMPLETIONS }])
        }}>
          <Plus data-icon="inline-start" />{t('settings.model.addEndpoint')}
        </Button>
      </div>
    </div>
  )
}

function SessionKeyMapping({ endpoint, onChange }: { endpoint: ModelEndpointSettings; onChange: (patch: Partial<ModelEndpointSettings>) => void }) {
  const { t } = useTranslation()
  return (
    <>
      <ModelProfileField label={t('settings.model.sessionKeyMappingLabel')} className="md:col-span-4">
        <Select
          value={endpoint.session_key_mapping?.location ?? PROVIDER_DEFAULT_SESSION_KEY_MAPPING}
          onValueChange={(location) => onChange({ session_key_mapping: sessionKeyMappingForLocation(location) })}
        >
          <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
          <SelectContent className="nova-panel border text-[var(--nova-text)]">
            <SelectItem value={PROVIDER_DEFAULT_SESSION_KEY_MAPPING}>{t('settings.model.sessionKeyMappingProviderDefault')}</SelectItem>
            <SelectItem value="none">{t('settings.model.sessionKeyMappingDisabled')}</SelectItem>
            <SelectItem value="header">{t('settings.model.sessionKeyMappingHeader')}</SelectItem>
            <SelectItem value="body">{t('settings.model.sessionKeyMappingBody')}</SelectItem>
          </SelectContent>
        </Select>
      </ModelProfileField>
      {(endpoint.session_key_mapping?.location === 'header' || endpoint.session_key_mapping?.location === 'body') && (
        <ModelProfileField label={t('settings.model.sessionKeyFieldLabel')} className="md:col-span-4">
          <Input value={endpoint.session_key_mapping.name ?? ''} placeholder={endpoint.session_key_mapping.location === 'header' ? 'X-Session-Id' : 'session_id'} onChange={(event) => onChange({ session_key_mapping: { ...endpoint.session_key_mapping!, name: event.target.value } })} />
        </ModelProfileField>
      )}
      <div className="text-[11px] leading-5 text-[var(--nova-text-faint)] md:col-span-12">{t('settings.model.sessionKeyMappingHint')}</div>
    </>
  )
}

function sessionKeyMappingForLocation(location: string): ModelEndpointSettings['session_key_mapping'] {
  switch (location) {
    case PROVIDER_DEFAULT_SESSION_KEY_MAPPING:
      return undefined
    case 'none':
      return { location: 'none' }
    case 'header':
      return { location: 'header', name: 'X-Session-Id' }
    case 'body':
      return { location: 'body', name: 'session_id' }
    default:
      return undefined
  }
}

function modelProfileOptions(profiles: ModelProfileSettings[], effectiveProfiles: ModelProfileSettings[]): Array<{ id: string; label: string }> {
  const options = new Map<string, string>()
  for (const profile of [...effectiveProfiles, ...profiles]) {
    const id = modelProfileID(profile)
    if (id) options.set(id, modelProfileLabel(profile) || id)
  }
  return Array.from(options, ([id, label]) => ({ id, label }))
}

function ProviderPicker({ label, providers, value, placeholder, onChange }: { label: string; providers: ModelProviderPreset[]; value: string; placeholder: string; onChange: (provider: string) => void }) {
  const [open, setOpen] = useState(false)
  const normalizedValue = value.trim()
  const selectedProvider = providers.find((provider) => provider.id === normalizedValue)
  const options = normalizedValue !== '' && !selectedProvider
    ? [{ id: normalizedValue, name: normalizedValue, default_protocol: '', endpoints: {} }, ...providers]
    : providers
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" role="combobox" aria-label={label} aria-expanded={open} className="nova-field w-full min-w-0 justify-between px-2.5 text-xs font-normal text-[var(--nova-text)]">
          <span className="min-w-0 flex-1 truncate text-left">{selectedProvider?.name || normalizedValue || placeholder}</span>
          <ChevronDown className={cn('size-4 shrink-0 text-[var(--nova-text-faint)] transition-transform', open && 'rotate-180')} />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" sideOffset={4} collisionPadding={8} className="nova-panel w-[var(--radix-popover-trigger-width)] max-w-[calc(100vw-1rem)] gap-0 border p-1 text-[var(--nova-text)]">
        <div role="listbox" aria-label={label} className="max-h-64 overflow-y-auto overscroll-contain [scrollbar-gutter:stable]">
          {options.map((provider) => (
            <button key={provider.id} type="button" role="option" aria-selected={provider.id === normalizedValue} className={cn('flex h-8 w-full min-w-0 items-center gap-2 rounded-md px-2 text-left text-xs', provider.id === normalizedValue ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]')} onClick={() => { setOpen(false); onChange(provider.id) }}>
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

function endpointSummary(endpoint: ModelEndpointSettings, count: number, t: (key: string, options?: Record<string, unknown>) => string): string {
  const route = endpoint.base_url?.trim() || endpoint.provider?.trim() || t('settings.model.endpointRouteMissing')
  return t('settings.model.endpointSummary', { route, count })
}

function modelProfileSummary(profile: ModelProfileSettings, endpoint: ModelEndpointSettings, title: string, missingModel: string): string {
  const model = profile.model?.trim() ?? ''
  const connection = modelEndpointLabel(endpoint)
  const details: string[] = []
  if (!model) details.push(missingModel)
  else if (model !== title) details.push(model)
  if (connection && connection !== title && connection !== model) details.push(connection)
  return details.join(' · ') || model || missingModel
}

function isEndpointComplete(endpoint: ModelEndpointSettings, modelCount: number): boolean {
  return Boolean(endpoint.provider?.trim() && endpoint.base_url?.trim() && modelCount > 0)
}

function stableKeys(reference: React.MutableRefObject<string[]>, length: number, prefix: string): string[] {
  if (reference.current.length > length) reference.current.length = length
  while (reference.current.length < length) reference.current.push(`${prefix}-${Date.now()}-${reference.current.length}`)
  return reference.current
}

function uniqueModelProfileID(base: string, profiles: ModelProfileSettings[], currentIndex: number): string {
  const used = profiles.flatMap((profile, index) => index === currentIndex ? [] : [modelProfileID(profile)]).filter(Boolean)
  return uniqueID(base.trim() || 'model', used)
}

function uniqueID(base: string, values: string[]): string {
  const normalized = base.trim() || 'item'
  const used = new Set(values.filter(Boolean))
  if (!used.has(normalized)) return normalized
  for (let suffix = 2; ; suffix += 1) {
    const candidate = `${normalized}-${suffix}`
    if (!used.has(candidate)) return candidate
  }
}

function baseURLAfterRouteChange(currentValue: string | undefined, catalog: ModelCatalog, providerID?: string, protocolID?: string) {
  const current = currentValue?.trim() ?? ''
  const knownBaseURLs = new Set(catalog.providers.flatMap((provider) => Object.values(provider.endpoints).map((endpoint) => normalizeBaseURL(endpoint.base_url)).filter(Boolean)))
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
    case MODEL_PROTOCOL_CHAT_COMPLETIONS: return t('settings.model.profileProtocolChatCompletions')
    case MODEL_PROTOCOL_RESPONSES: return t('settings.model.profileProtocolResponses')
    case MODEL_PROTOCOL_ANTHROPIC_MESSAGES: return t('settings.model.profileProtocolAnthropicMessages')
    default: return protocol
  }
}

function ModelProfileField({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return <label className={`flex min-w-0 flex-col gap-1 ${className ?? ''}`}><span className="text-[11px] leading-none text-[var(--nova-text-faint)]">{label}</span>{children}</label>
}

function ContextWindowInput({ value, onChange }: { value: number; onChange: (value: number | null) => void }) {
  const { t } = useTranslation()
  const [customDraft, setCustomDraft] = useState<string | null>(null)
  const preset = customDraft === null && CONTEXT_WINDOW_PRESETS.includes(value) ? String(value) : 'custom'
  const customValue = customDraft ?? String(value)
  return (
    <div className="flex w-full min-w-0 flex-1 flex-col gap-2 sm:flex-row">
      <Select value={preset} onValueChange={(nextValue) => {
        if (nextValue === 'custom') { setCustomDraft(String(value)); return }
        setCustomDraft(null)
        onChange(Number(nextValue))
      }}>
        <SelectTrigger size="sm" className="w-full min-w-0 flex-1"><SelectValue /></SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value="200000">{t('settings.model.contextWindow200k')}</SelectItem>
            <SelectItem value={String(DEFAULT_CONTEXT_WINDOW_TOKENS)}>{t('settings.model.contextWindow400k')}</SelectItem>
            <SelectItem value="1000000">{t('settings.model.contextWindow1m')}</SelectItem>
            <SelectItem value="custom">{t('settings.model.contextWindowCustom')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
      {preset === 'custom' && <Input type="number" min={MIN_CONTEXT_WINDOW_TOKENS} max={MAX_CONTEXT_WINDOW_TOKENS} step={1000} value={customValue} placeholder={t('settings.model.contextWindowPlaceholder')} onBlur={() => {
        if (customDraft === null) return
        const normalized = normalizeContextWindowDraft(customDraft)
        setCustomDraft(normalized)
        onChange(normalized === '' ? null : Number(normalized))
      }} onChange={(event) => {
        const raw = event.target.value
        setCustomDraft(raw)
        const numeric = Number(raw)
        if (raw.trim() && Number.isFinite(numeric) && numeric >= MIN_CONTEXT_WINDOW_TOKENS && numeric <= MAX_CONTEXT_WINDOW_TOKENS) onChange(Math.trunc(numeric))
      }} className="sm:max-w-40" />}
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
