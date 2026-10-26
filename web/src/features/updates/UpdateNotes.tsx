import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, ArrowRight, ChevronDown, ExternalLink, Loader2 } from 'lucide-react'
import { MarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { getUpdateNotes } from '@/features/settings/api'
import type { UpdateNotesRange } from '@/features/settings/types'
import { errorMessage } from '@/lib/error-diagnostics'
import { DENOVA_GITHUB_URL } from '@/lib/product-links'

export function displayVersion(version: string) {
  return version.startsWith('v') ? version : `v${version}`
}

type UpdateNotesProps = { range: UpdateNotesRange } & (
  | { variant: 'preview'; onOpen: () => void }
  | { variant?: 'full' }
)

/** Both the inbox and the pre-update reader use the same localized range.
 * Query caching shares a successful load without coupling reading to installing. */
export function UpdateNotesView(props: UpdateNotesProps) {
  const { range } = props
  const { t, i18n } = useTranslation()
  const [history, setHistory] = useState(false)
  const from = history ? undefined : range.fromVersion
  const notes = useQuery({
    queryKey: ['update-notes', from, range.toVersion, i18n.resolvedLanguage],
    queryFn: ({ signal }) => getUpdateNotes({ fromVersion: from, toVersion: range.toVersion }, signal),
    staleTime: 5 * 60_000,
    retry: false,
  })
  const preview = props.variant === 'preview'
  return <section aria-label={t(history ? 'settings.updates.notes.history' : 'settings.updates.notes.thisUpgrade')} className="flex min-h-0 min-w-0 flex-col gap-4 text-xs [--nova-reading-font-size:var(--nova-ui-body-font-size)] [--nova-reading-font-family:var(--nova-ui-font-family)]">
    <div className="flex shrink-0 flex-wrap items-center gap-2 text-sm tabular-nums text-muted-foreground">
      {history && <span>{t('settings.updates.notes.history')}</span>}
      {from && <><span>{displayVersion(from)}</span><ArrowRight className="size-3.5" aria-hidden="true" /></>}
      <span>{displayVersion(range.toVersion)}</span>
      {notes.data && <span className="text-xs">{t('settings.updates.notes.versionCount', { count: notes.data.releases.length })}</span>}
    </div>
    <div className="flex min-h-0 flex-col gap-4 overflow-y-auto overscroll-contain">
    {notes.isPending ? <div role="status" className="flex items-center gap-2 py-4 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" />{t('settings.updates.notes.loading')}</div>
      : notes.isError ? <div className="flex flex-col items-start gap-2">
        <InlineErrorNotice message={errorMessage(notes.error, t('settings.updates.notes.loadFailed'))} />
        <div className="flex flex-wrap gap-2"><Button variant="outline" size="sm" onClick={() => void notes.refetch()}>{t('common.retry')}</Button><Button variant="ghost" size="sm" asChild><a href={`${DENOVA_GITHUB_URL}/releases`} target="_blank" rel="noreferrer">{t('settings.updates.notes.releaseHistory')}<ExternalLink data-icon="inline-end" /></a></Button></div>
      </div> : notes.data && <>
        {!notes.data.complete && <p role="status" className="text-sm text-muted-foreground">{t('settings.updates.notes.incomplete')}</p>}
        {notes.data.releases.length === 0 ? <Empty><EmptyHeader><EmptyTitle>{t('settings.updates.notes.empty')}</EmptyTitle></EmptyHeader></Empty>
          : preview ? <ul className="flex flex-col gap-2 text-sm text-muted-foreground">
            {notes.data.releases.map(release => <li key={release.version} className="flex items-start gap-3"><span className="shrink-0 text-xs leading-5 tabular-nums">{displayVersion(release.version)}</span><span>{release.summary || release.version}</span></li>)}
          </ul> : <div className="flex flex-col divide-y divide-border">
            {notes.data.releases.map(release => {
              const heading = <div className="flex flex-wrap items-center gap-3">
                <span className="font-medium tabular-nums text-foreground">{displayVersion(release.version)}</span>
                {release.published_at && <time dateTime={release.published_at} className="text-xs text-muted-foreground">{release.published_at.slice(0, 10)}</time>}
                {release.body && <span className="ml-auto flex items-center gap-1 text-xs text-muted-foreground"><span className="group-open:hidden">{t('settings.updates.notes.details')}</span><span className="hidden group-open:inline">{t('settings.updates.notes.collapse')}</span><ChevronDown className="size-3.5 transition-transform group-open:rotate-180" /></span>}
              </div>
              const brief = <div className="chat-agent-message pt-2 text-sm text-muted-foreground"><MarkdownRenderer content={release.brief} /></div>
              return release.body ? <details key={release.version} className="group py-4 first:pt-0">
                <summary className="list-none rounded-sm focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring [&::-webkit-details-marker]:hidden">{heading}<div className="group-open:hidden">{brief}</div></summary>
                <div className="chat-agent-message pt-3 text-sm text-muted-foreground"><MarkdownRenderer content={release.body} /></div>
              </details> : <article key={release.version} className="py-4 first:pt-0">{heading}{brief}</article>
            })}
          </div>}
      </>}
    {preview ? <Button variant="outline" size="sm" className="self-start" onClick={props.onOpen}>{t('settings.updates.notes.view')}<ArrowRight data-icon="inline-end" /></Button>
      : range.fromVersion && <Button variant="ghost" size="sm" className="self-start" onClick={() => setHistory(!history)}>{history && <ArrowLeft data-icon="inline-start" />}{t(history ? 'settings.updates.notes.backToUpgrade' : 'settings.updates.notes.history')}</Button>}
    </div>
  </section>
}

export function UpdateNotesDialog({ range, kind = 'available', open, onOpenChange, actions, error }: {
  range: UpdateNotesRange
  kind?: 'available' | 'completed'
  open: boolean
  onOpenChange: (open: boolean) => void
  actions?: ReactNode
  error?: string | null
}) {
  const { t } = useTranslation()
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col max-md:overflow-hidden">
      <DialogHeader className="shrink-0 pr-6">
        <DialogTitle>{t(kind === 'completed' ? 'settings.updates.notes.completedTitle' : 'settings.updates.notes.availableTitle', { version: displayVersion(range.toVersion) })}</DialogTitle>
        <DialogDescription className="sr-only">{t('settings.updates.notes.thisUpgrade')}</DialogDescription>
      </DialogHeader>
      <div className="flex min-h-0 flex-col gap-4">
        {error && <InlineErrorNotice message={error} />}
        <UpdateNotesView key={`${range.fromVersion}:${range.toVersion}`} range={range} />
      </div>
      <DialogFooter className="shrink-0">
        <Button variant="ghost" onClick={() => onOpenChange(false)}>{t(kind === 'completed' ? 'common.close' : 'settings.updates.notes.later')}</Button>
        {actions}
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
