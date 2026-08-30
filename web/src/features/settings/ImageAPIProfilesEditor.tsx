import { useMemo, useRef, type MutableRefObject, type ReactNode } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { ApiKeyInput } from './ApiKeyInput'
import { ComfyUIWorkflowBrowser } from './ComfyUIWorkflowBrowser'
import { ImageProfilePingButton } from './ImageProfilePingButton'
import {
  DEFAULT_IMAGE_API_PROFILE_ID,
  IMAGE_API_PROTOCOLS,
  imageAPIEndpointDefaults,
  imageAPIEndpointID,
  imageAPIEndpointLabel,
  imageAPIProfileDefaults,
  imageAPIProfileID,
  imageAPIProfileLabel,
  imageAPIProvider,
  newImageAPIEndpoint,
  newImageAPIProfile,
  type ImageAPIProvider,
} from './image-profiles'
import { nextProfileIDAfterRemoval } from './profile-list'
import { SettingsDisclosureCard } from './SettingsDisclosureCard'
import type { ImageAPIEndpointSettings, ImageAPIProfileSettings } from './types'

const INHERIT_VALUE = '__inherit__'
const PROVIDER_DEFAULT_VALUE = '__provider_default__'
const MAX_PROMPT_GUIDE_LENGTH = 64 * 1024
const OPENAI_QUALITY_OPTIONS = ['auto', 'high', 'medium', 'low', 'standard', 'hd']
const XAI_QUALITY_OPTIONS = ['medium', 'low']
const FORMAT_OPTIONS = ['png', 'jpeg', 'webp']
const XAI_ASPECT_RATIO_OPTIONS = ['1:1', '16:9', '9:16', '4:3', '3:4', '3:2', '2:3', '2:1', '1:2', '19.5:9', '9:19.5', '20:9', '9:20']
const GEMINI_ASPECT_RATIO_OPTIONS = ['1:1', '1:4', '1:8', '2:3', '3:2', '3:4', '4:1', '4:3', '4:5', '5:4', '8:1', '9:16', '16:9', '21:9']
const XAI_RESOLUTION_OPTIONS = ['1k', '2k']
const GEMINI_RESOLUTION_OPTIONS = ['512', '1K', '2K', '4K']
const ARK_RESOLUTION_OPTIONS = ['1K', '2K', '4K']
const IMAGE_PROVIDERS: ImageAPIProvider[] = ['openai', 'xai', 'comfyui', 'volcengine', 'google', 'custom']

interface ImageAPIProfilesEditorProps {
  endpoints: ImageAPIEndpointSettings[]
  effectiveEndpoints: ImageAPIEndpointSettings[]
  profiles: ImageAPIProfileSettings[]
  effectiveProfiles: ImageAPIProfileSettings[]
  defaultProfileID: string
  effectiveDefaultProfileID: string
  onDefaultProfileChange: (profileID: string) => void
  onEndpointsChange: (endpoints: ImageAPIEndpointSettings[]) => void
  onProfilesChange: (profiles: ImageAPIProfileSettings[]) => void
}

export function ImageAPIProfilesEditor({
  endpoints,
  effectiveEndpoints,
  profiles,
  effectiveProfiles,
  defaultProfileID,
  effectiveDefaultProfileID,
  onDefaultProfileChange,
  onEndpointsChange,
  onProfilesChange,
}: ImageAPIProfilesEditorProps) {
  const { t } = useTranslation()
  const profileKeysRef = useRef<string[]>([])
  const profileKeys = useMemo(() => stableKeys(profileKeysRef, profiles.length), [profiles.length])
  const profileOptions = imageProfileOptions(profiles, effectiveProfiles)
  const effectiveDefaultLabel = profileOptions.find((profile) => profile.id === effectiveDefaultProfileID)?.label
    || effectiveDefaultProfileID
    || DEFAULT_IMAGE_API_PROFILE_ID
  const selectedDefaultProfileID = defaultProfileID || effectiveDefaultProfileID || DEFAULT_IMAGE_API_PROFILE_ID

  const updateEndpoint = (index: number, patch: Partial<ImageAPIEndpointSettings>) => {
    onEndpointsChange(endpoints.map((endpoint, current) => current === index ? { ...endpoint, ...patch } : endpoint))
  }
  const replaceEndpointProvider = (index: number, provider: ImageAPIProvider) => {
    const current = endpoints[index]
    onEndpointsChange(endpoints.map((endpoint, currentIndex) => currentIndex === index
      ? { id: current.id, name: current.name, ...newImageAPIEndpoint(provider) }
      : endpoint))
  }
  const updateProfile = (index: number, patch: Partial<ImageAPIProfileSettings>) => {
    onProfilesChange(profiles.map((profile, current) => current === index ? { ...profile, ...patch } : profile))
  }
  const updateModel = (index: number, model: string) => {
    const current = profiles[index]
    const previousID = imageAPIProfileID(current)
    const previousModel = current.model?.trim() ?? ''
    const syncID = !previousID || previousID === previousModel
    const nextID = syncID ? uniqueImageProfileID(model, profiles, index) : current.id
    updateProfile(index, { id: nextID, model })
    if (syncID && previousID && selectedDefaultProfileID === previousID && nextID !== previousID) onDefaultProfileChange(nextID || '')
  }
  const removeProfile = (index: number) => {
    const removedID = imageAPIProfileID(profiles[index])
    if (removedID && selectedDefaultProfileID === removedID) {
      onDefaultProfileChange(nextProfileIDAfterRemoval(profiles, index, imageAPIProfileID))
    }
    profileKeysRef.current.splice(index, 1)
    onProfilesChange(profiles.filter((_, current) => current !== index))
  }
  return (
    <div className="nova-settings-row rounded-md px-2 py-1.5">
      <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('settings.imageApi.profiles')}</div>
      <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('settings.imageApi.endpointRoutingHint')}
      </div>
      <div className="flex flex-col gap-2">
        <ProfileField label={t('settings.imageApi.defaultProfile')}>
          <Select value={defaultProfileID || INHERIT_VALUE} onValueChange={(value) => onDefaultProfileChange(value === INHERIT_VALUE ? '' : value)}>
            <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectGroup>
                <SelectItem value={INHERIT_VALUE}>{t('common.inherit', { value: effectiveDefaultLabel })}</SelectItem>
                {profileOptions.map((profile) => <SelectItem key={profile.id} value={profile.id}>{profile.label}</SelectItem>)}
              </SelectGroup>
            </SelectContent>
          </Select>
        </ProfileField>

        {endpoints.length === 0 && (
          <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-3 text-[var(--nova-text-faint)]">
            {t('settings.imageApi.endpointEmpty', { count: effectiveEndpoints.length })}
          </div>
        )}

        {endpoints.map((endpoint, endpointIndex) => {
          const endpointID = imageAPIEndpointID(endpoint)
          const provider = imageAPIProvider(endpoint.provider)
          const endpointDefaults = imageAPIEndpointDefaults(provider)
          const protocol = endpoint.protocol || endpointDefaults.protocol || 'openai-images'
          const endpointProfiles = profiles
            .map((profile, index) => ({ profile, index }))
            .filter(({ profile }) => profile.endpoint_id?.trim() === endpointID)
          return (
            <SettingsDisclosureCard
              key={endpointID || `image-endpoint-${endpointIndex}`}
              level="connection"
              badge={t('settings.imageApi.endpointName', { index: endpointIndex + 1 })}
              title={imageAPIEndpointLabel(endpoint) || t('settings.imageApi.endpointUntitled')}
              subtitle={t('settings.imageApi.endpointSummary', {
                route: endpoint.base_url?.trim() || providerLabel(provider, t),
                count: endpointProfiles.length,
              })}
              defaultOpen={!endpoint.base_url?.trim() || endpointProfiles.length === 0}
              actions={<Button type="button" variant="ghost" size="icon-sm" disabled={endpointProfiles.length > 0} title={endpointProfiles.length > 0 ? t('settings.imageApi.endpointDeleteBlocked') : t('settings.imageApi.deleteEndpoint')} aria-label={t('settings.imageApi.deleteEndpoint')} onClick={() => onEndpointsChange(endpoints.filter((_, current) => current !== endpointIndex))}><Trash2 /></Button>}
            >
              <div className="px-3 pt-2.5 text-[11px] font-medium text-[var(--nova-text-muted)]">
                {t('settings.imageApi.endpointSettings')}
              </div>
              <div className="grid gap-2 p-3 pt-2 md:grid-cols-12">
                <ProfileField label={t('settings.imageApi.endpointAliasLabel')} className="md:col-span-3">
                  <Input value={endpoint.name ?? ''} placeholder={t('settings.imageApi.endpointAliasPlaceholder')} onChange={(event) => updateEndpoint(endpointIndex, { name: event.target.value })} />
                </ProfileField>
                <ProfileField label={t('settings.imageApi.provider')} className="md:col-span-3">
                  <Select value={provider} onValueChange={(value) => replaceEndpointProvider(endpointIndex, value as ImageAPIProvider)}>
                    <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      <SelectGroup>{IMAGE_PROVIDERS.map((value) => <SelectItem key={value} value={value}>{providerLabel(value, t)}</SelectItem>)}</SelectGroup>
                    </SelectContent>
                  </Select>
                </ProfileField>
                {provider === 'custom' && (
                  <ProfileField label={t('settings.imageApi.protocol')} className="md:col-span-3">
                    <Select value={protocol} onValueChange={(value) => updateEndpoint(endpointIndex, { protocol: value })}>
                      <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
                      <SelectContent className="nova-panel border text-[var(--nova-text)]">
                        <SelectGroup>{IMAGE_API_PROTOCOLS.map((value) => <SelectItem key={value} value={value}>{protocolLabel(value, t)}</SelectItem>)}</SelectGroup>
                      </SelectContent>
                    </Select>
                  </ProfileField>
                )}
                <ProfileField label={t('common.baseUrl')} className={provider === 'custom' ? 'md:col-span-3' : 'md:col-span-6'}>
                  <Input value={endpoint.base_url ?? ''} placeholder={endpointDefaults.base_url || t('settings.imageApi.baseUrlPlaceholder')} onChange={(event) => updateEndpoint(endpointIndex, { base_url: event.target.value })} />
                </ProfileField>
                {provider !== 'comfyui' && (
                  <ProfileField label={t('settings.imageApi.profileKeyLabel')} className="md:col-span-6">
                    <ApiKeyInput label={t('settings.imageApi.profileKeyLabel')} value={endpoint.api_key ?? ''} placeholder={t('settings.imageApi.profileKeyInheritPlaceholder')} onChange={(apiKey) => updateEndpoint(endpointIndex, { api_key: apiKey })} />
                  </ProfileField>
                )}
              </div>

              <div className="border-t border-[var(--nova-border)] p-2.5">
                <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                  <span className="text-[11px] font-medium text-[var(--nova-text-muted)]">{t('settings.imageApi.endpointModels', { count: endpointProfiles.length })}</span>
                  <Button type="button" variant="outline" size="sm" onClick={() => {
                    const profile = newImageAPIProfile(provider, endpointID)
                    profile.id = uniqueImageProfileID(profile.model || `${provider}-image`, profiles, -1)
                    profile.model = ''
                    onProfilesChange([...profiles, profile])
                  }}>
                    <Plus data-icon="inline-start" />{t('settings.imageApi.addProfile')}
                  </Button>
                </div>
                <div className={cn(
                  'flex flex-col gap-1.5',
                  endpointProfiles.length > 0 && 'ml-1 border-l border-[var(--nova-border)] pl-2.5',
                )}>
                  {endpointProfiles.length === 0 && <div className="rounded-md border border-dashed border-[var(--nova-border)] px-2.5 py-3 text-xs text-[var(--nova-text-faint)]">{t('settings.imageApi.endpointModelsEmpty')}</div>}
                  {endpointProfiles.map(({ profile, index }, childIndex) => {
                    const defaults = imageAPIProfileDefaults(provider)
                    const usesComfyUI = protocol === 'comfyui-workflow'
                    const profileKey = profileKeys[index]
                    const configured = usesComfyUI ? Boolean(profile.comfyui?.workflow) : Boolean(profile.model?.trim())
                    const profileTitle = imageAPIProfileLabel(profile) || t('settings.imageApi.profileUntitled')
                    const profileIdentity = (usesComfyUI ? profile.comfyui?.workflow_name?.trim() : profile.model?.trim()) ?? ''
                    return (
                      <SettingsDisclosureCard
                        key={profileKey}
                        level="model"
                        badge={imageAPIProfileID(profile) === selectedDefaultProfileID ? t('settings.imageApi.defaultProfileName') : t('settings.imageApi.profileName', { index: childIndex + 1 })}
                        title={profileTitle}
                        subtitle={imageProfileSummary(profileIdentity, endpoint, profileTitle, t('settings.imageApi.profileModelMissing'))}
                        defaultOpen={!configured}
                        actions={<Button type="button" variant="ghost" size="icon-sm" onClick={() => removeProfile(index)} aria-label={t('settings.imageApi.deleteProfile')}><Trash2 /></Button>}
                      >
                        <div className="px-2.5 pt-2.5 text-[11px] font-medium text-[var(--nova-text-muted)]">
                          {t('settings.imageApi.profileSettings')}
                        </div>
                        <div className="grid gap-2 p-2.5 pt-2 md:grid-cols-12">
                          {!usesComfyUI && (
                            <ProfileField label={t('settings.imageApi.profileModelLabel')} className="md:col-span-8">
                              <Input value={profile.model ?? ''} placeholder={defaults.model || t('settings.imageApi.profileModelPlaceholder')} onChange={(event) => updateModel(index, event.target.value)} />
                            </ProfileField>
                          )}
                          <ProfileField label={t('settings.imageApi.profileAliasLabel')} className={usesComfyUI ? 'md:col-span-12' : 'md:col-span-4'}>
                            <Input value={profile.name ?? ''} placeholder={t('settings.imageApi.profileAliasPlaceholder')} onChange={(event) => updateProfile(index, { name: event.target.value })} />
                          </ProfileField>
                          {usesComfyUI && (
                            <ComfyUIWorkflowBrowser endpoint={endpoint} profile={profile} onChange={(comfyui) => updateProfile(index, { comfyui })} />
                          )}
                          <ProtocolOptions protocol={protocol} profile={profile} defaults={defaults} onUpdate={(patch) => updateProfile(index, patch)} />
                          <ProfileField label={t('settings.imageApi.promptGuide')} className="md:col-span-12">
                            <Textarea aria-label={t('settings.imageApi.promptGuide')} value={profile.prompt_guide ?? ''} maxLength={MAX_PROMPT_GUIDE_LENGTH} rows={5} placeholder={t('settings.imageApi.promptGuidePlaceholder')} onChange={(event) => updateProfile(index, { prompt_guide: event.target.value })} />
                            <span className="text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('settings.imageApi.promptGuideHint')}</span>
                          </ProfileField>
                        </div>
                        <div className="border-t border-[var(--nova-border-soft)] px-2.5 py-2"><ImageProfilePingButton endpoint={endpoint} profile={profile} /></div>
                      </SettingsDisclosureCard>
                    )
                  })}
                </div>
              </div>
            </SettingsDisclosureCard>
          )
        })}

        <Button type="button" variant="outline" size="sm" onClick={() => {
          const id = uniqueID('image-endpoint', endpoints.map(imageAPIEndpointID))
          onEndpointsChange([...endpoints, { id, name: '', ...newImageAPIEndpoint('openai') }])
        }}>
          <Plus data-icon="inline-start" />{t('settings.imageApi.addEndpoint')}
        </Button>
      </div>
    </div>
  )
}

function ProtocolOptions({ protocol, profile, defaults, onUpdate }: { protocol: string; profile: ImageAPIProfileSettings; defaults: ImageAPIProfileSettings; onUpdate: (patch: Partial<ImageAPIProfileSettings>) => void }) {
  const { t } = useTranslation()
  const { aspectRatioOptions, resolutionOptions, qualityOptions } = protocolOptionLists(protocol)
  return (
    <>
      {(protocol === 'openai-images' || protocol === 'comfyui-workflow') && (
        <ProfileField label={t('settings.imageApi.defaultSize')} className="md:col-span-3">
          <Input value={profile.default_size ?? ''} placeholder={defaults.default_size || t('settings.imageApi.providerDefault')} onChange={(event) => onUpdate({ default_size: event.target.value })} />
        </ProfileField>
      )}
      {aspectRatioOptions && <OptionSelect label={t('settings.imageApi.defaultAspectRatio')} value={profile.default_aspect_ratio ?? ''} options={aspectRatioOptions} onChange={(value) => onUpdate({ default_aspect_ratio: value })} />}
      {resolutionOptions && <OptionSelect label={t('settings.imageApi.defaultResolution')} value={profile.default_resolution ?? ''} options={resolutionOptions} onChange={(value) => onUpdate({ default_resolution: value })} />}
      {qualityOptions && <OptionSelect label={t('settings.imageApi.defaultQuality')} value={profile.default_quality ?? ''} options={qualityOptions} onChange={(value) => onUpdate({ default_quality: value })} />}
      {(protocol === 'openai-images' || protocol === 'ark-images') && <OptionSelect label={t('settings.imageApi.defaultOutputFormat')} value={profile.default_output_format ?? ''} options={FORMAT_OPTIONS} onChange={(value) => onUpdate({ default_output_format: value })} />}
    </>
  )
}

function protocolOptionLists(protocol: string): {
  aspectRatioOptions?: string[]
  resolutionOptions?: string[]
  qualityOptions?: string[]
} {
  switch (protocol) {
    case 'xai-images':
      return {
        aspectRatioOptions: XAI_ASPECT_RATIO_OPTIONS,
        resolutionOptions: XAI_RESOLUTION_OPTIONS,
        qualityOptions: XAI_QUALITY_OPTIONS,
      }
    case 'gemini-images':
      return {
        aspectRatioOptions: GEMINI_ASPECT_RATIO_OPTIONS,
        resolutionOptions: GEMINI_RESOLUTION_OPTIONS,
      }
    case 'ark-images':
      return { resolutionOptions: ARK_RESOLUTION_OPTIONS }
    case 'openai-images':
      return { qualityOptions: OPENAI_QUALITY_OPTIONS }
    case 'comfyui-workflow':
      return {}
    default:
      return {}
  }
}

function OptionSelect({ label, value, options, onChange }: { label: string; value: string; options: string[]; onChange: (value: string) => void }) {
  const { t } = useTranslation()
  return (
    <ProfileField label={label} className="md:col-span-3">
      <Select value={value || PROVIDER_DEFAULT_VALUE} onValueChange={(next) => onChange(next === PROVIDER_DEFAULT_VALUE ? '' : next)}>
        <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]"><SelectGroup><SelectItem value={PROVIDER_DEFAULT_VALUE}>{t('settings.imageApi.providerDefault')}</SelectItem>{options.map((option) => <SelectItem key={option} value={option}>{option}</SelectItem>)}</SelectGroup></SelectContent>
      </Select>
    </ProfileField>
  )
}

function ProfileField({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return <label className={`flex min-w-0 flex-col gap-1 ${className ?? ''}`}><span className="text-[11px] leading-none text-[var(--nova-text-faint)]">{label}</span>{children}</label>
}

function imageProfileOptions(localProfiles: ImageAPIProfileSettings[], effectiveProfiles: ImageAPIProfileSettings[]) {
  const options: Array<{ id: string; label: string }> = []
  const seen = new Set<string>()
  for (const profile of [...effectiveProfiles, ...localProfiles]) {
    const id = imageAPIProfileID(profile)
    if (!id || seen.has(id)) continue
    seen.add(id)
    options.push({ id, label: imageAPIProfileLabel(profile) || id })
  }
  return options
}

function imageProfileSummary(identity: string, endpoint: ImageAPIEndpointSettings, title: string, missingModel: string): string {
  const connection = imageAPIEndpointLabel(endpoint)
  const details: string[] = []
  if (!identity) details.push(missingModel)
  else if (identity !== title) details.push(identity)
  if (connection && connection !== title && connection !== identity) details.push(connection)
  return details.join(' · ') || identity || missingModel
}

function stableKeys(reference: MutableRefObject<string[]>, length: number): string[] {
  if (reference.current.length > length) reference.current.length = length
  while (reference.current.length < length) reference.current.push(`image-profile-${Date.now()}-${reference.current.length}`)
  return reference.current
}

function uniqueImageProfileID(base: string, profiles: ImageAPIProfileSettings[], currentIndex: number): string {
  const used = profiles.flatMap((profile, index) => index === currentIndex ? [] : [imageAPIProfileID(profile)]).filter(Boolean)
  return uniqueID(base.trim() || 'image-model', used)
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

function providerLabel(provider: ImageAPIProvider, t: (key: string) => string): string {
  switch (provider) {
    case 'openai': return 'OpenAI'
    case 'xai': return 'xAI / Grok'
    case 'comfyui': return 'ComfyUI'
    case 'volcengine': return t('settings.imageApi.providerVolcengine')
    case 'google': return 'Google Gemini'
    case 'custom': return t('settings.imageApi.providerCustom')
  }
}

function protocolLabel(protocol: string, t: (key: string) => string): string {
  switch (protocol) {
    case 'openai-images': return 'OpenAI Images API'
    case 'xai-images': return 'xAI Images API'
    case 'ark-images': return t('settings.imageApi.protocolArk')
    case 'gemini-images': return 'Gemini generateContent'
    case 'comfyui-workflow': return 'ComfyUI Workflow API'
    default: return protocol
  }
}
