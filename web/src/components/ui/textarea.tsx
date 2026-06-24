import * as React from "react"

import { cn } from "@/lib/utils"
import { preserveNativeTextEditingShortcut } from "@/lib/keyboard"

type TextareaProps = React.ComponentProps<"textarea"> & {
  autoResize?: boolean
  maxRows?: number
  multilineMode?: 'auto' | 'sticky-until-empty'
}

const DEFAULT_MAX_ROWS = 10
let measureCanvas: HTMLCanvasElement | null = null

function getMeasureContext() {
  if (!measureCanvas) measureCanvas = document.createElement('canvas')
  return measureCanvas.getContext('2d')
}

function textExceedsSingleVisualRow(el: HTMLTextAreaElement, computed: CSSStyleDeclaration, singleLineHeight: number) {
  if (el.value.includes('\n')) return true

  const paddingLeft = Number.parseFloat(computed.paddingLeft) || 0
  const paddingRight = Number.parseFloat(computed.paddingRight) || 0
  const availableWidth = el.clientWidth - paddingLeft - paddingRight
  if (availableWidth <= 0) return el.scrollHeight > singleLineHeight + 1

  const context = getMeasureContext()
  if (!context) return el.scrollHeight > singleLineHeight + 1

  context.font = computed.font || `${computed.fontStyle} ${computed.fontVariant} ${computed.fontWeight} ${computed.fontSize} ${computed.fontFamily}`
  const letterSpacing = Number.parseFloat(computed.letterSpacing) || 0
  const measuredWidth = context.measureText(el.value).width + Math.max(0, el.value.length - 1) * letterSpacing
  return measuredWidth > availableWidth + 1
}

function resizeTextarea(el: HTMLTextAreaElement, maxRows: number, multilineMode: NonNullable<TextareaProps['multilineMode']>) {
  const computed = window.getComputedStyle(el)
  const lineHeight = Number.parseFloat(computed.lineHeight) || 20
  const paddingTop = Number.parseFloat(computed.paddingTop) || 0
  const paddingBottom = Number.parseFloat(computed.paddingBottom) || 0
  const borderTop = Number.parseFloat(computed.borderTopWidth) || 0
  const borderBottom = Number.parseFloat(computed.borderBottomWidth) || 0
  const singleLineHeight = lineHeight + paddingTop + paddingBottom
  const singleLineOuterHeight = singleLineHeight + borderTop + borderBottom
  const maxHeight = lineHeight * maxRows + paddingTop + paddingBottom + borderTop + borderBottom
  el.style.height = 'auto'

  if (!el.value) {
    const wasMultiline = el.dataset.novaMultiline === 'true'
    el.style.height = `${singleLineOuterHeight}px`
    el.style.overflowY = 'hidden'
    delete el.dataset.novaMultiline
    return wasMultiline
  }

  const nextHeight = Math.min(Math.max(el.scrollHeight, singleLineOuterHeight), maxHeight)
  el.style.height = `${nextHeight}px`
  el.style.overflowY = el.scrollHeight > maxHeight ? 'auto' : 'hidden'

  const hasMultilineInput = textExceedsSingleVisualRow(el, computed, singleLineHeight)
  const keepMultiline = multilineMode === 'sticky-until-empty' && Boolean(el.value) && el.dataset.novaMultiline === 'true'
  const wasMultiline = el.dataset.novaMultiline === 'true'
  let layoutChanged = false
  if (hasMultilineInput || keepMultiline) {
    el.dataset.novaMultiline = 'true'
    layoutChanged = !wasMultiline
  } else {
    delete el.dataset.novaMultiline
    layoutChanged = wasMultiline
  }
  return layoutChanged
}

function Textarea({ className, onInput, onKeyDownCapture, autoResize = false, maxRows = DEFAULT_MAX_ROWS, multilineMode = 'auto', ref, ...props }: TextareaProps) {
  const innerRef = React.useRef<HTMLTextAreaElement | null>(null)
  const setRef = React.useCallback((node: HTMLTextAreaElement | null) => {
    innerRef.current = node
    if (typeof ref === 'function') {
      ref(node)
    } else if (ref && typeof ref === 'object') {
      ref.current = node
    }
  }, [ref])

  React.useLayoutEffect(() => {
    if (autoResize && innerRef.current) {
      const layoutChanged = resizeTextarea(innerRef.current, maxRows, multilineMode)
      if (layoutChanged) {
        window.requestAnimationFrame(() => {
          if (innerRef.current) resizeTextarea(innerRef.current, maxRows, multilineMode)
        })
      }
    }
  }, [autoResize, maxRows, multilineMode, props.value])

  return (
    <textarea
      ref={setRef}
      data-slot="textarea"
      className={cn(
        "flex field-sizing-content min-h-16 w-full rounded-md border border-input bg-transparent px-3 py-2 text-base shadow-xs transition-[color,box-shadow] outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 md:text-sm dark:bg-input/30 dark:aria-invalid:ring-destructive/40",
        className
      )}
      onInput={(event) => {
        if (autoResize) {
          const layoutChanged = resizeTextarea(event.currentTarget, maxRows, multilineMode)
          if (layoutChanged) {
            const target = event.currentTarget
            window.requestAnimationFrame(() => resizeTextarea(target, maxRows, multilineMode))
          }
        }
        onInput?.(event)
      }}
      onKeyDownCapture={(event) => {
        preserveNativeTextEditingShortcut(event)
        onKeyDownCapture?.(event)
      }}
      {...props}
    />
  )
}

export { Textarea }
