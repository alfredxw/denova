import type { TFunction } from 'i18next'
import type { Teller } from './types'

export const DEFAULT_NARRATIVE_STYLE_ID = 'rhythm'

export type NarrativeStyleMode = 'writing' | 'game'

const ALL_NARRATIVE_STYLE_MODES: NarrativeStyleMode[] = ['writing', 'game']
const BUILTIN_NARRATIVE_STYLE_IDS = new Set(['rhythm', 'classic', 'screenwriter', 'grimdark', 'direct-erotica'])

/** Missing or unusable mode metadata remains compatible with legacy shared styles. */
export function narrativeStyleModes(teller: Pick<Teller, 'modes'>): NarrativeStyleMode[] {
  const modes = new Set((teller.modes || []).filter((mode): mode is NarrativeStyleMode => mode === 'writing' || mode === 'game'))
  return modes.size > 0 ? ALL_NARRATIVE_STYLE_MODES.filter((mode) => modes.has(mode)) : [...ALL_NARRATIVE_STYLE_MODES]
}

export function narrativeStyleSupportsMode(teller: Pick<Teller, 'modes'>, mode: NarrativeStyleMode): boolean {
  return narrativeStyleModes(teller).includes(mode)
}

export function narrativeStylesForMode(tellers: Teller[], mode: NarrativeStyleMode): Teller[] {
  return tellers.filter((teller) => narrativeStyleSupportsMode(teller, mode))
}

/** Resolve an available selection without depending on backend list ordering. */
export function resolveNarrativeStyle(tellers: Teller[], requestedID: string | undefined, mode: NarrativeStyleMode): Teller | undefined {
  const available = narrativeStylesForMode(tellers, mode)
  return available.find((teller) => teller.id === requestedID)
    ?? available.find((teller) => teller.id === DEFAULT_NARRATIVE_STYLE_ID)
    ?? available[0]
}

export function narrativeStyleName(teller: Teller, t: TFunction): string {
  if (!isUntouchedBuiltin(teller)) return teller.name || teller.id
  return t(`narrativeStyle.builtin.${teller.id}.name`)
}

export function narrativeStyleDescription(teller: Teller, t: TFunction): string {
  if (!isUntouchedBuiltin(teller)) return teller.description || ''
  return t(`narrativeStyle.builtin.${teller.id}.description`)
}

function isUntouchedBuiltin(teller: Teller): boolean {
  return BUILTIN_NARRATIVE_STYLE_IDS.has(teller.id) && !teller.custom && !teller.builtin_overridden
}
