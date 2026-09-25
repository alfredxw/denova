import { useEffect, useRef, useState } from 'react'
import { ArrowLeft, Download, ExternalLink, Package } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { dependencySelection, discardPreview, localized, previewSource, type ImportSelection, type MarketEntry, type Preview } from './api'
import { ImportResourcePicker } from './ImportResourcePicker'

export function MarketEntryDetail({ detail, acquired, onBack, onImport, onManage }: {
  detail: MarketEntry
  acquired: number
  onBack: () => void
  onImport: (preview: Preview, selection: ImportSelection) => void
  onManage: () => void
}) {
  const { t, i18n } = useTranslation()
  const [preview, setPreview] = useState<Preview>()
  const [selection, setSelection] = useState<ImportSelection>()
  const [busy, setBusy] = useState(true)
  const [error, setError] = useState('')
  const alive = useRef(false)
  const pending = useRef<Promise<Preview> | undefined>(undefined)
  const ownedPreview = useRef<Preview | undefined>(undefined)
  const candidate = preview?.candidates.find((item) => item.candidate_id === selection?.candidateID)
  const chosen = dependencySelection(candidate?.resources || [], selection?.resourceIDs || [])
  const load = async () => {
    if (pending.current) return
    setBusy(true)
    setError('')
    try {
      pending.current = previewSource(detail.source)
      const result = await pending.current
      if (!alive.current) { discardPreview(result); return }
      ownedPreview.current = result
      setPreview(result)
      setSelection({ candidateID: result.candidates[0].candidate_id, resourceIDs: result.candidates[0].resources.map((r) => r.id) })
    } catch (cause) {
      console.error('[market] failed to load package contents', cause)
      if (alive.current) setError(cause instanceof Error ? cause.message : t('market.errors.operationFailed'))
    } finally { pending.current = undefined; if (alive.current) setBusy(false) }
  }
  useEffect(() => {
    alive.current = true
    void load()
    // The page owns the preview; dialogs borrow it. A late response is released by load.
    return () => { alive.current = false; if (ownedPreview.current) discardPreview(ownedPreview.current) }
  }, [])
  return <div className="h-full min-h-0 overflow-auto @container" data-testid="market-entry-detail">
    <article className="mx-auto max-w-6xl space-y-6 p-4 @xl:p-6 @4xl:p-8">
      <Button variant="ghost" className="-ml-3" onClick={onBack}><ArrowLeft data-icon="inline-start" />{t('market.back')}</Button>
      <header className="space-y-4">
        <div className="flex items-start gap-3">
          <div className="flex size-11 shrink-0 items-center justify-center rounded-lg border bg-muted/30"><Package className="size-5 text-muted-foreground" /></div>
          <div className="min-w-0 space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight [overflow-wrap:anywhere]">{localized(detail.name, i18n.language)}</h1>
            <p className="text-sm text-muted-foreground [overflow-wrap:anywhere]">{detail.author} · {new Date(`${detail.updated_at}T00:00:00`).toLocaleDateString(i18n.language)}{candidate?.package.version && ` · v${candidate.package.version}`}</p>
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs leading-relaxed text-muted-foreground [overflow-wrap:anywhere]">
              <a className="inline-flex min-w-0 items-center gap-1 underline-offset-4 hover:underline" href={detail.source.url} target="_blank" rel="noreferrer"><span className="min-w-0 [overflow-wrap:anywhere]">{detail.source.url?.replace(/^https:\/\//, '')}</span><ExternalLink className="size-3 shrink-0" /></a>
              {detail.source.ref && <span>· {detail.source.ref}</span>}
              {detail.source.path && <span className="min-w-0 font-mono">· {detail.source.path}</span>}
            </div>
          </div>
        </div>
        <p className="max-w-3xl whitespace-pre-wrap leading-relaxed [overflow-wrap:anywhere]">{localized(detail.description, i18n.language)}</p>
        <div className="flex flex-wrap gap-2">{detail.kinds.map((kind) => <Badge key={kind} variant="secondary">{t(`market.kinds.${kind}`)}</Badge>)}{detail.tags.map((tag) => <Badge key={tag} variant="outline">{t(`market.tag.${tag}`, { defaultValue: tag })}</Badge>)}</div>
      </header>
      <div className="grid min-w-0 items-start gap-6 border-t pt-6 @4xl:grid-cols-[minmax(0,1fr)_17rem] @4xl:gap-8">
        <aside className="space-y-3 @4xl:space-y-4 @4xl:rounded-xl @4xl:border @4xl:bg-muted/20 @4xl:p-4 @4xl:sticky @4xl:top-6 @4xl:col-start-2 @4xl:row-start-1">
          <div className="hidden space-y-2 @4xl:block">
            <h2 className="text-sm font-semibold">{t('market.contents.get')}</h2>
            <p className="text-xs leading-relaxed text-muted-foreground">{t('market.contents.selectHelp')}</p>
          </div>
          <Button className="h-auto min-h-9 w-full whitespace-normal" disabled={busy || (!!preview && !chosen.length)} onClick={() => preview && selection ? onImport(preview, selection) : void load()}>
            <Download data-icon="inline-start" />{t(busy ? 'market.contents.loading' : preview ? 'market.contents.importSelected' : 'market.contents.retry', { count: chosen.length })}
          </Button>
          {acquired > 0 && <Button variant="outline" className="w-full" onClick={onManage}>{t('market.contents.acquired', { count: acquired })}</Button>}
          {error && <p role="alert" className="text-sm text-destructive [overflow-wrap:anywhere]">{error}</p>}
          {detail.compatibility && <div className="space-y-2 @4xl:border-t @4xl:pt-4"><h3 className="hidden text-xs font-medium @4xl:block">{t('market.contents.compatibility')}</h3><p className="whitespace-pre-wrap text-xs leading-relaxed text-muted-foreground">{localized(detail.compatibility, i18n.language)}</p></div>}
        </aside>
        <div className="min-w-0 space-y-7 @4xl:col-start-1 @4xl:row-start-1">
          <section className="min-w-0 space-y-4" aria-label={t('market.contents.title')}>
            <div className="space-y-1"><h2 className="font-semibold">{t('market.contents.title')}</h2><p className="text-sm text-muted-foreground">{t('market.contents.help')}</p></div>
            {preview && candidate && selection ? <>
              {preview.candidates.length > 1 && <Select value={selection.candidateID} onValueChange={(id) => {
                const next = preview.candidates.find((item) => item.candidate_id === id)!
                setSelection({ candidateID: id, resourceIDs: next.resources.map((r) => r.id) })
              }}><SelectTrigger className="w-full" aria-label={t('market.import.package')}><SelectValue /></SelectTrigger><SelectContent>{preview.candidates.map((item) => <SelectItem key={item.candidate_id} value={item.candidate_id}>{item.package.name}</SelectItem>)}</SelectContent></Select>}
              <ImportResourcePicker key={candidate.candidate_id} previewID={preview.preview_id} candidate={candidate} selected={selection.resourceIDs} onChange={(resourceIDs) => setSelection({ ...selection, resourceIDs })} />
            </> : <div className="rounded-lg border border-dashed p-6 text-sm leading-relaxed text-muted-foreground" aria-busy={busy}>{t(busy ? 'market.contents.loading' : 'market.contents.failed')}</div>}
          </section>
          {detail.usage && <section className="space-y-3 border-t pt-6"><h2 className="font-semibold">{t('market.contents.usage')}</h2><p className="whitespace-pre-wrap text-sm leading-7 text-muted-foreground [overflow-wrap:anywhere]">{localized(detail.usage, i18n.language)}</p></section>}
        </div>
      </div>
    </article>
  </div>
}
