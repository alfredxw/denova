import { useEffect, useMemo, useState } from 'react'
import { Loader2, RefreshCw, Tags } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { applyLoreClassification, previewLoreClassification, type LoreClassificationMode, type LoreClassificationPreview, type LoreItem } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

const LORE_TYPES: LoreItem['type'][] = ['character', 'world', 'location', 'faction', 'rule', 'item', 'other']

export function LoreClassificationDialog({
  open,
  onOpenChange,
  onApplied,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onApplied: (items: LoreItem[]) => void
}) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<LoreClassificationMode>('semantic')
  const [preview, setPreview] = useState<LoreClassificationPreview | null>(null)
  const [types, setTypes] = useState<Record<string, LoreItem['type']>>({})
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [refreshToken, setRefreshToken] = useState(0)
  const [loading, setLoading] = useState(false)
  const [applying, setApplying] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    let cancelled = false
    setLoading(true)
    setError('')
    previewLoreClassification({ mode })
      .then((result) => {
        if (cancelled) return
        const nextTypes: Record<string, LoreItem['type']> = {}
        const nextSelected = new Set<string>()
        result.items.forEach((item) => {
          nextTypes[item.id] = item.suggested_type
          if (item.suggested_type !== item.current_type) nextSelected.add(item.id)
        })
        setPreview(result)
        setTypes(nextTypes)
        setSelected(nextSelected)
      })
      .catch((err) => {
        if (cancelled) return
        console.warn('[lore-classification] 生成分类预览失败', err)
        setPreview(null)
        setError(t('settingPanel.loreClassification.previewFailed'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [mode, open, refreshToken, t])

  const changes = useMemo(() => {
    if (!preview) return []
    return preview.items
      .filter((item) => selected.has(item.id) && types[item.id] !== item.current_type)
      .map((item) => ({ id: item.id, type: types[item.id] }))
  }, [preview, selected, types])

  const updateType = (id: string, type: LoreItem['type'], currentType: LoreItem['type']) => {
    setTypes((current) => ({ ...current, [id]: type }))
    setSelected((current) => {
      const next = new Set(current)
      if (type === currentType) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const applyChanges = async () => {
    if (!preview || changes.length === 0) return
    setApplying(true)
    setError('')
    try {
      const result = await applyLoreClassification({ revision: preview.revision, changes })
      onApplied(result.items)
      toast.success(t('settingPanel.loreClassification.applied', { count: result.updated.length }))
      onOpenChange(false)
    } catch (err) {
      console.warn('[lore-classification] 应用分类失败', err)
      setError(t('settingPanel.loreClassification.applyFailed'))
    } finally {
      setApplying(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => {
      if (!applying) onOpenChange(nextOpen)
    }}>
      <DialogContent className="max-w-[min(calc(100vw-2rem),760px)] gap-3 border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2"><Tags className="h-4 w-4" />{t('settingPanel.loreClassification.title')}</DialogTitle>
          <DialogDescription>{t('settingPanel.loreClassification.description')}</DialogDescription>
        </DialogHeader>

        <div className="flex items-start justify-between gap-4 rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5">
          <div className="min-w-0">
            <div className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.loreClassification.semantic')}</div>
            <div className="mt-1 text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('settingPanel.loreClassification.semanticHint')}</div>
          </div>
          <Switch
            checked={mode === 'semantic'}
            onCheckedChange={(checked) => setMode(checked ? 'semantic' : 'heuristic')}
            disabled={loading || applying}
            aria-label={t('settingPanel.loreClassification.semantic')}
          />
        </div>

        {preview?.warning ? (
          <div className="rounded-lg border border-[var(--nova-warning)]/25 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] leading-4 text-[var(--nova-text-muted)]">
            {t('settingPanel.loreClassification.fallbackWarning')}
          </div>
        ) : null}
        {error ? <div className="rounded-lg border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">{error}</div> : null}

        <ScrollArea className="h-[min(48vh,420px)] rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          {loading ? (
            <div className="flex h-40 items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]"><Loader2 className="h-4 w-4 animate-spin" />{t('settingPanel.loreClassification.analyzing')}</div>
          ) : !preview?.items.length ? (
            <div className="flex h-40 items-center justify-center px-6 text-center text-xs leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.loreClassification.empty')}</div>
          ) : (
            <div className="divide-y divide-[var(--nova-border)]">
              {preview.items.map((item) => {
                const nextType = types[item.id] || item.suggested_type
                const checked = selected.has(item.id) && nextType !== item.current_type
                return (
                  <div key={item.id} className="grid min-h-16 grid-cols-[auto_minmax(0,1fr)] items-center gap-3 px-3 py-2 sm:grid-cols-[auto_minmax(0,1fr)_minmax(210px,0.7fr)]">
                    <input
                      type="checkbox"
                      className="h-4 w-4 accent-[var(--nova-accent)]"
                      checked={checked}
                      disabled={applying || nextType === item.current_type}
                      onChange={(event) => setSelected((current) => {
                        const next = new Set(current)
                        if (event.target.checked) next.add(item.id)
                        else next.delete(item.id)
                        return next
                      })}
                      aria-label={item.name}
                    />
                    <div className="min-w-0">
                      <div className="truncate text-xs font-medium text-[var(--nova-text)]">{item.name}</div>
                      <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">
                        {t('settingPanel.loreClassification.current', { type: t(`lore.type.${item.current_type}`) })} · {t(`settingPanel.loreClassification.confidence.${item.confidence}`)} · {t(`settingPanel.loreClassification.source.${item.suggestion_source}`)}
                      </div>
                    </div>
                    <Select value={nextType} onValueChange={(value) => updateType(item.id, value as LoreItem['type'], item.current_type)} disabled={applying}>
                      <SelectTrigger size="sm" className="nova-field col-span-2 h-8 text-xs focus:ring-0 sm:col-span-1"><SelectValue /></SelectTrigger>
                      <SelectContent className="nova-panel border text-[var(--nova-text)]">
                        {LORE_TYPES.map((type) => <SelectItem key={type} value={type}>{t(`lore.type.${type}`)}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                )
              })}
            </div>
          )}
        </ScrollArea>

        <DialogFooter className="flex-row flex-wrap items-center gap-2 sm:justify-between">
          <div className="w-full text-[11px] text-[var(--nova-text-faint)] sm:mr-auto sm:w-auto">{t('settingPanel.loreClassification.selected', { count: changes.length })}</div>
          <Button variant="outline" size="sm" disabled={loading || applying} onClick={() => {
            setPreview(null)
            setRefreshToken((current) => current + 1)
          }}>
            <RefreshCw className="h-3.5 w-3.5" />{t('settingPanel.loreClassification.refresh')}
          </Button>
          <Button variant="outline" size="sm" disabled={applying} onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button size="sm" disabled={loading || applying || changes.length === 0} onClick={() => void applyChanges()}>
            {applying ? <Loader2 className="h-4 w-4 animate-spin" /> : <Tags className="h-4 w-4" />}
            {t('settingPanel.loreClassification.apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
