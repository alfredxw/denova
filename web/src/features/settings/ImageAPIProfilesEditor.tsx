import { useId, useMemo, useRef, useState, type ChangeEvent, type ReactNode } from 'react'
import { FileJson, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { ApiKeyInput } from './ApiKeyInput'
import { ImageProfilePingButton } from './ImageProfilePingButton'
import {
  DEFAULT_IMAGE_API_PROFILE_ID,
  IMAGE_API_PROTOCOLS,
  imageAPIProfileID,
  imageAPIProfileLabel,
  imageAPIProvider,
  imageAPIProviderDefaults,
  newImageAPIProfile,
  type ImageAPIProvider,
} from './image-profiles'
import type { ImageAPIProfileSettings } from './types'

const INHERIT_VALUE = '__inherit__'
const PROVIDER_DEFAULT_VALUE = '__provider_default__'
const MAX_WORKFLOW_BYTES = 5 * 1024 * 1024
const OPENAI_QUALITY_OPTIONS = ['auto', 'high', 'medium', 'low', 'standard', 'hd']
const XAI_QUALITY_OPTIONS = ['medium', 'low']
const FORMAT_OPTIONS = ['png', 'jpeg', 'webp']
const XAI_ASPECT_RATIO_OPTIONS = ['1:1', '16:9', '9:16', '4:3', '3:4', '3:2', '2:3', '2:1', '1:2', '19.5:9', '9:19.5', '20:9', '9:20']
const GEMINI_ASPECT_RATIO_OPTIONS = ['1:1', '1:4', '1:8', '2:3', '3:2', '3:4', '4:1', '4:3', '4:5', '5:4', '8:1', '9:16', '16:9', '21:9']
const XAI_RESOLUTION_OPTIONS = ['1k', '2k']
const GEMINI_RESOLUTION_OPTIONS = ['512', '1K', '2K', '4K']
const ARK_RESOLUTION_OPTIONS = ['1K', '2K', '4K']
const IMAGE_PROVIDERS: ImageAPIProvider[] = ['openai', 'xai', 'comfyui', 'volcengine', 'google', 'custom']

export function ImageAPIProfilesEditor({ profiles, effectiveProfiles, defaultProfileID, effectiveDefaultProfileID, onDefaultProfileChange, onChange }: {
  profiles: ImageAPIProfileSettings[]
  effectiveProfiles: ImageAPIProfileSettings[]
  defaultProfileID: string
  effectiveDefaultProfileID: string
  onDefaultProfileChange: (profileID: string) => void
  onChange: (profiles: ImageAPIProfileSettings[]) => void
}) {
  const { t } = useTranslation()
  const uploadID = useId()
  const profileKeysRef = useRef<string[]>([])
  const [workflowErrors, setWorkflowErrors] = useState<Record<string, string>>({})
  const profileKeys = useMemo(() => {
    if (profileKeysRef.current.length > profiles.length) profileKeysRef.current.length = profiles.length
    while (profileKeysRef.current.length < profiles.length) {
      profileKeysRef.current.push(`image-profile-${Date.now()}-${profileKeysRef.current.length}`)
    }
    return profileKeysRef.current
  }, [profiles.length])
  const profileOptions = imageProfileOptions(profiles, effectiveProfiles)
  const effectiveDefaultLabel = profileOptions.find((profile) => profile.id === effectiveDefaultProfileID)?.label
    || effectiveDefaultProfileID
    || DEFAULT_IMAGE_API_PROFILE_ID

  const updateProfile = (index: number, patch: Partial<ImageAPIProfileSettings>) => {
    onChange(profiles.map((profile, current) => current === index ? { ...profile, ...patch } : profile))
  }
  const replaceProvider = (index: number, provider: ImageAPIProvider) => {
    const current = profiles[index]
    const previousID = imageAPIProfileID(current)
    const defaults = newImageAPIProfile(provider)
    const explicitID = current.id?.trim() ?? ''
    const previousModel = current.model?.trim() ?? ''
    const derivedID = !explicitID || explicitID === previousModel
    const nextID = derivedID
      ? uniqueImageProfileID(defaults.model || (provider === 'custom' ? 'custom-image' : provider), profiles, index)
      : current.id
    onChange(profiles.map((profile, currentIndex) => currentIndex === index
      ? { id: nextID, name: current.name, ...defaults }
      : profile))
    if (previousID && defaultProfileID === previousID && nextID !== previousID) onDefaultProfileChange(nextID || '')
  }
  const updateModel = (index: number, model: string) => {
    const current = profiles[index]
    const previousID = imageAPIProfileID(current)
    const previousModel = current.model?.trim() ?? ''
    const syncID = !previousID || previousID === previousModel
    const nextID = syncID ? model : current.id
    updateProfile(index, { id: nextID, model })
    if (syncID && previousID && defaultProfileID === previousID && nextID !== previousID) onDefaultProfileChange(nextID || '')
  }
  const removeProfile = (index: number) => {
    const removedID = imageAPIProfileID(profiles[index])
    onChange(profiles.filter((_, current) => current !== index))
    if (removedID && defaultProfileID === removedID) onDefaultProfileChange('')
  }
  const uploadWorkflow = async (index: number, profileKey: string, event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    if (file.size > MAX_WORKFLOW_BYTES) {
      setWorkflowErrors((current) => ({ ...current, [profileKey]: t('settings.imageApi.workflowTooLarge') }))
      return
    }
    try {
      const workflow = await file.text()
      const parsed: unknown = JSON.parse(workflow)
      if (!isComfyUIAPIWorkflow(parsed)) throw new Error('invalid-api-format')
      updateProfile(index, {
        id: imageAPIProfileID(profiles[index]) || uniqueImageProfileID('comfyui', profiles, index),
        comfyui: { workflow_mode: 'api', workflow, workflow_name: file.name },
      })
      setWorkflowErrors((current) => ({ ...current, [profileKey]: '' }))
    } catch (error) {
      console.warn(`[settings] failed to read ComfyUI API workflow file=${file.name}`, error)
      setWorkflowErrors((current) => ({ ...current, [profileKey]: t('settings.imageApi.workflowInvalid') }))
    }
  }

  return (
    <div className="nova-settings-row rounded-md px-2 py-1.5">
      <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('settings.imageApi.profiles')}</div>
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
        {profiles.length === 0 && (
          <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2 text-[var(--nova-text-faint)]">
            {t('settings.imageApi.profileEmpty', { count: effectiveProfiles.length || 1 })}
          </div>
        )}
        {profiles.map((profile, index) => {
          const provider = imageAPIProvider(profile.provider)
          const defaults = imageAPIProviderDefaults(provider)
          const protocol = profile.protocol || defaults.protocol || 'openai-images'
          const usesComfyUI = protocol === 'comfyui-workflow'
          const workflowMode = profile.comfyui?.workflow_mode === 'api' ? 'api' : 'builtin'
          const profileKey = profileKeys[index]
          const inputID = `${uploadID}-${index}`
          return (
            <div key={profileKey} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
              <div className="flex items-center gap-2 px-2.5 py-2">
                <Badge variant="outline" className="shrink-0">{t('settings.imageApi.profileName', { index: index + 1 })}</Badge>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-xs font-medium text-[var(--nova-text)]">
                    {imageAPIProfileLabel(profile) || t('settings.imageApi.profileUntitled')}
                  </div>
                  <div className="truncate text-[11px] text-[var(--nova-text-faint)]">
                    {profile.model?.trim() || profile.comfyui?.workflow_name?.trim() || providerLabel(provider, t)}
                  </div>
                </div>
                <Button type="button" variant="outline" size="icon-sm" onClick={() => removeProfile(index)} aria-label={t('settings.imageApi.deleteProfile')}>
                  <Trash2 data-icon="inline-start" />
                </Button>
              </div>
              <Separator />
              <div className="grid gap-2 p-2.5 md:grid-cols-12">
                <ProfileField label={t('settings.imageApi.provider')} className="md:col-span-3">
                  <Select value={provider} onValueChange={(value) => replaceProvider(index, value as ImageAPIProvider)}>
                    <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      <SelectGroup>{IMAGE_PROVIDERS.map((value) => <SelectItem key={value} value={value}>{providerLabel(value, t)}</SelectItem>)}</SelectGroup>
                    </SelectContent>
                  </Select>
                </ProfileField>
                {provider === 'custom' && (
                  <ProfileField label={t('settings.imageApi.protocol')} className="md:col-span-3">
                    <Select value={protocol} onValueChange={(value) => updateProfile(index, {
                      protocol: value,
                      model: '',
                      default_size: '',
                      default_aspect_ratio: '',
                      default_resolution: '',
                      default_quality: '',
                      default_output_format: '',
                      comfyui: value === 'comfyui-workflow' ? { workflow_mode: 'builtin' } : undefined,
                    })}>
                      <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
                      <SelectContent className="nova-panel border text-[var(--nova-text)]">
                        <SelectGroup>{IMAGE_API_PROTOCOLS.map((value) => <SelectItem key={value} value={value}>{protocolLabel(value, t)}</SelectItem>)}</SelectGroup>
                      </SelectContent>
                    </Select>
                  </ProfileField>
                )}
                <ProfileField label={t('common.baseUrl')} className={provider === 'custom' ? 'md:col-span-6' : 'md:col-span-6'}>
                  <Input value={profile.base_url ?? ''} placeholder={defaults.base_url || t('settings.imageApi.baseUrlPlaceholder')} onChange={(event) => updateProfile(index, { base_url: event.target.value })} />
                </ProfileField>
                {!usesComfyUI && (
                  <ProfileField label={t('settings.imageApi.profileModelLabel')} className="md:col-span-3">
                    <Input value={profile.model ?? ''} placeholder={defaults.model || t('settings.imageApi.profileModelPlaceholder')} onChange={(event) => updateModel(index, event.target.value)} />
                  </ProfileField>
                )}
                <ProfileField label={t('settings.imageApi.profileAliasLabel')} className="md:col-span-3">
                  <Input value={profile.name ?? ''} placeholder={t('settings.imageApi.profileAliasPlaceholder')} onChange={(event) => updateProfile(index, { name: event.target.value })} />
                </ProfileField>
                {provider !== 'comfyui' && (
                  <ProfileField label={t('settings.imageApi.profileKeyLabel')} className="md:col-span-5">
                    <ApiKeyInput label={t('settings.imageApi.profileKeyLabel')} value={profile.api_key ?? ''} placeholder={t('settings.imageApi.profileKeyInheritPlaceholder')} onChange={(apiKey) => updateProfile(index, { api_key: apiKey })} />
                  </ProfileField>
                )}
                {usesComfyUI && (
                  <>
                    <ProfileField label={t('settings.imageApi.workflowMode')} className="md:col-span-4">
                      <Select value={workflowMode} onValueChange={(value) => updateProfile(index, {
                        comfyui: value === 'api'
                          ? { workflow_mode: 'api', workflow: profile.comfyui?.workflow, workflow_name: profile.comfyui?.workflow_name }
                          : { workflow_mode: 'builtin' },
                      })}>
                        <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
                        <SelectContent className="nova-panel border text-[var(--nova-text)]">
                          <SelectGroup>
                            <SelectItem value="builtin">{t('settings.imageApi.workflowBuiltin')}</SelectItem>
                            <SelectItem value="api">{t('settings.imageApi.workflowUpload')}</SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </ProfileField>
                    {workflowMode === 'builtin' ? (
                      <ProfileField label={t('settings.imageApi.checkpoint')} className="md:col-span-5">
                        <Input value={profile.model ?? ''} placeholder={t('settings.imageApi.checkpointPlaceholder')} onChange={(event) => updateModel(index, event.target.value)} />
                      </ProfileField>
                    ) : (
                      <div className="flex min-w-0 flex-col gap-1 md:col-span-5">
                        <span className="text-[11px] leading-none text-[var(--nova-text-faint)]">{t('settings.imageApi.workflowFile')}</span>
                        <div className="flex min-w-0 items-center gap-2">
                          <input id={inputID} type="file" accept=".json,application/json" className="sr-only" onChange={(event) => void uploadWorkflow(index, profileKey, event)} />
                          <Button variant="outline" size="sm" asChild>
                            <label htmlFor={inputID}><FileJson data-icon="inline-start" />{profile.comfyui?.workflow ? t('settings.imageApi.workflowReplace') : t('settings.imageApi.workflowChoose')}</label>
                          </Button>
                          <span className="min-w-0 truncate text-xs text-[var(--nova-text-faint)]">{profile.comfyui?.workflow_name || t('settings.imageApi.workflowMissing')}</span>
                        </div>
                        {workflowErrors[profileKey] && <span role="alert" className="text-[11px] text-red-600 dark:text-red-400">{workflowErrors[profileKey]}</span>}
                      </div>
                    )}
                    <div className="self-end text-[11px] leading-5 text-[var(--nova-text-faint)] md:col-span-3">
                      {workflowMode === 'builtin' ? t('settings.imageApi.workflowBuiltinHint') : t('settings.imageApi.workflowUploadHint')}
                    </div>
                  </>
                )}
                <ProtocolOptions protocol={protocol} profile={profile} defaults={defaults} onUpdate={(patch) => updateProfile(index, patch)} />
              </div>
              <div className="border-t border-[var(--nova-border)] px-2.5 py-2"><ImageProfilePingButton profile={profile} /></div>
            </div>
          )
        })}
        <Button type="button" onClick={() => {
          const profile = newImageAPIProfile()
          profile.id = uniqueImageProfileID(profile.model || 'openai', profiles, -1)
          onChange([...profiles, profile])
        }} variant="outline" size="sm">
          <Plus data-icon="inline-start" />{t('settings.imageApi.addProfile')}
        </Button>
      </div>
    </div>
  )
}

function ProtocolOptions({ protocol, profile, defaults, onUpdate }: {
  protocol: string
  profile: ImageAPIProfileSettings
  defaults: ImageAPIProfileSettings
  onUpdate: (patch: Partial<ImageAPIProfileSettings>) => void
}) {
  const { t } = useTranslation()
  const aspectRatioOptions = protocol === 'xai-images'
    ? XAI_ASPECT_RATIO_OPTIONS
    : protocol === 'gemini-images' ? GEMINI_ASPECT_RATIO_OPTIONS : undefined
  const resolutionOptions = protocol === 'xai-images'
    ? XAI_RESOLUTION_OPTIONS
    : protocol === 'gemini-images' ? GEMINI_RESOLUTION_OPTIONS : protocol === 'ark-images' ? ARK_RESOLUTION_OPTIONS : undefined
  const qualityOptions = protocol === 'xai-images'
    ? XAI_QUALITY_OPTIONS
    : protocol === 'openai-images' ? OPENAI_QUALITY_OPTIONS : undefined
  return (
    <>
      {(protocol === 'openai-images' || protocol === 'comfyui-workflow') && (
        <ProfileField label={t('settings.imageApi.defaultSize')} className="md:col-span-3">
          <Input value={profile.default_size ?? ''} placeholder={defaults.default_size || t('settings.imageApi.providerDefault')} onChange={(event) => onUpdate({ default_size: event.target.value })} />
        </ProfileField>
      )}
      {aspectRatioOptions && (
        <OptionSelect label={t('settings.imageApi.defaultAspectRatio')} value={profile.default_aspect_ratio ?? ''} options={aspectRatioOptions} onChange={(value) => onUpdate({ default_aspect_ratio: value })} />
      )}
      {resolutionOptions && (
        <OptionSelect label={t('settings.imageApi.defaultResolution')} value={profile.default_resolution ?? ''} options={resolutionOptions} onChange={(value) => onUpdate({ default_resolution: value })} />
      )}
      {qualityOptions && (
        <OptionSelect label={t('settings.imageApi.defaultQuality')} value={profile.default_quality ?? ''} options={qualityOptions} onChange={(value) => onUpdate({ default_quality: value })} />
      )}
      {(protocol === 'openai-images' || protocol === 'ark-images') && (
        <OptionSelect label={t('settings.imageApi.defaultOutputFormat')} value={profile.default_output_format ?? ''} options={FORMAT_OPTIONS} onChange={(value) => onUpdate({ default_output_format: value })} />
      )}
    </>
  )
}

function OptionSelect({ label, value, options, onChange }: { label: string; value: string; options: string[]; onChange: (value: string) => void }) {
  const { t } = useTranslation()
  return (
    <ProfileField label={label} className="md:col-span-3">
      <Select value={value || PROVIDER_DEFAULT_VALUE} onValueChange={(next) => onChange(next === PROVIDER_DEFAULT_VALUE ? '' : next)}>
        <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={PROVIDER_DEFAULT_VALUE}>{t('settings.imageApi.providerDefault')}</SelectItem>
            {options.map((option) => <SelectItem key={option} value={option}>{option}</SelectItem>)}
          </SelectGroup>
        </SelectContent>
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

function uniqueImageProfileID(base: string, profiles: ImageAPIProfileSettings[], currentIndex: number): string {
  const normalized = base.trim() || 'image-model'
  const used = new Set(profiles.flatMap((profile, index) => index === currentIndex ? [] : [imageAPIProfileID(profile)]).filter(Boolean))
  if (!used.has(normalized)) return normalized
  for (let suffix = 2; ; suffix += 1) {
    const candidate = `${normalized}-${suffix}`
    if (!used.has(candidate)) return candidate
  }
}

function isComfyUIAPIWorkflow(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const nodes = Object.values(value)
  return nodes.length > 0 && nodes.every((node) => {
    if (!node || typeof node !== 'object' || Array.isArray(node)) return false
    const record = node as Record<string, unknown>
    return typeof record.class_type === 'string' && Boolean(record.inputs) && typeof record.inputs === 'object' && !Array.isArray(record.inputs)
  })
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
