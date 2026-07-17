import type { IDisposable, editor } from 'monaco-editor'

export interface ReviewEditorLayoutTarget {
  layout: (dimension: editor.IDimension) => void
}

/** Keeps a lazily mounted Monaco editor sized from its stable outer host. */
export function fitReviewEditorToHost(target: ReviewEditorLayoutTarget, host: HTMLElement): IDisposable {
  let disposed = false
  let frame: number | null = null

  const layout = () => {
    if (disposed) return
    const width = Math.floor(host.clientWidth)
    const height = Math.floor(host.clientHeight)
    if (width <= 0 || height <= 0) return
    target.layout({ width, height })
  }

  layout()
  frame = window.requestAnimationFrame(() => {
    frame = null
    layout()
  })
  const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(layout) : null
  observer?.observe(host)

  return {
    dispose: () => {
      disposed = true
      observer?.disconnect()
      if (frame !== null) window.cancelAnimationFrame(frame)
    },
  }
}

/** Focuses Monaco before its mouse handler runs so the browser cannot reveal the stale first-line input node. */
export function installReviewEditorPointerFocus(codeEditor: editor.IStandaloneCodeEditor): IDisposable {
  const getDomNode = codeEditor.getDomNode
  const domNode = typeof getDomNode === 'function' ? getDomNode.call(codeEditor) : null
  if (!domNode) return { dispose: () => {} }

  const handleMouseDown = (event: MouseEvent) => {
    if (event.button !== 0) return
    const eventTarget = event.target instanceof Element ? event.target : null
    if (eventTarget?.closest('.nova-review-zone-host, button, input, textarea, select, a[href], [role="button"]')) return
    if (codeEditor.hasTextFocus()) return
    const target = codeEditor.getTargetAtClientPoint(event.clientX, event.clientY)
    if (!target?.position) return

    codeEditor.setPosition(target.position, 'review-pointer')
    const editContext = domNode.querySelector<HTMLElement>('.native-edit-context, textarea.inputarea, textarea.ime-text-area')
    editContext?.focus({ preventScroll: true })
  }

  domNode.addEventListener('mousedown', handleMouseDown, true)
  return { dispose: () => domNode.removeEventListener('mousedown', handleMouseDown, true) }
}
