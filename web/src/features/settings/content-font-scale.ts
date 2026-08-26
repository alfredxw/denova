export interface ContentFontScale {
  sourceEditor: number
  terminal: number
}

type ContentFontScaleListener = () => void

const listeners = new Set<ContentFontScaleListener>()
let currentScale: ContentFontScale = {
  sourceEditor: 14,
  terminal: 12,
}

export function getContentFontScale(): ContentFontScale {
  return currentScale
}

export function subscribeContentFontScale(listener: ContentFontScaleListener) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** Publishes the content ramp used by canvas-backed editors and terminals. */
export function setContentFontScale(readingFontSize: number): ContentFontScale {
  const ratio = readingFontSize / 18
  const nextScale: ContentFontScale = {
    sourceEditor: roundPixels(14 * ratio),
    terminal: roundPixels(12 * ratio),
  }
  if (
    nextScale.sourceEditor === currentScale.sourceEditor
    && nextScale.terminal === currentScale.terminal
  ) return currentScale

  currentScale = nextScale
  listeners.forEach((listener) => listener())
  return currentScale
}

function roundPixels(value: number) {
  return Math.round(value * 10) / 10
}
