import { useEffect, useRef, useState, type ReactNode } from 'react'
import { RotateCcw, X, ZoomIn, ZoomOut } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from '@/components/ui/dialog'

const MIN_ZOOM = 0.25
const MAX_ZOOM = 5
const ZOOM_STEP = 0.25
const PINCH_ZOOM_SENSITIVITY = 0.0012

interface ImagePreviewDialogProps {
  src: string
  title: string
  alt?: string
  path?: string
  children: ReactNode
}

export function ImagePreviewDialog({ src, title, alt, children }: ImagePreviewDialogProps) {
  const { t } = useTranslation()
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const zoomRef = useRef(1)
  const [open, setOpen] = useState(false)
  const [zoom, setZoom] = useState(1)
  const description = alt || title
  const zoomLabel = `${Math.round(zoom * 100)}%`
  const updateZoom = (delta: number) => {
    setZoom((value) => {
      const next = clampZoom(Math.round((value + delta) * 100) / 100)
      zoomRef.current = next
      return next
    })
  }
  const zoomIn = () => updateZoom(ZOOM_STEP)
  const zoomOut = () => updateZoom(-ZOOM_STEP)
  const resetZoom = () => {
    zoomRef.current = 1
    setZoom(1)
  }
  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (nextOpen) resetZoom()
  }

  useEffect(() => {
    zoomRef.current = zoom
  }, [zoom])

  useEffect(() => {
    if (!open) return undefined

    const handlePinchWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return
      event.preventDefault()
      zoomAtClientPoint(event.deltaY, event.clientX, event.clientY)
    }

    window.addEventListener('wheel', handlePinchWheel, { capture: true, passive: false })
    return () => {
      window.removeEventListener('wheel', handlePinchWheel, { capture: true })
    }
  }, [open])

  const zoomAtClientPoint = (deltaY: number, clientX: number, clientY: number) => {
    const viewport = viewportRef.current
    if (!viewport) return

    const rect = viewport.getBoundingClientRect()
    if (clientX < rect.left || clientX > rect.right || clientY < rect.top || clientY > rect.bottom) return

    const pointerX = clientX - rect.left
    const pointerY = clientY - rect.top
    const previousZoom = zoomRef.current
    const nextZoom = clampZoom(previousZoom * Math.exp(-deltaY * PINCH_ZOOM_SENSITIVITY))
    const roundedNextZoom = Math.round(nextZoom * 100) / 100
    if (roundedNextZoom === previousZoom) return

    const contentX = (viewport.scrollLeft + pointerX) / previousZoom
    const contentY = (viewport.scrollTop + pointerY) / previousZoom
    zoomRef.current = roundedNextZoom
    setZoom(roundedNextZoom)
    window.requestAnimationFrame(() => {
      viewport.scrollLeft = Math.max(0, contentX * roundedNextZoom - pointerX)
      viewport.scrollTop = Math.max(0, contentY * roundedNextZoom - pointerY)
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent
        showCloseButton={false}
        className="overflow-hidden rounded-xl border border-white/10 bg-black p-0 text-white shadow-2xl"
        style={{
          top: 'clamp(1rem, 2.5vw, 2rem)',
          left: 'clamp(1rem, 2.5vw, 2rem)',
          width: 'calc(100vw - clamp(1rem, 2.5vw, 2rem) * 2)',
          height: 'calc(100vh - clamp(1rem, 2.5vw, 2rem) * 2)',
          maxWidth: 'none',
          transform: 'none',
          translate: 'none',
        }}
      >
        <DialogTitle className="sr-only">{title}</DialogTitle>
        <DialogDescription className="sr-only">{description}</DialogDescription>
        <div className="absolute right-3 top-3 z-10 flex items-center gap-1 rounded-lg border border-white/15 bg-black/55 p-1 text-white shadow-lg backdrop-blur">
          <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white disabled:opacity-35" disabled={zoom <= MIN_ZOOM} onClick={zoomOut} aria-label={t('common.imageViewer.zoomOut')} title={t('common.imageViewer.zoomOut')}>
            <ZoomOut className="h-4 w-4" />
          </Button>
          <span className="min-w-12 select-none text-center font-mono text-[11px] text-white/80" aria-live="polite">{zoomLabel}</span>
          <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white disabled:opacity-35" disabled={zoom >= MAX_ZOOM} onClick={zoomIn} aria-label={t('common.imageViewer.zoomIn')} title={t('common.imageViewer.zoomIn')}>
            <ZoomIn className="h-4 w-4" />
          </Button>
          <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white disabled:opacity-35" disabled={zoom === 1} onClick={resetZoom} aria-label={t('common.imageViewer.resetZoom')} title={t('common.imageViewer.resetZoom')}>
            <RotateCcw className="h-4 w-4" />
          </Button>
          <DialogClose asChild>
            <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white" aria-label={t('common.close')} title={t('common.close')}>
              <X className="h-4 w-4" />
            </Button>
          </DialogClose>
        </div>
        <div ref={viewportRef} data-testid="image-preview-viewport" className="h-full w-full overflow-auto bg-black p-4 sm:p-8">
          <div
            className="flex min-h-full min-w-full items-center justify-center"
          >
            <div
              className="flex items-center justify-center"
              style={{
                width: `${zoom * 100}%`,
                height: `${zoom * 100}%`,
                minWidth: `${zoom * 100}%`,
                minHeight: `${zoom * 100}%`,
              }}
            >
            <img
              src={src}
              alt={alt || title}
              className="block h-full w-full rounded-lg object-contain transition-[width,height] duration-150"
            />
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function clampZoom(value: number) {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, value))
}
