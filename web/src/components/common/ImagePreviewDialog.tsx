import { useState, type HTMLAttributes, type ReactNode, type WheelEvent as ReactWheelEvent } from 'react'
import { RotateCcw, X, ZoomIn, ZoomOut } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TransformComponent, TransformWrapper, type ReactZoomPanPinchHandlers, type ReactZoomPanPinchState } from 'react-zoom-pan-pinch'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from '@/components/ui/dialog'

const MIN_ZOOM = 0.25
const MAX_ZOOM = 5
const ZOOM_STEP = 0.25
const GESTURE_ZOOM_SPEED_MULTIPLIER = 4
const CONTROL_ZOOM_STEP = Math.log(1 + ZOOM_STEP)
const PROPORTIONAL_WHEEL_ZOOM_STEP = 0.001 * GESTURE_ZOOM_SPEED_MULTIPLIER
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
  const [open, setOpen] = useState(false)
  const [zoom, setZoom] = useState(1)
  const description = alt || title
  const zoomLabel = `${Math.round(zoom * 100)}%`
  const viewportProps: HTMLAttributes<HTMLDivElement> & { 'data-testid': string } = {
    'aria-label': description,
    'data-testid': 'image-preview-viewport',
  }
  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (nextOpen) setZoom(1)
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
        <TransformWrapper
          key={open ? src : 'closed'}
          initialScale={1}
          minScale={MIN_ZOOM}
          maxScale={MAX_ZOOM}
          centerOnInit
          centerZoomedOut
          smooth
          wheel={{ disabled: true }}
          panning={{ allowLeftClickPan: true, velocityDisabled: false }}
          trackPadPanning={{ disabled: false, velocityDisabled: false }}
          pinch={{ allowPanning: true }}
          doubleClick={{ mode: 'toggle', step: CONTROL_ZOOM_STEP, animationTime: CONTROL_ZOOM_ANIMATION_MS }}
          zoomAnimation={{ disabled: true }}
          onInit={(ref) => setZoom(roundZoom(ref.state.scale))}
          onTransform={(_, state) => setZoom(roundZoom(state.scale))}
        >
          {({ zoomIn, zoomOut, resetTransform, setTransform, state }) => (
            <>
              <div className="absolute right-3 top-3 z-10 flex items-center gap-1 rounded-lg border border-white/15 bg-black/55 p-1 text-white shadow-lg backdrop-blur">
                <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white disabled:opacity-35" disabled={zoom <= MIN_ZOOM} onClick={() => zoomOut(CONTROL_ZOOM_STEP, CONTROL_ZOOM_ANIMATION_MS)} aria-label={t('common.imageViewer.zoomOut')} title={t('common.imageViewer.zoomOut')}>
                  <ZoomOut className="h-4 w-4" />
                </Button>
                <span className="min-w-12 select-none text-center font-mono text-[11px] text-white/80" aria-live="polite">{zoomLabel}</span>
                <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white disabled:opacity-35" disabled={zoom >= MAX_ZOOM} onClick={() => zoomIn(CONTROL_ZOOM_STEP, CONTROL_ZOOM_ANIMATION_MS)} aria-label={t('common.imageViewer.zoomIn')} title={t('common.imageViewer.zoomIn')}>
                  <ZoomIn className="h-4 w-4" />
                </Button>
                <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white disabled:opacity-35" disabled={zoom === 1} onClick={() => resetTransform(CONTROL_ZOOM_ANIMATION_MS)} aria-label={t('common.imageViewer.resetZoom')} title={t('common.imageViewer.resetZoom')}>
                  <RotateCcw className="h-4 w-4" />
                </Button>
                <DialogClose asChild>
                  <Button type="button" variant="ghost" size="icon-sm" className="text-white hover:bg-white/15 hover:text-white" aria-label={t('common.close')} title={t('common.close')}>
                    <X className="h-4 w-4" />
                  </Button>
                </DialogClose>
              </div>
              <TransformComponent
                wrapperClass="h-full w-full cursor-grab bg-black p-4 active:cursor-grabbing sm:p-8"
                contentClass="h-full w-full items-center justify-center"
                wrapperStyle={{ width: '100%', height: '100%' }}
                contentStyle={{ width: '100%', height: '100%' }}
                wrapperProps={{
                  ...viewportProps,
                  onWheel: (event) => handleProportionalWheelZoom(event, state, setTransform),
                }}
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

function roundZoom(value: number) {
  return Math.round(value * 100) / 100
}

function handleProportionalWheelZoom(
  event: ReactWheelEvent<HTMLDivElement>,
  state: ReactZoomPanPinchState,
  setTransform: ReactZoomPanPinchHandlers['setTransform'],
) {
  if (!event.ctrlKey && !event.metaKey) return

  event.preventDefault()
  event.stopPropagation()

  const wrapper = event.currentTarget
  const content = wrapper.querySelector('.react-transform-component')
  if (!(content instanceof HTMLElement)) return

  const previousScale = state.scale
  const nextScale = clampZoom(previousScale * Math.exp(-event.deltaY * PROPORTIONAL_WHEEL_ZOOM_STEP))
  if (roundZoom(nextScale) === roundZoom(previousScale)) return

  const contentRect = content.getBoundingClientRect()
  const pointerX = (event.clientX - contentRect.left) / previousScale
  const pointerY = (event.clientY - contentRect.top) / previousScale
  const nextX = state.positionX - pointerX * (nextScale - previousScale)
  const nextY = state.positionY - pointerY * (nextScale - previousScale)
  setTransform(nextX, nextY, nextScale, CONTROL_ZOOM_ANIMATION_MS)
}

function clampZoom(value: number) {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, value))
}
