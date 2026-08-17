import { Check, ExternalLink, ListChecks, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import type { GlobalAgentRunTraceSummary } from '@/lib/api'
import { cn } from '@/lib/utils'

interface HarnessRunPickerProps {
  runs: GlobalAgentRunTraceSummary[]
  selected: ReadonlySet<string>
  loading: boolean
  onToggle: (trajectoryURI: string) => void
  onClear: () => void
  onView: (trajectoryURI: string) => void
}

/** Compact evidence picker backed by the same global Run catalog as Trajectory. */
export function HarnessRunPicker({ runs, selected, loading, onToggle, onClear, onView }: HarnessRunPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const selectedCount = runs.reduce((count, run) => count + Number(selected.has(run.trajectory_uri)), 0)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          size="xs"
          variant="outline"
          className="max-w-52 justify-start"
          aria-label={t('continualLearning.evidence.open')}
          aria-expanded={open}
        >
          <ListChecks />
          <span className="truncate">
            {selectedCount > 0
              ? t('continualLearning.evidence.selected', { count: selectedCount })
              : t('continualLearning.evidence.auto')}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={6}
        collisionPadding={8}
        className="w-[min(92vw,430px)] overflow-hidden rounded-[var(--nova-radius)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 shadow-[var(--nova-shadow)]"
      >
        <Command className="bg-transparent">
          <CommandInput placeholder={t('continualLearning.evidence.search')} />
          <CommandList className="max-h-[min(58dvh,26rem)]">
            <CommandEmpty>{loading ? t('common.loading') : t('continualLearning.evidence.empty')}</CommandEmpty>
            <CommandGroup>
              {runs.map((run) => {
                const checked = selected.has(run.trajectory_uri)
                const label = `${run.project_name} ${run.agent_kind || ''} ${run.status} ${run.id}`
                return (
                  <CommandItem
                    key={run.trajectory_uri}
                    value={label}
                    aria-checked={checked}
                    onSelect={() => onToggle(run.trajectory_uri)}
                    className="group min-h-12 items-start gap-2 px-2 py-2"
                  >
                    <span
                      aria-hidden="true"
                      className={cn(
                        'mt-0.5 grid size-4 shrink-0 place-items-center rounded-[4px] border',
                        checked
                          ? 'border-[var(--nova-text)] bg-[var(--nova-text)] text-[var(--nova-bg)]'
                          : 'border-[var(--nova-border-strong)]',
                      )}
                    >
                      {checked ? <Check className="size-3" /> : null}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="flex min-w-0 items-center gap-2">
                        <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-[var(--nova-text)]">{run.project_name}</span>
                        <Badge variant="outline" className="h-4 shrink-0 px-1 text-[8px] uppercase">{run.status}</Badge>
                      </span>
                      <span className="mt-0.5 block truncate font-mono text-[9px] text-[var(--nova-text-faint)]">
                        {run.agent_kind || t('trajectory.runs.agent')} · {shortRunID(run.id)}
                      </span>
                    </span>
                    <Button
                      type="button"
                      size="icon-xs"
                      variant="ghost"
                      className="shrink-0 opacity-70 group-hover:opacity-100"
                      aria-label={t('continualLearning.evidence.view', { project: run.project_name })}
                      onPointerDown={(event) => {
                        event.preventDefault()
                        event.stopPropagation()
                      }}
                      onClick={(event) => {
                        event.preventDefault()
                        event.stopPropagation()
                        setOpen(false)
                        onView(run.trajectory_uri)
                      }}
                    >
                      <ExternalLink />
                    </Button>
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </CommandList>
          <div className="flex min-h-9 items-center justify-between gap-2 border-t border-[var(--nova-border)] px-2 py-1">
            <span className="text-[9px] text-[var(--nova-text-faint)]">
              {selectedCount > 0
                ? t('continualLearning.evidence.strictHint')
                : t('continualLearning.evidence.autoHint')}
            </span>
            <Button type="button" size="xs" variant="ghost" disabled={selectedCount === 0} onClick={onClear}>
              <X />{t('continualLearning.evidence.clear')}
            </Button>
          </div>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

function shortRunID(runID: string) {
  return runID.length <= 28 ? runID : `${runID.slice(0, 18)}…${runID.slice(-7)}`
}
