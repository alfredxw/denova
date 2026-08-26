import { fontStackFor } from './font-options'
import { getSourceEditorFontFamily, setSourceEditorFont } from './source-editor-font'
import { setContentFontScale } from './content-font-scale'
import { DEFAULT_READING_FONT_SIZE, DEFAULT_UI_FONT_SIZE } from './font-size-steps'
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

  document.documentElement.style.setProperty('--nova-ui-font-family', fontStackFor(settings.uiFont, 'apple-system'))
  applyReadingTypographySettings(settings)
  setSourceEditorFont(settings.sourceEditorFont)
  document.documentElement.style.setProperty('--nova-source-editor-font-family', getSourceEditorFontFamily())
  applyUIFontSize(settings.uiFontSize)
}

export function applyUIFontSize(value: number | null | undefined) {
  if (typeof document === 'undefined') return
  const baseSize = clampFontSize(value, 11, 18, DEFAULT_UI_FONT_SIZE)
  const ratio = baseSize / DEFAULT_UI_FONT_SIZE
  const typeRamp = {
    nano: scaleTypeToken(10, 14, ratio),
    micro: scaleTypeToken(12, 16, ratio),
    compact: scaleTypeToken(13, 18, ratio),
    body: scaleTypeToken(14, 20, ratio),
    large: scaleTypeToken(16, 22, ratio),
    title: scaleTypeToken(18, 24, ratio),
    heading: scaleTypeToken(20, 28, ratio),
    display: scaleTypeToken(24, 32, ratio),
  }

  setTypeToken('--nova-ui-nano', typeRamp.nano)
  setTypeToken('--nova-ui-micro', typeRamp.micro)
  setTypeToken('--nova-ui-compact', typeRamp.compact)
  setTypeToken('--nova-ui-body', typeRamp.body)
  setTypeToken('--nova-ui-large', typeRamp.large)
  setTypeToken('--nova-ui-title', typeRamp.title)
  setTypeToken('--nova-ui-heading', typeRamp.heading)
  setTypeToken('--nova-ui-display', typeRamp.display)

  // Keep the established aliases while components move onto the semantic ramp.
  setCSSPixels('--nova-ui-font-size', typeRamp.body.size)
  setCSSPixels('--nova-ui-line-height', typeRamp.body.lineHeight)
  setCSSPixels('--nova-ui-sm-font-size', typeRamp.large.size)
  setCSSPixels('--nova-ui-sm-line-height', typeRamp.large.lineHeight)
  setCSSPixels('--nova-ui-compact-font-size', typeRamp.compact.size)
  setCSSPixels('--nova-ui-compact-line-height', typeRamp.compact.lineHeight)
  setCSSPixels('--nova-ui-micro-font-size', typeRamp.micro.size)
  setCSSPixels('--nova-ui-micro-line-height', typeRamp.micro.lineHeight)
}

export function applyReadingTypographySettings(settings: Pick<FontSettingsInput, 'readingFont' | 'readingFontSize'>) {
  if (typeof document === 'undefined') return
  const readingFontSize = clampFontSize(settings.readingFontSize, 14, 28, DEFAULT_READING_FONT_SIZE)
  const contentScale = setContentFontScale(readingFontSize)
  document.documentElement.style.setProperty('--nova-reading-font-family', fontStackFor(settings.readingFont, 'apple-system'))
  setCSSPixels('--nova-reading-font-size', readingFontSize)
  setCSSPixels('--nova-source-editor-font-size', contentScale.sourceEditor)
  setCSSPixels('--nova-terminal-font-size', contentScale.terminal)
}

function clampFontSize(value: unknown, min: number, max: number, fallback: number) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, Math.round(numberValue)))
}

function scaleTypeToken(size: number, lineHeight: number, ratio: number) {
  return {
    size: Math.max(10, roundPixels(size * ratio)),
    lineHeight: Math.max(14, roundPixels(lineHeight * ratio)),
  }
}

function setTypeToken(variable: string, token: { size: number; lineHeight: number }) {
  setCSSPixels(`${variable}-font-size`, token.size)
  setCSSPixels(`${variable}-line-height`, token.lineHeight)
}

function setCSSPixels(variable: string, value: number) {
  document.documentElement.style.setProperty(variable, `${value}px`)
}

function roundPixels(value: number) {
  return Math.round(value * 10) / 10
}
