import * as React from "react"

import { cn } from "@/lib/utils"

type TextareaProps = React.ComponentProps<"textarea"> & {
  autoResize?: boolean
  minRows?: number
  maxRows?: number
  multilineMode?: "auto" | "sticky-until-empty" | "always"
}

const DEFAULT_MAX_ROWS = 10
let textMeasureCanvas: HTMLCanvasElement | null = null

function Textarea({
  className,
  autoResize = true,
  minRows = 1,
  maxRows = DEFAULT_MAX_ROWS,
  multilineMode = "auto",
  onInput,
  ref: forwardedRef,
  ...props
}: TextareaProps) {
  const textareaRef = React.useRef<HTMLTextAreaElement | null>(null)
  const multilineRef = React.useRef(false)
  const compactTextWidthRef = React.useRef(0)
  const [multiline, setMultiline] = React.useState(false)

  const syncHeight = React.useCallback(() => {
    const textarea = textareaRef.current
    if (!textarea || !autoResize) return

    const computed = window.getComputedStyle(textarea)
    const lineHeight = parseCssPixels(computed.lineHeight) || 20
    const paddingTop = parseCssPixels(computed.paddingTop)
    const paddingBottom = parseCssPixels(computed.paddingBottom)
    const borderTop = parseCssPixels(computed.borderTopWidth)
    const borderBottom = parseCssPixels(computed.borderBottomWidth)
    const minHeight = parseCssPixels(computed.minHeight)
    const verticalChrome = paddingTop + paddingBottom + borderTop + borderBottom
    const minRowCount = Math.max(1, minRows)
    const maxRowCount = Math.max(minRowCount, maxRows)
    const oneRowHeight = Math.ceil(Math.max(lineHeight + verticalChrome, minHeight))
    const minRowsHeight = Math.ceil(Math.max(minRowCount * lineHeight + verticalChrome, minHeight))
    const cappedHeight = Math.ceil(maxRowCount * lineHeight + verticalChrome)
    const previousScrollTop = textarea.scrollTop
    const compactTextWidth = resolveCompactTextWidth(textarea, computed)
    if (!multilineRef.current || compactTextWidthRef.current <= 0) {
      compactTextWidthRef.current = compactTextWidth
    }
    const textWidthLimit = compactTextWidthRef.current || compactTextWidth

    textarea.style.height = "auto"

    const hasValue = textarea.value.length > 0
    const measuredHeight = hasValue ? textarea.scrollHeight : minRowsHeight
    const nextHeight = Math.max(minRowsHeight, Math.min(measuredHeight, cappedHeight))
    const overflows = hasValue && measuredHeight > cappedHeight
    const wrappedByHeight = measuredHeight > oneRowHeight
    const wrappedByWidth = textWidthLimit > 0 && measureLongestLineWidth(textarea.value, computed) > textWidthLimit
    const wrapped = hasValue && (wrappedByHeight || wrappedByWidth)

    textarea.style.height = `${nextHeight}px`
    textarea.style.overflowY = overflows ? "auto" : "hidden"
    if (overflows) textarea.scrollTop = previousScrollTop

    setMultiline((current) => {
      const next = multilineMode === "always"
        ? true
        : hasValue && (multilineMode === "sticky-until-empty" && current ? true : wrapped)
      multilineRef.current = next
      return next
    })
  }, [autoResize, minRows, maxRows, multilineMode])

  React.useLayoutEffect(() => {
    syncHeight()
  }, [props.value, syncHeight])

  return (
    <textarea
      {...props}
      ref={(node) => {
        textareaRef.current = node
        if (typeof forwardedRef === "function") {
          forwardedRef(node)
        } else if (forwardedRef && typeof forwardedRef === "object") {
          ;(forwardedRef as React.MutableRefObject<HTMLTextAreaElement | null>).current = node
        }
      }}
      data-slot="textarea"
      data-nova-multiline={multiline ? "true" : undefined}
      className={cn(
        "flex field-sizing-content min-h-16 w-full resize-none rounded-2xl border border-transparent bg-input/50 px-2.5 py-2 text-base transition-[color,box-shadow] duration-200 outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40",
        className
      )}
      onInput={(event) => {
        onInput?.(event)
        syncHeight()
      }}
    />
  )
}

export { Textarea }

function parseCssPixels(value: string) {
  const parsed = Number.parseFloat(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function resolveCompactTextWidth(textarea: HTMLTextAreaElement, computed: CSSStyleDeclaration) {
  const paddingLeft = parseCssPixels(computed.paddingLeft)
  const paddingRight = parseCssPixels(computed.paddingRight)
  const composerWidth = resolveComposerCompactInputWidth(textarea)
  const fallbackWidth = textarea.clientWidth || parseCssPixels(computed.width)
  return Math.max(0, (composerWidth || fallbackWidth) - paddingLeft - paddingRight)
}

function resolveComposerCompactInputWidth(textarea: HTMLTextAreaElement) {
  const toolbar = textarea.closest<HTMLElement>(".nova-agent-composer-toolbar")
  if (!toolbar) return 0

  const start = toolbar.querySelector<HTMLElement>('[data-slot="agent-composer-start"]')
  const end = toolbar.querySelector<HTMLElement>('[data-slot="agent-composer-end"]')
  const toolbarStyle = window.getComputedStyle(toolbar)
  const gap = parseCssPixels(toolbarStyle.columnGap || toolbarStyle.gap)
  const toolbarWidth = toolbar.getBoundingClientRect().width || toolbar.clientWidth
  const startWidth = start?.getBoundingClientRect().width || 0
  const endWidth = end?.getBoundingClientRect().width || 0
  return Math.max(0, toolbarWidth - startWidth - endWidth - gap * 2)
}

function measureLongestLineWidth(value: string, computed: CSSStyleDeclaration) {
  if (!value) return 0
  const canvas = textMeasureCanvas ?? (textMeasureCanvas = document.createElement("canvas"))
  const context = canvas.getContext("2d")
  if (!context) return 0

  context.font = computed.font || `${computed.fontStyle || "normal"} ${computed.fontVariant || "normal"} ${computed.fontWeight || "400"} ${computed.fontSize || "16px"} ${computed.fontFamily || "sans-serif"}`
  return value
    .split(/\r\n|\r|\n/)
    .reduce((maxWidth, line) => Math.max(maxWidth, context.measureText(line).width), 0)
}
