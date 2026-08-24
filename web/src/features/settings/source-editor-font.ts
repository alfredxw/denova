import { fontStackFor } from './font-options'

export const DEFAULT_SOURCE_EDITOR_FONT = 'mono'

type SourceEditorFontListener = () => void

const listeners = new Set<SourceEditorFontListener>()
let currentFontFamily = fontStackFor(DEFAULT_SOURCE_EDITOR_FONT, DEFAULT_SOURCE_EDITOR_FONT)

/** Returns the literal stack Monaco needs for reliable glyph measurement. */
export function getSourceEditorFontFamily() {
  return currentFontFamily
}

/** Subscribes mounted Monaco editors so a saved setting triggers updateOptions. */
export function subscribeSourceEditorFont(listener: SourceEditorFontListener) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** Resolves one persisted preset or local family and publishes only real changes. */
export function setSourceEditorFont(font?: string | null) {
  const nextFontFamily = fontStackFor(font, DEFAULT_SOURCE_EDITOR_FONT)
  if (nextFontFamily === currentFontFamily) return
  currentFontFamily = nextFontFamily
  listeners.forEach((listener) => listener())
}
