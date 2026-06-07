import * as React from "react"

import { cn } from "@/lib/utils"
import { preserveNativeTextEditingShortcut } from "@/lib/keyboard"

type TextareaProps = React.ComponentProps<"textarea"> & {
  autoResize?: boolean
  maxRows?: number
}

const DEFAULT_MAX_ROWS = 10

function resizeTextarea(el: HTMLTextAreaElement, maxRows: number) {
  const computed = window.getComputedStyle(el)
  const lineHeight = Number.parseFloat(computed.lineHeight) || 20
  const paddingTop = Number.parseFloat(computed.paddingTop) || 0
  const paddingBottom = Number.parseFloat(computed.paddingBottom) || 0
  const borderTop = Number.parseFloat(computed.borderTopWidth) || 0
  const borderBottom = Number.parseFloat(computed.borderBottomWidth) || 0
  const maxHeight = lineHeight * maxRows + paddingTop + paddingBottom + borderTop + borderBottom
  el.style.height = 'auto'
  const nextHeight = Math.min(el.scrollHeight, maxHeight)
  el.style.height = `${nextHeight}px`
  el.style.overflowY = el.scrollHeight > maxHeight ? 'auto' : 'hidden'
}

function Textarea({ className, onInput, onKeyDownCapture, autoResize = false, maxRows = DEFAULT_MAX_ROWS, ref, ...props }: TextareaProps) {
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
      resizeTextarea(innerRef.current, maxRows)
    }
  }, [autoResize, maxRows, props.value])

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
          resizeTextarea(event.currentTarget, maxRows)
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
