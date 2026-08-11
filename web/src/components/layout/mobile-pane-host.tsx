import { useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { X } from 'lucide-react'

export interface MobilePane {
  id: string
  title: string
  side: 'left' | 'right'
  content: ReactNode
  icon?: ReactNode
  onOpen?: () => void
  onClose?: () => void
  className?: string
}

export interface MobilePaneControls {
  openPaneId: string | null
  openPane: (id: string) => void
  closePane: () => void
  togglePane: (id: string) => void
}

interface MobilePaneHostProps {
  panes: MobilePane[]
  closeLabel: string
  children: ReactNode | ((controls: MobilePaneControls) => ReactNode)
  className?: string
  openPaneId?: string | null
  onOpenPaneChange?: (id: string | null) => void
}

export function MobilePaneHost({
  panes,
  closeLabel,
  children,
  className = 'relative h-full min-h-0',
  openPaneId: controlledOpenPaneId,
  onOpenPaneChange,
}: MobilePaneHostProps) {
  const [internalOpenPaneId, setInternalOpenPaneId] = useState<string | null>(null)
  const openPaneId = controlledOpenPaneId === undefined ? internalOpenPaneId : controlledOpenPaneId
  const paneIds = useMemo(() => new Set(panes.map((pane) => pane.id)), [panes])
  const openPane = panes.find((pane) => pane.id === openPaneId) || null
  const latestOpenPaneRef = useRef<MobilePane | null>(openPane)
  latestOpenPaneRef.current = openPane

  const setOpenPaneId = (nextId: string | null) => {
    const current = latestOpenPaneRef.current
    if (current?.id === nextId) return
    current?.onClose?.()
    if (controlledOpenPaneId === undefined) setInternalOpenPaneId(nextId)
    onOpenPaneChange?.(nextId)
    if (nextId) panes.find((pane) => pane.id === nextId)?.onOpen?.()
  }

  const controls: MobilePaneControls = {
    openPaneId,
    openPane: (id) => {
      if (paneIds.has(id)) setOpenPaneId(id)
    },
    closePane: () => setOpenPaneId(null),
    togglePane: (id) => setOpenPaneId(openPaneId === id ? null : id),
  }

  useEffect(() => {
    if (openPaneId && !paneIds.has(openPaneId)) setOpenPaneId(null)
  }, [openPaneId, paneIds])

  // Allow external code (e.g. file selection in the project drawer) to close
  // all open panes by dispatching this event. Used for file-tree auto-close.
  useEffect(() => {
    const close = () => setOpenPaneId(null)
    window.addEventListener('nova:mobile-close-panes', close)
    return () => window.removeEventListener('nova:mobile-close-panes', close)
  }, [])

  return (
    <div className={className} data-nova-mobile-pane-host="true">
      {typeof children === 'function' ? children(controls) : children}
      {panes.map((pane) => (
        <MobileDrawer
          key={pane.id}
          pane={pane}
          open={pane.id === openPaneId}
          closeLabel={closeLabel}
          side={pane.side}
          onClose={() => setOpenPaneId(null)}
        />
      ))}
    </div>
  )
}

function MobileDrawer({
  pane,
  open,
  closeLabel,
  side,
  onClose,
}: {
  pane: MobilePane
  open: boolean
  closeLabel: string
  side: 'left' | 'right'
  onClose: () => void
}) {
  const titleId = `nova-mobile-pane-title-${pane.id}`
  const sideClassName = side === 'left'
    ? 'left-0 border-r'
    : 'right-0 border-l'

  return (
    <>
      {open && (
        <div
          aria-hidden="true"
          data-nova-mobile-pane-overlay="true"
          className="fixed inset-0 z-50 bg-black/50"
          onClick={onClose}
        />
      )}
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        hidden={!open}
        aria-hidden={!open}
        data-state={open ? 'open' : 'closed'}
        data-nova-mobile-pane-content="true"
        data-side={side}
        className={`fixed inset-y-0 z-50 flex w-[min(92vw,420px)] max-w-none flex-col gap-0 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)] sm:max-w-none ${sideClassName} ${pane.className || ''}`}
      >
        <div className="nova-topbar flex h-11 shrink-0 items-center justify-between border-b border-[var(--nova-border)] px-3">
          <h2 id={titleId} className="flex min-w-0 items-center gap-2 text-xs font-semibold text-[var(--nova-text)]">
            {pane.icon ? <span className="flex h-4 w-4 shrink-0 items-center justify-center text-[var(--nova-text-muted)]">{pane.icon}</span> : null}
            <span className="min-w-0 truncate">{pane.title}</span>
          </h2>
          <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={closeLabel} onClick={onClose}>
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-hidden">{pane.content}</div>
      </section>
    </>
  )
}
