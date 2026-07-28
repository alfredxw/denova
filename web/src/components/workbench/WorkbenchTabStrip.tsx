import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
} from 'react'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

const TAB_TITLE_TOOLTIP_DELAY_MS = 500

const WORKBENCH_TAB_CLASS = [
  'group/tab relative h-full min-w-28 max-w-40 flex-[1_1_10rem] justify-start gap-1.5 px-3 text-xs',
  'rounded-none border-0 border-r border-[var(--nova-border)] after:hidden',
  'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
  // Context-menu triggers also use data-state, so aria-selected is the stable active selector.
  'aria-selected:bg-[var(--nova-active)] aria-selected:text-[var(--nova-text)]',
  'dark:aria-selected:border-[var(--nova-border)] dark:aria-selected:bg-[var(--nova-active)] dark:aria-selected:text-[var(--nova-text)]',
].join(' ')

function revealActiveTab(strip: HTMLElement) {
  const activeTab = strip.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]')
  if (!activeTab) return
  const stripRect = strip.getBoundingClientRect()
  const activeRect = activeTab.getBoundingClientRect()
  if (activeRect.left < stripRect.left) {
    strip.scrollLeft -= stripRect.left - activeRect.left
  } else if (activeRect.right > stripRect.right) {
    strip.scrollLeft += activeRect.right - stripRect.right
  }
}

interface WorkbenchTabStripProps extends Omit<ComponentProps<'div'>, 'children'> {
  value: string
  onValueChange: (value: string) => void
  children: ReactNode
  /** An add control that follows the final tab until overflow pins it to the strip edge. */
  flowAction?: ReactNode
  /** Persistent controls that remain outside the scrollable document tabs. */
  endActions?: ReactNode
}

/**
 * Shared shell for document-style workbench tabs.
 *
 * Tabs start at one common maximum width, shrink uniformly to the shared minimum, then scroll
 * without exposing a scrollbar. Navigation/segmented tabs intentionally use the base Tabs API.
 */
export function WorkbenchTabStrip({
  value,
  onValueChange,
  children,
  flowAction,
  endActions,
  className,
  ...props
}: WorkbenchTabStripProps) {
  const stripRef = useRef<HTMLDivElement | null>(null)
  const [overflowing, setOverflowing] = useState(false)

  useEffect(() => {
    const strip = stripRef.current
    if (!strip) return
    const syncLayout = () => {
      setOverflowing(strip.scrollWidth > strip.clientWidth + 1)
      revealActiveTab(strip)
    }
    syncLayout()
    const resizeObserver = new ResizeObserver(syncLayout)
    resizeObserver.observe(strip)
    const mutationObserver = new MutationObserver(syncLayout)
    mutationObserver.observe(strip, { childList: true, subtree: true, characterData: true })
    return () => {
      resizeObserver.disconnect()
      mutationObserver.disconnect()
    }
  }, [])

  useLayoutEffect(() => {
    const strip = stripRef.current
    if (strip) revealActiveTab(strip)
  }, [overflowing, value])

  return (
    <div
      className={cn(
        'nova-sidebar flex h-9 shrink-0 items-stretch border-b border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs',
        className,
      )}
      {...props}
    >
      <Tabs value={value} onValueChange={onValueChange} className="h-full min-w-0 flex-1 gap-0">
        <TabsList
          ref={stripRef}
          className="!h-full w-full justify-start gap-0 overflow-x-auto overflow-y-hidden overscroll-x-contain rounded-none bg-transparent p-0 [&::-webkit-scrollbar]:hidden"
          style={{ scrollbarWidth: 'none' }}
        >
          {children}
          {!overflowing ? flowAction : null}
        </TabsList>
      </Tabs>
      {overflowing ? flowAction : null}
      {endActions ? (
        <div className="flex shrink-0 items-center gap-1 border-l border-[var(--nova-border)] px-2">
          {endActions}
        </div>
      ) : null}
    </div>
  )
}

interface WorkbenchTabProps extends Omit<ComponentProps<typeof TabsTrigger>, 'children'> {
  label: string
  icon?: ReactNode
  trailing?: ReactNode
  indicatorClassName?: string
}

/** Standard visual and overflow-title treatment for a workbench document tab. */
export function WorkbenchTab({
  label,
  icon,
  trailing,
  indicatorClassName,
  className,
  ...props
}: WorkbenchTabProps) {
  return (
    <TabsTrigger className={cn(WORKBENCH_TAB_CLASS, className)} {...props}>
      <span
        aria-hidden="true"
        className={cn(
          'pointer-events-none absolute inset-x-0 top-0 h-0.5 bg-[var(--nova-accent)] opacity-0 group-aria-[selected=true]/tab:opacity-100',
          indicatorClassName,
        )}
      />
      {icon ? <span className="shrink-0 text-[var(--nova-text-faint)]">{icon}</span> : null}
      <OverflowTabTitle label={label} />
      {trailing}
    </TabsTrigger>
  )
}

/** Shared compact add control for workbench tab strips. */
export function WorkbenchTabAddButton({ className, children, ...props }: ComponentProps<typeof Button>) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-xs"
      className={cn(
        'mx-1 h-7 w-8 shrink-0 rounded-lg border border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] hover:bg-[var(--nova-hover)]',
        className,
      )}
      {...props}
    >
      {children ?? <Plus className="size-4" />}
    </Button>
  )
}

/** Shows the full label only when this rendered title is actually clipped by its tab. */
function OverflowTabTitle({ label }: { label: string }) {
  const titleRef = useRef<HTMLSpanElement | null>(null)
  const [truncated, setTruncated] = useState(false)
  const [tooltipOpen, setTooltipOpen] = useState(false)

  useLayoutEffect(() => {
    const title = titleRef.current
    if (!title) return
    const measure = () => {
      const next = title.scrollWidth > title.clientWidth + 1
      setTruncated((current) => current === next ? current : next)
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(title)
    return () => observer.disconnect()
  }, [label])

  return (
    <Tooltip
      open={truncated && tooltipOpen}
      onOpenChange={(open) => setTooltipOpen(truncated && open)}
      delayDuration={TAB_TITLE_TOOLTIP_DELAY_MS}
    >
      <TooltipTrigger asChild>
        <span ref={titleRef} className="min-w-0 flex-1 truncate text-left">{label}</span>
      </TooltipTrigger>
      {truncated ? (
        <TooltipContent side="bottom" sideOffset={6} className="max-w-80 break-words">
          {label}
        </TooltipContent>
      ) : null}
    </Tooltip>
  )
}
