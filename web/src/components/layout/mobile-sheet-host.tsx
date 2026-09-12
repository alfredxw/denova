import { useEffect, useRef, useState } from 'react'
import { closeMobilePanes, MOBILE_PANES_CLOSE_EVENT } from './mobile-pane-events'
import { ChevronLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import type { MobilePaneControls, MobilePaneHostProps } from './mobile-pane-host'
import { useMobileDrawerSwipe } from './use-mobile-drawer-swipe'

/** Touch panes share modal focus, dismissal, and opt-in edge swipe behavior. */
export function MobileSheetHost({ panes, closeLabel, children, className, openPaneId: controlledId, onOpenPaneChange }: MobilePaneHostProps) {
  const backRef = useRef<HTMLButtonElement>(null)
  const surfaceRef = useRef<HTMLDivElement>(null)
  const drawerRef = useRef<HTMLDivElement>(null)
  const returnFocusRef = useRef<HTMLElement | null>(null)
  const [internalId, setInternalId] = useState<string | null>(null)
  const openId = controlledId === undefined ? internalId : controlledId
  const pane = panes.find((item) => item.id === openId)
  const lastPane = useRef(pane)
  if (pane) lastPane.current = pane
  const renderedPane = pane ?? lastPane.current
  const changePane = (id: string | null) => {
    if (id === openId) return
    if (id && !openId) {
      closeMobilePanes()
      returnFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    }
    pane?.onClose?.()
    if (controlledId === undefined) setInternalId(id)
    onOpenPaneChange?.(id)
    if (id) panes.find((item) => item.id === id)?.onOpen?.()
  }
  const changeRef = useRef(changePane)
  changeRef.current = changePane
  useEffect(() => {
    const close = () => changeRef.current(null)
    window.addEventListener(MOBILE_PANES_CLOSE_EVENT, close)
    return () => window.removeEventListener(MOBILE_PANES_CLOSE_EVENT, close)
  }, [])
  useEffect(() => {
    if (openId && !pane) changeRef.current(null)
  }, [openId, pane])
  const controls: MobilePaneControls = {
    openPaneId: pane?.id ?? null,
    openPane: (id) => { if (panes.some((item) => item.id === id)) changePane(id) },
    closePane: () => changePane(null),
    togglePane: (id) => changePane(openId === id ? null : id),
  }
  useMobileDrawerSwipe({
    surface: surfaceRef, drawer: drawerRef, openSide: pane?.side,
    sides: panes.filter((item) => item.swipeToOpen).map((item) => item.side),
    onOpen: (side) => { const target = panes.find((item) => item.side === side && item.swipeToOpen); if (target) changePane(target.id) },
    onClose: controls.closePane,
  })
  return (
    <div ref={surfaceRef} className={className ?? 'relative h-full min-h-0'} data-nova-mobile-pane-host="true">
      {typeof children === 'function' ? children(controls) : children}
      <Sheet open={Boolean(pane)} onOpenChange={(open) => { if (!open) changePane(null) }}>
        <SheetContent
          ref={drawerRef}
          side={renderedPane?.side ?? 'left'}
          showCloseButton={false}
          aria-describedby={undefined}
          data-nova-mobile-pane-content="true"
          className="nova-mobile-sheet gap-0 p-0 text-foreground"
          onOpenAutoFocus={(event) => {
            // Opening a file or settings pane must not summon the soft keyboard.
            event.preventDefault()
            backRef.current?.focus()
          }}
          onCloseAutoFocus={(event) => {
            // Controls live outside SheetTrigger, so restore the original keyboard focus explicitly.
            event.preventDefault()
            const target = returnFocusRef.current
            returnFocusRef.current = null
            if (document.activeElement?.closest('[role="dialog"][data-state="open"]')) return
            if (target?.isConnected && !target.closest('[inert], [hidden]') && target.getClientRects().length) {
              target.focus({ preventScroll: true })
            }
          }}
        >
          <header data-mobile-drawer-header className="nova-topbar flex min-h-12 shrink-0 items-center gap-2 border-b px-2">
            <Button ref={backRef} data-pane-back type="button" variant="ghost" size="icon" aria-label={closeLabel} onClick={controls.closePane}>
              <ChevronLeft />
            </Button>
            <SheetTitle className="min-w-0 truncate text-sm">{renderedPane?.title}</SheetTitle>
          </header>
          <div className="min-h-0 flex-1 overflow-hidden">{renderedPane?.content}</div>
        </SheetContent>
      </Sheet>
    </div>
  )
}
