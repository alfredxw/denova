import { useId } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { ConfigurationForm } from './ConfigurationForm'
import { getAgentChatProjects } from '@/features/agent-chat/api'
import { fetchSettings } from '@/features/settings/api'
import { modelProfilesWithDefault } from '@/features/settings/model-profiles'
import { imageAPIProfilesWithDefault } from '@/features/settings/image-profiles'
import { management, platformError, type ConfigurationDocument, type Installed, type Manifest } from './api'

export interface Setup {
  projectId: string
  models: Record<string, string>
  configuration: Record<string, unknown>
}
export const emptySetup: Setup = {
  projectId: '',
  models: {},
  configuration: {},
}

/** Explicit Project and model assignments never contain provider credentials. */
export function RuntimeSetup({
  manifest,
  value,
  onChange,
  projectLocked = false,
  configurationEndpoint,
}: {
  manifest?: Manifest
  value: Setup
  onChange: (value: Setup) => void
  projectLocked?: boolean
  configurationEndpoint?: string
}) {
  const { t, i18n } = useTranslation()
  const inputId = useId()
  const form = useQuery({ queryKey: ['platform', 'setup', configurationEndpoint, i18n.language], queryFn: () => management<ConfigurationDocument>(configurationEndpoint! + (configurationEndpoint!.includes('?') ? '&' : '?') + 'locale=' + (i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US')), enabled: !!configurationEndpoint, staleTime: 'static', refetchOnWindowFocus: false })
  const projects = useQuery({
    queryKey: ['platform', 'projects'],
    queryFn: getAgentChatProjects,
    enabled: !projectLocked,
  })
  const settings = useQuery({
    queryKey: ['platform', 'settings'],
    queryFn: fetchSettings,
  })
  const plugins = useQuery({
    queryKey: ['platform', 'packages', 'plugin'],
    queryFn: () => management<Installed[]>('/packages/plugin'),
  })
  const providers = manifest ? [manifest] : []
  const visited = new Set(manifest && !manifest.game ? [manifest.id] : [])
  const visit = (id: string) => {
    if (visited.has(id)) return
    visited.add(id)
    const installed = plugins.data?.find((item) => item.id === id)
    const next = installed?.releases.find(
      (release) => release.ref.releaseId === installed.currentRelease,
    )?.manifest
    if (next) {
      providers.push(next)
      next.requires?.forEach((dependency) => visit(dependency.pluginId))
    }
  }
  manifest?.requires?.forEach((dependency) => visit(dependency.pluginId))
  const slots = providers.flatMap((provider) =>
    (provider.modelSlots ?? []).map((slot) => ({
      required: slot.required,
      kind: slot.kind,
      key:
        provider === manifest && manifest.game
          ? `local:${slot.id}`
          : `${provider.id}/${slot.id}`,
    })),
  )
  const profiles = modelProfilesWithDefault(settings.data?.effective).filter(
    (profile): profile is typeof profile & { id: string } => !!profile.id,
  )
  const imageProfiles = imageAPIProfilesWithDefault(settings.data?.effective).filter(
    (profile): profile is typeof profile & { id: string } => !!profile.id,
  )
  if (manifest?.game?.uses?.agents?.includes('builtin/assistant')) {
    slots.push({ key: 'builtin/assistant', required: true, kind: 'text' })
  }
  return (
    <FieldGroup>
      {!projectLocked && <Field>
        <FieldLabel htmlFor={inputId + '-project'}>{t('platform.project')}</FieldLabel>
        <Select
          value={value.projectId}
          onValueChange={(projectId) => onChange({ ...value, projectId })}
        >
          <SelectTrigger id={inputId + '-project'} className="w-full">
            <SelectValue placeholder={t('platform.selectProject')} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {(projects.data ?? [])
                .filter((project) => project.status === 'available')
                .map((project) => (
                  <SelectItem key={project.id} value={project.id}>
                    {project.name}
                  </SelectItem>
                ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>}
      {slots.map((slot, index) => (
        <Field key={slot.key}>
          <FieldLabel htmlFor={inputId + '-model-' + index}>
            {t(slot.kind === 'image' ? 'platform.imageModel' : 'platform.model')}{slots.length > 1 ? ` ${index + 1}` : ''}
            {slot.required ? ` (${t('platform.required')})` : ''}
          </FieldLabel>
          <Select
            value={value.models[slot.key] ?? ''}
            onValueChange={(profile) =>
              onChange({
                ...value,
                models: { ...value.models, [slot.key]: profile },
              })
            }
          >
            <SelectTrigger id={inputId + '-model-' + index} className="w-full">
              <SelectValue placeholder={t('platform.selectModel')} />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {(slot.kind === 'image' ? imageProfiles : profiles).map((profile) => (
                  <SelectItem key={profile.id} value={profile.id}>
                    {profile.name || profile.id}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      ))}
      {form.error && <InlineErrorNotice message={platformError(form.error)} />}
      {form.data?.form && <div className="flex min-w-0 flex-col gap-3">
        <p className="text-sm text-muted-foreground">{t(manifest?.game ? 'platform.settings.setupHelp' : 'platform.previewDescription')}</p>
        <ConfigurationForm definition={form.data.form} values={Object.keys(value.configuration).length ? value.configuration : form.data.values} disabled={false} onChange={configuration => onChange({ ...value, configuration })} />
      </div>}
    </FieldGroup>
  )
}
