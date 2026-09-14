import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { getCurrentWorkspace } from '@/lib/api-client/workspace'
import { RuntimeSetup, emptySetup } from './RuntimeSetup'
import { ToolConsole } from './ToolConsole'
import { management, localized, platformError, type Candidate, type Instance, type Release, type RuntimeSnapshot } from './api'

/** Model and test configuration belong only to this disposable development run. */
export function DevelopmentPreviewDialog({ candidate, projectId, open, onClose, onPlay, onFeedback }: {
  candidate: Candidate
  projectId: string
  open: boolean
  onClose: () => void
  onPlay: (runtime: RuntimeSnapshot) => void
  onFeedback: (feedback: string) => void
}) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [setup, setSetup] = useState({ ...emptySetup, projectId })
  const [runtime, setRuntime] = useState<RuntimeSnapshot | null>(null)
  const [busy, setBusy] = useState(false)
  const [optionalGrants, setOptionalGrants] = useState<string[]>([])
  const storyPreview = candidate.manifest.game?.storage.kind === 'story'
  const workspace = useQuery({ queryKey: ['platform', 'preview-workspace'], queryFn: getCurrentWorkspace, enabled: open && storyPreview, staleTime: 0 })
  const previewProjectId = storyPreview ? workspace.data?.project_id ?? '' : projectId
  const permissions = [...(candidate.manifest.permissions.required ?? []), ...optionalGrants]
  useEffect(() => () => {
    if (runtime) void management(`/runtimes/${runtime.id}/stop`, 'POST', {}).catch(error => console.error('[development] stop tool preview failed', error))
  }, [runtime?.id])
  const start = async () => {
    setBusy(true)
    try {
      const release = await management<Release>(`/candidates/${candidate.candidateId}/prepare-preview`, 'POST', { grants: permissions })
      const options = { locale: i18n.language, theme: resolvedTheme }
      if (candidate.kind === 'game') {
        const instance = await management<Instance>('/instances', 'POST', {
          gameId: release.manifest.id, releaseId: release.ref.releaseId,
          title: localized(release.manifest.name, i18n.language), projectId: previewProjectId,
          setup: setup.configuration, models: setup.models, preview: true,
        })
        onPlay(await management(`/instances/${instance.instanceId}/open`, 'POST', options))
        onClose()
      } else {
        setRuntime(await management('/runtimes/plugin', 'POST', {
          pluginId: release.manifest.id, releaseId: release.ref.releaseId,
          scope: { kind: 'project', projectId }, settings: setup.configuration,
          models: setup.models, preview: true, ...options,
        }))
      }
    } catch (error) {
      console.error('[development] start preview failed', { projectId, packageId: candidate.manifest.id, error })
      onFeedback(`Preview failed: ${error instanceof Error ? error.message : String(error)}`)
      toast.error(platformError(error))
    } finally { setBusy(false) }
  }
  return <Dialog open={open} onOpenChange={next => { if (!next && !busy) onClose() }}>
    <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-xl">
      <DialogHeader><DialogTitle>{t('platform.preview')}</DialogTitle><DialogDescription>{t('platform.previewDescription')}</DialogDescription></DialogHeader>
      {runtime ? <ToolConsole runtime={runtime} manifest={candidate.manifest} onStop={() => setRuntime(null)} onFeedback={onFeedback} /> : <fieldset disabled={busy} className="flex min-w-0 flex-col gap-3">
        {storyPreview && <p className="text-sm text-muted-foreground">{t(previewProjectId ? 'platform.storyPreviewProject' : 'platform.storyPreviewProjectRequired')}</p>}
        <RuntimeSetup projectLocked manifest={candidate.manifest}
          configurationEndpoint={`/candidates/${candidate.candidateId}/${candidate.kind === 'game' ? 'setup' : 'settings'}`}
          value={{ ...setup, projectId: previewProjectId }} onChange={setSetup} />
        {Boolean(candidate.manifest.permissions.optional?.length) && <details>
          <summary className="cursor-pointer text-sm">{t('platform.optionalCapabilities')}</summary>
          <FieldGroup className="mt-3">{candidate.manifest.permissions.optional.map(permission => <Field key={permission} orientation="horizontal">
            <Switch id={`preview-${permission}`} checked={optionalGrants.includes(permission)} onCheckedChange={checked => setOptionalGrants(current => checked ? [...current, permission] : current.filter(value => value !== permission))} />
            <FieldLabel htmlFor={`preview-${permission}`}>{t(`platform.permission.${permission}`)}</FieldLabel>
          </Field>)}</FieldGroup>
        </details>}
        {permissions.length > 0 && <p className="text-sm text-muted-foreground">{t('platform.previewPermissions', { permissions: permissions.map(permission => t(`platform.permission.${permission}`)).join(i18n.language.startsWith('zh') ? '、' : ', ') })}</p>}
      </fieldset>}
      <DialogFooter>
        <Button variant="outline" disabled={busy} onClick={onClose}>{t('platform.close')}</Button>
        {!runtime && <Button disabled={busy || ((candidate.kind === 'plugin' || permissions.includes('agents.run') || storyPreview) && !previewProjectId)} onClick={() => void start()}>{t('platform.startPreview')}</Button>}
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
