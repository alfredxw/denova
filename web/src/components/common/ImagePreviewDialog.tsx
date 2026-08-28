import { useRef, useState, type HTMLAttributes, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react'
import { RotateCcw, X, ZoomIn, ZoomOut } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TransformComponent, TransformWrapper, type ReactZoomPanPinchRef } from 'react-zoom-pan-pinch'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from '@/components/ui/dialog'

const MIN_ZOOM = 0.25
const MAX_ZOOM = 5
const ZOOM_STEP = 0.25
const CONTROL_ZOOM_STEP = Math.log(1 + ZOOM_STEP)
const DOUBLE_CLICK_ZOOM_STEP = Math.log(2)
const WHEEL_ZOOM_STEP = 0.002
const KEYBOARD_PAN_STEP = 80
const CONTROL_ZOOM_ANIMATION_MS = 0

interface ImagePreviewDialogProps {
  src: string
  title: string
  alt?: string
  path?: string
  children: ReactNode
}

export function ImagePreviewDialog({ src, title, alt, children }: ImagePreviewDialogProps) {
  const { t } = useTranslation()
  const transformRef = useRef<ReactZoomPanPinchRef>(null)
  const [open, setOpen] = useState(false)
  const [zoom, setZoom] = useState(1)
  const description = alt || title
  const zoomLabel = `${Math.round(zoom * 100)}%`
  const viewportProps: HTMLAttributes<HTMLDivElement> & { 'data-testid': string } = {
    'aria-label': description,
    'data-testid': 'image-preview-viewport',
    role: 'region',
    tabIndex: 0,
  }
  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (nextOpen) setZoom(1)
  }
  const handleKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.altKey || event.ctrlKey || event.metaKey) return

    const transform = transformRef.current
    if (!transform) return

    switch (event.key) {
      case '+':
      case '=':
        transform.zoomIn(CONTROL_ZOOM_STEP, CONTROL_ZOOM_ANIMATION_MS)
        break
      case '-':
        transform.zoomOut(CONTROL_ZOOM_STEP, CONTROL_ZOOM_ANIMATION_MS)
        break
      case '0':
        transform.resetTransform(CONTROL_ZOOM_ANIMATION_MS)
        break
      case 'ArrowLeft':
        panBy(transform, KEYBOARD_PAN_STEP, 0)
        break
      case 'ArrowRight':
        panBy(transform, -KEYBOARD_PAN_STEP, 0)
        break
      case 'ArrowUp':
        panBy(transform, 0, KEYBOARD_PAN_STEP)
        break
      case 'ArrowDown':
        panBy(transform, 0, -KEYBOARD_PAN_STEP)
        break
      default:
        return
    }

    event.preventDefault()
    event.stopPropagation()
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent
        showCloseButton={false}
        onKeyDown={handleKeyDown}
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
        <DialogDescription className="sr-only">{description} {t('common.imageViewer.instructions')}</DialogDescription>
        <TransformWrapper
          ref={transformRef}
          key={open ? src : 'closed'}
          initialScale={1}
          minScale={MIN_ZOOM}
          maxScale={MAX_ZOOM}
          centerOnInit
          centerZoomedOut
          smooth
          wheel={{ step: WHEEL_ZOOM_STEP }}
          panning={{ allowLeftClickPan: true, allowMiddleClickPan: false, allowRightClickPan: false, velocityDisabled: false }}
          pinch={{ allowPanning: true }}
          doubleClick={{ mode: 'toggle', step: DOUBLE_CLICK_ZOOM_STEP, animationTime: CONTROL_ZOOM_ANIMATION_MS }}
          zoomAnimation={{ disabled: true }}
          onInit={(ref) => setZoom(roundZoom(ref.state.scale))}
          onTransform={(_, state) => setZoom(roundZoom(state.scale))}
        >
          {({ zoomIn, zoomOut, resetTransform }) => (
            <>
              <div className="absolute right-3 top-3 z-10 flex items-center gap-1 rounded-lg border border-white/15 bg-black/55 p-1 text-white shadow-lg backdrop-blur">
                <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white disabled:opacity-35" disabled={zoom <= MIN_ZOOM} onClick={() => zoomOut(CONTROL_ZOOM_STEP, CONTROL_ZOOM_ANIMATION_MS)} aria-label={t('common.imageViewer.zoomOut')} title={`${t('common.imageViewer.zoomOut')} (-)`}>
                  <ZoomOut className="h-4 w-4" />
                </Button>
                <span className="min-w-12 select-none text-center font-mono text-[11px] text-white/80" aria-live="polite">{zoomLabel}</span>
                <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white disabled:opacity-35" disabled={zoom >= MAX_ZOOM} onClick={() => zoomIn(CONTROL_ZOOM_STEP, CONTROL_ZOOM_ANIMATION_MS)} aria-label={t('common.imageViewer.zoomIn')} title={`${t('common.imageViewer.zoomIn')} (+)`}>
                  <ZoomIn className="h-4 w-4" />
                </Button>
                <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white disabled:opacity-35" disabled={zoom === 1} onClick={() => resetTransform(CONTROL_ZOOM_ANIMATION_MS)} aria-label={t('common.imageViewer.resetZoom')} title={`${t('common.imageViewer.resetZoom')} (0)`}>
                  <RotateCcw className="h-4 w-4" />
                </Button>
                <DialogClose asChild>
                  <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white" aria-label={t('common.close')} title={`${t('common.close')} (Esc)`}>
                    <X className="h-4 w-4" />
                  </Button>
                </DialogClose>
              </div>
              <TransformComponent
                wrapperClass="h-full w-full cursor-grab bg-black p-4 active:cursor-grabbing sm:p-8"
                contentClass="h-full w-full items-center justify-center"
                wrapperStyle={{ width: '100%', height: '100%' }}
                contentStyle={{ width: '100%', height: '100%' }}
                wrapperProps={viewportProps}
              >
                <img
                  src={src}
                  alt={alt || title}
                  draggable={false}
                  className="block max-h-full max-w-full rounded-lg object-contain shadow-2xl"
                />
              </TransformComponent>
            </>
          )}
        </TransformWrapper>
      </DialogContent>
    </Dialog>
  )
}

function roundZoom(value: number): number {
  return Math.round(value * 100) / 100
}

function panBy(transform: ReactZoomPanPinchRef, deltaX: number, deltaY: number): void {
  const bounds = transform.instance.bounds
  if (!bounds) return

  const { positionX, positionY, scale } = transform.state
  const nextX = clamp(positionX + deltaX, bounds.minPositionX, bounds.maxPositionX)
  const nextY = clamp(positionY + deltaY, bounds.minPositionY, bounds.maxPositionY)
  transform.setTransform(nextX, nextY, scale, CONTROL_ZOOM_ANIMATION_MS)
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}
