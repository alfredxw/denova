import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel, FieldGroup } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  management,
  managementBase,
  localized,
  platformError,
  type Candidate,
  type Release,
  type Instance,
  type RuntimeSnapshot,
} from './api'
import { RuntimeSetup, emptySetup } from './RuntimeSetup'
import { ToolConsole } from './ToolConsole'
import { openInstalledExtension } from './extension-navigation'
import { PackageSourcePicker } from './PackageSourcePicker'

export function InstallDialog({
  projectId,
  open,
  candidate,
  onCandidate,
  onOpenChange,
  onInstalled,
  onPlay,
  onFeedback,
  updateGrants,
}: {
  projectId?: string
  open: boolean
  candidate: Candidate | null
  onCandidate: (candidate: Candidate | null) => void
  onOpenChange: (open: boolean) => void
  onInstalled: () => void
  onPlay?: (runtime: RuntimeSnapshot) => void
  onFeedback?: (feedback: string) => void
  updateGrants?: string[]
}) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [setup, setSetup] = useState({ ...emptySetup, projectId: projectId ?? '' })
  const [runtime, setRuntime] = useState<RuntimeSnapshot | null>(null)
  const [grants, setGrants] = useState<string[]>(() => {
    const permissions = [...(candidate?.manifest.permissions.required ?? []), ...(candidate?.manifest.permissions.optional ?? [])]
    return (updateGrants ?? []).filter(permission => permissions.includes(permission))
  })
  const [busy, setBusy] = useState(false)
  useEffect(() => () => {
    if (runtime) void management(`/runtimes/${runtime.id}/stop`, 'POST', {}).catch(error => console.error('[extensions] stop preview failed', error))
  }, [runtime?.id])
  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    try {
      await action()
    } catch (error) {
      console.error('[extensions] package operation failed', error)
      onFeedback?.(`Package operation failed: ${error instanceof Error ? error.message : String(error)}`)
      toast.error(platformError(error))
    } finally {
      setBusy(false)
    }
  }
  const close = async () => {
    if (runtime) await management(`/runtimes/${runtime.id}/stop`, 'POST', {})
    if (candidate)
      await management(`/candidates/${candidate.candidateId}`, 'DELETE')
    setRuntime(null)
    onCandidate(null)
    onOpenChange(false)
  }
  const preview = async (load: () => Promise<Candidate>) => {
    const next = await load()
    if (candidate)
      await management(`/candidates/${candidate.candidateId}`, 'DELETE')
    onCandidate(next)
    setGrants([])
  }
  const requiredPermissions = candidate?.manifest.permissions.required ?? []
  const permissions = [...requiredPermissions, ...(candidate?.manifest.permissions.optional ?? [])]
  const missingPermissions = requiredPermissions.some(permission => !grants.includes(permission))
  return <Dialog open={open} onOpenChange={next => {
    if (next) onOpenChange(true)
    else if (!busy) void run(close)
  }}>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t(updateGrants ? 'platform.github.reviewUpdate' : 'platform.install')}</DialogTitle>
          <DialogDescription>
            {t(updateGrants ? 'platform.github.updateHelp' : 'platform.installDescription')}
          </DialogDescription>
        </DialogHeader>
        {!candidate ? (
          <PackageSourcePicker busy={busy} run={run} onPreview={preview} onImported={() => onOpenChange(false)} />
        ) : (
          <div className="flex flex-col gap-4">
            <p className="break-words font-medium">
              {localized(candidate.manifest.name, i18n.language)} ·{' '}
              {candidate.manifest.version} · {t('platform.type.' + candidate.kind)}
            </p>
            <p className="text-sm text-muted-foreground">
              {t(
                candidate.manifest.runtime?.backend
                  ? 'platform.nativeNotice'
                  : 'platform.staticNotice',
              )}
            </p>
            <p className="text-sm text-muted-foreground">
              {t('platform.packageFiles', { count: candidate.files.length })} ·{' '}
              {(candidate.bytes / 1024).toFixed(1)} KB
            </p>
            {candidate.source && <p className="break-all text-sm text-muted-foreground">
              <a href={candidate.source.url} target="_blank" rel="noreferrer" className="underline underline-offset-4">{candidate.source.url}</a>
              {' · '}{candidate.source.ref}{' · '}<span className="whitespace-nowrap">{candidate.source.commit?.slice(0, 7)}</span>
              {candidate.source.path !== '.' && <> · {candidate.source.path}</>}
            </p>}
            <details>
              <summary className="cursor-pointer text-sm">
                {t('platform.files')}
              </summary>
              <pre className="mt-2 max-h-36 overflow-auto text-xs">
                {candidate.files.join('\n')}
              </pre>
            </details>
            <FieldGroup>
              {permissions.map((permission) => (
                <Field key={permission} orientation="horizontal">
                  <Switch
                    id={`grant-${permission}`}
                    checked={grants.includes(permission)}
                    onCheckedChange={(checked) =>
                      setGrants((current) =>
                        checked
                          ? [...current, permission]
                          : current.filter((value) => value !== permission),
                      )
                    }
                  />
                  <FieldLabel htmlFor={`grant-${permission}`}>
                    {t(`platform.permission.${permission}`)}
                    {requiredPermissions.includes(permission)
                      ? ` (${t('platform.required')})`
                      : ''}
                  </FieldLabel>
                </Field>
              ))}
            </FieldGroup>
            {(candidate.manifest.requires ?? []).map((dependency) => (
              <p
                className="break-words text-sm text-muted-foreground"
                key={dependency.pluginId}
              >
                {t('platform.dependency')}: {dependency.pluginId}{' '}
                {dependency.versionRange}
              </p>
            ))}
            {onPlay && <details open>
              <summary className="cursor-pointer text-sm">
                {t('platform.preview')}
              </summary>
              <div className="mt-3 flex flex-col gap-3">
                <p className="text-sm text-muted-foreground">
                  {t('platform.previewDescription')}
                </p>
                <RuntimeSetup
                  projectLocked={!!projectId}
                  manifest={candidate.manifest}
                  configurationEndpoint={`/candidates/${candidate.candidateId}/${candidate.kind === 'game' ? 'setup' : 'settings'}`}
                  value={setup}
                  onChange={setSetup}
                />
                <Button
                  variant="outline"
                  disabled={
                    busy ||
                    ((candidate.kind === 'plugin' ||
                      requiredPermissions.includes('agents.run')) &&
                      !setup.projectId) ||
                    missingPermissions
                  }
                  onClick={() =>
                    void run(async () => {
                      const release = await management<Release>(
                        `/candidates/${candidate.candidateId}/prepare-preview`,
                        'POST',
                        { grants },
                      )
                      const options = {
                        locale: i18n.language,
                        theme: resolvedTheme,
                      }
                      if (candidate.kind === 'game') {
                        const instance = await management<Instance>(
                          '/instances',
                          'POST',
                          {
                            gameId: release.manifest.id,
                            releaseId: release.ref.releaseId,
                            title: localized(
                              release.manifest.name,
                              i18n.language,
                            ),
                            projectId: setup.projectId,
                            setup: setup.configuration,
                            models: setup.models,
                            preview: true,
                          },
                        )
                        onPlay(
                          await management(
                            `/instances/${instance.instanceId}/open`,
                            'POST',
                            options,
                          ),
                        )
                        await close()
                      } else
                        setRuntime(
                          await management('/runtimes/plugin', 'POST', {
                            pluginId: release.manifest.id,
                            releaseId: release.ref.releaseId,
                            scope: {
                              kind: 'project',
                              projectId: setup.projectId,
                            },
                            settings: setup.configuration,
                            models: setup.models,
                            preview: true,
                            ...options,
                          }),
                        )
                    })
                  }
                >
                  {t('platform.startPreview')}
                </Button>
              </div>
            </details>}
            {runtime && (
              <ToolConsole
                runtime={runtime}
                manifest={candidate.manifest}
                onStop={() => setRuntime(null)}
                onFeedback={onFeedback}
              />
            )}
          </div>
        )}
        <DialogFooter className="flex-wrap gap-2">
          {candidate && (
            <Button variant="outline" asChild>
              <a
                href={`${managementBase}/candidates/${candidate.candidateId}/archive`}
              >
                {t('platform.exportPackage')}
              </a>
            </Button>
          )}
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => void run(close)}
          >
            {t('platform.close')}
          </Button>
          {candidate && (
            <Button
              disabled={busy || missingPermissions}
              onClick={() =>
                void run(async () => {
                  const installed = await management<Release>('/packages/install', 'POST', {
                    candidateId: candidate.candidateId,
                    grants,
                  })
                  await close()
                  onInstalled()
                  toast.success(t('platform.installedSuccess'), { action: { label: t('platform.viewInstalled'), onClick: () => openInstalledExtension(installed.ref.package.kind, installed.ref.package.id) } })
                })
              }
            >
              {t(updateGrants ? 'platform.github.update' : 'platform.install')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
}
