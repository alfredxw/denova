let textMeasureCanvas: HTMLCanvasElement | null = null

export function parseCssPixels(value: string): number {
  const parsed = Number.parseFloat(value)
  return Number.isFinite(parsed) ? parsed : 0
}

/** Width available to compact composer text after accounting for start/end tools. */
export function resolveCompactTextWidth(element: HTMLElement, computed: CSSStyleDeclaration): number {
  const paddingLeft = parseCssPixels(computed.paddingLeft)
  const paddingRight = parseCssPixels(computed.paddingRight)
  const composerWidth = resolveComposerCompactInputWidth(element)
  const fallbackWidth = element.clientWidth || parseCssPixels(computed.width)
  return Math.max(0, (composerWidth || fallbackWidth) - paddingLeft - paddingRight)
}

export function measureLongestLineWidth(value: string, computed: CSSStyleDeclaration): number {
  if (!value) return 0
  const canvas = textMeasureCanvas ?? (textMeasureCanvas = document.createElement('canvas'))
  const context = canvas.getContext('2d')
  if (!context) return 0

  context.font = computed.font || `${computed.fontStyle || 'normal'} ${computed.fontVariant || 'normal'} ${computed.fontWeight || '400'} ${computed.fontSize || '16px'} ${computed.fontFamily || 'sans-serif'}`
  return value
    .split(/\r\n|\r|\n/)
    .reduce((maxWidth, line) => Math.max(maxWidth, context.measureText(line).width), 0)
}

function resolveComposerCompactInputWidth(element: HTMLElement): number {
  const toolbar = element.closest<HTMLElement>('.nova-agent-composer-toolbar')
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
