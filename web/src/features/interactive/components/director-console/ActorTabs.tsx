import { useEffect, useMemo, useRef, useState } from 'react'
import { MoreHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

export interface ActorTabItem {
  id: string
  name: string
}

const MIN_TAB_WIDTH = 84
const MORE_TRIGGER_WIDTH = 72

export function ActorTabs({ actors, value, onValueChange }: { actors: ActorTabItem[]; value: string; onValueChange: (actorId: string) => void }) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement>(null)
  const [visibleCapacity, setVisibleCapacity] = useState(actors.length)

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const updateCapacity = (width: number) => {
      if (width <= 0) return
      const capacityWithoutOverflow = Math.max(1, Math.floor(width / MIN_TAB_WIDTH))
      const nextCapacity = actors.length <= capacityWithoutOverflow
        ? actors.length
        : Math.max(1, Math.floor((width - MORE_TRIGGER_WIDTH) / MIN_TAB_WIDTH))
      setVisibleCapacity(Math.min(actors.length, nextCapacity))
    }

    updateCapacity(container.clientWidth)
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(([entry]) => updateCapacity(entry.contentRect.width))
    observer.observe(container)
    return () => observer.disconnect()
  }, [actors.length])

  const visibleActors = useMemo(() => {
    const visible = actors.slice(0, visibleCapacity)
    const selectedActor = actors.find((actor) => actor.id === value)
    if (!selectedActor || visible.some((actor) => actor.id === value) || visible.length === 0) return visible
    return [...visible.slice(0, -1), selectedActor]
  }, [actors, value, visibleCapacity])
  const visibleActorIds = useMemo(() => new Set(visibleActors.map((actor) => actor.id)), [visibleActors])
  const overflowActors = actors.filter((actor) => !visibleActorIds.has(actor.id))

  return (
    <div ref={containerRef} className="flex min-w-0 items-center gap-1.5">
      <Tabs value={value} onValueChange={onValueChange} className="min-w-0 flex-1 gap-0">
        <TabsList variant="line" aria-label={t('directorPanel.actorCue')} className="flex h-8 w-full min-w-0 justify-start gap-0 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0">
          {visibleActors.map((actor) => (
            <TabsTrigger
              key={actor.id}
              value={actor.id}
              title={actor.name}
              className="h-full min-w-0 flex-1 rounded-none border-0 border-r border-[var(--nova-border)] px-2 text-[11px] after:bottom-0 last:border-r-0 data-active:bg-[var(--nova-active)]"
            >
              <span className="min-w-0 truncate">{actor.name}</span>
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {overflowActors.length > 0 ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label={t('directorPanel.actorMoreLabel')}
              title={t('directorPanel.actorMoreLabel')}
              className="flex h-8 shrink-0 items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[10px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
            >
              <MoreHorizontal className="h-3.5 w-3.5" />
              {t('directorPanel.actorMore')}
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-40 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]">
            {overflowActors.map((actor) => (
              <DropdownMenuItem key={actor.id} onSelect={() => onValueChange(actor.id)} className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]">
                <span className="min-w-0 truncate">{actor.name}</span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  )
}
