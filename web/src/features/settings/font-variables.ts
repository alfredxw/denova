import { fontStackFor } from './font-options'
import { getSourceEditorFontFamily, setSourceEditorFont } from './source-editor-font'
import type { Settings } from './types'

export interface FontSettingsInput {
  uiFont?: string | null
  uiFontSize?: number | null
  readingFont?: string | null
  readingFontSize?: number | null
  sourceEditorFont?: string | null
}

export function fontSettingsFromEffective(effective?: Settings | null): FontSettingsInput {
  return {
    uiFont: effective?.ui_font_family,
    uiFontSize: effective?.ui_font_size,
    readingFont: effective?.reading_font_family,
    readingFontSize: effective?.reading_font_size,
    sourceEditorFont: effective?.source_editor_font_family,
  }
}

export function applyFontSettings(settings: FontSettingsInput) {
  if (typeof document === 'undefined') return
  const baseSize = clampFontSize(settings.uiFontSize, 11, 16, 14)
  const smSize = clampFontSize(baseSize + 2, 12, 18, 14)
  const compactSize = clampFontSize(baseSize - 1, 10, 15, 11)
  const microSize = clampFontSize(baseSize - 2, 10, 14, 10)

  document.documentElement.style.setProperty('--nova-ui-font-family', fontStackFor(settings.uiFont, 'apple-system'))
  applyReadingTypographySettings(settings)
  setSourceEditorFont(settings.sourceEditorFont)
  document.documentElement.style.setProperty('--nova-source-editor-font-family', getSourceEditorFontFamily())
  document.documentElement.style.setProperty('--nova-ui-font-size', `${baseSize}px`)
  document.documentElement.style.setProperty('--nova-ui-line-height', `${baseSize + 6}px`)
  document.documentElement.style.setProperty('--nova-ui-sm-font-size', `${smSize}px`)
  document.documentElement.style.setProperty('--nova-ui-sm-line-height', `${smSize + 6}px`)
  document.documentElement.style.setProperty('--nova-ui-caption-size', `${compactSize}px`)
  document.documentElement.style.setProperty('--nova-ui-compact-font-size', `${compactSize}px`)
  document.documentElement.style.setProperty('--nova-ui-compact-line-height', `${compactSize + 5}px`)
  document.documentElement.style.setProperty('--nova-ui-micro-font-size', `${microSize}px`)
  document.documentElement.style.setProperty('--nova-ui-micro-line-height', `${microSize + 4}px`)
}

export function applyReadingTypographySettings(settings: Pick<FontSettingsInput, 'readingFont' | 'readingFontSize'>) {
  if (typeof document === 'undefined') return
  document.documentElement.style.setProperty('--nova-reading-font-family', fontStackFor(settings.readingFont, 'apple-system'))
  document.documentElement.style.setProperty('--nova-reading-font-size', `${clampFontSize(settings.readingFontSize, 14, 28, 18)}px`)
}

function clampFontSize(value: unknown, min: number, max: number, fallback: number) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, Math.round(numberValue)))
}
