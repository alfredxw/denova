import { customFontFamilyFromValue, customFontValue } from './font-options'

interface LocalFontData {
  family: string
  fullName: string
  postscriptName: string
  style: string
}

type LocalFontWindow = Window & {
  queryLocalFonts?: () => Promise<LocalFontData[]>
}

export type LocalFontQueryResult =
  | { status: 'ready'; families: string[] }
  | { status: 'unsupported' | 'denied' | 'failed'; families: [] }

/** Reads only family names and tolerates malformed individual browser results. */
export async function queryLocalFontFamilies(): Promise<LocalFontQueryResult> {
  const queryLocalFonts = typeof window === 'undefined'
    ? undefined
    : (window as LocalFontWindow).queryLocalFonts
  if (typeof queryLocalFonts !== 'function') {
    return { status: 'unsupported', families: [] }
  }

  try {
    const fonts = await queryLocalFonts.call(window)
    const familiesByIdentity = new Map<string, string>()
    for (const font of fonts) {
      const value = typeof font?.family === 'string' ? customFontValue(font.family) : null
      const family = customFontFamilyFromValue(value)
      if (!family) continue
      const identity = family.toLowerCase()
      if (!familiesByIdentity.has(identity)) familiesByIdentity.set(identity, family)
    }
    return {
      status: 'ready',
      families: [...familiesByIdentity.values()].sort((left, right) => left.localeCompare(right, undefined, { sensitivity: 'base' })),
    }
  } catch (cause) {
    const name = cause instanceof DOMException ? cause.name : ''
    if (name === 'NotAllowedError') return { status: 'denied', families: [] }
    if (name === 'SecurityError') return { status: 'unsupported', families: [] }
    console.warn('[local-fonts.ts] failed to enumerate local font families; manual entry remains available', cause)
    return { status: 'failed', families: [] }
  }
}
