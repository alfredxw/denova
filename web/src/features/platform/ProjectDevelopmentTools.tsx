import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { MoreHorizontal } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { APIError } from '@/lib/api-client/client'
import { BuildTerminal } from './BuildTerminal'
import { GamePlayer } from './GamePlayer'
import { DevelopmentPreviewDialog } from './DevelopmentPreviewDialog'
import { SourceManifestEditor } from './SourceManifestEditor'
import { useDevelopmentContext, useProjectDevelopment } from './development-context'
import { openInstalledExtension } from './extension-navigation'
import { management, platformError, type Candidate, type CatalogEntry, type DevelopmentSource, type Instance, type Release, type RuntimeSnapshot } from './api'

interface ProjectDevelopmentToolsProps {
  projectId: string
  visible: boolean
  refreshSignal: number
  /** Commit open Project editors before reading source for build, preview or installation. */
  beforeAction: () => Promise<boolean>
  onOpenFile: (path: string) => void
}

/** The workbench owns the only development workflow, using ordinary Project editors and sessions. */
export function ProjectDevelopmentTools(props: ProjectDevelopmentToolsProps) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const { sources, source } = useProjectDevelopment(props.projectId, props.visible)
  useEffect(() => {
    void client.invalidateQueries({ queryKey: ['platform', 'development'] })
  }, [client, props.refreshSignal])
  if (!source) return null
  return <div className="flex flex-wrap items-center gap-2 border-b bg-background px-3 py-2" aria-label={t('platform.developmentTools')}>
    {sources.length > 1 ? <Select value={source.developmentId} onValueChange={id => useDevelopmentContext.getState().select(props.projectId, id)}>
      <SelectTrigger aria-label={t('platform.developmentSource')} className="max-w-full sm:max-w-64"><SelectValue /></SelectTrigger>
      <SelectContent><SelectGroup>{sources.map(item => <SelectItem key={item.developmentId} value={item.developmentId}>{item.relativePath}</SelectItem>)}</SelectGroup></SelectContent>
    </Select> : <span className="mr-auto text-xs text-muted-foreground">{t('platform.developmentTools')} · {t('platform.type.' + source.kind)}</span>}
    <SourceDevelopmentActions key={source.developmentId} {...props} source={source} />
  </div>
}

// A selected source owns its checked bytes and temporary runtimes. Switching sources
// disposes them; a different Project or source can never consume stale check results.
function SourceDevelopmentActions({ source, visible, refreshSignal, beforeAction, onOpenFile }: ProjectDevelopmentToolsProps & { source: DevelopmentSource }) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  const client = useQueryClient()
  const [candidate, setCandidate] = useState<Candidate | null>(null)
  const [player, setPlayer] = useState<RuntimeSnapshot | null>(null)
  const [manifestAction, setManifestAction] = useState<'edit' | 'publish' | null>(null)
  const [testsOpen, setTestsOpen] = useState(false)
  const [build, setBuild] = useState<{ directory: string; command: { command: string; args: string[] } } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [diagnostic, setDiagnostic] = useState('')
  const buildOutput = useRef('')
  const flushManifest = useRef<EditorFlushHandler | null>(null)
  const setManifestFlush = useCallback((flush: EditorFlushHandler | null) => { flushManifest.current = flush }, [])
  const previews = useQuery({ queryKey: ['platform', 'instances'], queryFn: () => management<Instance[]>('/instances'), enabled: visible && source.kind === 'game' })
  const catalog = useQuery({ queryKey: ['platform', 'catalog'], queryFn: () => management<CatalogEntry[]>('/catalog'), enabled: visible })
  const installed = catalog.data?.find(item => !item.removed && item.kind === source.kind && item.id === source.manifest?.id)
  const tests = (previews.data ?? []).filter(instance => instance.preview && (instance.projectId === source.projectId || !!instance.storyId) && instance.gameId === source.manifest?.id)
  const refresh = useCallback(() => { void client.invalidateQueries({ queryKey: ['platform'] }) }, [client])
  const recordFeedback = useCallback((feedback: string) => {
    useDevelopmentContext.getState().recordFeedback(source.developmentId, feedback)
  }, [source.developmentId])
  const receiveBuildOutput = useCallback((chunk: string) => {
    buildOutput.current = (buildOutput.current + chunk).slice(-16384)
    recordFeedback('Latest build terminal output (last 16,384 characters):\n' + buildOutput.current)
  }, [recordFeedback])
  useEffect(() => () => {
    if (player) void management(`/runtimes/${player.id}/stop`, 'POST', {}).catch(error => console.error('[development] stop preview failed', error))
  }, [player?.id])
  useEffect(() => () => {
    if (candidate) void management(`/candidates/${candidate.candidateId}`, 'DELETE').catch(error => console.error('[development] discard candidate failed', error))
  }, [candidate?.candidateId])
  const saveSource = async () => {
    if (flushManifest.current && !await flushManifest.current()) return false
    return beforeAction()
  }
  const run = async (operation: string, action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    setDiagnostic('')
    try {
      await action()
    } catch (cause) {
      const detail = cause instanceof APIError && typeof cause.payload.diagnostic === 'string' ? cause.payload.diagnostic : String(cause)
      console.error('[development] operation failed', { operation, projectId: source.projectId, source: source.relativePath, cause })
      setError(platformError(cause))
      setDiagnostic(detail)
      recordFeedback(`${operation} failed:\n${detail}`)
    } finally { setBusy(false) }
  }
  return <>
    <Button size="sm" variant="outline" disabled={busy || !!player || !!build} onClick={() => void run('Prepare preview', async () => {
      if (!await saveSource()) return
      const checked = await management<Candidate>(`/development/${source.developmentId}/check`)
      setCandidate(checked)
      recordFeedback(`Package validation passed: ${checked.manifest.id} ${checked.manifest.version}; ${checked.files.length} files. Source edits require another check before preview or installation.`)
    })}>{t('platform.preview')}</Button>
    <Button size="sm" disabled={busy || !!build} onClick={() => void run('Open publication', async () => { if (await saveSource()) setManifestAction('publish') })}>{t('platform.publishLocal')}</Button>
    {tests.length > 0 && <Button size="sm" variant="ghost" onClick={() => setTestsOpen(true)}>{t('platform.testSaves', { count: tests.length })}</Button>}
    <DropdownMenu>
      <DropdownMenuTrigger asChild><Button size="icon-sm" variant="ghost" disabled={busy} aria-label={t('platform.developmentMore')}><MoreHorizontal /></Button></DropdownMenuTrigger>
      <DropdownMenuContent align="end"><DropdownMenuGroup>
        <DropdownMenuItem onSelect={() => void run('Open source manifest', async () => { if (await saveSource()) setManifestAction('edit') })}>{t('platform.editManifest')}</DropdownMenuItem>
        <DropdownMenuItem disabled={!!player || !!build} onSelect={() => void run('Build', async () => {
          if (!await saveSource()) return
          const recipe = await management<{ command: { command: string; args: string[] } | null; directory: string }>(`/development/${source.developmentId}/build`)
          if (recipe.command) { buildOutput.current = ''; setBuild({ ...recipe, command: recipe.command }) }
          else { setDiagnostic(t('platform.noBuild')); recordFeedback('This source declares no build command. Proceed to preview or publication.') }
        })}>{t('platform.build')}</DropdownMenuItem>
        {installed && <DropdownMenuItem onSelect={() => openInstalledExtension(installed.kind, installed.id)}>{t('platform.viewInstalled')}</DropdownMenuItem>}
      </DropdownMenuGroup></DropdownMenuContent>
    </DropdownMenu>
    {(source.messageKey || error) && <div className="w-full"><InlineErrorNotice message={error || t(source.messageKey!)} /></div>}
    {diagnostic && <pre className="max-h-36 w-full overflow-auto whitespace-pre-wrap break-words text-xs" role="status">{diagnostic}</pre>}
    <Dialog open={visible && !!manifestAction} onOpenChange={open => { if (!open && !busy) void run('Save source manifest', async () => { if (await saveSource()) setManifestAction(null) }) }}>
      <DialogContent className="flex max-h-[85dvh] flex-col overflow-hidden max-md:max-h-[85dvh] max-md:overflow-hidden sm:max-w-2xl">
        <DialogHeader className="shrink-0 pr-6"><DialogTitle>{t(manifestAction === 'publish' ? 'platform.publishLocal' : 'platform.editManifest')}</DialogTitle><DialogDescription>{t(manifestAction === 'publish' ? 'platform.publishDescription' : 'platform.configurationHelp')}</DialogDescription></DialogHeader>
        <div className="-mx-1 min-h-0 space-y-4 overflow-y-auto px-1">
          <fieldset disabled={busy} className="min-w-0">
            <SourceManifestEditor source={source} refreshSignal={refreshSignal} onSaved={refresh} onFlushChange={setManifestFlush} onOpenFile={path => {
              void run('Open source file', async () => { if (await saveSource()) { setManifestAction(null); onOpenFile(path) } })
            }} />
          </fieldset>
          {error && <InlineErrorNotice message={error} />}
          {diagnostic && <pre className="max-h-36 overflow-auto whitespace-pre-wrap break-words text-xs">{diagnostic}</pre>}
        </div>
        {manifestAction === 'publish' && <DialogFooter className="shrink-0">
          <Button variant="outline" disabled={busy} onClick={() => void run('Save source manifest', async () => { if (await saveSource()) setManifestAction(null) })}>{t('common.cancel')}</Button>
          <Button disabled={busy} onClick={() => void run('Publish locally', async () => {
            if (!await saveSource()) return
            const checked = await management<Candidate>(`/development/${source.developmentId}/check`)
            try {
              // Authors confirm the declared required capabilities with publication.
              // Existing optional grants survive updates only while still declared.
              const optional = checked.manifest.permissions.optional ?? []
              const grants = [...new Set([...(checked.manifest.permissions.required ?? []), ...(installed?.grants ?? []).filter(permission => optional.includes(permission))])]
              const release = await management<Release>('/packages/install', 'POST', { candidateId: checked.candidateId, grants })
              recordFeedback(`Published locally: ${release.manifest.id} ${release.manifest.version}; ${checked.files.length} files.`)
              setManifestAction(null)
              refresh()
              toast.success(t('platform.publishedLocal'))
              openInstalledExtension(release.ref.package.kind, release.ref.package.id)
            } finally {
              await management(`/candidates/${checked.candidateId}`, 'DELETE').catch(error => console.error('[development] discard publication candidate failed', error))
            }
          })}>{t('platform.confirmPublish')}</Button>
        </DialogFooter>}
      </DialogContent>
    </Dialog>
    {candidate && <DevelopmentPreviewDialog key={candidate.candidateId} projectId={source.projectId} open={visible} candidate={candidate} onClose={() => setCandidate(null)}
      onFeedback={recordFeedback} onPlay={runtime => { setPlayer(runtime); refresh() }} />}
    {build && visible && <BuildTerminal {...build} projectId={source.projectId} onOutput={receiveBuildOutput} onClose={() => setBuild(null)} />}
    <Dialog open={visible && !!player} onOpenChange={open => { if (!open) void run('Stop preview', async () => { if (player) await management(`/runtimes/${player.id}/stop`, 'POST', {}); setPlayer(null) }) }}>
      <DialogContent className="flex h-[85dvh] max-w-[calc(100vw-2rem)] flex-col sm:max-w-5xl">
        <DialogHeader><DialogTitle>{t('platform.preview')}</DialogTitle><DialogDescription>{t('platform.previewDescription')}</DialogDescription></DialogHeader>
        {player && <GamePlayer key={player.id} runtime={player} visible={visible} onExit={() => setPlayer(null)} onOpenInstance={async instance => {
          const next = await management<RuntimeSnapshot>(`/instances/${instance.instanceId}/open`, 'POST', { locale: i18n.language, theme: resolvedTheme })
          setPlayer(next)
          refresh()
        }} />}
      </DialogContent>
    </Dialog>
    <Dialog open={visible && testsOpen} onOpenChange={setTestsOpen}>
      <DialogContent className="max-h-[85dvh] overflow-y-auto">
        <DialogHeader><DialogTitle>{t('platform.testSaves', { count: tests.length })}</DialogTitle><DialogDescription>{t('platform.previewDescription')}</DialogDescription></DialogHeader>
        {tests.map(instance => <div key={instance.instanceId} className="flex flex-wrap items-center gap-2 rounded-md border p-3">
          <span className="min-w-0 flex-1 break-words text-sm">{instance.title}</span>
          <Button variant="outline" disabled={busy} onClick={() => void run('Resume preview', async () => {
            setPlayer(await management(`/instances/${instance.instanceId}/open`, 'POST', { locale: i18n.language, theme: resolvedTheme })); setTestsOpen(false)
          })}>{t('platform.continue')}</Button>
          <Button variant="ghost" disabled={busy} onClick={() => void run('Remove preview save', async () => {
            await management(`/instances/${instance.instanceId}`, 'DELETE'); refresh()
          })}>{t('platform.resetPreview')}</Button>
        </div>)}
      </DialogContent>
    </Dialog>
  </>
}
