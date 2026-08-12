import { useCallback, useEffect, useMemo, useState } from 'react'
import { Clock3, FileCode2, FilePlus2, History, Plus, RotateCcw, Save, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  getContinualLearningSchedule,
  getHarnessState,
  getHarnessStateVersionDiff,
  getHarnessStateVersions,
  restoreHarnessStateVersion,
  updateHarnessState,
} from '@/lib/api'
import type {
  ContinualLearningScheduleStatus,
  HarnessStateSnapshot,
  HarnessStateVersion,
} from '@/lib/api'
import { formatDateTime } from '@/i18n'
import { AGENTS } from './agent-registry'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

type PendingAction = { kind: 'delete'; path: string } | { kind: 'restore'; version: HarnessStateVersion }

export function ContinualLearningPage({ refreshToken = 0 }: { refreshToken?: number }) {
  const { t } = useTranslation()
  const [snapshot, setSnapshot] = useState<HarnessStateSnapshot | null>(null)
  const [versions, setVersions] = useState<HarnessStateVersion[]>([])
  const [schedule, setSchedule] = useState<ContinualLearningScheduleStatus | null>(null)
  const [selectedPath, setSelectedPath] = useState('')
  const [content, setContent] = useState('')
  const [savedContent, setSavedContent] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [diff, setDiff] = useState('')
  const [diffLoading, setDiffLoading] = useState(false)
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null)

  const load = useCallback(async (preferredPath?: string) => {
    setLoading(true)
    setError(null)
    try {
      const [nextSnapshot, nextVersions, nextSchedule] = await Promise.all([
        getHarnessState(),
        getHarnessStateVersions(),
        getContinualLearningSchedule(),
      ])
      setSnapshot(nextSnapshot)
      setVersions(nextVersions)
      setSchedule(nextSchedule)
      const paths = nextSnapshot.files.map((file) => file.path)
      const nextPath = preferredPath && paths.includes(preferredPath)
        ? preferredPath
        : paths.includes(selectedPath) ? selectedPath : paths[0] || ''
      const nextContent = nextSnapshot.files.find((file) => file.path === nextPath)?.content || ''
      setSelectedPath(nextPath)
      setContent(nextContent)
      setSavedContent(nextContent)
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setLoading(false)
    }
  }, [selectedPath])

  useEffect(() => {
    void load(selectedPath)
    // refreshToken is an explicit publication signal from Harness Optimizer.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshToken])

  const selectFile = (path: string) => {
    if (!snapshot) return
    if (content !== savedContent) {
      toast.error(t('continualLearning.unsavedConfirm'))
      return
    }
    const nextContent = snapshot.files.find((file) => file.path === path)?.content || ''
    setSelectedPath(path)
    setContent(nextContent)
    setSavedContent(nextContent)
  }

  const createFile = (kind: 'prompt' | 'context' | 'tools' | 'subagent') => {
    if (!snapshot) return
    if (content !== savedContent) {
      toast.error(t('continualLearning.unsavedConfirm'))
      return
    }
    const existing = new Set(snapshot.files.map((file) => file.path))
    const created = newFile(kind, existing)
    if (!created) {
      toast.error(t('continualLearning.promptFilesComplete'))
      return
    }
    setSelectedPath(created.path)
    setContent(created.content)
    setSavedContent('')
  }

  const save = async () => {
    if (!snapshot || !selectedPath || content === savedContent || saving) return
    setSaving(true)
    setError(null)
    try {
      await updateHarnessState({
        base_revision: snapshot.revision,
        summary: `Update ${selectedPath}`,
        changes: [{ path: selectedPath, content }],
      })
      toast.success(t('continualLearning.saved'))
      await load(selectedPath)
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setSaving(false)
    }
  }

  const deleteFile = async (path: string) => {
    if (!snapshot || saving) return
    setSaving(true)
    setError(null)
    try {
      await updateHarnessState({
        base_revision: snapshot.revision,
        summary: `Remove ${path}`,
        changes: [{ path, delete: true }],
      })
      toast.success(t('continualLearning.deleted'))
      await load()
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setSaving(false)
      setPendingAction(null)
    }
  }

  const showDiff = async (index: number) => {
    const current = versions[index]
    const previous = versions[index + 1]
    if (!current || !previous) {
      setDiff(t('continualLearning.history.firstVersion'))
      return
    }
    setDiffLoading(true)
    setError(null)
    try {
      const result = await getHarnessStateVersionDiff(previous.id, current.id)
      setDiff(result.patch || t('continualLearning.history.noChanges'))
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setDiffLoading(false)
    }
  }

  const restoreVersion = async (version: HarnessStateVersion) => {
    if (content !== savedContent) {
      toast.error(t('continualLearning.unsavedConfirm'))
      setPendingAction(null)
      return
    }
    setSaving(true)
    setError(null)
    try {
      await restoreHarnessStateVersion(version.id)
      toast.success(t('continualLearning.history.restored'))
      setDiff('')
      await load(selectedPath)
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setSaving(false)
      setPendingAction(null)
    }
  }

  const discard = () => {
    if (!snapshot) return
    const savedFile = snapshot.files.find((file) => file.path === selectedPath)
    if (savedFile) {
      setContent(savedFile.content)
      setSavedContent(savedFile.content)
      return
    }
    const fallback = snapshot.files[0]
    setSelectedPath(fallback?.path || '')
    setContent(fallback?.content || '')
    setSavedContent(fallback?.content || '')
  }

  const dirty = content !== savedContent
  const pathGroups = useMemo(() => groupFiles(snapshot?.files.map((file) => file.path) || []), [snapshot])
  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)]">
      <header className="shrink-0 border-b border-[var(--nova-border)] px-4 py-3 sm:px-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h1 className="text-sm font-semibold text-[var(--nova-text)]">{t('continualLearning.title')}</h1>
              <Badge variant="outline" className="h-4 px-1.5 text-[9px] tracking-[0.12em]">LAB</Badge>
            </div>
            <p className="mt-1 max-w-2xl text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('continualLearning.description')}</p>
          </div>
          {schedule && (
            <div className="flex items-center gap-2 text-[10px] text-[var(--nova-text-muted)]">
              <Clock3 className="h-3.5 w-3.5" />
              {schedule.enabled
                ? t('continualLearning.schedule.enabled', { hours: schedule.interval_hours })
                : t('continualLearning.schedule.disabled')}
              {schedule.last_success && <span>· {formatDateTime(schedule.last_success)}</span>}
            </div>
          )}
        </div>
      </header>
      {error && <div className="shrink-0 border-b border-[var(--nova-border)] bg-red-500/5 px-4 py-2 text-xs text-red-400">{error}</div>}
      <Tabs defaultValue="state" className="min-h-0 flex-1 gap-0">
        <TabsList variant="line" className="mx-4 h-10 shrink-0 sm:mx-5">
          <TabsTrigger value="state"><FileCode2 />{t('continualLearning.state')}</TabsTrigger>
          <TabsTrigger value="history"><History />{t('continualLearning.history')}</TabsTrigger>
        </TabsList>
        <TabsContent value="state" className="min-h-0 overflow-hidden border-t border-[var(--nova-border)]">
          {loading ? (
            <div className="grid h-full place-items-center text-xs text-[var(--nova-text-faint)]">{t('common.loading')}</div>
          ) : (
            <div className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)] md:grid-cols-[220px_minmax(0,1fr)] md:grid-rows-1">
              <aside className="min-h-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] md:border-r md:border-b-0">
                <div className="flex h-10 items-center justify-between border-b border-[var(--nova-border)] px-3">
                  <span className="text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">{t('continualLearning.files')}</span>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button type="button" size="icon-xs" variant="ghost" aria-label={t('continualLearning.newFile')}><Plus /></Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-48">
                      <DropdownMenuItem onSelect={() => createFile('prompt')}><FilePlus2 />{t('continualLearning.file.prompt')}</DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => createFile('context')}><FilePlus2 />{t('continualLearning.file.context')}</DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => createFile('subagent')}><FilePlus2 />{t('continualLearning.file.subagent')}</DropdownMenuItem>
                      <DropdownMenuItem disabled={snapshot?.files.some((file) => file.path === 'tools.toml')} onSelect={() => createFile('tools')}><FilePlus2 />{t('continualLearning.file.tools')}</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
                <div className="flex gap-1 overflow-x-auto p-2 md:block md:h-[calc(100%-2.5rem)] md:overflow-y-auto">
                  {pathGroups.length === 0 && <div className="px-2 py-5 text-center text-[11px] text-[var(--nova-text-faint)]">{t('continualLearning.empty')}</div>}
                  {pathGroups.map((group) => (
                    <div key={group.name} className="shrink-0 md:mb-3">
                      <div className="hidden px-2 py-1 text-[9px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-faint)] md:block">{group.name}</div>
                      {group.paths.map((path) => (
                        <button
                          key={path}
                          type="button"
                          onClick={() => selectFile(path)}
                          className={`block w-full min-w-36 truncate rounded-[var(--nova-radius)] px-2 py-1.5 text-left font-mono text-[11px] transition-colors ${selectedPath === path ? 'bg-[var(--nova-surface-3)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-surface-3)]'}`}
                        >
                          {path.split('/').at(-1)}
                        </button>
                      ))}
                    </div>
                  ))}
                </div>
              </aside>
              <section className="flex min-h-0 min-w-0 flex-col">
                <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
                  <code className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-muted)]">{selectedPath || t('continualLearning.noFile')}</code>
                  {dirty && <span className="h-1.5 w-1.5 rounded-full bg-amber-500" aria-label={t('continualLearning.unsaved')} />}
                  {selectedPath && snapshot?.files.some((file) => file.path === selectedPath) && (
                    <Button type="button" size="icon-xs" variant="ghost" disabled={saving} onClick={() => setPendingAction({ kind: 'delete', path: selectedPath })} aria-label={t('continualLearning.delete')}><Trash2 /></Button>
                  )}
                  {dirty && (
                    <Button type="button" size="xs" variant="ghost" disabled={saving} onClick={discard}>
                      <RotateCcw />{t('continualLearning.discard')}
                    </Button>
                  )}
                  <Button type="button" size="xs" disabled={!dirty || saving} onClick={() => void save()}>
                    <Save />{saving ? t('continualLearning.saving') : t('continualLearning.save')}
                  </Button>
                </div>
                {selectedPath ? (
                  <Textarea
                    autoResize={false}
                    value={content}
                    onChange={(event) => setContent(event.target.value)}
                    spellCheck={false}
                    aria-label={t('continualLearning.editorLabel', { path: selectedPath })}
                    className="h-full min-h-0 flex-1 resize-none rounded-none border-0 bg-[var(--nova-bg)] p-4 font-mono text-xs leading-5 focus-visible:ring-0"
                  />
                ) : (
                  <div className="grid h-full place-items-center px-6 text-center text-xs text-[var(--nova-text-faint)]">{t('continualLearning.emptyAction')}</div>
                )}
              </section>
            </div>
          )}
        </TabsContent>
        <TabsContent value="history" className="min-h-0 overflow-hidden border-t border-[var(--nova-border)]">
          <div className="grid h-full min-h-0 md:grid-cols-[minmax(240px,34%)_minmax(0,1fr)]">
            <div className="min-h-0 overflow-y-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 md:border-r md:border-b-0">
              {versions.length === 0 && <div className="py-8 text-center text-xs text-[var(--nova-text-faint)]">{t('continualLearning.history.empty')}</div>}
              {versions.map((version, index) => (
                <div key={version.id} className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
                  <button type="button" className="w-full text-left" onClick={() => void showDiff(index)}>
                    <div className="line-clamp-2 text-xs font-medium text-[var(--nova-text)]">{version.summary}</div>
                    <div className="mt-1 font-mono text-[9px] text-[var(--nova-text-faint)]">{formatDateTime(version.created_at)} · {version.revision.slice(0, 10)}</div>
                  </button>
                  <Button type="button" size="xs" variant="ghost" className="mt-2" disabled={saving || version.revision === snapshot?.revision} onClick={() => setPendingAction({ kind: 'restore', version })}>
                    <RotateCcw />{t('continualLearning.history.restore')}
                  </Button>
                </div>
              ))}
            </div>
            <pre className="min-h-0 overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-[11px] leading-5 text-[var(--nova-text-muted)]">{diffLoading ? t('common.loading') : diff || t('continualLearning.history.select')}</pre>
          </div>
        </TabsContent>
      </Tabs>
      <AlertDialog open={pendingAction !== null} onOpenChange={(open) => { if (!open) setPendingAction(null) }}>
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogTitle>{pendingAction?.kind === 'delete' ? t('continualLearning.deleteTitle') : t('continualLearning.history.restoreTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{pendingAction?.kind === 'delete' ? t('continualLearning.deleteDescription', { path: pendingAction.path }) : t('continualLearning.history.restoreDescription')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant={pendingAction?.kind === 'delete' ? 'destructive' : 'default'}
              onClick={() => {
                if (pendingAction?.kind === 'delete') void deleteFile(pendingAction.path)
                else if (pendingAction?.kind === 'restore') void restoreVersion(pendingAction.version)
              }}
            >
              {pendingAction?.kind === 'delete' ? t('continualLearning.delete') : t('continualLearning.history.restore')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function groupFiles(paths: string[]) {
  const order = ['prompts', 'context', 'subagents', 'root']
  const groups = new Map<string, string[]>()
  for (const path of paths) {
    const group = path.includes('/') ? path.split('/')[0] : 'root'
    groups.set(group, [...(groups.get(group) || []), path])
  }
  return [...groups.entries()]
    .sort(([left], [right]) => order.indexOf(left) - order.indexOf(right))
    .map(([name, groupedPaths]) => ({ name, paths: groupedPaths.sort() }))
}

function newFile(kind: 'prompt' | 'context' | 'tools' | 'subagent', existing: Set<string>) {
  if (kind === 'tools') {
    return { path: 'tools.toml', content: '[tools.read]\ndescription = "Read the narrowest relevant source before making a change."\n' }
  }
  if (kind === 'prompt') {
    const agent = AGENTS.find((candidate) => !existing.has(`prompts/${candidate.key}.md`))?.key
    if (!agent) return null
    return { path: `prompts/${agent}.md`, content: 'Add durable user preferences and reusable behavior here.\n' }
  }
  const base = kind === 'context' ? 'preference' : 'specialist'
  let index = 1
  while (existing.has(`${kind === 'context' ? 'context' : 'subagents'}/${base}-${index}.md`)) index++
  const id = `${base}-${index}`
  if (kind === 'context') {
    return {
      path: `context/${id}.md`,
      content: `---\nid: ${id}\npurpose: Preserve one durable user preference.\nagents: [general]\nplacement: leading_message\nenabled: true\n---\n\nDescribe the reusable context to inject.\n`,
    }
  }
  return {
    path: `subagents/${id}.md`,
    content: `---\nid: ${id}\nname: Specialist\ndescription: Handle one bounded specialist task.\nenabled: true\nparents: [general]\nmodel_profile: default\ntools: [workspace_read]\n---\n\nHandle only the delegated task and return concise, evidence-backed results.\n`,
  }
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'Unknown error')
}
