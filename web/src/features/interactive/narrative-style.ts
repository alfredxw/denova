import type { TFunction } from 'i18next'
import type { Teller } from './types'

export const DEFAULT_NARRATIVE_STYLE_ID = 'rhythm'

const BUILTIN_NARRATIVE_STYLE_IDS = new Set(['rhythm', 'classic', 'screenwriter', 'grimdark', 'direct-erotica'])

/** Resolve an available selection without depending on backend list ordering. */
export function resolveNarrativeStyle(tellers: Teller[], requestedID: string | undefined): Teller | undefined {
  return tellers.find((teller) => teller.id === requestedID)
    ?? tellers.find((teller) => teller.id === DEFAULT_NARRATIVE_STYLE_ID)
    ?? tellers[0]
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
