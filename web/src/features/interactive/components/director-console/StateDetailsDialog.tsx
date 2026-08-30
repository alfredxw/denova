import { useMemo } from 'react'
import { ArrowUpRight, Gauge } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import type { Snapshot } from '../../types'
import { buildStoryStateModel } from '../story-state/model'
import { StoryStateDetails } from '../story-state/StoryStateLedger'

interface StateDetailsDialogProps {
  snapshot: Snapshot | null
  stateError?: string
}

export function StateDetailsDialog({ snapshot, stateError }: StateDetailsDialogProps) {
  const { t } = useTranslation()
  const model = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const actorCount = model.actors.length
  const worldCount = model.worldFacts.length
  const summary = t('directorPanel.overview.state.summary', { actors: actorCount, world: worldCount })
  const error = snapshot?.current_turn?.state_error || stateError

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button type="button" variant="outline" className="h-auto w-full justify-start gap-3 px-3 py-3 text-left whitespace-normal">
          <Gauge data-icon="inline-start" />
          <span className="min-w-0 flex-1">
            <span className="block text-xs font-semibold">{t('directorPanel.overview.state.title')}</span>
            <span className="mt-0.5 block truncate text-[10px] font-normal text-muted-foreground">{summary}</span>
          </span>
          <ArrowUpRight data-icon="inline-end" />
        </Button>
      </DialogTrigger>
      <DialogContent className="director-console grid h-[min(84dvh,760px)] w-[min(760px,calc(100vw-1rem))] max-w-[min(760px,calc(100vw-1rem))] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden bg-[var(--director-canvas)] p-0 text-[var(--nova-text)]">
        <DialogHeader className="border-b border-[var(--nova-border)] bg-[var(--director-panel)] px-4 py-3 pr-12 text-left">
          <DialogTitle className="text-sm">{t('directorPanel.overview.state.dialogTitle')}</DialogTitle>
          <DialogDescription className="text-[11px] text-[var(--nova-text-faint)]">
            {t('directorPanel.overview.state.dialogDescription', { summary })}
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto p-3">
          {error ? <InlineErrorNotice className="mb-3" message={error} /> : null}
          <StoryStateDetails snapshot={snapshot} />
        </div>
      </DialogContent>
    </Dialog>
  )
}
